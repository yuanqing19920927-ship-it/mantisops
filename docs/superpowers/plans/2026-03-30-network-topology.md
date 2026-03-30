# 网络拓扑探知 实现计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 MantisOps 中实现网络设备发现、SNMP 采集、拓扑可视化和连通性监控功能。

**Architecture:** Server 集中扫描模式。新增 `server/internal/network/` 包含 Scanner（ICMP）、SNMPProber、TopologyBuilder、ConnectivityMonitor 四个核心模块。数据存入 SQLite（3 张新表），前端新增 `/network` 页面（3 个 Tab）。

**Tech Stack:** Go（pro-bing + gosnmp + 内嵌 OUI）、React 19（D3.js 力导向图）、SQLite、WebSocket

---

## 文件结构

### 后端新增文件

| 文件 | 职责 |
|------|------|
| `server/internal/network/scanner.go` | ICMP ping sweep，发现活跃 IP |
| `server/internal/network/snmp.go` | SNMP v2c 探测 + 设备信息 + 邻居表读取 |
| `server/internal/network/topology.go` | 从 SNMP 邻居数据构建拓扑图（去重、方向归一化） |
| `server/internal/network/monitor.go` | 定时 ping 连通性监控 + 状态变更通知 |
| `server/internal/network/oui.go` | MAC → 厂商 OUI 查询（内嵌数据） |
| `server/internal/network/oui_data.go` | OUI JSON 数据（go:embed） |
| `server/internal/store/network_store.go` | SQLite CRUD（3 张表：subnets、devices、links） |
| `server/internal/api/network_handler.go` | HTTP API 处理器 |

### 后端修改文件

| 文件 | 修改内容 |
|------|---------|
| `server/internal/store/sqlite.go` | 添加 3 张表的 DDL + migrateV3 |
| `server/internal/config/config.go` | 添加 NetworkConfig 结构体 + 默认值 |
| `server/internal/api/router.go` | 添加 NetworkHandler 到 RouterDeps + 注册路由 |
| `server/cmd/server/main.go` | 初始化 network 模块、启动 Monitor |
| `server/internal/alert/evaluator.go` | 添加 network_device_offline 告警类型 |
| `server/go.mod` / `server/go.sum` | 添加 pro-bing + gosnmp 依赖 |
| `server/configs/server.yaml` | 添加 network 配置段 |

### 前端新增文件

| 文件 | 职责 |
|------|------|
| `web/src/api/network.ts` | 网络拓扑 API 客户端 |
| `web/src/stores/networkStore.ts` | Zustand 状态管理 |
| `web/src/pages/Network/index.tsx` | 网络拓扑主页面（3 Tab） |
| `web/src/pages/Network/TopologyGraph.tsx` | D3.js 力导向拓扑图组件 |
| `web/src/pages/Network/DeviceList.tsx` | 设备列表表格 |
| `web/src/pages/Network/SubnetOverview.tsx` | 网段概览卡片 |
| `web/src/pages/Network/ScanDialog.tsx` | 扫描确认弹窗 |

### 前端修改文件

| 文件 | 修改内容 |
|------|---------|
| `web/src/App.tsx` | 添加 `/network` 路由 |
| `web/src/components/Layout/Sidebar.tsx` | 添加「网络拓扑」菜单项 |
| `web/src/hooks/useWebSocket.ts` | 处理 network_* WebSocket 事件 |

---

## Chunk 1: 数据层（Store + Migration + Config）

### Task 1: 添加 Go 依赖

**Files:**
- Modify: `server/go.mod`

- [ ] **Step 1: 添加 pro-bing 和 gosnmp 依赖**

```bash
cd server
go get github.com/prometheus-community/pro-bing@latest
go get github.com/gosnmp/gosnmp@latest
```

- [ ] **Step 2: 验证编译**

Run: `go build ./cmd/server/`
Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add pro-bing and gosnmp for network topology"
```

### Task 2: 添加 NetworkConfig 配置结构

**Files:**
- Modify: `server/internal/config/config.go`
- Modify: `server/configs/server.yaml`

- [ ] **Step 1: 在 config.go 添加 NetworkConfig 结构体**

在 `AIConfig` 结构体之后添加：

```go
type NetworkConfig struct {
	Enabled         bool     `yaml:"enabled"`
	MonitorInterval int      `yaml:"monitor_interval"`
	SNMPInterval    int      `yaml:"snmp_interval"`
	SNMPCommunities []string `yaml:"snmp_communities"`
	SNMPCollect     struct {
		Concurrency int `yaml:"concurrency"`
		IntervalMs  int `yaml:"interval_ms"`
	} `yaml:"snmp_collect"`
	Scan struct {
		ICMPConcurrency int    `yaml:"icmp_concurrency"`
		ICMPIntervalMs  int    `yaml:"icmp_interval_ms"`
		SNMPConcurrency int    `yaml:"snmp_concurrency"`
		SNMPTimeoutMs   int    `yaml:"snmp_timeout_ms"`
		ICMPTimeoutMs   int    `yaml:"icmp_timeout_ms"`
		Schedule        string `yaml:"schedule"`
		ScheduleSubnets []string `yaml:"schedule_subnets"`
	} `yaml:"scan"`
}
```

在 `Config` 结构体中添加字段：

```go
Network NetworkConfig `yaml:"network"`
```

- [ ] **Step 2: 在 Load() 函数中添加默认值**

在 AI 默认值之后添加：

```go
// Network defaults
if cfg.Network.MonitorInterval <= 0 {
	cfg.Network.MonitorInterval = 60
}
if cfg.Network.SNMPInterval <= 0 {
	cfg.Network.SNMPInterval = 300
}
if len(cfg.Network.SNMPCommunities) == 0 {
	cfg.Network.SNMPCommunities = []string{"public", "private"}
}
if cfg.Network.SNMPCollect.Concurrency <= 0 {
	cfg.Network.SNMPCollect.Concurrency = 3
}
if cfg.Network.SNMPCollect.IntervalMs <= 0 {
	cfg.Network.SNMPCollect.IntervalMs = 100
}
if cfg.Network.Scan.ICMPConcurrency <= 0 {
	cfg.Network.Scan.ICMPConcurrency = 10
}
if cfg.Network.Scan.ICMPIntervalMs <= 0 {
	cfg.Network.Scan.ICMPIntervalMs = 10
}
if cfg.Network.Scan.SNMPConcurrency <= 0 {
	cfg.Network.Scan.SNMPConcurrency = 5
}
if cfg.Network.Scan.SNMPTimeoutMs <= 0 {
	cfg.Network.Scan.SNMPTimeoutMs = 2000
}
if cfg.Network.Scan.ICMPTimeoutMs <= 0 {
	cfg.Network.Scan.ICMPTimeoutMs = 1000
}
// Hard cap enforcement
if cfg.Network.Scan.ICMPConcurrency > 10 {
	cfg.Network.Scan.ICMPConcurrency = 10
}
if cfg.Network.Scan.ICMPIntervalMs < 10 {
	cfg.Network.Scan.ICMPIntervalMs = 10
}
if cfg.Network.Scan.SNMPConcurrency > 5 {
	cfg.Network.Scan.SNMPConcurrency = 5
}
if cfg.Network.SNMPCollect.Concurrency > 3 {
	cfg.Network.SNMPCollect.Concurrency = 3
}
if cfg.Network.SNMPCollect.IntervalMs < 100 {
	cfg.Network.SNMPCollect.IntervalMs = 100
}
```

- [ ] **Step 3: 更新 server.yaml 模板**

在 `logging:` 段之后追加：

```yaml
network:
  enabled: false
  monitor_interval: 60
  snmp_interval: 300
  snmp_communities:
    - "public"
    - "private"
  snmp_collect:
    concurrency: 3
    interval_ms: 100
  scan:
    icmp_concurrency: 10
    icmp_interval_ms: 10
    snmp_concurrency: 5
    snmp_timeout_ms: 2000
    icmp_timeout_ms: 1000
    schedule: ""
    schedule_subnets: []
