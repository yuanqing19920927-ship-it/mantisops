# MantisOps NAS 监控模块设计文档

> 日期：2026-03-29
> 状态：已确认（rev.2 — 修复审查问题）

## 一、架构总览

```
┌──────────────────┐   SSH (定时采集)    ┌──────────────────────────────────┐
│  Synology NAS    │ ←───────────────── │         MantisOps Server         │
│  (DSM 7.x)       │                    │                                  │
└──────────────────┘                    │  ┌─ NasCollector ────────────┐   │
                                        │  │ • 每设备独立 goroutine     │   │
┌──────────────────┐   SSH (定时采集)    │  │ • SSH 连接 → 批量命令     │   │
│  飞牛 NAS        │ ←───────────────── │  │ • 解析指标写入 VM          │   │
│  (fnOS)          │                    │  │ • WebSocket 实时广播       │   │
└──────────────────┘                    │  │ • 缓存最新快照供 API 返回  │   │
                                        │  └──────────────────────────┘   │
                                        │         ↓ WebSocket              │
                                        └──────────────────────────────────┘
                                                    ↓
                                        ┌──────────────────────────────────┐
                                        │        前端「NAS 存储」页面       │
                                        │  ┌──────────┬─────────────────┐  │
                                        │  │ NAS 列表  │   NAS 详情      │  │
                                        │  │ (卡片视图) │ (RAID/磁盘/卷)  │  │
                                        │  └──────────┴─────────────────┘  │
                                        └──────────────────────────────────┘
```

### 1.1 核心设计决策

| 决策 | 选择 | 理由 |
|------|------|------|
| 采集方式 | SSH 命令 | NAS 用户不愿装额外软件；群晖/飞牛均基于 Linux，标准命令通用 |
| 数据模型 | 独立 nas_devices 表 | NAS 与服务器核心指标差异大（RAID/S.M.A.R.T./UPS），解耦更清晰 |
| 凭据管理 | 复用现有 credentials 系统 | 加密存储已完备，SSH 密钥可跨设备共用 |
| 接入管理 | 放在设置页 | 与托管服务器、云账号统一入口，保持一致性 |
| 指标存储 | VictoriaMetrics（mantisops_nas_* 前缀） | 与服务器指标隔离，复用 VM 查询能力 |

## 二、数据模型

### 2.1 nas_devices 表（SQLite）

```sql
CREATE TABLE nas_devices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,                    -- 用户自定义名称，如"客厅群晖"
    nas_type TEXT NOT NULL,                -- synology / fnos
    host TEXT NOT NULL,                    -- IP 或域名
    port INTEGER NOT NULL DEFAULT 22,      -- SSH 端口
    credential_id INTEGER NOT NULL REFERENCES credentials(id),
    collect_interval INTEGER DEFAULT 60,   -- 采集间隔（秒），最小 30 秒，API 层校验
    status TEXT DEFAULT 'unknown',         -- online / offline / degraded / unknown
    last_seen INTEGER,                     -- 最后采集成功的 Unix 时间戳
    system_info TEXT DEFAULT '{}',         -- JSON：见下方 system_info schema
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_nas_devices_host_port ON nas_devices(host, port);
```

**建表方式**：使用版本迁移（在现有 `migrate()` 中新增迁移步骤），而非直接 `CREATE TABLE IF NOT EXISTS`，因为项目已有生产部署。

**system_info JSON schema：**

```json
{
    "model": "DS920+",              // 硬件型号
    "serial": "20G0xxx",            // 序列号
    "os_version": "DSM 7.2-64570", // 系统版本（群晖为 DSM 版本，飞牛为 fnOS 版本）
    "kernel": "4.4.302+",          // 内核版本
    "arch": "x86_64",              // 架构
    "total_memory": 8589934592     // 总内存（字节）
}
```

**status 状态判定逻辑：**

