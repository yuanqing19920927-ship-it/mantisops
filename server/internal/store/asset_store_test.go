package store

import (
	"opsboard/server/internal/model"
	"testing"
)

func TestAssetStore_CRUD(t *testing.T) {
	db, err := InitSQLite(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 先创建 server（外键依赖）
	ss := NewServerStore(db)
	ss.Upsert(&model.Server{HostID: "h1", Hostname: "test"})
	servers, _ := ss.List()
	serverID := servers[0].ID

	as := NewAssetStore(db)

	// Create
	id, err := as.Create(&model.Asset{
		ServerID:  serverID,
		Name:      "RAGFlow",
		Category:  "项目",
		TechStack: "Docker",
		Path:      "~/ragflow",
		Port:      "80",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id <= 0 {
		t.Fatal("id should be > 0")
	}

	// List
	assets, err := as.List()
	if err != nil || len(assets) != 1 {
		t.Fatalf("list: %v, count=%d", err, len(assets))
	}
	if assets[0].Name != "RAGFlow" {
		t.Fatalf("name mismatch: %s", assets[0].Name)
	}

	// ListByServer
	byServer, err := as.ListByServer(serverID)
	if err != nil || len(byServer) != 1 {
		t.Fatalf("listByServer: %v, count=%d", err, len(byServer))
	}

	// Update
	assets[0].Name = "RAGFlow v2"
	if err := as.Update(&assets[0]); err != nil {
		t.Fatalf("update: %v", err)
	}
	updated, _ := as.List()
	if updated[0].Name != "RAGFlow v2" {
		t.Fatalf("update failed: %s", updated[0].Name)
	}

	// Delete
	if err := as.Delete(int(id)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	after, _ := as.List()
	if len(after) != 0 {
		t.Fatal("delete failed")
	}
}
