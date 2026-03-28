package logging

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mantisops/server/internal/config"
)

type Cleaner struct {
	logDir    string
	store     *LogStore
	retention config.RetentionConfig
	hour      int
	stopCh    chan struct{}
}

func NewCleaner(logDir string, store *LogStore, retention config.RetentionConfig, hour int) *Cleaner {
	return &Cleaner{
		logDir:    logDir,
		store:     store,
		retention: retention,
		hour:      hour,
		stopCh:    make(chan struct{}),
	}
}

func (c *Cleaner) Start() {
	go c.loop()
}

func (c *Cleaner) Stop() {
	close(c.stopCh)
}

func (c *Cleaner) loop() {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), c.hour, 0, 0, 0, now.Location())
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		timer := time.NewTimer(time.Until(next))

		select {
		case <-c.stopCh:
			timer.Stop()
			return
		case <-timer.C:
			c.run()
		}
	}
}

func (c *Cleaner) run() {
	log.Println("[logging] starting cleanup...")

	auditBefore := time.Now().Add(-time.Duration(c.retention.AuditDays) * 24 * time.Hour)
	systemBefore := time.Now().Add(-time.Duration(c.retention.SystemDays) * 24 * time.Hour)
	agentBefore := time.Now().Add(-time.Duration(c.retention.AgentDays) * 24 * time.Hour)

	// Clean DB
	auditDel, _ := c.store.CleanupAuditBefore(auditBefore)
	logDel, _ := c.store.CleanupLogIndexBefore(agentBefore) // use shortest retention for index

	// Clean files
	auditFiles := c.cleanDir(filepath.Join(c.logDir, "audit"), auditBefore)
	systemFiles := c.cleanDir(filepath.Join(c.logDir, "system"), systemBefore)
	agentFiles := c.cleanAgentDir(filepath.Join(c.logDir, "agent"), agentBefore)

	log.Printf("[logging] cleanup done: audit=%d/%d system=%d agent=%d index=%d",
		auditDel, auditFiles, systemFiles, agentFiles, logDel)
}

func (c *Cleaner) cleanDir(dir string, before time.Time) int {
	count := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		dateStr := strings.TrimSuffix(e.Name(), ".log")
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		if t.Before(before) {
			os.Remove(filepath.Join(dir, e.Name()))
			count++
		}
	}
	return count
}

func (c *Cleaner) cleanAgentDir(dir string, before time.Time) int {
	count := 0
	hostDirs, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, hd := range hostDirs {
		if !hd.IsDir() {
			continue
		}
		count += c.cleanDir(filepath.Join(dir, hd.Name()), before)
	}
	return count
}
