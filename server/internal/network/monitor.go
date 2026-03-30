package network

import (
	"context"
	"log"
	"sync"
	"time"

	probing "github.com/prometheus-community/pro-bing"

	"mantisops/server/internal/config"
	"mantisops/server/internal/store"
	"mantisops/server/internal/ws"
)

const (
	// consecutiveFailsForOffline is the number of successive ping failures
	// required before a device is marked offline.
	consecutiveFailsForOffline = 3

	// maxConcurrentPings caps the number of goroutines pinging simultaneously.
	maxConcurrentPings = 10

	// pingLaunchIntervalMs is the minimum delay (ms) between launching pings
	// to avoid flooding the network interface.
	pingLaunchIntervalMs = 20
)

// ConnectivityMonitor periodically pings all known network devices and updates
// their online/offline status in the database.
type ConnectivityMonitor struct {
	cfg        config.NetworkConfig
	store      *store.NetworkStore
	hub        *ws.Hub
	failCounts map[string]int // ip -> consecutive failure count
	mu         sync.Mutex
	cancel     context.CancelFunc
}

// NewConnectivityMonitor constructs a monitor.  Call Start() to begin polling.
func NewConnectivityMonitor(cfg config.NetworkConfig, ns *store.NetworkStore, hub *ws.Hub) *ConnectivityMonitor {
	return &ConnectivityMonitor{
		cfg:        cfg,
		store:      ns,
		hub:        hub,
		failCounts: make(map[string]int),
	}
}

// Start spawns the background polling goroutine.
func (m *ConnectivityMonitor) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go m.run(ctx)
}

// Stop cancels the background goroutine.
func (m *ConnectivityMonitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// run is the ticker loop; it calls checkAll on every tick.
func (m *ConnectivityMonitor) run(ctx context.Context) {
	interval := m.cfg.MonitorInterval
	if interval <= 0 {
		interval = 60 // default: 60 seconds
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
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

// checkAll fetches all devices and pings them concurrently.
func (m *ConnectivityMonitor) checkAll(ctx context.Context) {
	devices, err := m.store.GetAllDevices()
	if err != nil {
		log.Printf("[network/monitor] GetAllDevices error: %v", err)
		return
	}
	if len(devices) == 0 {
		return
	}

	timeoutMs := m.cfg.Scan.ICMPTimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 1000
	}

	sem := make(chan struct{}, maxConcurrentPings)
	var wg sync.WaitGroup

	for i, dev := range devices {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if i > 0 {
			time.Sleep(time.Duration(pingLaunchIntervalMs) * time.Millisecond)
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(d store.NetworkDevice) {
			defer wg.Done()
			defer func() { <-sem }()

			alive := pingOnce(d.IP, timeoutMs)
			m.handleResult(d, alive)
		}(dev)
	}

	wg.Wait()
}

// handleResult updates failure counters and triggers status changes when
// the device crosses the online/offline threshold.
func (m *ConnectivityMonitor) handleResult(dev store.NetworkDevice, alive bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if alive {
		// Clear failure count.
		m.failCounts[dev.IP] = 0

		// Recovery: device was offline and is now reachable.
		if dev.Status == "offline" {
			if err := m.store.UpdateDeviceStatus(dev.ID, "online"); err != nil {
				log.Printf("[network/monitor] UpdateDeviceStatus(%d, online) error: %v", dev.ID, err)
				return
			}
			log.Printf("[network/monitor] device %s (%s) recovered (online)", dev.IP, dev.Hostname)
			m.broadcastStatusChange(dev, "online")
		}
	} else {
		// Increment consecutive failure count.
		m.failCounts[dev.IP]++

		// Offline threshold: only transition from online to offline.
		if dev.Status == "online" && m.failCounts[dev.IP] >= consecutiveFailsForOffline {
			if err := m.store.UpdateDeviceStatus(dev.ID, "offline"); err != nil {
				log.Printf("[network/monitor] UpdateDeviceStatus(%d, offline) error: %v", dev.ID, err)
				return
			}
			log.Printf("[network/monitor] device %s (%s) unreachable (offline) after %d failures",
				dev.IP, dev.Hostname, m.failCounts[dev.IP])
			m.broadcastStatusChange(dev, "offline")
		}
	}
}

// broadcastStatusChange sends a WebSocket message to admin clients.
// Must be called with m.mu held (or after releasing it — here called while held
// since hub.BroadcastAdmin acquires its own lock and does not call back into m).
func (m *ConnectivityMonitor) broadcastStatusChange(dev store.NetworkDevice, status string) {
	if m.hub == nil {
		return
	}
	m.hub.BroadcastAdmin(map[string]interface{}{
		"type":      "network_device_status",
		"device_id": dev.ID,
		"ip":        dev.IP,
		"hostname":  dev.Hostname,
		"status":    status,
	})
}

// pingOnce sends a single ICMP echo to ip with the given timeout in milliseconds.
// Uses privileged raw sockets (requires root / CAP_NET_RAW).
func pingOnce(ip string, timeoutMs int) bool {
	pinger, err := probing.NewPinger(ip)
	if err != nil {
		return false
	}
	pinger.SetPrivileged(true)
	pinger.Count = 1
	pinger.Timeout = time.Duration(timeoutMs) * time.Millisecond

	if err := pinger.Run(); err != nil {
		return false
	}
	return pinger.Statistics().PacketsRecv > 0
}
