package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"mantisops/server/internal/store"
	"mantisops/server/internal/ws"
)

// NasCollector manages per-device collection goroutines.
type NasCollector struct {
	nasStore  *store.NasStore
	credStore *store.CredentialStore
	vm        *store.VictoriaStore
	hub       *ws.Hub

	cache   map[int64]*NasMetricsSnapshot
	prevRaw map[int64]*NasRawCounters
	health  map[int64]*NasDeviceHealth
	workers map[int64]context.CancelFunc
	mu      sync.RWMutex

	stopCh chan struct{}
}

// NewNasCollector creates a new NasCollector.
func NewNasCollector(
	nasStore *store.NasStore,
	credStore *store.CredentialStore,
	vm *store.VictoriaStore,
	hub *ws.Hub,
) *NasCollector {
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

// Start loads all devices from the database and starts a worker goroutine for each.
func (nc *NasCollector) Start() {
	devices, err := nc.nasStore.List()
	if err != nil {
		log.Printf("[nas-collector] failed to list devices: %v", err)
		return
	}
	for _, d := range devices {
		nc.startWorker(d)
	}
	log.Printf("[nas-collector] started %d device workers", len(devices))
}

// Stop signals all workers to stop and waits for cleanup.
func (nc *NasCollector) Stop() {
	close(nc.stopCh)
	nc.mu.Lock()
	defer nc.mu.Unlock()
	for id, cancel := range nc.workers {
		cancel()
		delete(nc.workers, id)
	}
	log.Printf("[nas-collector] stopped all workers")
}

// AddDevice starts a new worker for the given device.
func (nc *NasCollector) AddDevice(d store.NasDevice) {
	nc.startWorker(d)
}

// RemoveDevice cancels the worker for the given device and cleans up caches.
func (nc *NasCollector) RemoveDevice(id int) {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	nasID := int64(id)
	if cancel, ok := nc.workers[nasID]; ok {
		cancel()
		delete(nc.workers, nasID)
	}
	delete(nc.cache, nasID)
	delete(nc.prevRaw, nasID)
	delete(nc.health, nasID)
}

// UpdateDevice restarts the worker with updated device config.
func (nc *NasCollector) UpdateDevice(d store.NasDevice) {
	nc.RemoveDevice(d.ID)
	nc.startWorker(d)
}

// GetMetrics returns the cached metrics snapshot for a NAS device.
func (nc *NasCollector) GetMetrics(nasID int64) *NasMetricsSnapshot {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	return nc.cache[nasID]
}

// GetAllMetrics returns a copy of all cached snapshots.
func (nc *NasCollector) GetAllMetrics() map[int64]*NasMetricsSnapshot {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	result := make(map[int64]*NasMetricsSnapshot, len(nc.cache))
	for k, v := range nc.cache {
		result[k] = v
	}
	return result
}

// ListDeviceIDs loads device IDs from the database.
func (nc *NasCollector) ListDeviceIDs() []int64 {
	devices, err := nc.nasStore.List()
	if err != nil {
		log.Printf("[nas-collector] list device IDs: %v", err)
		return nil
	}
	ids := make([]int64, 0, len(devices))
	for _, d := range devices {
		ids = append(ids, int64(d.ID))
	}
	return ids
}

// GetDeviceHealth returns the health status for a NAS device.
func (nc *NasCollector) GetDeviceHealth(nasID int64) *NasDeviceHealth {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	return nc.health[nasID]
}

// startWorker creates a context and launches the collectLoop goroutine.
func (nc *NasCollector) startWorker(d store.NasDevice) {
	nasID := int64(d.ID)
	nc.mu.Lock()
	// Cancel existing worker if any
	if cancel, ok := nc.workers[nasID]; ok {
		cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	nc.workers[nasID] = cancel
	nc.health[nasID] = &NasDeviceHealth{}
	nc.mu.Unlock()

	go nc.collectLoop(ctx, d)
}

// collectLoop runs the collection ticker with backoff on repeated failures.
func (nc *NasCollector) collectLoop(ctx context.Context, d store.NasDevice) {
	nasID := int64(d.ID)
	interval := time.Duration(d.CollectInterval) * time.Second
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}

	// Collect immediately on start
	systemInfoCollected := false
	systemInfoCollected = nc.doCollect(nasID, d, systemInfoCollected)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	backoffActive := false
	var backoffTimer *time.Timer

	for {
		select {
		case <-ctx.Done():
			return
		case <-nc.stopCh:
			return
		case <-ticker.C:
			if backoffActive {
				continue
			}
			systemInfoCollected = nc.doCollect(nasID, d, systemInfoCollected)

			nc.mu.RLock()
			h := nc.health[nasID]
			nc.mu.RUnlock()
			if h != nil && h.FailureCount >= 3 && !backoffActive {
				backoffActive = true
				ticker.Stop()
				backoffTimer = time.NewTimer(5 * time.Minute)
				go func() {
					select {
					case <-ctx.Done():
						backoffTimer.Stop()
						return
					case <-nc.stopCh:
						backoffTimer.Stop()
						return
					case <-backoffTimer.C:
						backoffActive = false
						ticker.Reset(interval)
					}
				}()
			}
		}
	}
}

// doCollect performs a single collection cycle. Returns whether system info has been collected.
func (nc *NasCollector) doCollect(nasID int64, d store.NasDevice, systemInfoCollected bool) bool {
	// 1. Get credential
	cred, err := nc.credStore.Get(d.CredentialID)
	if err != nil {
		nc.recordFailure(nasID, fmt.Sprintf("get credential: %v", err))
		return systemInfoCollected
	}

	// 2. SSH connect using device's SSHUser (not credential username)
	client, err := SSHConnect(
		d.Host, d.Port, d.SSHUser,
		cred.Type,
		cred.Data["password"],
		cred.Data["private_key"],
		cred.Data["passphrase"],
	)
	if err != nil {
		nc.recordFailure(nasID, fmt.Sprintf("ssh connect: %v", err))
		if isAuthError(err) {
			log.Printf("[nas-collector] nas_id=%d auth error, check credential", nasID)
		}
		return systemInfoCollected
	}
	defer client.Close()

	// 3. Collect system info on first successful connection
	if !systemInfoCollected {
		nc.collectSystemInfoSSH(client, nasID, d)
		systemInfoCollected = true
	}

	// 4. Collect metrics
	nc.mu.RLock()
	prev := nc.prevRaw[nasID]
	nc.mu.RUnlock()

	snap, newRaw, err := collectNasMetrics(client, d.NasType, prev, d.CollectInterval)
	if err != nil {
		nc.recordFailure(nasID, fmt.Sprintf("collect metrics: %v", err))
		return systemInfoCollected
	}
	snap.NasID = nasID

	// 5. Determine status
	status := "online"
	for _, raid := range snap.Raids {
		if raid.Status == "degraded" || raid.Status == "rebuilding" {
			status = "degraded"
			break
		}
	}
	if status == "online" {
		for _, disk := range snap.Disks {
			if !disk.SmartHealthy {
				status = "degraded"
				break
			}
		}
	}

	// 6. Update DB status
	if err := nc.nasStore.UpdateStatus(d.ID, status); err != nil {
		log.Printf("[nas-collector] nas_id=%d update status: %v", nasID, err)
	}

	// 7. Update caches
	nc.mu.Lock()
	nc.cache[nasID] = snap
	nc.prevRaw[nasID] = newRaw
	h := nc.health[nasID]
	if h == nil {
		h = &NasDeviceHealth{}
		nc.health[nasID] = h
	}
	// Reset failure count on success
	h.FailureCount = 0
	h.LastError = ""
	prevStatus := h.LastStatus
	h.LastStatus = status
	nc.mu.Unlock()

	// 8. Write metrics to VictoriaMetrics
	if nc.vm != nil {
		nc.writeToVM(nasID, d.Name, snap)
	}

	// 9. Broadcast metrics via WebSocket
	nc.hub.BroadcastJSON(map[string]interface{}{
		"type": "nas_metrics",
		"data": snap,
	})

	// 10. Broadcast status change only if status changed
	if prevStatus != status {
		nc.hub.BroadcastJSON(map[string]interface{}{
			"type": "nas_status",
			"data": map[string]interface{}{
				"nas_id": nasID,
				"status": status,
			},
		})
	}

	return systemInfoCollected
}

// recordFailure increments the failure counter and broadcasts offline status if changed.
func (nc *NasCollector) recordFailure(nasID int64, errMsg string) {
	log.Printf("[nas-collector] nas_id=%d error: %s", nasID, errMsg)

	nc.mu.Lock()
	h := nc.health[nasID]
	if h == nil {
		h = &NasDeviceHealth{}
		nc.health[nasID] = h
	}
	h.FailureCount++
	h.LastError = errMsg
	prevStatus := h.LastStatus
	h.LastStatus = "offline"
	nc.mu.Unlock()

	if err := nc.nasStore.UpdateStatus(int(nasID), "offline"); err != nil {
		log.Printf("[nas-collector] nas_id=%d update offline status: %v", nasID, err)
	}

	if prevStatus != "offline" {
		nc.hub.BroadcastJSON(map[string]interface{}{
			"type": "nas_status",
			"data": map[string]interface{}{
				"nas_id": nasID,
				"status": "offline",
			},
		})
	}
}

// collectSystemInfoSSH gathers static system info based on NAS type and stores it in the database.
func (nc *NasCollector) collectSystemInfoSSH(client *ssh.Client, nasID int64, d store.NasDevice) {
	var infoJSON []byte
	var err error

	switch d.NasType {
	case "synology":
		info, e := collectSynologyInfo(client)
		if e != nil {
			log.Printf("[nas-collector] nas_id=%d collect synology info: %v", nasID, e)
			return
		}
		infoJSON, err = json.Marshal(info)
	case "fnos":
		info, e := collectFnOSInfo(client)
		if e != nil {
			log.Printf("[nas-collector] nas_id=%d collect fnos info: %v", nasID, e)
			return
		}
		infoJSON, err = json.Marshal(info)
	default:
		log.Printf("[nas-collector] nas_id=%d unknown nas_type: %s", nasID, d.NasType)
		return
	}

	if err != nil {
		log.Printf("[nas-collector] nas_id=%d marshal system info: %v", nasID, err)
		return
	}

	if err := nc.nasStore.UpdateSystemInfo(d.ID, string(infoJSON)); err != nil {
		log.Printf("[nas-collector] nas_id=%d update system info: %v", nasID, err)
	}
}

// writeToVM converts a metrics snapshot to Prometheus line format and writes to VictoriaMetrics.
func (nc *NasCollector) writeToVM(nasID int64, name string, snap *NasMetricsSnapshot) {
	var lines []string
	labels := fmt.Sprintf(`nas_id="%d",name="%s"`, nasID, escapeLabelValue(name))

	// CPU
	if snap.CPU != nil {
		lines = append(lines, fmt.Sprintf(`mantisops_nas_cpu_usage_percent{%s} %.2f`, labels, snap.CPU.UsagePercent))
	}

	// Memory
	if snap.Memory != nil {
		lines = append(lines, fmt.Sprintf(`mantisops_nas_memory_usage_percent{%s} %.2f`, labels, snap.Memory.UsagePercent))
		lines = append(lines, fmt.Sprintf(`mantisops_nas_memory_total_bytes{%s} %d`, labels, snap.Memory.Total))
		lines = append(lines, fmt.Sprintf(`mantisops_nas_memory_used_bytes{%s} %d`, labels, snap.Memory.Used))
	}

	// Networks
	for _, net := range snap.Networks {
		netLabels := fmt.Sprintf(`nas_id="%d",name="%s",interface="%s"`, nasID, escapeLabelValue(name), net.Interface)
		lines = append(lines, fmt.Sprintf(`mantisops_nas_network_rx_bytes_per_sec{%s} %d`, netLabels, net.RxBytesPerSec))
		lines = append(lines, fmt.Sprintf(`mantisops_nas_network_tx_bytes_per_sec{%s} %d`, netLabels, net.TxBytesPerSec))
	}

	// RAID
	for _, raid := range snap.Raids {
		raidLabels := fmt.Sprintf(`nas_id="%d",name="%s",array="%s",raid_type="%s"`, nasID, escapeLabelValue(name), raid.Array, raid.RaidType)
		var raidStatus int
		switch raid.Status {
		case "active":
			raidStatus = 0
		case "degraded":
			raidStatus = 1
		case "rebuilding":
			raidStatus = 2
		}
		lines = append(lines, fmt.Sprintf(`mantisops_nas_raid_status{%s} %d`, raidLabels, raidStatus))
		if raid.Status == "rebuilding" {
			lines = append(lines, fmt.Sprintf(`mantisops_nas_raid_rebuild_percent{%s} %.2f`, raidLabels, raid.RebuildPercent))
		}
	}

	// Disks
	for _, disk := range snap.Disks {
		diskLabels := fmt.Sprintf(`nas_id="%d",name="%s",disk="%s"`, nasID, escapeLabelValue(name), disk.Name)
		lines = append(lines, fmt.Sprintf(`mantisops_nas_disk_temperature_celsius{%s} %d`, diskLabels, disk.Temperature))
		lines = append(lines, fmt.Sprintf(`mantisops_nas_disk_power_on_hours{%s} %d`, diskLabels, disk.PowerOnHours))
		lines = append(lines, fmt.Sprintf(`mantisops_nas_disk_reallocated_sectors{%s} %d`, diskLabels, disk.ReallocatedSectors))
		smartVal := 1
		if !disk.SmartHealthy {
			smartVal = 0
		}
		lines = append(lines, fmt.Sprintf(`mantisops_nas_disk_smart_healthy{%s} %d`, diskLabels, smartVal))
	}

	// Volumes
	for _, vol := range snap.Volumes {
		volLabels := fmt.Sprintf(`nas_id="%d",name="%s",volume="%s"`, nasID, escapeLabelValue(name), vol.Mount)
		lines = append(lines, fmt.Sprintf(`mantisops_nas_volume_usage_percent{%s} %.2f`, volLabels, vol.UsagePercent))
		lines = append(lines, fmt.Sprintf(`mantisops_nas_volume_total_bytes{%s} %d`, volLabels, vol.Total))
		lines = append(lines, fmt.Sprintf(`mantisops_nas_volume_used_bytes{%s} %d`, volLabels, vol.Used))
	}

	// UPS
	if snap.UPS != nil {
		var upsStatus int
		switch snap.UPS.Status {
		case "online":
			upsStatus = 0
		case "on_battery":
			upsStatus = 1
		case "low_battery":
			upsStatus = 2
		default:
			upsStatus = -1
		}
		lines = append(lines, fmt.Sprintf(`mantisops_nas_ups_status{%s} %d`, labels, upsStatus))
		lines = append(lines, fmt.Sprintf(`mantisops_nas_ups_battery_percent{%s} %d`, labels, snap.UPS.BatteryPercent))
	}

	if len(lines) > 0 {
		if err := nc.vm.WriteMetrics(lines); err != nil {
			log.Printf("[nas-collector] nas_id=%d write to VM: %v", nasID, err)
		}
	}
}

// escapeLabelValue escapes double quotes and backslashes in Prometheus label values.
func escapeLabelValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

// isAuthError checks if an error is related to SSH authentication failure.
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unable to authenticate") ||
		strings.Contains(msg, "no supported methods remain") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "authentication failed") ||
		strings.Contains(msg, "parse private key")
}
