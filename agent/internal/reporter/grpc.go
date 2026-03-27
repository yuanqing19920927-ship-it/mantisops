//go:build linux

package reporter

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"mantisops/agent/internal/collector"
	"mantisops/agent/internal/config"
	"mantisops/agent/internal/hostid"
	pb "mantisops/agent/proto/gen"
)

type Reporter struct {
	cfg    *config.Config
	hostID string
	client pb.AgentServiceClient
	conn   *grpc.ClientConn
	cpuCol *collector.CPUCollector
	netCol *collector.NetworkCollector
}

func New(cfg *config.Config) *Reporter {
	return &Reporter{
		cfg:    cfg,
		hostID: hostid.Get(cfg.Agent.ID),
		cpuCol: collector.NewCPUCollector(),
		netCol: collector.NewNetworkCollector(),
	}
}

func (r *Reporter) Connect() error {
	conn, err := grpc.NewClient(r.cfg.Server.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	r.conn = conn
	r.client = pb.NewAgentServiceClient(conn)
	return nil
}

func (r *Reporter) authCtx() context.Context {
	md := metadata.Pairs("authorization", "Bearer "+r.cfg.Server.Token)
	return metadata.NewOutgoingContext(context.Background(), md)
}

func (r *Reporter) Register() error {
	host, err := collector.CollectHostInfo()
	if err != nil {
		return err
	}
	req := &pb.RegisterRequest{
		HostId:       r.hostID,
		Hostname:     host.Hostname,
		Os:           host.OS,
		Kernel:       host.Kernel,
		Arch:         host.Arch,
		BootTime:     host.BootTime,
		AgentVersion: "0.1.0",
		IpAddresses:  host.IPAddresses,
		CpuCores:     int32(host.CPUCores),
		CpuModel:     host.CPUModel,
		MemoryTotal:  host.MemoryTotal,
		DiskTotal:    host.DiskTotal,
	}

	if r.cfg.Collect.GPU {
		if gpu, err := collector.CollectGPU(); err == nil {
			req.GpuModel = gpu.Name
			req.GpuMemory = gpu.MemoryTotal
		}
	}

	_, err = r.client.Register(r.authCtx(), req)
	return err
}

func (r *Reporter) RunLoop(ctx context.Context) {
	interval := time.Duration(r.cfg.Collect.Interval) * time.Second
	metricsTicker := time.NewTicker(interval)
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer metricsTicker.Stop()
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-metricsTicker.C:
			r.reportMetrics()
		case <-heartbeatTicker.C:
			r.heartbeat()
		}
	}
}

func (r *Reporter) reportMetrics() {
	payload := &pb.MetricsPayload{
		HostId:    r.hostID,
		Timestamp: time.Now().Unix(),
	}
	if cpu, err := r.cpuCol.Collect(); err == nil {
		payload.Cpu = &pb.CpuMetrics{
			UsagePercent: cpu.UsagePercent,
			Load1:        cpu.Load1,
			Load5:        cpu.Load5,
			Load15:       cpu.Load15,
			Cores:        int32(cpu.Cores),
		}
	}
	if mem, err := collector.CollectMemory(); err == nil {
		payload.Memory = &pb.MemoryMetrics{
			Total:        mem.Total,
			Used:         mem.Used,
			Available:    mem.Available,
			UsagePercent: mem.UsagePercent,
			SwapTotal:    mem.SwapTotal,
			SwapUsed:     mem.SwapUsed,
		}
	}
	if disks, err := collector.CollectDisks(); err == nil {
		for _, d := range disks {
			payload.Disks = append(payload.Disks, &pb.DiskMetrics{
				MountPoint:   d.MountPoint,
				Device:       d.Device,
				FsType:       d.FSType,
				Total:        d.Total,
				Used:         d.Used,
				UsagePercent: d.UsagePercent,
			})
		}
	}
	if nets, err := r.netCol.Collect(); err == nil {
		for _, n := range nets {
			payload.Networks = append(payload.Networks, &pb.NetworkMetrics{
				Interface:     n.Interface,
				RxBytesPerSec: n.RxBytesPS,
				TxBytesPerSec: n.TxBytesPS,
				RxBytesTotal:  n.RxBytesTotal,
				TxBytesTotal:  n.TxBytesTotal,
			})
		}
	}
	// Docker 采集
	if r.cfg.Collect.Docker {
		if containers, err := collector.CollectDocker(); err == nil {
			for _, c := range containers {
				payload.Containers = append(payload.Containers, &pb.DockerMetrics{
					ContainerId: c.ContainerID,
					Name:        c.Name,
					Image:       c.Image,
					State:       c.State,
					Status:      c.Status,
					CpuPercent:  c.CPUPercent,
					MemoryUsage: c.MemoryUsage,
					MemoryLimit: c.MemoryLimit,
					Ports:       c.Ports,
				})
			}
		} else {
			log.Printf("docker collect error: %v", err)
		}
	}

	// GPU 采集
	if r.cfg.Collect.GPU {
		if gpu, err := collector.CollectGPU(); err == nil {
			payload.Gpu = &pb.GpuMetrics{
				Name:         gpu.Name,
				UsagePercent: gpu.UsagePercent,
				MemoryUsed:   gpu.MemoryUsed,
				MemoryTotal:  gpu.MemoryTotal,
				Temperature:  gpu.Temperature,
				PowerUsage:   gpu.PowerUsage,
			}
		}
	}

	if _, err := r.client.ReportMetrics(r.authCtx(), payload); err != nil {
		log.Printf("report error: %v", err)
	}
}

func (r *Reporter) heartbeat() {
	if _, err := r.client.Heartbeat(r.authCtx(), &pb.HeartbeatRequest{
		HostId:       r.hostID,
		AgentVersion: "0.1.0",
	}); err != nil {
		log.Printf("heartbeat error: %v", err)
	}
}

func (r *Reporter) Close() {
	if r.conn != nil {
		r.conn.Close()
	}
}
