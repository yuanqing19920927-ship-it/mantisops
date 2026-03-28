# MantisOps AI 分析模块设计文档

> 日期：2026-03-29
> 状态：已确认（rev.6）

## 一、架构总览

```
┌──────────────────────────────────────────────────────────────────┐
│                       MantisOps Server                           │
│                                                                  │
│  ┌─ ai/ ──────────────────────────────────────────────────────┐  │
│  │                                                            │  │
│  │  ┌─ Provider ─────────┐   ┌─ Reporter ──────────────────┐ │  │
│  │  │ • Claude API       │   │ • 数据采集（VM + SQLite）     │ │  │
│  │  │ • OpenAI API       │   │ • Prompt 组装               │ │  │
│  │  │ • Ollama (local)   │   │ • 调用 Provider 生成报告     │ │  │
│  │  │                    │   │ • Markdown 存库             │ │  │
│  │  │  统一接口：         │   └──────────────────────────────┘ │  │
│  │  │  Complete(prompt)  │                                    │  │
│  │  │  Stream(prompt)    │   ┌─ Scheduler ──────────────────┐ │  │
│  │  └────────────────────┘   │ • Cron 定时触发              │ │  │
│  │                           │ • 日/周/月/季度/年度          │ │  │
│  │  ┌─ ChatEngine ───────┐   └──────────────────────────────┘ │  │
│  │  │ • 会话管理          │                                    │  │
│  │  │ • 上下文注入        │   ┌─ DataCollector ─────────────┐ │  │
│  │  │ • 流式 WebSocket   │   │ • 查询 VM 时序数据           │ │  │
│  │  └────────────────────┘   │ • 查询 SQLite 告警/探测等    │ │  │
│  │                           │ • 组装结构化数据摘要          │ │  │
│  └───────────────────────┘   └──────────────────────────────┘ │  │
│                                                                  │
│       ↓ WebSocket (流式)              ↓ REST API                 │
└──────────────────────────────────────────────────────────────────┘
                          ↓
┌──────────────────────────────────────────────────────────────────┐
│                         前端                                      │
│  ┌─ AI 报告页 (/ai-reports) ─┐  ┌─ AI 助手 (全局浮窗) ────────┐ │
│  │ 报告列表 + 报告详情渲染    │  │ ChatBot 对话界面            │ │
│  │ Markdown 渲染 + 导出      │  │ 流式输出 + 上下文感知        │ │
│  └───────────────────────────┘  └─────────────────────────────┘ │
│  ┌─ 仪表盘摘要卡片 ──────────────────────────────────────────┐  │
│  │ 最新报告摘要 + 跳转链接                                    │  │
│  └───────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────┘
```

### 1.1 核心设计决策

| 决策 | 选择 | 理由 |
|------|------|------|
| LLM 接口 | 统一 Provider 接口 | Claude/OpenAI/Ollama 都是 HTTP JSON，抽象为 `Complete()` + `Stream()` 即可 |
| 数据采集 | 报告生成前实时查询 | 不缓存中间态，直接从 VM + SQLite 拉取时间窗口内的数据 |
| 报告存储 | SQLite `ai_reports` 表 | Markdown 文本 + 元数据，与其他数据统一管理 |
| 对话存储 | SQLite `ai_conversations` + `ai_messages` 表 | 支持多轮对话历史 |
| 流式输出 | 复用 WebSocket | 对话和报告生成进度都通过现有 `/ws` 连接推送 |
| 定时调度 | 内置 cron | 不引入外部调度器，Go 内置 time.Ticker + 时间判断 |
| PDF 导出 | 不做 | 第一版只提供 Markdown 存储 + 页面渲染 + Markdown 文件导出 |

## 二、数据模型

### 2.1 ai_reports 表（SQLite）

```sql
CREATE TABLE ai_reports (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    report_type TEXT NOT NULL,              -- daily / weekly / monthly / quarterly / yearly
    title TEXT NOT NULL,                    -- 自动生成的标题，如"2026年3月29日 运维日报"
    summary TEXT NOT NULL DEFAULT '',       -- 报告摘要（纯文本，剥离 Markdown 标记后取前 200 字），用于列表页和仪表盘卡片
    content TEXT NOT NULL DEFAULT '',        -- 完整 Markdown 内容（pending/generating 时为空）
    period_start INTEGER NOT NULL,          -- 报告覆盖的起始时间（Unix 时间戳）
    period_end INTEGER NOT NULL,            -- 报告覆盖的结束时间（Unix 时间戳）
    status TEXT NOT NULL DEFAULT 'pending', -- pending / generating / completed / failed / superseded
    error_message TEXT DEFAULT '',          -- 失败时的错误信息
    trigger_type TEXT NOT NULL,             -- scheduled / manual
    provider TEXT NOT NULL DEFAULT '',      -- 生成时使用的 LLM provider（claude / openai / ollama）
    model TEXT NOT NULL DEFAULT '',         -- 生成时使用的具体模型名
    token_usage INTEGER DEFAULT 0,          -- LLM token 消耗量（prompt + completion 总和，仅 completed 状态有意义）
    generation_time_ms INTEGER DEFAULT 0,   -- 生成耗时（毫秒，completed 时为总耗时，failed 时为到失败时刻的耗时）
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_ai_reports_type_period ON ai_reports(report_type, period_start DESC);
CREATE INDEX idx_ai_reports_status ON ai_reports(status);
CREATE UNIQUE INDEX idx_ai_reports_type_period_unique ON ai_reports(report_type, period_start, period_end) WHERE status = 'completed';
```

**status 状态流转：**

```
pending → generating → completed → superseded（被 force 覆盖时）
                    → failed
```

**崩溃恢复**：服务启动时，将所有 `status = 'pending'` 或 `status = 'generating'` 的记录标记为 `status = 'failed'`，`error_message = 'server_restart'`。这些残留占位行不会阻塞同一时间窗口的重新生成（唯一索引仅约束 `completed` 状态）。

**superseded 状态**：当 `force=true` 重新生成报告时，新报告成功后旧报告标记为 `superseded`。报告列表 API 默认不返回 `superseded` 的报告（除非传 `include_superseded=true`）。

