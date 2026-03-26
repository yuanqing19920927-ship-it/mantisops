//go:build linux

package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type DockerStats struct {
	ContainerID string
	Name        string
	Image       string
	State       string
	Status      string
	CPUPercent  float64
	MemoryUsage uint64
	MemoryLimit uint64
	Ports       []string
}

type dockerContainer struct {
	ID    string       `json:"Id"`
	Names []string     `json:"Names"`
	Image string       `json:"Image"`
	State string       `json:"State"`
	Status string      `json:"Status"`
	Ports []dockerPort `json:"Ports"`
}

type dockerPort struct {
	IP          string `json:"IP"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

type dockerStatsResp struct {
	CPUStats    cpuStatsJSON    `json:"cpu_stats"`
	PreCPUStats cpuStatsJSON    `json:"precpu_stats"`
	MemoryStats memoryStatsJSON `json:"memory_stats"`
}

type cpuStatsJSON struct {
	CPUUsage struct {
		TotalUsage uint64 `json:"total_usage"`
	} `json:"cpu_usage"`
	SystemCPUUsage uint64 `json:"system_cpu_usage"`
	OnlineCPUs     int    `json:"online_cpus"`
}

type memoryStatsJSON struct {
	Usage uint64 `json:"usage"`
	Limit uint64 `json:"limit"`
}

var dockerClient = &http.Client{
	Transport: &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", "/var/run/docker.sock")
		},
	},
	Timeout: 10 * time.Second,
}

func CollectDocker() ([]DockerStats, error) {
	resp, err := dockerClient.Get("http://localhost/containers/json?all=true")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var containers []dockerContainer
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, err
	}

	var stats []DockerStats
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}

		var ports []string
		for _, p := range c.Ports {
			if p.PublicPort > 0 {
				ports = append(ports, fmt.Sprintf("%s:%d->%d/%s", p.IP, p.PublicPort, p.PrivatePort, p.Type))
			}
		}

		ds := DockerStats{
			ContainerID: c.ID[:12],
			Name:        name,
			Image:       c.Image,
			State:       c.State,
			Status:      c.Status,
			Ports:       ports,
		}

		// 只对 running 容器获取资源使用
		if c.State == "running" {
			if cpu, mem, err := getContainerStats(c.ID); err == nil {
				ds.CPUPercent = cpu
				ds.MemoryUsage = mem.usage
				ds.MemoryLimit = mem.limit
			}
		}

		stats = append(stats, ds)
	}
	return stats, nil
}

type memInfo struct {
	usage, limit uint64
}

func getContainerStats(id string) (float64, memInfo, error) {
	resp, err := dockerClient.Get(fmt.Sprintf("http://localhost/containers/%s/stats?stream=false", id))
	if err != nil {
		return 0, memInfo{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var s dockerStatsResp
	if err := json.Unmarshal(body, &s); err != nil {
		return 0, memInfo{}, err
	}

	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage - s.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s.CPUStats.SystemCPUUsage - s.PreCPUStats.SystemCPUUsage)
	cpuPercent := 0.0
	if sysDelta > 0 && s.CPUStats.OnlineCPUs > 0 {
		cpuPercent = (cpuDelta / sysDelta) * float64(s.CPUStats.OnlineCPUs) * 100.0
	}

	return cpuPercent, memInfo{s.MemoryStats.Usage, s.MemoryStats.Limit}, nil
}
