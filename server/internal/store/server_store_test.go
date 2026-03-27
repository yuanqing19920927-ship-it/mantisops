package store

import (
	"testing"

	"mantisops/server/internal/model"
)

func setupTestDB(t *testing.T) *ServerStore {
	db, err := InitSQLite(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewServerStore(db)
}

func TestServerStore_Upsert_And_List(t *testing.T) {
	store := setupTestDB(t)
	srv := &model.Server{
		HostID: "test-host-1", Hostname: "testbox",
		OS: "Debian 13", CPUCores: 4, MemoryTotal: 16000000000,
	}
	if err := store.Upsert(srv); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	servers, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(servers) != 1 || servers[0].HostID != "test-host-1" {
		t.Fatalf("unexpected: %+v", servers)
	}
}

func TestServerStore_Upsert_Updates(t *testing.T) {
	store := setupTestDB(t)
	srv := &model.Server{HostID: "host-1", Hostname: "old"}
	store.Upsert(srv)
	srv.Hostname = "new"
	store.Upsert(srv)
	result, _ := store.GetByHostID("host-1")
	if result.Hostname != "new" {
		t.Fatalf("expected 'new', got '%s'", result.Hostname)
	}
}

func TestServerStore_MarkOffline(t *testing.T) {
	store := setupTestDB(t)
	srv := &model.Server{HostID: "host-1", Hostname: "test"}
	store.Upsert(srv)
	// Set last_seen to a past value so MarkOffline can catch it
	store.db.Exec("UPDATE servers SET last_seen = 1000 WHERE host_id = 'host-1'")
	store.MarkOffline(0)
	result, _ := store.GetByHostID("host-1")
	if result.Status != "offline" {
		t.Fatalf("expected offline, got %s", result.Status)
	}
}
