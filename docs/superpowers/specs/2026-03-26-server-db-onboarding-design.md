# 服务器与数据库接入管理

> 设计文档 — 2026-03-26

---

## 1. 概述

### 1.1 目标

将服务器和云资源的接入从「手动改配置文件 + 硬编码」升级为「UI 驱动的全流程管理」：

1. **本地服务器接入** — 在 UI 填写服务器 SSH 信息，一键远程安装 Agent，全程状态可视
2. **阿里云账号接入** — 在 UI 填写 RAM 子账号 AK/SK，自动发现 ECS + RDS 实例并纳入监控

### 1.2 入口

- **服务器列表页**（`/servers`）：新增「添加服务器」按钮 → 弹出本地服务器接入表单
- **数据库列表页**（`/databases`）：新增「添加云账号」按钮 → 弹出阿里云账号管理面板

### 1.3 设计原则

- 异步任务 + 状态机 — 安装/发现过程拆分为多步，WebSocket 实时推送进度
- 凭据加密存储 — SSH 密码/密钥和阿里云 AK 使用 AES-256-GCM 加密后入库
- 旧配置兼容 — 首次启动自动将 `server.yaml` 中已有的阿里云配置导入数据库
- 已有 Agent 注册机制不变 — 新增的是「如何让 Agent 到达目标机器」，注册/上报流程完全复用

---

## 2. 数据模型

### 2.1 凭据表 `credentials`

统一存储所有敏感凭据，被其他表引用。