```

- [ ] **Step 4: 验证编译**

Run: `go build ./cmd/server/`
Expected: 编译成功

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go configs/server.yaml
git commit -m "feat(network): add NetworkConfig with hard cap enforcement"
```

### Task 3: 添加 SQLite 表和 NetworkStore

**Files:**
- Modify: `server/internal/store/sqlite.go`
- Create: `server/internal/store/network_store.go`

- [ ] **Step 1: 在 sqlite.go 的 migrate() 函数中添加 3 张表 DDL**

在已有 `CREATE TABLE` 语句列表末尾追加：

```go
// Network topology tables
`CREATE TABLE IF NOT EXISTS network_subnets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	cidr TEXT NOT NULL UNIQUE,
	name TEXT DEFAULT '',
	gateway TEXT DEFAULT '',
	total_hosts INTEGER DEFAULT 0,
	alive_hosts INTEGER DEFAULT 0,
	last_scan DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
`CREATE TABLE IF NOT EXISTS network_devices (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	ip TEXT NOT NULL,
	mac TEXT DEFAULT '',
	vendor TEXT DEFAULT '',
	device_type TEXT DEFAULT 'unknown',
	hostname TEXT DEFAULT '',
	snmp_supported BOOLEAN DEFAULT FALSE,
	snmp_credential_id INTEGER DEFAULT 0,
	sys_descr TEXT DEFAULT '',
	sys_name TEXT DEFAULT '',
	sys_object_id TEXT DEFAULT '',
	model TEXT DEFAULT '',
	subnet_id INTEGER REFERENCES network_subnets(id),
	status TEXT DEFAULT 'online',
	last_seen DATETIME,
	first_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
	server_id INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
`CREATE UNIQUE INDEX IF NOT EXISTS idx_network_devices_ip ON network_devices(ip)`,
`CREATE TABLE IF NOT EXISTS network_links (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	source_id INTEGER REFERENCES network_devices(id) ON DELETE CASCADE,
	target_id INTEGER REFERENCES network_devices(id) ON DELETE CASCADE,
	source_port TEXT DEFAULT '',
	target_port TEXT DEFAULT '',
	protocol TEXT DEFAULT 'lldp',
	bandwidth TEXT DEFAULT '',
	last_seen DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(source_id, target_id, source_port)
)`,
```

- [ ] **Step 2: 创建 network_store.go**

