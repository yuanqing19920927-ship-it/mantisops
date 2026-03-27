# MantisOps 前端功能模块说明

> 访问地址：http://192.168.10.65:3080
> 技术栈：React 19 + TypeScript + TailwindCSS v4 + Recharts + Zustand
> 设计系统：Kinetic Observatory（深色/浅色双主题）
> 字体：Space Grotesk（标题）+ Inter（正文）+ Material Symbols Outlined（图标）
> 认证：JWT 登录鉴权，所有 API 受保护

---

## 一、页面总览

| 页面 | 路由 | 菜单名称 | 定位 |
|------|------|---------|------|
| 登录 | `/login` | — | JWT 登录页面，未认证自动跳转 |
| 仪表盘 | `/` | 仪表盘 | 全局总览中心：6 统计卡片 + 告警/RDS/到期摘要 + 分组服务器列表 + 端口摘要 + 资源排行 |
| 服务器列表 | `/servers` | 服务器 | 所有服务器详细视图，卡片/表格双模式，支持自定义分组 |
| 服务器详情 | `/servers/:id` | — | 单台服务器：实时概览 + 运行业务 + Docker 容器 + 历史趋势 |
| 数据库监控 | `/databases` | 数据库 | RDS 实例列表与实时指标 |
| 数据库详情 | `/databases/:id` | — | 单个数据库实例的实时指标瓦片 + 历史趋势图 |
| 容器管理 | `/containers` | 容器管理 | 全局 Docker 容器聚合列表，跨服务器查看/搜索/筛选 |
| 端口监控 | `/probes` | 端口监控 | TCP/HTTP/HTTPS 多协议探测规则管理与实时状态，SSL 证书到期徽章 |
| 本地业务 | `/assets` | 本地业务 | 服务器上部署的项目/服务资产管理 |
| 告警中心 | `/alerts` | 告警中心 | 告警事件管理 + 告警规则配置 + 通知渠道管理 |
| 资源到期 | `/billing` | 资源到期 | ECS/RDS/SSL 到期提醒，分类筛选（全部/ECS/RDS/SSL） |
| 系统信息 | `/settings` | 系统信息 | 系统版本 + 已注册 Agent 列表 |

---

## 二、认证系统

| 功能 | 说明 |
|------|------|
| 登录页 | 居中玻璃卡片，用户名 + 密码输入，渐变登录按钮 |
| JWT 鉴权 | 登录成功返回 JWT token（7 天有效期），持久化到 localStorage |
| 路由守卫 | `RequireAuth` 组件包裹所有受保护路由，未登录自动跳转 `/login` |
| Axios 拦截器 | 请求自动附加 `Authorization: Bearer` 头，401 响应自动跳转登录页 |
| 用户菜单 | 右上角头像图标，点击展开下拉菜单显示用户名 + 退出登录 |
| 主题切换 | 右上角太阳/月亮图标，点击切换深色/浅色主题 |

**API：**
- `POST /api/v1/auth/login` — 登录
- `GET /api/v1/auth/me` — 获取当前用户

---

## 三、各页面功能详解

### 3.1 仪表盘 (`/`)

全局总览中心，整合告警、资源到期、RDS、服务器分组等信息。

| 模块 | 位置 | 内容 |
|------|------|------|
| 统计卡片行 | 顶部 6 列 | 服务器在线数、运行中容器数、端口探测正常数、平均 CPU、**告警中（firing_unsilenced）**、**即将到期（30天内）** |
| 摘要区域 | 统计卡片下方 3 列 | **未处理告警**（最近 5 条未静默 firing 事件）、**数据库状态**（6 个 RDS 迷你 CPU/内存条）、**30天内到期**（ECS/RDS/SSL 到期资源列表） |
| 服务器状态列表 | 左列(7/12) | **按分组折叠展示**：每组组头（名称 + 在线/总数 + 折叠箭头），组内每台服务器一行（图标盒 + 主机名 + IP + CPU/MEM/DISK 进度条 + 网络速率） |
| 端口监控摘要 | 右列上(5/12) | 所有探测规则实时状态，15 秒自动刷新 |
| 资源使用排行 | 右列下 | Top 5 服务器，按 CPU 排序 |

