# AI 分析模块实现计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 MantisOps 添加 AI 驱动的运维分析报告（日/周/月/季度/年度）和全局 AI 对话助手

**Architecture:** Go Server 内新增 `ai/` 包，通过统一 Provider 接口对接 Claude/OpenAI/Ollama 三种 LLM 后端。报告生成前从 VictoriaMetrics + SQLite 实时采集数据，组装为结构化 prompt。对话流式输出通过 WebSocket stream_id 订阅模式推送。前端新增报告页面和全局 ChatBot 浮窗。

**Tech Stack:** Go (net/http for LLM API, robfig/cron/v3 for cron parsing), React (react-markdown + remark-gfm for Markdown 渲染, Zustand for状态管理)

**Spec:** `docs/superpowers/specs/2026-03-29-ai-analysis-design.md`

---

## 文件结构

### 后端新增文件

| 文件 | 职责 |
|------|------|
| `server/internal/ai/provider.go` | Provider 接口定义 + ProviderManager + 通用类型 |
| `server/internal/ai/claude.go` | Claude API Provider 实现 |
| `server/internal/ai/openai.go` | OpenAI API Provider 实现（兼容通义千问/DeepSeek） |
| `server/internal/ai/ollama.go` | Ollama Provider 实现 |
| `server/internal/ai/data_collector.go` | 从 VM + SQLite 采集时间窗口内的数据 |
| `server/internal/ai/prompts.go` | 各报告类型的 Prompt 模板 |
| `server/internal/ai/reporter.go` | 报告生成器（数据采集 → prompt → LLM → 存库） |
| `server/internal/ai/scheduler.go` | 定时调度器（cron 解析 + ticker） |
| `server/internal/ai/chat.go` | 对话引擎（上下文注入 + 流式输出 + 自动标题） |
| `server/internal/store/ai_store.go` | ai_reports / ai_conversations / ai_messages / ai_schedules CRUD |
| `server/internal/api/ai_handler.go` | HTTP API handler（报告 + 对话 + 设置） |

### 后端修改文件