```go
package store

import (
	"database/sql"
	"time"
)

// --- Models ---

type NetworkSubnet struct {
	ID         int       `json:"id"`
	CIDR       string    `json:"cidr"`
	Name       string    `json:"name"`
	Gateway    string    `json:"gateway"`
	TotalHosts int       `json:"total_hosts"`
	AliveHosts int       `json:"alive_hosts"`
	LastScan   *string   `json:"last_scan"`
	CreatedAt  time.Time `json:"created_at"`
}

type NetworkDevice struct {
	ID               int     `json:"id"`
	IP               string  `json:"ip"`
	MAC              string  `json:"mac"`
	Vendor           string  `json:"vendor"`
	DeviceType       string  `json:"device_type"`
	Hostname         string  `json:"hostname"`
	SNMPSupported    bool    `json:"snmp_supported"`
	SNMPCredentialID int     `json:"snmp_credential_id"`
	SysDescr         string  `json:"sys_descr"`
	SysName          string  `json:"sys_name"`
	SysObjectID      string  `json:"sys_object_id"`
	Model            string  `json:"model"`
	SubnetID         *int    `json:"subnet_id"`
	Status           string  `json:"status"`
	LastSeen         *string `json:"last_seen"`
	FirstSeen        string  `json:"first_seen"`
	ServerID         int     `json:"server_id"`
	CreatedAt        string  `json:"created_at"`
}

type NetworkLink struct {
	ID         int    `json:"id"`
	SourceID   int    `json:"source_id"`
	TargetID   int    `json:"target_id"`
	SourcePort string `json:"source_port"`
	TargetPort string `json:"target_port"`
	Protocol   string `json:"protocol"`
	Bandwidth  string `json:"bandwidth"`
	LastSeen   string `json:"last_seen"`
}

// --- Store ---

type NetworkStore struct {
	db *sql.DB
}

func NewNetworkStore(db *sql.DB) *NetworkStore {
	return &NetworkStore{db: db}
}

// --- Subnet CRUD ---

func (s *NetworkStore) ListSubnets() ([]NetworkSubnet, error) {
	rows, err := s.db.Query(`SELECT id, cidr, name, gateway, total_hosts, alive_hosts, last_scan, created_at FROM network_subnets ORDER BY cidr`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []NetworkSubnet
	for rows.Next() {
		var sub NetworkSubnet
		if err := rows.Scan(&sub.ID, &sub.CIDR, &sub.Name, &sub.Gateway, &sub.TotalHosts, &sub.AliveHosts, &sub.LastScan, &sub.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, sub)
	}
	return list, nil
}

func (s *NetworkStore) UpsertSubnet(cidr, name, gateway string, totalHosts, aliveHosts int) (int, error) {
	now := time.Now().Format(time.RFC3339)
	res, err := s.db.Exec(`INSERT INTO network_subnets (cidr, name, gateway, total_hosts, alive_hosts, last_scan) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(cidr) DO UPDATE SET name=excluded.name, gateway=excluded.gateway, total_hosts=excluded.total_hosts, alive_hosts=excluded.alive_hosts, last_scan=excluded.last_scan`,
		cidr, name, gateway, totalHosts, aliveHosts, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		s.db.QueryRow(`SELECT id FROM network_subnets WHERE cidr=?`, cidr).Scan(&id)
	}
	return int(id), nil
}

// --- Device CRUD ---

func (s *NetworkStore) ListDevices(subnetID int) ([]NetworkDevice, error) {
	q := `SELECT id, ip, mac, vendor, device_type, hostname, snmp_supported, snmp_credential_id, sys_descr, sys_name, sys_object_id, model, subnet_id, status, last_seen, first_seen, server_id, created_at FROM network_devices`
	var args []interface{}
	if subnetID > 0 {
		q += ` WHERE subnet_id=?`
		args = append(args, subnetID)
	}
	q += ` ORDER BY ip`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []NetworkDevice
	for rows.Next() {
		var d NetworkDevice
		if err := rows.Scan(&d.ID, &d.IP, &d.MAC, &d.Vendor, &d.DeviceType, &d.Hostname, &d.SNMPSupported, &d.SNMPCredentialID, &d.SysDescr, &d.SysName, &d.SysObjectID, &d.Model, &d.SubnetID, &d.Status, &d.LastSeen, &d.FirstSeen, &d.ServerID, &d.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, nil
}

func (s *NetworkStore) GetDevice(id int) (*NetworkDevice, error) {
	var d NetworkDevice
	err := s.db.QueryRow(`SELECT id, ip, mac, vendor, device_type, hostname, snmp_supported, snmp_credential_id, sys_descr, sys_name, sys_object_id, model, subnet_id, status, last_seen, first_seen, server_id, created_at FROM network_devices WHERE id=?`, id).
		Scan(&d.ID, &d.IP, &d.MAC, &d.Vendor, &d.DeviceType, &d.Hostname, &d.SNMPSupported, &d.SNMPCredentialID, &d.SysDescr, &d.SysName, &d.SysObjectID, &d.Model, &d.SubnetID, &d.Status, &d.LastSeen, &d.FirstSeen, &d.ServerID, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *NetworkStore) UpsertDevice(ip, mac, vendor, deviceType, hostname string, snmpSupported bool, snmpCredID int, sysDescr, sysName, sysObjID, model string, subnetID, serverID int) (int, error) {
	now := time.Now().Format(time.RFC3339)
	res, err := s.db.Exec(`INSERT INTO network_devices (ip, mac, vendor, device_type, hostname, snmp_supported, snmp_credential_id, sys_descr, sys_name, sys_object_id, model, subnet_id, status, last_seen, server_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'online', ?, ?)
		ON CONFLICT(ip) DO UPDATE SET mac=excluded.mac, vendor=excluded.vendor, device_type=CASE WHEN excluded.device_type='unknown' THEN network_devices.device_type ELSE excluded.device_type END, hostname=excluded.hostname, snmp_supported=excluded.snmp_supported, snmp_credential_id=CASE WHEN excluded.snmp_credential_id>0 THEN excluded.snmp_credential_id ELSE network_devices.snmp_credential_id END, sys_descr=excluded.sys_descr, sys_name=excluded.sys_name, sys_object_id=excluded.sys_object_id, model=excluded.model, subnet_id=excluded.subnet_id, status='online', last_seen=excluded.last_seen, server_id=CASE WHEN excluded.server_id>0 THEN excluded.server_id ELSE network_devices.server_id END`,
		ip, mac, vendor, deviceType, hostname, snmpSupported, snmpCredID, sysDescr, sysName, sysObjID, model, subnetID, now, serverID)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		s.db.QueryRow(`SELECT id FROM network_devices WHERE ip=?`, ip).Scan(&id)
	}
	return int(id), nil
}

func (s *NetworkStore) UpdateDevice(id int, deviceType, hostname string) error {
	_, err := s.db.Exec(`UPDATE network_devices SET device_type=?, hostname=? WHERE id=?`, deviceType, hostname, id)
	return err
}

func (s *NetworkStore) UpdateDeviceStatus(id int, status string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE network_devices SET status=?, last_seen=? WHERE id=?`, status, now, id)
	return err
}

func (s *NetworkStore) DeleteDevice(id int) error {
	_, err := s.db.Exec(`DELETE FROM network_devices WHERE id=?`, id)
	return err
}

func (s *NetworkStore) GetAllOnlineDevices() ([]NetworkDevice, error) {
	return s.listDevicesByStatus("online")
}

func (s *NetworkStore) GetAllDevices() ([]NetworkDevice, error) {
	return s.ListDevices(0)
}

