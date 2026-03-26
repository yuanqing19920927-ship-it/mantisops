package alert

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"opsboard/server/internal/model"
	"opsboard/server/internal/store"
	"opsboard/server/internal/ws"
	pb "opsboard/server/proto/gen"
)

type ruleState struct {
	consecutiveHits    int
	consecutiveNormals int
	firing             bool
	silenced           bool
	eventID            int
	lastValue          float64
}

// Alerter is the core alert engine implementing evaluation loop, state machine, and notification dispatch.
type Alerter struct {
	store   *store.AlertStore
	hub     *ws.Hub
	metrics MetricsProvider
	probes  ProbeProvider
	servers ServerProvider
	mu      sync.RWMutex
	states  map[string]*ruleState
	stopCh  chan struct{}
}

// NewAlerter creates a new Alerter instance.
func NewAlerter(s *store.AlertStore, hub *ws.Hub, metrics MetricsProvider, probes ProbeProvider, servers ServerProvider) *Alerter {
	return &Alerter{
		store:   s,
		hub:     hub,
		metrics: metrics,
		probes:  probes,
		servers: servers,
		states:  make(map[string]*ruleState),
		stopCh:  make(chan struct{}),
	}
}

// Start initializes and starts the alerter loops.
func (a *Alerter) Start() {
	a.recoverState()
	if err := a.store.ResetStaleNotifications(); err != nil {
		log.Printf("[alerter] reset stale notifications: %v", err)
	}
	go a.loop()
	go a.notifyLoop()
}

// Stop signals the alerter to stop.
func (a *Alerter) Stop() {
	close(a.stopCh)
}

// recoverState loads firing events from DB to restore in-memory state.
func (a *Alerter) recoverState() {
	events, err := a.store.ListFiringEvents()
	if err != nil {
		log.Printf("[alerter] recover state: %v", err)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, e := range events {
		key := fmt.Sprintf("%d:%s", e.RuleID, e.TargetID)
		a.states[key] = &ruleState{
			firing:   true,
			silenced: e.Silenced,
			eventID:  e.ID,
		}
	}
	log.Printf("[alerter] recovered %d firing states", len(events))
}

// loop runs the main evaluation ticker.
func (a *Alerter) loop() {
	a.evaluate()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.evaluate()
		}
	}
}

// evaluate runs one full evaluation cycle.
func (a *Alerter) evaluate() {
	rules, err := a.store.ListEnabledRules()
	if err != nil {
		log.Printf("[alerter] list rules: %v", err)
		return
	}

	a.cleanupDisabledRules(rules)

	servers, err := a.servers.List()
	if err != nil {
		log.Printf("[alerter] list servers: %v", err)
		return
	}

	for _, rule := range rules {
		results := Evaluate(rule, servers, a.metrics, a.probes)
		for _, r := range results {
			if r.Skip {
				continue
			}
			a.processResult(rule, r)
		}
	}

	a.cleanupGoneTargets(servers)
}

// processResult implements the state machine for a single evaluation result.
func (a *Alerter) processResult(rule model.AlertRule, result EvalResult) {
	a.mu.Lock()
	defer a.mu.Unlock()

	st, ok := a.states[result.StateKey]
	if !ok {
		st = &ruleState{}
		a.states[result.StateKey] = st
	}
	st.lastValue = result.Value

	if result.Hit {
		st.consecutiveHits++
		st.consecutiveNormals = 0
		if st.consecutiveHits >= rule.Duration && !st.firing {
			a.fireAlert(rule, result, st)
		}
	} else {
		st.consecutiveNormals++
		st.consecutiveHits = 0
		if st.consecutiveNormals >= rule.Duration && st.firing {
			a.resolveAlert(st, "auto")
		}
	}
}

// fireAlert creates a new alert event and broadcasts it.
// Called with mu held.
func (a *Alerter) fireAlert(rule model.AlertRule, result EvalResult, st *ruleState) {
	event := &model.AlertEvent{
		RuleID:      rule.ID,
		RuleName:    rule.Name,
		TargetID:    result.TargetID,
		TargetLabel: result.Label,
		Level:       rule.Level,
		Value:       result.Value,
		Message:     result.Message,
		FiredAt:     time.Now(),
	}

	eventID, err := a.store.FireAlert(event)
	if err != nil {
		log.Printf("[alerter] fire alert rule=%d target=%s: %v", rule.ID, result.TargetID, err)
		return
	}

	st.firing = true
	st.eventID = int(eventID)
	event.ID = int(eventID)
	event.Status = "firing"

	a.hub.BroadcastJSON(map[string]interface{}{
		"type":  "alert",
		"event": event,
	})
	log.Printf("[alerter] FIRE rule=%d target=%s value=%.2f", rule.ID, result.TargetID, result.Value)
}

