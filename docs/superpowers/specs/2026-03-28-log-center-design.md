# MantisOps 日志中心设计文档

> 日期：2026-03-28
> 状态：已确认

## 一、架构总览

```
┌─────────────┐    gRPC(ReportLogs)     ┌──────────────────────────────────┐
│   Agent ×N  │ ──── 30s批量上报 ────→  │         MantisOps Server         │
│ (本地缓冲)   │                        │                                  │
└─────────────┘                        │  ┌─ LogManager ──────────────┐   │
                                       │  │ • 接收各来源日志           │   │
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

## 二、存储设计

### 2.1 日志文件（主存储）

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
| msg | string | 日志消息 |
| host_id | string | Agent 来源标识，系统日志为空 |
| trace_id | string | 可选，用于关联请求链路 |

### 2.2 SQLite 索引（独立 logs.db）

**表 audit_logs — 操作审计全文存储**

```sql
CREATE TABLE audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL,
    username TEXT NOT NULL,
    action TEXT NOT NULL,            -- login/logout/create/update/delete
    resource_type TEXT NOT NULL,     -- alert_rule/cloud_account/managed_server/probe/asset/channel/credential/server
    resource_id TEXT,
    resource_name TEXT,
    detail TEXT,                     -- 变更详情 JSON
    ip_address TEXT,
    user_agent TEXT
);
CREATE INDEX idx_audit_timestamp ON audit_logs(timestamp DESC);
CREATE INDEX idx_audit_username ON audit_logs(username, timestamp DESC);
CREATE INDEX idx_audit_resource ON audit_logs(resource_type, timestamp DESC);
```

**表 log_index — 运行日志索引（不存全文）**

```sql
CREATE TABLE log_index (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL,
    level TEXT NOT NULL,
    module TEXT NOT NULL,
    source TEXT NOT NULL,            -- server / agent:{host_id}
    file_path TEXT NOT NULL,
    file_offset INTEGER NOT NULL,
    line_length INTEGER NOT NULL,
    message_preview TEXT             -- 消息前 200 字符
);
CREATE INDEX idx_log_timestamp ON log_index(timestamp DESC);
CREATE INDEX idx_log_level ON log_index(level, timestamp DESC);
CREATE INDEX idx_log_module ON log_index(module, timestamp DESC);
CREATE INDEX idx_log_source ON log_index(source, timestamp DESC);
```

### 2.3 配置（server.yaml 新增段）

```yaml
logging:
  dir: "./logs"
  level: "info"                     # 最低记录级别
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
├── manager.go          # LogManager：统一日志入口
├── writer.go           # 按天轮转文件写入器
├── store.go            # logs.db SQLite 操作
├── cleanup.go          # 定时清理
└── middleware.go        # Gin 审计中间件
```

### 3.2 LogManager 接口

```go
type LogManager struct {
    writer   *RotatingWriter
    store    *LogStore
    hub      *ws.Hub
    level    Level
}

func (m *LogManager) Info(module, msg string, args ...any)
func (m *LogManager) Warn(module, msg string, args ...any)
func (m *LogManager) Error(module, msg string, args ...any)
func (m *LogManager) Debug(module, msg string, args ...any)
func (m *LogManager) Audit(username, action, resourceType, resourceID, resourceName, detail, ip, ua string)
func (m *LogManager) IngestAgentLogs(hostID string, entries []*pb.LogEntry)
```

所有方法同时写入文件 + 索引 + stdout + WebSocket 广播。

### 3.3 Gin 审计中间件

拦截 POST/PUT/DELETE 成功请求，自动记录操作审计。

需要审计的操作：

| resource_type | 触发路径 |
|--------------|---------|
| auth | POST /auth/login |
| alert_rule | POST/PUT/DELETE /alerts/rules/* |
| channel | POST/PUT/DELETE /alerts/channels/* |
| alert_event | PUT /alerts/events/:id/ack |
| probe | POST/PUT/DELETE /probes/* |
| asset | POST/PUT/DELETE /assets/* |
| cloud_account | POST/PUT/DELETE /cloud-accounts/* |
| managed_server | POST /managed-servers/* |
| credential | POST/PUT/DELETE /credentials/* |
| server | PUT /servers/* |

### 3.4 结构化日志替换

将现有约 60 处 `log.Printf("[module] ...")` 替换为 `logMgr.Info("module", ...)`，同时保留 stdout 输出。

### 3.5 gRPC Agent 日志上报

Protobuf 新增：

```protobuf
message LogEntry {
    int64  timestamp = 1;
    string level = 2;
    string module = 3;
    string message = 4;
}

message ReportLogsRequest {
    string host_id = 1;
    repeated LogEntry entries = 2;
}

message ReportLogsResponse {
    bool ok = 1;
}

service AgentService {
    rpc ReportLogs(ReportLogsRequest) returns (ReportLogsResponse);
}
```

Agent 端：本地文件缓冲 → 每 30s 批量上报 → 成功后截断缓冲。网络断开时持续缓冲，恢复后补传。

### 3.6 API 端点

```
GET  /api/v1/logs/audit              # 操作审计列表
GET  /api/v1/logs/runtime            # 运行日志列表
GET  /api/v1/logs/runtime/stream     # WebSocket 实时日志流
GET  /api/v1/logs/export             # 导出日志
GET  /api/v1/logs/sources            # 日志来源列表
GET  /api/v1/logs/stats              # 日志统计
```

查询参数：

| 参数 | 类型 | 说明 |
|------|------|------|
| start | datetime | 起始时间 |
| end | datetime | 结束时间 |
| level | string | 级别筛选（逗号分隔多选） |
| module | string | 模块筛选 |
| source | string | 来源筛选 |
| keyword | string | 关键字搜索 |
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

复用现有 /ws 连接，新增消息类型：

```json
{"type":"log","data":{"ts":"...","level":"error","module":"alerter","source":"server","msg":"..."}}
```

前端通过发送订阅/取消控制：

```json
{"type":"log_subscribe","filter":{"level":"warn,error","source":"server"}}
{"type":"log_unsubscribe"}
```

## 六、保留策略与清理

每天在 cleanup_hour 执行：
1. 扫描日志目录，删除超过保留天数的 .log 文件
2. 清理 logs.db 中对应的索引和审计记录
3. 记录清理日志

## 七、现有代码改造

| 改造项 | 影响范围 |
|--------|---------|
| 替换 log.Printf | server/ 约 60 处 → logMgr.Info/Warn/Error |
| Agent 日志缓冲 | agent/internal/reporter/ 新增 |
| Protobuf | proto/agent.proto 新增 LogEntry + ReportLogs |
| Gin 中间件 | router.go 插入 AuditMiddleware |
| 配置结构体 | config.go 新增 Logging 段 |
| 前端路由 | App.tsx + Sidebar.tsx 新增 /logs |
| WebSocket | hub.go 新增 log 订阅/推送 |

## 八、不做的事（YAGNI）

- 不做全文搜索引擎（SQLite LIKE 足够）
- 不做日志告警（已有告警系统）
- 不做日志分析/统计图表（后续 AI 功能）
- 不做多用户权限控制（当前单用户）
- 不做日志压缩归档（保留期内直接删除）