func (s *NetworkStore) listDevicesByStatus(status string) ([]NetworkDevice, error) {
	rows, err := s.db.Query(`SELECT id, ip, mac, vendor, device_type, hostname, snmp_supported, snmp_credential_id, sys_descr, sys_name, sys_object_id, model, subnet_id, status, last_seen, first_seen, server_id, created_at FROM network_devices WHERE status=?`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []NetworkDevice
	for rows.Next() {
		var d NetworkDevice
		if err := rows.Scan(&d.ID, &d.IP, &d.MAC, &d.Vendor, &d.DeviceType, &d.Hostname, &d.SNMPSupported, &d.SNMPCredentialID, &d.SysDescr, &d.SysName, &d.SysObjectID, &d.Model, &d.SubnetID, &d.Status, &d.LastSeen, &d.FirstSeen, &d.ServerID, &d.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, nil
}

// --- Link CRUD ---

func (s *NetworkStore) UpsertLink(sourceID, targetID int, sourcePort, targetPort, protocol, bandwidth string) error {
	// Ensure source_id < target_id for dedup
	if sourceID > targetID {
		sourceID, targetID = targetID, sourceID
		sourcePort, targetPort = targetPort, sourcePort
	}
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO network_links (source_id, target_id, source_port, target_port, protocol, bandwidth, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id, target_id, source_port) DO UPDATE SET target_port=excluded.target_port, bandwidth=excluded.bandwidth, last_seen=excluded.last_seen`,
		sourceID, targetID, sourcePort, targetPort, protocol, bandwidth, now)
	return err
}

func (s *NetworkStore) ListLinks() ([]NetworkLink, error) {
	rows, err := s.db.Query(`SELECT id, source_id, target_id, source_port, target_port, protocol, bandwidth, last_seen FROM network_links ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []NetworkLink
	for rows.Next() {
		var l NetworkLink
		if err := rows.Scan(&l.ID, &l.SourceID, &l.TargetID, &l.SourcePort, &l.TargetPort, &l.Protocol, &l.Bandwidth, &l.LastSeen); err != nil {
			return nil, err
		}
		list = append(list, l)
	}
	return list, nil
}

// --- Topology API helper ---

type TopologyData struct {
	Nodes []NetworkDevice `json:"nodes"`
	Edges []NetworkLink   `json:"edges"`
}

func (s *NetworkStore) GetTopology() (*TopologyData, error) {
	devices, err := s.GetAllDevices()
	if err != nil {
		return nil, err
	}
	links, err := s.ListLinks()
	if err != nil {
		return nil, err
	}
	if devices == nil {
		devices = []NetworkDevice{}
	}
	if links == nil {
		links = []NetworkLink{}
	}
	return &TopologyData{Nodes: devices, Edges: links}, nil
}
```

- [ ] **Step 3: 验证编译**

Run: `go build ./cmd/server/`
Expected: 编译成功

- [ ] **Step 4: Commit**

```bash
git add internal/store/sqlite.go internal/store/network_store.go
git commit -m "feat(network): add network tables and NetworkStore CRUD"
```

---

## Chunk 2: 核心扫描引擎（Scanner + SNMP + OUI + Monitor）

### Task 4: OUI 厂商查询模块

**Files:**
- Create: `server/internal/network/oui.go`

- [ ] **Step 1: 创建 OUI 查询模块**

使用内嵌的精简 OUI 前缀 map（覆盖主流网络设备厂商），不需要完整的 IEEE 数据库：

```go
package network

// LookupVendor returns vendor name from MAC address prefix (first 3 bytes)
func LookupVendor(mac string) string {
	// 标准化 MAC 前缀为 XX:XX:XX 格式
	prefix := normalizeMACPrefix(mac)
	if v, ok := ouiDB[prefix]; ok {
		return v
	}
	return ""
}

func normalizeMACPrefix(mac string) string {
	// 处理 XX:XX:XX:XX:XX:XX, XX-XX-XX-XX-XX-XX, XXXXXXXXXXXX 格式
	// 返回大写 "XX:XX:XX"
	// ... 实现 ...
}

// ouiDB 精简版 OUI 数据库（主流网络设备厂商约 500 条）
var ouiDB = map[string]string{
	"00:00:0C": "Cisco",
	"00:1A:A1": "Cisco",
	"00:E0:FC": "Huawei",
	"48:46:FB": "Huawei",
	"3C:8C:40": "H3C",
	"00:23:89": "H3C",
	"00:1E:58": "D-Link",
	"00:26:5A": "D-Link",
	"AC:A3:1E": "Aruba",
	"24:DE:C6": "Aruba",
	"B4:A9:FC": "TP-Link",
	"50:C7:BF": "TP-Link",
	"00:0C:29": "VMware",
	"00:50:56": "VMware",
	"08:00:27": "VirtualBox",
	"52:54:00": "QEMU/KVM",
	// ... 更多条目在实现时从 IEEE OUI 精简导入 ...
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./cmd/server/`

- [ ] **Step 3: Commit**

```bash
git add internal/network/oui.go
git commit -m "feat(network): add OUI vendor lookup module"
```

### Task 5: ICMP Scanner 模块

**Files:**
- Create: `server/internal/network/scanner.go`

- [ ] **Step 1: 创建 Scanner**

```go
package network

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	probing "github.com/prometheus-community/pro-bing"

	"mantisops/server/internal/config"
	"mantisops/server/internal/ws"
)

type ScanResult struct {
	IP   string
	MAC  string // 可能为空（跨网段无法获取 ARP）
	Alive bool
}

type ScanJob struct {
	Status        string   `json:"status"` // idle, scanning, completed, cancelled, failed
	CurrentSubnet string   `json:"current_subnet"`
	Progress      float64  `json:"progress"`
	StartedAt     *string  `json:"started_at"`
	Error         string   `json:"error"`
	cancel        context.CancelFunc
}

type Scanner struct {
	cfg  config.NetworkConfig
	hub  *ws.Hub
	mu   sync.Mutex
	job  ScanJob
}

func NewScanner(cfg config.NetworkConfig, hub *ws.Hub) *Scanner {
	return &Scanner{cfg: cfg, hub: hub, job: ScanJob{Status: "idle"}}
}

func (s *Scanner) GetStatus() ScanJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.job
}

func (s *Scanner) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job.cancel != nil {
		s.job.cancel()
		s.job.Status = "cancelled"
	}
}

func (s *Scanner) StartScan(ctx context.Context, subnets []string) error {
	s.mu.Lock()
	if s.job.Status == "scanning" {
		s.mu.Unlock()
		return fmt.Errorf("scan already in progress")
	}
	scanCtx, cancel := context.WithCancel(ctx)
	now := time.Now().Format(time.RFC3339)
	s.job = ScanJob{Status: "scanning", StartedAt: &now, cancel: cancel}
	s.mu.Unlock()

	go s.runScan(scanCtx, subnets)
	return nil
}

func (s *Scanner) runScan(ctx context.Context, subnets []string) {
	defer func() {
		s.mu.Lock()
		if s.job.Status == "scanning" {
			s.job.Status = "completed"
		}
		s.mu.Unlock()
		// 5 分钟后回到 idle
		time.AfterFunc(5*time.Minute, func() {
			s.mu.Lock()
			if s.job.Status != "scanning" {
				s.job = ScanJob{Status: "idle"}
			}
			s.mu.Unlock()
		})
	}()

	totalDevices := 0
	for _, subnet := range subnets {
		if ctx.Err() != nil {
			break
		}
		s.mu.Lock()
		s.job.CurrentSubnet = subnet
		s.mu.Unlock()

		results := s.scanSubnet(ctx, subnet)
		totalDevices += len(results)
	}

	// Broadcast job done
	if s.hub != nil {
		s.hub.BroadcastAdmin(map[string]interface{}{
			"type": "network_scan_job_done",
			"data": map[string]interface{}{
				"total_subnets":  len(subnets),
				"total_devices":  totalDevices,
				"status":         s.GetStatus().Status,
			},
		})
	}
}

func (s *Scanner) scanSubnet(ctx context.Context, cidr string) []ScanResult {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		log.Printf("[network] invalid CIDR: %s: %v", cidr, err)
		return nil
	}

	ips := expandCIDR(ipNet)
	total := int64(len(ips))
	var done, found int64
	var results []ScanResult
	var mu sync.Mutex

	sem := make(chan struct{}, s.cfg.Scan.ICMPConcurrency)
	var wg sync.WaitGroup

	for _, ip := range ips {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}

			alive := s.pingHost(ctx, target)
			atomic.AddInt64(&done, 1)
			if alive {
				atomic.AddInt64(&found, 1)
				mu.Lock()
				results = append(results, ScanResult{IP: target, Alive: true})
				mu.Unlock()
			}

			// Rate limit
			time.Sleep(time.Duration(s.cfg.Scan.ICMPIntervalMs) * time.Millisecond)

			// Progress broadcast every 10 hosts
			if atomic.LoadInt64(&done)%10 == 0 && s.hub != nil {
				s.hub.BroadcastAdmin(map[string]interface{}{
					"type": "network_scan_progress",
					"data": map[string]interface{}{
						"subnet":  cidr,
						"total":   total,
						"scanned": atomic.LoadInt64(&done),
						"found":   atomic.LoadInt64(&found),
					},
				})
			}
		}(ip)
	}
	wg.Wait()

	// Subnet done broadcast
	if s.hub != nil {
		s.hub.BroadcastAdmin(map[string]interface{}{
			"type": "network_scan_subnet_done",
			"data": map[string]interface{}{
				"subnet":        cidr,
				"devices_found": found,
			},
		})
	}

	return results
}