// resolveAlert resolves an existing alert event and broadcasts it.
// Called with mu held.
func (a *Alerter) resolveAlert(st *ruleState, resolveType string) {
	if err := a.store.ResolveAlert(st.eventID, resolveType); err != nil {
		log.Printf("[alerter] resolve alert event=%d: %v", st.eventID, err)
		return
	}

	a.hub.BroadcastJSON(map[string]interface{}{
		"type":     "alert_resolved",
		"event_id": st.eventID,
	})
	log.Printf("[alerter] RESOLVE event=%d type=%s", st.eventID, resolveType)

	st.firing = false
	st.silenced = false
	st.eventID = 0
	st.consecutiveHits = 0
	st.consecutiveNormals = 0
}

// cleanupDisabledRules removes non-firing states for rules that are no longer enabled.
func (a *Alerter) cleanupDisabledRules(enabledRules []model.AlertRule) {
	enabledIDs := make(map[int]bool, len(enabledRules))
	for _, r := range enabledRules {
		enabledIDs[r.ID] = true
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for key, st := range a.states {
		ruleID := extractRuleID(key)
		if ruleID > 0 && !enabledIDs[ruleID] && !st.firing {
			delete(a.states, key)
		}
	}
}

// cleanupGoneTargets resolves alerts for targets that no longer exist.
func (a *Alerter) cleanupGoneTargets(servers []model.Server) {
	serverIDs := make(map[string]bool, len(servers))
	for _, s := range servers {
		serverIDs[s.HostID] = true
	}

	// Collect disk mounts and container names per host
	hostDisks := make(map[string]map[string]bool)
	hostContainers := make(map[string]map[string]bool)
	for hostID := range serverIDs {
		m := a.metrics.GetLatestMetrics(hostID)
		if m != nil {
			hostDisks[hostID] = extractDiskMounts(m)
			hostContainers[hostID] = extractContainerNames(m)
		}
	}

	// Collect probe result IDs
	probeIDs := make(map[string]bool)
	for _, pr := range a.probes.GetAllResults() {
		probeIDs[fmt.Sprintf("%d", pr.RuleID)] = true
	}

	// Find firing states whose targets are gone
	type goneEntry struct {
		key     string
		eventID int
	}
	var gone []goneEntry

	a.mu.Lock()
	for key, st := range a.states {
		if !st.firing {
			continue
		}
		if !a.isTargetPresent(key, serverIDs, hostDisks, hostContainers, probeIDs) {
			gone = append(gone, goneEntry{key: key, eventID: st.eventID})
		}
	}
	a.mu.Unlock()

	// Resolve gone targets outside lock, then delete states
	for _, g := range gone {
		if err := a.store.ResolveAlert(g.eventID, "target_gone"); err != nil {
			log.Printf("[alerter] resolve gone target event=%d: %v", g.eventID, err)
			continue
		}
		a.hub.BroadcastJSON(map[string]interface{}{
			"type":     "alert_resolved",
			"event_id": g.eventID,
		})
		log.Printf("[alerter] RESOLVE (target_gone) event=%d key=%s", g.eventID, g.key)

		a.mu.Lock()
		delete(a.states, g.key)
		a.mu.Unlock()
	}
}

// isTargetPresent checks if the target referenced by a state key still exists.
// Called with mu held for reading.
func (a *Alerter) isTargetPresent(key string, serverIDs map[string]bool, hostDisks, hostContainers map[string]map[string]bool, probeIDs map[string]bool) bool {
	// State key format: "ruleID:targetID" or "ruleID:hostID:mount_or_container"
	parts := strings.SplitN(key, ":", 3)
	if len(parts) < 2 {
		return true // can't parse, assume present
	}

	targetID := parts[1]

	// Check if it's a probe target (pure numeric after ruleID)
	if len(parts) == 2 {
		if _, err := strconv.Atoi(targetID); err == nil {
			// Could be a probe target ID — check both server and probe
			if serverIDs[targetID] {
				return true
			}
			return probeIDs[targetID]
		}
		// Simple server target
		return serverIDs[targetID]
	}

	// 3-part key: ruleID:hostID:subTarget (disk mount or container name)
	hostID := parts[1]
	subTarget := parts[2]

	if !serverIDs[hostID] {
		return false
	}

	// Check disk mounts
	if mounts, ok := hostDisks[hostID]; ok {
		if mounts[subTarget] {
			return true
		}
	}
	// Check container names
	if containers, ok := hostContainers[hostID]; ok {
		if containers[subTarget] {
			return true
		}
	}

	// Sub-target not found in current metrics
	return false
}

// notifyLoop runs the notification dispatch ticker.
func (a *Alerter) notifyLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.processNotifications()
		}
	}
}

