package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// PermChecker is the interface Hub uses to check resource visibility.
// Implemented by api.PermissionSet. nil means admin (see all).
type PermChecker interface {
	CanSeeServer(hostID string) bool
	CanSeeProbe(probeID string) bool
	CanSeeDatabase(hostID string) bool
	CanSeeAlertTarget(ruleType, targetID string) bool
	CanSeeLogSource(source string) bool
}

// client wraps a websocket conn with its own write mutex
type client struct {
	conn      *websocket.Conn
	mu        sync.Mutex
	userID    int64
	role      string
	perm      PermChecker // nil = admin (see all)
	logSub    bool
	logFilter string
}

func (c *client) writeMessage(msgType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(msgType, data)
}

func (c *client) isAdmin() bool {
	return c.role == "admin"
}

// alertTarget records target info for filtering resolved/acked broadcasts.
type alertTarget struct {
	RuleType string
	TargetID string
}

type Hub struct {
	mu           sync.RWMutex
	clients      map[*client]bool
	userClients  map[int64][]*client       // user_id → connections
	alertTargets map[int]alertTarget        // event_id → target info
	atMu         sync.RWMutex              // protects alertTargets
}

func NewHub() *Hub {
	return &Hub{
		clients:      make(map[*client]bool),
		userClients:  make(map[int64][]*client),
		alertTargets: make(map[int]alertTarget),
	}
}

// HandleWS accepts a websocket connection (legacy, no auth info).
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	h.HandleWSWithAuth(w, r, 0, "admin", nil)
}

// HandleWSWithAuth accepts a websocket connection with user auth info.
func (h *Hub) HandleWSWithAuth(w http.ResponseWriter, r *http.Request, userID int64, role string, perm PermChecker) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}
	c := &client{conn: conn, userID: userID, role: role, perm: perm}
	h.mu.Lock()
	h.clients[c] = true
	h.userClients[userID] = append(h.userClients[userID], c)
	h.mu.Unlock()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			h.removeClient(c)
			conn.Close()
			break
		}
		var wsMsg struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(msg, &wsMsg) == nil {
			switch wsMsg.Type {
			case "log_subscribe":
				c.mu.Lock()
				c.logSub = true
				c.mu.Unlock()
			case "log_unsubscribe":
				c.mu.Lock()
				c.logSub = false
				c.mu.Unlock()
			}
		}
	}
}

