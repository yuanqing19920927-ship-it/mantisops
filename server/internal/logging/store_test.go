package logging

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLogStore_InitSchema(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	s, err := NewLogStore(db)
	if err != nil {
		t.Fatal(err)
	}
	_ = s
	var count int
	db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&count)
	db.QueryRow("SELECT COUNT(*) FROM log_index").Scan(&count)
}

func TestLogStore_InsertAudit(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	s, _ := NewLogStore(db)
	err := s.InsertAudit(time.Now(), "admin", "create", "alert_rule", "1", "CPU Alert", `{"threshold":80}`, "192.168.1.1", "Mozilla/5.0")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 audit log, got %d", count)
	}
}

func TestLogStore_InsertLogIndex(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	s, _ := NewLogStore(db)
	err := s.InsertLogIndex(time.Now(), "error", "alerter", "server", "system/2026-03-28.log", 1024, 150, "FIRE rule=3 target=srv-71")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM log_index").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 log index, got %d", count)
	}
}

func TestLogStore_QueryAudit(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	s, _ := NewLogStore(db)
	now := time.Now()
	s.InsertAudit(now, "admin", "create", "alert_rule", "1", "Rule1", "", "1.2.3.4", "")
	s.InsertAudit(now, "admin", "delete", "probe", "2", "Probe1", "", "1.2.3.4", "")
	s.InsertAudit(now, "user2", "update", "asset", "3", "Asset1", "", "5.6.7.8", "")

	results, total, err := s.QueryAudit(AuditQuery{
		Start: now.Add(-time.Hour), End: now.Add(time.Hour),
		Username: "admin",
		Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("expected 2, got %d", total)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestLogStore_QueryLogIndex(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	s, _ := NewLogStore(db)
	now := time.Now()
	s.InsertLogIndex(now, "error", "alerter", "server", "system/2026-03-28.log", 0, 100, "FIRE rule=3")
	s.InsertLogIndex(now, "info", "collector", "agent:srv-71", "agent/srv-71/2026-03-28.log", 100, 80, "collected 12 metrics")

	results, total, err := s.QueryLogIndex(LogQuery{
		Start: now.Add(-time.Hour), End: now.Add(time.Hour),
		Level: "error",
		Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("expected 1, got %d", total)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestLogStore_CleanupOld(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	s, _ := NewLogStore(db)
	old := time.Now().Add(-100 * 24 * time.Hour)
	recent := time.Now()
	s.InsertAudit(old, "admin", "create", "probe", "1", "old", "", "", "")
	s.InsertAudit(recent, "admin", "create", "probe", "2", "new", "", "", "")

	deleted, err := s.CleanupAuditBefore(time.Now().Add(-50 * 24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}
}