### 2.2 ai_conversations 表（SQLite）

```sql
CREATE TABLE ai_conversations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL DEFAULT '新对话',    -- 对话标题，首条消息后由 LLM 自动命名
    user TEXT NOT NULL DEFAULT 'admin',     -- 创建用户（多用户 RBAC 实现后生效）
    provider TEXT NOT NULL DEFAULT '',      -- 使用的 LLM provider
    model TEXT NOT NULL DEFAULT '',         -- 使用的具体模型名
    message_count INTEGER DEFAULT 0,       -- 消息数量（每次写入消息时原子递增）
    last_message_at INTEGER,               -- 最后一条消息的 Unix 时间戳
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_ai_conversations_user ON ai_conversations(user, last_message_at DESC);
```

### 2.3 ai_messages 表（SQLite）

```sql
CREATE TABLE ai_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id INTEGER NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    role TEXT NOT NULL,                     -- user / assistant / system
    content TEXT NOT NULL DEFAULT '',       -- 消息内容（Markdown），assistant 消息预创建时为空，流结束后回填
    status TEXT NOT NULL DEFAULT 'done',   -- done / streaming / failed（assistant 消息专用，user 消息始终为 done）
    error_message TEXT DEFAULT '',          -- 失败原因（仅 status=failed 时有值）
    request_id TEXT DEFAULT '',             -- 客户端生成的请求 UUID，用于幂等去重（仅 user 消息有值）
    prompt_tokens INTEGER DEFAULT 0,        -- 本轮 API 调用的 prompt token 数（仅 assistant 消息填充）
    completion_tokens INTEGER DEFAULT 0,    -- 本轮 API 调用的 completion token 数（仅 assistant 消息填充）
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_ai_messages_conv ON ai_messages(conversation_id, created_at);
CREATE UNIQUE INDEX idx_ai_messages_request_id ON ai_messages(conversation_id, request_id) WHERE request_id != '';
```

**建表方式**：使用版本迁移（在现有 `migrate()` 中新增迁移步骤）。

### 2.4 ai_schedules 表（SQLite）

```sql
CREATE TABLE ai_schedules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    report_type TEXT NOT NULL UNIQUE,       -- daily / weekly / monthly / quarterly / yearly
    enabled INTEGER NOT NULL DEFAULT 0,     -- 0=禁用 1=启用
    cron_expr TEXT NOT NULL,               -- cron 表达式，如 "0 7 * * *"
    last_run_at INTEGER,                   -- 上次执行时间（Unix 时间戳）
    next_run_at INTEGER,                   -- 下次执行时间（Unix 时间戳）
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

**默认定时计划：**

| 报告类型 | 默认 cron | 说明 | 默认状态 |
|---------|-----------|------|---------|
| daily | `0 7 * * *` | 每天早上 7:00 | 禁用 |
| weekly | `0 8 * * 1` | 每周一早上 8:00 | 禁用 |
| monthly | `0 8 1 * *` | 每月 1 号早上 8:00 | 禁用 |
| quarterly | `0 8 1 1,4,7,10 *` | 每季度首月 1 号早上 8:00 | 禁用 |
| yearly | `0 8 1 1 *` | 每年 1 月 1 号早上 8:00 | 禁用 |

**所有 cron 时间和报告时间窗口均基于配置的业务时区**（见十二节 `ai.timezone`），默认 `Asia/Shanghai`。

首次初始化时插入默认记录，用户在设置页可修改 cron 表达式和启用/禁用。

## 三、LLM Provider 设计

### 3.1 统一接口

```go
type Provider interface {
    // Complete 完整生成（用于报告）
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    // Stream 流式生成（用于对话）
    Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error)
    // Name 返回 provider 名称
    Name() string
}

type CompletionRequest struct {
    Model       string            // 模型名，如 "claude-sonnet-4-20250514"、"gpt-4o"、"qwen2.5:14b"
    Messages    []Message         // 对话消息列表
    MaxTokens   int               // 最大输出 token 数
    Temperature float64           // 温度参数
}

type Message struct {
    Role    string // system / user / assistant
    Content string
}

type CompletionResponse struct {
    Content    string
    TokenUsage TokenUsage
}

type TokenUsage struct {
    PromptTokens     int
    CompletionTokens int
    TotalTokens      int
}

type StreamChunk struct {
    Content string // 增量文本片段
    Done    bool   // 是否结束
    Error   error  // 错误信息
    Usage   *TokenUsage // 仅最后一个 chunk 携带
}
```

### 3.2 三种 Provider 实现

**Claude Provider：**
- 端点：`https://api.anthropic.com/v1/messages`
- 认证：`x-api-key` 头
- 流式：SSE（`stream: true`），解析 `content_block_delta` 事件
- 推荐模型：`claude-sonnet-4-20250514`（报告生成）、`claude-haiku-4-5-20251001`（对话，速度快）

**OpenAI Provider：**
- 端点：`https://api.openai.com/v1/chat/completions`（支持自定义 base_url，兼容国内代理和 Azure）
- 认证：`Authorization: Bearer` 头
- 流式：SSE（`stream: true`），解析 `choices[0].delta.content`
- 支持所有 OpenAI 兼容 API（通义千问、DeepSeek 等，只需改 base_url + api_key）

**Ollama Provider：**
- 端点：`http://<host>:11434/api/chat`
- 认证：无（内网访问）
- 流式：NDJSON（逐行 JSON），解析 `message.content`
- 模型：用户在设置中配置，如 `qwen2.5:14b`、`llama3:8b`

### 3.3 Provider 管理

```go
type ProviderManager struct {
    providers map[string]Provider  // name → Provider 实例
    active    string               // 当前激活的 provider 名称
    mu        sync.RWMutex
}

// 初始化时根据配置创建可用的 providers
func NewProviderManager(cfg *config.AIConfig) *ProviderManager

// 获取当前激活的 provider
func (m *ProviderManager) Active() Provider

// 切换激活的 provider
func (m *ProviderManager) SetActive(name string) error

// 列出所有已配置的 providers
func (m *ProviderManager) List() []ProviderInfo
```