func (h *Hub) removeClient(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
	conns := h.userClients[c.userID]
	for i, cc := range conns {
		if cc == c {
			h.userClients[c.userID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}
	if len(h.userClients[c.userID]) == 0 {
		delete(h.userClients, c.userID)
	}
}

// --- Typed broadcast methods ---

// BroadcastMetrics sends metrics to clients that can see the given server.
func (h *Hub) BroadcastMetrics(hostID string, msg interface{}) {
	h.broadcastFiltered(msg, func(c *client) bool {
		if c.perm == nil {
			return true
		}
		return c.perm.CanSeeServer(hostID)
	})
}

// BroadcastAlertFiring broadcasts a new alert and records target info for later resolved/acked.
func (h *Hub) BroadcastAlertFiring(eventID int, ruleType, targetID string, msg interface{}) {
	h.atMu.Lock()
	h.alertTargets[eventID] = alertTarget{RuleType: ruleType, TargetID: targetID}
	h.atMu.Unlock()

	h.broadcastFiltered(msg, func(c *client) bool {
		if c.perm == nil {
			return true
		}
		return c.perm.CanSeeAlertTarget(ruleType, targetID)
	})
}

// BroadcastAlertResolved broadcasts alert resolved, filtered by stored target info.
func (h *Hub) BroadcastAlertResolved(eventID int, msg interface{}) {
	h.atMu.RLock()
	at, ok := h.alertTargets[eventID]
	h.atMu.RUnlock()

	h.broadcastFiltered(msg, func(c *client) bool {
		if c.perm == nil {
			return true
		}
		if !ok {
			return false // unknown target, don't broadcast to non-admin
		}
		return c.perm.CanSeeAlertTarget(at.RuleType, at.TargetID)
	})

	if ok {
		h.atMu.Lock()
		delete(h.alertTargets, eventID)
		h.atMu.Unlock()
	}
}

// BroadcastAlertAcked broadcasts alert acknowledged, filtered by stored target info.
func (h *Hub) BroadcastAlertAcked(eventID int, msg interface{}) {
	h.atMu.RLock()
	at, ok := h.alertTargets[eventID]
	h.atMu.RUnlock()

	h.broadcastFiltered(msg, func(c *client) bool {
		if c.perm == nil {
			return true
		}
		if !ok {
			return false
		}
		return c.perm.CanSeeAlertTarget(at.RuleType, at.TargetID)
	})
}

// BroadcastLog sends log messages to subscribed clients that can see the source.
func (h *Hub) BroadcastLog(source string, msg interface{}) {
	h.broadcastFiltered(msg, func(c *client) bool {
		c.mu.Lock()
		sub := c.logSub
		c.mu.Unlock()
		if !sub {
			return false
		}
		if c.perm == nil {
			return true
		}
		return c.perm.CanSeeLogSource(source)
	})
}

// BroadcastAuditLog sends audit log messages to admin clients only.
func (h *Hub) BroadcastAuditLog(msg interface{}) {
	h.broadcastFiltered(msg, func(c *client) bool {
		return c.isAdmin()
	})
}

// BroadcastAdmin sends messages to admin clients only (deploy progress, cloud sync, etc.)
func (h *Hub) BroadcastAdmin(msg interface{}) {
	h.broadcastFiltered(msg, func(c *client) bool {
		return c.isAdmin()
	})
}

// LoadAlertTargets pre-loads firing alert targets on startup.
func (h *Hub) LoadAlertTargets(targets map[int]alertTarget) {
	h.atMu.Lock()
	defer h.atMu.Unlock()
	for id, at := range targets {
		h.alertTargets[id] = at
	}
}

// DisconnectUser force-closes all connections for a user.
func (h *Hub) DisconnectUser(userID int64) {
	h.mu.RLock()
	conns := make([]*client, len(h.userClients[userID]))
	copy(conns, h.userClients[userID])
	h.mu.RUnlock()

	for _, c := range conns {
		c.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session revoked"))
		c.conn.Close()
		h.removeClient(c)
	}
}

// UpdateUserPermissions hot-updates the PermissionSet for all active connections of a user.
func (h *Hub) UpdateUserPermissions(userID int64, perm PermChecker) {
	h.mu.RLock()
	conns := h.userClients[userID]
	h.mu.RUnlock()

	for _, c := range conns {
		c.mu.Lock()
		c.perm = perm
		c.mu.Unlock()
	}
}

// --- internal helpers ---

func (h *Hub) broadcastFiltered(msg interface{}, filter func(*client) bool) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.RLock()
	targets := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		if filter(c) {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()

	if len(targets) == 0 {
		return
	}

	var failed []*client
	for _, c := range targets {
		if err := c.writeMessage(websocket.TextMessage, data); err != nil {
			failed = append(failed, c)
		}
	}
	if len(failed) > 0 {
		for _, c := range failed {
			h.removeClient(c)
			c.conn.Close()
		}
	}
}

// --- Legacy compatibility (kept for BroadcastLogJSON callers during migration) ---

// BroadcastLogJSON sends a log message only to clients that have subscribed to logs.
// Deprecated: use BroadcastLog with source filtering instead.
func (h *Hub) BroadcastLogJSON(msg interface{}) {
	h.broadcastFiltered(msg, func(c *client) bool {
		c.mu.Lock()
		sub := c.logSub
		c.mu.Unlock()
		return sub
	})
}

// extractHostIDFromSource extracts host_id from "agent:{host_id}" source string.
func extractHostIDFromSource(source string) string {
	if strings.HasPrefix(source, "agent:") {
		return strings.TrimPrefix(source, "agent:")
	}
	return ""
}
