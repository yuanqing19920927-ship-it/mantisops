# MantisOps 日志中心设计文档

> 日期：2026-03-28
> 状态：已确认（rev.2 — 修复审查问题）

## 一、架构总览

```
┌─────────────┐    gRPC(ReportLogs)     ┌──────────────────────────────────┐
│   Agent ×N  │ ──── 30s批量上报 ────→  │         MantisOps Server         │
│ (本地缓冲)   │                        │                                  │
└─────────────┘                        │  ┌─ LogManager ──────────────┐   │
                                       │  │ • 异步写入队列(chan)       │   │
┌─────────────┐   Gin中间件拦截         │  │ • 写入日志文件(按天轮转)    │   │
│  HTTP API   │ ──────────────────→    │  │ • 写入SQLite索引(logs.db)  │   │
│  操作审计    │                        │  │ • WebSocket实时广播        │   │
└─────────────┘                        │  │ • 定时清理过期日志          │   │
                                       │  └──────────────────────────┘   │
┌─────────────┐   内部各模块调用        │         ↓ WebSocket              │
│ 告警/采集器  │ ──────────────────→    │                                  │
│ 探测/部署器  │                        └──────────────────────────────────┘
└─────────────┘                                   ↓
                                       ┌──────────────────────────────────┐
                                       │        前端「日志中心」页面        │
                                       │  ┌──────────┬─────────────────┐  │
                                       │  │ 操作审计  │    运行日志      │  │
                                       │  │          │ (Server+Agent)  │  │
                                       │  └──────────┴─────────────────┘  │
                                       └──────────────────────────────────┘
```

### 1.1 写入路径与性能隔离

所有日志写入通过**异步队列**隔离，不阻塞请求/采集热路径：

```
调用方 → logMgr.Info() → 格式化 JSON → 写入 channel(缓冲 4096)
                                              ↓ (后台 goroutine)
                                     ┌─ 批量写文件 (fsync 每 1s)
                                     ├─ 批量写 SQLite 索引 (事务每 1s)
                                     ├─ stdout 输出
                                     └─ WebSocket 广播
```

- channel 满时丢弃 DEBUG 级别日志，WARN/ERROR 保证不丢（阻塞等待）
- 文件写入和 SQLite 写入在同一个后台 goroutine，保证顺序一致
- WebSocket 广播是 fire-and-forget，不阻塞写入

## 二、存储设计

### 2.1 日志文件（主存储，全文真实来源）

```
{logging.dir}/
├── audit/                    # 操作审计
│   ├── 2026-03-28.log
│   └── 2026-03-29.log
├── system/                   # 服务端运行日志
│   ├── 2026-03-28.log
│   └── 2026-03-29.log
└── agent/                    # Agent 日志（按 host_id 分目录）
    ├── srv-71-opsboard/
    │   └── 2026-03-28.log
    └── srv-69-ai/
        └── 2026-03-28.log
```

每行一条 JSON：

```json
{"ts":"2026-03-28T10:05:32.123Z","level":"info","module":"alerter","msg":"FIRE rule=3 target=srv-71 value=92.5","host_id":"","trace_id":"a1b2c3"}
```

字段规范：

| 字段 | 类型 | 说明 |
|------|------|------|
| ts | string | ISO 8601 时间戳，毫秒精度 |
| level | string | debug / info / warn / error |
| module | string | alerter / collector / deployer / probe / grpc / api / agent |
| msg | string | 日志消息（完整内容） |
| host_id | string | Agent 来源标识，系统日志为空 |
| trace_id | string | 可选，用于关联请求链路 |

### 2.2 SQLite 索引（独立 logs.db）

**表 audit_logs — 操作审计全文存储**

```sql
CREATE TABLE audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL,
    username TEXT NOT NULL DEFAULT '',  -- 操作人（登录请求从 body 提取，可为空字符串表示匿名/系统操作）
    action TEXT NOT NULL,               -- login / logout / create / update / delete / sync / deploy / uninstall / test / ack
    resource_type TEXT NOT NULL,        -- auth / alert_rule / alert_event / cloud_account / managed_server / probe / asset / channel / credential / server / group
    resource_id TEXT DEFAULT '',
    resource_name TEXT DEFAULT '',
    detail TEXT DEFAULT '',             -- 变更详情 JSON
    ip_address TEXT DEFAULT '',
    user_agent TEXT DEFAULT ''
);
CREATE INDEX idx_audit_timestamp ON audit_logs(timestamp DESC);
CREATE INDEX idx_audit_username ON audit_logs(username, timestamp DESC);
CREATE INDEX idx_audit_resource ON audit_logs(resource_type, timestamp DESC);
```

**表 log_index — 运行日志索引（索引 + 文件回查）**