| 状态 | 触发条件 |
|------|---------|
| `online` | 采集成功且所有 RAID 正常 |
| `offline` | SSH 连接失败或采集超时 |
| `degraded` | 采集成功但存在 RAID 降级或硬盘 S.M.A.R.T. 异常 |
| `unknown` | 初始状态，尚未完成首次采集 |

### 2.2 VictoriaMetrics 指标命名

NAS 专属指标统一前缀 `mantisops_nas_`，与服务器指标（`mantisops_`）隔离：

```
# 通用系统指标
mantisops_nas_cpu_usage_percent{nas_id="1",name="客厅群晖"}
mantisops_nas_memory_usage_percent{nas_id="1"}
mantisops_nas_network_rx_bytes_per_sec{nas_id="1",interface="eth0"}
mantisops_nas_network_tx_bytes_per_sec{nas_id="1",interface="eth0"}

# RAID / 存储池
mantisops_nas_raid_status{nas_id="1",array="md2",raid_type="raid1"}  -- 0=正常 1=降级 2=重建
mantisops_nas_raid_rebuild_percent{nas_id="1",array="md2"}

# 硬盘
mantisops_nas_disk_temperature_celsius{nas_id="1",disk="sda"}
mantisops_nas_disk_power_on_hours{nas_id="1",disk="sda"}
mantisops_nas_disk_reallocated_sectors{nas_id="1",disk="sda"}
mantisops_nas_disk_smart_healthy{nas_id="1",disk="sda"}  -- 1=健康 0=异常

# 卷
mantisops_nas_volume_total_bytes{nas_id="1",volume="/volume1"}
mantisops_nas_volume_used_bytes{nas_id="1",volume="/volume1"}
mantisops_nas_volume_usage_percent{nas_id="1",volume="/volume1"}

# UPS
mantisops_nas_ups_status{nas_id="1"}  -- 0=正常供电 1=电池供电 2=低电量
mantisops_nas_ups_battery_percent{nas_id="1"}
```

## 三、后端设计

### 3.1 新增 Go 包与文件

```
server/internal/collector/nas.go           # NasCollector：生命周期管理 + 采集调度
server/internal/collector/nas_ssh.go       # SSH 命令执行 + 指标解析
server/internal/collector/nas_synology.go  # 群晖专属命令与解析
server/internal/collector/nas_fnos.go      # 飞牛专属命令与解析
server/internal/store/nas_store.go         # nas_devices CRUD
server/internal/api/nas_handler.go         # HTTP API handler
```

### 3.2 NasCollector 架构

```go
type NasCollector struct {
    store      *store.NasStore
    credStore  *store.CredentialStore
    crypto     *crypto.AES
    vm         *VictoriaMetrics
    hub        *ws.Hub
    cache      map[int64]*NasMetricsSnapshot  // 最新指标缓存
    workers    map[int64]context.CancelFunc    // 每设备的采集 goroutine
    mu         sync.RWMutex
}

// 启动：加载所有 NAS 设备，为每个启动采集 goroutine
func (c *NasCollector) Start(ctx context.Context)

// 动态管理：设备增删改时调用
func (c *NasCollector) AddDevice(device *NasDevice)
func (c *NasCollector) RemoveDevice(deviceID int64)
func (c *NasCollector) UpdateDevice(device *NasDevice)

// 获取缓存的最新指标
func (c *NasCollector) GetMetrics(deviceID int64) *NasMetricsSnapshot
```

### 3.3 SSH 采集命令批次

一次 SSH 连接执行多条命令（减少连接开销），按 `nas_type` 区分。

**通用命令（synology + fnos 共用）：**