func (s *Scanner) pingHost(ctx context.Context, ip string) bool {
	pinger, err := probing.NewPinger(ip)
	if err != nil {
		return false
	}
	pinger.Count = 1
	pinger.Timeout = time.Duration(s.cfg.Scan.ICMPTimeoutMs) * time.Millisecond
	pinger.SetPrivileged(true) // requires CAP_NET_RAW

	done := make(chan error, 1)
	go func() { done <- pinger.Run() }()

	select {
	case <-ctx.Done():
		pinger.Stop()
		return false
	case err := <-done:
		if err != nil {
			return false
		}
		return pinger.Statistics().PacketsRecv > 0
	}
}

// expandCIDR returns all host IPs in the given network (excluding network and broadcast addresses)
func expandCIDR(ipNet *net.IPNet) []string {
	var ips []string
	ip := ipNet.IP.Mask(ipNet.Mask)
	for inc(ip); ipNet.Contains(ip); inc(ip) {
		// Skip broadcast
		if isBroadcast(ip, ipNet) {
			continue
		}
		ips = append(ips, ip.String())
	}
	return ips
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func isBroadcast(ip net.IP, ipNet *net.IPNet) bool {
	for i := range ip {
		if ip[i] != (ipNet.IP[i] | ^ipNet.Mask[i]) {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./cmd/server/`

- [ ] **Step 3: Commit**

```bash
git add internal/network/scanner.go
git commit -m "feat(network): add ICMP scanner with rate limiting and cancellation"
```

### Task 6: SNMP 探测模块

**Files:**
- Create: `server/internal/network/snmp.go`

- [ ] **Step 1: 创建 SNMPProber**

```go
package network

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gosnmp/gosnmp"
)

type SNMPResult struct {
	Supported   bool
	Community   string
	SysDescr    string
	SysName     string
	SysObjectID string
	Model       string
	DeviceType  string // switch/router/ap/firewall/unknown
}

type LLDPNeighbor struct {
	LocalPort  string
	RemoteIP   string
	RemotePort string
	RemoteName string
}

type SNMPProber struct {
	communities []string
	timeoutMs   int
}

func NewSNMPProber(communities []string, timeoutMs int) *SNMPProber {
	return &SNMPProber{communities: communities, timeoutMs: timeoutMs}
}

// Probe tests SNMP connectivity and reads system info
func (p *SNMPProber) Probe(ctx context.Context, ip string) *SNMPResult {
	for _, community := range p.communities {
		if ctx.Err() != nil {
			return nil
		}
		result := p.tryProbe(ip, community)
		if result != nil && result.Supported {
			return result
		}
	}
	return &SNMPResult{Supported: false}
}

func (p *SNMPProber) tryProbe(ip, community string) *SNMPResult {
	snmp := &gosnmp.GoSNMP{
		Target:    ip,
		Port:      161,
		Community: community,
		Version:   gosnmp.Version2c,
		Timeout:   time.Duration(p.timeoutMs) * time.Millisecond,
		Retries:   0,
	}
	if err := snmp.Connect(); err != nil {
		return nil
	}
	defer snmp.Conn.Close()

	// Read sysDescr, sysName, sysObjectID
	oids := []string{
		"1.3.6.1.2.1.1.1.0", // sysDescr
		"1.3.6.1.2.1.1.5.0", // sysName
		"1.3.6.1.2.1.1.2.0", // sysObjectID
	}
	res, err := snmp.Get(oids)
	if err != nil {
		return nil
	}

	result := &SNMPResult{Supported: true, Community: community}
	for _, v := range res.Variables {
		switch v.Name {
		case ".1.3.6.1.2.1.1.1.0":
			result.SysDescr = fmt.Sprintf("%s", v.Value)
		case ".1.3.6.1.2.1.1.5.0":
			result.SysName = fmt.Sprintf("%s", v.Value)
		case ".1.3.6.1.2.1.1.2.0":
			result.SysObjectID = fmt.Sprintf("%s", v.Value)
		}
	}

	result.DeviceType = inferDeviceType(result.SysDescr, result.SysObjectID)
	result.Model = inferModel(result.SysDescr)

	return result
}

// GetLLDPNeighbors reads LLDP neighbor table via SNMP
func (p *SNMPProber) GetLLDPNeighbors(ip, community string) []LLDPNeighbor {
	snmp := &gosnmp.GoSNMP{
		Target:    ip,
		Port:      161,
		Community: community,
		Version:   gosnmp.Version2c,
		Timeout:   time.Duration(p.timeoutMs) * time.Millisecond,
		Retries:   0,
	}
	if err := snmp.Connect(); err != nil {
		return nil
	}
	defer snmp.Conn.Close()

	// LLDP remote system name: 1.0.8802.1.1.2.1.4.1.1.9
	// LLDP remote port desc:   1.0.8802.1.1.2.1.4.1.1.8
	// LLDP remote management addr: 1.0.8802.1.1.2.1.4.2.1.4
	// 简化实现：walk LLDP MIB 获取邻居
	results, err := snmp.BulkWalkAll("1.0.8802.1.1.2.1.4.1.1.9")
	if err != nil {
		log.Printf("[network-snmp] LLDP walk failed for %s: %v", ip, err)
		return nil
	}

	var neighbors []LLDPNeighbor
	for _, r := range results {
		name := fmt.Sprintf("%s", r.Value)
		if name != "" {
			neighbors = append(neighbors, LLDPNeighbor{RemoteName: name})
		}
	}
	return neighbors
}

func inferDeviceType(sysDescr, sysObjID string) string {
	// 基于 sysDescr 关键词推断设备类型
	// Cisco IOS → router/switch, Huawei VRP → switch, etc.
	// 简单实现，后续可扩展
	desc := strings.ToLower(sysDescr)
	switch {
	case strings.Contains(desc, "router") || strings.Contains(desc, "ios"):
		return "router"
	case strings.Contains(desc, "switch") || strings.Contains(desc, "s5700") || strings.Contains(desc, "catalyst"):
		return "switch"
	case strings.Contains(desc, "access point") || strings.Contains(desc, "ap "):
		return "ap"
	case strings.Contains(desc, "firewall") || strings.Contains(desc, "fortigate") || strings.Contains(desc, "asa"):
		return "firewall"
	case strings.Contains(desc, "printer") || strings.Contains(desc, "jetdirect"):
		return "printer"
	default:
		return "unknown"
	}
}

func inferModel(sysDescr string) string {
	// 从 sysDescr 提取设备型号（取第一行或前 80 字符）
	if len(sysDescr) > 80 {
		return sysDescr[:80]
	}
	return sysDescr
}
```

注意需要在文件顶部添加 `"strings"` import。

- [ ] **Step 2: 验证编译**

Run: `go build ./cmd/server/`

- [ ] **Step 3: Commit**

```bash
git add internal/network/snmp.go
git commit -m "feat(network): add SNMP prober with device type inference and LLDP neighbor read"
```

### Task 7: ConnectivityMonitor 模块

**Files:**
- Create: `server/internal/network/monitor.go`

- [ ] **Step 1: 创建 ConnectivityMonitor**

```go
package network

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	probing "github.com/prometheus-community/pro-bing"

	"mantisops/server/internal/config"
	"mantisops/server/internal/store"
	"mantisops/server/internal/ws"
)

type ConnectivityMonitor struct {
	cfg         config.NetworkConfig
	store       *store.NetworkStore
	hub         *ws.Hub
	failCounts  sync.Map // ip -> int32 (consecutive failures)
	cancel      context.CancelFunc
}

func NewConnectivityMonitor(cfg config.NetworkConfig, ns *store.NetworkStore, hub *ws.Hub) *ConnectivityMonitor {
	return &ConnectivityMonitor{cfg: cfg, store: ns, hub: hub}
}

func (m *ConnectivityMonitor) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go m.run(ctx)
	log.Printf("[network-monitor] started, interval=%ds", m.cfg.MonitorInterval)
}

func (m *ConnectivityMonitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *ConnectivityMonitor) run(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(m.cfg.MonitorInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkAll(ctx)
		}
	}
}

