# MantisOps 多用户权限管理设计文档

> 日期：2026-03-29
> 状态：草稿

## 一、概述

为 MantisOps 添加多用户账号体系，支持三级角色（admin / operator / viewer）和资源级权限控制。面向 10-30 人中型运维团队，每个用户可配置可见的服务器分组、服务器、数据库、探测规则范围。

### 核心决策

| 决策项 | 选择 |
|--------|------|
| 角色模型 | admin / operator / viewer 三级，高权限包含低权限 |
| 资源权限 | 混合模式：按分组批量授权 + 单资源追加 |
| admin 权限 | 不受资源权限限制，自动看到全部 |
| 认证方式 | 账号密码（bcrypt），后续可扩展钉钉/LDAP |
| 即时踢下线 | JWT token_version 机制 |
| 强制改密 | 管理员创建用户后首次登录强制改密 |
| 旧账号迁移 | 首次启动从 server.yaml 创建初始 admin，配置文件账密字段废弃 |

## 二、数据模型

### 2.1 users 表

```sql
CREATE TABLE users (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    username        TEXT UNIQUE NOT NULL,
    password_hash   TEXT NOT NULL,
    display_name    TEXT DEFAULT '',
    role            TEXT NOT NULL DEFAULT 'viewer',
    enabled         BOOLEAN DEFAULT 1,
    must_change_pwd BOOLEAN DEFAULT 0,
    token_version   INTEGER DEFAULT 1,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

字段说明：

| 字段 | 说明 |
|------|------|
| password_hash | bcrypt（cost=10） |
| role | `admin` / `operator` / `viewer` |
| enabled | 0 = 禁用，禁用后 token_version 自动递增 |
| must_change_pwd | 管理员创建用户 / 重置密码后设为 1，用户改密后设为 0 |
| token_version | 角色变更 / 禁用 / 改密 / 重置密码时递增，使旧 JWT 立即失效 |

### 2.2 user_permissions 表

```sql
CREATE TABLE user_permissions (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    res_type TEXT NOT NULL,
    res_id   TEXT NOT NULL
);
CREATE UNIQUE INDEX idx_user_perm ON user_permissions(user_id, res_type, res_id);
```

字段说明：

| 字段 | 说明 |
|------|------|
| res_type | `group` / `server` / `database` / `probe` |
| res_id | 分组 ID（整数转字符串）、服务器 host_id、RDS host_id、探测规则 ID |

权限解析规则：
- **admin** 角色不查此表，直接放行全部资源
- **勾选 group** → 该组下所有服务器 + 服务器挂载的容器和资产自动可见
- **勾选 server** → 跨组追加单台服务器可见性
- **database** 独立于服务器分组，需要独立勾选
- **probe** 独立勾选。虽然 `probe_rules` 表有 `server_id` 外键，但探测规则经常跨服务器（如监控外部域名），因此不跟随服务器自动授权，必须显式勾选
- 容器和资产跟随服务器，不需要独立权限记录
- **去重规则**：后端保存权限时进行去重——若某 server 已经属于用户绑定的 group，则不重复存储该 server 的独立记录。前端提交时无需去重，后端在全量覆盖写入前负责清理冗余
- **operator 创建资源的自动归属**：operator 创建探测规则（POST /probes）或告警规则（POST /alerts/rules）后，后端自动为该用户添加对应的 `user_permissions` 记录（`res_type=probe, res_id=新规则ID`），确保创建者能立即看到和管理自己创建的资源。同理，operator 删除自己有权限的 probe/alert_rule 时，对应权限记录也自动清理。admin 创建资源无需此逻辑（admin 不受权限限制）

### 2.3 JWT payload

```json
{
  "user_id": 1,
  "username": "admin",
  "role": "admin",
  "token_version": 3,
  "must_change_pwd": false,
  "exp": 1775316743
}
```

## 三、认证与鉴权

### 3.1 认证流程

```
POST /api/v1/auth/login
  → 查 users 表（按 username）
  → bcrypt 校验密码
  → 检查 enabled（禁用用户拒绝登录）
  → 生成 JWT（含 user_id, username, role, token_version, must_change_pwd）
  → 返回 token + username + role + must_change_pwd
