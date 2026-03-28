package collector

import (
	"encoding/json"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type SynologySystemInfo struct {
	Model     string `json:"model"`
	Serial    string `json:"serial"`
	OSVersion string `json:"os_version"`
	Kernel    string `json:"kernel"`
	Arch      string `json:"arch"`
}

type SynologyPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Status  string `json:"status"` // running / stopped
}

// collectSynologyInfo 采集群晖静态系统信息（仅首次调用）
func collectSynologyInfo(client *ssh.Client) (*SynologySystemInfo, error) {
	timeout := 30 * time.Second
	info := &SynologySystemInfo{}

	// synoinfo.conf
	out, err := SSHExec(client, "cat /etc/synoinfo.conf 2>/dev/null", timeout)
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.Trim(strings.TrimSpace(parts[1]), "\"")
			switch key {
			case "upnpmodelname":
				info.Model = val
			case "unique":
				info.Serial = val
			}
		}
	}

	// VERSION
	out, err = SSHExec(client, "cat /etc.defaults/VERSION 2>/dev/null", timeout)
	if err == nil {
		var major, minor, build string
		for _, line := range strings.Split(out, "\n") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.Trim(strings.TrimSpace(parts[1]), "\"")
			switch key {
			case "majorversion":
				major = val
			case "minorversion":
				minor = val
			case "buildnumber":
				build = val
			}
		}
		info.OSVersion = "DSM " + major + "." + minor + "-" + build
	}

	// kernel + arch
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

// collectSynologyPackages 采集群晖已安装套件列表
func collectSynologyPackages(client *ssh.Client) ([]SynologyPackage, error) {
	timeout := 30 * time.Second
	out, err := SSHExec(client, "synopkg list --format json 2>/dev/null || echo '[]'", timeout)
	if err != nil {
		return nil, err
	}
	var pkgs []SynologyPackage
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &pkgs); err != nil {
		// 如果 JSON 解析失败，尝试按行解析（旧版 DSM 可能不支持 --format json）
		for _, line := range strings.Split(out, "\n") {
			name := strings.TrimSpace(line)
			if name != "" && name != "[]" {
				pkgs = append(pkgs, SynologyPackage{Name: name, Status: "running"})
			}
		}
	}
	return pkgs, nil
}