```sql
CREATE TABLE log_index (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL,
    level TEXT NOT NULL,
    module TEXT NOT NULL,
    source TEXT NOT NULL,            -- server / agent:{host_id}
    file_path TEXT NOT NULL,         -- 日志文件相对路径
    file_offset INTEGER NOT NULL,   -- 文件内字节偏移
    line_length INTEGER NOT NULL,   -- 行字节长度（用于精确读取原始行）
    message_preview TEXT             -- 消息前 200 字符（用于列表快速展示）
);
CREATE INDEX idx_log_timestamp ON log_index(timestamp DESC);
CREATE INDEX idx_log_level ON log_index(level, timestamp DESC);
CREATE INDEX idx_log_module ON log_index(module, timestamp DESC);
CREATE INDEX idx_log_source ON log_index(source, timestamp DESC);
```

### 2.3 关键字搜索的查询路径

关键字搜索针对**完整日志内容**，而非仅 message_preview。查询分两步：

1. **索引预筛**：先用 timestamp/level/module/source 缩小范围，得到候选 log_index 行
2. **文件回扫**：对候选行，通过 `file_path + file_offset + line_length` 从原始日志文件读取完整 JSON 行，对 `msg` 字段做关键字匹配

性能保护：
- 必须指定时间范围（默认最近 1 小时），避免全量扫描
- 单次查询最多回扫 10,000 条候选行，超出返回提示"请缩小时间范围或增加筛选条件"
- message_preview 的 LIKE 作为快速路径：如果 preview 已命中则跳过文件读取

### 2.4 配置（server.yaml 新增段）

```yaml
logging:
  dir: "./logs"
  level: "info"                     # 最低记录级别
  buffer_size: 4096                 # 异步队列缓冲大小
  retention:
    audit_days: 90
    system_days: 30
    agent_days: 7
  cleanup_hour: 3                   # 每天几点执行清理
```

## 三、后端设计

### 3.1 新增 Go 包

```
server/internal/logging/
├── manager.go          # LogManager：异步队列 + 统一日志入口
├── writer.go           # 按天轮转文件写入器
├── store.go            # logs.db SQLite 操作
├── query.go            # 关键字搜索（索引预筛 + 文件回扫）
├── cleanup.go          # 定时清理
└── middleware.go        # Gin 审计中间件
```

### 3.2 LogManager 接口

```go
type LogManager struct {
    ch       chan logEntry          // 异步写入队列
    writer   *RotatingWriter
    store    *LogStore
    hub      *ws.Hub
    level    Level
    logDir   string
}

// 系统运行日志（调用方只做格式化+入队，不阻塞）
func (m *LogManager) Info(module, msg string, args ...any)
func (m *LogManager) Warn(module, msg string, args ...any)
func (m *LogManager) Error(module, msg string, args ...any)
func (m *LogManager) Debug(module, msg string, args ...any)

// 操作审计（同步写入，保证不丢）
func (m *LogManager) Audit(username, action, resourceType, resourceID, resourceName, detail, ip, ua string)

// Agent 日志接收
func (m *LogManager) IngestAgentLogs(hostID string, batchID uint64, entries []*pb.LogEntry) uint64
```

后台 goroutine 批处理逻辑：
- 每 1 秒或累积 100 条时 flush 一次
- 单次 flush：批量写文件 → 单事务写 SQLite 索引 → 逐条 WebSocket 广播
- Audit 方法直接同步写（不经过 channel），保证审计日志不因队列满而丢失

### 3.3 Gin 审计中间件

#### 字段映射规则

中间件在 `c.Next()` **之后**执行（请求已处理完），根据路径和方法映射字段：