## 四、数据采集器（DataCollector）

报告生成前，需要从 VictoriaMetrics + SQLite 采集指定时间窗口内的结构化数据，组装为 LLM 可理解的文本摘要。

### 4.1 采集的数据维度

```go
type ReportData struct {
    Period       Period                  // 时间窗口
    Servers      []ServerSummary         // 服务器概况
    Alerts       AlertSummary            // 告警统计
    Probes       ProbeSummary            // 探测统计
    Containers   ContainerSummary        // 容器统计
    Cloud        CloudSummary            // 云资源统计
    NAS          []NASSummary            // NAS 设备概况（NAS 模块实现后）
    AuditLog     AuditSummary            // 审计日志摘要
    Predictions  []PredictionInput       // 趋势预测所需的历史序列
}
```

### 4.2 各维度采集细节

**服务器指标（查询 VictoriaMetrics）：**

| 指标 | 查询方式 | 聚合 |
|------|---------|------|
| CPU 使用率 | `mantisops_cpu_usage_percent` | avg / max / p95 / 趋势斜率 |
| 内存使用率 | `mantisops_memory_usage_percent` | avg / max |
| 磁盘使用率 | `mantisops_disk_usage_percent` | 最新值 + 时间窗口内增长量 |
| 网络流量 | `mantisops_network_rx/tx_bytes_per_sec` | 总量 / 峰值 |
| 系统负载 | `mantisops_cpu_load1` | avg / max |
| GPU（如有）| `mantisops_gpu_*` | 使用率 / 温度 / 显存 |

对于每台服务器，使用 VM 的 `query_range` API 拉取时间窗口内的时序数据，然后在 Go 侧计算统计值。

**告警事件（查询 SQLite）：**

```go
type AlertSummary struct {
    TotalFired       int                // 时间窗口内触发的告警总数
    TotalResolved    int                // 时间窗口内恢复的告警总数
    CurrentFiring    int                // 当前仍在触发的告警数
    ByType           map[string]int     // 按告警类型分布
    BySeverity       map[string]int     // 按严重级别分布
    TopTargets       []TargetAlertCount // 告警最多的 Top 5 目标
    MeanTimeToResolve float64           // 平均恢复时间（分钟）
    Events           []AlertEventBrief  // 重要告警事件摘要（critical 级别）
}
```

**探测统计（查询 SQLite + VM）：**

```go
type ProbeSummary struct {
    TotalProbes     int               // 探测规则总数
    CurrentUp       int               // 当前正常数
    CurrentDown     int               // 当前异常数
    Availability    map[string]float64 // 每个探测目标的可用性百分比（基于 VM 历史数据）
    AvgLatency      map[string]float64 // 每个探测目标的平均延迟
    SSLExpiring     []SSLExpiryInfo    // 即将到期的 SSL 证书
}
```

**容器统计（查询 VM）：**

```go
type ContainerSummary struct {
    TotalContainers   int
    RunningContainers int
    StoppedContainers int
    TopCPU            []ContainerMetric // CPU 使用率 Top 5 容器
    TopMemory         []ContainerMetric // 内存使用 Top 5 容器
    StateChanges      int               // 时间窗口内状态变化次数
}
```

**云资源统计（查询 SQLite + VM）：**

```go
type CloudSummary struct {
    ECSInstances    int               // ECS 实例总数
    RDSInstances    int               // RDS 实例总数
    ExpiringResources []ExpiryInfo    // 即将到期的资源
    RDSMetrics      []RDSMetricBrief  // RDS CPU/内存/连接数摘要
}
```

**NAS 概况（NAS 模块实现后）：**

```go
type NASSummary struct {
    DeviceID    int64
    Name        string
    Status      string              // online / offline / degraded
    RAIDStatus  []RAIDStatusBrief   // RAID 阵列状态
    DiskHealth  []DiskHealthBrief   // 硬盘温度 / S.M.A.R.T. 状态
    VolumeUsage []VolumeUsageBrief  // 存储卷用量
    UPSStatus   *UPSStatusBrief     // UPS 状态（如有）
}
```

**审计日志摘要（查询 logs.db）：**

```go
type AuditSummary struct {
    TotalActions    int              // 操作总数
    ByCategory      map[string]int   // 按操作类别分布（create/update/delete/login 等）
    ByUser          map[string]int   // 按用户分布（多用户 RBAC 实现后）
    KeyEvents       []AuditEventBrief // 重要操作事件列表
}
```

**趋势预测输入：**

```go
type PredictionInput struct {
    ServerID  string
    Metric    string     // disk_usage / memory_usage / cpu_usage
    DataPoints []float64 // 时间窗口内按天聚合的数据点序列
}
```

趋势预测由 LLM 基于历史数据点序列进行分析，不做本地数学预测。

### 4.3 时间窗口映射

| 报告类型 | 数据采集窗口 | VM step 粒度 |
|---------|-------------|-------------|
| daily | 过去 24 小时 | 5m |
| weekly | 过去 7 天 | 1h |
| monthly | 过去 30 天 | 6h |
| quarterly | 过去 90 天 | 1d |
| yearly | 过去 365 天 | 1d |

### 4.4 数据转文本

采集完成后，将 `ReportData` 结构体序列化为结构化的文本摘要（非 JSON），作为 LLM system prompt 的一部分。格式示例：

```
=== 服务器概况 ===
共 5 台服务器，4 台在线，1 台离线。

# yuanqing2 (192.168.10.65) — 在线
- CPU：平均 23.5%，最高 78.2%（3月28日 14:32），P95 45.1%
- 内存：平均 62.3%，最高 71.0%
- 磁盘 /：使用 78.5%（151.7G / 193.2G），30天增长 2.3%
- 网络：日均入站 12.5 MB/s，出站 3.2 MB/s，峰值入站 89.1 MB/s
- 容器：运行中 8 个

# ai (192.168.10.69) — 在线
- CPU：平均 45.2%，最高 95.1%（3月28日 09:15）
- GPU RTX 3090：平均使用率 61.3%，温度均值 52°C，最高 78°C
...

=== 告警统计 ===
触发 12 次，恢复 10 次，当前仍有 2 条 firing。
- critical: 3 次（RAID 降级 1，服务器离线 2）
- warning: 9 次（CPU 5，磁盘 3，温度 1）
- 平均恢复时间：8.3 分钟
- 告警最多的目标：ai (5 次)、yuanqing2 (3 次)

=== 趋势预测数据 ===
# yuanqing2 磁盘 / 使用率（过去30天每日值）
[75.1, 75.3, 75.5, 75.8, 76.0, 76.2, 76.5, 76.8, 77.0, 77.2, ...]
```

