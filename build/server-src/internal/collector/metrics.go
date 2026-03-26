package collector

import (
	"fmt"
	"log"

	pb "opsboard/server/proto/gen"
	"opsboard/server/internal/store"
	"opsboard/server/internal/ws"
)

type MetricsCollector struct {
	vm          *store.VictoriaStore
	hub         *ws.Hub
	serverStore *store.ServerStore
}

func NewMetricsCollector(vm *store.VictoriaStore, hub *ws.Hub, ss *store.ServerStore) *MetricsCollector {
	return &MetricsCollector{vm: vm, hub: hub, serverStore: ss}
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
			fmt.Sprintf(`opsboard_cpu_usage_percent{host_id="%s",host="%s"} %f`, hostID, host, cpu.UsagePercent),
			fmt.Sprintf(`opsboard_cpu_load1{host_id="%s",host="%s"} %f`, hostID, host, cpu.Load1),
			fmt.Sprintf(`opsboard_cpu_load5{host_id="%s",host="%s"} %f`, hostID, host, cpu.Load5),
		)
	}
	if mem := payload.Memory; mem != nil {
		lines = append(lines,
			fmt.Sprintf(`opsboard_memory_used_bytes{host_id="%s",host="%s"} %d`, hostID, host, mem.Used),
			fmt.Sprintf(`opsboard_memory_total_bytes{host_id="%s",host="%s"} %d`, hostID, host, mem.Total),
			fmt.Sprintf(`opsboard_memory_usage_percent{host_id="%s",host="%s"} %f`, hostID, host, mem.UsagePercent),
		)
	}
	for _, d := range payload.Disks {
		lines = append(lines,
			fmt.Sprintf(`opsboard_disk_usage_percent{host_id="%s",host="%s",mount="%s"} %f`, hostID, host, d.MountPoint, d.UsagePercent),
			fmt.Sprintf(`opsboard_disk_used_bytes{host_id="%s",host="%s",mount="%s"} %d`, hostID, host, d.MountPoint, d.Used),
			fmt.Sprintf(`opsboard_disk_total_bytes{host_id="%s",host="%s",mount="%s"} %d`, hostID, host, d.MountPoint, d.Total),
		)
	}
	for _, n := range payload.Networks {
		lines = append(lines,
			fmt.Sprintf(`opsboard_network_rx_bytes_per_sec{host_id="%s",host="%s",iface="%s"} %f`, hostID, host, n.Interface, n.RxBytesPerSec),
			fmt.Sprintf(`opsboard_network_tx_bytes_per_sec{host_id="%s",host="%s",iface="%s"} %f`, hostID, host, n.Interface, n.TxBytesPerSec),
		)
	}
	for _, c := range payload.Containers {
		state := 0
		if c.State == "running" {
			state = 1
		}
		lines = append(lines,
			fmt.Sprintf(`opsboard_container_state{host_id="%s",host="%s",container="%s"} %d`, hostID, host, c.Name, state),
			fmt.Sprintf(`opsboard_container_cpu_percent{host_id="%s",host="%s",container="%s"} %f`, hostID, host, c.Name, c.CpuPercent),
			fmt.Sprintf(`opsboard_container_memory_bytes{host_id="%s",host="%s",container="%s"} %d`, hostID, host, c.Name, c.MemoryUsage),
		)
	}
	if gpu := payload.Gpu; gpu != nil {
		lines = append(lines,
			fmt.Sprintf(`opsboard_gpu_usage_percent{host_id="%s",host="%s",gpu="%s"} %f`, hostID, host, gpu.Name, gpu.UsagePercent),
			fmt.Sprintf(`opsboard_gpu_memory_used_bytes{host_id="%s",host="%s",gpu="%s"} %d`, hostID, host, gpu.Name, gpu.MemoryUsed),
			fmt.Sprintf(`opsboard_gpu_temperature{host_id="%s",host="%s",gpu="%s"} %f`, hostID, host, gpu.Name, gpu.Temperature),
		)
	}

	if len(lines) > 0 {
		if err := m.vm.WriteMetrics(lines); err != nil {
			log.Printf("VM write error: %v", err)
		}
	}

	m.hub.BroadcastJSON(map[string]interface{}{
		"type":    "metrics",
		"host_id": hostID,
		"data":    payload,
	})
}