| 路径模式 | HTTP 方法 | action | resource_type | username 来源 |
|---------|-----------|--------|---------------|--------------|
| /auth/login | POST | login | auth | **从请求 body 的 username 字段提取**（非 JWT context） |
| /alerts/rules | POST | create | alert_rule | JWT context |
| /alerts/rules/:id | PUT | update | alert_rule | JWT context |
| /alerts/rules/:id | DELETE | delete | alert_rule | JWT context |
| /alerts/events/:id/ack | PUT | ack | alert_event | JWT context |
| /alerts/channels | POST | create | channel | JWT context |
| /alerts/channels/:id | PUT | update | channel | JWT context |
| /alerts/channels/:id | DELETE | delete | channel | JWT context |
| /alerts/channels/:id/test | POST | test | channel | JWT context |
| /probes | POST | create | probe | JWT context |
| /probes/:id | PUT | update | probe | JWT context |
| /probes/:id | DELETE | delete | probe | JWT context |
| /assets | POST | create | asset | JWT context |
| /assets/:id | PUT | update | asset | JWT context |
| /assets/:id | DELETE | delete | asset | JWT context |
| /cloud-accounts | POST | create | cloud_account | JWT context |
| /cloud-accounts/:id | PUT | update | cloud_account | JWT context |
| /cloud-accounts/:id | DELETE | delete | cloud_account | JWT context |
| /cloud-accounts/:id/sync | POST | sync | cloud_account | JWT context |
| /managed-servers | POST | create | managed_server | JWT context |
| /managed-servers/:id/deploy | POST | deploy | managed_server | JWT context |
| /managed-servers/:id/retry | POST | deploy | managed_server | JWT context |
| /managed-servers/:id/uninstall | POST | uninstall | managed_server | JWT context |
| /managed-servers/:id | DELETE | delete | managed_server | JWT context |
| /credentials | POST | create | credential | JWT context |
| /credentials/:id | PUT | update | credential | JWT context |
| /credentials/:id | DELETE | delete | credential | JWT context |
| /servers/:id/name | PUT | update | server | JWT context |
| /servers/:id/group | PUT | update | server | JWT context |
| /groups | POST | create | group | JWT context |
| /groups/:id | PUT | update | group | JWT context |
| /groups/:id | DELETE | delete | group | JWT context |

#### 特殊处理

- **登录请求**：`/auth/login` 是公开端点，无 JWT。中间件需要特殊处理：读取请求 body 中的 `username` 字段（body 需通过 `c.Get("audit_username")` 由 Login handler 主动设置到 context 中，避免重复读 body）
- **登录失败**：HTTP status >= 400 时跳过审计（不记录失败的登录尝试）
- **resource_id**：从 URL 路径参数 `:id` 提取
- **resource_name**：从响应 body 的 `name` 字段提取（如有），否则为空

### 3.4 结构化日志替换

将现有约 60 处 `log.Printf("[module] ...")` 替换为 `logMgr.Info("module", ...)`。

替换后保留 stdout 双写（LogManager 后台 goroutine 中同时写 stdout）。

### 3.5 gRPC Agent 日志上报

#### Protobuf 新增

```protobuf
message LogEntry {
    int64  timestamp = 1;    // Unix 毫秒
    string level = 2;        // info/warn/error
    string module = 3;       // collector/reporter 等
    string message = 4;
}

message ReportLogsRequest {
    string host_id = 1;
    uint64 batch_id = 2;            // Agent 单调递增的批次号
    repeated LogEntry entries = 3;
}

message ReportLogsResponse {
    bool   ok = 1;
    uint64 acked_batch_id = 2;      // Server 确认已落盘的批次号
}

service AgentService {
    // ... 现有方法
    rpc ReportLogs(ReportLogsRequest) returns (ReportLogsResponse);
}
```

#### 幂等上报协议

解决"服务端已落盘但响应丢失"的灰区问题：

1. Agent 为每批日志分配单调递增的 `batch_id`（uint64，持久化到本地文件）
2. Server 端维护 `map[host_id]uint64` 记录每个 Agent 最后成功落盘的 batch_id
3. Server 收到请求时：
   - 如果 `batch_id <= last_acked`：该批已落盘，直接返回 `ok=true, acked_batch_id=batch_id`（幂等跳过）
   - 如果 `batch_id == last_acked + 1`：正常落盘，更新 last_acked，返回确认
   - 如果 `batch_id > last_acked + 1`：中间有批次丢失，仍然落盘当前批次，但在日志中记录 gap 警告
4. Agent 收到响应后：
   - `acked_batch_id >= 本地 batch_id`：截断缓冲，推进到下一批
   - 超时/网络错误：下次重试相同 batch_id（幂等，不会重复写入）

#### Agent 端本地缓冲

- 缓冲文件：`~/.config/mantisops/log-buffer`
- 批次号文件：`~/.config/mantisops/log-batch-id`（uint64 文本）
- 每 30s 读取缓冲，打包为一个 batch 上报
- 上报确认后截断缓冲文件、递增批次号

### 3.6 API 端点

```
GET  /api/v1/logs/audit              # 操作审计列表（分页 + 筛选）
GET  /api/v1/logs/runtime            # 运行日志列表（分页 + 筛选 + 关键字搜索）
GET  /api/v1/logs/export             # 导出日志（CSV/JSON）
GET  /api/v1/logs/sources            # 日志来源列表（Server + 各 Agent）
GET  /api/v1/logs/stats              # 日志统计（各级别/来源数量）
```

查询参数：

