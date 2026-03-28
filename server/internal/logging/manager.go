package logging

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"mantisops/server/internal/ws"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "info"
	}
}

type logEntry struct {
	ts     time.Time
	level  string
	module string
	msg    string
	hostID string // empty for system logs
}

type LogManager struct {
	ch           chan logEntry
	systemWriter *RotatingWriter
	auditWriter  *RotatingWriter
	agentMu      sync.Mutex
	agentWriters map[string]*RotatingWriter // host_id → writer
	store        *LogStore
	hub          *ws.Hub
	level        Level
	logDir       string
	done         chan struct{}
	hooked       bool // true after HookStdLog() — emit writes to stderr directly
}

func NewLogManager(store *LogStore, hub *ws.Hub, level Level, logDir string) (*LogManager, error) {
	sysWriter, err := NewRotatingWriter(logDir, "system")
	if err != nil {
		return nil, fmt.Errorf("create system writer: %w", err)
	}
	auditWriter, err := NewRotatingWriter(logDir, "audit")
	if err != nil {
		sysWriter.Close()
		return nil, fmt.Errorf("create audit writer: %w", err)
	}

	m := &LogManager{
		ch:           make(chan logEntry, 4096),
		systemWriter: sysWriter,
		auditWriter:  auditWriter,
		agentWriters: make(map[string]*RotatingWriter),
		store:        store,
		hub:          hub,
		level:        level,
		logDir:       logDir,
		done:         make(chan struct{}),
	}
	go m.processLoop()
	return m, nil
}

func (m *LogManager) Close() {
	close(m.ch)
	<-m.done
	m.systemWriter.Close()
	m.auditWriter.Close()
	m.agentMu.Lock()
	for _, w := range m.agentWriters {
		w.Close()
	}
	m.agentMu.Unlock()
}

func (m *LogManager) Debug(module, msg string, args ...any) {
	m.emit(LevelDebug, module, "", msg, args...)
}

func (m *LogManager) Info(module, msg string, args ...any) {
	m.emit(LevelInfo, module, "", msg, args...)
}

func (m *LogManager) Warn(module, msg string, args ...any) {
	m.emit(LevelWarn, module, "", msg, args...)
}

func (m *LogManager) Error(module, msg string, args ...any) {
	m.emit(LevelError, module, "", msg, args...)
}

func (m *LogManager) emit(level Level, module, hostID, msg string, args ...any) {
	if level < m.level {
		return
	}
	formatted := msg
	if len(args) > 0 {
		formatted = fmt.Sprintf(msg, args...)
	}
	entry := logEntry{
		ts:     time.Now(),
		level:  level.String(),
		module: module,
		msg:    formatted,
		hostID: hostID,
	}

	// stdout/stderr (always) — use Stderr directly if hooked to avoid recursion
	if m.hooked {
		fmt.Fprintf(os.Stderr, "[%s] %s\n", module, formatted)
	} else {
		log.Printf("[%s] %s", module, formatted)
	}

	// async channel — drop DEBUG if full, block for WARN/ERROR
	if level >= LevelWarn {
		m.ch <- entry
	} else {
		select {
		case m.ch <- entry:
		default:
		}
	}
}

// HookStdLog redirects the standard Go logger output into LogManager.
// Existing log.Printf("[module] msg") calls get parsed and routed automatically.
// Also writes to stderr so systemd journal still captures output.
func (m *LogManager) HookStdLog() {
	m.hooked = true
	log.SetOutput(&stdLogWriter{mgr: m})
	log.SetFlags(0) // LogManager handles timestamps
}

type stdLogWriter struct {
	mgr *LogManager
}

func (w *stdLogWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}

	// Write to stderr for systemd journal
	os.Stderr.Write(p)

	// Parse "[module] message" or "[GIN] ..." format
	module := "server"
	level := LevelInfo
	text := msg

	if len(msg) > 2 && msg[0] == '[' {
		if end := strings.Index(msg, "]"); end > 0 {
			module = strings.ToLower(msg[1:end])
			text = strings.TrimSpace(msg[end+1:])
		}
	}

	// Detect level from common patterns
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "error") || strings.Contains(lower, "error:") || strings.Contains(lower, "failed") {
		level = LevelError
	} else if strings.HasPrefix(lower, "warn") {
		level = LevelWarn
	}

	// Skip GIN request logs from being stored (too noisy)
	if module == "gin" {
		return len(p), nil
	}

	entry := logEntry{
		ts:     time.Now(),
		level:  level.String(),
		module: module,
		msg:    text,
	}
	select {
	case w.mgr.ch <- entry:
	default:
	}
	return len(p), nil
}

