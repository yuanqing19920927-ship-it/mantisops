package alert

import (
	"encoding/json"
	"fmt"
	"time"

	"mantisops/server/internal/collector"
	"mantisops/server/internal/model"
	pb "mantisops/server/proto/gen"
)

// EvalResult represents evaluation result for one target
type EvalResult struct {
	StateKey string  // e.g. "3:srv-65-host" or "3:srv-65-host:/"
	TargetID string  // stored in alert_events.target_id
	Hit      bool    // whether threshold was breached
	Value    float64 // current metric value
	Skip     bool    // skip if data not fresh
	Label    string  // human-readable target label for alert_events.target_label
	Message  string  // alert description message
}

// MetricsProvider interface for reading cached metrics
type MetricsProvider interface {
	GetLatestMetrics(hostID string) *pb.MetricsPayload
	GetAllCachedHosts() []string
}

// ProbeProvider interface for reading probe results
type ProbeProvider interface {
	GetAllResults() []*model.ProbeResult
}

// ServerProvider interface for reading server info
type ServerProvider interface {
	List() ([]model.Server, error)
}

// Evaluate evaluates a rule against all applicable targets
func Evaluate(rule model.AlertRule, servers []model.Server, metrics MetricsProvider, probes ProbeProvider) []EvalResult {
	switch rule.Type {
	case "server_offline":
		return evalServerOffline(rule, servers)
	case "probe_down":
		return evalProbeDown(rule, probes)
	case "cpu":
		return evalSimpleMetric(rule, servers, metrics, func(p *pb.MetricsPayload) (float64, bool) {
			if p.Cpu == nil {
				return 0, false
			}
			return p.Cpu.UsagePercent, true
		})
	case "memory":
		return evalSimpleMetric(rule, servers, metrics, func(p *pb.MetricsPayload) (float64, bool) {
			if p.Memory == nil {
				return 0, false
			}
			return p.Memory.UsagePercent, true
		})
	case "gpu_temp":
		return evalSimpleMetric(rule, servers, metrics, func(p *pb.MetricsPayload) (float64, bool) {
			if p.Gpu == nil {
				return 0, false
			}
			return p.Gpu.Temperature, true
		})
	case "gpu_memory":
		return evalSimpleMetric(rule, servers, metrics, func(p *pb.MetricsPayload) (float64, bool) {
			if p.Gpu == nil {
				return 0, false
			}
			if p.Gpu.MemoryTotal == 0 {
				return 0, false
			}
			return float64(p.Gpu.MemoryUsed) / float64(p.Gpu.MemoryTotal) * 100, true
		})
	case "disk":
		return evalDisk(rule, servers, metrics)
	case "container":
		return evalContainer(rule, servers, metrics)
	case "network_rx":
		return evalNetwork(rule, servers, metrics, true)
	case "network_tx":
		return evalNetwork(rule, servers, metrics, false)
	default:
		return nil
	}
}

func compare(value float64, operator string, threshold float64) bool {
	switch operator {
	case ">":
		return value > threshold
	case "<":
		return value < threshold
	case ">=":
		return value >= threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	case "!=":
		return value != threshold
	default:
		return false
	}
}

func serverLabel(s model.Server) string {
	ips := ""
	var ipList []string
	if err := json.Unmarshal([]byte(s.IPAddresses), &ipList); err == nil && len(ipList) > 0 {
		ips = ipList[0]
	}
	name := s.DisplayName
	if name == "" {
		name = s.Hostname
	}
	if ips != "" {
		return fmt.Sprintf("%s (%s)", name, ips)
	}
	return name
}

func targetServers(rule model.AlertRule, servers []model.Server) []model.Server {
	if rule.TargetID == "" {
		return servers
	}
	for _, s := range servers {
		if s.HostID == rule.TargetID {
			return []model.Server{s}
		}
	}
	return nil
}

func evalServerOffline(rule model.AlertRule, servers []model.Server) []EvalResult {
	targets := targetServers(rule, servers)
	var results []EvalResult
	for _, s := range targets {
		elapsed := float64(time.Now().Unix() - s.LastSeen)
		hit := compare(elapsed, rule.Operator, rule.Threshold)
		results = append(results, EvalResult{
			StateKey: fmt.Sprintf("%d:%s", rule.ID, s.HostID),
			TargetID: s.HostID,
			Hit:      hit,
			Value:    elapsed,
			Label:    serverLabel(s),
			Message:  fmt.Sprintf("服务器 %s 已离线 %.0f 秒 (阈值: %s %.0f 秒)", serverLabel(s), elapsed, rule.Operator, rule.Threshold),
		})
	}
	return results
}