## 五、报告生成器（Reporter）

### 5.1 Prompt 策略

每种报告类型使用独立的 system prompt 模板，包含：

1. **角色定义**：你是 MantisOps 运维分析助手，负责生成专业的运维分析报告
2. **报告要求**：按照指定格式输出 Markdown 报告
3. **数据注入**：将 DataCollector 采集的结构化数据文本嵌入 prompt
4. **输出格式**：指定 Markdown 章节结构

### 5.2 各报告类型的 Prompt 模板与输出结构

**日报（Daily Report）：**

```markdown
# {日期} 运维日报

## 一、整体概况
- 基础设施健康评分：{AI 基于数据给出 0-100 分}
- 关键指标摘要（一句话总结当日状态）

## 二、服务器状态
- 各服务器关键指标表格
- 异常事项（如有）

## 三、告警回顾
- 告警统计
- 重要告警事件时间线
- 根因分析（如果 AI 能推断）

## 四、端口与服务可用性
- 探测结果汇总
- 异常服务说明

## 五、容器状态
- 运行概况
- 异常容器（如有）

## 六、AI 洞察与建议
- 异常模式识别
- 资源优化建议
- 风险提示
```

**周报（Weekly Report）：**

在日报基础上增加：
- 本周 vs 上周对比（CPU/内存/磁盘趋势）
- 周度可用性 SLA 计算
- 告警趋势分析（按天分布）
- 容量变化分析

**月报（Monthly Report）：**

在周报基础上增加：
- 月度趋势图表数据（供前端渲染）
- 容量规划建议（磁盘增长预测）
- 告警模式分析（重复告警识别）
- 云资源到期提醒
- 成本优化建议（如有云资源数据）

**季度总结（Quarterly Summary）：**

在月报基础上增加：
- 三个月趋势对比
- 基础设施演进（新增/下线服务器、容器变化）
- 重大事件复盘
- 下季度预测与规划建议

**年度总结（Annual Summary）：**

在季度总结基础上增加：
- 全年回顾（12 个月趋势）
- 可靠性评分（基于告警频率、恢复时间、可用性）
- 基础设施增长分析
- 年度成本/资源趋势
- 来年容量规划建议

### 5.3 生成流程

```
手动触发 / 定时触发
  → 创建 ai_reports 记录（status=pending）
  → 计算时间窗口（period_start, period_end）
  → 更新 status=generating
  → WebSocket 广播 {type: "ai_report_generating", report_id: N}
  → DataCollector.Collect(period) → ReportData
  → 选择 Prompt 模板 + 注入数据文本
  → Provider.Complete(prompt) → Markdown 内容
  → 剥离 Markdown 标记后提取前 200 字纯文本作为 summary
  → 在同一事务中：
      1. 如果是 force 覆盖：先将同类型同窗口的旧 completed 报告标记为 superseded
      2. 更新当前报告 status=completed, content=markdown
    （事务保证唯一索引不会被撞：旧记录先退出 completed 状态，新记录再进入）
  → WebSocket 广播 {type: "ai_report_completed", report_id: N}
  → 如果失败：status=failed, error_message=错误详情
  → WebSocket 广播 {type: "ai_report_failed", report_id: N}
```

### 5.4 报告生成约束

- **并发控制**：同一时刻最多 1 个报告在生成（串行队列），避免 LLM API 并发压力
- **超时**：单次报告生成最长 5 分钟，超时标记为 failed
- **去重**：同类型 + 同时间窗口的报告不重复生成（手动触发时检查，已有则提示用户确认覆盖）
- **Token 限制**：数据注入后的 prompt 不超过模型上下文窗口的 60%，为输出留空间。如数据过多，按优先级截断（保留异常数据，裁剪正常数据）

## 六、定时调度器（Scheduler）

### 6.1 调度逻辑

```go
type Scheduler struct {
    store     *store.AIStore
    reporter  *Reporter
    ticker    *time.Ticker      // 每分钟 tick 一次
    mu        sync.Mutex
}

func (s *Scheduler) Start(ctx context.Context) {
    // 启动时计算所有 enabled 计划的 next_run_at
    // 每分钟检查：now >= next_run_at 的计划 → 触发报告生成 → 更新 last_run_at + next_run_at
}
```

### 6.2 Cron 解析

使用标准 5 段 cron 表达式（分 时 日 月 周），Go 侧用 `github.com/robfig/cron/v3` 的 `cron.ParseStandard(expr)` 获取 `Schedule` 接口，调用 `Schedule.Next(now)` 计算下次执行时间。仅使用其解析能力，调度逻辑自己实现（每分钟 tick 检查，与 MantisOps 现有的 ticker 模式一致）。

### 6.3 容错

- **崩溃恢复**：服务启动时，将所有 `pending` / `generating` 状态的报告标记为 `failed`（error_message='server_restart'），释放占位行。
- 如果服务重启时已过了 next_run_at，**不补生成**（避免启动时批量生成报告）。直接计算下一个 next_run_at。
- 生成失败不重试，等待下一个周期。用户可手动触发补生成。

## 七、AI 对话引擎（ChatEngine）

### 7.1 对话流程