| 指标 | 命令 | 解析方式 |
|------|------|---------|
| CPU/内存 | `cat /proc/stat && cat /proc/meminfo` | 与 Agent 相同算法 |
| 网络 | `cat /proc/net/dev` | delta 计算 |
| RAID 状态 | `cat /proc/mdstat` | 正则解析 md 设备、类型、状态、重建进度 |
| 硬盘列表 | `lsblk -Jb -o NAME,SIZE,TYPE,MOUNTPOINT` | JSON 输出 |
| S.M.A.R.T. | `smartctl -A -H /dev/sdX` | 逐盘执行，解析温度/健康/关键属性 |
| 卷用量 | `df -B1` | 解析各挂载点 |
| UPS | `upsc ups@localhost 2>/dev/null` | 解析 NUT 输出（如有） |

**群晖专属补充：**

| 指标 | 命令 | 说明 |
|------|------|------|
| DSM 版本/型号 | `cat /etc/synoinfo.conf && cat /etc.defaults/VERSION` | 静态信息，仅首次采集 |
| 存储池详情 | `synospace --lib-get-volumes` | 群晖特有工具 |
| 套件状态 | `synopkg list --format json` | 已安装套件列表 |

**飞牛专属补充：**

| 指标 | 命令 | 说明 |
|------|------|------|
| 系统信息 | `cat /etc/os-release` | 版本识别 |
| Btrfs 卷状态 | `btrfs filesystem show && btrfs device stats /` | Btrfs 健康检查 |

### 3.4 SSH 连接管理

- **连接策略**：每次采集新建 SSH 连接，采集完成后关闭。60 秒间隔下长连接收益不大，且增加状态管理复杂度
- **连接超时**：拨号超时 10 秒，命令执行超时 30 秒
- **权限要求**：建议以 root 用户或配置了 sudo NOPASSWD 的用户连接。`smartctl`、`btrfs` 等命令需要 root 权限。添加 NAS 时的"测试连接"应验证 `sudo smartctl --version` 是否可执行，不满足时提示用户
- **错误区分**：区分"网络不可达"（dial timeout）和"认证失败"（auth failed），后者在设置页设备列表中额外显示"凭据失效"提示

### 3.5 采集容错

- SSH 连接失败：标记 `status=offline`，跳过本轮，下轮重试
- 认证失败：标记 `status=offline`，在 system_info 中记录 `"error": "auth_failed"`，前端展示凭据异常提示
- 单条命令失败（如 `smartctl` 无权限）：跳过该指标，其余照常采集，在日志中记录跳过原因
- 连续 3 轮全部失败：降低采集频率为 5 分钟，避免无意义重试

### 3.6 指标写入流程

```
SSH 采集结果
  → 解析为 NasMetricsSnapshot 结构体
  → 转换为 Prometheus line format
  → 写入 VictoriaMetrics
  → 更新内存缓存 cache[nasID]
  → WebSocket 广播 {type:"nas_metrics", nas_id:1, data:{...}}
  → 更新 nas_devices.last_seen + status
```

## 四、API 端点

### 4.1 NAS 设备管理（设置页使用）

```
GET    /api/v1/nas-devices              # 列表（含最新状态）
POST   /api/v1/nas-devices              # 添加 NAS 设备
PUT    /api/v1/nas-devices/:id          # 编辑（名称/地址/凭据/采集间隔）
DELETE /api/v1/nas-devices/:id          # 删除
POST   /api/v1/nas-devices/:id/test     # SSH 连通性测试（不落库）
```

### 4.2 NAS 监控数据（NAS 页面使用）

```
GET    /api/v1/nas-devices/:id/metrics  # 最新一次采集的完整指标快照（含 RAID、硬盘、卷、UPS 全部数据）
```

注：第一版只提供 `/metrics` 一个端点返回完整快照。S.M.A.R.T. 和 RAID 详情都包含在 metrics 响应中。如果后续数据量增大或需要独立刷新，再拆分独立端点。

历史趋势图表直接走 VictoriaMetrics 查询（`/vm/api/v1/query_range`），前端拼 PromQL。

### 4.3 请求/响应示例

**POST /api/v1/nas-devices**

