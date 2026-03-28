package collector

import (
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type FnOSSystemInfo struct {
	OSVersion string `json:"os_version"`
	Kernel    string `json:"kernel"`
	Arch      string `json:"arch"`
}

// collectFnOSInfo 采集飞牛静态系统信息
func collectFnOSInfo(client *ssh.Client) (*FnOSSystemInfo, error) {
	timeout := 30 * time.Second
	info := &FnOSSystemInfo{}

	out, err := SSHExec(client, "cat /etc/os-release 2>/dev/null", timeout)
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.Trim(strings.TrimSpace(parts[1]), "\"")
			switch key {
			case "PRETTY_NAME":
				info.OSVersion = val
			}
		}
	}

	out, _ = SSHExec(client, "uname -r && uname -m", timeout)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) >= 1 {
		info.Kernel = strings.TrimSpace(lines[0])
	}
	if len(lines) >= 2 {
		info.Arch = strings.TrimSpace(lines[1])
	}

	return info, nil
}

// collectBtrfsHealth 检查 Btrfs 文件系统健康状态
func collectBtrfsHealth(client *ssh.Client) (hasErrors bool, details string) {
	timeout := 30 * time.Second
	out, err := SSHExec(client, "sudo btrfs device stats / 2>/dev/null", timeout)
	if err != nil {
		return false, ""
	}
	// btrfs device stats 输出每行格式: [/dev/sda1].write_io_errs    0
	// 任何非 0 值表示有错误
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			val := strings.TrimSpace(fields[len(fields)-1])
			if val != "0" && val != "" {
				return true, out
			}
		}
	}
	return false, out
}