```
用户发送消息（通过 REST API）
  → request_id 去重检查
  → 查找或创建 ai_conversations 记录
  → 存储 user message 到 ai_messages（message_id=42）
  → 预创建 assistant message 到 ai_messages（message_id=43, content='', status='streaming'）
  → 生成 stream_id（UUID），创建 stream 任务
  → 立即返回 {user_message_id:42, assistant_message_id:43, stream_id:xxx}
  → 等待前端 WebSocket 订阅 stream_id（超时 5 秒）
  → 订阅到达后：注入系统上下文（见 7.2）
  → 组装消息列表（system + 历史消息 + 新消息）
  → Provider.Stream(messages) → token 流
  → 通过 WebSocket 逐 chunk 推送给已订阅的客户端
  → 流结束后，回填 assistant message 的 content + token 统计，status='done'
  → 更新 ai_conversations.last_message_at + message_count
  → 如果失败（订阅超时 / LLM 调用异常 / 流中断）：
      更新 assistant message status='failed', error_message=错误详情
      WebSocket 推送 {type: "ai_chat_error", stream_id, message_id, error}
      前端据此将占位气泡切换为失败状态，显示错误信息和"重试"按钮
```

### 7.2 系统上下文注入

每次对话请求前，自动注入 system prompt，包含当前基础设施的实时快照：

```
你是 MantisOps 运维助手。以下是当前基础设施状态：

=== 服务器 ===
共 5 台，4 台在线。
- yuanqing2 (192.168.10.65)：CPU 23%, 内存 62%, 磁盘 78%
- ai (192.168.10.69)：CPU 45%, 内存 38%, GPU 61%
...

=== 当前告警 ===
2 条 firing：
- [critical] 服务器 zentao 离线（已持续 15 分钟）
- [warning] ai 服务器 CPU > 90%（当前 92.3%）

=== 探测状态 ===
12 个探测规则，11 正常，1 异常（xxx.com:443 DOWN）

基于以上数据回答用户的运维相关问题。如果用户问的问题超出你已有的数据范围，请如实告知。
```

**上下文更新策略**：每次新对话的首条消息注入完整快照。同一对话中，如果距离上次注入上下文超过 1 小时，自动刷新 system prompt（基础设施状态可能已变化）。用户也可显式要求"刷新数据"触发更新。

### 7.3 对话上下文窗口管理

- **最大历史消息数**：保留最近 20 条消息（10 轮对话）
- **超出时**：保留 system prompt + 最早 2 条 + 最近 16 条（保持对话连贯性）
- **长消息截断**：单条消息超过 4000 个 Unicode 字符（`utf8.RuneCountInString()`）时，在发送给 LLM 前截断并追加 `[...内容已截断]`

### 7.4 自动标题生成

新对话的首次 assistant 回复完成后，异步调用 LLM 生成对话标题：

```
Prompt: 根据以下对话内容，生成一个简短的中文标题（不超过 20 个字）：
用户：{first_user_message}
助手：{first_assistant_message}
只输出标题，不要任何额外内容。
```

使用低成本模型（如 haiku / gpt-4o-mini）生成，失败则保持"新对话"。

## 八、API 端点

### 8.1 AI 报告

```
GET    /api/v1/ai/reports                  # 报告列表（分页，支持 type/status 筛选）
GET    /api/v1/ai/reports/:id              # 报告详情（含完整 Markdown 内容）
POST   /api/v1/ai/reports/generate         # 手动触发生成报告
DELETE /api/v1/ai/reports/:id              # 删除报告
GET    /api/v1/ai/reports/:id/download     # 导出 Markdown 文件下载
GET    /api/v1/ai/reports/latest           # 获取最新一份已完成的报告（仪表盘卡片用）
```

**POST /api/v1/ai/reports/generate 请求：**

```json
{
    "report_type": "daily",
    "period_start": 1711641600,
    "period_end": 1711728000,
    "force": false
}
```

- `period_start` 和 `period_end` 可选。省略时后端根据 `report_type` 自动计算：
- `force` 可选，默认 false。当已存在同类型 + 同 period_start + 同 period_end 的 completed 报告时：不带 force 返回 `409 Conflict`（附带已存在报告 ID），`force=true` 则启动新报告生成，**成功时在同一事务中先将旧报告标记为 `superseded` 再将新报告标记为 `completed`**（保留历史记录，不丢数据）。如果新报告生成失败，旧报告保持不变。

自动计算规则（均基于 `ai.timezone` 配置的业务时区）：
- daily：昨日 00:00:00 ~ 23:59:59
- weekly：上周一 00:00:00 ~ 上周日 23:59:59
- monthly：上月 1 日 00:00:00 ~ 上月末日 23:59:59
- quarterly：上季度首日 00:00:00 ~ 末日 23:59:59
- yearly：去年 1.1 00:00:00 ~ 12.31 23:59:59

**GET /api/v1/ai/reports 响应：**

```json
{
    "reports": [
        {
            "id": 1,
            "report_type": "daily",
            "title": "2026年3月28日 运维日报",
            "summary": "整体运行平稳，健康评分 92/100。5 台服务器全部在线...",
            "status": "completed",
            "trigger_type": "scheduled",
            "provider": "claude",
            "model": "claude-sonnet-4-20250514",
            "token_usage": 8520,
            "generation_time_ms": 12300,
            "period_start": 1711555200,
            "period_end": 1711641600,
            "created_at": "2026-03-29T07:00:00Z"
        }
    ],
    "total": 15
}
```

### 8.2 AI 对话

```
GET    /api/v1/ai/conversations                    # 对话列表（分页）
POST   /api/v1/ai/conversations                    # 创建新对话
GET    /api/v1/ai/conversations/:id                # 对话详情（含所有消息）
DELETE /api/v1/ai/conversations/:id                # 删除对话
POST   /api/v1/ai/conversations/:id/messages       # 发送消息（触发 AI 回复，流式通过 WebSocket 推送）
```

**POST /api/v1/ai/conversations/:id/messages 请求：**

```json
{
    "content": "哪台服务器本周 CPU 使用率最高？",
    "request_id": "client-uuid-123"
}
```

`request_id` 由前端生成（UUID），后端基于 `conversation_id + request_id` 去重。如果相同 `request_id` 重复提交，后端返回已有的 `message_id` 和 `stream_id`，不重复调用 LLM。

**响应（立即返回，AI 回复通过 WebSocket 流式推送）：**

```json
{
    "user_message_id": 42,
    "assistant_message_id": 43,
    "conversation_id": 5,
    "stream_id": "550e8400-e29b-41d4-a716-446655440000",
    "status": "streaming"
}
```

