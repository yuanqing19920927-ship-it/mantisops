//go:build linux

package collector

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ListeningService struct {
	PID      int
	Name     string
	CmdLine  string
	Port     int
	Protocol string
	BindAddr string
}

// CollectListeningServices scans /proc/net/tcp and /proc/net/tcp6
// for listening (state=0A) sockets bound to non-loopback addresses.
func CollectListeningServices() ([]ListeningService, error) {
	// Pre-build inode → PID map for efficiency
	inodeMap := buildInodeMap()

	var services []ListeningService
	seen := make(map[string]bool)

	for _, proto := range []string{"tcp", "tcp6"} {
		entries, err := parseProcNet("/proc/net/" + proto)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.state != "0A" {
				continue
			}
			if isLoopback(e.localAddr) {
				continue
			}
			pid := inodeMap[e.inode]
			if pid <= 0 {
				continue
			}
			key := fmt.Sprintf("%d:%d:%s:%s", pid, e.localPort, proto, e.localAddr)
			if seen[key] {
				continue
			}
			seen[key] = true

			name := readComm(pid)
			cmdLine := readCmdLine(pid)
			if len(cmdLine) > 200 {
				cmdLine = cmdLine[:200]
			}

			services = append(services, ListeningService{
				PID:      pid,
				Name:     name,
				CmdLine:  cmdLine,
				Port:     e.localPort,
				Protocol: proto,
				BindAddr: e.localAddr,
			})
		}
	}
	return services, nil
}

type procNetEntry struct {
	localAddr string
	localPort int
	state     string
	inode     int
}

func parseProcNet(path string) ([]procNetEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []procNetEntry
	scanner := bufio.NewScanner(f)
	scanner.Scan() // skip header
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}
		localParts := strings.Split(fields[1], ":")
		if len(localParts) != 2 {
			continue
		}
		port, _ := strconv.ParseInt(localParts[1], 16, 32)
		inode, _ := strconv.Atoi(fields[9])
		addr := hexToIP(localParts[0])

		entries = append(entries, procNetEntry{
			localAddr: addr,
			localPort: int(port),
			state:     fields[3],
			inode:     inode,
		})
	}
	return entries, nil
}

func hexToIP(h string) string {
	if len(h) == 8 {
		b, _ := hex.DecodeString(h)
		if len(b) == 4 {
			return net.IPv4(b[3], b[2], b[1], b[0]).String()
		}
	}
	if len(h) == 32 {
		b, _ := hex.DecodeString(h)
		if len(b) == 16 {
			for i := 0; i < 16; i += 4 {
				b[i], b[i+3] = b[i+3], b[i]
				b[i+1], b[i+2] = b[i+2], b[i+1]
			}
			return net.IP(b).String()
		}
	}
	return "0.0.0.0"
}

func isLoopback(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

// buildInodeMap scans all /proc/{pid}/fd/ to build inode→pid mapping.
func buildInodeMap() map[int]int {
	m := make(map[int]int)
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return m
	}
	for _, p := range procs {
		pid, err := strconv.Atoi(p.Name())
		if err != nil {
			continue
		}
		fdDir := fmt.Sprintf("/proc/%d/fd", pid)
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if strings.HasPrefix(link, "socket:[") && strings.HasSuffix(link, "]") {
				inode, _ := strconv.Atoi(link[8 : len(link)-1])
				if inode > 0 {
					m[inode] = pid
				}
			}
		}
	}
	return m
}

func readComm(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readCmdLine(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return ""
	}
	return strings.ReplaceAll(strings.TrimRight(string(data), "\x00"), "\x00", " ")
}