func (m *ConnectivityMonitor) checkAll(ctx context.Context) {
	devices, err := m.store.GetAllDevices()
	if err != nil || len(devices) == 0 {
		return
	}

	sem := make(chan struct{}, 10) // 10 concurrent pings
	var wg sync.WaitGroup

	for _, d := range devices {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(dev store.NetworkDevice) {
			defer wg.Done()
			defer func() { <-sem }()

			alive := pingOnce(dev.IP, 1000)
			m.handleResult(dev, alive)

			time.Sleep(20 * time.Millisecond) // rate limit
		}(d)
	}
	wg.Wait()
}

func (m *ConnectivityMonitor) handleResult(dev store.NetworkDevice, alive bool) {
	if alive {
		m.failCounts.Store(dev.IP, int32(0))
		if dev.Status == "offline" {
			// Recovery: 1 success = online
			m.store.UpdateDeviceStatus(dev.ID, "online")
			m.broadcastStatus(dev, "online", "offline")
			log.Printf("[network-monitor] device %s (%s) is back online", dev.IP, dev.Hostname)
		}
	} else {
		val, _ := m.failCounts.LoadOrStore(dev.IP, int32(0))
		count := atomic.AddInt32(val.(*int32), 1)
		// 这里有 bug：LoadOrStore 返回的是 interface{}，需要修正
		// 用简单计数器替代
		rawVal, _ := m.failCounts.Load(dev.IP)
		var count int32
		if rawVal != nil {
			count = rawVal.(int32) + 1
		} else {
			count = 1
		}
		m.failCounts.Store(dev.IP, count)

		if count >= 3 && dev.Status == "online" {
			// Offline: 3 consecutive failures
			m.store.UpdateDeviceStatus(dev.ID, "offline")
			m.broadcastStatus(dev, "offline", "online")
			log.Printf("[network-monitor] device %s (%s) is offline (3 consecutive failures)", dev.IP, dev.Hostname)
		}
	}
}