```

### 3.2 Token 版本号缓存

```go
type TokenVersionCache struct {
    mu    sync.RWMutex
    cache map[int64]int64  // user_id → token_version
}
```

- JWTMiddleware 解析 token 后，从缓存（或查库）获取当前 `token_version`
- **比对规则**：若 `jwt.token_version < db.token_version`，则返回 401（token 已失效）。使用 `<` 而非 `!=`，确保版本号只增不减的语义正确
- 缓存 miss 时查库并填充
- 以下操作触发版本号递增（`UPDATE users SET token_version = token_version + 1`）+ 清除该用户缓存条目：
  - 角色变更
  - 账号禁用 / 启用
  - 用户自己改密码
  - 管理员重置密码
  - 删除用户（删除后查库返回空，中间件直接 401）

### 3.3 强制改密拦截

`must_change_pwd = true` 的 token 只允许访问：
- `GET /api/v1/auth/me`
- `PUT /api/v1/auth/password`

其他请求返回 `403 {"error": "must_change_password"}`。前端据此强制跳转改密页面，拦截所有其他路由导航。

### 3.4 角色级鉴权

`RequireRole(minRole)` 中间件，角色层级：`admin > operator > viewer`。

| 路由 | 最低角色 | 备注 |
|------|---------|------|
| 大多数 GET 查询接口 | viewer | 含 servers、dashboard、probes/status、alerts/events、alerts/stats、databases、billing、assets |
| GET /alerts/rules | viewer | 只读，viewer 可看规则但不可修改 |
| GET /alerts/channels | operator | 包含 Webhook URL 等敏感信息，viewer 不可见 |
| GET /logs/audit | admin | 审计日志仅管理员可查 |
| 告警确认（ack） | operator | |
| 探测规则 / 资产 / 告警规则 / 通知渠道 CRUD | operator | |
| 用户管理 CRUD | admin | |
| 接入管理（服务器 / 云账号 / 凭据） | admin | |
| 平台配置（PUT /settings） | admin | |
| 服务器名称编辑、分组管理 | admin | |

### 3.5 资源级鉴权

admin 直接放行。operator / viewer 需要检查：

```
请求到达 → 从 JWT 取 user_id 和 role
  → role == admin → 放行
  → 否则查用户可见资源集合（带缓存）
  → 检查请求的目标资源是否在可见集合内
```

**可见资源集合计算：**

```
可见服务器 = 直接绑定的 server host_id
           ∪ 绑定的 group 下所有 server 的 host_id
可见容器   = 可见服务器上的所有容器
可见资产   = 可见服务器上的所有资产
可见数据库 = 直接绑定的 database host_id
可见探测   = 直接绑定的 probe ID
```

**列表接口过滤：**

返回数据时按用户可见资源集合过滤结果列表：
- `GET /servers`、`GET /dashboard` — 按可见服务器过滤，统计数字也按范围计算
- `GET /databases` — 按可见数据库过滤
- `GET /probes`、`GET /probes/status` — 按可见探测规则过滤
- `GET /assets` — 按可见服务器过滤（资产跟随服务器）
- `GET /containers`（Dashboard WebSocket） — 按可见服务器过滤
- `GET /alerts/events`、`GET /alerts/stats` — **按告警目标资源过滤**：事件的 `target_id` 映射到服务器/探测规则，只返回用户有权限的目标的告警
- `GET /alerts/rules` — 按规则的 `target_id` 过滤（`target_id` 为空表示全局规则，如 `server_offline`，所有用户可见）
- `GET /billing` — 按可见数据库/服务器过滤（ECS/RDS 资源与 cloud_instances 的 host_id 关联）

**资源权限缓存：**

```go
type PermissionCache struct {
    mu    sync.RWMutex
    cache map[int64]*PermissionSet  // user_id → 可见资源集合
}