**数据来源（并行 5 个 API + WebSocket）：**
- 服务器 + 分组 + 指标快照：`GET /api/v1/dashboard`（含 `groups` 和 `metrics` 字段）
- 告警统计：`GET /api/v1/alerts/stats`
- 告警事件：`GET /api/v1/alerts/events?status=firing&silenced=false&limit=5`
- RDS 状态：`GET /api/v1/databases`
- 到期信息：`GET /api/v1/billing`
- 实时更新：WebSocket（服务器指标 + `alert`/`alert_resolved`/`alert_acked` 消息）

---

### 3.2 服务器列表 (`/servers`)

所有服务器的详细视图，支持两种展示模式和自定义分组管理。

**分组展示（卡片和表格视图均支持）：**
- 按自定义分组折叠展示，组头：分组名 + (在线/总数) + 折叠箭头
- 未分组服务器归入"未分组"尾部
- 默认全部展开

**卡片视图（默认）：**
- 响应式网格（1-4 列自适应）
- 每张卡片：图标盒 + 主机名 + 状态标签 + IP + 硬件摘要标签 + 三条进度条 + 网络速率 + 分组选择器

**表格视图：**
- 紧凑行式：状态灯、主机名（可点击）、IP、系统、CPU%、内存%、磁盘%、流量、容器数、分组选择
- CPU/内存/磁盘百分比带颜色编码（绿/黄/红）

**分组管理：**
- 顶部"管理分组"按钮，点击展开管理面板
- 面板内：已有分组列表（可删除）+ 新建分组输入框
- 每个服务器卡片/表格行内有分组下拉选择器，切换即时生效

**底部统计栏：** 服务器总数/在线数、平均 CPU、总流量、运行容器数

**视图切换：** 分段控件（卡片/表格）

**分组 API：**
- `GET /api/v1/groups` — 列出所有分组（含 server_count）
- `POST /api/v1/groups` — 创建分组
- `PUT /api/v1/groups/:id` — 更新分组
- `DELETE /api/v1/groups/:id` — 删除分组（组内服务器解绑）
- `PUT /api/v1/servers/:id/group` — 设置服务器所属分组

---

### 3.3 服务器详情 (`/servers/:id`)

单台服务器的全维度监控视图。

| 模块 | 内容 |
|------|------|
| 头部 | 返回按钮 + 「服务器详情」标签 + 服务器名称（可编辑，点击铅笔图标） + 运行中/离线状态徽章 |
| Bento Grid 左列(1/3) | 服务器基本信息：OS、内核、CPU 型号、内存总量、磁盘总量、GPU（如有）、IP、心跳、Agent 版本、架构 |
| Bento Grid 右列(2/3) | **实时概览**：3x3 网格卡片 — CPU（含 load 1/5/15）、内存（含 swap）、磁盘、网络入站、网络出站、容器数。有 GPU 时追加 GPU 使用率/显存/温度 |
| 运行业务 | 右列下方，展示该服务器部署的所有业务项目：名称、描述、技术栈标签、路径、端口 |
| Docker 容器表格 | 容器名、状态（发光点）、CPU%、内存、镜像 |
| 历史趋势 | 时间范围切换（1h/6h/24h/7d）+ 刷新按钮。2 列网格展示 6 个历史图表：CPU 使用率、系统负载、内存使用率、磁盘使用率、网络流量合计、网络分网卡。有 GPU 时追加 3 个：GPU 使用率、GPU 显存、GPU 温度 |

**服务器名称编辑：** 点击标题旁铅笔图标，切换为输入框，Enter 保存 / Esc 取消。

**历史趋势特性：**
- 数据来自 VictoriaMetrics，通过 Nginx 反代 `/vm/api/v1/query_range`
- 父级统一计算时间窗口，所有图表 X 轴对齐
- AbortController 处理请求竞态（快速切换不会数据错乱）
- 自适应数值精度（小数值自动增加小数位数）
- 三态：加载中旋转动画 / 错误重试按钮 / 无数据提示

