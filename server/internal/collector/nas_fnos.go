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
// fnOS 基于 Debian，/etc/os-release 显示 Debian 信息，需要多路径探测 fnOS 版本
func collectFnOSInfo(client *ssh.Client) (*FnOSSystemInfo, error) {
	timeout := 30 * time.Second
	info := &FnOSSystemInfo{}

	// 尝试多种方式获取 fnOS 版本
	// 1. fnOS 专有版本文件
	if out, err := SSHExec(client, "cat /etc/fnos-release 2>/dev/null", timeout); err == nil && strings.TrimSpace(out) != "" {
		info.OSVersion = "fnOS " + strings.TrimSpace(out)
	}
	// 2. dpkg 查 fnos 包版本
	if info.OSVersion == "" {
		if out, err := SSHExec(client, "dpkg -l 2>/dev/null | grep fnos | head -1 | awk '{print $3}'", timeout); err == nil && strings.TrimSpace(out) != "" {
			info.OSVersion = "fnOS " + strings.TrimSpace(out)
		}
	}
	// 3. /etc/issue 包含 fnOS 版本（格式: "OS version:         fnOS v1.1.8"）
	if info.OSVersion == "" {
		if out, err := SSHExec(client, "grep 'OS version' /etc/issue 2>/dev/null", timeout); err == nil {
			line := strings.TrimSpace(out)
			if idx := strings.Index(line, "OS version:"); idx >= 0 {
				ver := strings.TrimSpace(line[idx+len("OS version:"):])
				if ver != "" {
					info.OSVersion = ver
				}
			}
		}
	}
	// 4. 回退到 /etc/os-release PRETTY_NAME
	if info.OSVersion == "" {
		if out, err := SSHExec(client, "cat /etc/os-release 2>/dev/null", timeout); err == nil {
			for _, line := range strings.Split(out, "\n") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 && strings.TrimSpace(parts[0]) == "PRETTY_NAME" {
					info.OSVersion = strings.Trim(strings.TrimSpace(parts[1]), "\"")
					break
				}
			}
		}
	}

	// kernel + arch
	if out, err := SSHExec(client, "uname -r 2>/dev/null", timeout); err == nil {
		info.Kernel = strings.TrimSpace(out)
	}
	if out, err := SSHExec(client, "uname -m 2>/dev/null", timeout); err == nil {
		info.Arch = strings.TrimSpace(out)
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
