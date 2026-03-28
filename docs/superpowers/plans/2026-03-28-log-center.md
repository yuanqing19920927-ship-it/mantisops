# 日志中心 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 MantisOps 添加完整的日志中心功能：操作审计、结构化系统日志、Agent 日志上报、前端日志查看/搜索/实时尾随/导出。

**Architecture:** 后端新增 `logging` 包，通过异步 channel 队列写入按天轮转的日志文件 + 独立 SQLite 索引库（logs.db）。Agent 通过 gRPC ReportLogs（含 batch_id 幂等协议）上报日志。前端新增日志中心页面（操作审计 Tab + 运行日志 Tab），通过 WebSocket 订阅实现实时尾随。

**Tech Stack:** Go (Gin + gRPC + SQLite), Protobuf, React 19 + TypeScript, WebSocket

**Spec:** `docs/superpowers/specs/2026-03-28-log-center-design.md`

---

## File Map

### 新建文件

| 文件 | 职责 |
|------|------|
| `server/internal/logging/manager.go` | LogManager：异步队列、日志级别、统一入口 |
| `server/internal/logging/writer.go` | RotatingWriter：按天轮转文件写入 |
| `server/internal/logging/store.go` | LogStore：logs.db 初始化、audit_logs 和 log_index 的 CRUD |
| `server/internal/logging/query.go` | 关键字搜索：索引预筛 + 文件回扫 |
| `server/internal/logging/cleanup.go` | 定时清理过期日志文件和索引 |
| `server/internal/logging/middleware.go` | Gin 审计中间件 |
| `server/internal/logging/manager_test.go` | LogManager 单元测试 |
| `server/internal/logging/store_test.go` | LogStore 单元测试 |
| `server/internal/logging/query_test.go` | 查询逻辑测试 |
| `server/internal/api/log_handler.go` | 日志 API handler（6 个端点） |
| `agent/internal/reporter/logbuffer.go` | Agent 日志本地缓冲 + 批量上报 |
| `agent/internal/reporter/logbuffer_test.go` | 缓冲测试 |
| `web/src/pages/Logs/index.tsx` | 日志中心页面 |
| `web/src/api/logs.ts` | 日志 API 客户端 |

### 修改文件

