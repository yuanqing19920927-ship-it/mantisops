package collector

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// NasMetricsSnapshot 是一次完整采集的快照
type NasMetricsSnapshot struct {
	NasID     int64        `json:"nas_id"`
	Timestamp time.Time    `json:"timestamp"`
	CPU       *NasCPU      `json:"cpu,omitempty"`
	Memory    *NasMemory   `json:"memory,omitempty"`
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
	Interface     string `json:"interface"`
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

// SSHExec 在已有连接上执行单条命令
func SSHExec(client *ssh.Client, cmd string, timeout time.Duration) (string, error) {
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
				!strings.Contains(mount, "/docker/") &&
				fsType != "overlay" &&
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

// collectNasMetrics SSH 连接并采集所有指标
func collectNasMetrics(client *ssh.Client, nasType string, prev *NasRawCounters, interval int) (*NasMetricsSnapshot, *NasRawCounters, error) {
	timeout := 30 * time.Second
	snap := &NasMetricsSnapshot{Timestamp: time.Now()}
	newRaw := &NasRawCounters{NetRx: make(map[string]uint64), NetTx: make(map[string]uint64)}

	// 1. CPU + 内存（一条命令）
	out, err := SSHExec(client, "cat /proc/stat && echo '---SEPARATOR---' && cat /proc/meminfo", timeout)
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
	out, err = SSHExec(client, "cat /proc/net/dev", timeout)
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
	out, err = SSHExec(client, "cat /proc/mdstat", timeout)
	if err == nil {
		snap.Raids = parseMdstat(out)
	}

	// 4. 卷用量
	out, err = SSHExec(client, "df -T -B1 -x tmpfs -x devtmpfs -x squashfs", timeout)
	if err == nil {
		snap.Volumes = parseDf(out, nasType)
	}

	// 5. 硬盘列表 + S.M.A.R.T.
	out, err = SSHExec(client, "lsblk -nd -o NAME,SIZE,MODEL,TYPE -b", timeout)
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
			smartOut, smartErr := SSHExec(client, fmt.Sprintf("sudo smartctl -A -H /dev/%s 2>/dev/null", disk.Name), timeout)
			if smartErr == nil {
				disk.Temperature, disk.PowerOnHours, disk.ReallocatedSectors, disk.SmartHealthy = parseSmartctl(smartOut)
			} else {
				disk.SmartHealthy = true // 无法获取时默认健康
			}
			snap.Disks = append(snap.Disks, disk)
		}
	}

	// 6. UPS
	out, _ = SSHExec(client, "upsc ups@localhost 2>/dev/null", timeout)
	snap.UPS = parseUpsc(out)

	return snap, newRaw, nil
}