| 参数 | 类型 | 说明 |
|------|------|------|
| start | datetime | 起始时间（**必填**，默认 1 小时前） |
| end | datetime | 结束时间（默认当前） |
| level | string | 级别筛选（逗号分隔多选） |
| module | string | 模块筛选 |
| source | string | 来源筛选 |
| keyword | string | 关键字搜索（搜索完整消息内容，见 2.3 节查询路径） |
| username | string | 操作人（仅 audit） |
| action | string | 操作类型（仅 audit） |
| page | int | 页码 |
| page_size | int | 每页条数（默认 50，最大 200） |

导出额外参数：type (audit/runtime)、format (csv/json)。

## 四、前端设计

### 4.1 侧边栏菜单

在「告警中心」和「资源到期」之间新增：
- 日志中心（图标：article）
- 路由：/logs

### 4.2 页面结构

```
┌─────────────────────────────────────────────────────────┐
│  日志中心                                                │
│  ┌──────────┬───────────────┐                           │
│  │ 操作审计  │   运行日志     │    ← Tab 切换             │
│  └──────────┴───────────────┘                           │
│                                                         │
│  ┌─ 筛选栏 ────────────────────────────────────────────┐ │
│  │ 时间范围 │ 级别 │ 模块 │ 来源 │ 关键字搜索 │ 导出 ↓ │ │
│  └─────────────────────────────────────────────────────┘ │
│                                                         │
│  ┌─ 工具栏 ───────────────────────────────────────────┐  │
│  │ ● 实时  ○ 查询     共 1,234 条    ↻ 刷新           │  │
│  └────────────────────────────────────────────────────┘  │
│                                                         │
│  ┌─ 日志列表 ─────────────────────────────────────────┐  │
│  │ 10:05:32 ERROR [alerter] FIRE rule=3 target=...    │  │
│  │ 10:05:33 INFO  [collector] collected srv-71: 12... │  │
│  │ 10:05:35 WARN  [probe] timeout checking 443...     │  │
│  │ ...                                                │  │
│  └────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### 4.3 操作审计 Tab

表格列：时间、操作人、操作、资源类型、资源名称、IP 地址、详情（可展开）。

### 4.4 运行日志 Tab

两种模式：
- **实时模式**（默认）：WebSocket 实时追加，自动滚动
- **查询模式**：REST API 分页查询历史

日志行样式：
- 级别颜色：DEBUG 灰、INFO 绿、WARN 黄、ERROR 红
- 来源标签：[server] 蓝、[agent:xxx] 紫
- 点击展开完整 JSON

### 4.5 导出

筛选栏右侧下拉按钮，支持 CSV / JSON 格式，按当前筛选条件生成下载。

## 五、WebSocket 日志流

**统一方案**：复用现有 `/ws` 连接（不新建独立 WebSocket 端点），新增消息类型。

前端发送订阅控制：

```json
{"type":"log_subscribe","filter":{"level":"warn,error","source":"server"}}
{"type":"log_unsubscribe"}
```

Server 推送日志消息：

```json
{"type":"log","data":{"ts":"...","level":"error","module":"alerter","source":"server","msg":"..."}}
```

实现细节：
- Hub 维护每个连接的日志订阅状态（filter 条件）
- LogManager 广播时，Hub 按每个连接的 filter 过滤后推送
- 前端进入日志页面时发送 subscribe，离开时发送 unsubscribe
- 未订阅的连接不会收到 log 消息（不影响现有 metrics/alert 推送）

## 六、保留策略与清理

每天在 cleanup_hour 执行：
1. 扫描日志目录，删除超过保留天数的 .log 文件
2. 清理 logs.db 中对应的索引和审计记录
3. 记录清理日志

## 七、现有代码改造

| 改造项 | 影响范围 |
|--------|---------|
| 替换 log.Printf | server/ 约 60 处 → logMgr.Info/Warn/Error |
| Agent 日志缓冲 | agent/internal/reporter/ 新增 log buffer + batch_id + ReportLogs |
| Protobuf | proto/agent.proto 新增 LogEntry + ReportLogs（含 batch_id） |
| Gin 中间件 | router.go 插入 AuditMiddleware |
| Login handler | 设置 c.Set("audit_username", req.Username) 供中间件使用 |
| 配置结构体 | config.go 新增 Logging 段（含 buffer_size） |
| 前端路由 | App.tsx + Sidebar.tsx 新增 /logs |
| WebSocket | hub.go 新增 log 订阅状态 + 过滤推送 |

## 八、不做的事（YAGNI）

- 不做独立全文搜索引擎（索引预筛 + 文件回扫足够）
- 不做日志告警（已有告警系统）
- 不做日志分析/统计图表（后续 AI 功能）
- 不做多用户权限控制（当前单用户）
- 不做日志压缩归档（保留期内直接删除）
- 不做独立 WebSocket 端点（复用现有 /ws）
