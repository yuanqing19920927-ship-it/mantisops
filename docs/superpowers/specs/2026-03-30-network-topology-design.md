# 网络拓扑探知 — 设计文档

> 状态：待审阅
> 日期：2026-03-30

---

## 一、目标

在 MantisOps 中新增「网络拓扑」功能，实现：

1. **主动扫描内网设备** — 发现交换机、路由器、AP、防火墙等网络设备（不限于已接入的服务器）
2. **跨网段/跨 VLAN 扫描** — Server 集中扫描，通过三层路由到达多个子网
3. **SNMP 自动探测与采集** — 自动识别 SNMP 支持，深度采集设备信息和邻居关系
4. **拓扑可视化** — 力导向图展示设备关系，后续迭代分层视图
5. **网段自动分组** — 按子网自动分组，展示在线率
6. **连通性监控** — 定时 ping 检测，状态变化告警

## 二、安全约束（硬性）

**绝不可对局域网造成负担，禁止产生广播风暴或类似攻击行为。**

### 2.1 速率限制

| 扫描类型 | 并发上限 | 单包间隔 | 说明 |
|---------|---------|---------|------|
| ICMP ping sweep | 10 并发 | 每包间隔 10ms | 一个 /24 网段约 25 秒完成 |
| SNMP 探测 | 5 并发 | 每请求间隔 50ms | 仅对 ping 可达的 IP 发起 |
| SNMP 定时采集 | 3 并发 | 每请求间隔 100ms | 仅对已确认支持 SNMP 的设备 |
| 连通性 ping | 10 并发 | 每包间隔 20ms | 定时监控，间隔 ≥ 60 秒 |

### 2.2 禁止行为

- **禁止广播包** — 不使用 ARP 广播扫描，仅用单播 ICMP echo
- **禁止 SYN flood** — 不做全端口扫描，SNMP 仅探测 UDP 161
- **禁止并行扫描多网段** — 同一时间只扫描一个网段，排队执行
- **超时快速放弃** — ICMP 超时 1 秒，SNMP 超时 2 秒，不重试不可达目标
- **扫描时间窗口** — 默认仅在非工作时间执行定时扫描（可配置）

### 2.3 用户控制

- 扫描必须由用户手动触发或通过定时任务配置，不自动扫描
- 前端扫描按钮需二次确认（显示目标网段和预估耗时）
- 支持随时取消正在进行的扫描

## 三、架构设计

### 3.1 整体架构

```
浏览器 → /api/v1/network/*  → NetworkHandler
                                  ├── Scanner（ICMP ping sweep）
                                  ├── SNMPProber（UDP 161 探测 + 采集）
                                  ├── TopologyBuilder（聚合拓扑关系）
                                  └── ConnectivityMonitor（定时 ping）

数据存储：
  SQLite → network_devices, network_subnets, network_links
  WebSocket → scan_progress, device_status_change
```

### 3.2 模块划分

| 模块 | 文件路径 | 职责 |
|------|---------|------|
| Scanner | `server/internal/network/scanner.go` | ICMP ping sweep，发现活跃 IP |
| SNMPProber | `server/internal/network/snmp.go` | SNMP v2c 探测 + 设备信息采集 |
| TopologyBuilder | `server/internal/network/topology.go` | 从 SNMP 邻居数据构建拓扑图 |
| ConnectivityMonitor | `server/internal/network/monitor.go` | 定时 ping + 状态变更通知 |
| NetworkHandler | `server/internal/api/network_handler.go` | HTTP API 层 |
| NetworkStore | `server/internal/store/network_store.go` | SQLite 数据层 |
| 前端页面 | `web/src/pages/Network/` | 拓扑图 + 设备列表 + 网段概览 |

## 四、数据模型

### 4.1 network_subnets（网段）