```json
{
    "name": "客厅群晖",
    "nas_type": "synology",
    "host": "192.168.1.100",
    "port": 22,
    "credential_id": 3,
    "collect_interval": 60
}
```

**GET /api/v1/nas-devices/:id/metrics 响应**

```json
{
    "nas_id": 1,
    "timestamp": 1711699200,
    "cpu": {"usage_percent": 35.2},
    "memory": {"total": 8589934592, "used": 4980736000, "usage_percent": 58.0},
    "networks": [
        {"interface": "eth0", "rx_bytes_per_sec": 89128960, "tx_bytes_per_sec": 12582912}
    ],
    "raids": [
        {"array": "md2", "raid_type": "raid1", "status": "active", "disks": ["sda", "sdb"], "rebuild_percent": 0}
    ],
    "volumes": [
        {"mount": "/volume1", "fs_type": "ext4", "total": 3960000000000, "used": 2808000000000, "usage_percent": 78.0}
    ],
    "disks": [
        {"name": "sda", "model": "WD Red Plus 4TB", "size": 3960000000000, "temperature": 35, "power_on_hours": 12345, "smart_healthy": true, "reallocated_sectors": 0}
    ],
    "ups": {"status": "online", "battery_percent": 100, "model": "APC BK650"}
}
```

## 五、前端设计

### 5.1 侧边栏菜单

在「服务器」和「数据库」之间新增：
- **NAS 存储**（图标：`hard_drive`，Material Symbols。注：`dns` 已被「服务器」使用）
- 路由：`/nas`

### 5.2 设置页 NAS 管理区块

在现有设置页中，与"托管服务器"、"云账号"并列新增 **NAS 设备** 管理区块：

```
NAS 设备                                           [+ 添加 NAS]
┌────────────────────────────────────────────────────────────┐
│ 🟢 客厅群晖    Synology  192.168.1.100   60s   [编辑] [删除] │
│ 🟢 书房飞牛    fnOS      192.168.1.101   60s   [编辑] [删除] │
└────────────────────────────────────────────────────────────┘
```

**添加 NAS 对话框字段：**
- 名称（必填）
- NAS 类型：下拉选择 Synology / fnOS
- 地址（IP/域名，必填）
- SSH 端口（默认 22）
- SSH 凭据（下拉选择已有凭据，复用 credentials 系统）
- 采集间隔（默认 60 秒）
- 「测试连接」按钮：SSH 连通性验证

### 5.3 NAS 列表页（`/nas`）