**数据来源：**
- 基本信息：`GET /api/v1/servers/:id`
- 实时指标：WebSocket
- 运行业务：`GET /api/v1/assets`（按 server_id 过滤）
- 历史趋势：`/vm/api/v1/query_range`（Nginx 代理 VictoriaMetrics）
- 名称修改：`PUT /api/v1/servers/:id/name`

---

### 3.4 数据库监控 (`/databases`)

RDS 云数据库实例监控。

| 功能 | 说明 |
|------|------|
| 实例列表 | 卡片式展示数据库实例，显示名称、类型（MySQL/PostgreSQL）、实时 CPU/内存/磁盘/连接数 |
| 实例详情 | `/databases/:id`，实时指标瓦片（8-10 个指标）+ 历史趋势图表，支持时间范围切换 |

**数据来源：**
- `GET /api/v1/databases` — 实例列表
- `GET /api/v1/databases/:id` — 实例详情

---

### 3.5 容器管理 (`/containers`)

全局 Docker 容器聚合列表，跨服务器查看所有容器。

| 功能 | 说明 |
|------|------|
| 统计卡片行 | 4 张卡片：总容器数、运行中、已停止、宿主机数 |
| 筛选 Tab | 全部 / 运行中 / 已停止，每项显示数量 |
| 搜索 | 按容器名、镜像、服务器名模糊搜索 |
| 容器表格 | 状态标签、容器名（含 ID）、镜像、宿主机（可点击跳转服务器详情）、CPU%、内存（使用/限制）、端口映射、运行状态 |
| 空状态 | 无容器时提示开启 Agent Docker 采集 |

**数据来源：**
- 服务器列表：`GET /api/v1/dashboard`
- 容器数据：WebSocket 实时推送 `MetricsPayload.containers`，聚合所有服务器

---

### 3.6 端口监控 (`/probes`)

多协议端口探测管理，支持 TCP / HTTP / HTTPS。

| 功能 | 说明 |
|------|------|
| 统计卡片行 | 4 张卡片：总探测任务数、正常运行数、异常告警数、平均响应延迟 |
| 探测卡片网格 | 每张卡片：服务名、地址（TCP 显示 host:port，HTTP/HTTPS 显示 URL）、状态、响应延迟 |
| **SSL 到期徽章** | HTTPS 探测卡片右上角显示证书剩余天数（>60天绿、30-60天黄、<30天红） |
| 异常卡片 | 红色边框标注 |
| **协议选择器** | 创建规则时选择 TCP / HTTP / HTTPS，不同协议显示不同表单字段 |
| TCP 表单 | 服务名 + 主机 IP + 端口 |
| HTTP/HTTPS 表单 | 服务名 + URL + 期望状态码（默认 200，0=不检查）+ 关键字匹配（可选） |
| 删除规则 | 卡片悬停显示删除按钮 |
| 自动刷新 | 每 10 秒拉取最新探测结果 |

**HTTP/HTTPS 探测特性：**
- URL 为唯一来源字段，host/port 由后端自动从 URL 解析
- HTTP 请求使用严格 TLS 验证（证书无效 → status=down）
- SSL 证书信息通过独立 TLS 握手采集（InsecureSkipVerify），即使证书过期/自签也能获取到期时间
- `expect_status=0` 时跳过状态码检查
- VictoriaMetrics 新指标：`mantisops_probe_http_status`、`mantisops_probe_ssl_days_left`

**数据来源：**
- 规则：`GET /api/v1/probes`
- 状态：`GET /api/v1/probes/status`（10 秒轮询）
- 操作：`POST`（创建）、`DELETE`（删除）

---

### 3.7 本地业务 (`/assets`)

服务器上部署的项目和服务信息管理。

| 功能 | 说明 |
|------|------|
| 按服务器分组 | 每组：服务器图标盒 + 名称 + IP 徽章 + 硬件摘要 |
| 资产表格 | 项目名称（含描述）、技术栈（彩色标签拆分）、路径（mono 字体）、端口 |
| 添加资产 | 渐变按钮展开玻璃卡片表单 |
| 删除 | 行悬停显示删除按钮，有确认弹窗 |
| 底部统计 | 活跃资产数、总计、服务器数 |

