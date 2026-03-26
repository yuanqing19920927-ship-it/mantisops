package store

import (
	"database/sql"
	"encoding/hex"
	"testing"
)

func setupTestSQLite(t *testing.T) *sql.DB {
	db, err := InitSQLite(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func testMasterKey() []byte {
	key, _ := hex.DecodeString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	return key
}

func TestCredentialStore_CreateAndGet(t *testing.T) {
	db := setupTestSQLite(t)
	cs := NewCredentialStore(db, testMasterKey())

	id, err := cs.Create("test-ssh", "ssh_password", map[string]string{"password": "secret"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id < 1 {
		t.Fatalf("expected positive id, got %d", id)
	}

	cred, err := cs.Get(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cred.Name != "test-ssh" {
		t.Errorf("name = %q, want %q", cred.Name, "test-ssh")
	}
	if cred.Type != "ssh_password" {
		t.Errorf("type = %q, want %q", cred.Type, "ssh_password")
	}
	if cred.Data["password"] != "secret" {
		t.Errorf("password = %q, want %q", cred.Data["password"], "secret")
	}
}

func TestCredentialStore_List(t *testing.T) {
	db := setupTestSQLite(t)
	cs := NewCredentialStore(db, testMasterKey())
	cs.Create("cred1", "ssh_password", map[string]string{"password": "x"})
	cs.Create("cred2", "aliyun_ak", map[string]string{"access_key_id": "ak", "access_key_secret": "sk"})

	list, err := cs.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 credentials, got %d", len(list))
	}
	if list[0].Name != "cred1" {
		t.Errorf("first name = %q", list[0].Name)
	}
}

func TestCredentialStore_Update(t *testing.T) {
	db := setupTestSQLite(t)
	cs := NewCredentialStore(db, testMasterKey())
	id, _ := cs.Create("original", "ssh_password", map[string]string{"password": "old"})

	err := cs.Update(id, "updated", map[string]string{"password": "new"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	cred, _ := cs.Get(id)
	if cred.Name != "updated" {
		t.Errorf("name = %q", cred.Name)
	}
	if cred.Data["password"] != "new" {
		t.Errorf("password = %q", cred.Data["password"])
	}
}

func TestCredentialStore_Delete(t *testing.T) {
	db := setupTestSQLite(t)
	cs := NewCredentialStore(db, testMasterKey())
	id, _ := cs.Create("to-delete", "ssh_password", map[string]string{"password": "x"})

	err := cs.Delete(id)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = cs.Get(id)
	if err == nil {
		t.Error("expected error getting deleted credential")
	}
}

func TestCredentialStore_Delete_WithReference(t *testing.T) {
	db := setupTestSQLite(t)
	cs := NewCredentialStore(db, testMasterKey())
	id, _ := cs.Create("ref-test", "ssh_password", map[string]string{"password": "x"})

	// Insert a managed_server referencing this credential
	db.Exec("INSERT INTO managed_servers (host, ssh_user, credential_id) VALUES ('1.2.3.4', 'root', ?)", id)

	err := cs.Delete(id)
	if err == nil {
		t.Error("expected error deleting referenced credential")
	}
}