func evalProbeDown(rule model.AlertRule, probes ProbeProvider) []EvalResult {
	allResults := probes.GetAllResults()
	var results []EvalResult
	for _, pr := range allResults {
		if rule.TargetID != "" && rule.TargetID != fmt.Sprintf("%d", pr.RuleID) {
			continue
		}
		hit := pr.Status == "down"
		results = append(results, EvalResult{
			StateKey: fmt.Sprintf("%d:%d", rule.ID, pr.RuleID),
			TargetID: fmt.Sprintf("%d", pr.RuleID),
			Hit:      hit,
			Value:    0,
			Label:    fmt.Sprintf("%s (%s:%d)", pr.Name, pr.Host, pr.Port),
			Message:  fmt.Sprintf("端口探测 %s (%s:%d) 状态: %s", pr.Name, pr.Host, pr.Port, pr.Status),
		})
	}
	return results
}

// evalSimpleMetric evaluates single-value metrics (cpu, memory, gpu_temp, gpu_memory)
func evalSimpleMetric(rule model.AlertRule, servers []model.Server, metrics MetricsProvider, extract func(*pb.MetricsPayload) (float64, bool)) []EvalResult {
	targets := targetServers(rule, servers)
	var results []EvalResult
	for _, s := range targets {
		m := metrics.GetLatestMetrics(s.HostID)
		if m == nil {
			results = append(results, EvalResult{StateKey: fmt.Sprintf("%d:%s", rule.ID, s.HostID), Skip: true})
			continue
		}
		val, ok := extract(m)
		if !ok {
			results = append(results, EvalResult{StateKey: fmt.Sprintf("%d:%s", rule.ID, s.HostID), Skip: true})
			continue
		}
		hit := compare(val, rule.Operator, rule.Threshold)
		results = append(results, EvalResult{
			StateKey: fmt.Sprintf("%d:%s", rule.ID, s.HostID),
			TargetID: s.HostID,
			Hit:      hit,
			Value:    val,
			Label:    serverLabel(s),
			Message:  fmt.Sprintf("%s %s: %.2f%s (阈值: %s %.2f%s)", serverLabel(s), rule.Name, val, rule.Unit, rule.Operator, rule.Threshold, rule.Unit),
		})
	}
	return results
}

// evalDisk evaluates per-mount-point, each independently tracked
func evalDisk(rule model.AlertRule, servers []model.Server, metrics MetricsProvider) []EvalResult {
	targets := targetServers(rule, servers)
	var results []EvalResult
	for _, s := range targets {
		m := metrics.GetLatestMetrics(s.HostID)
		if m == nil {
			continue
		}
		for _, d := range m.Disks {
			val := d.UsagePercent
			hit := compare(val, rule.Operator, rule.Threshold)
			results = append(results, EvalResult{
				StateKey: fmt.Sprintf("%d:%s:%s", rule.ID, s.HostID, d.MountPoint),
				TargetID: fmt.Sprintf("%s:%s", s.HostID, d.MountPoint),
				Hit:      hit,
				Value:    val,
				Label:    fmt.Sprintf("%s [%s]", serverLabel(s), d.MountPoint),
				Message:  fmt.Sprintf("%s 磁盘 %s: %.1f%% (阈值: %s %.1f%%)", serverLabel(s), d.MountPoint, val, rule.Operator, rule.Threshold),
			})
		}
	}
	return results
}

// evalContainer evaluates per-container, non-running = hit
func evalContainer(rule model.AlertRule, servers []model.Server, metrics MetricsProvider) []EvalResult {
	targets := targetServers(rule, servers)
	var results []EvalResult
	for _, s := range targets {
		m := metrics.GetLatestMetrics(s.HostID)
		if m == nil {
			continue
		}
		for _, c := range m.Containers {
			hit := c.State != "running"
			results = append(results, EvalResult{
				StateKey: fmt.Sprintf("%d:%s:%s", rule.ID, s.HostID, c.Name),
				TargetID: fmt.Sprintf("%s:%s", s.HostID, c.Name),
				Hit:      hit,
				Value:    0,
				Label:    fmt.Sprintf("%s [%s]", serverLabel(s), c.Name),
				Message:  fmt.Sprintf("%s 容器 %s 状态: %s", serverLabel(s), c.Name, c.State),
			})
		}
	}
	return results
}