```sql
CREATE TABLE network_subnets (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    cidr        TEXT NOT NULL UNIQUE,        -- 如 "192.168.10.0/24"
    name        TEXT DEFAULT '',             -- 自动生成或用户自定义
    gateway     TEXT DEFAULT '',             -- 网关 IP（扫描发现）
    total_hosts INTEGER DEFAULT 0,           -- 可用主机数
    alive_hosts INTEGER DEFAULT 0,           -- 存活主机数
    last_scan   DATETIME,                    -- 上次扫描时间
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### 4.2 network_devices（网络设备）

```sql
CREATE TABLE network_devices (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    ip              TEXT NOT NULL,
    mac             TEXT DEFAULT '',
    vendor          TEXT DEFAULT '',           -- OUI 厂商（从 MAC 推断）
    device_type     TEXT DEFAULT 'unknown',    -- switch/router/ap/firewall/printer/unknown
    hostname        TEXT DEFAULT '',
    snmp_supported  BOOLEAN DEFAULT FALSE,
    snmp_community  TEXT DEFAULT '',           -- 成功的 community string
    sys_descr       TEXT DEFAULT '',           -- SNMP sysDescr
    sys_name        TEXT DEFAULT '',           -- SNMP sysName
    sys_object_id   TEXT DEFAULT '',           -- SNMP sysObjectID
    model           TEXT DEFAULT '',           -- 设备型号（从 sysDescr 解析）
    subnet_id       INTEGER REFERENCES network_subnets(id),
    status          TEXT DEFAULT 'online',     -- online/offline
    last_seen       DATETIME,
    first_seen      DATETIME DEFAULT CURRENT_TIMESTAMP,
    -- 关联到已有服务器（如果是已接入的服务器）
    server_id       INTEGER DEFAULT 0,         -- 关联 servers 表
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_network_devices_ip ON network_devices(ip);
```

### 4.3 network_links（拓扑连接）

```sql
CREATE TABLE network_links (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id   INTEGER REFERENCES network_devices(id) ON DELETE CASCADE,
    target_id   INTEGER REFERENCES network_devices(id) ON DELETE CASCADE,
    source_port TEXT DEFAULT '',        -- 源端口名（如 "GigabitEthernet0/1"）
    target_port TEXT DEFAULT '',        -- 目标端口名
    protocol    TEXT DEFAULT 'lldp',    -- lldp/cdp/arp
    bandwidth   TEXT DEFAULT '',        -- 链路带宽
    last_seen   DATETIME,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source_id, target_id, source_port)
);
```

## 五、扫描流程

### 5.1 手动扫描流程

```
用户输入目标网段列表（如 "192.168.10.0/24, 192.168.20.0/24"）
  → 前端二次确认（显示网段数、预估 IP 数、预估耗时）
  → POST /api/v1/network/scan
  → Server 创建扫描任务，网段排队逐个执行

对每个网段：
  1. ICMP ping sweep（10 并发，10ms 间隔）
     → WebSocket 推送进度（已扫描/总数/已发现）
  2. 对存活 IP：OUI 厂商查询（内嵌 MAC-vendor 数据库，离线查询）
  3. 对存活 IP：SNMP v2c 探测（5 并发，尝试 community: public, private）
     → 成功则标记 snmp_supported=true，读取 sysDescr/sysName
  4. 对 SNMP 设备：读取 LLDP/CDP 邻居表 → 构建 network_links
  5. 结果写入 SQLite，自动创建/更新 network_subnets 和 network_devices
  6. WebSocket 推送 scan_complete
```

### 5.2 定时连通性监控

```
ConnectivityMonitor 每 60 秒对所有已发现设备执行 ping：
  - 10 并发，20ms 间隔
  - 超时 1 秒
  - 状态变化（online → offline 或 offline → online）：
    → 更新 DB
    → WebSocket 推送 device_status_change
    → 触发告警（可选，复用现有告警引擎）
```

### 5.3 SNMP 定时采集

```
每 5 分钟对所有 snmp_supported=true 的设备：
  - 3 并发，100ms 间隔
  - 读取端口流量、邻居表
  - 更新 network_links
  - 写入 VictoriaMetrics（可选，用于历史流量趋势）
```

## 六、API 设计

### 6.1 扫描管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| POST | `/api/v1/network/scan` | 触发扫描 `{ "subnets": ["192.168.10.0/24"] }` | admin |
| GET | `/api/v1/network/scan/status` | 当前扫描状态 | admin |
| DELETE | `/api/v1/network/scan` | 取消扫描 | admin |

### 6.2 设备管理（只读 + 类型修正）

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/v1/network/devices` | 设备列表（支持 subnet_id 筛选） | all |
| GET | `/api/v1/network/devices/:id` | 设备详情（含 SNMP 信息） | all |
| PUT | `/api/v1/network/devices/:id` | 修正设备类型/名称 | admin |
| DELETE | `/api/v1/network/devices/:id` | 删除设备记录 | admin |

### 6.3 拓扑与网段

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET | `/api/v1/network/topology` | 拓扑图数据（nodes + edges） | all |
| GET | `/api/v1/network/subnets` | 网段列表（含在线率） | all |

