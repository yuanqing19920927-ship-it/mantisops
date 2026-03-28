package grpc

import (
	"context"
	"log"
	"time"

	"mantisops/server/internal/model"
	"mantisops/server/internal/store"
	pb "mantisops/server/proto/gen"
)

type Handler struct {
	pb.UnimplementedAgentServiceServer
	serverStore     *store.ServerStore
	onMetrics       func(hostID string, payload *pb.MetricsPayload)
	onRegister      func(hostID string)
	discoveredStore *store.DiscoveredServiceStore
	assetStore      *store.AssetStore
}

func NewHandler(ss *store.ServerStore, onMetrics func(string, *pb.MetricsPayload), onRegister func(string), ds *store.DiscoveredServiceStore, as *store.AssetStore) *Handler {
	return &Handler{serverStore: ss, onMetrics: onMetrics, onRegister: onRegister, discoveredStore: ds, assetStore: as}
}

func (h *Handler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	srv := &model.Server{
		HostID:       req.HostId,
		Hostname:     req.Hostname,
		IPAddresses:  store.IPListToJSON(req.IpAddresses),
		OS:           req.Os,
		Kernel:       req.Kernel,
		Arch:         req.Arch,
		AgentVersion: req.AgentVersion,
		BootTime:     req.BootTime,
		CPUCores:     int(req.CpuCores),
		CPUModel:     req.CpuModel,
		MemoryTotal:  int64(req.MemoryTotal),
		DiskTotal:    int64(req.DiskTotal),
		GPUModel:     req.GpuModel,
		GPUMemory:    int64(req.GpuMemory),
	}
	if err := h.serverStore.Upsert(srv); err != nil {
		log.Printf("register error: %v", err)
	}
	log.Printf("agent registered: %s (%s)", req.Hostname, req.HostId)

	// Notify deployer if waiting for this agent
	if h.onRegister != nil {
		h.onRegister(req.HostId)
	}

	return &pb.RegisterResponse{Accepted: true, ReportInterval: 5}, nil
}

func (h *Handler) ReportMetrics(ctx context.Context, req *pb.MetricsPayload) (*pb.ReportResponse, error) {
	if h.onMetrics != nil {
		h.onMetrics(req.HostId, req)
	}
	return &pb.ReportResponse{Ok: true}, nil
}

func (h *Handler) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	h.serverStore.UpdateLastSeen(req.HostId)
	return &pb.HeartbeatResponse{ServerTime: time.Now().Unix(), ReportInterval: 0}, nil
}

func (h *Handler) ReportServices(ctx context.Context, req *pb.ReportServicesRequest) (*pb.ReportServicesResponse, error) {
	if h.discoveredStore == nil {
		return &pb.ReportServicesResponse{Ok: false}, nil
	}

	var services []store.DiscoveredService
	for _, s := range req.Services {
		services = append(services, store.DiscoveredService{
			PID:      int(s.Pid),
			Name:     s.Name,
			CmdLine:  s.CmdLine,
			Port:     int(s.Port),
			Protocol: s.Protocol,
			BindAddr: s.BindAddr,
		})
	}

	if err := h.discoveredStore.SyncServices(req.HostId, services, h.assetStore, h.serverStore); err != nil {
		log.Printf("[grpc] sync services for %s: %v", req.HostId, err)
		return &pb.ReportServicesResponse{Ok: false}, nil
	}

	return &pb.ReportServicesResponse{Ok: true}, nil
}