| 文件 | 改动 |
|------|------|
| `proto/agent.proto` | 新增 LogEntry, ReportLogsRequest/Response, ReportLogs RPC |
| `server/proto/gen/agent.pb.go` | 重新生成 |
| `server/proto/gen/agent_grpc.pb.go` | 重新生成 |
| `agent/proto/gen/agent.pb.go` | 重新生成 |
| `agent/proto/gen/agent_grpc.pb.go` | 重新生成 |
| `server/internal/config/config.go` | 新增 LoggingConfig 结构体 |
| `server/internal/ws/hub.go` | 新增日志订阅状态 + 过滤广播 |
| `server/internal/grpc/handler.go` | 新增 ReportLogs handler |
| `server/internal/api/router.go` | RouterDeps 新增 LogHandler，注册 /logs/* 路由 |
| `server/cmd/server/main.go` | 初始化 LogManager，注入各模块 |
| `agent/internal/reporter/grpc.go` | RunLoop 新增日志上报 ticker |
| `agent/cmd/agent/main.go` | 初始化 logbuffer |
| `web/src/App.tsx` | 新增 /logs 路由 |
| `web/src/components/Layout/Sidebar.tsx` | 新增日志中心菜单项 |
| `web/src/hooks/useWebSocket.ts` | 新增 log 消息处理 + subscribe/unsubscribe |
| `server/configs/server.yaml` | 新增 logging 配置段 |
| `server/configs/server.yaml.example` | 同上 |
| `server/internal/api/auth.go` | Login handler 设置 audit_username 到 context |

---

## Chunk 1: 后端日志核心（logging 包 + 配置 + 测试）

### Task 1: 配置结构体

**Files:**
- Modify: `server/internal/config/config.go`
- Modify: `server/configs/server.yaml`
- Modify: `server/configs/server.yaml.example`

- [ ] **Step 1: 添加 LoggingConfig 到 config.go**

在 Config 结构体中新增字段，在文件末尾新增结构体：

```go
// 在 Config 结构体中新增：
Logging LoggingConfig `yaml:"logging"`

// 新增结构体：
type LoggingConfig struct {
	Dir        string          `yaml:"dir"`
	Level      string          `yaml:"level"`
	BufferSize int             `yaml:"buffer_size"`
	Retention  RetentionConfig `yaml:"retention"`
	CleanupHour int            `yaml:"cleanup_hour"`
}

type RetentionConfig struct {
	AuditDays  int `yaml:"audit_days"`
	SystemDays int `yaml:"system_days"`
	AgentDays  int `yaml:"agent_days"`
}
```

- [ ] **Step 2: 在 Load 函数中设置默认值**

在 `config.Load()` 返回前添加默认值处理：

```go
if cfg.Logging.Dir == "" {
	cfg.Logging.Dir = "./logs"
}
if cfg.Logging.Level == "" {
	cfg.Logging.Level = "info"
}
if cfg.Logging.BufferSize <= 0 {
	cfg.Logging.BufferSize = 4096
}
if cfg.Logging.Retention.AuditDays <= 0 {
	cfg.Logging.Retention.AuditDays = 90
}
if cfg.Logging.Retention.SystemDays <= 0 {
	cfg.Logging.Retention.SystemDays = 30
}
if cfg.Logging.Retention.AgentDays <= 0 {
	cfg.Logging.Retention.AgentDays = 7
}
if cfg.Logging.CleanupHour < 0 || cfg.Logging.CleanupHour > 23 {
	cfg.Logging.CleanupHour = 3
}
```

- [ ] **Step 3: 更新 server.yaml 和 server.yaml.example**

在两个文件末尾追加：

```yaml
logging:
  dir: "./logs"
  level: "info"
  buffer_size: 4096
  retention:
    audit_days: 90
    system_days: 30
    agent_days: 7
  cleanup_hour: 3
```

- [ ] **Step 4: 编译验证**

Run: `cd server && go build ./...`
Expected: 编译成功

- [ ] **Step 5: 提交**

```bash
git add server/internal/config/config.go server/configs/server.yaml server/configs/server.yaml.example
git commit -m "feat(logging): add logging config struct and defaults"
```

---

### Task 2: RotatingWriter（按天轮转文件写入器）

**Files:**
- Create: `server/internal/logging/writer.go`

- [ ] **Step 1: 实现 RotatingWriter**

```go
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RotatingWriter writes log lines to daily-rotated files.
// Files are organized as: {baseDir}/{category}/{YYYY-MM-DD}.log
// For agent logs: {baseDir}/agent/{hostID}/{YYYY-MM-DD}.log
type RotatingWriter struct {
	baseDir string
	mu      sync.Mutex
	files   map[string]*os.File // key: full file path
}

func NewRotatingWriter(baseDir string) *RotatingWriter {
	return &RotatingWriter{
		baseDir: baseDir,
		files:   make(map[string]*os.File),
	}
}

// Write appends a line to the appropriate daily log file.
// Returns the file path (relative to baseDir) and byte offset before write.
func (w *RotatingWriter) Write(category, hostID string, line []byte) (relPath string, offset int64, err error) {
	now := time.Now()
	dateStr := now.Format("2006-01-02")

	var dir string
	if hostID != "" {
		dir = filepath.Join(w.baseDir, category, hostID)
		relPath = filepath.Join(category, hostID, dateStr+".log")
	} else {
		dir = filepath.Join(w.baseDir, category)
		relPath = filepath.Join(category, dateStr+".log")
	}

	fullPath := filepath.Join(dir, dateStr+".log")

	w.mu.Lock()
	defer w.mu.Unlock()

	f, ok := w.files[fullPath]
	if !ok {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", 0, fmt.Errorf("mkdir %s: %w", dir, err)
		}
		f, err = os.OpenFile(fullPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return "", 0, fmt.Errorf("open %s: %w", fullPath, err)
		}
		w.files[fullPath] = f
	}

	// Get current offset before writing
	offset, err = f.Seek(0, os.SEEK_END)
	if err != nil {
		return "", 0, err
	}

	if _, err := f.Write(line); err != nil {
		return "", 0, err
	}

	return relPath, offset, nil
}

// Close closes all open file handles.
func (w *RotatingWriter) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, f := range w.files {
		f.Close()
	}
	w.files = make(map[string]*os.File)
}

// CloseStaleFiles closes file handles for dates older than today.
// Call periodically (e.g., daily) to prevent file handle leaks.
func (w *RotatingWriter) CloseStaleFiles() {
	today := time.Now().Format("2006-01-02")
	w.mu.Lock()
	defer w.mu.Unlock()
	for path, f := range w.files {
		if filepath.Base(path) != today+".log" {
			f.Close()
			delete(w.files, path)
		}
	}
}
```

- [ ] **Step 2: 编译验证**

Run: `cd server && go build ./internal/logging/`
Expected: 编译成功

- [ ] **Step 3: 提交**

```bash
git add server/internal/logging/writer.go
git commit -m "feat(logging): add RotatingWriter with daily file rotation"
```

---

### Task 3: LogStore（SQLite 索引库）

**Files:**
- Create: `server/internal/logging/store.go`
- Create: `server/internal/logging/store_test.go`

- [ ] **Step 1: 编写 store_test.go 测试**

```go
package logging

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLogStore_InitSchema(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	s, err := NewLogStore(db)
	if err != nil {
		t.Fatal(err)
	}
	_ = s
	// Verify tables exist
	var count int
	db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&count)
	db.QueryRow("SELECT COUNT(*) FROM log_index").Scan(&count)
}

func TestLogStore_InsertAudit(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	s, _ := NewLogStore(db)
	err := s.InsertAudit(time.Now(), "admin", "create", "alert_rule", "1", "CPU Alert", `{"threshold":80}`, "192.168.1.1", "Mozilla/5.0")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 audit log, got %d", count)
	}
}

func TestLogStore_InsertLogIndex(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	s, _ := NewLogStore(db)
	err := s.InsertLogIndex(time.Now(), "error", "alerter", "server", "system/2026-03-28.log", 1024, 150, "FIRE rule=3 target=srv-71")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM log_index").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 log index, got %d", count)
	}
}

func TestLogStore_QueryAudit(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	s, _ := NewLogStore(db)
	now := time.Now()
	s.InsertAudit(now, "admin", "create", "alert_rule", "1", "Rule1", "", "1.2.3.4", "")
	s.InsertAudit(now, "admin", "delete", "probe", "2", "Probe1", "", "1.2.3.4", "")
	s.InsertAudit(now, "user2", "update", "asset", "3", "Asset1", "", "5.6.7.8", "")

	results, total, err := s.QueryAudit(AuditQuery{
		Start: now.Add(-time.Hour), End: now.Add(time.Hour),
		Username: "admin",
		Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("expected 2, got %d", total)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestLogStore_QueryLogIndex(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	s, _ := NewLogStore(db)
	now := time.Now()
	s.InsertLogIndex(now, "error", "alerter", "server", "system/2026-03-28.log", 0, 100, "FIRE rule=3")
	s.InsertLogIndex(now, "info", "collector", "agent:srv-71", "agent/srv-71/2026-03-28.log", 100, 80, "collected 12 metrics")

	results, total, err := s.QueryLogIndex(LogQuery{
		Start: now.Add(-time.Hour), End: now.Add(time.Hour),
		Level: "error",
		Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("expected 1, got %d", total)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestLogStore_CleanupOld(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	s, _ := NewLogStore(db)
	old := time.Now().Add(-100 * 24 * time.Hour)
	recent := time.Now()
	s.InsertAudit(old, "admin", "create", "probe", "1", "old", "", "", "")
	s.InsertAudit(recent, "admin", "create", "probe", "2", "new", "", "", "")

	deleted, err := s.CleanupAuditBefore(time.Now().Add(-50 * 24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd server && go test ./internal/logging/ -v`
Expected: 编译错误（NewLogStore 等未定义）

- [ ] **Step 3: 实现 store.go**

```go
package logging

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type LogStore struct {
	db *sql.DB
}

func NewLogStore(db *sql.DB) (*LogStore, error) {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME NOT NULL,
			username TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT DEFAULT '',
			resource_name TEXT DEFAULT '',
			detail TEXT DEFAULT '',
			ip_address TEXT DEFAULT '',
			user_agent TEXT DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_logs(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_username ON audit_logs(username, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_resource ON audit_logs(resource_type, timestamp DESC)`,
		`CREATE TABLE IF NOT EXISTS log_index (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME NOT NULL,
			level TEXT NOT NULL,
			module TEXT NOT NULL,
			source TEXT NOT NULL,
			file_path TEXT NOT NULL,
			file_offset INTEGER NOT NULL,
			line_length INTEGER NOT NULL,
			message_preview TEXT DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_log_timestamp ON log_index(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_log_level ON log_index(level, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_log_module ON log_index(module, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_log_source ON log_index(source, timestamp DESC)`,
	}
	for _, ddl := range schema {
		if _, err := db.Exec(ddl); err != nil {
			return nil, fmt.Errorf("init log schema: %w", err)
		}
	}
	return &LogStore{db: db}, nil
}

func (s *LogStore) InsertAudit(ts time.Time, username, action, resourceType, resourceID, resourceName, detail, ip, ua string) error {
	_, err := s.db.Exec(`INSERT INTO audit_logs (timestamp, username, action, resource_type, resource_id, resource_name, detail, ip_address, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ts, username, action, resourceType, resourceID, resourceName, detail, ip, ua)
	return err
}

func (s *LogStore) InsertLogIndex(ts time.Time, level, module, source, filePath string, offset int64, lineLen int, preview string) error {
	_, err := s.db.Exec(`INSERT INTO log_index (timestamp, level, module, source, file_path, file_offset, line_length, message_preview)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ts, level, module, source, filePath, offset, lineLen, preview)
	return err
}

// --- Query types ---

type AuditQuery struct {
	Start        time.Time
	End          time.Time
	Username     string
	Action       string
	ResourceType string
	Page         int
	PageSize     int
}

type AuditRecord struct {
	ID           int       `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	Username     string    `json:"username"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	ResourceName string    `json:"resource_name"`
	Detail       string    `json:"detail"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
}

func (s *LogStore) QueryAudit(q AuditQuery) ([]AuditRecord, int, error) {
	where := []string{"timestamp BETWEEN ? AND ?"}
	args := []interface{}{q.Start, q.End}

	if q.Username != "" {
		where = append(where, "username = ?")
		args = append(args, q.Username)
	}
	if q.Action != "" {
		where = append(where, "action = ?")
		args = append(args, q.Action)
	}
	if q.ResourceType != "" {
		where = append(where, "resource_type = ?")
		args = append(args, q.ResourceType)
	}

	whereClause := strings.Join(where, " AND ")

	// Count
	var total int
	err := s.db.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE "+whereClause, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Query
	if q.PageSize <= 0 {
		q.PageSize = 50
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	offset := (q.Page - 1) * q.PageSize
	queryArgs := append(args, q.PageSize, offset)

	rows, err := s.db.Query(
		"SELECT id, timestamp, username, action, resource_type, resource_id, resource_name, detail, ip_address, user_agent FROM audit_logs WHERE "+whereClause+" ORDER BY timestamp DESC LIMIT ? OFFSET ?",
		queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []AuditRecord
	for rows.Next() {
		var r AuditRecord
		if err := rows.Scan(&r.ID, &r.Timestamp, &r.Username, &r.Action, &r.ResourceType, &r.ResourceID, &r.ResourceName, &r.Detail, &r.IPAddress, &r.UserAgent); err != nil {
			return nil, 0, err
		}
		results = append(results, r)
	}
	return results, total, nil
}

type LogQuery struct {
	Start    time.Time
	End      time.Time
	Level    string // comma-separated
	Module   string
	Source   string
	Page     int
	PageSize int
}

type LogIndexRecord struct {
	ID             int       `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	Level          string    `json:"level"`
	Module         string    `json:"module"`
	Source         string    `json:"source"`
	FilePath       string    `json:"file_path"`
	FileOffset     int64     `json:"file_offset"`
	LineLength     int       `json:"line_length"`
	MessagePreview string    `json:"message_preview"`
}

func (s *LogStore) QueryLogIndex(q LogQuery) ([]LogIndexRecord, int, error) {
	where := []string{"timestamp BETWEEN ? AND ?"}
	args := []interface{}{q.Start, q.End}

	if q.Level != "" {
		levels := strings.Split(q.Level, ",")
		placeholders := strings.Repeat("?,", len(levels))
		placeholders = placeholders[:len(placeholders)-1]
		where = append(where, "level IN ("+placeholders+")")
		for _, l := range levels {
			args = append(args, strings.TrimSpace(l))
		}
	}
	if q.Module != "" {
		where = append(where, "module = ?")
		args = append(args, q.Module)
	}
	if q.Source != "" {
		where = append(where, "source = ?")
		args = append(args, q.Source)
	}

	whereClause := strings.Join(where, " AND ")

	var total int
	err := s.db.QueryRow("SELECT COUNT(*) FROM log_index WHERE "+whereClause, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	if q.PageSize <= 0 {
		q.PageSize = 50
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	offset := (q.Page - 1) * q.PageSize
	queryArgs := append(args, q.PageSize, offset)

	rows, err := s.db.Query(
		"SELECT id, timestamp, level, module, source, file_path, file_offset, line_length, message_preview FROM log_index WHERE "+whereClause+" ORDER BY timestamp DESC LIMIT ? OFFSET ?",
		queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []LogIndexRecord
	for rows.Next() {
		var r LogIndexRecord
		if err := rows.Scan(&r.ID, &r.Timestamp, &r.Level, &r.Module, &r.Source, &r.FilePath, &r.FileOffset, &r.LineLength, &r.MessagePreview); err != nil {
			return nil, 0, err
		}
		results = append(results, r)
	}
	return results, total, nil
}

func (s *LogStore) CleanupAuditBefore(before time.Time) (int64, error) {
	res, err := s.db.Exec("DELETE FROM audit_logs WHERE timestamp < ?", before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *LogStore) CleanupLogIndexBefore(before time.Time) (int64, error) {
	res, err := s.db.Exec("DELETE FROM log_index WHERE timestamp < ?", before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *LogStore) GetSources() ([]string, error) {
	rows, err := s.db.Query("SELECT DISTINCT source FROM log_index ORDER BY source")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sources []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		sources = append(sources, s)
	}
	return sources, nil
}

func (s *LogStore) GetStats(start, end time.Time) (map[string]int, error) {
	stats := make(map[string]int)
	rows, err := s.db.Query("SELECT level, COUNT(*) FROM log_index WHERE timestamp BETWEEN ? AND ? GROUP BY level", start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var level string
		var count int
		rows.Scan(&level, &count)
		stats[level] = count
	}
	return stats, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd server && go test ./internal/logging/ -v`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add server/internal/logging/store.go server/internal/logging/store_test.go
git commit -m "feat(logging): add LogStore with audit_logs and log_index tables"
```

---

### Task 4: LogManager（异步队列 + 统一入口）

**Files:**
- Create: `server/internal/logging/manager.go`
- Create: `server/internal/logging/manager_test.go`

- [ ] **Step 1: 编写 manager_test.go**

```go
package logging

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestLogManager_InfoWritesToFileAndIndex(t *testing.T) {
	dir := t.TempDir()
	db := testDB(t)
	defer db.Close()
	store, _ := NewLogStore(db)
	writer := NewRotatingWriter(dir)
	defer writer.Close()

	mgr := NewLogManager(writer, store, nil, LevelDebug, dir)
	defer mgr.Close()

	mgr.Info("alerter", "test message %d", 42)
	time.Sleep(2 * time.Second) // wait for async flush

	// Verify file written
	files, _ := filepath.Glob(filepath.Join(dir, "system", "*.log"))
	if len(files) == 0 {
		t.Fatal("expected log file to be created")
	}
	data, _ := os.ReadFile(files[0])
	if len(data) == 0 {
		t.Fatal("expected log file to have content")
	}

	// Verify index written
	results, total, _ := store.QueryLogIndex(LogQuery{
		Start: time.Now().Add(-time.Hour), End: time.Now().Add(time.Hour),
		Page: 1, PageSize: 10,
	})
	if total != 1 || len(results) != 1 {
		t.Fatalf("expected 1 index entry, got %d", total)
	}
	if results[0].Module != "alerter" {
		t.Fatalf("expected module=alerter, got %s", results[0].Module)
	}
}

func TestLogManager_LevelFiltering(t *testing.T) {
	dir := t.TempDir()
	db := testDB(t)
	defer db.Close()
	store, _ := NewLogStore(db)
	writer := NewRotatingWriter(dir)
	defer writer.Close()

	mgr := NewLogManager(writer, store, nil, LevelWarn, dir)
	defer mgr.Close()

	mgr.Debug("test", "should be dropped")
	mgr.Info("test", "should be dropped")
	mgr.Warn("test", "should be kept")
	mgr.Error("test", "should be kept")
	time.Sleep(2 * time.Second)

	_, total, _ := store.QueryLogIndex(LogQuery{
		Start: time.Now().Add(-time.Hour), End: time.Now().Add(time.Hour),
		Page: 1, PageSize: 10,
	})
	if total != 2 {
		t.Fatalf("expected 2 entries (warn+error), got %d", total)
	}
}

func TestLogManager_AuditIsSynchronous(t *testing.T) {
	dir := t.TempDir()
	db := testDB(t)
	defer db.Close()
	store, _ := NewLogStore(db)
	writer := NewRotatingWriter(dir)
	defer writer.Close()

	mgr := NewLogManager(writer, store, nil, LevelInfo, dir)
	defer mgr.Close()

	mgr.Audit("admin", "create", "alert_rule", "1", "CPU Alert", `{}`, "1.2.3.4", "")

	// Audit is synchronous, should be immediately queryable
	results, total, _ := store.QueryAudit(AuditQuery{
		Start: time.Now().Add(-time.Hour), End: time.Now().Add(time.Hour),
		Page: 1, PageSize: 10,
	})
	if total != 1 || len(results) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", total)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd server && go test ./internal/logging/ -run TestLogManager -v`
Expected: 编译错误（NewLogManager 未定义）

- [ ] **Step 3: 实现 manager.go**

```go
package logging

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
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
	ch     chan logEntry
	writer *RotatingWriter
	store  *LogStore
	hub    *ws.Hub
	level  Level
	logDir string
	done   chan struct{}
}

func NewLogManager(writer *RotatingWriter, store *LogStore, hub *ws.Hub, level Level, logDir string) *LogManager {
	m := &LogManager{
		ch:     make(chan logEntry, 4096),
		writer: writer,
		store:  store,
		hub:    hub,
		level:  level,
		logDir: logDir,
		done:   make(chan struct{}),
	}
	go m.processLoop()
	return m
}

func (m *LogManager) Close() {
	close(m.ch)
	<-m.done
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

	// stdout (always)
	prefix := fmt.Sprintf("[%s]", module)
	log.Printf("%s %s", prefix, formatted)

	// async channel — drop DEBUG if full, block for WARN/ERROR
	if level >= LevelWarn {
		m.ch <- entry // block
	} else {
		select {
		case m.ch <- entry:
		default:
			// channel full, drop non-critical log
		}
	}
}

// IngestAgentLogs handles a batch of logs from an Agent.
// Returns the acked batch_id.
func (m *LogManager) IngestAgentLogs(hostID string, entries []agentLogEntry) {
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

type agentLogEntry struct {
	Timestamp int64
	Level     string
	Module    string
	Message   string
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
	data = append(data, '\n')
	m.writer.Write("audit", "", data)

	// Write to DB (synchronous)
	m.store.InsertAudit(now, username, action, resourceType, resourceID, resourceName, detail, ip, ua)

	// stdout
	log.Printf("[audit] %s %s %s/%s (%s) from %s", username, action, resourceType, resourceID, resourceName, ip)

	// WebSocket broadcast
	if m.hub != nil {
		m.hub.BroadcastJSON(map[string]interface{}{
			"type": "audit_log",
			"data": line,
		})
	}
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
			// Determine category and source
			category := "system"
			source := "server"
			hostID := ""
			if e.hostID != "" {
				category = "agent"
				source = "agent:" + e.hostID
				hostID = e.hostID
			}

			// Build JSON line
			line := map[string]string{
				"ts":     e.ts.Format(time.RFC3339Nano),
				"level":  e.level,
				"module": e.module,
				"msg":    e.msg,
			}
			if e.hostID != "" {
				line["host_id"] = e.hostID
			}
			data, _ := json.Marshal(line)
			data = append(data, '\n')

			// Write file
			relPath, offset, err := m.writer.Write(category, hostID, data)
			if err != nil {
				log.Printf("[logging] write error: %v", err)
				continue
			}

			// Index
			preview := e.msg
			if len(preview) > 200 {
				preview = preview[:200]
			}
			m.store.InsertLogIndex(e.ts, e.level, e.module, source, relPath, offset, len(data), preview)

			// WebSocket
			if m.hub != nil {
				m.hub.BroadcastJSON(map[string]interface{}{
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
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd server && go test ./internal/logging/ -v -timeout 30s`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add server/internal/logging/manager.go server/internal/logging/manager_test.go
git commit -m "feat(logging): add LogManager with async queue, level filtering, audit"
```

---

### Task 5: 关键字搜索（索引预筛 + 文件回扫）

**Files:**
- Create: `server/internal/logging/query.go`
- Create: `server/internal/logging/query_test.go`

- [ ] **Step 1: 编写 query_test.go**

```go
package logging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestKeywordSearch_MatchesFullContent(t *testing.T) {
	dir := t.TempDir()
	db := testDB(t)
	defer db.Close()
	store, _ := NewLogStore(db)

	// Write a log file with known content
	logDir := filepath.Join(dir, "system")
	os.MkdirAll(logDir, 0755)
	logFile := filepath.Join(logDir, "2026-03-28.log")

	var lines [][]byte
	msgs := []string{
		"FIRE rule=3 target=srv-71 value=92.5 threshold=80 this is a very long message that exceeds 200 characters and contains the keyword CRITICAL_FAILURE somewhere in the middle of the text that would not appear in the preview",
		"collected 12 metrics from srv-71",
		"VM write error: connection refused to CRITICAL_FAILURE endpoint",
	}
	offset := int64(0)
	for i, msg := range msgs {
		line := map[string]string{"ts": time.Now().Format(time.RFC3339Nano), "level": "error", "module": "test", "msg": msg}
		data, _ := json.Marshal(line)
		data = append(data, '\n')
		lines = append(lines, data)

		preview := msg
		if len(preview) > 200 {
			preview = preview[:200]
		}
		store.InsertLogIndex(time.Now(), "error", "test", "server", "system/2026-03-28.log", offset, len(data), preview)
		offset += int64(len(data))
		_ = i
	}

	// Write all lines to file
	f, _ := os.Create(logFile)
	for _, l := range lines {
		f.Write(l)
	}
	f.Close()

	// Search for "CRITICAL_FAILURE" — line 0 has it beyond 200 chars, line 2 has it in preview
	searcher := NewLogSearcher(store, dir)
	results, err := searcher.Search(LogQuery{
		Start: time.Now().Add(-time.Hour), End: time.Now().Add(time.Hour),
		Page: 1, PageSize: 50,
	}, "CRITICAL_FAILURE")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results matching CRITICAL_FAILURE, got %d", len(results))
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd server && go test ./internal/logging/ -run TestKeywordSearch -v`
Expected: 编译错误

- [ ] **Step 3: 实现 query.go**

```go
package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxScanRows = 10000

type LogSearcher struct {
	store  *LogStore
	logDir string
}

func NewLogSearcher(store *LogStore, logDir string) *LogSearcher {
	return &LogSearcher{store: store, logDir: logDir}
}

type SearchResult struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Module    string    `json:"module"`
	Source    string    `json:"source"`
	Message   string    `json:"message"`
}

// Search performs a keyword search across full log content.
// 1. Query log_index with time/level/module/source filters (up to maxScanRows)
// 2. For each candidate, check message_preview first (fast path)
// 3. If preview doesn't match, read original file line and check full msg
func (s *LogSearcher) Search(q LogQuery, keyword string) ([]SearchResult, error) {
	if keyword == "" {
		return nil, fmt.Errorf("keyword is required for search")
	}

	keyword = strings.ToLower(keyword)

	// Get candidates from index (up to maxScanRows)
	q.Page = 1
	q.PageSize = maxScanRows
	candidates, _, err := s.store.QueryLogIndex(q)
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	for _, c := range candidates {
		// Fast path: check preview
		if strings.Contains(strings.ToLower(c.MessagePreview), keyword) {
			results = append(results, SearchResult{
				Timestamp: c.Timestamp,
				Level:     c.Level,
				Module:    c.Module,
				Source:    c.Source,
				Message:   c.MessagePreview,
			})
			continue
		}

		// Slow path: read full line from file
		fullMsg, err := s.readLogLine(c.FilePath, c.FileOffset, c.LineLength)
		if err != nil {
			continue // file might be cleaned up
		}
		if strings.Contains(strings.ToLower(fullMsg), keyword) {
			results = append(results, SearchResult{
				Timestamp: c.Timestamp,
				Level:     c.Level,
				Module:    c.Module,
				Source:    c.Source,
				Message:   fullMsg,
			})
		}
	}

	return results, nil
}

func (s *LogSearcher) readLogLine(relPath string, offset int64, length int) (string, error) {
	fullPath := filepath.Join(s.logDir, relPath)
	f, err := os.Open(fullPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	buf := make([]byte, length)
	_, err = f.ReadAt(buf, offset)
	if err != nil {
		return "", err
	}

	// Parse JSON to extract msg field
	var entry map[string]string
	if err := json.Unmarshal(buf, &entry); err != nil {
		return string(buf), nil // return raw if not valid JSON
	}
	return entry["msg"], nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd server && go test ./internal/logging/ -run TestKeywordSearch -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add server/internal/logging/query.go server/internal/logging/query_test.go
git commit -m "feat(logging): add keyword search with index pre-filter and file read-back"
```

---

### Task 6: 定时清理

**Files:**
- Create: `server/internal/logging/cleanup.go`

- [ ] **Step 1: 实现 cleanup.go**

```go
package logging

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mantisops/server/internal/config"
)

type Cleaner struct {
	logDir    string
	store     *LogStore
	retention config.RetentionConfig
	hour      int
	stopCh    chan struct{}
}

func NewCleaner(logDir string, store *LogStore, retention config.RetentionConfig, hour int) *Cleaner {
	return &Cleaner{
		logDir:    logDir,
		store:     store,
		retention: retention,
		hour:      hour,
		stopCh:    make(chan struct{}),
	}
}

func (c *Cleaner) Start() {
	go c.loop()
}

func (c *Cleaner) Stop() {
	close(c.stopCh)
}

func (c *Cleaner) loop() {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), c.hour, 0, 0, 0, now.Location())
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		timer := time.NewTimer(time.Until(next))

		select {
		case <-c.stopCh:
			timer.Stop()
			return
		case <-timer.C:
			c.run()
		}
	}
}

func (c *Cleaner) run() {
	log.Println("[logging] starting cleanup...")

	auditBefore := time.Now().Add(-time.Duration(c.retention.AuditDays) * 24 * time.Hour)
	systemBefore := time.Now().Add(-time.Duration(c.retention.SystemDays) * 24 * time.Hour)
	agentBefore := time.Now().Add(-time.Duration(c.retention.AgentDays) * 24 * time.Hour)

	// Clean DB
	auditDel, _ := c.store.CleanupAuditBefore(auditBefore)
	logDel, _ := c.store.CleanupLogIndexBefore(agentBefore) // use shortest retention for index

	// Clean files
	auditFiles := c.cleanDir(filepath.Join(c.logDir, "audit"), auditBefore)
	systemFiles := c.cleanDir(filepath.Join(c.logDir, "system"), systemBefore)
	agentFiles := c.cleanAgentDir(filepath.Join(c.logDir, "agent"), agentBefore)

	log.Printf("[logging] cleanup done: audit=%d/%d system=%d agent=%d index=%d",
		auditDel, auditFiles, systemFiles, agentFiles, logDel)
}

func (c *Cleaner) cleanDir(dir string, before time.Time) int {
	count := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		// Parse date from filename: 2026-03-28.log
		dateStr := strings.TrimSuffix(e.Name(), ".log")
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		if t.Before(before) {
			os.Remove(filepath.Join(dir, e.Name()))
			count++
		}
	}
	return count
}

func (c *Cleaner) cleanAgentDir(dir string, before time.Time) int {
	count := 0
	hostDirs, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, hd := range hostDirs {
		if !hd.IsDir() {
			continue
		}
		count += c.cleanDir(filepath.Join(dir, hd.Name()), before)
	}
	return count
}
```

- [ ] **Step 2: 编译验证**

Run: `cd server && go build ./internal/logging/`
Expected: 编译成功

- [ ] **Step 3: 提交**

```bash
git add server/internal/logging/cleanup.go
git commit -m "feat(logging): add daily log cleanup with configurable retention"
```

---

## Chunk 2: Gin 审计中间件 + API + Protobuf + Agent

### Task 7: Gin 审计中间件

**Files:**
- Create: `server/internal/logging/middleware.go`
- Modify: `server/internal/api/auth.go` (Login handler 设置 audit_username)

- [ ] **Step 1: 实现 middleware.go**

```go
package logging

import (
	"strings"

	"github.com/gin-gonic/gin"
)

type auditRoute struct {
	Method       string
	PathPrefix   string
	Action       string
	ResourceType string
}

var auditRoutes = []auditRoute{
	{"POST", "/api/v1/auth/login", "login", "auth"},
	{"POST", "/api/v1/alerts/rules", "create", "alert_rule"},
	{"PUT", "/api/v1/alerts/rules/", "update", "alert_rule"},
	{"DELETE", "/api/v1/alerts/rules/", "delete", "alert_rule"},
	{"PUT", "/api/v1/alerts/events/", "ack", "alert_event"},
	{"POST", "/api/v1/alerts/channels", "create", "channel"},
	{"PUT", "/api/v1/alerts/channels/", "update", "channel"},
	{"DELETE", "/api/v1/alerts/channels/", "delete", "channel"},
	{"POST", "/api/v1/alerts/channels/", "test", "channel"},
	{"POST", "/api/v1/probes", "create", "probe"},
	{"PUT", "/api/v1/probes/", "update", "probe"},
	{"DELETE", "/api/v1/probes/", "delete", "probe"},
	{"POST", "/api/v1/assets", "create", "asset"},
	{"PUT", "/api/v1/assets/", "update", "asset"},
	{"DELETE", "/api/v1/assets/", "delete", "asset"},
	{"POST", "/api/v1/cloud-accounts", "create", "cloud_account"},
	{"PUT", "/api/v1/cloud-accounts/", "update", "cloud_account"},
	{"DELETE", "/api/v1/cloud-accounts/", "delete", "cloud_account"},
	{"POST", "/api/v1/cloud-accounts/", "sync", "cloud_account"},
	{"POST", "/api/v1/managed-servers", "create", "managed_server"},
	{"DELETE", "/api/v1/managed-servers/", "delete", "managed_server"},
	{"POST", "/api/v1/managed-servers/", "deploy", "managed_server"},
	{"POST", "/api/v1/credentials", "create", "credential"},
	{"PUT", "/api/v1/credentials/", "update", "credential"},
	{"DELETE", "/api/v1/credentials/", "delete", "credential"},
	{"PUT", "/api/v1/servers/", "update", "server"},
	{"POST", "/api/v1/groups", "create", "group"},
	{"PUT", "/api/v1/groups/", "update", "group"},
	{"DELETE", "/api/v1/groups/", "delete", "group"},
}

func AuditMiddleware(mgr *LogManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next() // execute request first

		// Skip failed requests and GET
		if c.Writer.Status() >= 400 || c.Request.Method == "GET" || c.Request.Method == "OPTIONS" {
			return
		}

		path := c.Request.URL.Path
		method := c.Request.Method

		for _, r := range auditRoutes {
			if method != r.Method {
				continue
			}
			if r.PathPrefix == path || strings.HasPrefix(path, r.PathPrefix) {
				username, _ := c.Get("username")
				usernameStr, _ := username.(string)

				// Special case: login sets audit_username from request body
				if r.ResourceType == "auth" {
					if au, ok := c.Get("audit_username"); ok {
						usernameStr, _ = au.(string)
					}
				}

				resourceID := c.Param("id")

				mgr.Audit(
					usernameStr,
					r.Action,
					r.ResourceType,
					resourceID,
					"", // resource_name filled by handler if needed
					"",
					c.ClientIP(),
					c.Request.UserAgent(),
				)
				return
			}
		}
	}
}
```

- [ ] **Step 2: 修改 auth.go Login handler**

在 `server/internal/api/auth.go` 的 Login 方法中，bind 请求后添加一行将 username 存入 context：

```go
// 在 c.ShouldBindJSON(&req) 成功后立即添加：
c.Set("audit_username", req.Username)
```

- [ ] **Step 3: 编译验证**

Run: `cd server && go build ./...`
Expected: 编译成功

- [ ] **Step 4: 提交**

```bash
git add server/internal/logging/middleware.go server/internal/api/auth.go
git commit -m "feat(logging): add Gin audit middleware with route-action mapping"
```

---

### Task 8: 日志 API Handler

**Files:**
- Create: `server/internal/api/log_handler.go`
- Modify: `server/internal/api/router.go`

- [ ] **Step 1: 实现 log_handler.go**

```go
package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"mantisops/server/internal/logging"
)

type LogHandler struct {
	store    *logging.LogStore
	searcher *logging.LogSearcher
}

func NewLogHandler(store *logging.LogStore, searcher *logging.LogSearcher) *LogHandler {
	return &LogHandler{store: store, searcher: searcher}
}

func (h *LogHandler) ListAudit(c *gin.Context) {
	q := logging.AuditQuery{
		Start:        parseTime(c.Query("start"), time.Now().Add(-time.Hour)),
		End:          parseTime(c.Query("end"), time.Now()),
		Username:     c.Query("username"),
		Action:       c.Query("action"),
		ResourceType: c.Query("resource_type"),
		Page:         parseInt(c.Query("page"), 1),
		PageSize:     parseInt(c.Query("page_size"), 50),
	}
	if q.PageSize > 200 {
		q.PageSize = 200
	}
	results, total, err := h.store.QueryAudit(q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": results, "total": total, "page": q.Page, "page_size": q.PageSize})
}

func (h *LogHandler) ListRuntime(c *gin.Context) {
	q := logging.LogQuery{
		Start:    parseTime(c.Query("start"), time.Now().Add(-time.Hour)),
		End:      parseTime(c.Query("end"), time.Now()),
		Level:    c.Query("level"),
		Module:   c.Query("module"),
		Source:   c.Query("source"),
		Page:     parseInt(c.Query("page"), 1),
		PageSize: parseInt(c.Query("page_size"), 50),
	}
	if q.PageSize > 200 {
		q.PageSize = 200
	}

	keyword := c.Query("keyword")
	if keyword != "" {
		results, err := h.searcher.Search(q, keyword)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": results, "total": len(results)})
		return
	}

	results, total, err := h.store.QueryLogIndex(q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": results, "total": total, "page": q.Page, "page_size": q.PageSize})
}

func (h *LogHandler) Sources(c *gin.Context) {
	sources, err := h.store.GetSources()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sources)
}

func (h *LogHandler) Stats(c *gin.Context) {
	start := parseTime(c.Query("start"), time.Now().Add(-24*time.Hour))
	end := parseTime(c.Query("end"), time.Now())
	stats, err := h.store.GetStats(start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *LogHandler) Export(c *gin.Context) {
	format := c.DefaultQuery("format", "json")
	logType := c.DefaultQuery("type", "runtime")

	if logType == "audit" {
		q := logging.AuditQuery{
			Start:        parseTime(c.Query("start"), time.Now().Add(-time.Hour)),
			End:          parseTime(c.Query("end"), time.Now()),
			Username:     c.Query("username"),
			Action:       c.Query("action"),
			ResourceType: c.Query("resource_type"),
			Page:         1,
			PageSize:     10000,
		}
		results, _, err := h.store.QueryAudit(q)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		exportData(c, results, format, "audit-logs")
		return
	}

	// runtime
	q := logging.LogQuery{
		Start:    parseTime(c.Query("start"), time.Now().Add(-time.Hour)),
		End:      parseTime(c.Query("end"), time.Now()),
		Level:    c.Query("level"),
		Module:   c.Query("module"),
		Source:   c.Query("source"),
		Page:     1,
		PageSize: 10000,
	}
	results, _, err := h.store.QueryLogIndex(q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	exportData(c, results, format, "runtime-logs")
}

func exportData(c *gin.Context, data interface{}, format, filename string) {
	if format == "csv" {
		c.Header("Content-Disposition", "attachment; filename="+filename+".csv")
		c.Header("Content-Type", "text/csv")
		// Simple CSV: marshal as JSON and let client handle, or implement CSV serialization
		c.JSON(http.StatusOK, data) // TODO: proper CSV in future iteration
		return
	}
	c.Header("Content-Disposition", "attachment; filename="+filename+".json")
	c.Header("Content-Type", "application/json")
	c.JSON(http.StatusOK, data)
}

func parseTime(s string, def time.Time) time.Time {
	if s == "" {
		return def
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return def
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}
```

- [ ] **Step 2: 修改 router.go — 添加 LogHandler 到 RouterDeps 并注册路由**

在 `RouterDeps` 结构体中新增：

```go
LogHandler *LogHandler
```

在 `SetupRouter` 的 `v1` 路由组中新增：

```go
// Logs
if deps.LogHandler != nil {
	v1.GET("/logs/audit", deps.LogHandler.ListAudit)
	v1.GET("/logs/runtime", deps.LogHandler.ListRuntime)
	v1.GET("/logs/export", deps.LogHandler.Export)
	v1.GET("/logs/sources", deps.LogHandler.Sources)
	v1.GET("/logs/stats", deps.LogHandler.Stats)
}
```

同时在 `SetupRouter` 开头的中间件链中插入审计中间件（在 CORS 之后、路由之前）。在 `RouterDeps` 中新增 `LogManager *logging.LogManager`，然后：

```go
if deps.LogManager != nil {
	r.Use(logging.AuditMiddleware(deps.LogManager))
}
```

- [ ] **Step 3: 编译验证**

Run: `cd server && go build ./...`
Expected: 编译成功

- [ ] **Step 4: 提交**

```bash
git add server/internal/api/log_handler.go server/internal/api/router.go
git commit -m "feat(logging): add log API endpoints and audit middleware integration"
```

---

### Task 9: Protobuf + gRPC ReportLogs

**Files:**
- Modify: `proto/agent.proto`
- Regenerate: `server/proto/gen/`, `agent/proto/gen/`
- Modify: `server/internal/grpc/handler.go`

- [ ] **Step 1: 修改 proto/agent.proto**

在 `AgentService` 中新增 RPC，在文件末尾新增 message：

```protobuf
service AgentService {
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc ReportMetrics(MetricsPayload) returns (ReportResponse);
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
  rpc ReportLogs(ReportLogsRequest) returns (ReportLogsResponse);
}

// ... existing messages ...

message LogEntry {
  int64  timestamp = 1;
  string level = 2;
  string module = 3;
  string message = 4;
}

message ReportLogsRequest {
  string host_id = 1;
  uint64 batch_id = 2;
  repeated LogEntry entries = 3;
}

message ReportLogsResponse {
  bool   ok = 1;
  uint64 acked_batch_id = 2;
}
```

- [ ] **Step 2: 重新生成 proto**

```bash
cd /Users/piggy/Projects/opsboard/proto
export PATH=$PATH:$(go env GOPATH)/bin
protoc --go_out=. --go-grpc_out=. agent.proto
cp gen/agent.pb.go gen/agent_grpc.pb.go ../server/proto/gen/
cp gen/agent.pb.go gen/agent_grpc.pb.go ../agent/proto/gen/
```

- [ ] **Step 3: 修改 server/internal/grpc/handler.go**

在 Handler 结构体中新增 `logMgr` 字段和 `lastBatch` map：

```go
type Handler struct {
	pb.UnimplementedAgentServiceServer
	serverStore *store.ServerStore
	onMetrics   func(hostID string, payload *pb.MetricsPayload)
	onRegister  func(hostID string)
	logMgr      *logging.LogManager
	batchMu     sync.Mutex
	lastBatch   map[string]uint64 // host_id → last acked batch_id
}
```

更新 `NewHandler` 签名，添加 `logMgr` 参数。

新增 `ReportLogs` 方法：

```go
func (h *Handler) ReportLogs(ctx context.Context, req *pb.ReportLogsRequest) (*pb.ReportLogsResponse, error) {
	if h.logMgr == nil {
		return &pb.ReportLogsResponse{Ok: false}, nil
	}

	h.batchMu.Lock()
	lastAcked := h.lastBatch[req.HostId]

	// Idempotent: already processed this batch
	if req.BatchId > 0 && req.BatchId <= lastAcked {
		h.batchMu.Unlock()
		return &pb.ReportLogsResponse{Ok: true, AckedBatchId: req.BatchId}, nil
	}

	// Gap detection
	if req.BatchId > lastAcked+1 && lastAcked > 0 {
		log.Printf("[grpc] log batch gap: host=%s expected=%d got=%d", req.HostId, lastAcked+1, req.BatchId)
	}

	h.lastBatch[req.HostId] = req.BatchId
	h.batchMu.Unlock()

	// Convert and ingest
	entries := make([]logging.AgentLogEntry, len(req.Entries))
	for i, e := range req.Entries {
		entries[i] = logging.AgentLogEntry{
			Timestamp: e.Timestamp,
			Level:     e.Level,
			Module:    e.Module,
			Message:   e.Message,
		}
	}
	h.logMgr.IngestAgentLogs(req.HostId, entries)

	return &pb.ReportLogsResponse{Ok: true, AckedBatchId: req.BatchId}, nil
}
```

- [ ] **Step 4: 编译验证**

Run: `cd server && go build ./... && go test ./internal/grpc/ -v`
Expected: 编译成功，测试通过

- [ ] **Step 5: 提交**

```bash
git add proto/ server/proto/ agent/proto/ server/internal/grpc/handler.go
git commit -m "feat(logging): add ReportLogs gRPC with batch_id idempotency"
```

---

### Task 10: Agent 日志缓冲与上报

**Files:**
- Create: `agent/internal/reporter/logbuffer.go`
- Modify: `agent/internal/reporter/grpc.go`
- Modify: `agent/cmd/agent/main.go`

- [ ] **Step 1: 实现 logbuffer.go**

```go
//go:build linux

package reporter

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "mantisops/agent/proto/gen"
)

type LogBuffer struct {
	mu       sync.Mutex
	entries  []*pb.LogEntry
	batchID  uint64
	stateDir string
}

func NewLogBuffer(stateDir string) *LogBuffer {
	os.MkdirAll(stateDir, 0755)
	lb := &LogBuffer{stateDir: stateDir}

	// Load persisted batch_id
	data, err := os.ReadFile(filepath.Join(stateDir, "log-batch-id"))
	if err == nil {
		if id, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil {
			lb.batchID = id
		}
	}

	// Load buffered entries
	data, err = os.ReadFile(filepath.Join(stateDir, "log-buffer"))
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if line == "" {
				continue
			}
			var e pb.LogEntry
			if json.Unmarshal([]byte(line), &e) == nil {
				lb.entries = append(lb.entries, &e)
			}
		}
	}

	return lb
}

// Write adds a log entry to the buffer.
func (lb *LogBuffer) Write(level, module, msg string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.entries = append(lb.entries, &pb.LogEntry{
		Timestamp: time.Now().UnixMilli(),
		Level:     level,
		Module:    module,
		Message:   msg,
	})
}

// Flush returns current entries and advances batch_id.
// Call Ack() after successful upload.
func (lb *LogBuffer) Flush() ([]*pb.LogEntry, uint64) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	if len(lb.entries) == 0 {
		return nil, 0
	}
	lb.batchID++
	entries := lb.entries
	lb.entries = nil

	// Persist to disk for crash recovery
	lb.persistLocked(entries)

	return entries, lb.batchID
}

// Ack confirms the batch was received. Clears the buffer file.
func (lb *LogBuffer) Ack(batchID uint64) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	os.WriteFile(filepath.Join(lb.stateDir, "log-batch-id"), []byte(strconv.FormatUint(batchID, 10)), 0644)
	os.Remove(filepath.Join(lb.stateDir, "log-buffer"))
}

// Retry returns buffered entries for retry (from persisted file).
func (lb *LogBuffer) RetryEntries() []*pb.LogEntry {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.entries
}

func (lb *LogBuffer) persistLocked(entries []*pb.LogEntry) {
	var lines []byte
	for _, e := range entries {
		data, _ := json.Marshal(e)
		lines = append(lines, data...)
		lines = append(lines, '\n')
	}
	os.WriteFile(filepath.Join(lb.stateDir, "log-buffer"), lines, 0644)
	os.WriteFile(filepath.Join(lb.stateDir, "log-batch-id"), []byte(strconv.FormatUint(lb.batchID, 10)), 0644)
}

// HookStdLog redirects standard log output to the buffer.
func (lb *LogBuffer) HookStdLog() {
	log.SetOutput(&logWriter{buf: lb})
	log.SetFlags(0)
}

type logWriter struct {
	buf *LogBuffer
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}
	// Also write to stderr for local viewing
	os.Stderr.Write(p)
	w.buf.Write("info", "agent", msg)
	return len(p), nil
}
```

- [ ] **Step 2: 修改 agent/internal/reporter/grpc.go — RunLoop 添加日志上报**

在 `Reporter` 结构体新增 `logBuf *LogBuffer` 字段。

在 `RunLoop` 中新增一个 30s ticker 用于日志上报：

```go
logTicker := time.NewTicker(30 * time.Second)
defer logTicker.Stop()

// 在 select 中新增：
case <-logTicker.C:
    r.reportLogs()
```

新增 `reportLogs` 方法：

```go
func (r *Reporter) reportLogs() {
    if r.logBuf == nil {
        return
    }
    entries, batchID := r.logBuf.Flush()
    if len(entries) == 0 {
        return
    }
    resp, err := r.client.ReportLogs(r.authCtx(), &pb.ReportLogsRequest{
        HostId:  r.hostID,
        BatchId: batchID,
        Entries: entries,
    })
    if err != nil {
        log.Printf("report logs error: %v", err)
        return
    }
    if resp.Ok {
        r.logBuf.Ack(resp.AckedBatchId)
    }
}
```

- [ ] **Step 3: 修改 agent/cmd/agent/main.go — 初始化 LogBuffer**

在 `reporter.New(cfg)` 之后：

```go
logBuf := reporter.NewLogBuffer(filepath.Join(os.Getenv("HOME"), ".config", "mantisops"))
r.SetLogBuffer(logBuf)
logBuf.HookStdLog()
```

- [ ] **Step 4: 编译验证**

Run: `cd agent && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...`
Expected: 编译成功

- [ ] **Step 5: 提交**

```bash
git add agent/
git commit -m "feat(logging): add Agent log buffer with batch upload and crash recovery"
```

---

### Task 11: Server main.go 集成

**Files:**
- Modify: `server/cmd/server/main.go`

- [ ] **Step 1: 在 main() 中初始化 LogManager**

在 WebSocket Hub 创建后、Metrics Collector 之前插入：

```go
// 6. Logging
logDB, err := store.InitSQLite(filepath.Join(cfg.Logging.Dir, "logs.db"))
if err != nil {
    log.Fatalf("init log db: %v", err)
}
defer logDB.Close()

logStore, err := logging.NewLogStore(logDB)
if err != nil {
    log.Fatalf("init log store: %v", err)
}

logWriter := logging.NewRotatingWriter(cfg.Logging.Dir)
defer logWriter.Close()

logMgr := logging.NewLogManager(logWriter, logStore, hub, logging.ParseLevel(cfg.Logging.Level), cfg.Logging.Dir)
defer logMgr.Close()

logSearcher := logging.NewLogSearcher(logStore, cfg.Logging.Dir)

cleaner := logging.NewCleaner(cfg.Logging.Dir, logStore, cfg.Logging.Retention, cfg.Logging.CleanupHour)
cleaner.Start()
defer cleaner.Stop()
```

将 `logMgr` 传入 gRPC Handler 和 RouterDeps：

```go
// gRPC handler
grpcHandler := grpcpkg.NewHandler(serverStore, mc.OnMetrics, dep.OnAgentRegistered, logMgr)

// RouterDeps
LogHandler: api.NewLogHandler(logStore, logSearcher),
LogManager: logMgr,
```

- [ ] **Step 2: 添加 import**

```go
import (
    "mantisops/server/internal/logging"
    "path/filepath"
)
```

- [ ] **Step 3: 编译并运行测试**

Run: `cd server && go build ./... && go test ./... 2>&1 | tail -15`
Expected: 编译成功，测试全部通过

- [ ] **Step 4: 提交**

```bash
git add server/cmd/server/main.go
git commit -m "feat(logging): wire LogManager into server startup"
```

---

## Chunk 3: 前端日志中心页面

### Task 12: 日志 API 客户端

**Files:**
- Create: `web/src/api/logs.ts`

- [ ] **Step 1: 实现 logs.ts**

```typescript
import api from './client'

export interface AuditLog {
  id: number
  timestamp: string
  username: string
  action: string
  resource_type: string
  resource_id: string
  resource_name: string
  detail: string
  ip_address: string
  user_agent: string
}

export interface RuntimeLog {
  id: number
  timestamp: string
  level: string
  module: string
  source: string
  message_preview: string
  // search results use 'message' instead
  message?: string
}

export interface LogPage<T> {
  data: T[]
  total: number
  page?: number
  page_size?: number
}

export interface LogQuery {
  start?: string
  end?: string
  level?: string
  module?: string
  source?: string
  keyword?: string
  username?: string
  action?: string
  resource_type?: string
  page?: number
  page_size?: number
}

export async function getAuditLogs(q: LogQuery): Promise<LogPage<AuditLog>> {
  const { data } = await api.get('/logs/audit', { params: q })
  return data
}

export async function getRuntimeLogs(q: LogQuery): Promise<LogPage<RuntimeLog>> {
  const { data } = await api.get('/logs/runtime', { params: q })
  return data
}

export async function getLogSources(): Promise<string[]> {
  const { data } = await api.get('/logs/sources')
  return data || []
}

export async function getLogStats(start?: string, end?: string): Promise<Record<string, number>> {
  const { data } = await api.get('/logs/stats', { params: { start, end } })
  return data || {}
}

export function getExportUrl(params: LogQuery & { type: string; format: string }): string {
  const searchParams = new URLSearchParams()
  Object.entries(params).forEach(([k, v]) => {
    if (v !== undefined && v !== '') searchParams.set(k, String(v))
  })
  return `/api/v1/logs/export?${searchParams.toString()}`
}
```

- [ ] **Step 2: 提交**

```bash
git add web/src/api/logs.ts
git commit -m "feat(logging): add frontend log API client"
```

---

### Task 13: 日志中心页面

**Files:**
- Create: `web/src/pages/Logs/index.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/components/Layout/Sidebar.tsx`

- [ ] **Step 1: 创建 Logs 页面**

创建 `web/src/pages/Logs/index.tsx`，实现两个 Tab（操作审计 + 运行日志）、筛选栏、实时/查询模式切换、分页、导出功能。

页面需包含：
- Tab 切换：操作审计 | 运行日志
- 筛选栏：时间范围选择器、级别下拉（运行日志）、模块下拉、来源下拉、关键字输入框、导出按钮
- 工具栏：实时/查询 radio 切换、总数显示、刷新按钮
- 表格/列表：
  - 操作审计：时间、操作人、操作、资源类型、资源名称、IP、详情展开
  - 运行日志：时间、级别 badge（颜色编码）、来源 badge、模块、消息（截断+展开）
- 分页控件
- 实时模式下自动滚动

（完整 TSX 代码较长，此处标记为独立实现步骤，参考 spec 中的 UI 设计。遵循项目现有 Tailwind CSS 风格，与 Alerts 页面布局风格保持一致。）

- [ ] **Step 2: 修改 App.tsx — 添加路由**

```tsx
import Logs from './pages/Logs'

// 在 Routes 中添加（alerts 和 billing 之间）：
<Route path="/logs" element={<Logs />} />
```

- [ ] **Step 3: 修改 Sidebar.tsx — 添加菜单项**

在 `links` 数组中，`告警中心` 之后添加：

```tsx
{ to: '/logs', label: '日志中心', icon: 'article' },
```

- [ ] **Step 4: 前端构建验证**

Run: `cd web && npx tsc --noEmit`
Expected: 无类型错误

- [ ] **Step 5: 提交**

```bash
git add web/src/pages/Logs/ web/src/App.tsx web/src/components/Layout/Sidebar.tsx
git commit -m "feat(logging): add Log Center page with audit and runtime tabs"
```

---

### Task 14: WebSocket 日志订阅

**Files:**
- Modify: `web/src/hooks/useWebSocket.ts`
- Modify: `server/internal/ws/hub.go`

- [ ] **Step 1: 修改 hub.go — 添加日志订阅支持**

在 `client` 结构体中新增订阅状态：

```go
type client struct {
    conn       *websocket.Conn
    mu         sync.Mutex
    logFilter  *LogFilter // nil means not subscribed
}

type LogFilter struct {
    Level  string // comma-separated
    Source string
    Module string
}
```

新增 `BroadcastLog` 方法，只推送给已订阅且 filter 匹配的客户端：

```go
func (h *Hub) BroadcastLog(level, source, module string, msg interface{}) {
    data, err := json.Marshal(msg)
    if err != nil {
        return
    }
    h.mu.RLock()
    targets := make([]*client, 0)
    for c := range h.clients {
        if c.logFilter == nil {
            continue
        }
        if c.logFilter.matches(level, source, module) {
            targets = append(targets, c)
        }
    }
    h.mu.RUnlock()
    // ... write to targets (same pattern as BroadcastJSON)
}
```

Hub 需要处理客户端发送的 `log_subscribe` / `log_unsubscribe` 消息。在 WebSocket 读循环中解析这些消息并设置 client 的 `logFilter`。

- [ ] **Step 2: 修改 useWebSocket.ts — 添加 log 消息处理**

在 `ws.onmessage` 的 switch 中新增：

```typescript
if (msg.type === 'log') {
    window.dispatchEvent(new CustomEvent('ws_log', { detail: msg.data }))
}
```

新增发送订阅/取消的工具函数：

```typescript
export function subscribeLog(filter: { level?: string; source?: string; module?: string }) {
    // send via current ws connection
}
export function unsubscribeLog() {
    // send unsubscribe message
}
```

- [ ] **Step 3: 编译验证**

Run: `cd server && go build ./... && cd ../web && npx tsc --noEmit`
Expected: 全部通过

- [ ] **Step 4: 提交**

```bash
git add server/internal/ws/hub.go web/src/hooks/useWebSocket.ts
git commit -m "feat(logging): add WebSocket log subscribe/unsubscribe support"
```

---

### Task 15: 结构化日志替换（server 端 log.Printf → logMgr）

**Files:**
- Modify: 约 15 个 server/internal/ 下的文件

- [ ] **Step 1: 全局替换**

将所有 `log.Printf("[module] ...")` 替换为 `logMgr.Info("module", ...)`。

需要修改的文件清单（按模块）：

| 文件 | 约行数 | 模块前缀 |
|------|-------|---------|
| server/internal/alert/alerter.go | ~15 处 | alerter |
| server/internal/collector/aliyun.go | ~17 处 | aliyun |
| server/internal/collector/metrics.go | ~1 处 | collector |
| server/internal/cloud/manager.go | ~4 处 | cloud |
| server/internal/deployer/deployer.go | ~4 处 | deployer |
| server/internal/grpc/handler.go | ~2 处 | grpc |
| server/internal/grpc/server.go | ~1 处 | grpc |
| server/internal/probe/prober.go | ~2 处 | probe |
| server/internal/ws/hub.go | ~1 处 | ws |
| server/internal/api/billing_handler.go | ~7 处 | billing |
| server/internal/api/database_handler.go | ~4 处 | db-api |
| server/internal/crypto/aes.go | ~1 处 | crypto |
| server/cmd/server/main.go | ~5 处 | main |

每个文件需要：
1. 在结构体/函数中接收 `logMgr *logging.LogManager`
2. 替换 `log.Printf` 调用
3. 保持 `log.Fatalf` 不变（启动阶段错误仍用标准 log）

- [ ] **Step 2: 编译并运行所有测试**

Run: `cd server && go build ./... && go test ./...`
Expected: 全部通过

- [ ] **Step 3: 提交**

```bash
git add server/
git commit -m "refactor(logging): replace log.Printf with structured LogManager calls"
```

---

### Task 16: 端到端验证

- [ ] **Step 1: 启动 server 并验证日志写入**

```bash
cd server && go run ./cmd/server/
```

检查 `./logs/system/` 目录下是否生成当天日志文件。

- [ ] **Step 2: 验证审计日志**

```bash
# 登录
curl -X POST http://localhost:3100/api/v1/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"..."}'

# 查询审计日志
curl -H "Authorization: Bearer $TOKEN" http://localhost:3100/api/v1/logs/audit
```

Expected: 返回包含 login 操作的审计记录。

- [ ] **Step 3: 验证运行日志 API**

```bash
curl -H "Authorization: Bearer $TOKEN" "http://localhost:3100/api/v1/logs/runtime?start=2026-03-28T00:00:00&level=info"
```

Expected: 返回运行日志列表。

- [ ] **Step 4: 验证前端页面**

打开浏览器访问 `/logs`，确认：
- 两个 Tab 正常切换
- 筛选器工作正常
- 实时模式有新日志滚入
- 导出功能可下载文件

- [ ] **Step 5: 最终提交**

```bash
git add -A
git commit -m "feat(logging): log center feature complete"
```