func (m *ConnectivityMonitor) broadcastStatus(dev store.NetworkDevice, newStatus, prevStatus string) {
	if m.hub == nil {
		return
	}
	m.hub.BroadcastAdmin(map[string]interface{}{
		"type": "network_device_status",
		"data": map[string]interface{}{
			"device_id":   dev.ID,
			"ip":          dev.IP,
			"status":      newStatus,
			"prev_status": prevStatus,
		},
	})
}

func pingOnce(ip string, timeoutMs int) bool {
	pinger, err := probing.NewPinger(ip)
	if err != nil {
		return false
	}
	pinger.Count = 1
	pinger.Timeout = time.Duration(timeoutMs) * time.Millisecond
	pinger.SetPrivileged(true)
	if err := pinger.Run(); err != nil {
		return false
	}
	return pinger.Statistics().PacketsRecv > 0
}
```

注意：`handleResult` 中 failCounts 的处理需要在实际编码时修正为正确的 atomic 模式。上面的代码草稿有并发问题，实现时用 `sync.Map` 存 `*int32` 指针或改用简单 `sync.Mutex` + `map[string]int`。

- [ ] **Step 2: 验证编译**

Run: `go build ./cmd/server/`

- [ ] **Step 3: Commit**

```bash
git add internal/network/monitor.go
git commit -m "feat(network): add connectivity monitor with offline detection (3-fail threshold)"
```

---

## Chunk 3: API 层 + 主程序集成

### Task 8: NetworkHandler HTTP API

**Files:**
- Create: `server/internal/api/network_handler.go`

- [ ] **Step 1: 创建 NetworkHandler**

实现以下接口（遵循 NasHandler 模式）：
- `StartScan(c *gin.Context)` — POST /api/v1/network/scan（校验 CIDR 格式，拒绝 >/24）
- `GetScanStatus(c *gin.Context)` — GET /api/v1/network/scan/status
- `CancelScan(c *gin.Context)` — DELETE /api/v1/network/scan
- `ListDevices(c *gin.Context)` — GET /api/v1/network/devices（支持 subnet_id 查询参数）
- `GetDevice(c *gin.Context)` — GET /api/v1/network/devices/:id
- `UpdateDevice(c *gin.Context)` — PUT /api/v1/network/devices/:id（仅 device_type + hostname）
- `DeleteDevice(c *gin.Context)` — DELETE /api/v1/network/devices/:id
- `GetTopology(c *gin.Context)` — GET /api/v1/network/topology
- `ListSubnets(c *gin.Context)` — GET /api/v1/network/subnets
- `GetSNMPConfig(c *gin.Context)` — GET /api/v1/network/snmp-config
- `UpdateSNMPConfig(c *gin.Context)` — PUT /api/v1/network/snmp-config

CIDR 校验逻辑：
```go
func validateCIDR(cidr string) error {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR: %s", cidr)
	}
	ones, _ := ipNet.Mask.Size()
	if ones < 24 {
		return fmt.Errorf("subnet too large: %s (minimum /24)", cidr)
	}
	return nil
}
```

- [ ] **Step 2: 验证编译**
- [ ] **Step 3: Commit**

```bash
git add internal/api/network_handler.go
git commit -m "feat(network): add NetworkHandler HTTP API with CIDR validation"
```

### Task 9: 路由注册 + main.go 集成

**Files:**
- Modify: `server/internal/api/router.go`
- Modify: `server/cmd/server/main.go`

- [ ] **Step 1: 在 RouterDeps 中添加 NetworkHandler 字段**

```go
NetworkHandler *NetworkHandler
```

- [ ] **Step 2: 在 SetupRouter 中注册路由**

Viewer 组（all 权限）：
```go
if deps.NetworkHandler != nil {
	v1.GET("/network/devices", deps.NetworkHandler.ListDevices)
	v1.GET("/network/devices/:id", deps.NetworkHandler.GetDevice)
	v1.GET("/network/topology", deps.NetworkHandler.GetTopology)
	v1.GET("/network/subnets", deps.NetworkHandler.ListSubnets)
}
```

Admin 组：
```go
if deps.NetworkHandler != nil {
	adm.POST("/network/scan", deps.NetworkHandler.StartScan)
	adm.GET("/network/scan/status", deps.NetworkHandler.GetScanStatus)
	adm.DELETE("/network/scan", deps.NetworkHandler.CancelScan)
	adm.PUT("/network/devices/:id", deps.NetworkHandler.UpdateDevice)
	adm.DELETE("/network/devices/:id", deps.NetworkHandler.DeleteDevice)
	adm.GET("/network/snmp-config", deps.NetworkHandler.GetSNMPConfig)
	adm.PUT("/network/snmp-config", deps.NetworkHandler.UpdateSNMPConfig)
}
```

- [ ] **Step 3: 在 main.go 中初始化 network 模块**

在 NAS 初始化之后：
```go
// Network topology
networkStore := store.NewNetworkStore(db)
var networkHandler *api.NetworkHandler
var networkMonitor *network.ConnectivityMonitor
if cfg.Network.Enabled {
	scanner := network.NewScanner(cfg.Network, hub)
	snmpProber := network.NewSNMPProber(cfg.Network.SNMPCommunities, cfg.Network.Scan.SNMPTimeoutMs)
	networkHandler = api.NewNetworkHandler(networkStore, scanner, snmpProber, hub, credentialStore, serverStore)
	networkMonitor = network.NewConnectivityMonitor(cfg.Network, networkStore, hub)
	networkMonitor.Start()
	defer networkMonitor.Stop()
	log.Println("[network] topology module enabled")
}
```

在 RouterDeps 中添加：
```go
NetworkHandler: networkHandler,
```

- [ ] **Step 4: 验证编译 + 启动测试**

Run: `go build ./cmd/server/`

- [ ] **Step 5: Commit**

```bash
git add internal/api/router.go cmd/server/main.go
git commit -m "feat(network): wire network module into router and main.go"
```

---

## Chunk 4: 前端实现

### Task 10: 前端 API 客户端 + Store

**Files:**
- Create: `web/src/api/network.ts`
- Create: `web/src/stores/networkStore.ts`

- [ ] **Step 1: 创建 API 客户端 `network.ts`**

类型定义 + API 函数（listDevices, getDevice, getTopology, listSubnets, startScan, cancelScan, getScanStatus, updateDevice, deleteDevice）

- [ ] **Step 2: 创建 Zustand store `networkStore.ts`**

状态：devices, subnets, topology, scanStatus, loading
动作：fetchDevices, fetchSubnets, fetchTopology, updateDeviceStatus

- [ ] **Step 3: TypeScript 编译检查**

Run: `npx tsc --noEmit`

- [ ] **Step 4: Commit**

```bash
git add src/api/network.ts src/stores/networkStore.ts
git commit -m "feat(network): add frontend API client and Zustand store"
```

### Task 11: 网络拓扑页面 + 路由 + 菜单

**Files:**
- Create: `web/src/pages/Network/index.tsx` — 主页面（3 Tab 容器 + 扫描按钮）
- Create: `web/src/pages/Network/DeviceList.tsx` — 设备列表表格
- Create: `web/src/pages/Network/SubnetOverview.tsx` — 网段概览卡片
- Create: `web/src/pages/Network/ScanDialog.tsx` — 扫描确认弹窗
- Modify: `web/src/App.tsx` — 添加路由
- Modify: `web/src/components/Layout/Sidebar.tsx` — 添加菜单项

- [ ] **Step 1: 创建主页面 `index.tsx`**

3 个 Tab：拓扑图（占位，Task 12 实现）、设备列表、网段概览。
顶部扫描按钮（admin only）。

- [ ] **Step 2: 创建 DeviceList.tsx**

表格：状态灯、IP、MAC、厂商、类型（可编辑下拉）、型号、SNMP 支持、网段、最后在线。
筛选：按网段/类型/状态。搜索框。

- [ ] **Step 3: 创建 SubnetOverview.tsx**

网段卡片：CIDR、网关、设备数/在线数、在线率进度条。

- [ ] **Step 4: 创建 ScanDialog.tsx**

输入 CIDR（逗号分隔），前端校验 /24 限制，显示预估耗时，二次确认。

- [ ] **Step 5: 添加路由和菜单**

App.tsx 添加 `<Route path="/network" element={<Network />} />`。
Sidebar.tsx 在告警中心之后添加 `{ to: '/network', label: '网络拓扑', icon: 'device_hub' }`。

- [ ] **Step 6: TypeScript 编译 + 构建检查**

Run: `npx tsc --noEmit && npm run build`

- [ ] **Step 7: Commit**

```bash
git add src/pages/Network/ src/App.tsx src/components/Layout/Sidebar.tsx
git commit -m "feat(network): add network topology page with device list and subnet overview"
```

### Task 12: D3.js 力导向拓扑图

**Files:**
- Create: `web/src/pages/Network/TopologyGraph.tsx`

- [ ] **Step 1: 安装 D3**

```bash
npm install d3 @types/d3
```

- [ ] **Step 2: 创建 TopologyGraph.tsx**

使用 D3 force simulation：
- 节点：设备（图标按 device_type 区分）
- 边：network_links
- 颜色：online 绿 / offline 灰
- 已关联服务器：绿色边框
- 悬停：设备信息卡片
- 支持拖拽、缩放

- [ ] **Step 3: 集成到主页面 Tab 1**
- [ ] **Step 4: 构建检查**

Run: `npm run build`

- [ ] **Step 5: Commit**

```bash
git add src/pages/Network/TopologyGraph.tsx package.json package-lock.json
git commit -m "feat(network): add D3.js force-directed topology graph"
```

### Task 13: WebSocket 事件处理

**Files:**
- Modify: `web/src/hooks/useWebSocket.ts`

- [ ] **Step 1: 添加 network_* 事件处理**

```typescript
case 'network_scan_progress':
case 'network_scan_subnet_done':
case 'network_scan_job_done':
  // 更新 networkStore 的 scanStatus
