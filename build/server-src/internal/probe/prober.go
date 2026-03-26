package probe

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"opsboard/server/internal/model"
	"opsboard/server/internal/store"
	"opsboard/server/internal/ws"
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
	for _, rule := range rules {
		go p.probeOne(rule)
	}
}

func (p *Prober) probeOne(rule model.ProbeRule) {
	start := time.Now()
	addr := fmt.Sprintf("%s:%d", rule.Host, rule.Port)
	timeout := time.Duration(rule.TimeoutSec) * time.Second
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	conn, err := net.DialTimeout("tcp", addr, timeout)
	latency := time.Since(start).Seconds() * 1000

	result := &model.ProbeResult{
		RuleID:    rule.ID,
		Name:      rule.Name,
		Host:      rule.Host,
		Port:      rule.Port,
		LatencyMs: latency,
		CheckedAt: time.Now().Unix(),
	}

	if err != nil {
		result.Status = "down"
		result.Error = err.Error()
	} else {
		result.Status = "up"
		conn.Close()
	}

	p.mu.Lock()
	p.results[rule.ID] = result
	p.mu.Unlock()

	// 写入 VictoriaMetrics
	statusVal := 0
	if result.Status == "up" {
		statusVal = 1
	}
	lines := []string{
		fmt.Sprintf(`opsboard_probe_status{probe_id="%d",name="%s",target_host="%s",port="%d"} %d`,
			rule.ID, rule.Name, rule.Host, rule.Port, statusVal),
		fmt.Sprintf(`opsboard_probe_latency_ms{probe_id="%d",name="%s",target_host="%s",port="%d"} %f`,
			rule.ID, rule.Name, rule.Host, rule.Port, latency),
	}
	if err := p.vmStore.WriteMetrics(lines); err != nil {
		log.Printf("probe VM write error: %v", err)
	}

	// 推送 WebSocket
	p.hub.BroadcastJSON(map[string]interface{}{
		"type": "probe",
		"data": result,
	})
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
