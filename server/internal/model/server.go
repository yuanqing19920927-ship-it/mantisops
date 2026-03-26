package model

import "time"

type Server struct {
	ID           int       `json:"id"`
	HostID       string    `json:"host_id"`
	Hostname     string    `json:"hostname"`
	IPAddresses  string    `json:"ip_addresses"`
	OS           string    `json:"os"`
	Kernel       string    `json:"kernel"`
	Arch         string    `json:"arch"`
	AgentVersion string    `json:"agent_version"`
	CPUCores     int       `json:"cpu_cores"`
	CPUModel     string    `json:"cpu_model"`
	MemoryTotal  int64     `json:"memory_total"`
	DiskTotal    int64     `json:"disk_total"`
	GPUModel     string    `json:"gpu_model"`
	GPUMemory    int64     `json:"gpu_memory"`
	BootTime     int64     `json:"boot_time"`
	LastSeen     int64     `json:"last_seen"`
	Status       string    `json:"status"`
	DisplayName  string    `json:"display_name"`
	SortOrder    int       `json:"sort_order"`
	GroupID      *int      `json:"group_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