- `user_message_id`：用户消息的 ID（已落库）
- `assistant_message_id`：预创建的 assistant 消息 ID（content 为空，流结束后回填）。前端用此 ID 创建占位消息气泡，WebSocket chunk 中的 `message_id` 与此一致
- `stream_id`：前端收到后立即通过 WebSocket 发送 `ai_stream_subscribe` 订阅

### 8.3 AI 设置

```
GET    /api/v1/ai/settings                # 获取 AI 配置（providers + schedules）
PUT    /api/v1/ai/settings                # 更新 AI 配置（底层存储在 settings 表，key 前缀 ai.*）
GET    /api/v1/ai/providers               # 列出可用 providers 及状态
POST   /api/v1/ai/providers/test          # 测试 provider 连通性
GET    /api/v1/ai/schedules               # 获取定时计划列表
PUT    /api/v1/ai/schedules/:id           # 更新定时计划（启用/禁用/修改 cron）
```

**POST /api/v1/ai/providers/test 请求：**

```json
{
    "provider": "claude",
    "api_key": "sk-ant-...",
    "model": "claude-sonnet-4-20250514"
}
```

发送一条简短的测试 prompt（"回复 OK"），验证 API 密钥和模型是否可用。

**Ollama 测试请求示例：**

```json
{
    "provider": "ollama",
    "host": "http://192.168.10.69:11434",
    "model": "qwen2.5:14b"
}
```

各 provider 的测试请求字段不同：Claude/OpenAI 需要 `api_key`，Ollama 需要 `host`，都需要 `model`。

## 九、WebSocket 消息

复用现有 `/ws` 连接，新增消息类型：

**服务端 → 客户端：**

```json
// AI 对话流式输出（逐 token 推送，message_id 对应 assistant_message_id）
{"type": "ai_chat_chunk", "stream_id": "xxx", "conversation_id": 5, "message_id": 43, "content": "根据", "done": false}
{"type": "ai_chat_chunk", "stream_id": "xxx", "conversation_id": 5, "message_id": 43, "content": "本周数据", "done": false}
{"type": "ai_chat_chunk", "stream_id": "xxx", "conversation_id": 5, "message_id": 43, "content": "", "done": true, "token_usage": {"prompt_tokens": 1200, "completion_tokens": 350}}

// AI 对话失败（订阅超时、LLM 调用失败等）
{"type": "ai_chat_error", "stream_id": "xxx", "conversation_id": 5, "message_id": 43, "error": "LLM API 请求超时"}

// 报告开始生成通知（前端据此显示"生成中"卡片）
{"type": "ai_report_generating", "report_id": 1, "report_type": "daily", "title": "2026年3月28日 运维日报"}

// 报告生成完成通知
{"type": "ai_report_completed", "report_id": 1, "report_type": "daily", "title": "2026年3月28日 运维日报"}

// 报告生成失败通知
{"type": "ai_report_failed", "report_id": 1, "error": "LLM API 请求超时"}
```

**广播策略**：`ai_chat_chunk` 和 `ai_chat_error` 均仅推送给订阅了对应 `stream_id` 的客户端连接（通过 `BroadcastAIStreamJSON` 定向分发）。`ai_report_generating` / `ai_report_completed` / `ai_report_failed` 广播给所有连接。

### 9.1 Hub 改造 — 基于 stream_id 订阅

采用与现有 `log_subscribe` / `log_unsubscribe` 一致的订阅模式，不引入 Client ID：

**流程（解决订阅竞态）：**
1. 前端通过 REST API 发送对话消息，后端创建 stream 任务但**不立即启动 LLM 调用**，返回 `stream_id`（UUID）
2. 前端通过 WebSocket 发送订阅消息：`{"type": "ai_stream_subscribe", "stream_id": "xxx"}`
3. 后端在 Hub 中记录该 client 订阅的 stream_id，**订阅注册完成后触发 LLM 流式调用开始**（通过 channel 通知）
4. LLM 流式输出时，Hub 只向订阅了该 stream_id 的 client 推送 chunk
5. 流结束后，后端自动清理订阅关系
6. **超时保护**：如果订阅消息在 5 秒内未到达，后端取消该 stream 任务，标记为失败

关键点：LLM 调用被阻塞在一个 channel 上，直到收到 WebSocket 订阅消息后才放行，确保不会丢失任何首屏 token。

**Hub 新增：**

```go
// Client 结构体新增字段
type Client struct {
    conn     *websocket.Conn
    mu       sync.Mutex
    logSub   bool              // 已有：日志订阅
    aiStream map[string]bool   // 新增：AI stream_id 订阅集合
}

// BroadcastAIStreamJSON 向订阅了指定 stream_id 的客户端推送消息
func (h *Hub) BroadcastAIStreamJSON(streamID string, msg interface{})
```

**客户端 WebSocket 消息：**

```json
// 订阅（收到 REST 响应中的 stream_id 后立即发送）
{"type": "ai_stream_subscribe", "stream_id": "550e8400-e29b-41d4-a716-446655440000"}

// 取消订阅（流结束后前端主动发送，或后端流结束时自动清理）
{"type": "ai_stream_unsubscribe", "stream_id": "550e8400-e29b-41d4-a716-446655440000"}
```

这与现有的 `log_subscribe` 模式完全一致，改造量最小。

## 十、前端设计

### 10.1 侧边栏菜单

在「告警中心」和「资源到期」之间新增：
- **AI 报告**（图标：`analytics`，Material Symbols）
- 路由：`/ai-reports`

### 10.2 AI 报告列表页（`/ai-reports`）

