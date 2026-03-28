package probe

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"mantisops/server/internal/model"
	"mantisops/server/internal/store"
	"mantisops/server/internal/ws"
)

type Prober struct {
	probeStore *store.ProbeStore
	vmStore    *store.VictoriaStore
	hub        *ws.Hub
	mu         sync.RWMutex
	results    map[int]*model.ProbeResult // rule_id -> latest result
	stopCh     chan struct{}
}

func NewProber(ps *store.ProbeStore, vm *store.VictoriaStore, hub *ws.Hub) *Prober {
	return &Prober{
		probeStore: ps,
		vmStore:    vm,
		hub:        hub,
		results:    make(map[int]*model.ProbeResult),
		stopCh:     make(chan struct{}),
	}
}

func (p *Prober) Start(defaultInterval int) {
	go func() {
		ticker := time.NewTicker(time.Duration(defaultInterval) * time.Second)
		defer ticker.Stop()
		// 立即执行一次
		p.probeAll()
		for {
			select {
			case <-p.stopCh:
				return
			case <-ticker.C:
				p.probeAll()
			}
		}
	}()
}

func (p *Prober) Stop() {
	close(p.stopCh)
}

func (p *Prober) GetAllResults() []*model.ProbeResult {
	p.mu.RLock()
	defer p.mu.RUnlock()
	results := make([]*model.ProbeResult, 0, len(p.results))
	for _, r := range p.results {
		results = append(results, r)
	}
	return results
}

func (p *Prober) probeAll() {
	rules, err := p.probeStore.ListEnabled()
	if err != nil {
		log.Printf("probe list error: %v", err)
		return
	}

	// 清理已删除规则的旧结果
	activeIDs := make(map[int]bool, len(rules))
	for _, r := range rules {
		activeIDs[r.ID] = true
	}
	p.mu.Lock()
	for id := range p.results {
		if !activeIDs[id] {
			delete(p.results, id)
		}
	}
	p.mu.Unlock()

	for _, rule := range rules {
		go p.probeOne(rule)
	}
}

func (p *Prober) probeOne(rule model.ProbeRule) {
	var result *model.ProbeResult
	switch rule.Protocol {
	case "http", "https":
		result = p.probeHTTP(rule)
	default:
		result = p.probeTCP(rule)
	}

	p.mu.Lock()
	p.results[rule.ID] = result
	p.mu.Unlock()

	// VictoriaMetrics
	statusVal := 0
	if result.Status == "up" {
		statusVal = 1
	}
	lines := []string{
		fmt.Sprintf(`mantisops_probe_status{probe_id="%d",name="%s",target_host="%s",port="%d"} %d`,
			rule.ID, rule.Name, result.Host, result.Port, statusVal),
		fmt.Sprintf(`mantisops_probe_latency_ms{probe_id="%d",name="%s",target_host="%s",port="%d"} %f`,
			rule.ID, rule.Name, result.Host, result.Port, result.LatencyMs),
	}
	if result.HttpStatus > 0 {
		lines = append(lines, fmt.Sprintf(`mantisops_probe_http_status{probe_id="%d",name="%s",target_host="%s"} %d`,
			rule.ID, rule.Name, result.Host, result.HttpStatus))
	}
	if result.SSLExpiryDays != nil {
		lines = append(lines, fmt.Sprintf(`mantisops_probe_ssl_days_left{probe_id="%d",name="%s",target_host="%s"} %d`,
			rule.ID, rule.Name, result.Host, *result.SSLExpiryDays))
	}
	if err := p.vmStore.WriteMetrics(lines); err != nil {
		log.Printf("probe VM write error: %v", err)
	}
	p.hub.BroadcastMetrics(result.Host, map[string]interface{}{
		"type": "probe",
		"data": result,
	})
}

func (p *Prober) probeTCP(rule model.ProbeRule) *model.ProbeResult {
	start := time.Now()
	addr := fmt.Sprintf("%s:%d", rule.Host, rule.Port)
	timeout := time.Duration(rule.TimeoutSec) * time.Second
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	conn, err := net.DialTimeout("tcp", addr, timeout)
	latency := time.Since(start).Seconds() * 1000
	result := &model.ProbeResult{
		RuleID: rule.ID, Name: rule.Name,
		Host: rule.Host, Port: rule.Port,
		LatencyMs: latency, CheckedAt: time.Now().Unix(),
	}
	if err != nil {
		result.Status = "down"
		result.Error = err.Error()
	} else {
		result.Status = "up"
		conn.Close()
	}
	return result
}

func (p *Prober) probeHTTP(rule model.ProbeRule) *model.ProbeResult {
	timeout := time.Duration(rule.TimeoutSec) * time.Second
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	result := &model.ProbeResult{
		RuleID: rule.ID, Name: rule.Name,
		Host: rule.Host, Port: rule.Port,
		CheckedAt: time.Now().Unix(),
	}

	// HTTPS: collect SSL cert info first (independent TLS handshake with InsecureSkipVerify)
	if rule.Protocol == "https" {
		p.collectSSLInfo(rule, result, timeout)
	}

	// HTTP request (uses default strict TLS for HTTPS)
	client := &http.Client{Timeout: timeout}
	start := time.Now()
	resp, err := client.Get(rule.URL)
	result.LatencyMs = time.Since(start).Seconds() * 1000
	if err != nil {
		result.Status = "down"
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()
	result.HttpStatus = resp.StatusCode

	// Check status code (0 = skip check)
	if rule.ExpectStatus != 0 && resp.StatusCode != rule.ExpectStatus {
		result.Status = "down"
		result.Error = fmt.Sprintf("expected status %d, got %d", rule.ExpectStatus, resp.StatusCode)
		return result
	}

	// Check body keyword
	if rule.ExpectBody != "" {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if !strings.Contains(string(body), rule.ExpectBody) {
			result.Status = "down"
			result.Error = fmt.Sprintf("body does not contain '%s'", rule.ExpectBody)
			return result
		}
	}

	result.Status = "up"
	return result
}

func (p *Prober) collectSSLInfo(rule model.ProbeRule, result *model.ProbeResult, timeout time.Duration) {
	u, err := url.Parse(rule.URL)
	if err != nil {
		return
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "443"
	}
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", host+":"+port, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return
	}
	cert := certs[0]
	days := int(math.Ceil(time.Until(cert.NotAfter).Hours() / 24))
	result.SSLExpiryDays = &days
	if len(cert.Issuer.Organization) > 0 {
		result.SSLIssuer = cert.Issuer.Organization[0]
	} else {
		result.SSLIssuer = cert.Issuer.CommonName
	}
	result.SSLExpiryDate = cert.NotAfter.Format("2006-01-02")
}

// ProbeOnce 单次探测（用于测试）
func ProbeOnce(host string, port int, timeout time.Duration) (bool, float64) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
	latency := time.Since(start).Seconds() * 1000
	if err != nil {
		return false, latency
	}
	conn.Close()
	return true, latency
}
