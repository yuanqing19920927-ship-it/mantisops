//go:build linux

package collector

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type GPUStats struct {
	Name         string
	UsagePercent float64
	MemoryUsed   uint64
	MemoryTotal  uint64
	Temperature  float64
	PowerUsage   float64
}

func CollectGPU() (*GPUStats, error) {
	out, err := exec.Command("nvidia-smi",
		"--query-gpu=name,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return nil, err
	}

	line := strings.TrimSpace(string(out))
	parts := strings.Split(line, ", ")
	if len(parts) < 6 {
		return nil, fmt.Errorf("unexpected nvidia-smi output: %s", line)
	}

	usage, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	memUsed, _ := strconv.ParseUint(strings.TrimSpace(parts[2]), 10, 64)
	memTotal, _ := strconv.ParseUint(strings.TrimSpace(parts[3]), 10, 64)
	temp, _ := strconv.ParseFloat(strings.TrimSpace(parts[4]), 64)
	power, _ := strconv.ParseFloat(strings.TrimSpace(parts[5]), 64)

	return &GPUStats{
		Name:         strings.TrimSpace(parts[0]),
		UsagePercent: usage,
		MemoryUsed:   memUsed * 1024 * 1024, // MiB → bytes
		MemoryTotal:  memTotal * 1024 * 1024,
		Temperature:  temp,
		PowerUsage:   power,
	}, nil
}
