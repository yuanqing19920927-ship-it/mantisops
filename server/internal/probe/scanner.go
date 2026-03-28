package probe

import (
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"mantisops/server/internal/model"
	"mantisops/server/internal/store"
	"mantisops/server/internal/ws"
)

var httpPorts = map[int]bool{80: true, 8080: true, 9090: true}
var httpsPorts = map[int]bool{443: true, 8443: true}

func inferProtocol(port int) string {
	if httpPorts[port] {
		return "http"
	}
	if httpsPorts[port] {
		return "https"
	}
	return "tcp"
}

func generateURL(ip string, port int, proto string) string {
	switch proto {
	case "http":
		if port == 80 {
			return fmt.Sprintf("http://%s/", ip)
		}
		return fmt.Sprintf("http://%s:%d/", ip, port)
	case "https":
		if port == 443 {
			return fmt.Sprintf("https://%s/", ip)
		}
		return fmt.Sprintf("https://%s:%d/", ip, port)
	}
	return ""
}

type ScanTarget struct {
	HostID     string
	ServerID   int
	ServerName string
	IP         string
	Source     string // agent/managed/cloud
}

type Scanner struct {
	probeStore    *store.ProbeStore
	templateStore *store.ScanTemplateStore
	hub           *ws.Hub
}

func NewScanner(ps *store.ProbeStore, ts *store.ScanTemplateStore, hub *ws.Hub) *Scanner {
	return &Scanner{probeStore: ps, templateStore: ts, hub: hub}
}

func (s *Scanner) Scan(targets []ScanTarget) string {
	taskID := fmt.Sprintf("scan-%d", time.Now().UnixMilli())
	go s.runScan(taskID, targets)
	return taskID
}

func (s *Scanner) runScan(taskID string, targets []ScanTarget) {
	templates, err := s.templateStore.ListEnabled()
	if err != nil {
		log.Printf("[scanner] load templates: %v", err)
		return
	}

	type job struct {
		target   ScanTarget
		template store.ScanTemplate
	}
	var jobs []job
	for _, t := range targets {
		for _, tmpl := range templates {
			if t.Source == "cloud" && tmpl.Port != 80 && tmpl.Port != 443 {
				continue
			}
			jobs = append(jobs, job{target: t, template: tmpl})
		}
	}

	total := len(jobs)
	var done, found, created, skipped int64

	// Dedup against existing rules
	existingRules, _ := s.probeStore.List()
	existing := make(map[string]bool)
	for _, r := range existingRules {
		existing[fmt.Sprintf("%s:%d", r.Host, r.Port)] = true
	}

	sem := make(chan struct{}, 20)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			addr := fmt.Sprintf("%s:%d", j.target.IP, j.template.Port)
			conn, dialErr := net.DialTimeout("tcp", addr, 3*time.Second)
			atomic.AddInt64(&done, 1)

			if dialErr == nil {
				conn.Close()
				atomic.AddInt64(&found, 1)

				key := fmt.Sprintf("%s:%d", j.target.IP, j.template.Port)
				mu.Lock()
				if existing[key] {
					atomic.AddInt64(&skipped, 1)
					mu.Unlock()
					return
				}
				existing[key] = true
				mu.Unlock()

				proto := inferProtocol(j.template.Port)
				rule := &model.ProbeRule{
					ServerID:     &j.target.ServerID,
					Name:         fmt.Sprintf("%s-%s", j.target.ServerName, j.template.Name),
					Host:         j.target.IP,
					Port:         j.template.Port,
					Protocol:     proto,
					URL:          generateURL(j.target.IP, j.template.Port, proto),
					ExpectStatus: 200,
					IntervalSec:  30,
					TimeoutSec:   5,
					Enabled:      true,
					Source:       "scan",
				}
				if proto == "tcp" {
					rule.ExpectStatus = 0
				}
				if _, err := s.probeStore.Create(rule); err != nil {
					log.Printf("[scanner] create rule: %v", err)
				} else {
					atomic.AddInt64(&created, 1)
				}
			}

			if s.hub != nil {
				s.hub.BroadcastAdmin(map[string]interface{}{
					"type": "scan_progress",
					"data": map[string]interface{}{
						"task_id": taskID,
						"total":   total,
						"scanned": atomic.LoadInt64(&done),
						"found":   atomic.LoadInt64(&found),
					},
				})
			}
		}(j)
	}
	wg.Wait()

	if s.hub != nil {
		s.hub.BroadcastAdmin(map[string]interface{}{
			"type": "scan_complete",
			"data": map[string]interface{}{
				"task_id": taskID,
				"open_ports":    found,
				"rules_created": created,
				"skipped":       skipped,
			},
		})
	}
	log.Printf("[scanner] %s complete: %d/%d open, created=%d skipped=%d", taskID, found, total, created, skipped)
}