| 文件 | 改动 |
|------|------|
| `server/internal/config/config.go` | 新增 AIConfig 结构体 |
| `server/internal/store/sqlite.go` | 新增迁移步骤建 4 张 AI 表 |
| `server/internal/ws/hub.go` | Client 新增 aiStream 字段，新增 BroadcastAIStreamJSON + ai_stream_subscribe/unsubscribe |
| `server/internal/api/router.go` | RouterDeps 新增 AIHandler，注册 /ai/* 路由 |
| `server/cmd/server/main.go` | 初始化 AI 组件链 |
| `server/internal/logging/middleware.go` | auditRoutes 新增 AI 路由审计 |
| `server/configs/server.yaml.example` | 新增 ai 配置段 |

### 前端新增文件

| 文件 | 职责 |
|------|------|
| `web/src/api/ai.ts` | AI API 客户端函数 |
| `web/src/stores/aiStore.ts` | AI Zustand store（报告列表 + 对话 + 流式状态） |
| `web/src/pages/AIReports/index.tsx` | AI 报告列表页 |
| `web/src/pages/AIReports/ReportDetail.tsx` | 报告详情页（Markdown 渲染） |
| `web/src/components/AIChat/ChatButton.tsx` | 右下角悬浮按钮 |
| `web/src/components/AIChat/ChatPanel.tsx` | AI 对话面板 |

### 前端修改文件

| 文件 | 改动 |
|------|------|
| `web/src/App.tsx` | 新增 /ai-reports 和 /ai-reports/:id 路由 |
| `web/src/components/Layout/Sidebar.tsx` | 新增 AI 报告菜单项 |
| `web/src/components/Layout/MainLayout.tsx` | 挂载 ChatButton 组件 |
| `web/src/hooks/useWebSocket.ts` | 新增 ai_chat_chunk / ai_chat_error / ai_report_* 处理 |
| `web/src/pages/Settings/index.tsx` | 新增 AI 配置区块 |

---

## Chunk 1: 后端基础层（配置 + 数据库 + Store + Provider 接口）

### Task 1: 配置结构

**Files:**
- Modify: `server/internal/config/config.go`
- Modify: `server/configs/server.yaml.example`

- [ ] **Step 1: 在 config.go 新增 AIConfig 结构体**

在 `Config` 结构体中添加 `AI AIConfig` 字段，以及相关子结构体：

```go
type AIConfig struct {
	Enabled        bool              `yaml:"enabled"`
	ActiveProvider string            `yaml:"active_provider"`
	Timezone       string            `yaml:"timezone"`
	Claude         ClaudeConfig      `yaml:"claude"`
	OpenAI         OpenAIConfig      `yaml:"openai"`
	Ollama         OllamaConfig      `yaml:"ollama"`
	Report         AIReportConfig    `yaml:"report"`
	Chat           AIChatConfig      `yaml:"chat"`
}

type ClaudeConfig struct {
	APIKey      string `yaml:"api_key"`
	ReportModel string `yaml:"report_model"`
	ChatModel   string `yaml:"chat_model"`
	MaxTokens   int    `yaml:"max_tokens"`
}

type OpenAIConfig struct {
	APIKey      string `yaml:"api_key"`
	BaseURL     string `yaml:"base_url"`
	ReportModel string `yaml:"report_model"`
	ChatModel   string `yaml:"chat_model"`
	MaxTokens   int    `yaml:"max_tokens"`
}

type OllamaConfig struct {
	Host        string `yaml:"host"`
	ReportModel string `yaml:"report_model"`
	ChatModel   string `yaml:"chat_model"`
	MaxTokens   int    `yaml:"max_tokens"`
}

type AIReportConfig struct {
	MaxGenerationTime int `yaml:"max_generation_time"`
	MaxConcurrent     int `yaml:"max_concurrent"`
}

type AIChatConfig struct {
	MaxHistoryMessages int  `yaml:"max_history_messages"`
	MaxMessageLength   int  `yaml:"max_message_length"`
	SystemCtxRefresh   bool `yaml:"system_context_refresh"`
}
```

- [ ] **Step 2: 在 Load() 中为 AI 配置设置默认值**

在 `Load()` 函数的默认值设置区域追加：

```go
if cfg.AI.Timezone == "" {
	cfg.AI.Timezone = "Asia/Shanghai"
}
if cfg.AI.Claude.ReportModel == "" {
	cfg.AI.Claude.ReportModel = "claude-sonnet-4-20250514"
}
if cfg.AI.Claude.ChatModel == "" {
	cfg.AI.Claude.ChatModel = "claude-haiku-4-5-20251001"
}
if cfg.AI.Claude.MaxTokens == 0 {
	cfg.AI.Claude.MaxTokens = 8192
}
if cfg.AI.OpenAI.BaseURL == "" {
	cfg.AI.OpenAI.BaseURL = "https://api.openai.com/v1"
}
if cfg.AI.OpenAI.ReportModel == "" {
	cfg.AI.OpenAI.ReportModel = "gpt-4o"
}
if cfg.AI.OpenAI.ChatModel == "" {
	cfg.AI.OpenAI.ChatModel = "gpt-4o-mini"
}
if cfg.AI.OpenAI.MaxTokens == 0 {
	cfg.AI.OpenAI.MaxTokens = 8192
}
if cfg.AI.Ollama.Host == "" {
	cfg.AI.Ollama.Host = "http://127.0.0.1:11434"
}
if cfg.AI.Ollama.MaxTokens == 0 {
	cfg.AI.Ollama.MaxTokens = 4096
}
if cfg.AI.Report.MaxGenerationTime == 0 {
	cfg.AI.Report.MaxGenerationTime = 300
}
if cfg.AI.Report.MaxConcurrent == 0 {
	cfg.AI.Report.MaxConcurrent = 1
}
if cfg.AI.Chat.MaxHistoryMessages == 0 {
	cfg.AI.Chat.MaxHistoryMessages = 20
}
if cfg.AI.Chat.MaxMessageLength == 0 {
	cfg.AI.Chat.MaxMessageLength = 4000
}
```

- [ ] **Step 3: 更新 server.yaml.example**

在文件末尾追加完整的 ai 配置段示例（参考 spec 第十二节）。

- [ ] **Step 4: 编译验证**

Run: `cd server && go build ./cmd/server/`
Expected: 编译通过

- [ ] **Step 5: Commit**

```bash
git add server/internal/config/config.go server/configs/server.yaml.example
git commit -m "feat(ai): add AIConfig to server configuration"
```

---

### Task 2: SQLite 迁移 — 建 AI 表

**Files:**
- Modify: `server/internal/store/sqlite.go`

- [ ] **Step 1: 查看当前最高迁移版本**

读取 `sqlite.go` 找到当前最高的 `migrateVN`，确认下一个版本号。

- [ ] **Step 2: 新增迁移函数**

添加新的迁移函数（版本号基于 Step 1 确认的下一版本），建 4 张 AI 表。遵循现有模式：检查版本 → 开事务 → 建表 → 插版本号 → 提交。

```go
func migrateVN(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&version); err != nil {
		return err
	}
	if version >= N {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS ai_reports (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			report_type TEXT NOT NULL,
			title TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			period_start INTEGER NOT NULL,
			period_end INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			error_message TEXT DEFAULT '',
			trigger_type TEXT NOT NULL,
			provider TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			token_usage INTEGER DEFAULT 0,
			generation_time_ms INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_reports_type_period ON ai_reports(report_type, period_start DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_reports_status ON ai_reports(status)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_reports_type_period_unique ON ai_reports(report_type, period_start, period_end) WHERE status = 'completed'`,

		`CREATE TABLE IF NOT EXISTS ai_conversations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL DEFAULT '新对话',
			user TEXT NOT NULL DEFAULT 'admin',
			provider TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			message_count INTEGER DEFAULT 0,
			last_message_at INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_conversations_user ON ai_conversations(user, last_message_at DESC)`,

		`CREATE TABLE IF NOT EXISTS ai_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id INTEGER NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
			role TEXT NOT NULL,
			content TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'done',
			error_message TEXT DEFAULT '',
			request_id TEXT DEFAULT '',
			prompt_tokens INTEGER DEFAULT 0,
			completion_tokens INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ai_messages_conv ON ai_messages(conversation_id, created_at)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_messages_request_id ON ai_messages(conversation_id, request_id) WHERE request_id != ''`,

		`CREATE TABLE IF NOT EXISTS ai_schedules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			report_type TEXT NOT NULL UNIQUE,
			enabled INTEGER NOT NULL DEFAULT 0,
			cron_expr TEXT NOT NULL,
			last_run_at INTEGER,
			next_run_at INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("ai migration: %w", err)
		}
	}

	// 插入默认定时计划
	defaults := []struct{ typ, cron string }{
		{"daily", "0 7 * * *"},
		{"weekly", "0 8 * * 1"},
		{"monthly", "0 8 1 * *"},
		{"quarterly", "0 8 1 1,4,7,10 *"},
		{"yearly", "0 8 1 1 *"},
	}
	for _, d := range defaults {
		_, err := tx.Exec(
			`INSERT OR IGNORE INTO ai_schedules (report_type, cron_expr) VALUES (?, ?)`,
			d.typ, d.cron)
		if err != nil {
			return fmt.Errorf("insert default schedule: %w", err)
		}
	}

	if _, err := tx.Exec(`INSERT INTO schema_version VALUES(?)`, N); err != nil {
		return err
	}
	return tx.Commit()
}
```

- [ ] **Step 3: 在 InitSQLite 中调用新迁移**

在 `migrate()` 函数中 `migrateV1(db)` 调用之后追加：

```go
if err := migrateVN(db); err != nil {
	return err
}
```

- [ ] **Step 4: 编译验证**

Run: `cd server && go build ./cmd/server/`
Expected: 编译通过

- [ ] **Step 5: Commit**

```bash
git add server/internal/store/sqlite.go
git commit -m "feat(ai): add SQLite migration for AI tables"
```

---

### Task 3: AI Store — 数据访问层

**Files:**
- Create: `server/internal/store/ai_store.go`

- [ ] **Step 1: 创建 AIStore 结构体和构造函数**

```go
package store

import (
	"database/sql"
	"time"
)

type AIStore struct {
	db *sql.DB
}

func NewAIStore(db *sql.DB) *AIStore {
	return &AIStore{db: db}
}
```

- [ ] **Step 2: 实现报告 CRUD**

包含以下方法，遵循现有 Store 模式（参考 `alert_store.go`）：

- `CreateReport(r *AIReport) (int64, error)` — INSERT 返回 ID
- `GetReport(id int64) (*AIReport, error)` — QueryRow + Scan
- `ListReports(filter ReportFilter) ([]AIReport, int, error)` — 分页 + 筛选（type, status），默认不返回 superseded
- `UpdateReportStatus(id int64, status, errorMsg string) error`
- `UpdateReportCompleted(id int64, content, summary, provider, model string, tokenUsage int, genTimeMs int64) error`
- `SupersedeReport(oldID int64) error` — 标记为 superseded
- `FindCompletedReport(reportType string, periodStart, periodEnd int64) (*AIReport, error)` — 查找已完成的同窗口报告
- `LatestCompletedReport() (*AIReport, error)` — 最新一份 completed 报告
- `DeleteReport(id int64) error`
- `CleanupStaleReports() error` — 启动时将 pending/generating 标记为 failed
- `CompleteReportInTx(reportID int64, content, summary, provider, model string, tokenUsage int, genTimeMs int64, supersedeID *int64) error` — 事务内先 supersede 旧报告再 complete 新报告

Model 结构体定义在同一文件中：

```go
type AIReport struct {
	ID              int64  `json:"id"`
	ReportType      string `json:"report_type"`
	Title           string `json:"title"`
	Summary         string `json:"summary"`
	Content         string `json:"content,omitempty"`
	PeriodStart     int64  `json:"period_start"`
	PeriodEnd       int64  `json:"period_end"`
	Status          string `json:"status"`
	ErrorMessage    string `json:"error_message,omitempty"`
	TriggerType     string `json:"trigger_type"`
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	TokenUsage      int    `json:"token_usage"`
	GenerationTimeMs int64 `json:"generation_time_ms"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type ReportFilter struct {
	Type               string
	Status             string
	IncludeSuperseded  bool
	Limit              int
	Offset             int
}
```

- [ ] **Step 3: 实现对话 CRUD**

- `CreateConversation(c *AIConversation) (int64, error)`
- `GetConversation(id int64) (*AIConversation, error)`
- `ListConversations(user string, limit, offset int) ([]AIConversation, int, error)`
- `UpdateConversationTitle(id int64, title string) error`
- `DeleteConversation(id int64) error` — CASCADE 删除消息

- [ ] **Step 4: 实现消息 CRUD**

- `CreateMessage(m *AIMessage) (int64, error)` — 同时原子递增 conversation.message_count 和更新 last_message_at
- `GetMessagesByConversation(convID int64) ([]AIMessage, error)`
- `UpdateMessageContent(id int64, content string, promptTokens, completionTokens int) error` — 流结束后回填
- `UpdateMessageFailed(id int64, errorMsg string) error` — status='failed'
- `FindByRequestID(convID int64, requestID string) (*AIMessage, error)` — 幂等去重查找

Model 结构体：

```go
type AIConversation struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	User          string `json:"user"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	MessageCount  int    `json:"message_count"`
	LastMessageAt *int64 `json:"last_message_at"`
	CreatedAt     string `json:"created_at"`
}

type AIMessage struct {
	ID               int64  `json:"id"`
	ConversationID   int64  `json:"conversation_id"`
	Role             string `json:"role"`
	Content          string `json:"content"`
	Status           string `json:"status"`
	ErrorMessage     string `json:"error_message,omitempty"`
	RequestID        string `json:"request_id,omitempty"`
	PromptTokens     int    `json:"prompt_tokens,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"`
	CreatedAt        string `json:"created_at"`
}
```

- [ ] **Step 5: 实现定时计划 CRUD**

- `ListSchedules() ([]AISchedule, error)`
- `GetSchedule(id int64) (*AISchedule, error)`
- `UpdateSchedule(id int64, enabled bool, cronExpr string) error`
- `UpdateScheduleRun(id int64, lastRunAt, nextRunAt int64) error`

```go
type AISchedule struct {
	ID         int64  `json:"id"`
	ReportType string `json:"report_type"`
	Enabled    bool   `json:"enabled"`
	CronExpr   string `json:"cron_expr"`
	LastRunAt  *int64 `json:"last_run_at"`
	NextRunAt  *int64 `json:"next_run_at"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}
```

- [ ] **Step 6: 编译验证**

Run: `cd server && go build ./cmd/server/`
Expected: 编译通过

- [ ] **Step 7: Commit**

```bash
git add server/internal/store/ai_store.go
git commit -m "feat(ai): add AIStore with reports, conversations, messages, schedules CRUD"
```

---

### Task 4: LLM Provider 接口 + 三个实现

**Files:**
- Create: `server/internal/ai/provider.go`
- Create: `server/internal/ai/claude.go`
- Create: `server/internal/ai/openai.go`
- Create: `server/internal/ai/ollama.go`

- [ ] **Step 1: 创建 provider.go — 接口定义 + ProviderManager**

定义 spec 第三节中的所有类型：`Provider` 接口、`CompletionRequest`、`Message`、`CompletionResponse`、`TokenUsage`、`StreamChunk`、`ProviderManager`。

`ProviderManager` 管理多个 Provider 实例，支持 `Active()`、`SetActive(name)`、`List()` 方法。

构造函数签名（需要 settingsStore 和 masterKey 用于运行时读取加密的 API Key）：

```go
func NewProviderManager(cfg *config.AIConfig, settings *store.SettingsStore, masterKey []byte) *ProviderManager
```

- [ ] **Step 2: 创建 claude.go — Claude API Provider**

实现 `ClaudeProvider`：
- `Complete()`: POST `https://api.anthropic.com/v1/messages`，Header `x-api-key` + `anthropic-version: 2023-06-01`
- `Stream()`: 同上但 `stream: true`，SSE 解析 `content_block_delta` 事件中的 `delta.text`
- 请求体格式：`{"model": "...", "max_tokens": N, "messages": [...], "stream": false}`
- 响应解析：`content[0].text` 和 `usage.input_tokens` / `usage.output_tokens`

使用标准库 `net/http`，不引入第三方 SDK。

- [ ] **Step 3: 创建 openai.go — OpenAI API Provider**

实现 `OpenAIProvider`：
- `Complete()`: POST `{base_url}/chat/completions`，Header `Authorization: Bearer {api_key}`
- `Stream()`: 同上但 `stream: true`，SSE 解析 `choices[0].delta.content`
- 支持自定义 `base_url`（兼容通义千问、DeepSeek）

- [ ] **Step 4: 创建 ollama.go — Ollama Provider**

实现 `OllamaProvider`：
- `Complete()`: POST `{host}/api/chat`，`stream: false`
- `Stream()`: POST `{host}/api/chat`，`stream: true`，NDJSON 逐行解析 `message.content`
- 无认证

- [ ] **Step 5: 编译验证**

Run: `cd server && go build ./cmd/server/`
Expected: 编译通过

- [ ] **Step 6: Commit**

```bash
git add server/internal/ai/
git commit -m "feat(ai): add LLM Provider interface with Claude, OpenAI, Ollama implementations"
```

---

## Chunk 2: 核心 AI 逻辑（数据采集 + Prompt + 报告生成 + 调度 + WebSocket + 对话）

### Task 5: 数据采集器

**Files:**
- Create: `server/internal/ai/data_collector.go`

- [ ] **Step 1: 定义 ReportData 及子结构体**

按 spec 第四节定义 `ReportData`、`ServerSummary`、`AlertSummary`、`ProbeSummary`、`ContainerSummary`、`CloudSummary`、`AuditSummary`、`PredictionInput` 等结构体。

- [ ] **Step 2: 实现 DataCollector 结构体**

```go
type DataCollector struct {
	vmURL       string         // VictoriaMetrics URL
	db          *sql.DB        // SQLite
	serverStore *store.ServerStore
	alertStore  *store.AlertStore
	probeStore  *store.ProbeStore
	// ... 其他 store
	httpClient  *http.Client
	timezone    *time.Location
}
```

- [ ] **Step 3: 实现 VM 查询方法**

`queryVM(query string, start, end time.Time, step string) ([]VMDataPoint, error)` — 调用 VictoriaMetrics `/api/v1/query_range` API，解析 JSON 响应。

- [ ] **Step 4: 实现 Collect(period) 方法**

按 spec 4.3 节的时间窗口映射，根据 report_type 确定 step 粒度，查询各维度数据，组装为 `ReportData`。

对每台服务器查询 CPU/内存/磁盘/网络的 avg/max/p95；对告警从 SQLite 查询时间窗口内的事件统计；对探测计算可用性百分比等。

- [ ] **Step 5: 实现 FormatAsText(data *ReportData) string**

将 ReportData 格式化为 spec 4.4 节描述的结构化文本，供 prompt 注入。

- [ ] **Step 6: 实现 CollectChatContext() string**

采集当前实时快照（最新指标 + 当前告警 + 探测状态），生成 spec 7.2 节描述的对话系统上下文文本。

- [ ] **Step 7: 编译验证**

Run: `cd server && go build ./cmd/server/`

- [ ] **Step 8: Commit**

```bash
git add server/internal/ai/data_collector.go
git commit -m "feat(ai): add DataCollector for VM + SQLite data gathering"
```

---

### Task 6: Prompt 模板

**Files:**
- Create: `server/internal/ai/prompts.go`

- [ ] **Step 1: 定义各报告类型的 system prompt 模板**

每种报告类型一个函数，返回 system prompt 字符串。模板包含角色定义、输出格式要求、数据注入占位。

```go
func DailyReportPrompt(data string) string
func WeeklyReportPrompt(data string) string
func MonthlyReportPrompt(data string) string
func QuarterlyReportPrompt(data string) string
func YearlyReportPrompt(data string) string
func ChatSystemPrompt(context string) string
func GenerateTitlePrompt(userMsg, assistantMsg string) string
```

按 spec 5.2 节描述的章节结构编写每种报告类型的输出要求。

- [ ] **Step 2: Commit**

```bash
git add server/internal/ai/prompts.go
git commit -m "feat(ai): add prompt templates for all report types and chat"
```

---

### Task 7: 报告生成器

**Files:**
- Create: `server/internal/ai/reporter.go`

- [ ] **Step 1: 实现 Reporter 结构体**

```go
type Reporter struct {
	store     *store.AIStore
	collector *DataCollector
	provider  *ProviderManager
	hub       *ws.Hub
	timezone  *time.Location
	maxTime   time.Duration
	mu        sync.Mutex  // 串行队列
}
```

- [ ] **Step 2: 实现 Generate 方法**

按 spec 5.3 节流程：创建记录 → 更新 generating → 广播 → 采集数据 → 组装 prompt → LLM Complete → 提取 summary（剥离 Markdown 标记取前 200 字）→ 事务内 supersede+complete → 广播完成/失败。

`StripMarkdown(md string) string` 辅助函数：去除 `#`、`**`、`|`、`-` 等 Markdown 语法标记。

- [ ] **Step 3: 实现时间窗口计算**

`CalcPeriod(reportType string, tz *time.Location) (start, end int64)` — 按 spec 8.1 节规则计算各报告类型的默认时间窗口。

- [ ] **Step 4: 实现报告标题生成**

`GenerateTitle(reportType string, periodStart int64, tz *time.Location) string` — 生成如"2026年3月28日 运维日报"的标题。

- [ ] **Step 5: 编译验证**

Run: `cd server && go build ./cmd/server/`

- [ ] **Step 6: Commit**

```bash
git add server/internal/ai/reporter.go
git commit -m "feat(ai): add Reporter for report generation with LLM"
```

---

### Task 8: 定时调度器

**Files:**
- Create: `server/internal/ai/scheduler.go`

- [ ] **Step 1: 安装 cron 依赖**

Run: `cd server && go get github.com/robfig/cron/v3`

- [ ] **Step 2: 实现 Scheduler 结构体**

```go
type Scheduler struct {
	store    *store.AIStore
	reporter *Reporter
	timezone *time.Location
	stopCh   chan struct{}
}
```

- [ ] **Step 3: 实现 Start/Stop 方法**

`Start()`: 启动 goroutine，每分钟 tick 检查所有 enabled 计划。使用 `github.com/robfig/cron/v3` 的 `cron.ParseStandard()` 获取 `Schedule.Next(now)` 计算下次执行时间。

启动时：清理 stale 报告（调 `store.CleanupStaleReports()`）+ 计算所有 enabled 计划的 `next_run_at`（跳过已过期的，直接算下一次）。

- [ ] **Step 4: 编译验证**

Run: `cd server && go build ./cmd/server/`

- [ ] **Step 5: Commit**

```bash
git add server/internal/ai/scheduler.go server/go.mod server/go.sum
git commit -m "feat(ai): add Scheduler for cron-based report generation"
```

---

### Task 10: 对话引擎

**Files:**
- Create: `server/internal/ai/chat.go`

- [ ] **Step 1: 实现 ChatEngine 结构体**

```go
type ChatEngine struct {
	store         *store.AIStore
	provider      *ProviderManager
	collector     *DataCollector
	hub           *ws.Hub
	maxHistory    int
	maxMsgLen     int
	ctxRefreshAll bool
	streamGates   map[string]chan struct{} // stream_id → 订阅到达通知
	mu            sync.Mutex
}
```

- [ ] **Step 2: 实现 SendMessage 方法**

按 spec 7.1 节流程：
1. request_id 去重检查
2. 创建/获取 conversation
3. 存 user message
4. 预创建 assistant message（status='streaming'）
5. 生成 stream_id，创建 gate channel
6. 返回 user_message_id + assistant_message_id + stream_id
7. 启动 goroutine：等待 gate（5s 超时）→ **无论成功或超时都从 streamGates 中删除条目（防内存泄漏）** → 注入上下文（检查距上次注入是否超过 1 小时，超过则刷新 system prompt）→ 组装消息 → Provider.Stream → 逐 chunk 通过 Hub 推送 → 回填 content → 失败时推送 ai_chat_error → Hub.CleanupAIStream(streamID)

- [ ] **Step 3: 实现 OnStreamSubscribe 方法**

当 Hub 收到 `ai_stream_subscribe` 消息时调用，释放对应 stream_id 的 gate channel。

- [ ] **Step 4: 实现上下文窗口管理**

`buildMessages(convID int64, newUserMsg string, systemCtx string) ([]Message, error)` — 按 spec 7.3 节规则截取历史消息。

- [ ] **Step 5: 实现自动标题生成**

流结束后异步调用 LLM 生成标题（spec 7.4 节），使用 chat_model。

- [ ] **Step 6: 编译验证**

Run: `cd server && go build ./cmd/server/`

- [ ] **Step 7: Commit**

```bash
git add server/internal/ai/chat.go
git commit -m "feat(ai): add ChatEngine with streaming, context injection, auto-title"
```

---

### Task 9: WebSocket Hub 改造（前置于 ChatEngine，因 ChatEngine 依赖 Hub 的 BroadcastAIStreamJSON）

**Files:**
- Modify: `server/internal/ws/hub.go`

- [ ] **Step 1: Client 结构体新增 aiStream 字段**

```go
type client struct {
	conn      *websocket.Conn
	mu        sync.Mutex
	logSub    bool
	logFilter string
	aiStream  map[string]bool // AI stream_id 订阅集合
}
```

- [ ] **Step 2: HandleWS 新增 ai_stream_subscribe/unsubscribe 处理**

在消息解析 switch 中追加：

```go
case "ai_stream_subscribe":
	var subMsg struct {
		StreamID string `json:"stream_id"`
	}
	if json.Unmarshal(msg, &subMsg) == nil && subMsg.StreamID != "" {
		c.mu.Lock()
		if c.aiStream == nil {
			c.aiStream = make(map[string]bool)
		}
		c.aiStream[subMsg.StreamID] = true
		c.mu.Unlock()
		// 通知 ChatEngine 订阅已到达
		if h.onAIStreamSubscribe != nil {
			h.onAIStreamSubscribe(subMsg.StreamID)
		}
	}
case "ai_stream_unsubscribe":
	var subMsg struct {
		StreamID string `json:"stream_id"`
	}
	if json.Unmarshal(msg, &subMsg) == nil && subMsg.StreamID != "" {
		c.mu.Lock()
		delete(c.aiStream, subMsg.StreamID)
		c.mu.Unlock()
	}
```

- [ ] **Step 3: Hub 新增 onAIStreamSubscribe 回调和 setter**

```go
type Hub struct {
	mu                  sync.RWMutex
	clients             map[*client]bool
	onAIStreamSubscribe func(streamID string)
}

func (h *Hub) SetOnAIStreamSubscribe(fn func(streamID string)) {
	h.onAIStreamSubscribe = fn
}
```

- [ ] **Step 4: 新增 BroadcastAIStreamJSON 方法**

仿照 `BroadcastLogJSON` 模式，只向订阅了指定 stream_id 的 client 推送：

```go
func (h *Hub) BroadcastAIStreamJSON(streamID string, msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.RLock()
	var targets []*client
	for c := range h.clients {
		c.mu.Lock()
		if c.aiStream[streamID] {
			targets = append(targets, c)
		}
		c.mu.Unlock()
	}
	h.mu.RUnlock()

	var failed []*client
	for _, c := range targets {
		if err := c.writeMessage(websocket.TextMessage, data); err != nil {
			failed = append(failed, c)
		}
	}
	if len(failed) > 0 {
		h.mu.Lock()
		for _, c := range failed {
			delete(h.clients, c)
			c.conn.Close()
		}
		h.mu.Unlock()
	}
}
```

- [ ] **Step 5: 新增 CleanupAIStream 方法**

流结束后清理所有 client 的该 stream_id 订阅：

```go
func (h *Hub) CleanupAIStream(streamID string) {
	h.mu.RLock()
	for c := range h.clients {
		c.mu.Lock()
		delete(c.aiStream, streamID)
		c.mu.Unlock()
	}
	h.mu.RUnlock()
}
```

- [ ] **Step 6: 编译验证**

Run: `cd server && go build ./cmd/server/`

- [ ] **Step 7: Commit**

```bash
git add server/internal/ws/hub.go
git commit -m "feat(ai): add stream_id subscription to WebSocket Hub"
```

---

## Chunk 3: API 层 + 集成

### Task 11: AI API Handler

**Files:**
- Create: `server/internal/api/ai_handler.go`

- [ ] **Step 1: 创建 AIHandler 结构体**

```go
type AIHandler struct {
	store       *store.AIStore
	reporter    *ai.Reporter
	chat        *ai.ChatEngine
	provider    *ai.ProviderManager
	scheduler   *ai.Scheduler
	settings    *store.SettingsStore
	masterKey   []byte
}

func NewAIHandler(
	store *store.AIStore,
	reporter *ai.Reporter,
	chat *ai.ChatEngine,
	provider *ai.ProviderManager,
	scheduler *ai.Scheduler,
	settings *store.SettingsStore,
	masterKey []byte,
) *AIHandler
```

- [ ] **Step 2: 实现报告 API handlers**

按 spec 8.1 节：
- `ListReports(c *gin.Context)` — 分页 + type/status 筛选
- `GetReport(c *gin.Context)` — 含完整 Markdown
- `GenerateReport(c *gin.Context)` — 手动触发，检查 force + 冲突
- `DeleteReport(c *gin.Context)`
- `DownloadReport(c *gin.Context)` — Content-Disposition: attachment, text/markdown
- `LatestReport(c *gin.Context)` — 仪表盘用

- [ ] **Step 3: 实现对话 API handlers**

按 spec 8.2 节：
- `ListConversations(c *gin.Context)` — 分页
- `CreateConversation(c *gin.Context)`
- `GetConversation(c *gin.Context)` — 含所有消息
- `DeleteConversation(c *gin.Context)`
- `SendMessage(c *gin.Context)` — 触发流式回复

- [ ] **Step 4: 实现设置 API handlers**

按 spec 8.3 节：
- `GetAISettings(c *gin.Context)` — 从 settings 表读取 ai.* 前缀配置
- `UpdateAISettings(c *gin.Context)` — API Key 加密存储
- `ListProviders(c *gin.Context)` — 列出可用 providers 及连接状态
- `TestProvider(c *gin.Context)` — 发送测试 prompt
- `ListSchedules(c *gin.Context)`
- `UpdateSchedule(c *gin.Context)`

- [ ] **Step 5: 编译验证**

Run: `cd server && go build ./cmd/server/`

- [ ] **Step 6: Commit**

```bash
git add server/internal/api/ai_handler.go
git commit -m "feat(ai): add AI HTTP API handlers for reports, chat, settings"
```

---

### Task 12: 路由注册 + main.go 集成

**Files:**
- Modify: `server/internal/api/router.go`
- Modify: `server/cmd/server/main.go`
- Modify: `server/internal/logging/middleware.go`

- [ ] **Step 1: RouterDeps 新增 AIHandler**

在 `RouterDeps` 结构体中添加：

```go
AIHandler *AIHandler
```

- [ ] **Step 2: 在 SetupRouter 中注册 AI 路由**

在 `v1` 路由组（JWT 保护下）中追加：

```go
if deps.AIHandler != nil {
	v1.GET("/ai/reports", deps.AIHandler.ListReports)
	v1.GET("/ai/reports/latest", deps.AIHandler.LatestReport)
	v1.GET("/ai/reports/:id", deps.AIHandler.GetReport)
	v1.POST("/ai/reports/generate", deps.AIHandler.GenerateReport)
	v1.DELETE("/ai/reports/:id", deps.AIHandler.DeleteReport)
	v1.GET("/ai/reports/:id/download", deps.AIHandler.DownloadReport)

	v1.GET("/ai/conversations", deps.AIHandler.ListConversations)
	v1.POST("/ai/conversations", deps.AIHandler.CreateConversation)
	v1.GET("/ai/conversations/:id", deps.AIHandler.GetConversation)
	v1.DELETE("/ai/conversations/:id", deps.AIHandler.DeleteConversation)
	v1.POST("/ai/conversations/:id/messages", deps.AIHandler.SendMessage)

	v1.GET("/ai/settings", deps.AIHandler.GetAISettings)
	v1.PUT("/ai/settings", deps.AIHandler.UpdateAISettings)
	v1.GET("/ai/providers", deps.AIHandler.ListProviders)
	v1.POST("/ai/providers/test", deps.AIHandler.TestProvider)
	v1.GET("/ai/schedules", deps.AIHandler.ListSchedules)
	v1.PUT("/ai/schedules/:id", deps.AIHandler.UpdateSchedule)
}
```

- [ ] **Step 3: main.go 初始化 AI 组件链**

在 `// 21. Settings`（settingsStore 和 settingsHandler 初始化）之后、`// 22. HTTP API router`（RouterDeps 构建）之前插入：

```go
// AI system (optional, only if enabled or has settings)
var aiHandler *api.AIHandler
if cfg.AI.Enabled {
	aiStore := store.NewAIStore(db)
	providerMgr := ai.NewProviderManager(&cfg.AI, settingsStore, masterKey)
	dataCollector := ai.NewDataCollector(cfg.Victoria.URL, db, serverStore, alertStore, probeStore, cfg.AI.Timezone)
	reporter := ai.NewReporter(aiStore, dataCollector, providerMgr, hub, cfg.AI)
	chatEngine := ai.NewChatEngine(aiStore, providerMgr, dataCollector, hub, cfg.AI)
	scheduler := ai.NewScheduler(aiStore, reporter, cfg.AI.Timezone)

	// 连接 Hub 的 stream 订阅回调
	hub.SetOnAIStreamSubscribe(chatEngine.OnStreamSubscribe)

	scheduler.Start()
	defer scheduler.Stop()

	aiHandler = api.NewAIHandler(aiStore, reporter, chatEngine, providerMgr, scheduler, settingsStore, masterKey)
}
```

在 `RouterDeps` 构造中追加：`AIHandler: aiHandler,`

- [ ] **Step 4: 审计中间件新增 AI 路由**

在 `auditRoutes` 数组中追加：

```go
{"POST", "/api/v1/ai/reports/generate", "generate", "ai_report"},
{"DELETE", "/api/v1/ai/reports/", "delete", "ai_report"},
{"POST", "/api/v1/ai/conversations", "create", "ai_conversation"},
{"DELETE", "/api/v1/ai/conversations/", "delete", "ai_conversation"},
{"PUT", "/api/v1/ai/settings", "update", "ai_settings"},
{"PUT", "/api/v1/ai/schedules/", "update", "ai_schedule"},
{"POST", "/api/v1/ai/providers/test", "test", "ai_provider"},
```

- [ ] **Step 5: 编译验证**

Run: `cd server && go build ./cmd/server/`

- [ ] **Step 6: Commit**

```bash
git add server/internal/api/router.go server/cmd/server/main.go server/internal/logging/middleware.go
git commit -m "feat(ai): integrate AI into router, main.go, and audit middleware"
```

---

## Chunk 4: 前端实现

### Task 13: 前端依赖 + API 客户端 + Store

**Files:**
- Create: `web/src/api/ai.ts`
- Create: `web/src/stores/aiStore.ts`

- [ ] **Step 1: 安装 react-markdown + remark-gfm**

Run: `cd web && npm install react-markdown remark-gfm`

- [ ] **Step 2: 创建 ai.ts API 客户端**

按现有 `client.ts` 模式，定义所有 AI API 函数：

```typescript
import api from './client'

// 报告
export const listReports = (params?: { type?: string; status?: string; limit?: number; offset?: number }) =>
  api.get('/ai/reports', { params }).then(r => r.data)

export const getReport = (id: number) =>
  api.get(`/ai/reports/${id}`).then(r => r.data)

export const generateReport = (data: { report_type: string; period_start?: number; period_end?: number; force?: boolean }) =>
  api.post('/ai/reports/generate', data).then(r => r.data)

export const deleteReport = (id: number) =>
  api.delete(`/ai/reports/${id}`)

export const downloadReport = (id: number) =>
  api.get(`/ai/reports/${id}/download`, { responseType: 'blob' }).then(r => r.data)

export const latestReport = () =>
  api.get('/ai/reports/latest').then(r => r.data)

// 对话
export const listConversations = (params?: { limit?: number; offset?: number }) =>
  api.get('/ai/conversations', { params }).then(r => r.data)

export const createConversation = () =>
  api.post('/ai/conversations').then(r => r.data)

export const getConversation = (id: number) =>
  api.get(`/ai/conversations/${id}`).then(r => r.data)

export const deleteConversation = (id: number) =>
  api.delete(`/ai/conversations/${id}`)

export const sendMessage = (convId: number, data: { content: string; request_id: string }) =>
  api.post(`/ai/conversations/${convId}/messages`, data).then(r => r.data)

// 设置
export const getAISettings = () => api.get('/ai/settings').then(r => r.data)
export const updateAISettings = (data: any) => api.put('/ai/settings', data)
export const listProviders = () => api.get('/ai/providers').then(r => r.data)
export const testProvider = (data: any) => api.post('/ai/providers/test', data).then(r => r.data)
export const listSchedules = () => api.get('/ai/schedules').then(r => r.data)
export const updateSchedule = (id: number, data: any) => api.put(`/ai/schedules/${id}`, data)
```

- [ ] **Step 3: 创建 aiStore.ts Zustand store**

管理报告列表、当前对话、流式状态：

```typescript
import { create } from 'zustand'

interface AIState {
  // 报告
  reports: AIReport[]
  reportsTotal: number
  generatingReportIds: number[]

  // 对话
  conversations: AIConversation[]
  activeConversationId: number | null
  messages: AIMessage[]
  streamingContent: string
  streamingMessageId: number | null
  chatOpen: boolean
  chatListOpen: boolean

  // Actions
  setReports: (reports: AIReport[], total: number) => void
  addGeneratingReport: (id: number) => void
  removeGeneratingReport: (id: number) => void
  setChatOpen: (open: boolean) => void
  setChatListOpen: (open: boolean) => void
  setActiveConversation: (id: number | null) => void
  setMessages: (msgs: AIMessage[]) => void
  setConversations: (convs: AIConversation[]) => void
  appendStreamChunk: (content: string) => void
  finalizeStream: (messageId: number, content: string) => void
  setStreamError: (messageId: number, error: string) => void
  startStreaming: (messageId: number) => void
  resetStreaming: () => void
}
```

- [ ] **Step 4: Commit**

```bash
git add web/src/api/ai.ts web/src/stores/aiStore.ts web/package.json web/package-lock.json
git commit -m "feat(ai): add frontend AI API client and Zustand store"
```

---

### Task 14: WebSocket 消息处理

**Files:**
- Modify: `web/src/hooks/useWebSocket.ts`

- [ ] **Step 1: 导入 aiStore**

```typescript
import { useAIStore } from '../stores/aiStore'
```

- [ ] **Step 2: 在 useEffect 中获取 aiStore actions 的 refs**

按现有模式，在 hook 中创建 refs：

```typescript
const appendChunk = useAIStore((s) => s.appendStreamChunk)
const appendChunkRef = useRef(appendChunk)
appendChunkRef.current = appendChunk
// ... 同理其他 actions
```

- [ ] **Step 3: 在 onmessage 处理中新增 AI 消息类型**

```typescript
if (msg.type === 'ai_chat_chunk' && msg.stream_id) {
  if (msg.done) {
    finalizeStreamRef.current(msg.message_id, '')
  } else {
    appendChunkRef.current(msg.content)
  }
}
if (msg.type === 'ai_chat_error' && msg.stream_id) {
  setStreamErrorRef.current(msg.message_id, msg.error)
}
if (msg.type === 'ai_report_generating') {
  addGeneratingReportRef.current(msg.report_id)
}
if (msg.type === 'ai_report_completed') {
  removeGeneratingReportRef.current(msg.report_id)
  window.dispatchEvent(new CustomEvent('ai_report_completed', { detail: msg }))
}
if (msg.type === 'ai_report_failed') {
  removeGeneratingReportRef.current(msg.report_id)
  window.dispatchEvent(new CustomEvent('ai_report_failed', { detail: msg }))
}
```

- [ ] **Step 4: 导出 sendWsMessage 辅助函数**

前端需要通过 WebSocket 发送 `ai_stream_subscribe`：

```typescript
export function sendWsMessage(msg: any) {
  const ws = (window as any).__mantisops_ws as WebSocket | null
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(msg))
  }
}
```

- [ ] **Step 5: Commit**

```bash
git add web/src/hooks/useWebSocket.ts
git commit -m "feat(ai): handle AI WebSocket messages in useWebSocket hook"
```

---

### Task 15: AI 报告列表页

**Files:**
- Create: `web/src/pages/AIReports/index.tsx`

- [ ] **Step 1: 实现报告列表页**

按 spec 10.2 节的 UI 设计：
- 顶部标题 + 「生成报告」按钮
- 筛选 Tab（全部/日报/周报/月报/季度/年度）
- 报告卡片列表（标题、摘要、provider 信息、时间）
- 正在生成的报告显示 loading 状态（根据 `ai_report_generating` 事件）
- 「生成报告」对话框：报告类型下拉 + 时间范围

使用现有 Kinetic Observatory 设计系统组件样式（glass-card、渐变按钮等）。

- [ ] **Step 2: Commit**

```bash
git add web/src/pages/AIReports/index.tsx
git commit -m "feat(ai): add AI reports list page"
```

---

### Task 16: AI 报告详情页

**Files:**
- Create: `web/src/pages/AIReports/ReportDetail.tsx`

- [ ] **Step 1: 实现报告详情页**

按 spec 10.3 节：
- 顶部：返回按钮 + 标题 + 「导出 Markdown」按钮
- 元信息行：provider、token 用量、耗时、生成方式
- Markdown 渲染区域：使用 `react-markdown` + `remark-gfm`
- 导出功能：调 download API，触发浏览器下载 `.md` 文件

- [ ] **Step 2: Commit**

```bash
git add web/src/pages/AIReports/ReportDetail.tsx
git commit -m "feat(ai): add AI report detail page with Markdown rendering"
```

---

### Task 17: AI 对话浮窗

**Files:**
- Create: `web/src/components/AIChat/ChatButton.tsx`
- Create: `web/src/components/AIChat/ChatPanel.tsx`

- [ ] **Step 1: 实现 ChatButton — 悬浮按钮**

右下角固定定位，脉冲动画，点击切换 ChatPanel 显示。

- [ ] **Step 2: 实现 ChatPanel — 对话面板**

按 spec 10.4 节：
- 400x600 固定尺寸（< 768px 时全屏）
- 左侧对话列表（可收起）+ 右侧对话区域
- 消息气泡：用户/助手区分
- 流式输出：打字机效果（从 aiStore 读取 streamingContent）
- 输入框 + 发送按钮
- 发送逻辑：调 `sendMessage` API → 收到 stream_id → 调 `sendWsMessage({type: 'ai_stream_subscribe', stream_id})` → chunk 通过 store 更新 UI
- 失败状态：显示错误信息 + 重试按钮
- 新对话按钮

- [ ] **Step 3: Commit**

```bash
git add web/src/components/AIChat/
git commit -m "feat(ai): add AI chat floating panel with streaming support"
```

---

### Task 18: 路由 + 侧边栏 + 布局集成

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/components/Layout/Sidebar.tsx`
- Modify: `web/src/components/Layout/MainLayout.tsx`

- [ ] **Step 1: App.tsx 新增路由**

在 RequireAuth 路由组内追加：

```tsx
<Route path="/ai-reports" element={<AIReports />} />
<Route path="/ai-reports/:id" element={<ReportDetail />} />
```

- [ ] **Step 2: Sidebar.tsx 新增菜单项**

在 `links` 数组中，「告警中心」和「资源到期」之间插入：

```typescript
{ to: '/ai-reports', label: 'AI 报告', icon: 'analytics' },
```

- [ ] **Step 3: MainLayout.tsx 挂载 ChatButton**

在 MainLayout 组件的最外层追加 `<ChatButton />`（仅在已登录时渲染）。

- [ ] **Step 4: 编译验证**

Run: `cd web && npx tsc --noEmit`
Expected: 无类型错误

- [ ] **Step 5: Commit**

```bash
git add web/src/App.tsx web/src/components/Layout/Sidebar.tsx web/src/components/Layout/MainLayout.tsx
git commit -m "feat(ai): integrate AI routes, sidebar menu, and chat button"
```

---

### Task 19: 设置页 AI 配置区块

**Files:**
- Modify: `web/src/pages/Settings/index.tsx`

- [ ] **Step 1: 新增 AI 配置区块**

按 spec 10.6 节，在设置页现有内容下方新增 AI 配置区域：
- LLM 服务商选择（Claude / OpenAI / Ollama）
- 各 provider 的配置表单（API Key 输入、模型选择、Host 地址）
- 「测试连接」按钮
- 定时报告开关和 cron 编辑

使用条件渲染：仅在 AI 功能启用时显示。

- [ ] **Step 2: Commit**

```bash
git add web/src/pages/Settings/index.tsx
git commit -m "feat(ai): add AI configuration section to Settings page"
```

---

### Task 20: 仪表盘 AI 摘要卡片

**Files:**
- Modify: `web/src/pages/Dashboard/index.tsx`

- [ ] **Step 1: 新增 AI 分析摘要卡片**

按 spec 10.5 节：
- 在摘要区域新增一张卡片
- 显示最新报告标题 + 摘要 + 「查看完整报告」链接
- 无报告时显示空状态提示
- 使用 `latestReport()` API 获取数据

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/
git commit -m "feat(ai): add AI summary card to Dashboard"
```

---

### Task 21: 最终验证

- [ ] **Step 1: 后端编译**

Run: `cd server && go build ./cmd/server/`

- [ ] **Step 2: 前端类型检查**

Run: `cd web && npx tsc --noEmit`

- [ ] **Step 3: 前端构建**

Run: `cd web && npm run build`

- [ ] **Step 4: 全量 Commit（如有遗漏修改）**

```bash
git add -A
git commit -m "feat(ai): final integration and build verification"
```