**数据来源：**
- `GET /api/v1/assets`、`POST`（创建）、`PUT`（更新）、`DELETE`（删除）

---

### 3.8 告警中心 (`/alerts`)

告警通知系统的管理中心，包含三个 Tab。

**统计卡片行：** 4 张卡片 — 当前触发中、今日触发、今日恢复、今日确认

**Tab 1 — 告警事件：**

| 功能 | 说明 |
|------|------|
| 状态筛选 | 全部 / 触发中 / 已恢复 / 已静默 |
| 事件表格 | 级别 emoji、告警名称（rule_name 快照）、目标（target_label 快照）、触发值、状态、触发时间、操作 |
| firing 行 | 左红边框 + 浅红背景，"确认"按钮 |
| firing + silenced 行 | 左橙边框，显示确认人和时间 |
| resolved 行 | 显示恢复时间 + 恢复原因（自动恢复/目标消失/规则禁用/规则删除） |
| 通知投递详情 | 点击行展开，查看各渠道投递状态（sent/failed/pending） |
| 自动刷新 | 15 秒轮询 |

**Tab 2 — 告警规则：**

| 功能 | 说明 |
|------|------|
| 规则列表 | 规则名、类型、目标、条件、连续次数、级别、启用开关、删除 |
| 添加规则 | 表单：名称、类型（10 种）、目标、运算符、阈值、连续次数、级别 |
| 规则类型 | server_offline(服务器离线)、probe_down(端口异常)、cpu、memory、disk、container(容器异常)、gpu_temp、gpu_memory、network_rx、network_tx |
| 动态生效 | 启用/禁用/删除规则时自动处理关联的 firing 事件 |

**Tab 3 — 通知渠道：**

| 功能 | 说明 |
|------|------|
| 渠道列表 | 卡片式：渠道名、类型图标、URL 脱敏、启用开关 |
| 添加渠道 | 表单：名称、类型（钉钉/Webhook）、URL、密钥 |
| 测试通知 | 每张卡片有"测试"按钮，发送测试消息验证配置 |

**告警引擎特性：**
- 后端 30 秒轮询评估所有 enabled 规则
- 连续 N 次超阈值才触发（防抖动），连续 N 次正常才恢复（对称）
- 同一规则未处理前不重复发送
- 手动确认 = 静默通知（不改变 firing 状态，等待自动恢复）
- 事件 + 通知记录在同一 SQLite 事务中原子落库
- 通知异步发送，带认领机制防重复，失败自动重试（最多 3 次）
- 恢复通知只发给触发时绑定的渠道集合
- 目标消失（容器删除/磁盘卸载/主机移除）自动 resolve，不发外部通知
- 指标新鲜度检查（>120s 的陈旧数据跳过，防误报）

**数据来源：**
- 规则：`GET /api/v1/alerts/rules`、`POST`、`PUT`、`DELETE`
- 事件：`GET /api/v1/alerts/events`（支持 status/silenced/since/until/limit/offset 分页）
- 统计：`GET /api/v1/alerts/stats`
- 确认：`PUT /api/v1/alerts/events/:id/ack`
- 投递详情：`GET /api/v1/alerts/events/:id/notifications`
- 渠道：`GET /api/v1/alerts/channels`、`POST`、`PUT`、`DELETE`
- 测试：`POST /api/v1/alerts/channels/:id/test`
- 实时推送：WebSocket `alert`/`alert_resolved`/`alert_acked` 消息

---

### 3.9 资源到期 (`/billing`)

ECS / RDS / SSL 证书到期提醒。

| 功能 | 说明 |
|------|------|
| 统计卡片行 | 5 张卡片：紧急(30天内)、预警(60天内)、ECS 实例数、RDS 实例数、SSL 证书数 |
| 紧急告警横幅 | 30 天内到期资源高亮提醒（红色） |
| **分类筛选 Tab** | 全部 / ECS / RDS / SSL，每项显示数量，点击切换过滤 |
| 到期表格 | 类型标签（ECS绿/RDS青/SSL黄）、名称（含 ID）、所属账号、规格/引擎、计费方式、到期日期、剩余天数 |
| 排序 | 有效资源按天数升序在前，已过期按天数降序在后 |
| 颜色编码 | ≤30天红色、30-60天黄色、>60天绿色、已过期红色负数 |

