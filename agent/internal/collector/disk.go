//go:build linux

package collector

import (
	"os"
	"strings"
	"syscall"
)

type DiskStats struct {
	MountPoint   string
	Device       string
	FSType       string
	Total        uint64
	Used         uint64
	UsagePercent float64
}

func CollectDisks() ([]DiskStats, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil, err
	}
	var disks []DiskStats
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		device, mount, fstype := fields[0], fields[1], fields[2]
		if !strings.HasPrefix(device, "/dev/") {
			continue
		}
		if seen[device] {
			continue
		}
		seen[device] = true
		var stat syscall.Statfs_t
		if err := syscall.Statfs(mount, &stat); err != nil {
			continue
		}
		total := stat.Blocks * uint64(stat.Bsize)
		free := stat.Bfree * uint64(stat.Bsize)
		used := total - free
		var pct float64
		if total > 0 {
			pct = float64(used) / float64(total) * 100
		}
		disks = append(disks, DiskStats{
			MountPoint:   mount,
			Device:       device,
			FSType:       fstype,
			Total:        total,
			Used:         used,
			UsagePercent: pct,
		})
	}
	return disks, nil
}
