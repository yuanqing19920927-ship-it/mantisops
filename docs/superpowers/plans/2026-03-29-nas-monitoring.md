# NAS 监控模块 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 MantisOps 添加 NAS 设备监控功能：通过 SSH 采集群晖/飞牛 NAS 的 RAID、S.M.A.R.T.、存储卷、UPS 等指标，前端新增 NAS 列表/详情页，设置页新增 NAS 设备管理。

**Architecture:** 后端新增 `collector/nas*.go` 通过 SSH 定时采集 NAS 指标，`store/nas_store.go` 管理设备 CRUD，`api/nas_handler.go` 提供 REST API。指标写入 VictoriaMetrics（`mantisops_nas_*` 前缀），通过 WebSocket 实时广播。前端新增 `/nas` 列表页 + `/nas/:id` 详情页，设置页新增 NAS 设备管理区块。

**Tech Stack:** Go (Gin + SQLite + SSH), React 19 + TypeScript + Zustand, VictoriaMetrics, WebSocket

**Spec:** `docs/superpowers/specs/2026-03-29-nas-monitoring-design.md`

---

## File Map

### 新建文件

| 文件 | 职责 |
|------|------|
| `server/internal/store/nas_store.go` | NasStore：nas_devices 表 CRUD |
| `server/internal/collector/nas.go` | NasCollector：生命周期管理、采集调度、指标缓存 |
| `server/internal/collector/nas_ssh.go` | SSH 命令执行 + 通用指标解析（CPU/内存/网络/RAID/S.M.A.R.T./卷/UPS） |
| `server/internal/collector/nas_synology.go` | 群晖专属命令与解析（synoinfo/synopkg） |
| `server/internal/collector/nas_fnos.go` | 飞牛专属命令与解析（btrfs） |
| `server/internal/api/nas_handler.go` | NAS HTTP API handler（CRUD + test + metrics） |
| `web/src/pages/NAS/index.tsx` | NAS 列表页 |
| `web/src/pages/NAS/NASDetail.tsx` | NAS 详情页 |
| `web/src/api/nas.ts` | NAS API 客户端 |
| `web/src/stores/nasStore.ts` | NAS Zustand store |

### 修改文件