// processNotifications processes all pending notifications.
func (a *Alerter) processNotifications() {
	if err := a.store.ResetStaleNotifications(); err != nil {
		log.Printf("[alerter] reset stale notifications: %v", err)
	}

	pending, err := a.store.ListPendingNotifications()
	if err != nil {
		log.Printf("[alerter] list pending notifications: %v", err)
		return
	}

	for _, p := range pending {
		claimed, err := a.store.ClaimNotification(p.ID)
		if err != nil {
			log.Printf("[alerter] claim notification %d: %v", p.ID, err)
			continue
		}
		if !claimed {
			continue
		}

		ch := p.Channel
		evt := p.Event
		if err := SendNotification(&ch, &evt, p.NotifyType); err != nil {
			log.Printf("[alerter] send notification %d: %v", p.ID, err)
			_ = a.store.MarkNotificationFailed(p.ID, err.Error())
		} else {
			_ = a.store.MarkNotificationSent(p.ID)
		}
	}
}

// AckEvent acknowledges (silences) an alert event.
func (a *Alerter) AckEvent(eventID int, username string) error {
	if err := a.store.AckEvent(eventID, username); err != nil {
		return err
	}

	a.mu.Lock()
	for _, st := range a.states {
		if st.eventID == eventID {
			st.silenced = true
			break
		}
	}
	a.mu.Unlock()

	a.hub.BroadcastJSON(map[string]interface{}{
		"type":     "alert_acked",
		"event_id": eventID,
		"acked_by": username,
	})
	return nil
}

// OnRuleChanged resolves all firing events for a rule and clears related states.
func (a *Alerter) OnRuleChanged(ruleID int, resolveType string) {
	if err := a.store.ResolveEventsByRule(ruleID, resolveType); err != nil {
		log.Printf("[alerter] resolve events for rule %d: %v", ruleID, err)
	}

	prefix := fmt.Sprintf("%d:", ruleID)
	a.mu.Lock()
	for key, st := range a.states {
		if strings.HasPrefix(key, prefix) {
			if st.firing {
				a.hub.BroadcastJSON(map[string]interface{}{
					"type":     "alert_resolved",
					"event_id": st.eventID,
				})
			}
			delete(a.states, key)
		}
	}
	a.mu.Unlock()
}

// OnRuleUpdated resets consecutive counters for a rule so it re-evaluates from scratch.
func (a *Alerter) OnRuleUpdated(ruleID int) {
	prefix := fmt.Sprintf("%d:", ruleID)
	a.mu.Lock()
	defer a.mu.Unlock()
	for key, st := range a.states {
		if strings.HasPrefix(key, prefix) {
			st.consecutiveHits = 0
			st.consecutiveNormals = 0
		}
	}
}

// --- helpers ---

func extractRuleID(stateKey string) int {
	idx := strings.Index(stateKey, ":")
	if idx < 0 {
		return 0
	}
	id, err := strconv.Atoi(stateKey[:idx])
	if err != nil {
		return 0
	}
	return id
}

func extractDiskMounts(m *pb.MetricsPayload) map[string]bool {
	mounts := make(map[string]bool)
	for _, d := range m.Disks {
		mounts[d.MountPoint] = true
	}
	return mounts
}

func extractContainerNames(m *pb.MetricsPayload) map[string]bool {
	names := make(map[string]bool)
	for _, c := range m.Containers {
		names[c.Name] = true
	}
	return names
}