**SSL 证书数据来源：**
- 阿里云 CAS API（`ListUserCertificateOrder`），查询 CPACK + CERT 两种类型
- 按域名+到期日去重
- 过期超过 90 天的自动过滤不展示
- 与 ECS/RDS 一起在 BillingHandler 每小时缓存刷新

**数据来源：**
- `GET /api/v1/billing` — 返回 ECS + RDS + SSL 统一列表

---

### 3.10 系统信息 (`/settings`)

系统版本和 Agent 管理。

| 模块 | 内容 |
|------|------|
| 系统信息条 | 前端版本号、Agent 在线数/总数 |
| 已注册 Agent | 表格：主机名、Host ID（mono）、Agent 版本、最后心跳、在线/离线状态（发光点 + 文字标签） |

---

## 四、布局与导航

### 顶部导航栏（固定，响应式 left-0 md:left-[250px]）

| 元素 | 位置 | 功能 |
|------|------|------|
| 汉堡菜单 | 左（移动端） | 切换侧边栏 |
| 搜索框 | 左（桌面端） | 搜索服务器名/IP/host_id，下拉结果可点击跳转 |
| 刷新按钮 | 右 | sync 图标 |
| 通知铃铛 | 右 | NotificationBell 组件：红色徽章显示 firing_unsilenced 数，点击下拉显示最近 10 条告警，"查看全部"跳转 /alerts |
| 用户菜单 | 右 | 头像 + 用户名，下拉：用户信息 + 退出登录 |

### 侧边栏（固定，移动端可收起）

| 菜单项 | 图标 | 路由 |
|--------|------|------|
| 仪表盘 | dashboard | `/` |
| 服务器 | dns | `/servers` |
| 数据库 | database | `/databases` |
| 容器管理 | deployed_code | `/containers` |
| 端口监控 | sensors | `/probes` |
| 本地业务 | inventory_2 | `/assets` |
| 告警中心 | notifications_active | `/alerts` |
| 资源到期 | event_upcoming | `/billing` |
| 系统信息 | settings | `/settings` |

激活项样式：绿色文字高亮

---

## 五、通用组件

| 组件 | 文件 | 说明 |
|------|------|------|
| ProgressBar | `components/ProgressBar.tsx` | 进度条，色调分层（<60% 绿、<80% 黄、>=80% 红），支持 sm/md 尺寸 |
| StatusBadge | `components/StatusBadge.tsx` | 发光脉冲状态指示器，支持 online/offline/up/down + 文字标签 |
| ServerCard | `components/ServerCard.tsx` | 服务器卡片，含硬件标签、进度条、网络速率、容器数、GPU 徽章、分组选择器 |
| HistoryChart | `components/HistoryChart.tsx` | 历史趋势图表，查询 VictoriaMetrics，支持多线叠加、AbortController 竞态处理、三态 UI、自适应精度 |
| ThemeToggle | `components/ThemeToggle.tsx` | 深色/浅色分段控件 |
| NotificationBell | `components/NotificationBell.tsx` | 顶栏告警铃铛：红色徽章 + 下拉面板 + 新告警脉冲动画 |
| Sidebar | `components/Layout/Sidebar.tsx` | 左侧导航栏（深色），9 个导航项，移动端可收起 |
| MainLayout | `components/Layout/MainLayout.tsx` | 页面骨架：顶栏（Logo + 刷新 + 通知铃铛 + 主题切换 + 用户菜单）+ 侧边栏 + 内容区 |

---

## 六、实时数据机制

