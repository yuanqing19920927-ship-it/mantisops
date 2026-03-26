//go:build linux

package collector

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type CPUStats struct {
	UsagePercent float64
	Load1        float64
	Load5        float64
	Load15       float64
	Cores        int
}

type CPUCollector struct {
	prevIdle  uint64
	prevTotal uint64
}

func NewCPUCollector() *CPUCollector {
	return &CPUCollector{}
}

func (c *CPUCollector) Collect() (*CPUStats, error) {
	usage, err := c.cpuUsage()
	if err != nil {
		return nil, err
	}
	load1, load5, load15, _ := loadAvg()
	return &CPUStats{
		UsagePercent: usage,
		Load1:        load1,
		Load5:        load5,
		Load15:       load15,
		Cores:        runtime.NumCPU(),
	}, nil
}

func (c *CPUCollector) cpuUsage() (float64, error) {
	idle, total, err := readCPUStat()
	if err != nil {
		return 0, err
	}
	if c.prevTotal == 0 {
		c.prevIdle, c.prevTotal = idle, total
		time.Sleep(100 * time.Millisecond)
		idle, total, err = readCPUStat()
		if err != nil {
			return 0, err
		}
	}
	deltaTotal := float64(total - c.prevTotal)
	deltaIdle := float64(idle - c.prevIdle)
	c.prevIdle, c.prevTotal = idle, total
	if deltaTotal == 0 {
		return 0, nil
	}
	return (1 - deltaIdle/deltaTotal) * 100, nil
}

func readCPUStat() (idle, total uint64, err error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	line := strings.Split(string(data), "\n")[0]
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return 0, 0, nil
	}
	for i := 1; i < len(fields); i++ {
		val, _ := strconv.ParseUint(fields[i], 10, 64)
		total += val
		if i == 4 {
			idle = val
		}
	}
	return idle, total, nil
}

func loadAvg() (float64, float64, float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0, nil
	}
	l1, _ := strconv.ParseFloat(fields[0], 64)
	l5, _ := strconv.ParseFloat(fields[1], 64)
	l15, _ := strconv.ParseFloat(fields[2], 64)
	return l1, l5, l15, nil
}