| 文件 | 改动 |
|------|------|
| `server/internal/store/sqlite.go` | migrate() 新增 nas_devices 建表 + 索引 |
| `server/internal/store/credential_store.go` | List() 和 Delete() SQL 加入 nas_devices 引用计数 |
| `server/internal/config/config.go` | Config 结构体无需改（NAS 设备全部存 DB，不走 yaml 配置） |
| `server/internal/api/router.go` | RouterDeps 新增 NasHandler，注册 /nas-devices/* 路由 |
| `server/cmd/server/main.go` | 初始化 NasStore、NasCollector、NasHandler，注入 RouterDeps |
| `server/internal/ws/hub.go` | 无需改（复用 BroadcastJSON） |
| `server/internal/logging/middleware.go` | auditRoutes 新增 NAS 审计条目 |
| `server/internal/alert/alerter.go` | 新增 NasMetricsProvider 字段、evaluateNas() 调用 |
| `server/internal/alert/evaluator.go` | Evaluate switch 新增 nas_* case、新增 evalNas* 函数 |
| `web/src/App.tsx` | 新增 /nas 和 /nas/:id 路由 |
| `web/src/components/Layout/Sidebar.tsx` | links 数组新增 NAS 存储菜单项 |
| `web/src/hooks/useWebSocket.ts` | 新增 nas_metrics / nas_status 消息处理 |
| `web/src/pages/Settings/index.tsx` | 新增 NAS 设备管理区块 |

---

## Chunk 1: 后端数据层（Store + 建表 + 凭据修复）

### Task 1: nas_devices 建表迁移

**Files:**
- Modify: `server/internal/store/sqlite.go`

- [ ] **Step 1: 在 migrate() 的 stmts 数组末尾添加 nas_devices 建表语句**

在现有 `stmts` 数组（约 line 190 前）添加：

```go
`CREATE TABLE IF NOT EXISTS nas_devices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    nas_type TEXT NOT NULL,
    host TEXT NOT NULL,
    port INTEGER NOT NULL DEFAULT 22,
    ssh_user TEXT NOT NULL DEFAULT 'root',
    credential_id INTEGER NOT NULL REFERENCES credentials(id),
    collect_interval INTEGER DEFAULT 60,
    status TEXT DEFAULT 'unknown',
    last_seen INTEGER,
    system_info TEXT DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
`CREATE UNIQUE INDEX IF NOT EXISTS idx_nas_devices_host_port ON nas_devices(host, port)`,
```

- [ ] **Step 2: 验证编译通过**

```bash
cd server && go build ./cmd/server/
```

- [ ] **Step 3: Commit**

```bash
git add server/internal/store/sqlite.go
git commit -m "feat(store): add nas_devices table migration"
```

### Task 2: NasStore CRUD

**Files:**
- Create: `server/internal/store/nas_store.go`

- [ ] **Step 1: 创建 NasStore 结构体和模型**

```go
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type NasDevice struct {
	ID              int        `json:"id"`
	Name            string     `json:"name"`
	NasType         string     `json:"nas_type"`
	Host            string     `json:"host"`
	Port            int        `json:"port"`
	SSHUser         string     `json:"ssh_user"`
	CredentialID    int        `json:"credential_id"`
	CollectInterval int        `json:"collect_interval"`
	Status          string     `json:"status"`
	LastSeen        *time.Time `json:"last_seen"`
	SystemInfo      string     `json:"system_info"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type NasStore struct {
	db *sql.DB
}

func NewNasStore(db *sql.DB) *NasStore {
	return &NasStore{db: db}
}
```

- [ ] **Step 2: 实现 List 方法**

```go
func (s *NasStore) List() ([]NasDevice, error) {
	rows, err := s.db.Query(`SELECT id, name, nas_type, host, port, ssh_user, credential_id,
		collect_interval, status, last_seen, system_info, created_at, updated_at
		FROM nas_devices ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var devices []NasDevice
	for rows.Next() {
		d, err := scanNasDevice(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, *d)
	}
	return devices, rows.Err()
}
```

- [ ] **Step 3: 实现 Get / Create / Update / Delete 方法**

```go
func (s *NasStore) Get(id int) (*NasDevice, error) {
	row := s.db.QueryRow(`SELECT id, name, nas_type, host, port, ssh_user, credential_id,
		collect_interval, status, last_seen, system_info, created_at, updated_at
		FROM nas_devices WHERE id=?`, id)
	return scanNasDevice(row)
}

func (s *NasStore) Create(name, nasType, host string, port int, sshUser string, credentialID, collectInterval int) (int, error) {
	if collectInterval < 30 {
		collectInterval = 30
	}
	if sshUser == "" {
		sshUser = "root"
	}
	res, err := s.db.Exec(`INSERT INTO nas_devices (name, nas_type, host, port, ssh_user, credential_id, collect_interval)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, name, nasType, host, port, sshUser, credentialID, collectInterval)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return int(id), err
}

func (s *NasStore) Update(id int, name, nasType, host string, port int, sshUser string, credentialID, collectInterval int) error {
	if collectInterval < 30 {
		collectInterval = 30
	}
	if sshUser == "" {
		sshUser = "root"
	}
	_, err := s.db.Exec(`UPDATE nas_devices SET name=?, nas_type=?, host=?, port=?, ssh_user=?, credential_id=?,
		collect_interval=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		name, nasType, host, port, sshUser, credentialID, collectInterval, id)
	return err
}

func (s *NasStore) Delete(id int) error {
	_, err := s.db.Exec(`DELETE FROM nas_devices WHERE id=?`, id)
	return err
}

// UpdateStatus 更新状态，仅 online/degraded 时刷新 last_seen
func (s *NasStore) UpdateStatus(id int, status string) error {
	if status == "online" || status == "degraded" {
		_, err := s.db.Exec(`UPDATE nas_devices SET status=?, last_seen=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
			status, time.Now().Unix(), id)
		return err
	}
	// offline/unknown 只更新 status，不刷新 last_seen
	_, err := s.db.Exec(`UPDATE nas_devices SET status=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		status, id)
	return err
}

func (s *NasStore) UpdateSystemInfo(id int, info string) error {
	_, err := s.db.Exec(`UPDATE nas_devices SET system_info=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		info, id)
	return err
}
```

- [ ] **Step 4: 实现 scanner helper**

```go
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanNasDevice(row rowScanner) (*NasDevice, error) {
	var d NasDevice
	var lastSeen sql.NullInt64
	err := row.Scan(&d.ID, &d.Name, &d.NasType, &d.Host, &d.Port, &d.SSHUser, &d.CredentialID,
		&d.CollectInterval, &d.Status, &lastSeen, &d.SystemInfo, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if lastSeen.Valid {
		t := time.Unix(lastSeen.Int64, 0)
		d.LastSeen = &t
	}
	return &d, nil
}
```

注意：`rowScanner` 接口已在 `cloud_store.go` 中定义，这里不需要重复定义。检查是否已存在，如果已存在则删除此处的定义。

- [ ] **Step 5: 验证编译**

```bash
cd server && go build ./cmd/server/
```

- [ ] **Step 6: Commit**

```bash
git add server/internal/store/nas_store.go
git commit -m "feat(store): add NasStore CRUD for nas_devices"
```

### Task 3: 修复凭据引用计数

**Files:**
- Modify: `server/internal/store/credential_store.go`

- [ ] **Step 1: 修改 List() 的 used_by 子查询**

在 `List()` 方法的 SQL 中（约 line 84-86），将：
```sql
(SELECT COUNT(*) FROM managed_servers WHERE credential_id = c.id) +
(SELECT COUNT(*) FROM cloud_accounts WHERE credential_id = c.id) AS used_by
```
改为：
```sql
(SELECT COUNT(*) FROM managed_servers WHERE credential_id = c.id) +
(SELECT COUNT(*) FROM cloud_accounts WHERE credential_id = c.id) +
(SELECT COUNT(*) FROM nas_devices WHERE credential_id = c.id) AS used_by
```

- [ ] **Step 2: 修改 Delete() 的引用检查**

在 `Delete()` 方法的 SQL 中（约 line 122-124），将：
```sql
SELECT (SELECT COUNT(*) FROM managed_servers WHERE credential_id = ?) +
       (SELECT COUNT(*) FROM cloud_accounts WHERE credential_id = ?)
```
改为：
```sql
SELECT (SELECT COUNT(*) FROM managed_servers WHERE credential_id = ?) +
       (SELECT COUNT(*) FROM cloud_accounts WHERE credential_id = ?) +
       (SELECT COUNT(*) FROM nas_devices WHERE credential_id = ?)
```

注意 `Delete()` 方法的参数绑定也需要加一个 `id` 参数（从 2 个变为 3 个）。

- [ ] **Step 3: 验证编译**

```bash
cd server && go build ./cmd/server/
```

- [ ] **Step 4: Commit**

```bash
git add server/internal/store/credential_store.go
git commit -m "fix(store): include nas_devices in credential reference count"
```

---

## Chunk 2: SSH 采集器

### Task 4: 通用 SSH 采集与指标解析

**Files:**
- Create: `server/internal/collector/nas_ssh.go`

- [ ] **Step 1: 定义指标数据结构**

```go
package collector

import "time"

// NasMetricsSnapshot 是一次完整采集的快照
type NasMetricsSnapshot struct {
	NasID     int64     `json:"nas_id"`
	Timestamp time.Time `json:"timestamp"`
	CPU       *NasCPU   `json:"cpu,omitempty"`
	Memory    *NasMemory `json:"memory,omitempty"`
	Networks  []NasNetwork `json:"networks,omitempty"`
	Raids     []NasRaid    `json:"raids,omitempty"`
	Volumes   []NasVolume  `json:"volumes,omitempty"`
	Disks     []NasDisk    `json:"disks,omitempty"`
	UPS       *NasUPS      `json:"ups,omitempty"`
}

type NasCPU struct {
	UsagePercent float64 `json:"usage_percent"`
}

type NasMemory struct {
	Total        uint64  `json:"total"`
	Used         uint64  `json:"used"`
	UsagePercent float64 `json:"usage_percent"`
}

type NasNetwork struct {
	Interface    string  `json:"interface"`
	RxBytesPerSec uint64 `json:"rx_bytes_per_sec"`
	TxBytesPerSec uint64 `json:"tx_bytes_per_sec"`
}

type NasRaid struct {
	Array          string   `json:"array"`
	RaidType       string   `json:"raid_type"`
	Status         string   `json:"status"` // active / degraded / rebuilding
	Disks          []string `json:"disks"`
	RebuildPercent float64  `json:"rebuild_percent"`
}

type NasVolume struct {
	Mount        string  `json:"mount"`
	FsType       string  `json:"fs_type"`
	Total        uint64  `json:"total"`
	Used         uint64  `json:"used"`
	UsagePercent float64 `json:"usage_percent"`
}

type NasDisk struct {
	Name               string `json:"name"`
	Model              string `json:"model"`
	Size               uint64 `json:"size"`
	Temperature        int    `json:"temperature"`
	PowerOnHours       int    `json:"power_on_hours"`
	SmartHealthy       bool   `json:"smart_healthy"`
	ReallocatedSectors int    `json:"reallocated_sectors"`
}

type NasUPS struct {
	Status         string `json:"status"` // online / on_battery / low_battery
	BatteryPercent int    `json:"battery_percent"`
	Model          string `json:"model"`
}

// NasRawCounters 上一轮原始计数（用于 delta）
type NasRawCounters struct {
	CPUIdle  uint64
	CPUTotal uint64
	NetRx    map[string]uint64
	NetTx    map[string]uint64
}

// NasDeviceHealth 采集健康状态
type NasDeviceHealth struct {
	FailureCount int
	LastError    string
	LastStatus   string // 上次广播的状态，用于检测变更
}
```

- [ ] **Step 2: 实现 SSH 连接与批量命令执行**

```go
import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHConnect 建立 SSH 连接
// credType: "ssh_password" 或 "ssh_key"
// password: ssh_password 类型的密码
// privateKey: ssh_key 类型的私钥 PEM
// passphrase: ssh_key 类型的私钥口令（可为空）
func SSHConnect(host string, port int, username, credType, password, privateKey, passphrase string) (*ssh.Client, error) {
	var authMethods []ssh.AuthMethod

	switch credType {
	case "ssh_key":
		var signer ssh.Signer
		var err error
		if passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(privateKey), []byte(passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(privateKey))
		}
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	case "ssh_password":
		authMethods = append(authMethods, ssh.Password(password))
	default:
		return nil, fmt.Errorf("unsupported credential type: %s", credType)
	}

	config := &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		Timeout:         10 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	return ssh.Dial("tcp", addr, config)
}

// sshExec 在已有连接上执行单条命令
func sshExec(client *ssh.Client, cmd string, timeout time.Duration) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() { done <- session.Run(cmd) }()

	select {
	case err := <-done:
		if err != nil {
			return stdout.String(), fmt.Errorf("cmd %q: %w (stderr: %s)", cmd, err, stderr.String())
		}
		return stdout.String(), nil
	case <-time.After(timeout):
		return "", fmt.Errorf("cmd %q: timeout after %s", cmd, timeout)
	}
}
```

注意：项目中 `deployer/` 已经有 SSH 连接实现，查看 `server/internal/deployer/` 是否有可复用的 SSH 客户端代码。如有，优先复用。如果 deployer 的 SSH 逻辑与这里差异较大（deployer 需要 SFTP + systemd 操作），则独立实现上述精简版本。

- [ ] **Step 3: 实现通用指标解析函数**

```go
import (
	"regexp"
	"strconv"
	"strings"
)

// parseProcStat 解析 /proc/stat，返回 (idle, total)
func parseProcStat(data string) (uint64, uint64) {
	for _, line := range strings.Split(data, "\n") {
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 5 {
				return 0, 0
			}
			var total, idle uint64
			for i, f := range fields[1:] {
				v, _ := strconv.ParseUint(f, 10, 64)
				total += v
				if i == 3 { // idle is 4th field (0-indexed: 3)
					idle = v
				}
			}
			return idle, total
		}
	}
	return 0, 0
}

// parseProcMeminfo 解析 /proc/meminfo
func parseProcMeminfo(data string) *NasMemory {
	mem := &NasMemory{}
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, _ := strconv.ParseUint(fields[1], 10, 64)
		val *= 1024 // kB → bytes
		switch fields[0] {
		case "MemTotal:":
			mem.Total = val
		case "MemAvailable:":
			mem.Used = mem.Total - val
		}
	}
	if mem.Total > 0 {
		mem.UsagePercent = float64(mem.Used) / float64(mem.Total) * 100
	}
	return mem
}

// parseProcNetDev 解析 /proc/net/dev，返回 interface → (rx_bytes, tx_bytes)
func parseProcNetDev(data string) map[string][2]uint64 {
	result := make(map[string][2]uint64)
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, ":") || strings.HasPrefix(line, "Inter") || strings.HasPrefix(line, "face") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		iface := strings.TrimSpace(parts[0])
		if iface == "lo" {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 10 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		result[iface] = [2]uint64{rx, tx}
	}
	return result
}

// parseMdstat 解析 /proc/mdstat
func parseMdstat(data string) []NasRaid {
	var raids []NasRaid
	lines := strings.Split(data, "\n")
	mdRe := regexp.MustCompile(`^(md\d+)\s*:\s*active\s+(\w+)\s+(.+)$`)
	rebuildRe := regexp.MustCompile(`recovery\s*=\s*([\d.]+)%`)
	degradedRe := regexp.MustCompile(`\[.*_.*\]`) // [UU_] means degraded

	for i, line := range lines {
		matches := mdRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		raid := NasRaid{
			Array:    matches[1],
			RaidType: matches[2],
			Status:   "active",
		}
		// 提取磁盘列表
		diskParts := strings.Fields(matches[3])
		for _, dp := range diskParts {
			// 格式: sda1[0] or sda1[0](S)
			diskName := strings.Split(dp, "[")[0]
			// 去掉分区号，只保留磁盘名
			diskName = regexp.MustCompile(`\d+$`).ReplaceAllString(diskName, "")
			if diskName != "" {
				raid.Disks = append(raid.Disks, diskName)
			}
		}
		// 检查降级状态（下一行或下两行）
		for j := i + 1; j < len(lines) && j <= i+2; j++ {
			if degradedRe.MatchString(lines[j]) {
				raid.Status = "degraded"
			}
			if m := rebuildRe.FindStringSubmatch(lines[j]); m != nil {
				raid.Status = "rebuilding"
				raid.RebuildPercent, _ = strconv.ParseFloat(m[1], 64)
			}
		}
		// 去重磁盘列表
		seen := make(map[string]bool)
		var unique []string
		for _, d := range raid.Disks {
			if !seen[d] {
				seen[d] = true
				unique = append(unique, d)
			}
		}
		raid.Disks = unique
		raids = append(raids, raid)
	}
	return raids
}

// parseDf 解析 df -T -B1 输出（含 fstype 列），按 nasType 过滤
func parseDf(data string, nasType string) []NasVolume {
	var volumes []NasVolume
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		// df -T 输出格式: Filesystem Type 1B-blocks Used Available Use% Mounted
		if len(fields) < 7 || fields[0] == "Filesystem" {
			continue
		}
		fsType := fields[1]
		mount := fields[6]
		total, _ := strconv.ParseUint(fields[2], 10, 64)
		used, _ := strconv.ParseUint(fields[3], 10, 64)

		// 按 NAS 类型过滤
		keep := false
		switch nasType {
		case "synology":
			keep = strings.HasPrefix(mount, "/volume")
		case "fnos":
			keep = !strings.HasPrefix(mount, "/boot") &&
				mount != "/" &&
				!strings.HasPrefix(mount, "/snap") &&
				total >= 10*1024*1024*1024 // >= 10GB
		default:
			keep = total >= 10*1024*1024*1024
		}
		if !keep {
			continue
		}

		var usagePct float64
		if total > 0 {
			usagePct = float64(used) / float64(total) * 100
		}
		volumes = append(volumes, NasVolume{
			Mount: mount, FsType: fsType, Total: total, Used: used, UsagePercent: usagePct,
		})
	}
	return volumes
}

// parseSmartctl 解析单个硬盘的 smartctl -A -H 输出
func parseSmartctl(data string) (temperature, powerOnHours, reallocSectors int, healthy bool) {
	healthy = true
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "SMART overall-health") {
			if strings.Contains(line, "PASSED") {
				healthy = true
			} else {
				healthy = false
			}
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		attrID := fields[0]
		rawValue, _ := strconv.Atoi(fields[9])
		switch attrID {
		case "194": // Temperature
			temperature = rawValue
		case "9": // Power-On Hours
			powerOnHours = rawValue
		case "5": // Reallocated Sectors
			reallocSectors = rawValue
		}
	}
	return
}

// parseUpsc 解析 upsc 输出
func parseUpsc(data string) *NasUPS {
	if strings.TrimSpace(data) == "" {
		return nil
	}
	ups := &NasUPS{Status: "unknown"}
	for _, line := range strings.Split(data, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "ups.status":
			switch {
			case strings.Contains(val, "OL"):
				ups.Status = "online"
			case strings.Contains(val, "OB"):
				ups.Status = "on_battery"
			case strings.Contains(val, "LB"):
				ups.Status = "low_battery"
			}
		case "battery.charge":
			ups.BatteryPercent, _ = strconv.Atoi(val)
		case "ups.model":
			ups.Model = val
		}
	}
	return ups
}
```

- [ ] **Step 4: 实现 collectNasMetrics 主采集函数**

```go
// collectNasMetrics SSH 连接并采集所有指标
func collectNasMetrics(client *ssh.Client, nasType string, prev *NasRawCounters, interval int) (*NasMetricsSnapshot, *NasRawCounters, error) {
	timeout := 30 * time.Second
	snap := &NasMetricsSnapshot{Timestamp: time.Now()}
	newRaw := &NasRawCounters{NetRx: make(map[string]uint64), NetTx: make(map[string]uint64)}

	// 1. CPU + 内存（一条命令）
	out, err := sshExec(client, "cat /proc/stat && echo '---SEPARATOR---' && cat /proc/meminfo", timeout)
	if err == nil {
		parts := strings.SplitN(out, "---SEPARATOR---", 2)
		if len(parts) == 2 {
			idle, total := parseProcStat(parts[0])
			newRaw.CPUIdle = idle
			newRaw.CPUTotal = total
			if prev != nil && prev.CPUTotal > 0 && total > prev.CPUTotal {
				deltaTotal := total - prev.CPUTotal
				deltaIdle := idle - prev.CPUIdle
				snap.CPU = &NasCPU{UsagePercent: float64(deltaTotal-deltaIdle) / float64(deltaTotal) * 100}
			}
			snap.Memory = parseProcMeminfo(parts[1])
		}
	}

	// 2. 网络
	out, err = sshExec(client, "cat /proc/net/dev", timeout)
	if err == nil {
		netStats := parseProcNetDev(out)
		for iface, vals := range netStats {
			newRaw.NetRx[iface] = vals[0]
			newRaw.NetTx[iface] = vals[1]
			if prev != nil {
				if prevRx, ok := prev.NetRx[iface]; ok {
					rxPerSec := (vals[0] - prevRx) / uint64(interval)
					txPerSec := (vals[1] - prev.NetTx[iface]) / uint64(interval)
					snap.Networks = append(snap.Networks, NasNetwork{
						Interface: iface, RxBytesPerSec: rxPerSec, TxBytesPerSec: txPerSec,
					})
				}
			}
		}
	}

	// 3. RAID
	out, err = sshExec(client, "cat /proc/mdstat", timeout)
	if err == nil {
		snap.Raids = parseMdstat(out)
	}

	// 4. 卷用量
	out, err = sshExec(client, "df -T -B1 -x tmpfs -x devtmpfs -x squashfs", timeout)
	if err == nil {
		snap.Volumes = parseDf(out, nasType)
	}

	// 5. 硬盘列表 + S.M.A.R.T.
	out, err = sshExec(client, "lsblk -nd -o NAME,SIZE,MODEL,TYPE -b", timeout)
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			fields := strings.Fields(line)
			if len(fields) < 4 || fields[3] != "disk" {
				continue
			}
			disk := NasDisk{Name: fields[0]}
			disk.Size, _ = strconv.ParseUint(fields[1], 10, 64)
			if len(fields) > 2 {
				disk.Model = strings.Join(fields[2:len(fields)-1], " ")
			}
			// S.M.A.R.T.
			smartOut, smartErr := sshExec(client, fmt.Sprintf("sudo smartctl -A -H /dev/%s 2>/dev/null", disk.Name), timeout)
			if smartErr == nil {
				disk.Temperature, disk.PowerOnHours, disk.ReallocatedSectors, disk.SmartHealthy = parseSmartctl(smartOut)
			} else {
				disk.SmartHealthy = true // 无法获取时默认健康
			}
			snap.Disks = append(snap.Disks, disk)
		}
	}

	// 6. UPS
	out, _ = sshExec(client, "upsc ups@localhost 2>/dev/null", timeout)
	snap.UPS = parseUpsc(out)

	return snap, newRaw, nil
}
```

- [ ] **Step 5: 验证编译**

```bash
cd server && go build ./cmd/server/
```

- [ ] **Step 6: Commit**

```bash
git add server/internal/collector/nas_ssh.go
git commit -m "feat(collector): add NAS SSH collection and metric parsing"
```

### Task 5: 群晖专属采集

**Files:**
- Create: `server/internal/collector/nas_synology.go`

- [ ] **Step 1: 实现群晖系统信息和套件采集**

```go
package collector

import (
	"encoding/json"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type SynologySystemInfo struct {
	Model     string `json:"model"`
	Serial    string `json:"serial"`
	OSVersion string `json:"os_version"`
	Kernel    string `json:"kernel"`
	Arch      string `json:"arch"`
}

type SynologyPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Status  string `json:"status"` // running / stopped
}

// collectSynologyInfo 采集群晖静态系统信息（仅首次调用）
func collectSynologyInfo(client *ssh.Client) (*SynologySystemInfo, error) {
	timeout := 30 * time.Second
	info := &SynologySystemInfo{}

	// synoinfo.conf
	out, err := sshExec(client, "cat /etc/synoinfo.conf 2>/dev/null", timeout)
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.Trim(strings.TrimSpace(parts[1]), "\"")
			switch key {
			case "upnpmodelname":
				info.Model = val
			case "unique":
				info.Serial = val
			}
		}
	}

	// VERSION
	out, err = sshExec(client, "cat /etc.defaults/VERSION 2>/dev/null", timeout)
	if err == nil {
		var major, minor, build string
		for _, line := range strings.Split(out, "\n") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.Trim(strings.TrimSpace(parts[1]), "\"")
			switch key {
			case "majorversion":
				major = val
			case "minorversion":
				minor = val
			case "buildnumber":
				build = val
			}
		}
		info.OSVersion = "DSM " + major + "." + minor + "-" + build
	}

	// kernel + arch
	out, _ = sshExec(client, "uname -r && uname -m", timeout)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) >= 1 {
		info.Kernel = strings.TrimSpace(lines[0])
	}
	if len(lines) >= 2 {
		info.Arch = strings.TrimSpace(lines[1])
	}

	return info, nil
}

// collectSynologyPackages 采集群晖已安装套件列表
func collectSynologyPackages(client *ssh.Client) ([]SynologyPackage, error) {
	timeout := 30 * time.Second
	out, err := sshExec(client, "synopkg list --format json 2>/dev/null || echo '[]'", timeout)
	if err != nil {
		return nil, err
	}
	var pkgs []SynologyPackage
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &pkgs); err != nil {
		// 如果 JSON 解析失败，尝试按行解析（旧版 DSM 可能不支持 --format json）
		for _, line := range strings.Split(out, "\n") {
			name := strings.TrimSpace(line)
			if name != "" && name != "[]" {
				pkgs = append(pkgs, SynologyPackage{Name: name, Status: "running"})
			}
		}
	}
	return pkgs, nil
}
```

- [ ] **Step 2: Commit**

```bash
git add server/internal/collector/nas_synology.go
git commit -m "feat(collector): add Synology-specific SSH collection"
```

### Task 6: 飞牛专属采集

**Files:**
- Create: `server/internal/collector/nas_fnos.go`

- [ ] **Step 1: 实现飞牛系统信息和 Btrfs 采集**

```go
package collector

import (
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type FnOSSystemInfo struct {
	OSVersion string `json:"os_version"`
	Kernel    string `json:"kernel"`
	Arch      string `json:"arch"`
}

// collectFnOSInfo 采集飞牛静态系统信息
func collectFnOSInfo(client *ssh.Client) (*FnOSSystemInfo, error) {
	timeout := 30 * time.Second
	info := &FnOSSystemInfo{}

	out, err := sshExec(client, "cat /etc/os-release 2>/dev/null", timeout)
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.Trim(strings.TrimSpace(parts[1]), "\"")
			switch key {
			case "PRETTY_NAME":
				info.OSVersion = val
			}
		}
	}

	out, _ = sshExec(client, "uname -r && uname -m", timeout)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) >= 1 {
		info.Kernel = strings.TrimSpace(lines[0])
	}
	if len(lines) >= 2 {
		info.Arch = strings.TrimSpace(lines[1])
	}

	return info, nil
}

// collectBtrfsHealth 检查 Btrfs 文件系统健康状态
func collectBtrfsHealth(client *ssh.Client) (hasErrors bool, details string) {
	timeout := 30 * time.Second
	out, err := sshExec(client, "sudo btrfs device stats / 2>/dev/null", timeout)
	if err != nil {
		return false, ""
	}
	// btrfs device stats 输出每行格式: [/dev/sda1].write_io_errs    0
	// 任何非 0 值表示有错误
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			val := strings.TrimSpace(fields[len(fields)-1])
			if val != "0" && val != "" {
				return true, out
			}
		}
	}
	return false, out
}
```

- [ ] **Step 2: Commit**

```bash
git add server/internal/collector/nas_fnos.go
git commit -m "feat(collector): add fnOS-specific SSH collection"
```

### Task 7: NasCollector 生命周期管理

**Files:**
- Create: `server/internal/collector/nas.go`

- [ ] **Step 1: 实现 NasCollector 主体**

```go
package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"mantisops/server/internal/store"
	"mantisops/server/internal/ws"
)

type NasCollector struct {
	nasStore  *store.NasStore
	credStore *store.CredentialStore
	vm        *store.VictoriaStore
	hub       *ws.Hub
	cache     map[int64]*NasMetricsSnapshot
	prevRaw   map[int64]*NasRawCounters
	health    map[int64]*NasDeviceHealth
	workers   map[int64]context.CancelFunc
	mu        sync.RWMutex
	stopCh    chan struct{}
}

func NewNasCollector(nasStore *store.NasStore, credStore *store.CredentialStore, vm *store.VictoriaStore, hub *ws.Hub) *NasCollector {
	return &NasCollector{
		nasStore:  nasStore,
		credStore: credStore,
		vm:        vm,
		hub:       hub,
		cache:     make(map[int64]*NasMetricsSnapshot),
		prevRaw:   make(map[int64]*NasRawCounters),
		health:    make(map[int64]*NasDeviceHealth),
		workers:   make(map[int64]context.CancelFunc),
		stopCh:    make(chan struct{}),
	}
}

func (c *NasCollector) Start() {
	devices, err := c.nasStore.List()
	if err != nil {
		log.Printf("[nas] failed to load devices: %v", err)
		return
	}
	for _, d := range devices {
		c.startWorker(d)
	}
	log.Printf("[nas] started collectors for %d devices", len(devices))
}

func (c *NasCollector) Stop() {
	close(c.stopCh)
	c.mu.Lock()
	for _, cancel := range c.workers {
		cancel()
	}
	c.mu.Unlock()
}

func (c *NasCollector) AddDevice(d store.NasDevice) {
	c.startWorker(d)
}

func (c *NasCollector) RemoveDevice(id int) {
	c.mu.Lock()
	if cancel, ok := c.workers[int64(id)]; ok {
		cancel()
		delete(c.workers, int64(id))
		delete(c.cache, int64(id))
		delete(c.prevRaw, int64(id))
		delete(c.health, int64(id))
	}
	c.mu.Unlock()
}

func (c *NasCollector) UpdateDevice(d store.NasDevice) {
	c.RemoveDevice(d.ID)
	c.startWorker(d)
}

func (c *NasCollector) GetMetrics(nasID int64) *NasMetricsSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cache[nasID]
}

func (c *NasCollector) GetAllMetrics() map[int64]*NasMetricsSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cp := make(map[int64]*NasMetricsSnapshot, len(c.cache))
	for k, v := range c.cache {
		cp[k] = v
	}
	return cp
}

func (c *NasCollector) ListDeviceIDs() []int64 {
	devices, err := c.nasStore.List()
	if err != nil {
		return nil
	}
	ids := make([]int64, len(devices))
	for i, d := range devices {
		ids[i] = int64(d.ID)
	}
	return ids
}

func (c *NasCollector) GetDeviceHealth(nasID int64) *NasDeviceHealth {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if h, ok := c.health[nasID]; ok {
		return h
	}
	return &NasDeviceHealth{}
}

func (c *NasCollector) startWorker(d store.NasDevice) {
	ctx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.workers[int64(d.ID)] = cancel
	if c.health[int64(d.ID)] == nil {
		c.health[int64(d.ID)] = &NasDeviceHealth{}
	}
	c.mu.Unlock()

	go c.collectLoop(ctx, d)
}

func (c *NasCollector) collectLoop(ctx context.Context, d store.NasDevice) {
	nasID := int64(d.ID)
	interval := time.Duration(d.CollectInterval) * time.Second
	systemInfoCollected := false

	// 首次立即采集
	c.doCollect(nasID, d, &systemInfoCollected)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.doCollect(nasID, d, &systemInfoCollected)

			// 检查是否需要调整采集频率
			c.mu.RLock()
			h := c.health[nasID]
			c.mu.RUnlock()

			if h != nil && h.FailureCount >= 3 {
				// 降频到 5 分钟
				ticker.Reset(5 * time.Minute)
			} else {
				ticker.Reset(interval)
			}
		}
	}
}

func (c *NasCollector) doCollect(nasID int64, d store.NasDevice, systemInfoCollected *bool) {
	// 获取凭据
	cred, err := c.credStore.Get(d.CredentialID)
	if err != nil {
		c.recordFailure(nasID, "credential_error: "+err.Error())
		return
	}

	// SSH 连接（用户名从 nas_devices.ssh_user 取，不从 credential 取）
	client, err := SSHConnect(d.Host, d.Port, d.SSHUser, cred.Type, cred.Data["password"], cred.Data["private_key"], cred.Data["passphrase"])
	if err != nil {
		errType := "connect_failed"
		if isAuthError(err) {
			errType = "auth_failed"
		}
		c.recordFailure(nasID, errType)
		return
	}
	defer client.Close()

	// 首次采集系统信息
	if !*systemInfoCollected {
		c.collectSystemInfo(client, nasID, d)
		*systemInfoCollected = true
	}

	// 采集指标
	c.mu.RLock()
	prev := c.prevRaw[nasID]
	c.mu.RUnlock()

	snap, newRaw, err := collectNasMetrics(client, d.NasType, prev, d.CollectInterval)
	if err != nil {
		c.recordFailure(nasID, "collect_error: "+err.Error())
		return
	}
	snap.NasID = nasID

	// 成功：更新缓存、写 VM、广播 WS
	c.mu.Lock()
	c.cache[nasID] = snap
	c.prevRaw[nasID] = newRaw
	c.health[nasID] = &NasDeviceHealth{FailureCount: 0}
	c.mu.Unlock()

	// 判定状态
	status := "online"
	for _, r := range snap.Raids {
		if r.Status == "degraded" || r.Status == "rebuilding" {
			status = "degraded"
			break
		}
	}
	for _, disk := range snap.Disks {
		if !disk.SmartHealthy {
			status = "degraded"
			break
		}
	}
	c.nasStore.UpdateStatus(d.ID, status)

	// 写 VictoriaMetrics
	c.writeToVM(nasID, d.Name, snap)

	// WebSocket 广播指标
	c.hub.BroadcastJSON(map[string]interface{}{
		"type":   "nas_metrics",
		"nas_id": nasID,
		"data":   snap,
	})

	// 广播状态变更（与上次状态不同时才发送）
	c.mu.RLock()
	prevStatus := ""
	if h, ok := c.health[nasID]; ok {
		prevStatus = h.LastStatus
	}
	c.mu.RUnlock()
	if status != prevStatus {
		c.hub.BroadcastJSON(map[string]interface{}{
			"type":   "nas_status",
			"nas_id": nasID,
			"status": status,
		})
		c.mu.Lock()
		if c.health[nasID] != nil {
			c.health[nasID].LastStatus = status
		}
		c.mu.Unlock()
	}
}

func (c *NasCollector) recordFailure(nasID int64, errMsg string) {
	c.mu.Lock()
	h := c.health[nasID]
	if h == nil {
		h = &NasDeviceHealth{}
		c.health[nasID] = h
	}
	h.FailureCount++
	h.LastError = errMsg
	c.mu.Unlock()

	c.nasStore.UpdateStatus(int(nasID), "offline")

	// 广播状态变更
	if h.LastStatus != "offline" {
		c.hub.BroadcastJSON(map[string]interface{}{
			"type":   "nas_status",
			"nas_id": nasID,
			"status": "offline",
		})
		h.LastStatus = "offline"
	}

	log.Printf("[nas] device %d collection failed (%d): %s", nasID, h.FailureCount, errMsg)
}

func (c *NasCollector) collectSystemInfo(client *ssh.Client, nasID int64, d store.NasDevice) {
	var infoJSON []byte
	switch d.NasType {
	case "synology":
		info, err := collectSynologyInfo(client)
		if err == nil {
			infoJSON, _ = json.Marshal(info)
		}
	case "fnos":
		info, err := collectFnOSInfo(client)
		if err == nil {
			infoJSON, _ = json.Marshal(info)
		}
	}
	if len(infoJSON) > 0 {
		c.nasStore.UpdateSystemInfo(d.ID, string(infoJSON))
	}
}

func (c *NasCollector) writeToVM(nasID int64, name string, snap *NasMetricsSnapshot) {
	labels := fmt.Sprintf(`nas_id="%d",name="%s"`, nasID, name)
	var lines []string

	if snap.CPU != nil {
		lines = append(lines, fmt.Sprintf("mantisops_nas_cpu_usage_percent{%s} %.2f", labels, snap.CPU.UsagePercent))
	}
	if snap.Memory != nil {
		lines = append(lines, fmt.Sprintf("mantisops_nas_memory_usage_percent{%s} %.2f", labels, snap.Memory.UsagePercent))
	}
	for _, n := range snap.Networks {
		nl := fmt.Sprintf(`%s,interface="%s"`, labels, n.Interface)
		lines = append(lines, fmt.Sprintf("mantisops_nas_network_rx_bytes_per_sec{%s} %d", nl, n.RxBytesPerSec))
		lines = append(lines, fmt.Sprintf("mantisops_nas_network_tx_bytes_per_sec{%s} %d", nl, n.TxBytesPerSec))
	}
	for _, r := range snap.Raids {
		rl := fmt.Sprintf(`%s,array="%s",raid_type="%s"`, labels, r.Array, r.RaidType)
		statusVal := 0
		if r.Status == "degraded" {
			statusVal = 1
		} else if r.Status == "rebuilding" {
			statusVal = 2
		}
		lines = append(lines, fmt.Sprintf("mantisops_nas_raid_status{%s} %d", rl, statusVal))
		lines = append(lines, fmt.Sprintf("mantisops_nas_raid_rebuild_percent{%s} %.2f", rl, r.RebuildPercent))
	}
	for _, v := range snap.Volumes {
		vl := fmt.Sprintf(`%s,volume="%s"`, labels, v.Mount)
		lines = append(lines, fmt.Sprintf("mantisops_nas_volume_total_bytes{%s} %d", vl, v.Total))
		lines = append(lines, fmt.Sprintf("mantisops_nas_volume_used_bytes{%s} %d", vl, v.Used))
		lines = append(lines, fmt.Sprintf("mantisops_nas_volume_usage_percent{%s} %.2f", vl, v.UsagePercent))
	}
	for _, disk := range snap.Disks {
		dl := fmt.Sprintf(`%s,disk="%s"`, labels, disk.Name)
		lines = append(lines, fmt.Sprintf("mantisops_nas_disk_temperature_celsius{%s} %d", dl, disk.Temperature))
		lines = append(lines, fmt.Sprintf("mantisops_nas_disk_power_on_hours{%s} %d", dl, disk.PowerOnHours))
		lines = append(lines, fmt.Sprintf("mantisops_nas_disk_reallocated_sectors{%s} %d", dl, disk.ReallocatedSectors))
		healthVal := 1
		if !disk.SmartHealthy {
			healthVal = 0
		}
		lines = append(lines, fmt.Sprintf("mantisops_nas_disk_smart_healthy{%s} %d", dl, healthVal))
	}
	if snap.UPS != nil {
		statusVal := 0
		switch snap.UPS.Status {
		case "on_battery":
			statusVal = 1
		case "low_battery":
			statusVal = 2
		}
		lines = append(lines, fmt.Sprintf("mantisops_nas_ups_status{%s} %d", labels, statusVal))
		lines = append(lines, fmt.Sprintf("mantisops_nas_ups_battery_percent{%s} %d", labels, snap.UPS.BatteryPercent))
	}

	if len(lines) > 0 {
		c.vm.WriteMetrics(lines)
	}
}

func isAuthError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unable to authenticate") || strings.Contains(msg, "no supported methods") || strings.Contains(msg, "permission denied")
}

// （直接用 strings.Contains + strings.ToLower）
```

注意：`c.vm.WriteMetrics(lines)` 需要确认 `VictoriaStore` 是否有 `WriteLines` 方法。查看现有 `store/victoria_store.go` 中的方法名。如果方法名不同（比如是 `ImportLines` 或 `Write`），需要适配。

- [ ] **Step 2: 验证编译**

```bash
cd server && go build ./cmd/server/
```

可能需要先添加 `golang.org/x/crypto/ssh` 依赖：

```bash
cd server && go get golang.org/x/crypto/ssh
```

检查 deployer 模块是否已引入该依赖。如果 `go.mod` 中已有则无需额外操作。

- [ ] **Step 3: Commit**

```bash
git add server/internal/collector/nas.go
git commit -m "feat(collector): add NasCollector lifecycle management and VM writing"
```

---

## Chunk 3: API 层 + 路由 + main.go 接线

### Task 8: NAS API Handler

**Files:**
- Create: `server/internal/api/nas_handler.go`

- [ ] **Step 1: 创建 NasHandler 结构体和 CRUD 方法**

```go
package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"mantisops/server/internal/collector"
	"mantisops/server/internal/store"
)

type NasHandler struct {
	nasStore     *store.NasStore
	credStore    *store.CredentialStore
	nasCollector *collector.NasCollector
}

func NewNasHandler(nasStore *store.NasStore, credStore *store.CredentialStore, nc *collector.NasCollector) *NasHandler {
	return &NasHandler{nasStore: nasStore, credStore: credStore, nasCollector: nc}
}

func (h *NasHandler) List(c *gin.Context) {
	devices, err := h.nasStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if devices == nil {
		devices = []store.NasDevice{}
	}
	c.JSON(http.StatusOK, devices)
}

func (h *NasHandler) Create(c *gin.Context) {
	var req struct {
		Name            string `json:"name" binding:"required"`
		NasType         string `json:"nas_type" binding:"required"`
		Host            string `json:"host" binding:"required"`
		Port            int    `json:"port"`
		SSHUser         string `json:"ssh_user"`
		CredentialID    int    `json:"credential_id" binding:"required"`
		CollectInterval int    `json:"collect_interval"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 校验 nas_type
	if req.NasType != "synology" && req.NasType != "fnos" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nas_type must be synology or fnos"})
		return
	}
	// 校验 credential 类型
	cred, err := h.credStore.Get(req.CredentialID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "credential not found"})
		return
	}
	if cred.Type != "ssh_password" && cred.Type != "ssh_key" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "credential must be ssh_password or ssh_key type"})
		return
	}
	if req.Port == 0 {
		req.Port = 22
	}
	if req.CollectInterval == 0 {
		req.CollectInterval = 60
	}

	id, err := h.nasStore.Create(req.Name, req.NasType, req.Host, req.Port, req.SSHUser, req.CredentialID, req.CollectInterval)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 通知 collector 启动采集
	d, _ := h.nasStore.Get(id)
	if d != nil && h.nasCollector != nil {
		h.nasCollector.AddDevice(*d)
	}

	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *NasHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		Name            string `json:"name" binding:"required"`
		NasType         string `json:"nas_type" binding:"required"`
		Host            string `json:"host" binding:"required"`
		Port            int    `json:"port"`
		SSHUser         string `json:"ssh_user"`
		CredentialID    int    `json:"credential_id" binding:"required"`
		CollectInterval int    `json:"collect_interval"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Port == 0 {
		req.Port = 22
	}
	if req.CollectInterval == 0 {
		req.CollectInterval = 60
	}

	if err := h.nasStore.Update(id, req.Name, req.NasType, req.Host, req.Port, req.SSHUser, req.CredentialID, req.CollectInterval); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 通知 collector 重启采集
	d, _ := h.nasStore.Get(id)
	if d != nil && h.nasCollector != nil {
		h.nasCollector.UpdateDevice(*d)
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *NasHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if h.nasCollector != nil {
		h.nasCollector.RemoveDevice(id)
	}
	if err := h.nasStore.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *NasHandler) TestConnection(c *gin.Context) {
	var req struct {
		Host         string `json:"host" binding:"required"`
		Port         int    `json:"port"`
		SSHUser      string `json:"ssh_user"`
		CredentialID int    `json:"credential_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Port == 0 {
		req.Port = 22
	}

	cred, err := h.credStore.Get(req.CredentialID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "credential not found"})
		return
	}

	sshUser := req.SSHUser
	if sshUser == "" {
		sshUser = "root"
	}
	client, err := collector.SSHConnect(req.Host, req.Port, sshUser, cred.Type, cred.Data["password"], cred.Data["private_key"], cred.Data["passphrase"])
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	defer client.Close()

	// 自动检测 NAS 类型
	nasType := "unknown"
	if out, err := collector.SSHExec(client, "cat /etc/synoinfo.conf 2>/dev/null | head -1", 10*1e9); err == nil && out != "" {
		nasType = "synology"
	} else if out, err := collector.SSHExec(client, "cat /etc/os-release 2>/dev/null | grep -i fnos", 10*1e9); err == nil && out != "" {
		nasType = "fnos"
	}

	// 检查 smartctl 权限
	smartOK := false
	if _, err := collector.SSHExec(client, "sudo smartctl --version 2>/dev/null", 10*1e9); err == nil {
		smartOK = true
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":             true,
		"detected_type":  nasType,
		"smart_available": smartOK,
	})
}

func (h *NasHandler) GetMetrics(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if h.nasCollector == nil {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	metrics := h.nasCollector.GetMetrics(int64(id))
	if metrics == nil {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	c.JSON(http.StatusOK, metrics)
}
```

注意：`SSHConnect` 和 `SSHExec` 已在 `nas_ssh.go` 中以大写命名导出。`sshExec` 同样需要改名为 `SSHExec`，并更新 `collectNasMetrics` 中的所有调用。

- [ ] **Step 3: 验证编译**

```bash
cd server && go build ./cmd/server/
```

- [ ] **Step 4: Commit**

```bash
git add server/internal/api/nas_handler.go server/internal/collector/nas_ssh.go
git commit -m "feat(api): add NAS handler with CRUD, test connection, and metrics endpoints"
```

### Task 9: 路由注册 + main.go 接线

**Files:**
- Modify: `server/internal/api/router.go`
- Modify: `server/cmd/server/main.go`

- [ ] **Step 1: 在 RouterDeps 中新增 NasHandler**

在 `router.go` 的 `RouterDeps` 结构体中添加：

```go
NasHandler           *NasHandler
```

- [ ] **Step 2: 在路由注册中添加 NAS 路由**

在 `SetupRouter` 函数的 `v1` 路由组中（CloudHandler 注册之后），添加：

```go
if deps.NasHandler != nil {
    v1.POST("/nas-devices/test", deps.NasHandler.TestConnection)
    v1.GET("/nas-devices", deps.NasHandler.List)
    v1.POST("/nas-devices", deps.NasHandler.Create)
    v1.PUT("/nas-devices/:id", deps.NasHandler.Update)
    v1.DELETE("/nas-devices/:id", deps.NasHandler.Delete)
    v1.GET("/nas-devices/:id/metrics", deps.NasHandler.GetMetrics)
}
```

注意：`POST /nas-devices/test` 必须在 `POST /nas-devices` 之前注册，Gin 路由树会正确区分精确路径和参数路径，但保持顺序更安全。

- [ ] **Step 3: 在 main.go 中初始化 NAS 组件**

在现有 store 初始化区域（约 line 48-52）之后添加：

```go
nasStore := store.NewNasStore(db)
```

在 alerter 创建之前（约 line 119，`alertStore` 之后、`alerter :=` 之前）添加 NAS collector 初始化：

```go
// NAS（必须在 alerter 之前，因为 alerter 需要 NasProvider）
nasCollector := collector.NewNasCollector(nasStore, credentialStore, vmStore, hub)
nasCollector.Start()
defer nasCollector.Stop()
```

修改 alerter 创建行，增加 `nasCollector` 参数：

```go
alerter := alert.NewAlerter(alertStore, hub, mc, prober, serverStore, nasCollector)
```

在 handler 初始化区域（约 line 185-192 之后）添加 handler：

```go
nasHandler := api.NewNasHandler(nasStore, credentialStore, nasCollector)
```

在 `RouterDeps` 构造中添加：

```go
NasHandler: nasHandler,
```

- [ ] **Step 4: 验证编译**

```bash
cd server && go build ./cmd/server/
```

- [ ] **Step 5: Commit**

```bash
git add server/internal/api/router.go server/cmd/server/main.go
git commit -m "feat: wire NAS collector, handler, and routes in main.go"
```

### Task 10: 审计中间件

**Files:**
- Modify: `server/internal/logging/middleware.go`

- [ ] **Step 1: 在 auditRoutes 数组中添加 NAS 条目**

在现有 `auditRoutes` 数组末尾、groups 条目之后添加（注意 test 精确匹配必须在 create 前缀匹配之前）：

```go
{"POST", "/api/v1/nas-devices/test", "test", "nas_device"},
{"POST", "/api/v1/nas-devices", "create", "nas_device"},
{"PUT", "/api/v1/nas-devices/", "update", "nas_device"},
{"DELETE", "/api/v1/nas-devices/", "delete", "nas_device"},
```

- [ ] **Step 2: 验证编译 + Commit**

```bash
cd server && go build ./cmd/server/
git add server/internal/logging/middleware.go
git commit -m "feat(audit): add NAS device audit route entries"
```

---

## Chunk 4: 告警引擎集成

### Task 11: NAS 告警评估

**Files:**
- Modify: `server/internal/alert/alerter.go`
- Modify: `server/internal/alert/evaluator.go`

- [ ] **Step 1: 在 alerter.go 中添加 NasMetricsProvider 接口**

在 `Alerter` 结构体中新增字段：

```go
nas NasProvider
```

新增接口定义（在文件头部的接口区域）：

```go
type NasProvider interface {
    GetAllMetrics() map[int64]*collector.NasMetricsSnapshot
    GetDeviceHealth(nasID int64) *collector.NasDeviceHealth
    ListDeviceIDs() []int64
}
```

修改 `NewAlerter` 签名，新增 `nas NasProvider` 参数并赋值。

- [ ] **Step 2: 在 evaluate() 中调用 evaluateNas()**

在 `evaluate()` 方法的 `a.cleanupGoneTargets(servers)` 之前添加：

```go
// NAS 评估
if a.nas != nil {
    a.evaluateNas(rules)
}
```

- [ ] **Step 3: 实现 evaluateNas() 方法**

在 `alerter.go` 中添加：

```go
func (a *Alerter) evaluateNas(rules []model.AlertRule) {
    allMetrics := a.nas.GetAllMetrics()

    for _, rule := range rules {
        if !strings.HasPrefix(rule.Type, "nas_") {
            continue
        }

        // nas_offline 走独立路径
        if rule.Type == "nas_offline" {
            a.evaluateNasOffline(rule, allMetrics)
            continue
        }

        // 按 target_id 过滤：如果规则指定了 target（如 "nas:1"），只评估该设备
        targetNasIDs := make(map[int64]bool)
        if rule.TargetID != "" {
            // 格式 "nas:1"
            parts := strings.SplitN(rule.TargetID, ":", 2)
            if len(parts) == 2 {
                id, _ := strconv.ParseInt(parts[1], 10, 64)
                if id > 0 {
                    targetNasIDs[id] = true
                }
            }
        }

        for nasID, snap := range allMetrics {
            // 如果规则指定了 target，跳过不匹配的设备
            if len(targetNasIDs) > 0 && !targetNasIDs[nasID] {
                continue
            }
            // 时间戳去重：如果与上次评估的快照相同则跳过
            stateKey := fmt.Sprintf("%d:nas:%d", rule.ID, nasID)
            a.mu.RLock()
            st := a.states[stateKey]
            a.mu.RUnlock()
            if st != nil && snap != nil && st.lastTimestamp == snap.Timestamp {
                continue
            }

            results := EvaluateNas(rule, nasID, snap)
            for _, r := range results {
                a.processResult(rule, r)
            }

            // 记录时间戳
            a.mu.Lock()
            if a.states[stateKey] == nil {
                a.states[stateKey] = &ruleState{}
            }
            if snap != nil {
                a.states[stateKey].lastTimestamp = snap.Timestamp
            }
            a.mu.Unlock()
        }
    }
}

func (a *Alerter) evaluateNasOffline(rule model.AlertRule, allMetrics map[int64]*collector.NasMetricsSnapshot) {
    allIDs := a.nas.ListDeviceIDs()
    for _, nasID := range allIDs {
        // 按 target_id 过滤
        if rule.TargetID != "" {
            parts := strings.SplitN(rule.TargetID, ":", 2)
            if len(parts) == 2 {
                tid, _ := strconv.ParseInt(parts[1], 10, 64)
                if tid > 0 && tid != nasID {
                    continue
                }
            }
        }
        targetID := fmt.Sprintf("nas:%d", nasID)
        stateKey := fmt.Sprintf("%d:%s", rule.ID, targetID)

        health := a.nas.GetDeviceHealth(nasID)
        hit := health != nil && health.FailureCount >= 3

        a.processResult(rule, EvalResult{
            StateKey: stateKey,
            TargetID: targetID,
            Hit:      hit,
            Value:    float64(health.FailureCount),
            Label:    "",
            Message:  fmt.Sprintf("NAS device %d offline (failures: %d, last: %s)", nasID, health.FailureCount, health.LastError),
        })
    }
}
```

注意：`ruleState` 结构体需要新增 `lastTimestamp time.Time` 字段。在 `alerter.go` 中找到 `type ruleState struct` 定义，添加：
```go
lastTimestamp time.Time // NAS：上次评估的快照时间戳，用于去重
```

- [ ] **Step 4: 在 evaluator.go 中添加 EvaluateNas 函数**

```go
func EvaluateNas(rule model.AlertRule, nasID int64, snap *collector.NasMetricsSnapshot) []EvalResult {
    if snap == nil {
        return nil
    }
    targetID := fmt.Sprintf("nas:%d", nasID)

    switch rule.Type {
    case "nas_raid_degraded":
        var results []EvalResult
        for _, r := range snap.Raids {
            hit := r.Status == "degraded" || r.Status == "rebuilding"
            results = append(results, EvalResult{
                StateKey: fmt.Sprintf("%d:%s:%s", rule.ID, targetID, r.Array),
                TargetID: targetID,
                Hit:      hit,
                Value:    0,
                Label:    r.Array,
                Message:  fmt.Sprintf("RAID %s status: %s", r.Array, r.Status),
            })
        }
        return results

    case "nas_disk_smart":
        var results []EvalResult
        for _, d := range snap.Disks {
            results = append(results, EvalResult{
                StateKey: fmt.Sprintf("%d:%s:%s", rule.ID, targetID, d.Name),
                TargetID: targetID,
                Hit:      !d.SmartHealthy,
                Value:    0,
                Label:    d.Name,
                Message:  fmt.Sprintf("Disk %s S.M.A.R.T. %s", d.Name, map[bool]string{true: "healthy", false: "FAILED"}[d.SmartHealthy]),
            })
        }
        return results

    case "nas_disk_temperature":
        var results []EvalResult
        for _, d := range snap.Disks {
            results = append(results, EvalResult{
                StateKey: fmt.Sprintf("%d:%s:%s", rule.ID, targetID, d.Name),
                TargetID: targetID,
                Hit:      float64(d.Temperature) > rule.Threshold,
                Value:    float64(d.Temperature),
                Label:    d.Name,
                Message:  fmt.Sprintf("Disk %s temperature: %d°C", d.Name, d.Temperature),
            })
        }
        return results

    case "nas_volume_usage":
        var results []EvalResult
        for _, v := range snap.Volumes {
            results = append(results, EvalResult{
                StateKey: fmt.Sprintf("%d:%s:%s", rule.ID, targetID, v.Mount),
                TargetID: targetID,
                Hit:      v.UsagePercent > rule.Threshold,
                Value:    v.UsagePercent,
                Label:    v.Mount,
                Message:  fmt.Sprintf("Volume %s usage: %.1f%%", v.Mount, v.UsagePercent),
            })
        }
        return results

    case "nas_ups_battery":
        if snap.UPS == nil {
            return nil
        }
        hit := snap.UPS.Status == "on_battery" || snap.UPS.Status == "low_battery"
        return []EvalResult{{
            StateKey: fmt.Sprintf("%d:%s:ups", rule.ID, targetID),
            TargetID: targetID,
            Hit:      hit,
            Value:    0,
            Label:    "UPS",
            Message:  fmt.Sprintf("UPS status: %s (battery: %d%%)", snap.UPS.Status, snap.UPS.BatteryPercent),
        }}
    }

    return nil
}
```

- [ ] **Step 5: 更新 cleanupGoneTargets**

在 `cleanupGoneTargets` 方法中，增加 NAS 目标的存活判断。在现有的 serverIDs/probeIDs 构建之后，新增 NAS ID 集合：

```go
// NAS device IDs
nasIDs := make(map[string]bool)
if a.nas != nil {
    for _, id := range a.nas.ListDeviceIDs() {
        nasIDs[fmt.Sprintf("nas:%d", id)] = true
    }
}
```

在 state key 遍历循环中，当 `parts[1] == "nas"` 时，用 `nasIDs[parts[1]+":"+parts[2]]` 判断存活：

```go
// 在 isTargetPresent 逻辑中增加:
if len(parts) >= 3 && parts[1] == "nas" {
    nasKey := parts[1] + ":" + parts[2]  // "nas:1"
    if !nasIDs[nasKey] {
        // NAS 已删除，自动恢复关联告警
    }
    continue
}
```

- [ ] **Step 6: 更新 main.go 中 NewAlerter 的调用**

将 `NewAlerter` 调用增加 `nasCollector` 参数：

```go
alerter := alert.NewAlerter(alertStore, hub, mc, prober, serverStore, nasCollector)
```

- [ ] **Step 7: 验证编译**

```bash
cd server && go build ./cmd/server/
```

- [ ] **Step 8: Commit**

```bash
git add server/internal/alert/alerter.go server/internal/alert/evaluator.go server/cmd/server/main.go
git commit -m "feat(alert): add NAS alert evaluation (RAID/SMART/temperature/volume/UPS)"
```

---

## Chunk 5: 前端 — API 客户端 + Store + 路由 + 侧边栏 + WebSocket

### Task 12: NAS API 客户端

**Files:**
- Create: `web/src/api/nas.ts`

- [ ] **Step 1: 创建 API 客户端**

```typescript
import api from './client'

export interface NasDevice {
  id: number
  name: string
  nas_type: 'synology' | 'fnos'
  host: string
  port: number
  ssh_user: string
  credential_id: number
  collect_interval: number
  status: 'online' | 'offline' | 'degraded' | 'unknown'
  last_seen: string | null
  system_info: string
  created_at: string
  updated_at: string
}

export interface NasCPU { usage_percent: number }
export interface NasMemory { total: number; used: number; usage_percent: number }
export interface NasNetwork { interface: string; rx_bytes_per_sec: number; tx_bytes_per_sec: number }
export interface NasRaid { array: string; raid_type: string; status: string; disks: string[]; rebuild_percent: number }
export interface NasVolume { mount: string; fs_type: string; total: number; used: number; usage_percent: number }
export interface NasDisk { name: string; model: string; size: number; temperature: number; power_on_hours: number; smart_healthy: boolean; reallocated_sectors: number }
export interface NasUPS { status: string; battery_percent: number; model: string }

export interface NasMetrics {
  nas_id: number
  timestamp: string
  cpu?: NasCPU
  memory?: NasMemory
  networks?: NasNetwork[]
  raids?: NasRaid[]
  volumes?: NasVolume[]
  disks?: NasDisk[]
  ups?: NasUPS
}

export async function getNasDevices(): Promise<NasDevice[]> {
  const { data } = await api.get('/nas-devices')
  return data
}

export async function createNasDevice(req: { name: string; nas_type: string; host: string; port: number; ssh_user: string; credential_id: number; collect_interval: number }): Promise<{ id: number }> {
  const { data } = await api.post('/nas-devices', req)
  return data
}

export async function updateNasDevice(id: number, req: { name: string; nas_type: string; host: string; port: number; ssh_user: string; credential_id: number; collect_interval: number }): Promise<void> {
  await api.put(`/nas-devices/${id}`, req)
}

export async function deleteNasDevice(id: number): Promise<void> {
  await api.delete(`/nas-devices/${id}`)
}

export async function testNasConnection(req: { host: string; port: number; ssh_user: string; credential_id: number }): Promise<{ ok: boolean; error?: string; detected_type?: string; smart_available?: boolean }> {
  const { data } = await api.post('/nas-devices/test', req)
  return data
}

export async function getNasMetrics(id: number): Promise<NasMetrics> {
  const { data } = await api.get(`/nas-devices/${id}/metrics`)
  return data
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/api/nas.ts
git commit -m "feat(web): add NAS API client"
```

### Task 13: NAS Zustand Store

**Files:**
- Create: `web/src/stores/nasStore.ts`

- [ ] **Step 1: 创建 store**

```typescript
import { create } from 'zustand'
import { getNasDevices, getNasMetrics, type NasDevice, type NasMetrics } from '../api/nas'

interface NasState {
  devices: NasDevice[]
  metrics: Record<number, NasMetrics>
  loading: boolean
  fetchDevices: () => Promise<void>
  updateMetrics: (nasId: number, data: NasMetrics) => void
  updateStatus: (nasId: number, status: string) => void
}

export const useNasStore = create<NasState>((set) => ({
  devices: [],
  metrics: {},
  loading: false,
  fetchDevices: async () => {
    set({ loading: true })
    try {
      const devices = await getNasDevices()
      set({ devices: devices || [] })
      // 首屏：批量加载所有设备的缓存指标
      const metricsMap: Record<number, NasMetrics> = {}
      await Promise.all(
        (devices || []).map(async (d) => {
          try {
            const m = await getNasMetrics(d.id)
            if (m && m.timestamp) metricsMap[d.id] = m
          } catch { /* 设备可能尚无指标 */ }
        })
      )
      set({ metrics: metricsMap })
    } finally {
      set({ loading: false })
    }
  },
  updateMetrics: (nasId, data) =>
    set((state) => ({
      metrics: { ...state.metrics, [nasId]: data },
    })),
  updateStatus: (nasId, status) =>
    set((state) => ({
      devices: state.devices.map((d) =>
        d.id === nasId ? { ...d, status: status as NasDevice['status'] } : d
      ),
    })),
}))
```

- [ ] **Step 2: Commit**

```bash
git add web/src/stores/nasStore.ts
git commit -m "feat(web): add NAS Zustand store"
```

### Task 14: WebSocket NAS 消息处理

**Files:**
- Modify: `web/src/hooks/useWebSocket.ts`

- [ ] **Step 1: 在 onmessage handler 中添加 NAS 消息处理**

在现有 `ws.onmessage` 的 try 块中，`log` 消息处理之后添加：

```typescript
// nas_metrics: NAS metrics update
if (msg.type === 'nas_metrics' && msg.nas_id && msg.data) {
  window.dispatchEvent(new CustomEvent('nas_metrics', { detail: { nas_id: msg.nas_id, data: msg.data } }))
}