// AgentLogEntry represents a log entry received from an Agent via gRPC.
type AgentLogEntry struct {
	Timestamp int64
	Level     string
	Module    string
	Message   string
}

// IngestAgentLogs handles a batch of logs from an Agent.
func (m *LogManager) IngestAgentLogs(hostID string, entries []AgentLogEntry) {
	for _, e := range entries {
		lvl := ParseLevel(e.Level)
		if lvl < m.level {
			continue
		}
		entry := logEntry{
			ts:     time.UnixMilli(e.Timestamp),
			level:  e.Level,
			module: e.Module,
			msg:    e.Message,
			hostID: hostID,
		}
		select {
		case m.ch <- entry:
		default:
		}
	}
}

// Audit writes an audit log synchronously (bypasses channel).
func (m *LogManager) Audit(username, action, resourceType, resourceID, resourceName, detail, ip, ua string) {
	now := time.Now()

	// Write to file
	line := map[string]interface{}{
		"ts":            now.Format(time.RFC3339Nano),
		"type":          "audit",
		"username":      username,
		"action":        action,
		"resource_type": resourceType,
		"resource_id":   resourceID,
		"resource_name": resourceName,
		"ip":            ip,
	}
	data, _ := json.Marshal(line)
	m.auditWriter.Write(data)

	// Write to DB (synchronous)
	m.store.InsertAudit(now, username, action, resourceType, resourceID, resourceName, detail, ip, ua)

	// stdout
	log.Printf("[audit] %s %s %s/%s (%s) from %s", username, action, resourceType, resourceID, resourceName, ip)

	// WebSocket broadcast
	if m.hub != nil {
		m.hub.BroadcastAuditLog(map[string]interface{}{
			"type": "audit_log",
			"data": line,
		})
	}
}

func (m *LogManager) getAgentWriter(hostID string) *RotatingWriter {
	m.agentMu.Lock()
	defer m.agentMu.Unlock()
	w, ok := m.agentWriters[hostID]
	if ok {
		return w
	}
	w, err := NewRotatingWriter(m.logDir, "agent/"+hostID)
	if err != nil {
		log.Printf("[logging] create agent writer for %s: %v", hostID, err)
		return nil
	}
	m.agentWriters[hostID] = w
	return w
}

func (m *LogManager) processLoop() {
	defer close(m.done)

	batch := make([]logEntry, 0, 100)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		for _, e := range batch {
			source := "server"
			var writer *RotatingWriter
			if e.hostID != "" {
				source = "agent:" + e.hostID
				writer = m.getAgentWriter(e.hostID)
			} else {
				writer = m.systemWriter
			}

			// Build JSON line
			lineMap := map[string]string{
				"ts":     e.ts.Format(time.RFC3339Nano),
				"level":  e.level,
				"module": e.module,
				"msg":    e.msg,
			}
			if e.hostID != "" {
				lineMap["host_id"] = e.hostID
			}
			data, _ := json.Marshal(lineMap)

			// Write file
			var relPath string
			var offset int64
			var lineLen int
			if writer != nil {
				off, ln, err := writer.Write(data)
				if err != nil {
					log.Printf("[logging] write error: %v", err)
					continue
				}
				relPath = writer.FilePath()
				offset = off
				lineLen = ln
			}

			// Index
			preview := e.msg
			if len(preview) > 200 {
				preview = preview[:200]
			}
			m.store.InsertLogIndex(e.ts, e.level, e.module, source, relPath, offset, lineLen, preview)

			// WebSocket
			if m.hub != nil {
				m.hub.BroadcastLog(source, map[string]interface{}{
					"type": "log",
					"data": map[string]string{
						"ts":     e.ts.Format(time.RFC3339Nano),
						"level":  e.level,
						"module": e.module,
						"source": source,
						"msg":    e.msg,
					},
				})
			}
		}
		batch = batch[:0]
	}

	for {
		select {
		case entry, ok := <-m.ch:
			if !ok {
				flush()
				return
			}
			batch = append(batch, entry)
			if len(batch) >= 100 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
