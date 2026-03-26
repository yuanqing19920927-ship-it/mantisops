//go:build linux

package collector

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type NetworkStats struct {
	Interface    string
	RxBytesPS    float64
	TxBytesPS    float64
	RxBytesTotal uint64
	TxBytesTotal uint64
}

type NetworkCollector struct {
	prevStats map[string][2]uint64
	prevTime  time.Time
}

func NewNetworkCollector() *NetworkCollector {
	return &NetworkCollector{
		prevStats: make(map[string][2]uint64),
	}
}

func (nc *NetworkCollector) Collect() ([]NetworkStats, error) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil, err
	}
	now := time.Now()
	current := make(map[string][2]uint64)
	var result []NetworkStats

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, ":") || strings.HasPrefix(line, "Inter") || strings.HasPrefix(line, "face") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		iface := strings.TrimSpace(parts[0])
		if iface == "lo" {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 10 {
			continue
		}
		rxBytes, _ := strconv.ParseUint(fields[0], 10, 64)
		txBytes, _ := strconv.ParseUint(fields[8], 10, 64)
		current[iface] = [2]uint64{rxBytes, txBytes}

		stat := NetworkStats{
			Interface:    iface,
			RxBytesTotal: rxBytes,
			TxBytesTotal: txBytes,
		}
		if prev, ok := nc.prevStats[iface]; ok && !nc.prevTime.IsZero() {
			elapsed := now.Sub(nc.prevTime).Seconds()
			if elapsed > 0 {
				stat.RxBytesPS = float64(rxBytes-prev[0]) / elapsed
				stat.TxBytesPS = float64(txBytes-prev[1]) / elapsed
			}
		}
		result = append(result, stat)
	}
	nc.prevStats = current
	nc.prevTime = now
	return result, nil
}
