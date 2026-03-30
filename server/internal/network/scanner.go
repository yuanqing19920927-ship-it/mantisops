package network

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	probing "github.com/prometheus-community/pro-bing"

	"mantisops/server/internal/config"
	"mantisops/server/internal/ws"
)

// ScanResult holds the outcome of probing a single host.
type ScanResult struct {
	IP    string `json:"ip"`
	MAC   string `json:"mac"`
	Alive bool   `json:"alive"`
}

// ScanJob describes the current (or last) scan operation.
type ScanJob struct {
	Status        string  `json:"status"`         // idle | scanning | completed | cancelled | failed
	CurrentSubnet string  `json:"current_subnet"` // CIDR being scanned right now
	Progress      float64 `json:"progress"`       // 0.0 – 1.0
	StartedAt     *string `json:"started_at"`
	Error         string  `json:"error,omitempty"`

	cancel context.CancelFunc `json:"-"`
}

// Scanner runs ICMP ping sweeps across one or more subnets.
type Scanner struct {
	cfg config.NetworkConfig
	hub *ws.Hub
	mu  sync.Mutex
	job ScanJob
}

// NewScanner constructs a Scanner backed by the given config and WebSocket hub.
func NewScanner(cfg config.NetworkConfig, hub *ws.Hub) *Scanner {
	return &Scanner{
		cfg: cfg,
		hub: hub,
		job: ScanJob{Status: "idle"},
	}
}

// GetStatus returns a snapshot of the current job state.
func (s *Scanner) GetStatus() ScanJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.job
}

// Cancel stops a running scan.  No-op if idle.
func (s *Scanner) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job.Status == "scanning" && s.job.cancel != nil {
		s.job.cancel()
	}
}

// StartScan begins an asynchronous scan of the provided subnets.
// Returns an error immediately if a scan is already in progress.
func (s *Scanner) StartScan(ctx context.Context, subnets []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.job.Status == "scanning" {
		return fmt.Errorf("scan already in progress")
	}

	scanCtx, cancel := context.WithCancel(ctx)
	now := time.Now().Format(time.RFC3339)
	s.job = ScanJob{
		Status:    "scanning",
		StartedAt: &now,
		cancel:    cancel,
	}

	go s.runScan(scanCtx, subnets)
	return nil
}

// runScan executes the full scan sequentially across subnets.
func (s *Scanner) runScan(ctx context.Context, subnets []string) {
	var allResults []ScanResult

	for _, cidr := range subnets {
		// Update current subnet.
		s.mu.Lock()
		s.job.CurrentSubnet = cidr
		s.mu.Unlock()

		results := s.scanSubnet(ctx, cidr)
		allResults = append(allResults, results...)

		// Check for cancellation between subnets.
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.job.Status = "cancelled"
			s.job.cancel = nil
			s.mu.Unlock()
			s.broadcastAdmin("network_scan_job_done", map[string]interface{}{
				"status":  "cancelled",
				"results": allResults,
			})
			// Auto-reset to idle after 5 minutes.
			go s.autoReset(5 * time.Minute)
			return
		default:
		}
	}

	s.mu.Lock()
	s.job.Status = "completed"
	s.job.Progress = 1.0
	s.job.CurrentSubnet = ""
	s.job.cancel = nil
	s.mu.Unlock()

	s.broadcastAdmin("network_scan_job_done", map[string]interface{}{
		"status":  "completed",
		"results": allResults,
	})

	go s.autoReset(5 * time.Minute)
}