```
┌─────────────────────────────────────────────────────────────────┐
│  AI 报告                                          [+ 生成报告]   │
│                                                                 │
│  ┌─ 筛选 Tab ─────────────────────────────────────────────────┐ │
│  │ 全部(15) │ 日报(10) │ 周报(3) │ 月报(1) │ 季度(1) │ 年度(0) │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌─ 报告卡片 ─────────────────────────────────────────────────┐ │
│  │  2026年3月28日 运维日报                          日报  自动  │ │
│  │  整体运行平稳，健康评分 92/100。5 台服务器全部在线，         │ │
│  │  共触发 3 次告警均已自动恢复...                              │ │
│  │  claude-sonnet-4 · 8,520 tokens · 12.3s       03-29 07:00  │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌─ 报告卡片 ─────────────────────────────────────────────────┐ │
│  │  2026年第12周 运维周报                          周报  手动  │ │
│  │  本周基础设施整体可用性 99.7%，较上周提升 0.2%...            │ │
│  │  claude-sonnet-4 · 12,800 tokens · 18.7s      03-28 10:30  │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌─ 生成中（收到 ai_report_generating 后展示）────────────────┐ │
│  │  ⏳ 正在生成 2026年3月 运维月报...                          │ │
│  │  已运行 25s（前端基于 generating 事件时间戳本地计时）        │ │
│  └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**「生成报告」对话框：**
- 报告类型：下拉选择（日报/周报/月报/季度/年度）
- 时间范围：根据类型自动填充，可手动调整
- 「生成」按钮

### 10.3 报告详情页（`/ai-reports/:id`）

```
┌─────────────────────────────────────────────────────────────────┐
│  ← 返回    2026年3月28日 运维日报              [导出 Markdown]   │
│  claude-sonnet-4 · 8,520 tokens · 12.3s · 自动生成              │
│                                                                 │
│  ┌─ Markdown 渲染区域 ────────────────────────────────────────┐ │
│  │                                                            │ │
│  │  # 一、整体概况                                             │ │
│  │  基础设施健康评分：92/100                                    │ │
│  │  今日整体运行平稳，5 台服务器全部保持在线状态...              │ │
│  │                                                            │ │
│  │  # 二、服务器状态                                           │ │
│  │  | 服务器 | CPU 均值 | CPU 峰值 | 内存 | 磁盘 |             │ │
│  │  |--------|---------|---------|------|------|              │ │
│  │  | yuanqing2 | 23.5% | 78.2% | 62% | 78% |              │ │
│  │  ...                                                       │ │
│  │                                                            │ │
│  │  # 六、AI 洞察与建议                                        │ │
│  │  1. yuanqing2 磁盘使用率持续增长，按当前速率预计 45 天后     │ │
│  │     达到 90%，建议提前清理或扩容。                           │ │
│  │  2. ai 服务器 GPU 温度在工作时段频繁接近 75°C...            │ │
│  │                                                            │ │
│  └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

Markdown 渲染使用 `react-markdown` + `remark-gfm`（支持表格、任务列表等 GFM 扩展）。

### 10.4 AI 助手浮窗（全局）

右下角固定悬浮按钮，点击展开对话面板：

```
                                    ┌─────────────────────────┐
                                    │ AI 助手        [─] [×]  │
                                    │ ┌─ 对话列表 ──────────┐ │
                                    │ │ • 服务器性能咨询     │ │
                                    │ │ • 磁盘扩容建议       │ │
                                    │ │ + 新对话             │ │
                                    │ └────────────────────┘ │
                                    │                         │
                                    │ ┌─ 对话区域 ──────────┐ │
                                    │ │ 🧑 哪台服务器本周   │ │
                                    │ │    CPU 最高？        │ │
                                    │ │                      │ │
                                    │ │ 🤖 根据本周数据，   │ │
                                    │ │    ai 服务器的 CPU   │ │
                                    │ │    使用率最高，      │ │
                                    │ │    峰值达到 95.1%    │ │
                                    │ │    （周三 09:15），   │ │
                                    │ │    主要由 Ollama     │ │
                                    │ │    推理任务引起...   │ │
                                    │ │    █                 │ │
                                    │ └────────────────────┘ │
                                    │ ┌────────────────[发送]┐ │
                                    │ │ 输入消息...          │ │
                                    │ └────────────────────┘ │
                                    └─────────────────────────┘
                                              [🤖 AI]  ← 悬浮按钮
```

**浮窗特性：**
- 右下角悬浮按钮，带脉冲动画（与 Kinetic Observatory 风格一致）
- 点击展开 400px 宽 × 600px 高的玻璃拟态面板
- 左侧对话列表（可收起），右侧对话内容
- 流式输出：逐 token 显示，打字机效果
- 支持最小化（收回悬浮按钮）和关闭
- 响应式：窗口宽度 < 768px 时改为全屏覆盖模式
- 新对话自动注入当前基础设施上下文
- 对话标题自动生成

### 10.5 仪表盘摘要卡片

在仪表盘「摘要区域」（告警摘要、数据库状态、到期资源三列下方或替换其中一列）新增 AI 分析摘要卡片：

```
┌─ AI 分析 ───────────────────────────────────────┐
│  📊 最新日报 · 3月28日 · 健康评分 92/100         │
│                                                  │
│  整体运行平稳，5 台服务器全部在线。               │
│  yuanqing2 磁盘使用率持续增长，建议关注。         │
│                                                  │
│  [查看完整报告 →]                                │
└──────────────────────────────────────────────────┘
```

如果没有已生成的报告，显示空状态：「暂无 AI 报告，前往设置页配置 AI 后开始使用」。

### 10.6 设置页 AI 配置区块

在现有设置页中新增 **AI 配置** 区块：

```
AI 配置                                                    [测试连接]
┌────────────────────────────────────────────────────────────────┐
│  LLM 服务商                                                    │
│  ┌─ 当前：Claude API ─────────────────────────── [切换] ─────┐ │
│  │  API Key: sk-ant-***...***abc                   [编辑]    │ │
│  │  报告模型: claude-sonnet-4-20250514              [▼]      │ │
│  │  对话模型: claude-haiku-4-5-20251001             [▼]      │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                │
│  已配置的服务商                                                 │
│  • Claude API    ✅ 已连接    [编辑] [删除]                     │
│  • OpenAI API    ⚠️ 未配置   [配置]                            │
│  • Ollama        ✅ 已连接    [编辑] [删除]                     │
│                                                                │
│  定时报告                                                      │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  日报   每天 07:00      [开关]   [编辑 cron]               │ │
│  │  周报   每周一 08:00    [开关]   [编辑 cron]               │ │
│  │  月报   每月1日 08:00   [开关]   [编辑 cron]               │ │
│  │  季度   每季首月1日     [开关]   [编辑 cron]               │ │
│  │  年度   每年1月1日      [开关]   [编辑 cron]               │ │
│  └────────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────┘
```