```sql
CREATE TABLE IF NOT EXISTS credentials (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,  -- 'ssh_password' | 'ssh_key' | 'aliyun_ak'
    encrypted   TEXT NOT NULL,  -- AES-256-GCM 加密后的 JSON，Base64 编码
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

加密内容按类型：

| type | 加密前 JSON |
|------|------------|
| `ssh_password` | `{"password": "xxx"}` |
| `ssh_key` | `{"private_key": "-----BEGIN...","passphrase": "xxx"}` |
| `aliyun_ak` | `{"access_key_id": "LTAI...","access_key_secret": "xxx"}` |

**加密方案：**
- 算法：AES-256-GCM（认证加密，防篡改）
- 密钥来源：`server.yaml` 新增 `encryption_key` 字段（32 字节 hex）
- 首次启动时若未配置，自动生成随机密钥并写回配置文件
- 存储格式：`base64(nonce + ciphertext + tag)`

### 2.2 托管服务器表 `managed_servers`

记录通过 UI 添加的本地服务器及其安装状态。

```sql
CREATE TABLE IF NOT EXISTS managed_servers (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    host          TEXT NOT NULL,
    ssh_port      INTEGER DEFAULT 22,
    ssh_user      TEXT NOT NULL,
    credential_id INTEGER NOT NULL REFERENCES credentials(id),
    install_state TEXT DEFAULT 'pending',
    install_error TEXT DEFAULT '',
    agent_host_id TEXT DEFAULT '',
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_managed_servers_host ON managed_servers(host);
```

### 2.3 云账号表 `cloud_accounts`

```sql
CREATE TABLE IF NOT EXISTS cloud_accounts (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    provider        TEXT DEFAULT 'aliyun',
    credential_id   INTEGER NOT NULL REFERENCES credentials(id),
    region_ids      TEXT DEFAULT '[]',      -- JSON 数组，空数组=全部区域
    auto_discover   INTEGER DEFAULT 1,
    sync_state      TEXT DEFAULT 'pending', -- 'pending' | 'syncing' | 'synced' | 'failed'
    sync_error      TEXT DEFAULT '',
    last_synced_at  DATETIME,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### 2.4 云实例表 `cloud_instances`

替代现有的配置文件硬编码，存储自动发现或手动添加的云实例。

```sql
CREATE TABLE IF NOT EXISTS cloud_instances (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    cloud_account_id INTEGER NOT NULL REFERENCES cloud_accounts(id) ON DELETE CASCADE,
    instance_type    TEXT NOT NULL,           -- 'ecs' | 'rds'
    instance_id      TEXT NOT NULL,           -- 阿里云实例 ID（如 i-xxx, rm-xxx）
    host_id          TEXT NOT NULL UNIQUE,    -- OpsBoard 内部标识
    instance_name    TEXT DEFAULT '',
    region_id        TEXT DEFAULT '',
    spec             TEXT DEFAULT '',         -- 实例规格
    engine           TEXT DEFAULT '',         -- RDS: "MySQL 8.0" 等
    endpoint         TEXT DEFAULT '',         -- RDS 连接地址
    monitored        INTEGER DEFAULT 1,
    extra            TEXT DEFAULT '{}',       -- JSON 扩展字段
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_cloud_instances_account ON cloud_instances(cloud_account_id);
CREATE INDEX IF NOT EXISTS idx_cloud_instances_type ON cloud_instances(instance_type);
```

### 2.5 表关系

```
credentials (1) ←── (N) managed_servers
credentials (1) ←── (N) cloud_accounts (1) ←── (N) cloud_instances
                                                         │
                                        instance_type='ecs' → 注册到 servers 表
                                        instance_type='rds' → DatabaseHandler 查询
```

---

## 3. 功能一：本地服务器 Agent 自动部署

### 3.1 用户流程

```
┌─ 服务器列表页 ──────────────────────────────────────────┐
│ [+ 添加服务器] 按钮                                      │
└───────────────────────────────────────────────────────────┘
        │ 点击
        ▼
┌─ 添加服务器对话框 ───────────────────────────────────────┐
│  服务器地址: [192.168.10.62    ]                         │
│  SSH 端口:   [22               ]                         │
│  用户名:     [yuanqing         ]                         │
│                                                           │
│  认证方式:   (●) 密码   ( ) SSH 密钥                      │
│  密码:       [••••••••         ]                         │
│                                                           │
│  高级选项 ▼                                               │
│    Agent ID:     [auto-generate    ] (留空自动生成)       │
│    采集间隔:     [5] 秒                                   │
│    启用 Docker:  [✓]                                      │
│    启用 GPU:     [ ]                                      │
│                                                           │
│            [测试连接]   [取消]   [安装]                    │
└───────────────────────────────────────────────────────────┘
        │ 点击「测试连接」
        ▼
┌─ 安装进度面板 ───────────────────────────────────────────┐
│  ✅ SSH 连通性测试          成功 (延迟 23ms)             │
│  ⏳ 检查目标环境            检测系统架构...               │
│  ○  上传 Agent 二进制                                     │
│  ○  生成并上传配置文件                                    │
│  ○  安装 systemd 服务                                     │
│  ○  启动 Agent                                           │
│  ○  等待 Agent 注册                                       │
│                                                           │
│            [取消安装]                                      │
└───────────────────────────────────────────────────────────┘
        │ 全部完成
        ▼
┌─ 安装成功 ───────────────────────────────────────────────┐
│  ✅ Agent 已在线！                                        │
│  服务器: 192.168.10.62 (zentao)                          │
│  Agent ID: srv-62-zentao                                 │
│                        [完成]                             │
└───────────────────────────────────────────────────────────┘
```

### 3.2 安装状态机

```
                    ┌─────────┐
                    │ pending │ (初始)
                    └────┬────┘
                         │ 触发安装
                    ┌────▼────┐
                    │ testing │ SSH 连通性测试
                    └────┬────┘
                    成功  │  失败 → failed (error: "SSH连接失败: ...")
                    ┌────▼─────┐
                    │ connected│ 检查目标环境（架构、磁盘空间、已有Agent）
                    └────┬─────┘
                    成功  │  失败 → failed
                    ┌────▼──────┐
                    │ uploading │ SCP 上传二进制
                    └────┬──────┘
                    成功  │  失败 → failed
                    ┌────▼───────┐
                    │ installing │ 写配置 + 创建 systemd 服务 + 启动
                    └────┬───────┘
                    成功  │  失败 → failed
                    ┌────▼────┐
                    │ waiting │ 等待 Agent gRPC 注册 (超时 60 秒)
                    └────┬────┘
                    收到注册 │  超时 → failed (error: "Agent注册超时")
                    ┌────▼──┐
                    │ online│ 完成！关联 agent_host_id
                    └───────┘

任何 failed 状态均可点击「重试」，从 testing 重新开始。
```

### 3.3 后端实现

#### 3.3.1 新增包 `server/internal/deployer`

```go
// deployer/deployer.go — Agent 远程部署器

type Deployer struct {
    db            *sql.DB
    credStore     *store.CredentialStore
    serverStore   *store.ServerStore
    hub           *ws.Hub
    pskToken      string        // Agent PSK token，写入 agent.yaml
    grpcAddr      string        // gRPC 地址，写入 agent.yaml
    agentBinPath  string        // 本地 Agent 二进制路径
    pendingCh     map[string]chan struct{}  // agent_host_id → 注册通知 channel
    mu            sync.Mutex
}

// Deploy 启动异步安装流程
func (d *Deployer) Deploy(managedID int) error

// NotifyRegistered 由 gRPC handler 调用，当新 Agent 注册时通知 deployer
func (d *Deployer) NotifyRegistered(hostID string)
```

#### 3.3.2 SSH 执行器

```go
// deployer/ssh.go — SSH 连接和命令执行

type SSHClient struct {
    host     string
    port     int
    user     string
    authMethod ssh.AuthMethod  // 密码或密钥
}

func (s *SSHClient) TestConnection() (latencyMs int, err error)
func (s *SSHClient) DetectArch() (arch string, err error)
func (s *SSHClient) Upload(localPath, remotePath string) error     // SCP
func (s *SSHClient) Execute(cmd string) (stdout, stderr string, err error)
```

#### 3.3.3 安装流程伪代码

```go
func (d *Deployer) runDeploy(managed *ManagedServer) {
    // 1. testing: SSH 连通性
    d.updateState(managed.ID, "testing", "")
    d.broadcast(managed.ID, "testing", "SSH 连通性测试中...")

    client, err := d.newSSHClient(managed)
    latency, err := client.TestConnection()
    if err != nil {
        d.fail(managed.ID, "SSH连接失败: " + err.Error())
        return
    }
    d.broadcast(managed.ID, "testing_done", fmt.Sprintf("成功 (延迟 %dms)", latency))

    // 2. connected: 检查目标环境
    d.updateState(managed.ID, "connected", "")
    arch, err := client.DetectArch()  // uname -m
    // 检查磁盘空间: df -h /usr/local/bin
    // 检查是否已有 Agent 运行: pgrep opsboard-agent

    // 3. uploading: SCP 二进制
    d.updateState(managed.ID, "uploading", "")
    binPath := d.agentBinPath  // build/opsboard-agent-linux-{arch}
    err = client.Upload(binPath, "/tmp/opsboard-agent")

    // 4. installing: 配置 + systemd + 启动
    d.updateState(managed.ID, "installing", "")

    agentID := generateAgentID(managed.Host)  // 如 "srv-62-zentao"
    agentYAML := generateAgentConfig(d.grpcAddr, d.pskToken, agentID, managed.Options)
    // 写配置到远端
    client.Execute("sudo mkdir -p /etc/opsboard")
    client.Execute(fmt.Sprintf("echo '%s' | sudo tee /etc/opsboard/agent.yaml", agentYAML))
    // 安装二进制
    client.Execute("sudo mv /tmp/opsboard-agent /usr/local/bin/opsboard-agent")
    client.Execute("sudo chmod +x /usr/local/bin/opsboard-agent")
    // 创建 systemd 服务
    client.Execute(fmt.Sprintf("echo '%s' | sudo tee /etc/systemd/system/opsboard-agent.service", systemdUnit))
    client.Execute("sudo systemctl daemon-reload && sudo systemctl enable opsboard-agent && sudo systemctl start opsboard-agent")

    // 5. waiting: 等待注册
    d.updateState(managed.ID, "waiting", "")
    waitCh := d.registerWait(agentID)
    select {
    case <-waitCh:
        // Agent 注册成功
        d.updateState(managed.ID, "online", "")
        d.updateAgentHostID(managed.ID, agentID)
        d.broadcast(managed.ID, "online", "Agent 已在线")
    case <-time.After(60 * time.Second):
        d.fail(managed.ID, "Agent注册超时，请检查网络和防火墙")
    }
}
```

#### 3.3.4 WebSocket 消息格式

安装进度通过现有 WebSocket Hub 广播：

```json
{
    "type": "deploy_progress",
    "managed_id": 3,
    "state": "uploading",
    "message": "上传 Agent 二进制 (17MB)...",
    "progress": 45,
    "timestamp": 1711439200
}
```

### 3.4 API 端点

| 方法 | 路由 | 功能 |
|------|------|------|
| GET | `/api/v1/managed-servers` | 列出所有托管服务器 |
| POST | `/api/v1/managed-servers` | 添加托管服务器（保存信息，不立即安装）|
| POST | `/api/v1/managed-servers/:id/test` | 测试 SSH 连通性 |
| POST | `/api/v1/managed-servers/:id/deploy` | 触发 Agent 安装 |
| POST | `/api/v1/managed-servers/:id/retry` | 从失败状态重试安装 |
| DELETE | `/api/v1/managed-servers/:id` | 删除托管服务器记录 |
| POST | `/api/v1/managed-servers/:id/uninstall` | 远程卸载 Agent |

**POST `/api/v1/managed-servers` 请求体：**

```json
{
    "host": "192.168.10.62",
    "ssh_port": 22,
    "ssh_user": "yuanqing",
    "credential": {
        "name": "内网服务器密码",
        "type": "ssh_password",
        "data": { "password": "qw159753" }
    },
    "options": {
        "agent_id": "",
        "collect_interval": 5,
        "docker": true,
        "gpu": false
    }
}
```

也可复用已有凭据：

```json
{
    "host": "192.168.10.63",
    "ssh_port": 22,
    "ssh_user": "yuanqing",
    "credential_id": 1,
    "options": { "docker": false, "gpu": false }
}
```

### 3.5 gRPC 注册通知

现有 `grpc/handler.go` 的 `Register` 方法在新 Agent 注册时，增加一行回调：

```go
func (h *Handler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
    // ... 现有逻辑 ...

    // 通知 deployer（如果有安装任务在等待）
    if h.deployer != nil {
        h.deployer.NotifyRegistered(req.HostId)
    }

    return &pb.RegisterResponse{Accepted: true, ReportInterval: 5}, nil
}
```

---

## 4. 功能二：阿里云账号接入

### 4.1 用户流程

```
┌─ 数据库列表页 ──────────────────────────────────────────┐
│ [+ 添加云账号] 按钮                                      │
└───────────────────────────────────────────────────────────┘
        │ 点击
        ▼
┌─ 添加云账号对话框 ──────────────────────────────────────┐
│  云服务商:    阿里云 (目前仅支持)                         │
│  账号名称:    [碧橙-生产环境        ]                    │
│  AccessKey ID:     [LTAI5tQ58rX6...  ]                   │
│  AccessKey Secret: [••••••••••••     ]                    │
│                                                           │
│  自动发现实例: [✓] (关闭后需手动输入实例ID)               │
│  区域过滤:     [全部区域 ▼] (可选择指定区域)              │
│                                                           │
│            [验证AK]   [取消]   [保存]                     │
└───────────────────────────────────────────────────────────┘
        │ 点击「验证AK」
        ▼
┌─ 验证结果 ──────────────────────────────────────────────┐
│  ✅ AK 有效，RAM 子账号: monitor@12345678                │
│  权限检查:                                                │
│    ✅ ECS DescribeInstances — 可用                        │
│    ✅ CMS DescribeMetricLast — 可用                       │
│    ✅ RDS DescribeDBInstances — 可用                      │
└──────────────────────────────────────────────────────────┘
        │ 点击「保存」→ 自动触发实例发现
        ▼
┌─ 实例发现结果 ──────────────────────────────────────────┐
│  发现 14 台 ECS 实例 + 6 个 RDS 实例                     │
│                                                           │
│  ECS 实例                              监控               │
│  ┌──────────────────────────────────┬──────┐             │
│  │ 碧橙-AI工具平台 (i-uf6xxx)       │  [✓] │             │
│  │ 碧橙-Jenkins (i-uf6yyy)          │  [✓] │             │
│  │ ...                               │  [✓] │             │
│  └──────────────────────────────────┴──────┘             │
│                                                           │
│  RDS 实例                              监控               │
│  ┌──────────────────────────────────┬──────┐             │
│  │ BI组-魔镜 MySQL 8.0 (rm-bp1xxx)  │  [✓] │             │
│  │ EP全面预算 PG 15 (pgm-bp1xxx)    │  [✓] │             │
│  │ ...                               │  [✓] │             │
│  └──────────────────────────────────┴──────┘             │
│                                                           │
│  ⚠️ 提示：如需获取 ECS 完整操作系统级指标（内存、磁盘     │
│  使用率等），请前往阿里云云监控控制台安装监控插件：         │
│  https://cloudmonitor.console.aliyun.com/                 │
│  路径：云监控 → 主机监控 → 安装/升级插件                   │
│                                                           │
│            [全选] [全不选]  [确认接入]                     │
└──────────────────────────────────────────────────────────┘
```

### 4.2 手动添加实例（auto_discover=false）

当用户关闭自动发现时，显示手动输入界面：

```
┌─ 手动添加实例 ──────────────────────────────────────────┐
│  实例类型:  (●) ECS   ( ) RDS                            │
│  实例ID:    [i-uf6xxxxxxxxxxxx  ]                        │
│  区域:      [cn-hangzhou ▼      ]                        │
│                           [+ 添加]                       │
│                                                           │
│  已添加实例:                                              │
│  ┌───────────────────────────────────────────┐           │
│  │ ECS  i-uf6xxx  cn-hangzhou        [删除]  │           │
│  │ RDS  rm-bp1xxx cn-hangzhou        [删除]  │           │
│  └───────────────────────────────────────────┘           │
│                           [确认接入]                      │
└──────────────────────────────────────────────────────────┘
```

### 4.3 同步状态机

```
    ┌─────────┐
    │ pending │ (初始/刚创建)
    └────┬────┘
         │ 保存后自动触发
    ┌────▼────┐
    │ syncing │ 验证 AK + 发现/验证实例 + 注册
    └────┬────┘
    成功  │  失败 → failed (error 详情)
    ┌────▼────┐
    │ synced  │ 完成，实例已入库
    └─────────┘

synced 状态的账号会被 AliyunCollector 纳入采集循环。
可手动触发「重新同步」刷新实例列表。
```

### 4.4 后端实现

#### 4.4.1 新增包 `server/internal/cloud`

```go
// cloud/manager.go — 云账号管理器

type Manager struct {
    db          *sql.DB
    credStore   *store.CredentialStore
    hub         *ws.Hub
}

// Verify 验证 AK 有效性 + 检查权限
func (m *Manager) Verify(ak, sk string) (*VerifyResult, error)

// Sync 同步实例列表（自动发现 or 验证手动输入的实例）
func (m *Manager) Sync(accountID int) error

// DiscoverECS 发现所有 ECS 实例
func (m *Manager) DiscoverECS(ak, sk string, regionIDs []string) ([]CloudInstance, error)

// DiscoverRDS 发现所有 RDS 实例
func (m *Manager) DiscoverRDS(ak, sk string, regionIDs []string) ([]CloudInstance, error)
```

#### 4.4.2 AK 验证流程

```go
type VerifyResult struct {
    Valid       bool
    AccountUID  string   // RAM 账号 UID
    AccountName string   // RAM 用户名
    Permissions []PermissionCheck
}

type PermissionCheck struct {
    Action  string  // "ECS:DescribeInstances"
    Allowed bool
}

func (m *Manager) Verify(ak, sk string) (*VerifyResult, error) {
    // 1. 调用 STS GetCallerIdentity 验证 AK 有效性
    // 2. 尝试 ECS DescribeInstances (limit=1) 验证 ECS 权限
    // 3. 尝试 CMS DescribeMetricLast 验证 CloudMonitor 权限
    // 4. 尝试 RDS DescribeDBInstances (limit=1) 验证 RDS 权限
    // 返回各项权限检查结果
}
```

#### 4.4.3 实例发现

```go
func (m *Manager) DiscoverECS(ak, sk string, regionIDs []string) ([]CloudInstance, error) {
    // 若 regionIDs 为空，先调用 DescribeRegions 获取所有区域
    // 对每个区域调用 DescribeInstances (分页，PageSize=100)
    // 返回: instance_id, instance_name, region_id, spec, IPs, 状态
}

func (m *Manager) DiscoverRDS(ak, sk string, regionIDs []string) ([]CloudInstance, error) {
    // 若 regionIDs 为空，先调用 DescribeRegions 获取所有区域
    // 对每个区域调用 DescribeDBInstances (分页)
    // 返回: instance_id, instance_name, region_id, engine+version, spec, endpoint
}
```

#### 4.4.4 host_id 生成规则

```
ECS: "ecs-{instance_id}"       例: "ecs-i-uf6xxxxxxxxxxxx"
RDS: "rds-{instance_id}"       例: "rds-rm-bp1cvllbx3442lf0p"
```

### 4.5 API 端点

| 方法 | 路由 | 功能 |
|------|------|------|
| GET | `/api/v1/cloud-accounts` | 列出所有云账号 |
| POST | `/api/v1/cloud-accounts` | 添加云账号 |
| PUT | `/api/v1/cloud-accounts/:id` | 更新云账号 |
| DELETE | `/api/v1/cloud-accounts/:id` | 删除云账号（级联删除实例） |
| POST | `/api/v1/cloud-accounts/:id/verify` | 验证 AK 有效性 + 权限检查 |
| POST | `/api/v1/cloud-accounts/:id/sync` | 触发实例同步 |
| GET | `/api/v1/cloud-accounts/:id/instances` | 获取该账号下的实例列表 |
| PUT | `/api/v1/cloud-instances/:id` | 更新实例（启用/停用监控） |
| POST | `/api/v1/cloud-instances` | 手动添加实例 |
| DELETE | `/api/v1/cloud-instances/:id` | 删除实例 |

**POST `/api/v1/cloud-accounts` 请求体：**

```json
{
    "name": "碧橙-生产环境",
    "provider": "aliyun",
    "credential": {
        "name": "碧橙阿里云AK",
        "type": "aliyun_ak",
        "data": {
            "access_key_id": "LTAI5tQ58rX6zYTb7rejSPTW",
            "access_key_secret": "xxxxx"
        }
    },
    "auto_discover": true,
    "region_ids": []
}
```

### 4.6 WebSocket 消息

```json
{
    "type": "cloud_sync_progress",
    "account_id": 1,
    "state": "syncing",
    "message": "正在发现 ECS 实例 (cn-hangzhou)...",
    "timestamp": 1711439200
}
```

---

## 5. AliyunCollector 重构

### 5.1 数据源切换

现有 `AliyunCollector` 从 `config.AliyunConfig` 读取实例列表。重构后改为**双数据源**：

1. **数据库优先** — 从 `cloud_accounts` + `cloud_instances` 读取
2. **配置文件回退** — 如果数据库中无云账号，仍从 `server.yaml` 读取（向后兼容）

```go
type AliyunCollector struct {
    // 移除: cfg config.AliyunConfig
    // 新增:
    cloudStore  *store.CloudStore     // 从数据库读取账号和实例
    credStore   *store.CredentialStore // 解密 AK
    fallbackCfg *config.AliyunConfig  // 旧配置回退
    // ... 其余不变
}

func (ac *AliyunCollector) loadInstances() (ecsInstances []ECSTarget, rdsInstances []RDSTarget) {
    // 1. 从数据库加载所有 synced 状态、monitored=1 的实例
    // 2. 如果数据库为空，从 fallbackCfg 加载
    // 3. 每 60 秒刷新一次（支持运行时动态增减实例）
}
```

### 5.2 旧配置自动导入

Server 启动时执行一次：

```go
func (ac *AliyunCollector) migrateFromConfig(cfg config.AliyunConfig) error {
    // 1. 检查 cloud_accounts 表是否为空
    // 2. 如果为空且 cfg 有内容：
    //    a. 创建 credential (type=aliyun_ak, 加密存储 AK/SK)
    //    b. 创建 cloud_account (name="从配置文件导入", auto_discover=false)
    //    c. 为每个 cfg.Instances 创建 cloud_instance (type=ecs)
    //    d. 为每个 cfg.RDS 创建 cloud_instance (type=rds)
    //    e. 设置 sync_state=synced
    // 3. 记录日志: "已从 server.yaml 导入 N 个 ECS + M 个 RDS 实例"
}
```

### 5.3 DatabaseHandler 重构

现有 `DatabaseHandler` 接收 `[]config.AliyunRDS` 参数（来自配置文件）。重构后改为从数据库读取：

```go
type DatabaseHandler struct {
    cloudStore *store.CloudStore   // 替代 rdsConfig []config.AliyunRDS
    vm         *store.VictoriaStore
    // ... 缓存逻辑不变
}
```

### 5.4 前端 RDS 元信息去硬编码

现有 `Databases/index.tsx` 中硬编码的 `RDS_INSTANCES` 映射表删除，改为从 API 获取：

```typescript
// 删除硬编码的 RDS_INSTANCES
// 新增 API 返回字段:
interface RDSInfo {
    host_id: string
    name: string           // 从 cloud_instances.instance_name 获取
    engine: string         // 从 cloud_instances.engine 获取
    spec: string           // 从 cloud_instances.spec 获取
    endpoint: string       // 从 cloud_instances.endpoint 获取
    metrics: Record<string, number>
}
```

---

## 6. 凭据管理

### 6.1 Store 层

```go
// store/credential_store.go

type CredentialStore struct {
    db        *sql.DB
    masterKey []byte  // 32 字节 AES-256 密钥
}

func NewCredentialStore(db *sql.DB, keyHex string) *CredentialStore
func (s *CredentialStore) Create(name, credType string, data map[string]string) (int, error)
func (s *CredentialStore) Get(id int) (*Credential, error)       // 返回解密后的数据
func (s *CredentialStore) List() ([]CredentialSummary, error)    // 不含敏感数据
func (s *CredentialStore) Update(id int, name string, data map[string]string) error
func (s *CredentialStore) Delete(id int) error                   // 检查引用关系

// 加密/解密内部方法
func (s *CredentialStore) encrypt(plaintext []byte) (string, error)  // → base64
func (s *CredentialStore) decrypt(ciphertext string) ([]byte, error) // base64 →
```

### 6.2 API 端点

| 方法 | 路由 | 功能 |
|------|------|------|
| GET | `/api/v1/credentials` | 列出凭据（不含敏感数据） |
| POST | `/api/v1/credentials` | 创建凭据 |
| PUT | `/api/v1/credentials/:id` | 更新凭据 |
| DELETE | `/api/v1/credentials/:id` | 删除凭据（检查引用） |

**GET 响应**（脱敏）：

```json
[
    {
        "id": 1,
        "name": "内网服务器密码",
        "type": "ssh_password",
        "created_at": "2026-03-26T10:00:00Z",
        "used_by": 3
    }
]
```

---

## 7. 配置变更

### 7.1 `server.yaml` 新增字段

```yaml
# 加密密钥（首次启动自动生成）
encryption_key: "a1b2c3d4e5f6..."  # 64 位 hex = 32 字节

# Agent 二进制路径（部署器使用）
agent:
  binary_dir: "./build"  # 存放 opsboard-agent-linux-{arch} 的目录
```

### 7.2 Config 结构新增

```go
type Config struct {
    // ... 现有字段 ...
    EncryptionKey string       `yaml:"encryption_key"`
    Agent         AgentBinConfig `yaml:"agent"`
}

type AgentBinConfig struct {
    BinaryDir string `yaml:"binary_dir"`
}
```

---

## 8. 前端变更

### 8.1 新增页面/组件

| 组件 | 位置 | 功能 |
|------|------|------|
| `AddServerDialog` | `web/src/components/AddServerDialog.tsx` | 添加服务器对话框（表单 + 进度面板） |
| `DeployProgress` | `web/src/components/DeployProgress.tsx` | 安装进度步骤指示器 |
| `AddCloudAccountDialog` | `web/src/components/AddCloudAccountDialog.tsx` | 添加云账号对话框 |
| `InstanceSelector` | `web/src/components/InstanceSelector.tsx` | 实例勾选列表 |
| `ManualInstanceForm` | `web/src/components/ManualInstanceForm.tsx` | 手动添加实例表单 |

### 8.2 修改的现有页面

| 页面 | 变更 |
|------|------|
| `Servers/index.tsx` | 顶部添加「+ 添加服务器」按钮；已有托管服务器显示安装状态徽章 |
| `Databases/index.tsx` | 顶部添加「+ 添加云账号」按钮；删除硬编码的 `RDS_INSTANCES`，改从 API 获取元信息 |
| `DatabaseDetail/index.tsx` | 数据库信息卡从 API 获取而非硬编码映射 |

### 8.3 新增 API 客户端

```typescript
// api/client.ts 新增

// 托管服务器
export const getManagedServers = () => get<ManagedServer[]>('/managed-servers')
export const addManagedServer = (data: AddServerRequest) => post<ManagedServer>('/managed-servers', data)
export const testSSH = (id: number) => post<SSHTestResult>(`/managed-servers/${id}/test`)
export const deployAgent = (id: number) => post(`/managed-servers/${id}/deploy`)
export const retryDeploy = (id: number) => post(`/managed-servers/${id}/retry`)
export const deleteManagedServer = (id: number) => del(`/managed-servers/${id}`)
export const uninstallAgent = (id: number) => post(`/managed-servers/${id}/uninstall`)

// 云账号
export const getCloudAccounts = () => get<CloudAccount[]>('/cloud-accounts')
export const addCloudAccount = (data: AddCloudAccountRequest) => post<CloudAccount>('/cloud-accounts', data)
export const updateCloudAccount = (id: number, data: Partial<CloudAccount>) => put(`/cloud-accounts/${id}`, data)
export const deleteCloudAccount = (id: number) => del(`/cloud-accounts/${id}`)
export const verifyAK = (id: number) => post<VerifyResult>(`/cloud-accounts/${id}/verify`)
export const syncInstances = (id: number) => post(`/cloud-accounts/${id}/sync`)
export const getCloudInstances = (accountId: number) => get<CloudInstance[]>(`/cloud-accounts/${accountId}/instances`)
export const updateCloudInstance = (id: number, data: Partial<CloudInstance>) => put(`/cloud-instances/${id}`, data)
export const addCloudInstance = (data: AddInstanceRequest) => post<CloudInstance>('/cloud-instances', data)
export const deleteCloudInstance = (id: number) => del(`/cloud-instances/${id}`)

// 凭据
export const getCredentials = () => get<CredentialSummary[]>('/credentials')
export const deleteCredential = (id: number) => del(`/credentials/${id}`)
```

### 8.4 WebSocket 消息处理

在 `useWebSocket.ts` 中新增处理：

```typescript
case 'deploy_progress':
    // 更新 AddServerDialog 的安装进度
    break
case 'cloud_sync_progress':
    // 更新 AddCloudAccountDialog 的同步进度
    break
```

---

## 9. 新增文件清单

### 后端

| 文件 | 功能 |
|------|------|
| `server/internal/crypto/aes.go` | AES-256-GCM 加密/解密 |
| `server/internal/store/credential_store.go` | 凭据 CRUD + 加解密 |
| `server/internal/store/managed_server_store.go` | 托管服务器 CRUD + 状态更新 |
| `server/internal/store/cloud_store.go` | 云账号 + 云实例 CRUD |
| `server/internal/deployer/deployer.go` | Agent 远程部署器（状态机 + 异步执行） |
| `server/internal/deployer/ssh.go` | SSH 连接、命令执行、SCP 上传 |
| `server/internal/cloud/manager.go` | 云账号管理（验证、发现、同步） |
| `server/internal/api/managed_server_handler.go` | 托管服务器 API |
| `server/internal/api/cloud_handler.go` | 云账号 + 云实例 API |
| `server/internal/api/credential_handler.go` | 凭据 API |

### 前端

| 文件 | 功能 |
|------|------|
| `web/src/components/AddServerDialog.tsx` | 添加服务器对话框 |
| `web/src/components/DeployProgress.tsx` | 安装进度组件 |
| `web/src/components/AddCloudAccountDialog.tsx` | 添加云账号对话框 |
| `web/src/components/InstanceSelector.tsx` | 实例勾选列表 |
| `web/src/components/ManualInstanceForm.tsx` | 手动添加实例 |
| `web/src/api/onboarding.ts` | 接入管理相关 API 调用 |
| `web/src/types/onboarding.ts` | 接入管理相关类型定义 |

### 修改的文件

| 文件 | 变更 |
|------|------|
| `server/internal/store/sqlite.go` | migrate() 新增 4 张表 + 索引 |
| `server/internal/config/config.go` | 新增 EncryptionKey、AgentBinConfig |
| `server/internal/grpc/handler.go` | Register 中回调 deployer.NotifyRegistered |
| `server/internal/collector/aliyun.go` | 重构数据源，支持从数据库读取 |
| `server/internal/api/database_handler.go` | 重构，从 CloudStore 读取 RDS 列表 |
| `server/internal/api/router.go` | 新增路由组 |
| `server/cmd/server/main.go` | 初始化新组件、旧配置迁移 |
| `web/src/pages/Servers/index.tsx` | 添加「+ 添加服务器」按钮 |
| `web/src/pages/Databases/index.tsx` | 添加「+ 添加云账号」按钮，去硬编码 |
| `web/src/pages/DatabaseDetail/index.tsx` | 元信息从 API 获取 |
| `web/src/hooks/useWebSocket.ts` | 处理新消息类型 |
| `web/src/api/client.ts` | 新增 API 函数 |
| `web/src/types/index.ts` | 新增类型定义 |

---

## 10. 安全考量

1. **凭据加密** — 所有敏感信息（密码、密钥、AK/SK）使用 AES-256-GCM 加密存储，master key 仅在 server.yaml 中
2. **API 脱敏** — GET 凭据列表不返回敏感字段，只返回名称、类型、使用数
3. **SSH 安全** — SSH 连接使用 `golang.org/x/crypto/ssh`，支持 host key 验证（首次连接信任，后续校验）
4. **命令注入防护** — 所有 SSH 远端执行的命令使用模板生成，不拼接用户输入；agent.yaml 内容由后端生成
5. **权限控制** — 所有接入管理 API 受 JWT 认证保护

---

## 11. 依赖

### 新增 Go 依赖

| 包 | 用途 |
|-----|------|
| `golang.org/x/crypto/ssh` | SSH 客户端连接、命令执行 |
| `github.com/pkg/sftp` | SFTP 文件上传（替代 SCP） |
| `github.com/alibabacloud-go/sts-20150401/v2` | STS GetCallerIdentity（AK 验证） |
| `github.com/alibabacloud-go/rds-20140815/v6` | RDS DescribeDBInstances（实例发现） |

### 现有依赖复用

| 包 | 用途 |
|-----|------|
| `github.com/alibabacloud-go/ecs-20140526/v4` | ECS 实例发现（已有） |
| `github.com/alibabacloud-go/cms-20190101/v2` | CloudMonitor 指标采集（已有） |
| `crypto/aes` + `crypto/cipher` | AES-256-GCM 加密（标准库） |