```
┌─────────────────────────────────────────────────────────┐
│  NAS 存储                                                │
│                                                         │
│  ┌─ 统计卡片 ──────────────────────────────────────────┐ │
│  │ 设备总数: 2  │  在线: 2  │  RAID降级: 0  │  磁盘异常: 0 │ │
│  └─────────────────────────────────────────────────────┘ │
│                                                         │
│  ┌─ NAS 卡片 ─────────────────────────────────────────┐  │
│  │  客厅群晖                    Synology  🟢 在线      │  │
│  │  192.168.1.100               DSM 7.2-64570          │  │
│  │                                                     │  │
│  │  CPU  ████░░░░░░  35%    内存  ██████░░░░  58%      │  │
│  │                                                     │  │
│  │  ┌─ 存储池 ──────────────────────────────────────┐  │  │
│  │  │  RAID 1 (md2)  正常    卷1  2.8T / 3.6T  78%  │  │  │
│  │  └───────────────────────────────────────────────┘  │  │
│  │                                                     │  │
│  │  ┌─ 硬盘 ────────────────────────────────────────┐  │  │
│  │  │  sda 🟢 35°C  │  sdb 🟢 36°C                  │  │  │
│  │  └───────────────────────────────────────────────┘  │  │
│  │                                                     │  │
│  │  网络 ↓ 85 MB/s  ↑ 12 MB/s      UPS 🔋 100%       │  │
│  └─────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### 5.4 NAS 详情页（`/nas/:id`）

```
┌─────────────────────────────────────────────────────────┐
│  ← 返回    客厅群晖    Synology DSM 7.2    🟢 在线      │
│                                                         │
│  ┌─ 设备信息 ──────────────────────────────────────────┐ │
│  │  型号: DS920+  │  序列号: 20G0xxx  │  IP: 192.168.1.100  │
│  └─────────────────────────────────────────────────────┘ │
│                                                         │
│  ┌─ 实时概览 ──────────────────────────────────────────┐ │
│  │  CPU 35%  │  内存 58%  │  网络 ↓85/↑12 MB/s        │ │
│  └─────────────────────────────────────────────────────┘ │
│                                                         │
│  ┌─ RAID 阵列 ─────────────────────────────────────────┐ │
│  │  md2 (RAID 1)    状态: 正常    [sda] [sdb]          │ │
│  │  md3 (RAID 5)    状态: 正常    [sda] [sdb] [sdc]    │ │
│  └─────────────────────────────────────────────────────┘ │
│                                                         │
│  ┌─ 存储卷 ────────────────────────────────────────────┐ │
│  │  /volume1  ████████░░  78%    2.8T / 3.6T   ext4   │ │
│  │  /volume2  ███░░░░░░░  25%    1.2T / 4.8T   btrfs  │ │
│  └─────────────────────────────────────────────────────┘ │
│                                                         │
│  ┌─ 硬盘健康 ──────────────────────────────────────────┐ │
│  │  盘位  型号              容量   温度  通电    健康   │ │
│  │  sda   WD Red Plus 4TB  3.6T   35°C  12345h  🟢    │ │
│  │  sdb   WD Red Plus 4TB  3.6T   36°C  12340h  🟢    │ │
│  │  sdc   Seagate IronW..  4.0T   38°C   8900h  🟢    │ │
│  │        ▸ 展开 S.M.A.R.T. 详情                       │ │
│  └─────────────────────────────────────────────────────┘ │
│                                                         │
│  ┌─ UPS 电源 ──────────────────────────────────────────┐ │
│  │  状态: 正常供电  │  电池: 100%  │  型号: APC BK650   │ │
│  └─────────────────────────────────────────────────────┘ │
│                                                         │
│  ┌─ 历史趋势  [1h] [6h] [24h] [7d] ───────────────────┐ │
│  │  CPU / 内存 / 网络 / 磁盘温度 图表                   │ │
│  └─────────────────────────────────────────────────────┘ │
│                                                         │
│  ┌─ 群晖套件（仅 Synology）─────────────────────────────┐ │
│  │  Docker ▶ 运行中  │  Hyper Backup ▶ 运行中           │ │
│  └─────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

### 5.5 WebSocket 实时更新

复用现有 `/ws` 连接，新增消息类型：

```json
{"type": "nas_metrics", "nas_id": 1, "data": { ... 完整指标快照 ... }}
{"type": "nas_status", "nas_id": 1, "status": "degraded"}
```

**广播策略**：使用 `BroadcastJSON()` 全量推送给所有连接（与服务器指标一致）。NAS 设备数量少（通常 1-5 台）且采集间隔 60 秒，全量推送的开销可忽略，不需要像日志那样增加订阅机制。

前端 `useWebSocket` hook 新增处理，更新 NAS Zustand store。

### 5.6 新增前端文件

```
web/src/pages/NAS/index.tsx             # NAS 列表页
web/src/pages/NAS/NASDetail.tsx         # NAS 详情页
web/src/api/nas.ts                      # NAS API 客户端
web/src/stores/nasStore.ts              # NAS Zustand store
```

### 5.7 修改前端文件

```
web/src/App.tsx                          # 新增 /nas 和 /nas/:id 路由
web/src/components/Layout/Sidebar.tsx    # 新增 NAS 存储菜单项
web/src/hooks/useWebSocket.ts            # 新增 nas_metrics/nas_status 处理
web/src/pages/Settings/index.tsx         # 新增 NAS 设备管理区块
```