| 机制 | 说明 |
|------|------|
| WebSocket | 全局单连接（MainLayout 层），`/ws?token=<jwt>` 端点（鉴权），自动断线 3 秒重连，未登录不连接 |
| 消息格式 | `{"type": "metrics", "host_id": "xxx", "data": MetricsPayload}` |
| 告警消息 | `{"type": "alert", "data": AlertEvent}`、`{"type": "alert_resolved", "data": {"id": N}}`、`{"type": "alert_acked", "data": {"id": N, "acked_by": "admin"}}` |
| 状态管理 | Zustand serverStore（服务器/指标）+ alertStore（告警事件/统计） |
| 认证管理 | Zustand authStore 维护 `token` 和 `username`，持久化到 localStorage |
| 端口探测轮询 | 仪表盘 15 秒、端口监控页 10 秒轮询 `GET /api/v1/probes/status` |
| 历史数据 | 前端通过 `/vm/api/v1/query_range` 查询 VictoriaMetrics（Nginx 精确代理） |

---

## 七、设计系统 — Kinetic Observatory

### 色彩体系（深色主题 / 浅色主题）

通过 TailwindCSS v4 `@theme` 定义设计令牌，`<html class="dark">` 切换。

**深色（默认）：**
- 背景层级：`#0b1326` → `#131b2e` → `#171f33` → `#222a3d` → `#2d3449`
- 主色：`#a4c9ff`（Primary）、`#4edea3`（Tertiary/成功）、`#ffb4ab`（Error）、`#fbbf24`（Warning）
- 文字：`#dae2fd`（主）、`#c1c7d3`（副）

**浅色：**
- 完整浅色 token 覆盖，通过 `html:not(.dark)` CSS 规则

### 核心视觉特性
- 玻璃拟态（`glass-card`：rgba 背景 + backdrop-blur）
- 发光脉冲状态指示器（`pulse-glow-success/error`）
- 悬停卡片发光（`glow-card`）
- 网格背景（`bg-grid-pattern`）
- 无边框设计（色调层级区分层次）
- 渐变按钮（`bg-gradient-to-br from-primary to-primary-container`）

---

## 八、安全

| 层面 | 机制 |
|------|------|
| REST API | JWT Bearer token 鉴权（`Authorization: Bearer <token>`），所有 `/api/v1/*` 路由受 JWTMiddleware 保护 |
| WebSocket | 连接时通过 query param `?token=<jwt>` 鉴权，服务端校验后才允许 upgrade |
| gRPC Agent | PSK（Pre-Shared Key）拦截器，Agent 连接时携带 `Authorization: Bearer <psk_token>` |
| 启动校验 | 服务启动时校验 auth.username、auth.password、auth.jwt_secret、server.psk_token 均非空，空值拒绝启动 |
| 配置文件 | 仓库中 `configs/server.yaml` 为空值模板，实际密钥只存在于部署服务器的配置中，不入版本控制 |
| Nginx | `/vm/api/v1/query_range` 仅允许 GET 请求（`limit_except GET { deny all; }`），限制 VictoriaMetrics 访问范围 |

---

## 九、架构

```
浏览器 → Nginx (:3080)
            ├── /                          → 静态文件 (~/mantisops/web/dist/)
            ├── /api/*                     → Go Server (127.0.0.1:3100)
            ├── /ws                        → Go Server (WebSocket)
            └── /vm/api/v1/query_range     → VictoriaMetrics (127.0.0.1:8428)
```

---

## 十、当前接入服务器

| 服务器 | Host ID | IP | CPU | 内存 | 磁盘 | 特性 |
|--------|---------|-----|-----|------|------|------|
| yuanqing2 | srv-65-yuanqing2 | 192.168.10.65 | 8核 Xeon 4210 | 16GB | 193GB | Docker 容器、MantisOps Server |
| ai | srv-69-ai | 192.168.10.69 | 16核 i7-10700K | 64GB | 434GB | GPU 采集 (RTX 3090 24GB)、Ollama |
| zentao | srv-62-zentao | 192.168.10.62 | 4核 Xeon 4210 | 16GB | 46GB | Docker（权限受限） |
| sing-box | srv-63-singbox | 192.168.10.63 | 2核 Xeon 4210 | 4GB | 46GB | 代理网关 |
| 阿里云 ECS | aliyun-i-bp1... | 47.98.217.67 | 2核 Xeon Platinum | 8GB | 197GB | 云监控 API 采集 |
