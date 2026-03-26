package store

import "testing"

func TestInitSQLite(t *testing.T) {
	path := t.TempDir() + "/test.db"
	db, err := InitSQLite(path)
	if err != nil {
		t.Fatalf("InitSQLite failed: %v", err)
	}
	defer db.Close()

	tables := []string{"servers", "assets", "probe_rules"}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", table, err)
		}
	}
}

func TestInitSQLite_Idempotent(t *testing.T) {
	path := t.TempDir() + "/test.db"
	db1, _ := InitSQLite(path)
	db1.Close()
	db2, err := InitSQLite(path)
	if err != nil {
		t.Fatalf("second InitSQLite failed: %v", err)
	}
	db2.Close()
}