// EvaluateNas evaluates a NAS alert rule against a single NAS device's metrics snapshot.
func EvaluateNas(rule model.AlertRule, nasID int64, snap *collector.NasMetricsSnapshot) []EvalResult {
	if snap == nil {
		return nil
	}
	targetID := fmt.Sprintf("nas:%d", nasID)

	switch rule.Type {
	case "nas_raid_degraded":
		var results []EvalResult
		for _, raid := range snap.Raids {
			hit := raid.Status == "degraded" || raid.Status == "rebuilding"
			stateKey := fmt.Sprintf("%d:%s:%s", rule.ID, targetID, raid.Array)
			msg := fmt.Sprintf("NAS RAID %s 状态: %s", raid.Array, raid.Status)
			if raid.Status == "rebuilding" {
				msg = fmt.Sprintf("NAS RAID %s 重建中 (%.1f%%)", raid.Array, raid.RebuildPercent)
			}
			results = append(results, EvalResult{
				StateKey: stateKey,
				TargetID: targetID,
				Hit:      hit,
				Value:    0,
				Label:    fmt.Sprintf("NAS %d [%s]", nasID, raid.Array),
				Message:  msg,
			})
		}
		return results

	case "nas_disk_smart":
		var results []EvalResult
		for _, disk := range snap.Disks {
			hit := !disk.SmartHealthy
			stateKey := fmt.Sprintf("%d:%s:%s", rule.ID, targetID, disk.Name)
			results = append(results, EvalResult{
				StateKey: stateKey,
				TargetID: targetID,
				Hit:      hit,
				Value:    0,
				Label:    fmt.Sprintf("NAS %d [%s]", nasID, disk.Name),
				Message:  fmt.Sprintf("NAS 磁盘 %s SMART 状态: %v", disk.Name, disk.SmartHealthy),
			})
		}
		return results

	case "nas_disk_temperature":
		var results []EvalResult
		for _, disk := range snap.Disks {
			val := float64(disk.Temperature)
			hit := compare(val, rule.Operator, rule.Threshold)
			stateKey := fmt.Sprintf("%d:%s:%s", rule.ID, targetID, disk.Name)
			results = append(results, EvalResult{
				StateKey: stateKey,
				TargetID: targetID,
				Hit:      hit,
				Value:    val,
				Label:    fmt.Sprintf("NAS %d [%s]", nasID, disk.Name),
				Message:  fmt.Sprintf("NAS 磁盘 %s 温度: %.0f°C (阈值: %s %.0f°C)", disk.Name, val, rule.Operator, rule.Threshold),
			})
		}
		return results

	case "nas_volume_usage":
		var results []EvalResult
		for _, vol := range snap.Volumes {
			val := vol.UsagePercent
			hit := compare(val, rule.Operator, rule.Threshold)
			stateKey := fmt.Sprintf("%d:%s:%s", rule.ID, targetID, vol.Mount)
			results = append(results, EvalResult{
				StateKey: stateKey,
				TargetID: targetID,
				Hit:      hit,
				Value:    val,
				Label:    fmt.Sprintf("NAS %d [%s]", nasID, vol.Mount),
				Message:  fmt.Sprintf("NAS 卷 %s 使用率: %.1f%% (阈值: %s %.1f%%)", vol.Mount, val, rule.Operator, rule.Threshold),
			})
		}
		return results

	case "nas_ups_battery":
		if snap.UPS == nil {
			return nil
		}
		hit := snap.UPS.Status == "on_battery" || snap.UPS.Status == "low_battery"
		stateKey := fmt.Sprintf("%d:%s:ups", rule.ID, targetID)
		return []EvalResult{{
			StateKey: stateKey,
			TargetID: targetID,
			Hit:      hit,
			Value:    float64(snap.UPS.BatteryPercent),
			Label:    fmt.Sprintf("NAS %d [UPS]", nasID),
			Message:  fmt.Sprintf("NAS UPS 状态: %s (电量: %d%%)", snap.UPS.Status, snap.UPS.BatteryPercent),
		}}
	}

	return nil
}

// evalNetwork evaluates aggregated network traffic across all interfaces
func evalNetwork(rule model.AlertRule, servers []model.Server, metrics MetricsProvider, isRx bool) []EvalResult {
	targets := targetServers(rule, servers)
	var results []EvalResult
	for _, s := range targets {
		m := metrics.GetLatestMetrics(s.HostID)
		if m == nil {
			results = append(results, EvalResult{StateKey: fmt.Sprintf("%d:%s", rule.ID, s.HostID), Skip: true})
			continue
		}
		var total float64
		for _, n := range m.Networks {
			if isRx {
				total += n.RxBytesPerSec
			} else {
				total += n.TxBytesPerSec
			}
		}
		// Convert bytes/sec to MB/s for comparison (threshold is in MB/s)
		totalMBps := total / 1024 / 1024
		hit := compare(totalMBps, rule.Operator, rule.Threshold)
		direction := "入站"
		if !isRx {
			direction = "出站"
		}
		results = append(results, EvalResult{
			StateKey: fmt.Sprintf("%d:%s", rule.ID, s.HostID),
			TargetID: s.HostID,
			Hit:      hit,
			Value:    totalMBps,
			Label:    serverLabel(s),
			Message:  fmt.Sprintf("%s 网络%s: %.2f MB/s (阈值: %s %.2f MB/s)", serverLabel(s), direction, totalMBps, rule.Operator, rule.Threshold),
		})
	}
	return results
}
