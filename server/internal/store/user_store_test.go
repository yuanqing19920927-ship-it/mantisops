package store

import (
	"database/sql"
	"testing"
)

func initTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := InitSQLite(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestUserStore_CreateAndGet(t *testing.T) {
	db := initTestDB(t)
	defer db.Close()
	s := NewUserStore(db)

	id, err := s.Create("admin", "$2a$10$hashhere", "管理员", "admin")
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.GetByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "admin" || u.Role != "admin" || u.DisplayName != "管理员" {
		t.Fatalf("unexpected user: %+v", u)
	}
	if !u.Enabled || !u.MustChangePwd {
		t.Fatalf("expected enabled=true must_change=true, got enabled=%v must_change=%v", u.Enabled, u.MustChangePwd)
	}
}

func TestUserStore_CreateInitialAdmin(t *testing.T) {
	db := initTestDB(t)
	defer db.Close()
	s := NewUserStore(db)

	id, err := s.CreateInitialAdmin("admin", "$2a$10$hashhere")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := s.GetByID(id)
	if u.Role != "admin" {
		t.Fatalf("expected admin role, got %s", u.Role)
	}
	if u.MustChangePwd {
		t.Fatal("initial admin should not require password change")
	}
}

func TestUserStore_GetByUsername(t *testing.T) {
	db := initTestDB(t)
	defer db.Close()
	s := NewUserStore(db)
	s.Create("testuser", "hash", "", "viewer")

	u, err := s.GetByUsername("testuser")
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "testuser" {
		t.Fatalf("expected testuser, got %s", u.Username)
	}

	_, err = s.GetByUsername("nonexist")
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestUserStore_Update(t *testing.T) {
	db := initTestDB(t)
	defer db.Close()
	s := NewUserStore(db)
	id, _ := s.Create("u1", "hash", "", "viewer")

	err := s.Update(id, "显示名", "operator", true)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := s.GetByID(id)
	if u.DisplayName != "显示名" || u.Role != "operator" {
		t.Fatalf("update failed: %+v", u)
	}
}

func TestUserStore_IncrementTokenVersion(t *testing.T) {
	db := initTestDB(t)
	defer db.Close()
	s := NewUserStore(db)
	id, _ := s.Create("u1", "hash", "", "viewer")

	u1, _ := s.GetByID(id)
	s.IncrementTokenVersion(id)
	u2, _ := s.GetByID(id)
	if u2.TokenVersion != u1.TokenVersion+1 {
		t.Fatalf("expected version %d, got %d", u1.TokenVersion+1, u2.TokenVersion)
	}
}

func TestUserStore_CountEnabledAdmins(t *testing.T) {
	db := initTestDB(t)
	defer db.Close()
	s := NewUserStore(db)
	s.Create("a1", "h", "", "admin")
	s.Create("a2", "h", "", "admin")
	s.Create("v1", "h", "", "viewer")

	count, _ := s.CountEnabledAdmins()
	if count != 2 {
		t.Fatalf("expected 2 admins, got %d", count)
	}
}

func TestUserStore_Permissions(t *testing.T) {
	db := initTestDB(t)
	defer db.Close()
	s := NewUserStore(db)
	id, _ := s.Create("u1", "h", "", "operator")

	perms := []Permission{
		{ResType: "group", ResID: "1"},
		{ResType: "server", ResID: "srv-69-ai"},
		{ResType: "probe", ResID: "5"},
	}
	err := s.SetPermissions(id, perms)
	if err != nil {
		t.Fatal(err)
	}

	got, _ := s.GetPermissions(id)
	if len(got) != 3 {
		t.Fatalf("expected 3 permissions, got %d", len(got))
	}

	// overwrite
	err = s.SetPermissions(id, []Permission{{ResType: "group", ResID: "2"}})
	if err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetPermissions(id)
	if len(got) != 1 || got[0].ResID != "2" {
		t.Fatalf("overwrite failed: %+v", got)
	}
}

func TestUserStore_Delete(t *testing.T) {
	db := initTestDB(t)
	defer db.Close()
	s := NewUserStore(db)
	id, _ := s.Create("u1", "h", "", "viewer")
	s.SetPermissions(id, []Permission{{ResType: "group", ResID: "1"}})

	err := s.Delete(id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.GetByID(id)
	if err == nil {
		t.Fatal("expected error after delete")
	}
	perms, _ := s.GetPermissions(id)
	if len(perms) != 0 {
		t.Fatalf("expected 0 permissions after cascade delete, got %d", len(perms))
	}
}

func TestUserStore_PasswordOps(t *testing.T) {
	db := initTestDB(t)
	defer db.Close()
	s := NewUserStore(db)
	id, _ := s.Create("u1", "oldhash", "", "viewer")

	// UpdatePassword clears must_change_pwd
	s.UpdatePassword(id, "newhash")
	u, _ := s.GetByID(id)
	if u.PasswordHash != "newhash" || u.MustChangePwd {
		t.Fatalf("UpdatePassword failed: hash=%s must_change=%v", u.PasswordHash, u.MustChangePwd)
	}

	// ResetPassword sets must_change_pwd
	s.ResetPassword(id, "resethash")
	u, _ = s.GetByID(id)
	if u.PasswordHash != "resethash" || !u.MustChangePwd {
		t.Fatalf("ResetPassword failed: hash=%s must_change=%v", u.PasswordHash, u.MustChangePwd)
	}
}

func TestUserStore_HasAnyUser(t *testing.T) {
	db := initTestDB(t)
	defer db.Close()
	s := NewUserStore(db)

	has, _ := s.HasAnyUser()
	if has {
		t.Fatal("expected no users")
	}

	s.Create("u1", "h", "", "viewer")
	has, _ = s.HasAnyUser()
	if !has {
		t.Fatal("expected has users")
	}
}