### 10.7 新增前端文件

```
web/src/pages/AIReports/index.tsx           # AI 报告列表页
web/src/pages/AIReports/ReportDetail.tsx    # 报告详情页
web/src/components/AIChat/ChatButton.tsx    # AI 悬浮按钮
web/src/components/AIChat/ChatPanel.tsx     # AI 对话面板
web/src/api/ai.ts                          # AI API 客户端
web/src/stores/aiStore.ts                  # AI Zustand store
```

### 10.8 修改前端文件

```
web/src/App.tsx                             # 新增 /ai-reports 和 /ai-reports/:id 路由
web/src/components/Layout/Sidebar.tsx       # 新增 AI 报告菜单项
web/src/components/Layout/MainLayout.tsx    # 挂载 AI 助手浮窗组件
web/src/hooks/useWebSocket.ts              # 新增 ai_chat_chunk / ai_report_completed 处理
web/src/pages/Settings/index.tsx           # 新增 AI 配置区块
web/src/pages/Dashboard/index.tsx          # 新增 AI 分析摘要卡片（仪表盘首页路由即 /）
```

## 十一、后端新增文件

```
server/internal/ai/provider.go             # Provider 接口 + ProviderManager
server/internal/ai/claude.go               # Claude API Provider 实现
server/internal/ai/openai.go               # OpenAI API Provider 实现
server/internal/ai/ollama.go               # Ollama Provider 实现
server/internal/ai/reporter.go             # 报告生成器
server/internal/ai/scheduler.go            # 定时调度器
server/internal/ai/chat.go                 # 对话引擎
server/internal/ai/data_collector.go       # 数据采集器
server/internal/ai/prompts.go              # Prompt 模板
server/internal/store/ai_store.go          # ai_reports + ai_conversations + ai_messages CRUD
server/internal/api/ai_handler.go          # HTTP API handler
```

## 十二、配置结构

`server.yaml` 新增 `ai` 段：

```yaml
ai:
  enabled: false                          # 总开关
  active_provider: ""                     # 当前激活的 provider（claude / openai / ollama）
  timezone: "Asia/Shanghai"               # 业务时区，影响 cron 触发时刻和报告时间窗口计算

  claude:
    api_key: ""                           # 初始导入用，运行时加密存入 settings 表
    report_model: "claude-sonnet-4-20250514"
    chat_model: "claude-haiku-4-5-20251001"
    max_tokens: 8192                      # 最大输出 token

  openai:
    api_key: ""
    base_url: "https://api.openai.com/v1" # 支持自定义（兼容通义千问、DeepSeek 等）
    report_model: "gpt-4o"
    chat_model: "gpt-4o-mini"
    max_tokens: 8192

  ollama:
    host: "http://192.168.10.69:11434"    # Ollama 服务地址
    report_model: "qwen2.5:14b"
    chat_model: "qwen2.5:7b"
    max_tokens: 4096

  report:
    max_generation_time: 300              # 单次报告生成超时（秒）
    max_concurrent: 1                     # 最大并发生成数

  chat:
    max_history_messages: 20              # 对话最大历史消息数
    max_message_length: 4000             # 单条消息最大 Unicode 字符数
    system_context_refresh: false         # true=每轮对话都刷新系统上下文，false=仅超过 1 小时未刷新时自动刷新
```

**API Key 存储**：Claude/OpenAI 的 API Key 属于敏感信息。**不复用现有的 credentials 表**（credentials 是面向 SSH/云账号的设备凭据，有 `used_by` 引用计数等机制，与 AI API Key 的使用场景不匹配）。API Key 通过现有的 AES-256-GCM `crypto` 模块加密后存入 `settings` 表（key 为 `ai.claude.api_key` 等），与其他 AI 配置统一管理。server.yaml 中的 api_key 字段仅作为初始导入用途（与阿里云 AK/SK 的处理方式一致）。

## 十三、现有代码改造

| 改造项 | 影响范围 |
|--------|---------|
| SQLite 建表 | store/sqlite.go 版本迁移新增 ai_reports、ai_conversations、ai_messages、ai_schedules 表 |
| 路由注册 | router.go 新增 /ai/* 路由 |
| main.go 初始化 | 创建 ProviderManager、Reporter、Scheduler、ChatEngine，注入依赖 |
| WebSocket Hub | hub.go Client 结构体新增 aiStream 字段，新增 BroadcastAIStreamJSON() 方法 + ai_stream_subscribe/unsubscribe 消息处理 |
| WebSocket 消息 | 新增 ai_chat_chunk / ai_chat_error / ai_report_generating / ai_report_completed / ai_report_failed 类型 |
| 配置结构 | config.go 新增 AIConfig 结构体 |
| 设置页 | Settings/index.tsx 新增 AI 配置区块 |
| 仪表盘 | Dashboard 页面新增 AI 摘要卡片 |
| 侧边栏 | Sidebar.tsx 新增菜单项 |
| 审计中间件 | logging/middleware.go auditRoutes 新增 AI 相关路由的审计记录 |

## 十四、不做的事（YAGNI）

- 不做 PDF 导出（已确认去掉）
- 不做 RAG / 向量数据库（数据量不大，直接在 prompt 中注入结构化摘要足够）
- 不做 AI 自动执行运维操作（只分析和建议，不触发实际操作）
- 不做多语言报告（固定中文输出）
- 不做报告模板自定义（第一版使用内置 prompt 模板）
- 不做 AI 告警规则自动创建（只做建议，不自动落库）
- 不做本地模型下载管理（Ollama 模型管理由用户在 Ollama 侧操作）
- 不做对话中的图表渲染（对话只输出文本，图表留给报告页面）
- 不做 Function Calling / Tool Use（第一版 LLM 不调用工具，数据预先注入 prompt）