### 6.4 SNMP 配置

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| PUT | `/api/v1/network/snmp-config` | 配置全局 SNMP community 列表 | admin |
| GET | `/api/v1/network/snmp-config` | 获取当前 SNMP 配置 | admin |

### 6.5 WebSocket 事件

| 事件类型 | 数据 | 说明 |
|---------|------|------|
| `network_scan_progress` | `{ subnet, total, scanned, found }` | 扫描进度 |
| `network_scan_complete` | `{ subnet, devices_found, snmp_count }` | 扫描完成 |
| `network_device_status` | `{ device_id, ip, status, prev_status }` | 设备状态变化 |

## 七、前端设计

### 7.1 页面结构

新增侧边栏菜单项「网络拓扑」（图标：`lan`），路由 `/network`。

页面包含三个 Tab：

**Tab 1 — 拓扑图**
- D3.js 力导向图
- 节点图标按设备类型区分（交换机/路由器/AP/服务器/未知）
- 已接入 MantisOps 的服务器用特殊标识（如绿色边框）
- 连线来自 SNMP LLDP/CDP 邻居发现
- 节点颜色表示在线/离线状态
- 悬停显示设备信息卡片（IP、型号、端口数）
- 支持拖拽、缩放

**Tab 2 — 设备列表**
- 表格：状态、IP、MAC、厂商、类型、型号、SNMP、网段、最后在线
- 筛选：按网段、按类型、按状态
- 搜索：IP/MAC/厂商/型号
- 设备类型可手动修正（下拉选择）

**Tab 3 — 网段概览**
- 卡片式展示每个网段：CIDR、网关、设备数/在线数、在线率进度条
- 点击网段跳转到该网段的设备列表筛选视图

**顶部操作区**
- 「扫描网段」按钮 → 弹窗输入目标 CIDR（支持多个，逗号分隔）
- 扫描中显示进度条 + 取消按钮

### 7.2 侧边栏位置

插入到「告警中心」之后、「日志中心」之前：

```
...
告警中心
网络拓扑    ← 新增
日志中心
...
```

## 八、Go 依赖

| 库 | 用途 | 许可 |
|----|------|------|
| `pro-bing` (github.com/prometheus-community/pro-bing) | ICMP ping（无需 root，用 UDP ping） | MIT |
| `gosnmp` (github.com/gosnmp/gosnmp) | SNMP v2c/v3 客户端 | BSD |
| 内嵌 OUI 数据 | MAC → 厂商查询（编译时嵌入 JSON） | 公共数据 |

**关于权限**：使用 `pro-bing` 的 UDP ping 模式，无需 `CAP_NET_RAW`，普通用户即可执行。如果 UDP ping 不可用，降级为 TCP connect 探测。

## 九、配置（server.yaml）

```yaml
network:
  enabled: false                    # 默认关闭，需手动开启
  monitor_interval: 60              # 连通性监控间隔（秒）
  snmp_interval: 300                # SNMP 采集间隔（秒）
  snmp_communities:                 # SNMP community 列表
    - "public"
    - "private"
  scan:
    icmp_concurrency: 10            # ICMP 并发数
    icmp_interval_ms: 10            # ICMP 包间隔（毫秒）
    snmp_concurrency: 5             # SNMP 探测并发数
    snmp_timeout_ms: 2000           # SNMP 超时（毫秒）
    icmp_timeout_ms: 1000           # ICMP 超时（毫秒）
```

## 十、与现有系统集成

| 集成点 | 方式 |
|--------|------|
| **已有服务器** | 扫描发现的 IP 匹配 servers 表的 ip_addresses，自动关联 server_id |
| **告警引擎** | 新增告警类型 `network_device_offline`，复用现有 AlertEngine |
| **WebSocket** | 复用现有 Hub，新增 network_* 事件类型 |
| **权限** | 扫描操作 admin only，查看 all（复用 RequireRole） |

## 十一、不做的事情（YAGNI）

- 不做 SNMP v3 认证（v2c 覆盖绝大多数设备，后续按需加）
- 不做 SNMP trap 接收（被动监听复杂度高，第一版用主动轮询）
- 不做端口级流量图表（第一版只做设备级拓扑）
- 不做自动拓扑分层（第一版力导向图，后续迭代）
- 不做跨网段路由发现（依赖三层路由已通的前提）
