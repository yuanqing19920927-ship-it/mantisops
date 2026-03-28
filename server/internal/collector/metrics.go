package collector

import (
	"fmt"
	"log"
	"sync"
	"time"

	pb "mantisops/server/proto/gen"
	"mantisops/server/internal/store"
	"mantisops/server/internal/ws"
)

type cachedMetrics struct {
	payload   *pb.MetricsPayload
	updatedAt time.Time
}

type MetricsCollector struct {
	vm          *store.VictoriaStore
	hub         *ws.Hub
	serverStore *store.ServerStore
	mu          sync.RWMutex
	cache       map[string]*cachedMetrics
}

func NewMetricsCollector(vm *store.VictoriaStore, hub *ws.Hub, ss *store.ServerStore) *MetricsCollector {
	return &MetricsCollector{vm: vm, hub: hub, serverStore: ss, cache: make(map[string]*cachedMetrics)}
}

func (m *MetricsCollector) Handle(hostID string, payload *pb.MetricsPayload) {
	host := hostID
	if srv, err := m.serverStore.GetByHostID(hostID); err == nil {
		host = srv.Hostname
	} else if len(host) > 16 {
		host = host[:16]
	}

	var lines []string

	if cpu := payload.Cpu; cpu != nil {
		lines = append(lines,
			fmt.Sprintf(`mantisops_cpu_usage_percent{host_id="%s",host="%s"} %f`, hostID, host, cpu.UsagePercent),
			fmt.Sprintf(`mantisops_cpu_load1{host_id="%s",host="%s"} %f`, hostID, host, cpu.Load1),
			fmt.Sprintf(`mantisops_cpu_load5{host_id="%s",host="%s"} %f`, hostID, host, cpu.Load5),
		)
	}
	if mem := payload.Memory; mem != nil {
		lines = append(lines,
			fmt.Sprintf(`mantisops_memory_used_bytes{host_id="%s",host="%s"} %d`, hostID, host, mem.Used),
			fmt.Sprintf(`mantisops_memory_total_bytes{host_id="%s",host="%s"} %d`, hostID, host, mem.Total),
			fmt.Sprintf(`mantisops_memory_usage_percent{host_id="%s",host="%s"} %f`, hostID, host, mem.UsagePercent),
		)
	}
	for _, d := range payload.Disks {
		lines = append(lines,
			fmt.Sprintf(`mantisops_disk_usage_percent{host_id="%s",host="%s",mount="%s"} %f`, hostID, host, d.MountPoint, d.UsagePercent),
			fmt.Sprintf(`mantisops_disk_used_bytes{host_id="%s",host="%s",mount="%s"} %d`, hostID, host, d.MountPoint, d.Used),
			fmt.Sprintf(`mantisops_disk_total_bytes{host_id="%s",host="%s",mount="%s"} %d`, hostID, host, d.MountPoint, d.Total),
		)
	}
	for _, n := range payload.Networks {
		lines = append(lines,
			fmt.Sprintf(`mantisops_network_rx_bytes_per_sec{host_id="%s",host="%s",iface="%s"} %f`, hostID, host, n.Interface, n.RxBytesPerSec),
			fmt.Sprintf(`mantisops_network_tx_bytes_per_sec{host_id="%s",host="%s",iface="%s"} %f`, hostID, host, n.Interface, n.TxBytesPerSec),
		)
	}
	for _, c := range payload.Containers {
		state := 0
		if c.State == "running" {
			state = 1
		}
		lines = append(lines,
			fmt.Sprintf(`mantisops_container_state{host_id="%s",host="%s",container="%s"} %d`, hostID, host, c.Name, state),
			fmt.Sprintf(`mantisops_container_cpu_percent{host_id="%s",host="%s",container="%s"} %f`, hostID, host, c.Name, c.CpuPercent),
			fmt.Sprintf(`mantisops_container_memory_bytes{host_id="%s",host="%s",container="%s"} %d`, hostID, host, c.Name, c.MemoryUsage),
		)
	}
	if gpu := payload.Gpu; gpu != nil {
		lines = append(lines,
			fmt.Sprintf(`mantisops_gpu_usage_percent{host_id="%s",host="%s",gpu="%s"} %f`, hostID, host, gpu.Name, gpu.UsagePercent),
			fmt.Sprintf(`mantisops_gpu_memory_used_bytes{host_id="%s",host="%s",gpu="%s"} %d`, hostID, host, gpu.Name, gpu.MemoryUsed),
			fmt.Sprintf(`mantisops_gpu_temperature{host_id="%s",host="%s",gpu="%s"} %f`, hostID, host, gpu.Name, gpu.Temperature),
		)
	}

	if len(lines) > 0 {
		if err := m.vm.WriteMetrics(lines); err != nil {
			log.Printf("VM write error: %v", err)
		}
	}

	m.hub.BroadcastMetrics(hostID, map[string]interface{}{
		"type":    "metrics",
		"host_id": hostID,
		"data":    payload,
	})

	m.mu.Lock()
	m.cache[hostID] = &cachedMetrics{payload: payload, updatedAt: time.Now()}
	m.mu.Unlock()
}

const FreshnessTTL = 120 * time.Second

func (m *MetricsCollector) GetLatestMetrics(hostID string) *pb.MetricsPayload {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cm, ok := m.cache[hostID]
	if !ok || time.Since(cm.updatedAt) > FreshnessTTL {
		return nil
	}
	return cm.payload
}

func (m *MetricsCollector) GetAllCachedHosts() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	hosts := make([]string, 0, len(m.cache))
	for k := range m.cache {
		hosts = append(hosts, k)
	}
	return hosts
}
