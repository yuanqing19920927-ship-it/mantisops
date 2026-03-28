package ai

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"mantisops/server/internal/store"
)

// Scheduler checks cron-based schedules every minute and triggers report
// generation via the Reporter when a schedule is due.
type Scheduler struct {
	store    *store.AIStore
	reporter *Reporter
	timezone *time.Location
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewScheduler creates a Scheduler. The timezone string (e.g. "Asia/Shanghai")
// is used for cron calculations; it falls back to UTC on parse failure.
func NewScheduler(aiStore *store.AIStore, reporter *Reporter, timezone string) *Scheduler {
	tz, err := time.LoadLocation(timezone)
	if err != nil {
		log.Printf("[scheduler] invalid timezone %q, falling back to UTC: %v", timezone, err)
		tz = time.UTC
	}
	return &Scheduler{
		store:    aiStore,
		reporter: reporter,
		timezone: tz,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the scheduler loop. It cleans up stale reports, initialises
// next-run times for all enabled schedules, and starts a 1-minute ticker.
func (s *Scheduler) Start() {
	// Clean up reports that were pending/generating when the server last stopped.
	if err := s.store.CleanupStaleReports(); err != nil {
		log.Printf("[scheduler] cleanup stale reports: %v", err)
	}

	// Calculate next_run_at for all enabled schedules.
	s.initSchedules()

	s.wg.Add(1)
	go s.loop()
	log.Println("[scheduler] started")
}

// Stop signals the scheduler to stop and waits for the loop goroutine to exit.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	log.Println("[scheduler] stopped")
}

// loop is the main scheduler goroutine. It ticks every minute and checks for
// due schedules.
func (s *Scheduler) loop() {
	defer s.wg.Done()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

// tick checks all enabled schedules and triggers report generation for any
// that are due.
func (s *Scheduler) tick() {
	schedules, err := s.store.ListSchedules()
	if err != nil {
		log.Printf("[scheduler] list schedules: %v", err)
		return
	}

	now := time.Now().In(s.timezone)
	nowUnix := now.Unix()

	for _, sc := range schedules {
		if !sc.Enabled {
			continue
		}
		if sc.NextRunAt == nil {
			continue
		}
		if nowUnix < *sc.NextRunAt {
			continue
		}

		// Schedule is due — trigger generation.
		schedID := sc.ID
		reportType := sc.ReportType
		cronExpr := sc.CronExpr

		// Update last_run_at and next_run_at immediately so the schedule is not
		// triggered again on the next tick.
		nextRun, err := s.calcNextRun(cronExpr, now)
		if err != nil {
			log.Printf("[scheduler] calc next run for schedule %d: %v", schedID, err)
			continue
		}
		if err := s.store.UpdateScheduleRun(schedID, nowUnix, nextRun); err != nil {
			log.Printf("[scheduler] update schedule %d run times: %v", schedID, err)
			continue
		}

		// Launch report generation in a goroutine so the scheduler is not blocked.
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			log.Printf("[scheduler] triggering %s report (schedule %d)", reportType, schedID)
			ctx := context.Background()
			if _, err := s.reporter.Generate(ctx, reportType, 0, 0, "scheduled", false); err != nil {
				log.Printf("[scheduler] report generation failed for schedule %d: %v", schedID, err)
			}
		}()
	}
}

// initSchedules calculates next_run_at for all enabled schedules that have no
// next run time or whose next run is in the past. Past runs are skipped (no
// backfill).
func (s *Scheduler) initSchedules() {
	schedules, err := s.store.ListSchedules()
	if err != nil {
		log.Printf("[scheduler] init: list schedules: %v", err)
		return
	}

	now := time.Now().In(s.timezone)
	nowUnix := now.Unix()

	for _, sc := range schedules {
		if !sc.Enabled {
			continue
		}

		// If next_run_at is set and in the future, nothing to do.
		if sc.NextRunAt != nil && *sc.NextRunAt > nowUnix {
			continue
		}

		// Calculate next run from now (skip past, don't backfill).
		nextRun, err := s.calcNextRun(sc.CronExpr, now)
		if err != nil {
			log.Printf("[scheduler] init: bad cron expr for schedule %d (%q): %v", sc.ID, sc.CronExpr, err)
			continue
		}

		// Update only next_run_at; preserve existing last_run_at.
		lastRun := nowUnix
		if sc.LastRunAt != nil {
			lastRun = *sc.LastRunAt
		}
		if err := s.store.UpdateScheduleRun(sc.ID, lastRun, nextRun); err != nil {
			log.Printf("[scheduler] init: update schedule %d: %v", sc.ID, err)
		}
	}
}

// calcNextRun parses a standard 5-field cron expression and returns the next
// run time (as a Unix timestamp) after the given time, in the scheduler's
// configured timezone.
func (s *Scheduler) calcNextRun(cronExpr string, after time.Time) (int64, error) {
	sched, err := cron.ParseStandard(cronExpr)
	if err != nil {
		return 0, err
	}
	next := sched.Next(after.In(s.timezone))
	return next.Unix(), nil
}
