//go:build linux

package collector

import (
	"net"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
)

type HostInfo struct {
	Hostname    string
	OS          string
	Kernel      string
	Arch        string
	BootTime    int64
	IPAddresses []string
	CPUModel    string
	CPUCores    int
	MemoryTotal uint64
	DiskTotal   uint64
}

func CollectHostInfo() (*HostInfo, error) {
	hostname, _ := os.Hostname()
	var utsname syscall.Utsname
	syscall.Uname(&utsname)
	kernel := int8sToString(utsname.Release[:])

	osName := "Linux"
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				osName = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
				break
			}
		}
	}

	var cpuModel string
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					cpuModel = strings.TrimSpace(parts[1])
				}
				break
			}
		}
	}

	var sysinfo syscall.Sysinfo_t
	syscall.Sysinfo(&sysinfo)
	bootTime := time.Now().Unix() - int64(sysinfo.Uptime)
	memTotal := sysinfo.Totalram

	ips := getIPs()

	return &HostInfo{
		Hostname:    hostname,
		OS:          osName,
		Kernel:      kernel,
		Arch:        runtime.GOARCH,
		BootTime:    bootTime,
		IPAddresses: ips,
		CPUModel:    cpuModel,
		CPUCores:    runtime.NumCPU(),
		MemoryTotal: memTotal,
	}, nil
}

func getIPs() []string {
	var ips []string
	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			ips = append(ips, ipnet.IP.String())
		}
	}
	return ips
}

func int8sToString(arr []int8) string {
	b := make([]byte, 0, len(arr))
	for _, c := range arr {
		if c == 0 {
			break
		}
		b = append(b, byte(c))
	}
	return string(b)
}