type PermissionSet struct {
    Groups    map[string]bool  // group ID
    Servers   map[string]bool  // host_id（直接 + 组展开）
    Databases map[string]bool  // RDS host_id
    Probes    map[string]bool  // probe ID
}
```

用户权限变更时清除该用户的缓存条目。服务器分组变更（服务器移入/移出分组）时清除所有缓存。

### 3.6 WebSocket 鉴权扩展

现有 `/ws?token=<jwt>` 不变，但连接建立后 Hub 记录该连接的 user_id、role 和 `*PermissionSet`（从 PermissionCache 获取）。

**职责划分**：Hub 负责按连接的 PermissionSet 过滤推送，调用方（metrics collector 等）无需感知权限，统一调用 `hub.BroadcastMetrics(hostID, msg)` 即可。Hub 内部遍历连接时，检查每个连接的 PermissionSet 是否包含该 hostID，不包含则跳过。admin 连接的 PermissionSet 为 nil，表示全部放行。

新增 Hub 方法：
```go
func (h *Hub) BroadcastMetrics(hostID string, msg interface{})            // 按可见服务器过滤
func (h *Hub) BroadcastAlert(targetType, targetID string, msg interface{}) // 按告警目标资源过滤
func (h *Hub) BroadcastLog(source string, msg interface{})                // 按日志来源过滤
```

**告警广播过滤规则**：告警事件携带 `target_id`（服务器 host_id 或探测规则 ID），`BroadcastAlert` 根据告警类型判断目标资源类型：
- `server_offline / cpu / memory / disk / network_rx / network_tx / gpu_*` → 检查连接的 `PermissionSet.Servers` 是否包含 `target_id`
- `probe_down` → 检查连接的 `PermissionSet.Probes` 是否包含 `target_id`
- `container` → 检查容器所在服务器是否在 `PermissionSet.Servers` 中
- admin 连接（PermissionSet 为 nil）始终放行

`alert_resolved` 和 `alert_acked` 同样需要按原始事件的 target 过滤，不能无差别广播。原有 `BroadcastJSON` 废弃，所有推送都走带过滤的方法。

**连接生命周期管理：**

Hub 维护 `map[user_id][]*Connection` 索引，支持按 user_id 查找所有活跃连接。

以下操作触发 **强制断开** 该用户所有 WS 连接（发送 close frame 后关闭）：
- 账号禁用
- 删除用户
- 角色降权（如 admin → operator，operator → viewer）
- 管理员重置密码

以下操作触发 **更新 PermissionSet**（不断连，热更新权限范围）：
- 资源权限变更（PUT /users/:id/permissions）
- 角色升权（如 viewer → operator）

断连后前端 WebSocket 自动重连机制会触发，此时用旧 token 重连会被 JWT 校验拒绝（token_version 已递增），前端收到 401 后跳转登录页。

日志订阅同理：运行日志按可见来源过滤，审计日志仅 admin 连接可收到。

### 3.7 系统不变量

**至少保留一个启用的 admin 用户**。以下操作必须在后端校验此约束，违反时返回 `409 Conflict`：

- 禁用用户：若目标是 admin，检查剩余 enabled admin 数量 > 1
- 删除用户：若目标是 admin，检查剩余 enabled admin 数量 > 1
- 角色降权：若从 admin 降为其他角色，检查剩余 enabled admin 数量 > 1
- 禁用自己：**不允许**（不论角色），返回 `409 {"error": "cannot disable yourself"}`
- 删除自己：**不允许**，返回 `409 {"error": "cannot delete yourself"}`

前端同步做防护：
- 编辑自己时，角色选择器和启用/禁用开关置灰不可操作
- 当系统只剩一个 admin 时，该用户的角色选择器和禁用/删除按钮置灰，tooltip 提示"系统至少需要一个管理员"

## 四、API 端点

### 4.1 用户管理（admin）

```
POST   /api/v1/users                  # 创建用户
GET    /api/v1/users                  # 用户列表
GET    /api/v1/users/:id              # 用户详情
PUT    /api/v1/users/:id              # 编辑用户（角色/显示名/启用禁用）
DELETE /api/v1/users/:id              # 删除用户
PUT    /api/v1/users/:id/reset-pwd    # 重置密码 → must_change_pwd=1
PUT    /api/v1/users/:id/permissions  # 设置资源权限（全量覆盖）
GET    /api/v1/users/:id/permissions  # 查看资源权限
```

### 4.2 用户自助

```
PUT    /api/v1/auth/password          # 修改自己的密码
GET    /api/v1/auth/me                # 获取当前用户信息（含 role）
```

**改密接口详细定义：**

`PUT /api/v1/auth/password`

请求体：
```json
{
  "old_password": "current123",
  "new_password": "newpass456"
}
```

- 强制改密场���（`must_change_pwd=true`）：`old_password` 为管理员设置的初始密码
- 校验旧密码正确后，bcrypt 新密码写入，`must_change_pwd = 0`，`token_version += 1`
- 响应中直接返回新 token��含更新后的 `must_change_pwd=false` 和新 `token_version`），前端替换本地 token 后跳转首页

响应体：
```json
{
  "token": "eyJ...",
  "username": "zhangsan",
  "role": "operator",
  "display_name": "张三",
  "must_change_pwd": false
}
```

### 4.3 创建用户请求体

```json
{
  "username": "zhangsan",
  "password": "initial123",
  "display_name": "张三",
  "role": "operator"
}
```

创建时自动设置 `must_change_pwd = 1`。

### 4.4 设置权限请求体

```json
{
  "permissions": [
    {"res_type": "group", "res_id": "1"},
    {"res_type": "group", "res_id": "3"},
    {"res_type": "server", "res_id": "srv-69-ai"},
    {"res_type": "database", "res_id": "rds-xxx"},
    {"res_type": "probe", "res_id": "5"}
  ]
}
```

全量覆盖：先删除该用户所有权限记录，再批量插入新记录。

### 4.5 登录响应扩展

```json
{
  "token": "eyJ...",
  "username": "zhangsan",
  "role": "operator",
  "display_name": "张三",
  "must_change_pwd": true
}
```

### 4.6 /auth/me 响应扩展

```json
{
  "user_id": 2,
  "username": "zhangsan",
  "role": "operator",
  "display_name": "张三",
  "must_change_pwd": false
}
```

## 五、旧账号迁移

服务启动时在 `main.go` 中执行：

1. 检查 `users` 表是否有记录
2. 若为空，用 `server.yaml` 中的 `auth.username` 和 `auth.password` 创建初始 admin 用户
3. **`server.yaml` 中的密码始终为明文**（当前代码 `auth.go` 直接明文比对），迁移时对其做 bcrypt 哈希后写入 `password_hash`
4. `must_change_pwd = 0`（初始管理员无需强制改密）
5. 日志记录迁移完成
6. `server.yaml` 文件不做修改（保持兼容），但代码中 `auth.username/password` 仅用于此次迁移，后续认证完全走数据库
7. 若 `server.yaml` 中 `auth.username` 为空，跳过迁移（兼容未配置的情况）

## 六、前端改造

### 6.1 新增页面

**用户管理页（/users，仅 admin 可见）：**
- 用户列表表格：用户名、显示名、角色标签、状态（启用/禁用）、创建时间、操作
- 创建用户对话框：用户名、初始密码、显示名、角色选择
- 编辑用户对话框：显示名、角色选择、启用/禁用
- 重置密码按钮（确认对话框）
- 删除用户按钮（确认对话框，不可删除自己）

**权限配置页（/users/:id/permissions，仅 admin 可见）：**
- 组织架构树形结构，左侧树、右侧已选列表
- 树形节点层级：
  ```
  ├── 服务器分组
  │   ├── [组] 生产环境        ☑ （勾选组 = 组下所有服务器）
  │   │   ├── srv-71-opsboard  ☐ （组已勾选时自动勾选，灰色不可单独取消）
  │   │   └── srv-69-ai        ☐
  │   ├── [组] 开发环境        ☐
  │   │   └── srv-65-yuanqing2 ☑ （可单独勾选跨组追加）
  │   └── 未分组
  │       └── ...
  ├── 数据库
  │   ├── rds-xxx              ☑
  │   └── rds-yyy              ☐
  └── 探测规则
      ├── HTTP 探测 - xxx      ☑
      └── TCP 探测 - yyy       ☐
  ```
- 保存按钮提交全量权限列表

**强制改密页（/change-password）：**
- 初始密码（旧密码）+ 新密码 + 确认新密码
- 提示文案："管理员已为您设置初始密码，请输入初始密码后设置新密码"
- 调用 `PUT /api/v1/auth/password`（`old_password` = 初始密码，`new_password` = 新密码）
- 成功后用响应中的新 token 替换本地存储，跳转首页

**普通改密（用户菜单入口）：**
- 在用户菜单下拉中增加"修改密码"入口，打开对话框
- 旧密码 + 新密码 + 确认新密码
- 复用同一个 `PUT /api/v1/auth/password` 接口

### 6.2 路由守卫扩展

```
RequireAuth        → 未登录跳转 /login
RequireChangePwd   → must_change_pwd=true 时跳转 /change-password
RequireRole(role)  → 权限不足显示 403 页面或隐藏菜单项
```

### 6.3 侧边栏菜单

- 「用户管理」菜单项：图标 `group`，路由 `/users`，仅 admin 角色显示
- 位置：在「系统信息」下方

### 6.4 authStore 扩展

```typescript
interface AuthState {
  token: string | null
  username: string | null
  role: string | null          // 新增
  displayName: string | null   // 新增
  mustChangePwd: boolean       // 新增
  // ...
}
```

### 6.5 前端按角色隐藏

| 元素 | viewer 可见 | operator 可见 | admin 可见 |
|------|------------|--------------|------------|
| 所有监控数据（按权限范围） | ✅ | ✅ | ✅ |
| 告警确认按钮 | ❌ | ✅ | ✅ |
| 探测/资产/告警规则 CRUD 按钮 | ❌ | ✅ | ✅ |
| 接入管理区域 | ❌ | ❌ | ✅ |
| 用户管理菜单 | ❌ | ❌ | ✅ |
| 平台配置保存按钮 | ❌ | ❌ | ✅ |
| 分组管理按钮 | ❌ | ❌ | ✅ |

## 七、日志与审计

### 7.1 审计日志

所有用户管理操作记录到 `audit_logs`：
- 创建/编辑/删除用户
- 角色变更
- 权限变更
- 重置密码
- 用户自行改密

`username` 字段记录操作人（从 JWT 提取）。

### 7.2 运行日志权限过滤

- 审计日志（audit tab）：仅 admin 可查看
- 运行日志（runtime tab）：按用户可见范围严格过滤
  - admin 看到全部（`source=server` + 所有 `agent:*`）
  - **operator/viewer 只能看到 `source = agent:{可见的host_id}` 的日志**
  - `source = server` 是全局服务端系统日志（告警引擎、采集器、部署器等模块输出），包含所有服务器的运维信息，**非 admin 不可见**
  - WebSocket 日志推送同理：订阅时 Hub 按连接的 PermissionSet 过滤 source

### 7.3 日志端点 RBAC 规则

| 端点 | 最低角色 | 资源过滤 |
|------|---------|---------|
| GET /logs/audit | admin | 无需过滤（admin only） |
| GET /logs/runtime | viewer | 按上述 7.2 规则过滤 source |
| GET /logs/export | viewer | 审计导出需 admin；运行日志导出按 7.2 过滤 |
| GET /logs/sources | viewer | 只返回用户可见的 source 列表（admin 全部，其他用户只有 `agent:{可见host_id}`） |
| GET /logs/stats | viewer | 按用户可见 source 范围统计（admin 看全局统计，其他用户看可见范围统计） |

## 八、现有代码改造

| 改造项 | 影响范围 |
|--------|---------|
| auth.go 重写 | Login 查库校验 bcrypt，JWT payload 扩展，JWTMiddleware 增加 token_version 检查 |
| 新增 user_store.go | users 表 + user_permissions 表 CRUD |
| 新增 user_handler.go | 用户管理 API 端点 |
| 新增 permission.go | 资源权限中间件 + 缓存 |
| router.go | 路由分组挂载 RequireRole 中间件，注册用户管理路由 |
| main.go | 初始化 UserStore、迁移逻辑、AuthHandler 改为依赖 UserStore |
| config.go | auth 段保留但仅用于首次迁移 |
| sqlite.go | 新增 users 和 user_permissions 建表 |
| hub.go | 连接记录 user_id/role，metrics 推送按权限过滤 |
| 所有列表 handler | 注入用户可见资源集合，过滤返回数据 |
| Dashboard handler | 统计数字和列表按权限范围计算 |
| 审计中间件 | username 从 JWT 的 user_id 关联，无变化（已用 c.Get("username")） |
| 前端 authStore | 扩展 role、displayName、mustChangePwd |
| 前端路由 | 新增 /users、/change-password，路由守卫扩展 |
| 前端 Sidebar | 新增用户管理菜单，按角色显隐 |
| 前端各页面 | 操作按钮按角色显隐 |
| 前端 useWebSocket | 无需改动（后端推送时已过滤） |

## 九、不做的事（YAGNI）

- 不做 LDAP / 钉钉 OAuth（当前阶段账号密码足够）
- 不做部门/组织架构（角色 + 资源权限足够）
- 不做 API 级细粒度权限（不区分"可查看告警"和"可查看服务器"，同角色权限统一）
- 不做操作审批流（如删除需审批）
- 不做会话管理 UI（查看/踢出在线用户）
- 不做密码复杂度策略（保持简单）
- 不做 token 刷新机制（7 天有效期 + token_version 足够）