## 六、告警集成

复用现有告警引擎，为 NAS 新增告警类型：

| 告警类型 | type 值 | 触发条件 | 默认级别 |
|---------|---------|---------|---------|
| NAS 离线 | `nas_offline` | 连续 3 轮采集失败 | critical |
| RAID 降级 | `nas_raid_degraded` | RAID 状态 = degraded | critical |
| 硬盘 S.M.A.R.T. 异常 | `nas_disk_smart` | smart_healthy = 0 | critical |
| 硬盘温度过高 | `nas_disk_temperature` | 温度 > 阈值（默认 55°C） | warning |
| 存储卷空间不足 | `nas_volume_usage` | 用量 > 阈值（默认 90%） | warning |
| UPS 电池供电 | `nas_ups_battery` | UPS 状态 = 电池供电 | warning |

### 6.1 实现路径

现有 `AlertEngine.evaluate()` 与服务器模型深度耦合（遍历 `[]model.Server`，调用 `MetricsProvider` 获取 `*pb.MetricsPayload`）。NAS 需要独立的评估路径：

1. **新增 `NasMetricsProvider` 接口**：由 `NasCollector` 实现，返回 `map[int64]*NasMetricsSnapshot`（所有 NAS 设备的最新指标）
2. **新增 `evaluateNas()` 方法**：在 `AlertEngine` 的评估循环中，`evaluate()` 之后调用 `evaluateNas()`，遍历所有 NAS 设备，对每台设备评估所有 `nas_*` 类型的规则
3. **alert_rules 表**：`type` 字段使用 `nas_` 前缀区分（如 `nas_raid_degraded`），`target_id` 格式为 `nas:{id}`（如 `nas:1`）
4. **cleanupGoneTargets**：新增 NAS 设备清理逻辑，删除 NAS 设备时清理关联的 firing 事件
5. **告警规则 UI**：目标类型下拉新增 `nas` 选项，选中后 target 列表展示 NAS 设备而非服务器

## 七、现有代码改造

| 改造项 | 影响范围 |
|--------|---------|
| SQLite 建表 | store/sqlite.go 版本迁移新增 nas_devices 表 |
| 凭据引用计数 | credential_store.go 的 `List()` 和 `Delete()` SQL 需加入 `nas_devices` 表的引用计数，否则被 NAS 引用的凭据会显示 used_by=0 且可被删除 |
| 路由注册 | router.go 新增 /nas-devices/* 路由 |
| main.go 初始化 | 创建 NasCollector，注入依赖 |
| 告警引擎 | alert/engine.go 新增 evaluateNas() + NasMetricsProvider 接口 + cleanupGoneTargets NAS 分支 |
| 前端路由 | App.tsx 新增 /nas 和 /nas/:id |
| 侧边栏 | Sidebar.tsx 新增菜单项 |
| WebSocket | hub.go 新增 nas_metrics/nas_status 广播，useWebSocket.ts 新增处理 |
| 设置页 | Settings/index.tsx 新增 NAS 设备管理区块 |
| 审计中间件 | logging/middleware.go 新增 NAS 相关路由的审计映射 |

## 八、不做的事（YAGNI）

- 不做文件浏览器 / 文件管理（NAS 系统自身功能）
- 不做共享文件夹权限管理（复杂且各厂商差异大）
- 不做 NAS 套件/应用的安装/卸载操作（只做状态展示）
- 不做远程关机/重启（风险高，直接去 NAS 管理界面操作）
- 不做 RAID 创建/修改（极度危险的操作）
- 不做 NAS 之间的数据同步/备份管理
- 不做仪表盘集成（第一版 NAS 独立页面，后续按需加到 Dashboard）
- 不做 SNMP 采集（SSH 更通用，且已有 SSH 基础设施）