// nas_status: NAS status change
if (msg.type === 'nas_status' && msg.nas_id) {
  window.dispatchEvent(new CustomEvent('nas_status', { detail: { nas_id: msg.nas_id, status: msg.status } }))
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/hooks/useWebSocket.ts
git commit -m "feat(web): add NAS metrics/status WebSocket message handling"
```

### Task 15: 侧边栏 + 路由

**Files:**
- Modify: `web/src/components/Layout/Sidebar.tsx`
- Modify: `web/src/App.tsx`

- [ ] **Step 1: 在 Sidebar.tsx 的 links 数组中添加 NAS 菜单项**

在 `{ to: '/servers', label: '服务器', icon: 'dns' }` 之后、`{ to: '/databases' ... }` 之前添加：

```typescript
{ to: '/nas', label: 'NAS 存储', icon: 'hard_drive' },
```

- [ ] **Step 2: 在 App.tsx 中注册 NAS 路由**

在文件顶部添加 lazy import：

```typescript
import NAS from './pages/NAS'
import NASDetail from './pages/NAS/NASDetail'
```

在 `<Route path="/databases/:id" ...>` 之后添加：

```typescript
<Route path="/nas" element={<NAS />} />
<Route path="/nas/:id" element={<NASDetail />} />
```

- [ ] **Step 3: 创建占位页面（确保编译通过）**

创建 `web/src/pages/NAS/index.tsx`：

```typescript
export default function NAS() {
  return <div className="p-6"><h1 className="text-xl font-semibold text-[#495057]">NAS 存储</h1><p className="text-[#878a99] mt-2">加载中...</p></div>
}
```

创建 `web/src/pages/NAS/NASDetail.tsx`：

```typescript
export default function NASDetail() {
  return <div className="p-6"><h1 className="text-xl font-semibold text-[#495057]">NAS 详情</h1><p className="text-[#878a99] mt-2">加载中...</p></div>
}
```

- [ ] **Step 4: 验证前端编译**

```bash
cd web && npx tsc --noEmit
```

- [ ] **Step 5: Commit**

```bash
git add web/src/components/Layout/Sidebar.tsx web/src/App.tsx web/src/pages/NAS/
git commit -m "feat(web): add NAS sidebar menu, routes, and placeholder pages"
```

---

## Chunk 6: 前端 — NAS 列表页 + 详情页 + 设置页管理

### Task 16: NAS 列表页

**Files:**
- Modify: `web/src/pages/NAS/index.tsx`

- [ ] **Step 1: 实现完整的 NAS 列表页**

参考现有 Servers 页面的卡片布局模式，实现：
- 顶部统计卡片（设备总数、在线数、RAID 降级数、磁盘异常数）
- NAS 设备卡片列表（每张卡片含：名称、类型、IP、状态、CPU/内存进度条、存储池摘要、硬盘温度、网络吞吐、UPS 状态）
- 监听 `nas_metrics` 和 `nas_status` CustomEvent 实时更新 store

**首屏指标加载**：WebSocket 广播间隔为 60 秒，首屏会空一段时间。解决方案：页面 mount 时，对每个设备调用 `getNasMetrics(id)` 获取缓存的最新快照，填充 store。后续由 WebSocket 事件覆盖更新。参考现有 Dashboard 页面的 `fetchDashboard()` 模式（REST 加载初始数据 + WebSocket 实时覆盖）。

在 `nasStore.ts` 的 `fetchDevices` 方法中，加载设备列表后批量拉取指标：

```typescript
fetchDevices: async () => {
    set({ loading: true })
    try {
        const devices = await getNasDevices()
        set({ devices: devices || [] })
        // 首屏：批量加载所有设备的缓存指标
        const metricsMap: Record<number, NasMetrics> = {}
        await Promise.all(
            (devices || []).map(async (d) => {
                try {
                    const m = await getNasMetrics(d.id)
                    if (m && m.timestamp) metricsMap[d.id] = m
                } catch { /* 设备可能尚无指标，忽略 */ }
            })
        )
        set({ metrics: metricsMap })
    } finally {
        set({ loading: false })
    }
},
```

代码较长，实现时参考 spec 文档 5.3 节的 wireframe 和现有 `pages/Servers/` 的组件模式。

- [ ] **Step 2: 验证前端编译 + 视觉检查**

```bash
cd web && npx tsc --noEmit
```

启动 dev server 手动检查页面渲染。

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/NAS/index.tsx
git commit -m "feat(web): implement NAS list page with real-time metric cards"
```

### Task 17: NAS 详情页

**Files:**
- Modify: `web/src/pages/NAS/NASDetail.tsx`

- [ ] **Step 1: 实现完整的 NAS 详情页**

参考 spec 文档 5.4 节的 wireframe，实现：
- 设备信息头（名称、类型、版本、状态）
- 实时概览（CPU/内存/网络）
- RAID 阵列状态表格
- 存储卷用量进度条
- 硬盘健康表格（含 S.M.A.R.T. 展开详情）
- UPS 电源状态
- 历史趋势图表（复用 VictoriaMetrics 查询模式，参考 ServerDetail 页面的图表实现）
- 群晖套件列表（仅 synology 类型显示）

使用 `useParams` 获取 `:id`，调用 `getNasMetrics(id)` 加载初始数据，监听 WebSocket 事件实时更新。

- [ ] **Step 2: 验证前端编译**

```bash
cd web && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/NAS/NASDetail.tsx
git commit -m "feat(web): implement NAS detail page with RAID/SMART/volume/UPS sections"
```

### Task 18: 设置页 NAS 管理区块

**Files:**
- Modify: `web/src/pages/Settings/index.tsx`

- [ ] **Step 1: 添加 NAS 设备管理区块**

在设置页的"托管服务器"区块之前，新增 NAS 设备管理区块：

1. 加载 NAS 设备列表和凭据列表（复用现有 `fetchCredentials`）
2. 设备表格：名称、类型（Synology/fnOS）、地址、采集间隔、状态指示灯、编辑/删除按钮
3. "添加 NAS" 按钮 → 弹出对话框：
   - 名称输入框
   - NAS 类型下拉（Synology / fnOS）
   - 地址 + 端口输入框
   - SSH 凭据下拉（**仅显示 ssh_password / ssh_key 类型**），尾部有"+ 新建 SSH 凭据"
   - 采集间隔输入框
   - "测试连接"按钮（调用 `testNasConnection`，显示结果：连通性、自动检测类型、smartctl 可用性）
   - 确认/取消按钮

参考现有"托管服务器"区块的 UI 模式（表格 + 弹窗 CRUD）。

- [ ] **Step 2: 验证前端编译**

```bash
cd web && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/Settings/index.tsx
git commit -m "feat(web): add NAS device management section in Settings page"
```

### Task 19: 告警规则 UI 支持 NAS 目标

**Files:**
- Modify: 告警规则创建/编辑页面（查找 `web/src/pages/Alerts/` 中的规则表单组件）

- [ ] **Step 1: 在告警规则类型下拉中添加 NAS 类型**

找到告警规则创建/编辑组件中 `type` 下拉选项，新增以下选项：
- `nas_offline` — NAS 离线
- `nas_raid_degraded` — RAID 降级
- `nas_disk_smart` — 硬盘 S.M.A.R.T. 异常
- `nas_disk_temperature` — 硬盘温度过高
- `nas_volume_usage` — 存储卷空间不足
- `nas_ups_battery` — UPS 电池供电

- [ ] **Step 2: 目标选择器支持 NAS 设备**

当选中 `nas_*` 类型时，目标下拉列表应展示 NAS 设备列表（从 `getNasDevices()` 获取）而非服务器列表。`target_id` 格式为 `nas:{id}`。

- [ ] **Step 3: 验证前端编译 + Commit**

```bash
cd web && npx tsc --noEmit
git add web/src/pages/Alerts/
git commit -m "feat(web): add NAS alert rule types and target selection in alert UI"
```

---

## Chunk 7: 端到端验证

### Task 20: 全量编译 + 启动验证

- [ ] **Step 1: 后端全量编译**

```bash
cd server && go build ./cmd/server/
```

- [ ] **Step 2: 前端类型检查 + 构建**

```bash
cd web && npx tsc --noEmit && npm run build
```

- [ ] **Step 3: 启动服务验证**

```bash
cd server && go run ./cmd/server/
```

检查启动日志中是否有 `[nas] started collectors for 0 devices`（初始无 NAS 设备是正常的）。

- [ ] **Step 4: API 冒烟测试**

```bash
# 获取 token
TOKEN=$(curl -s -X POST http://localhost:3100/api/v1/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"xxx"}' | jq -r .token)

# NAS 列表（应返回空数组）
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:3100/api/v1/nas-devices

# 测试连接（需要真实 NAS IP 和凭据）
# curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' http://localhost:3100/api/v1/nas-devices/test -d '{"host":"192.168.1.100","port":22,"credential_id":1}'
```

- [ ] **Step 5: 最终 Commit**

```bash
git add -A
git commit -m "feat: NAS monitoring module - complete implementation"
```