// scanSubnet pings every host in cidr concurrently and returns live hosts.
func (s *Scanner) scanSubnet(ctx context.Context, cidr string) []ScanResult {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		log.Printf("[network/scanner] invalid CIDR %q: %v", cidr, err)
		return nil
	}

	hosts := expandCIDR(ipNet)
	total := len(hosts)
	if total == 0 {
		return nil
	}

	concurrency := s.cfg.Scan.ICMPConcurrency
	if concurrency <= 0 {
		concurrency = 64
	}
	intervalMs := s.cfg.Scan.ICMPIntervalMs
	if intervalMs <= 0 {
		intervalMs = 5
	}

	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var results []ScanResult
	var wg sync.WaitGroup
	done := 0

	for i, ip := range hosts {
		select {
		case <-ctx.Done():
			break
		default:
		}

		// Rate limiting: sleep between launches.
		if i > 0 && intervalMs > 0 {
			time.Sleep(time.Duration(intervalMs) * time.Millisecond)
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(ipStr string, idx int) {
			defer wg.Done()
			defer func() { <-sem }()

			alive := s.pingHost(ctx, ipStr)
			result := ScanResult{IP: ipStr, Alive: alive}

			mu.Lock()
			results = append(results, result)
			done++
			localDone := done
			mu.Unlock()

			// Broadcast progress every 10 hosts.
			if localDone%10 == 0 || localDone == total {
				progress := float64(localDone) / float64(total)
				s.mu.Lock()
				s.job.Progress = progress
				s.mu.Unlock()
				s.broadcastAdmin("network_scan_progress", map[string]interface{}{
					"subnet":   cidr,
					"done":     localDone,
					"total":    total,
					"progress": progress,
				})
			}
		}(ip, i)
	}

	wg.Wait()

	s.broadcastAdmin("network_scan_subnet_done", map[string]interface{}{
		"subnet":  cidr,
		"total":   total,
		"results": results,
	})

	return results
}

// pingHost sends a single ICMP echo to ip and returns true if it responds.
func (s *Scanner) pingHost(ctx context.Context, ip string) bool {
	timeoutMs := s.cfg.Scan.ICMPTimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 1000
	}
	return pingOnce(ip, timeoutMs)
}

// ValidateCIDR checks that cidr is valid and has a prefix length of at least /24.
func ValidateCIDR(cidr string) error {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR notation: %w", err)
	}
	ones, _ := ipNet.Mask.Size()
	if ones < 24 {
		return fmt.Errorf("prefix length /%d is too broad; minimum is /24", ones)
	}
	return nil
}

// ---- helpers ----------------------------------------------------------------

// expandCIDR returns all usable host IP strings in the network (excludes
// network address and broadcast address).
func expandCIDR(ipNet *net.IPNet) []string {
	var hosts []string
	for ip := cloneIP(ipNet.IP.Mask(ipNet.Mask)); ipNet.Contains(ip); inc(ip) {
		if ip.Equal(ipNet.IP) {
			continue // network address
		}
		if isBroadcast(ip, ipNet) {
			continue
		}
		hosts = append(hosts, ip.String())
	}
	return hosts
}

// cloneIP returns a deep copy of ip to avoid mutations sharing underlying slice.
func cloneIP(ip net.IP) net.IP {
	dup := make(net.IP, len(ip))
	copy(dup, ip)
	return dup
}

// inc increments an IP address in place.
func inc(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

// isBroadcast returns true if ip is the broadcast address of ipNet.
func isBroadcast(ip net.IP, ipNet *net.IPNet) bool {
	// Broadcast = network address OR all host bits set.
	ip4 := ip.To4()
	mask4 := ipNet.Mask
	if ip4 == nil || len(mask4) != 4 {
		return false
	}
	ipInt := binary.BigEndian.Uint32(ip4)
	maskInt := binary.BigEndian.Uint32(mask4)
	broadcast := (binary.BigEndian.Uint32(ipNet.IP.To4()) & maskInt) | ^maskInt
	return ipInt == broadcast
}

// autoReset transitions the job back to idle after delay.
func (s *Scanner) autoReset(delay time.Duration) {
	time.Sleep(delay)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job.Status != "scanning" {
		s.job = ScanJob{Status: "idle"}
	}
}

// broadcastAdmin is a nil-safe wrapper around hub.BroadcastAdmin.
func (s *Scanner) broadcastAdmin(msgType string, payload map[string]interface{}) {
	if s.hub == nil {
		return
	}
	payload["type"] = msgType
	s.hub.BroadcastAdmin(payload)
}

// pingOnce sends one ICMP echo to ip with the given timeout in milliseconds.
// It uses SetPrivileged(true) so that it works without raw-socket capabilities
// when run as root.
func pingOnce(ip string, timeoutMs int) bool {
	pinger, err := probing.NewPinger(ip)
	if err != nil {
		return false
	}
	pinger.SetPrivileged(true)
	pinger.Count = 1
	pinger.Timeout = time.Duration(timeoutMs) * time.Millisecond

	if err := pinger.Run(); err != nil {
		return false
	}
	return pinger.Statistics().PacketsRecv > 0
}
