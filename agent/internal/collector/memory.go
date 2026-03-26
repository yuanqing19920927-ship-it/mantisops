//go:build linux

package collector

import (
	"os"
	"strconv"
	"strings"
)

type MemoryStats struct {
	Total        uint64
	Used         uint64
	Available    uint64
	UsagePercent float64
	SwapTotal    uint64
	SwapUsed     uint64
}

func CollectMemory() (*MemoryStats, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil, err
	}
	info := parseMemInfo(string(data))
	total := info["MemTotal"]
	available := info["MemAvailable"]
	used := total - available
	swapTotal := info["SwapTotal"]
	swapFree := info["SwapFree"]
	var pct float64
	if total > 0 {
		pct = float64(used) / float64(total) * 100
	}
	return &MemoryStats{
		Total:        total,
		Used:         used,
		Available:    available,
		UsagePercent: pct,
		SwapTotal:    swapTotal,
		SwapUsed:     swapTotal - swapFree,
	}, nil
}

func parseMemInfo(data string) map[string]uint64 {
	result := make(map[string]uint64)
	for _, line := range strings.Split(data, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, " kB")
		val, _ := strconv.ParseUint(strings.TrimSpace(valStr), 10, 64)
		result[key] = val * 1024
	}
	return result
}