case 'network_device_status':
  // 更新 networkStore 中设备状态
```

- [ ] **Step 2: Commit**

```bash
git add src/hooks/useWebSocket.ts
git commit -m "feat(network): handle network WebSocket events"
```

---

## Chunk 5: 集成测试 + 部署

### Task 14: 告警引擎集成

**Files:**
- Modify: `server/internal/alert/evaluator.go`

- [ ] **Step 1: 添加 network_device_offline 告警类型**

在告警评估逻辑中，检查 ConnectivityMonitor 标记为 offline 的设备，触发告警事件。

- [ ] **Step 2: Commit**

```bash
git add internal/alert/evaluator.go
git commit -m "feat(network): add network_device_offline alert type"
```

### Task 15: 编译部署验证

- [ ] **Step 1: 完整后端编译**

```bash
cd server && go build ./cmd/server/
```

- [ ] **Step 2: 前端 TypeScript + 构建**

```bash
cd web && npx tsc --noEmit && npm run build
```

- [ ] **Step 3: 生产环境部署**

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/mantisops-deploy/opsboard-server ./cmd/server/
# 上传 + 设置 CAP_NET_RAW + 重启
sudo setcap cap_net_raw+ep /opt/opsboard/opsboard-server
```

- [ ] **Step 4: 更新生产 server.yaml 添加 network 配置段**
- [ ] **Step 5: 功能验证**

通过浏览器访问 `/network` 页面，执行扫描测试，验证设备发现、拓扑图、连通性监控。

- [ ] **Step 6: 最终 Commit + Push**

```bash
git add -A
git commit -m "feat(network): complete network topology discovery feature"
git push github main
```
