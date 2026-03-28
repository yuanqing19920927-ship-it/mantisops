# 多用户权限管理 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 MantisOps 添加多用户账号体系（admin/operator/viewer 三级角色 + 资源级权限控制），支持 10-30 人团队按服务器分组/单资源维度配置可见范围。

**Architecture:** 后端新增 users/user_permissions 表到现有 SQLite，重写 auth.go 为查库 + bcrypt 校验，新增 TokenVersionCache 和 PermissionCache 做内存缓存实现即时踢下线和资源过滤。Hub 改造为按连接权限过滤推送。前端新增用户管理页、权限配置页、强制改密页，authStore 扩展 role/mustChangePwd，Sidebar 和各页面按角色显隐操作按钮。

**Tech Stack:** Go (Gin + SQLite + bcrypt), React 19 + TypeScript + Zustand

**Spec:** `docs/superpowers/specs/2026-03-29-multi-user-rbac-design.md`

---

## File Map

### 新建文件

| 文件 | 职责 |
|------|------|
| `server/internal/store/user_store.go` | users 表 + user_permissions 表 CRUD，admin 计数查询 |
| `server/internal/store/user_store_test.go` | UserStore 单元测试 |
| `server/internal/api/user_handler.go` | 用户管理 API（CRUD + 权限 + 重置密码） |
| `server/internal/api/permission.go` | PermissionCache + PermissionSet + RequireRole/RequireResource 中间件 |
| `server/internal/api/permission_test.go` | 权限中间件单元测试 |
| `web/src/pages/Users/index.tsx` | 用户管理页 |
| `web/src/pages/Users/PermissionTree.tsx` | 权限配置树形组件 |
| `web/src/pages/ChangePassword/index.tsx` | 强制改密页 |
| `web/src/api/users.ts` | 用户管理 API 客户端 |

### 修改文件

| 文件 | 改动 |
|------|------|
| `server/internal/store/sqlite.go` | migrateV2：新增 users + user_permissions 建表 |
| `server/internal/api/auth.go` | 重写：查库 bcrypt 校验，JWT payload 扩展（user_id/role/token_version/must_change_pwd），TokenVersionCache，强制改密拦截，改密接口 |
| `server/internal/api/router.go` | RouterDeps 新增字段，路由分组挂载 RequireRole，注册 /users/* 路由 |
| `server/internal/ws/hub.go` | client 扩展 user_id/role/PermissionSet，BroadcastMetrics/BroadcastAlert/BroadcastLog 替代 BroadcastJSON，连接生命周期管理 |
| `server/cmd/server/main.go` | 初始化 UserStore/PermissionCache，旧账号迁移逻辑，AuthHandler 改为依赖 UserStore |
| `server/internal/api/log_handler.go` | ListRuntime/Export/Sources/Stats 注入 source 过滤 |
| `server/internal/logging/middleware.go` | auditRoutes 新增用户管理路由 |
| `web/src/stores/authStore.ts` | 扩展 role/displayName/mustChangePwd，login 响应解析 |
| `web/src/App.tsx` | 新增 /users、/users/:id/permissions、/change-password 路由，RequireChangePwd 守卫 |
| `web/src/components/Layout/Sidebar.tsx` | 按角色显隐菜单项，新增用户管理 |
| `web/src/components/Layout/MainLayout.tsx` | 用户菜单增加"修改密码"入口 |

---

## Chunk 1: 数据库 + UserStore + 迁移

### Task 1: 数据库 Schema 迁移

**Files:**
- Modify: `server/internal/store/sqlite.go`

- [ ] **Step 1: 在 migrate() 的建表语句列表末尾追加 users 和 user_permissions 表**

在 `settings` 建表语句之后，`}` 闭合之前追加：

```go
// Users (multi-user RBAC)
`CREATE TABLE IF NOT EXISTS users (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    username        TEXT UNIQUE NOT NULL,
    password_hash   TEXT NOT NULL,
    display_name    TEXT DEFAULT '',
    role            TEXT NOT NULL DEFAULT 'viewer',
    enabled         BOOLEAN DEFAULT 1,
    must_change_pwd BOOLEAN DEFAULT 0,
    token_version   INTEGER DEFAULT 1,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
)`,

// User resource permissions
`CREATE TABLE IF NOT EXISTS user_permissions (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    res_type TEXT NOT NULL,
    res_id   TEXT NOT NULL
)`,
`CREATE UNIQUE INDEX IF NOT EXISTS idx_user_perm ON user_permissions(user_id, res_type, res_id)`,
```

- [ ] **Step 2: 编译验证**

Run: `cd server && go build ./...`
Expected: 编译成功

- [ ] **Step 3: 提交**

```bash
git add server/internal/store/sqlite.go
git commit -m "feat(auth): add users and user_permissions tables"
```

---

### Task 2: UserStore CRUD

**Files:**
- Create: `server/internal/store/user_store.go`
- Create: `server/internal/store/user_store_test.go`

- [ ] **Step 1: 编写 user_store_test.go**

```go
package store

import (
	"testing"
)

// initTestDB creates a temp SQLite with full schema for testing.
// Reuses InitSQLite which runs all migrations (matching existing test pattern in sqlite_test.go).
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
	// Create() sets must_change_pwd = 1 (admin-created users must change password)
	if !u.Enabled || !u.MustChangePwd {
		t.Fatalf("unexpected flags: enabled=%v must_change=%v (expected true, true)", u.Enabled, u.MustChangePwd)
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
	// CreateInitialAdmin sets must_change_pwd = 0 (migrated admin, no forced change)
	if u.MustChangePwd {
		t.Fatalf("initial admin should not require password change")
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

	// 全量覆盖
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
	// CASCADE: permissions should be gone too
	perms, _ := s.GetPermissions(id)
	if len(perms) != 0 {
		t.Fatalf("expected 0 permissions after cascade delete, got %d", len(perms))
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd server && go test ./internal/store/ -run TestUserStore -v`
Expected: 编译错误（NewUserStore 等未定义）

- [ ] **Step 3: 实现 user_store.go**

```go
package store

import (
	"database/sql"
	"fmt"
	"time"
)

type User struct {
	ID            int64     `json:"id"`
	Username      string    `json:"username"`
	PasswordHash  string    `json:"-"`
	DisplayName   string    `json:"display_name"`
	Role          string    `json:"role"`
	Enabled       bool      `json:"enabled"`
	MustChangePwd bool      `json:"must_change_pwd"`
	TokenVersion  int64     `json:"-"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Permission struct {
	ResType string `json:"res_type"`
	ResID   string `json:"res_id"`
}

type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) Create(username, passwordHash, displayName, role string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO users (username, password_hash, display_name, role, must_change_pwd) VALUES (?, ?, ?, ?, 1)`,
		username, passwordHash, displayName, role,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CreateInitialAdmin creates the migrated admin without must_change_pwd.
func (s *UserStore) CreateInitialAdmin(username, passwordHash string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO users (username, password_hash, role, must_change_pwd) VALUES (?, ?, 'admin', 0)`,
		username, passwordHash,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *UserStore) GetByID(id int64) (*User, error) {
	var u User
	err := s.db.QueryRow(
		`SELECT id, username, password_hash, display_name, role, enabled, must_change_pwd, token_version, created_at, updated_at FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Role, &u.Enabled, &u.MustChangePwd, &u.TokenVersion, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *UserStore) GetByUsername(username string) (*User, error) {
	var u User
	err := s.db.QueryRow(
		`SELECT id, username, password_hash, display_name, role, enabled, must_change_pwd, token_version, created_at, updated_at FROM users WHERE username = ?`, username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Role, &u.Enabled, &u.MustChangePwd, &u.TokenVersion, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *UserStore) List() ([]User, error) {
	rows, err := s.db.Query(
		`SELECT id, username, password_hash, display_name, role, enabled, must_change_pwd, token_version, created_at, updated_at FROM users ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Role, &u.Enabled, &u.MustChangePwd, &u.TokenVersion, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (s *UserStore) Update(id int64, displayName, role string, enabled bool) error {
	_, err := s.db.Exec(
		`UPDATE users SET display_name = ?, role = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		displayName, role, enabled, id,
	)
	return err
}

func (s *UserStore) UpdatePassword(id int64, passwordHash string) error {
	_, err := s.db.Exec(
		`UPDATE users SET password_hash = ?, must_change_pwd = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		passwordHash, id,
	)
	return err
}

func (s *UserStore) ResetPassword(id int64, passwordHash string) error {
	_, err := s.db.Exec(
		`UPDATE users SET password_hash = ?, must_change_pwd = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		passwordHash, id,
	)
	return err
}

func (s *UserStore) IncrementTokenVersion(id int64) error {
	_, err := s.db.Exec(`UPDATE users SET token_version = token_version + 1 WHERE id = ?`, id)
	return err
}

func (s *UserStore) GetTokenVersion(id int64) (int64, error) {
	var v int64
	err := s.db.QueryRow(`SELECT token_version FROM users WHERE id = ?`, id).Scan(&v)
	return v, err
}

func (s *UserStore) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

func (s *UserStore) CountEnabledAdmins() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin' AND enabled = 1`).Scan(&count)
	return count, err
}

func (s *UserStore) HasAnyUser() (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	return count > 0, err
}

// --- Permissions ---

func (s *UserStore) SetPermissions(userID int64, perms []Permission) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM user_permissions WHERE user_id = ?`, userID); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO user_permissions (user_id, res_type, res_id) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, p := range perms {
		if _, err := stmt.Exec(userID, p.ResType, p.ResID); err != nil {
			return fmt.Errorf("insert perm %s/%s: %w", p.ResType, p.ResID, err)
		}
	}
	return tx.Commit()
}

func (s *UserStore) GetPermissions(userID int64) ([]Permission, error) {
	rows, err := s.db.Query(`SELECT res_type, res_id FROM user_permissions WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var perms []Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ResType, &p.ResID); err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	return perms, nil
}

func (s *UserStore) AddPermission(userID int64, resType, resID string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO user_permissions (user_id, res_type, res_id) VALUES (?, ?, ?)`,
		userID, resType, resID,
	)
	return err
}

func (s *UserStore) RemovePermission(userID int64, resType, resID string) error {
	_, err := s.db.Exec(
		`DELETE FROM user_permissions WHERE user_id = ? AND res_type = ? AND res_id = ?`,
		userID, resType, resID,
	)
	return err
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd server && go test ./internal/store/ -run TestUserStore -v`
Expected: 全部 PASS

- [ ] **Step 5: 编译全项目**

Run: `cd server && go build ./...`
Expected: 编译成功

- [ ] **Step 6: 提交**

```bash
git add server/internal/store/user_store.go server/internal/store/user_store_test.go
git commit -m "feat(auth): add UserStore with CRUD and permissions"
```

---

## Chunk 2: 认证重写 + 鉴权中间件

### Task 3: 重写 auth.go

**Files:**
- Modify: `server/internal/api/auth.go`

重写整个文件。关键变化：
- `AuthHandler` 依赖 `*store.UserStore` 而非硬编码 username/password
- `jwtPayload` 扩展 UserID/Role/TokenVersion/MustChangePwd
- `JWTMiddleware` 增加 token_version 校验 + 强制改密拦截
- 新增 `ChangePassword` handler
- 新增 `TokenVersionCache`

- [ ] **Step 1: 重写 auth.go**

完整代码（保留原有 JWT 签名算法，扩展 payload）：

TokenVersionCache 提取为独立顶层对象（被 AuthHandler 和 UserHandler 共享）：
```go
type TokenVersionCache struct {
	mu    sync.RWMutex
	cache map[int64]int64
}

func NewTokenVersionCache() *TokenVersionCache {
	return &TokenVersionCache{cache: make(map[int64]int64)}
}

func (c *TokenVersionCache) Get(userID int64) (int64, bool) { ... }
func (c *TokenVersionCache) Set(userID int64, version int64) { ... }
func (c *TokenVersionCache) Invalidate(userID int64) { ... }
```

AuthHandler 结构体改为：
```go
type AuthHandler struct {
	userStore *store.UserStore
	jwtSecret []byte
	tvCache   *TokenVersionCache  // 共享引用，非自己创建
}
```

jwtPayload 扩展为：
```go
type jwtPayload struct {
	UserID        int64  `json:"user_id"`
	Username      string `json:"username"`
	Role          string `json:"role"`
	TokenVersion  int64  `json:"token_version"`
	MustChangePwd bool   `json:"must_change_pwd"`
	Exp           int64  `json:"exp"`
}
```

Login handler 改为：
- 查 `userStore.GetByUsername`
- `bcrypt.CompareHashAndPassword` 校验
- 检查 `enabled`
- 生成包含新字段的 JWT
- 响应包含 role、display_name、must_change_pwd

JWTMiddleware 改为：
- 解析 JWT 取 payload
- 从 tvCache 获取 token_version（miss 则查库）
- `jwt.token_version < db.token_version` → 401
- `must_change_pwd == true` 且路径不是 `/auth/me` 或 `/auth/password` → 403 `must_change_password`
- 设置 context: username, user_id, role

新增 ChangePassword handler：
- 校验 old_password（bcrypt 比对）
- bcrypt new_password，写入 UserStore
- IncrementTokenVersion + 清 tvCache
- 签发新 token（must_change_pwd=false，新 token_version）
- 返回新 token + 用户信息

新增 `NewAuthHandler(userStore *store.UserStore, jwtSecret string, tvCache *TokenVersionCache)` 构造函数（三参数，tvCache 由 main.go 注入）。

- [ ] **Step 2: 编译验证**

Run: `cd server && go build ./...`
Expected: 可能有 main.go 中 NewAuthHandler 签名变化导致的编译错误，暂时忽略（Task 5 修复）

- [ ] **Step 3: 提交**

```bash
git add server/internal/api/auth.go
git commit -m "feat(auth): rewrite auth with bcrypt, token_version, must_change_pwd"
```

---

### Task 4: RequireRole 和 PermissionCache 中间件

**Files:**
- Create: `server/internal/api/permission.go`
- Create: `server/internal/api/permission_test.go`

- [ ] **Step 1: 实现 permission.go**

```go
package api

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"mantisops/server/internal/store"
)

// --- Role hierarchy ---

var roleLevel = map[string]int{
	"viewer":   1,
	"operator": 2,
	"admin":    3,
}

func RequireRole(minRole string) gin.HandlerFunc {
	minLevel := roleLevel[minRole]
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		roleStr, _ := role.(string)
		if roleLevel[roleStr] < minLevel {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// --- PermissionSet ---

type PermissionSet struct {
	Groups    map[string]bool
	Servers   map[string]bool // direct + expanded from groups
	Databases map[string]bool
	Probes    map[string]bool
}

type PermissionCache struct {
	mu        sync.RWMutex
	cache     map[int64]*PermissionSet
	userStore *store.UserStore
	groupStore *store.GroupStore
	serverStore *store.ServerStore
}

func NewPermissionCache(us *store.UserStore, gs *store.GroupStore, ss *store.ServerStore) *PermissionCache {
	return &PermissionCache{
		cache:       make(map[int64]*PermissionSet),
		userStore:   us,
		groupStore:  gs,
		serverStore: ss,
	}
}

func (pc *PermissionCache) Get(userID int64) (*PermissionSet, error) {
	pc.mu.RLock()
	if ps, ok := pc.cache[userID]; ok {
		pc.mu.RUnlock()
		return ps, nil
	}
	pc.mu.RUnlock()

	// Build from DB
	perms, err := pc.userStore.GetPermissions(userID)
	if err != nil {
		return nil, err
	}

	ps := &PermissionSet{
		Groups:    make(map[string]bool),
		Servers:   make(map[string]bool),
		Databases: make(map[string]bool),
		Probes:    make(map[string]bool),
	}

	for _, p := range perms {
		switch p.ResType {
		case "group":
			ps.Groups[p.ResID] = true
		case "server":
			ps.Servers[p.ResID] = true
		case "database":
			ps.Databases[p.ResID] = true
		case "probe":
			ps.Probes[p.ResID] = true
		}
	}

	// Expand groups → servers
	if len(ps.Groups) > 0 {
		servers, _ := pc.serverStore.List()
		for _, srv := range servers {
			if srv.GroupID != nil {
				gidStr := fmt.Sprintf("%d", *srv.GroupID)
				if ps.Groups[gidStr] {
					ps.Servers[srv.HostID] = true
				}
			}
		}
	}

	pc.mu.Lock()
	pc.cache[userID] = ps
	pc.mu.Unlock()
	return ps, nil
}

func (pc *PermissionCache) Invalidate(userID int64) {
	pc.mu.Lock()
	delete(pc.cache, userID)
	pc.mu.Unlock()
}

func (pc *PermissionCache) InvalidateAll() {
	pc.mu.Lock()
	pc.cache = make(map[int64]*PermissionSet)
	pc.mu.Unlock()
}
```

**分组变更联动**：以下操作会改变 group→server 映射，导致已缓存的 PermissionSet 中展开的 Servers 集合过期：
- `PUT /servers/:id/group`（服务器移入/移出分组）
- `DELETE /groups/:id`（删除分组，组内服务器解绑）
- 新服务器注册并被自动分组

这些操作发生时，必须：
1. `permCache.InvalidateAll()` — 清除所有用户的缓存，下次请求重新从 DB 构建
2. Hub 中所有非 admin 连接的 PermissionSet 也需要热更新 — 通过 `hub.RefreshAllPermissions(permCache)` 方法：遍历所有连接，对非 admin 连接重新从 permCache.Get() 获取新 PermissionSet 并替换

实现点：
- `GroupHandler.SetServerGroup`、`GroupHandler.Delete` 方法末尾追加 `permCache.InvalidateAll()` + `hub.RefreshAllPermissions(permCache)`
- gRPC handler 中新服务器注册后，如果有自动分组逻辑，也需要触发（当前没有自动分组，暂不需要）

- [ ] **Step 2: 编译验证**

Run: `cd server && go build ./internal/api/`
Expected: 可能缺少 fmt import 或 ServerStore.List 签名不匹配——根据实际 serverStore 接口调整

- [ ] **Step 3: 提交**

```bash
git add server/internal/api/permission.go
git commit -m "feat(auth): add RequireRole middleware and PermissionCache"
```

---

### Task 5: main.go 初始化 + 旧账号迁移 + router.go 路由分组

**Files:**
- Modify: `server/cmd/server/main.go`
- Modify: `server/internal/api/router.go`

- [ ] **Step 1: main.go 改造**

关键变更：
- 创建 `userStore := store.NewUserStore(db)`
- 旧账号迁移逻辑：`userStore.HasAnyUser()` → false 时用 `cfg.Auth.Username/Password` + bcrypt 创建初始 admin
- `permCache := api.NewPermissionCache(userStore, groupStore, serverStore)`
- `authHandler := api.NewAuthHandler(userStore, cfg.Auth.JWTSecret, tvCache)` （三参数，与 Task 3 定义一致）
- RouterDeps 新增 `UserStore`、`PermissionCache`

- [ ] **Step 2: router.go 改造**

RouterDeps 新增字段：
```go
UserHandler      *UserHandler
PermissionCache  *PermissionCache
```

路由分组改造：
```go
// viewer 级（默认，所有 GET 查询 — 受资源级过滤保护）
v1.GET("/dashboard", ...)
v1.GET("/servers", ...)
v1.GET("/servers/:id", ...)
v1.GET("/probes", ...)
v1.GET("/probes/status", ...)
v1.GET("/assets", ...)
v1.GET("/databases", ...)
v1.GET("/databases/:id", ...)
v1.GET("/billing", ...)
v1.GET("/alerts/rules", ...)
v1.GET("/alerts/events", ...)
v1.GET("/alerts/stats", ...)
v1.GET("/alerts/events/:id/notifications", ...) // 资源级过滤见 Task 8
// 注意：GET /alerts/channels 含敏感信息（Webhook URL），放入 operator 组

// operator 级
op := v1.Group("")
op.Use(RequireRole("operator"))
{
    // 敏感 GET — 从 viewer 路径显式摘出
    op.GET("/alerts/channels", ...)

    op.POST("/probes", ...)
    op.PUT("/probes/:id", ...)
    op.DELETE("/probes/:id", ...)
    // ... 告警规则、资产、通知渠道 CRUD
    op.PUT("/alerts/events/:id/ack", ...)
}

// admin 级
adm := v1.Group("")
adm.Use(RequireRole("admin"))
{
    // 用户管理
    adm.POST("/users", ...)
    adm.GET("/users", ...)
    // ... 接入管理、分组管理、平台配置
}
```

- [ ] **Step 3: 编译全项目**

Run: `cd server && go build ./...`
Expected: 编译成功

- [ ] **Step 4: 提交**

```bash
git add server/cmd/server/main.go server/internal/api/router.go
git commit -m "feat(auth): wire up UserStore, migration, and role-based route groups"
```

---

## Chunk 3: 用户管理 API

### Task 6: UserHandler 实现

**Files:**
- Create: `server/internal/api/user_handler.go`

实现以下 handler 方法：

- `List` — GET /users，返回用户列表（不含 password_hash）
- `Get` — GET /users/:id
- `Create` — POST /users，bcrypt 密码，must_change_pwd=1
- `Update` — PUT /users/:id，角色/显示名/启用禁用 + 系统不变量校验 + token_version 递增 + Hub 断连/更新
- `Delete` — DELETE /users/:id，系统不变量校验 + Hub 断连
- `ResetPassword` — PUT /users/:id/reset-pwd，bcrypt 新密码 + must_change_pwd=1 + token_version 递增 + Hub 断连
- `GetPermissions` — GET /users/:id/permissions
- `SetPermissions` — PUT /users/:id/permissions，全量覆盖 + 去重 + PermissionCache 失效 + Hub 更新 PermissionSet

每个写操作都需要：
1. 系统不变量校验（§3.7）
2. token_version 递增（如角色/启用变更）
3. tvCache 失效
4. permCache 失效（如权限/角色变更）
5. Hub 连接管理（断连或热更新）

**tvCache 共享机制**：`TokenVersionCache` 必须从 `AuthHandler` 中提取为独立的顶层对象，在 `main.go` 中创建后同时注入 `AuthHandler` 和 `UserHandler`。不能把 tvCache 藏在 AuthHandler 内部，否则 UserHandler 无法清缓存。

```go
// main.go 中：
tvCache := api.NewTokenVersionCache()
authHandler := api.NewAuthHandler(userStore, cfg.Auth.JWTSecret, tvCache)
userHandler := api.NewUserHandler(userStore, tvCache, permCache, hub)
```

`UserHandler` 构造函数：
```go
type UserHandler struct {
    userStore  *store.UserStore
    tvCache    *TokenVersionCache
    permCache  *PermissionCache
    hub        *ws.Hub
}
```

- [ ] **Step 1: 实现 user_handler.go**

（完整代码，含所有 handler 方法、系统不变量校验、tvCache/permCache 失效调用、Hub 断连/热更新调用）

- [ ] **Step 2: 在 router.go 注册路由**

admin 路由组中注册所有 /users/* 端点。

- [ ] **Step 3: 在 logging/middleware.go 的 auditRoutes 中追加用户管理路由**

现有 `auditRoute` 结构体字段为 `{Method, PathPrefix, Action, ResourceType}`，中间件用 `PathPrefix` 做前缀匹配。注意匹配顺序：**更具体的路径必须排在前面**，否则 `/users/3/reset-pwd` 会被 `/api/v1/users/` 先匹配到。

```go
// 用户管理 — 更具体的路径在前
{"PUT", "/api/v1/users/", "reset_pwd", "user"},       // 匹配 /users/:id/reset-pwd（需在 middleware 内补充 path 包含 "reset-pwd" 的二次判断）
{"PUT", "/api/v1/users/", "set_permissions", "user"},  // 匹配 /users/:id/permissions（需 path 包含 "permissions" 的二次判断）
{"POST", "/api/v1/users", "create", "user"},
{"DELETE", "/api/v1/users/", "delete", "user"},
```

由于现有中间件只做前缀匹配，无法区分 `/users/:id`（update）、`/users/:id/reset-pwd`、`/users/:id/permissions` 三种 PUT 操作。需要在 `auditRoute` 结构体中新增可选的 `PathSuffix string` 字段，或者在中间件匹配逻辑中增加后缀检查：

```go
type auditRoute struct {
    Method       string
    PathPrefix   string
    PathSuffix   string // 新增，可选，用于区分同前缀不同后缀的路由
    Action       string
    ResourceType string
}
```

匹配逻辑改为：
```go
if method != r.Method { continue }
if !(r.PathPrefix == path || strings.HasPrefix(path, r.PathPrefix)) { continue }
if r.PathSuffix != "" && !strings.HasSuffix(path, r.PathSuffix) { continue }
```

新增路由条目（按匹配优先级排列）：
```go
{"PUT", "/api/v1/users/", "/reset-pwd", "reset_pwd", "user"},
{"PUT", "/api/v1/users/", "/permissions", "set_permissions", "user_permission"},
{"POST", "/api/v1/users", "", "create", "user"},
{"PUT", "/api/v1/users/", "", "update", "user"},
{"DELETE", "/api/v1/users/", "", "delete", "user"},
{"PUT", "/api/v1/auth/password", "", "change_password", "auth"},
```

现有的其他 auditRoute 条目 PathSuffix 字段留空字符串即可，不影响现有行为。

- [ ] **Step 4: 编译全项目**

Run: `cd server && go build ./...`
Expected: 编译成功

- [ ] **Step 5: 提交**

```bash
git add server/internal/api/user_handler.go server/internal/api/router.go server/internal/logging/middleware.go
git commit -m "feat(auth): add user management API handlers"
```

---

## Chunk 4: 资源过滤 + WebSocket Hub 改造

### Task 7: Hub 改造

**Files:**
- Modify: `server/internal/ws/hub.go`

关键变更：
- `client` 结构体扩展：`userID int64`、`role string`、`permSet *PermissionSet`
- 新增 `userConns map[int64][]*client` 索引
- 新增 `alertTargets map[int]alertTarget` — 记录 eventID → {targetType, targetID} 映射，用于 resolved/acked 过滤
- `HandleWS` 接收 user_id/role/PermissionSet 参数
- 新增 `BroadcastMetrics(hostID string, msg interface{})` — 遍历连接，admin(permSet==nil)放行，否则检查 Servers[hostID]
- 新增 `BroadcastAlertFiring(event model.AlertEvent, msg interface{})` — 从 event 取 targetID/type，记录到 alertTargets 映射，按 Servers/Probes 过滤
- 新增 `BroadcastAlertResolved(eventID int, msg interface{})` — 从 alertTargets 查 targetInfo 做过滤，过滤后清理映射条目
- 新增 `BroadcastAlertAcked(eventID int, msg interface{})` — 从 alertTargets 查 targetInfo 做过滤
- 新增 `BroadcastLog(source string, msg interface{})` — admin 放行，否则检查 source 前缀 "agent:{hostID}" 匹配 Servers
- 新增 `BroadcastAuditLog(msg interface{})` — 仅推送给 admin 连接（role=="admin"）
- 修改 `BroadcastLogJSON` 增加权限过滤
- 新增 `BroadcastAdmin(msg interface{})` — 仅推送给 admin 连接，用于 deploy_progress、cloud_sync 等管理事件
- 新增 `DisconnectUser(userID int64)` — 发送 close frame 并从 clients 移除
- 新增 `UpdateUserPermissions(userID int64, permSet *PermissionSet)` — 热更新

`alertTarget` 结构：
```go
type alertTarget struct {
    ruleType string // server_offline/cpu/memory/.../probe_down/container
    targetID string // host_id 或 probe ID
}
```
alertTargets 在 BroadcastAlertFiring 时写入，BroadcastAlertResolved 时读取+删除。内存占用极小（只有 firing 中的事件）。

**重启后恢复**：Hub 启动时需要从 DB 预加载所有 `status='firing'` 的事件到 alertTargets 映射。在 main.go 中，Hub 创建后立即调用：
```go
// main.go 中 Hub 创建后：
firingEvents, _ := alertStore.ListFiringEvents() // 新增方法，返回所有 firing 事件
hub.LoadAlertTargets(firingEvents)                // 预加载映射
```

`AlertStore.ListFiringEvents()` 新增方法：
```go
func (s *AlertStore) ListFiringEvents() ([]model.AlertEvent, error) {
    // SELECT id, target_id, rule_id FROM alert_events WHERE status='firing'
    // 还需要 JOIN alert_rules 获取 rule type
}
```

`Hub.LoadAlertTargets(events)` 遍历事件，通过 rule_id 查 rule type（可从 events 中关联的 rule_name 推断，或直接传入 rule type），填充 alertTargets 映射。

为避免 Hub 依赖 AlertStore，在 main.go 中查完后传 `[]struct{EventID int; RuleType, TargetID string}` 给 Hub。

- [ ] **Step 1: 重写 hub.go**

（保留现有日志订阅逻辑，扩展连接管理和过滤广播）

- [ ] **Step 2: 更新所有 BroadcastJSON 调用方**

搜索项目中所有 `hub.BroadcastJSON` 调用，替换为对应的 BroadcastMetrics/BroadcastAlert/BroadcastLog。

涉及文件和具体替换：

| 文件 | 原调用 | 替换为 | 说明 |
|------|--------|--------|------|
| `collector/metrics.go:93` | `BroadcastJSON({type:metrics, host_id, data})` | `BroadcastMetrics(hostID, msg)` | 按连接可见服务器过滤 |
| `collector/aliyun.go:722,876` | `BroadcastJSON({type:metrics, host_id, data})` | `BroadcastMetrics(hostID, msg)` | 阿里云 ECS/RDS 指标同理 |
| `alert/alerter.go:182` | `BroadcastJSON({type:alert, data:event})` | `BroadcastAlertFiring(event, msg)` | 传完整 event 对象，Hub 提取 target 信息并记录映射 |
| `alert/alerter.go:197,276,417` | `BroadcastJSON({type:alert_resolved, data:{id:N}})` | `BroadcastAlertResolved(eventID, msg)` | Hub 从 alertTargets 映射查目标做过滤 |
| `alert/alerter.go:399` | `BroadcastJSON({type:alert_acked, data:{id,acked_by}})` | `BroadcastAlertAcked(eventID, msg)` | 同上 |
| `logging/manager.go:277` | `BroadcastJSON({type:audit_log, data:line})` | `BroadcastAuditLog(msg)` | **仅推给 admin 连接** |
| `probe/prober.go:131` | `BroadcastJSON({type:metrics, host_id, data})` | `BroadcastMetrics(hostID, msg)` | 探测结果按服务器过滤 |
| `deployer/deployer.go:498` | `BroadcastJSON({type:deploy_progress})` | `BroadcastAdmin(msg)` | 部署进度仅 admin 可见 |
| `cloud/manager.go:184` | `BroadcastJSON({type:cloud_sync_progress})` | `BroadcastAdmin(msg)` | 云同步进度仅 admin 可见 |

注意：`BroadcastJSON` 方法完全废弃删除，确保不遗漏调用点。

- [ ] **Step 3: 更新 router.go 中 HandleWS 调用**

WS 连接建立时传入 user_id/role/PermissionSet：
```go
r.GET("/ws", func(c *gin.Context) {
    token := c.Query("token")
    payload, err := deps.AuthHandler.ValidateToken(token)
    // ... 校验 token_version ...
    var permSet *PermissionSet
    if payload.Role != "admin" {
        permSet, _ = deps.PermissionCache.Get(payload.UserID)
    }
    deps.Hub.HandleWSWithAuth(c.Writer, c.Request, payload.UserID, payload.Role, permSet)
})
```

- [ ] **Step 4: 编译全项目**

Run: `cd server && go build ./...`
Expected: 编译成功

- [ ] **Step 5: 提交**

```bash
git add server/internal/ws/hub.go server/internal/collector/metrics_collector.go server/internal/alert/alerter.go server/internal/logging/manager.go server/internal/api/router.go
git commit -m "feat(auth): Hub per-connection permission filtering + typed broadcast methods"
```

---

### Task 8: REST API 资源过滤

**Files:**
- Modify: 多个 handler 文件

为非 admin 用户注入 PermissionSet，过滤列表返回。

策略：在各 handler 的 List 方法中，从 context 取 role 和 user_id，admin 不过滤，其他角色从 PermissionCache 获取 PermissionSet 后过滤结果。

涉及的 handler 和过滤逻辑：

| Handler | 方法 | 过滤依据 |
|---------|------|---------|
| DashboardHandler | Overview | Servers 过滤服务器列表和统计 |
| ServerHandler | List, Get | Servers |
| ProbeHandler | List, Status | Probes |
| AssetHandler | List | Servers（asset.server_id → server.host_id） |
| DatabaseHandler | List, Get | Databases |
| AlertHandler | ListEvents, ListRules | 按 target_id 映射到 Servers/Probes（查后过滤） |
| AlertHandler | GetEventNotifications | 先查事件的 target_id，检查用户是否有权访问该目标，无权返回 403。有权则正常返回通知投递详情 |
| AlertHandler | **GetStats** | **必须 SQL 层过滤**（见下方） |
| BillingHandler | List | 见下方 Billing 映射说明 |
| LogHandler | ListRuntime | source 过滤（查后过滤，非 admin 排除 source=server） |
| LogHandler | **Export** | **type=audit 需 admin 角色检查**（在 handler 开头拦截）；type=runtime 按 source 过滤 |
| LogHandler | **ListAudit** | **已在 router 中放入 admin 路由组**，但还需在 handler 中二次确认（防止路由变更遗漏） |
| LogHandler | **Sources, Stats** | **必须 SQL 层过滤**（见下方） |

**Audit REST 收口**：
- `GET /logs/audit` — 在 router.go 中放入 admin 路由组（RequireRole("admin")）
- `GET /logs/export` — handler 中检查 `type` 参数：`type=audit` 时检查 `role != "admin"` 直接 403 拒绝。不能仅靠路由组，因为 export 端点同时服务 audit 和 runtime 两种类型
- `GET /logs/sources` 和 `GET /logs/stats` — 非 admin 时返回的结果自动排除 `source=server` 和审计相关数据

**Billing 资源映射**：
当前 `BillingItem` 只有云侧 ID（`BillingItem.ID` 是阿里云 instance_id），没有 host_id。需要通过 `cloud_instances` 表做映射：
- `cloud_instances` 表中 `instance_id → host_id` 提供映射关系
- BillingHandler.List 方法中，先从 `cloudStore` 加载 `instance_id → host_id` 映射表
- 过滤逻辑：对每个 BillingItem，通过 `instance_id` 查到 `host_id`，然后：
  - `type=ecs` → 检查 `ps.CanSeeServer(host_id)`
  - `type=rds` → 检查 `ps.CanSeeDatabase(host_id)`
  - `type=ssl` → SSL 证书不关联特定资源，**仅 admin 可见**（SSL 证书包含域名等敏感信息）
- 需要在 `CloudStore` 新增 `GetInstanceHostIDMap() map[string]string` 方法（instance_id → host_id），或在 BillingHandler 的 refreshLoop 中预构建映射

实现方式分两类：

**1. 列表接口（查后过滤）：**
写一个 helper 函数 `getPermissionSet(c *gin.Context, pc *PermissionCache) *PermissionSet`，返回 nil 表示 admin（不过滤），非 nil 表示需要过滤。各 handler 在查询结果后调用 `ps.CanSeeXxx()` 方法过滤。

**2. 聚合接口（SQL 层过滤）：**
聚合查询不能查完再过滤，否则统计数字错误。需要在 SQL WHERE 中注入 target_id IN (?) 或 source IN (?) 条件。

- `AlertStore.GetStats()` → 新增 `GetStatsFiltered(targetIDs []string)` 方法，WHERE 条件加 `target_id IN (?,?,...)`。targetIDs 由 PermissionSet 的 Servers + Probes 集合展开为列表传入。admin 调用原无过滤版本。
- `LogStore.GetStats()` → 新增 `GetStatsFiltered(sources []string)` 方法，WHERE 条件加 `source IN (?,?,...)`。sources 由 PermissionSet 的 Servers 展开为 `["agent:srv-69-ai", "agent:srv-71-opsboard", ...]` 传入。admin 调用原无过滤版本。
- `LogStore.GetSources()` → 同理新增 `GetSourcesFiltered(sources []string)`。

- [ ] **Step 1: 在 permission.go 中添加 PermissionSet 的 filter helper 方法**

```go
func (ps *PermissionSet) CanSeeServer(hostID string) bool {
    if ps == nil { return true } // admin
    return ps.Servers[hostID]
}
func (ps *PermissionSet) CanSeeProbe(probeID string) bool { ... }
func (ps *PermissionSet) CanSeeDatabase(hostID string) bool { ... }
func (ps *PermissionSet) CanSeeAlertTarget(targetType, targetID string) bool { ... }
func (ps *PermissionSet) CanSeeLogSource(source string) bool { ... }
```

- [ ] **Step 2: 逐个修改 handler 注入过滤**

每个 handler 的 List 方法开头加：
```go
ps := getPermissionSet(c, deps.PermissionCache)
// 查询后过滤
if ps != nil {
    filtered := make([]Server, 0, len(servers))
    for _, s := range servers {
        if ps.CanSeeServer(s.HostID) {
            filtered = append(filtered, s)
        }
    }
    servers = filtered
}
```

- [ ] **Step 3: 编译全项目**

Run: `cd server && go build ./...`
Expected: 编译成功

- [ ] **Step 4: 提交**

```bash
git add server/internal/api/
git commit -m "feat(auth): resource-level filtering for all list handlers"
```

---

### Task 9: operator 创建 probe 自动归属 + 删除清理

**Files:**
- Modify: `server/internal/api/probe_handler.go`（Create, Delete 方法）
- Modify: `server/internal/store/user_store.go`（新增 RemovePermissionByResource 方法）

注意：**告警规则不需要独立权限记录**。告警规则的可见性通过 `target_id` 映射到 server/probe 维度过滤——规则的 target_id 如果在用户可见的服务器或探测规则范围内，该规则就可见。所以 operator 创建告警规则后，只要规则的 target 在其权限范围内，规则自然可见，无需额外处理。

只有 **probe** 需要自动归属，因为 probe 是独立权限维度。

- [ ] **Step 1: ProbeHandler.Create — 创建后自动添加权限**

在 probe 创建成功后，如果 `role != "admin"`，调用 `userStore.AddPermission(userID, "probe", strconv.Itoa(probeID))`，并清除该用户的 PermissionCache。

- [ ] **Step 2: 删除时清理权限记录**

ProbeHandler.Delete 中，删除成功后调用 `userStore.RemovePermissionByResource("probe", probeID)` 清理所有用户的该 probe 权限记录（避免悬空记录），并调用 `permCache.InvalidateAll()`。

```go
// user_store.go 新增：
func (s *UserStore) RemovePermissionByResource(resType, resID string) error {
    _, err := s.db.Exec(
        `DELETE FROM user_permissions WHERE res_type = ? AND res_id = ?`,
        resType, resID,
    )
    return err
}
```

- [ ] **Step 3: 编译验证 + 提交**

```bash
git add server/internal/api/probe_handler.go server/internal/store/user_store.go
git commit -m "feat(auth): auto-grant probe permissions to operator creator, cleanup on delete"
```

---

## Chunk 5: 前端改造

### Task 10: authStore 扩展 + 路由守卫

**Files:**
- Modify: `web/src/stores/authStore.ts`
- Modify: `web/src/App.tsx`

- [ ] **Step 1: authStore 扩展**

```typescript
interface AuthState {
  token: string | null
  username: string | null
  role: string | null
  displayName: string | null
  mustChangePwd: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => void
  checkAuth: () => Promise<boolean>
  changePassword: (oldPwd: string, newPwd: string) => Promise<void>
}
```

login 解析新响应字段（role, display_name, must_change_pwd）。
changePassword 调用 PUT /auth/password，成功后替换 token 和状态。
localStorage 增加 role、displayName 持久化。

- [ ] **Step 2: App.tsx 新增路由和守卫**

新增 `RequireChangePwd` 组件：检查 `mustChangePwd`，true 时 redirect 到 /change-password。

路由新增：
- `/change-password` — ChangePassword 页面（需登录但不需 RequireChangePwd）
- `/users` — Users 页面（RequireRole admin）
- `/users/:id/permissions` — PermissionTree 页面（RequireRole admin）

- [ ] **Step 3: tsc --noEmit 验证**

Run: `cd web && npx tsc --noEmit`
Expected: 无错误

- [ ] **Step 4: 提交**

```bash
git add web/src/stores/authStore.ts web/src/App.tsx
git commit -m "feat(auth): extend authStore with role/mustChangePwd, add route guards"
```

---

### Task 11: 强制改密页 + 普通改密对话框

**Files:**
- Create: `web/src/pages/ChangePassword/index.tsx`
- Modify: `web/src/components/Layout/MainLayout.tsx`

- [ ] **Step 1: ChangePassword 页面**

居中卡片表单：
- 提示文案："管理员已为您设置初始密码，请输入初始密码后设置新密码"
- 初始密码输入框
- 新密码输入框
- 确认新密码输入框（前端校验一致性）
- 保存按钮 → 调用 authStore.changePassword → 成功跳转 /

- [ ] **Step 2: MainLayout 用户菜单增加"修改密码"**

在用户下拉菜单中，"退出登录"上方增加"修改密码"选项，点击打开对话框（旧密码 + 新密码 + 确认）。

- [ ] **Step 3: tsc 验证 + 提交**

```bash
git add web/src/pages/ChangePassword/ web/src/components/Layout/MainLayout.tsx
git commit -m "feat(auth): add change-password page and menu entry"
```

---

### Task 12: Sidebar 按角色显隐 + 各页面操作按钮

**Files:**
- Modify: `web/src/components/Layout/Sidebar.tsx`
- Modify: 各页面文件

- [ ] **Step 1: Sidebar 改造**

```typescript
const { role } = useAuthStore()
const links = [
  { to: '/', label: '仪表盘', icon: 'dashboard' },
  // ... 基础菜单（所有角色可见）
  ...(role === 'admin' ? [
    { to: '/users', label: '用户管理', icon: 'group' },
  ] : []),
  { to: '/settings', label: '系统信息', icon: 'settings' },
]
```

- [ ] **Step 2: 各页面按角色隐藏操作按钮**

涉及页面和隐藏规则：
- Probes: 创建/删除按钮 → operator+
- Assets: 创建/编辑/删除按钮 → operator+
- Alerts 告警事件 tab: 确认按钮 → operator+
- Alerts 告警规则 tab: 创建/编辑/删除 → operator+
- Alerts 通知渠道 tab: 整个 tab → operator+
- Settings 接入管理区域: → admin only
- Settings 平台配置保存按钮: → admin only
- Servers 分组管理按钮: → admin only

模式：在组件中 `const { role } = useAuthStore()`，条件渲染 `{role !== 'viewer' && <Button>...}`。

- [ ] **Step 3: tsc 验证 + npm run build**

Run: `cd web && npx tsc --noEmit && npm run build`
Expected: 编译成功

- [ ] **Step 4: 提交**

```bash
git add web/src/
git commit -m "feat(auth): role-based menu visibility and action button hiding"
```

---

### Task 13: 用户管理页

**Files:**
- Create: `web/src/pages/Users/index.tsx`
- Create: `web/src/api/users.ts`

- [ ] **Step 1: users.ts API 客户端**

```typescript
import api from './client'

export interface UserInfo {
  id: number; username: string; display_name: string;
  role: string; enabled: boolean; must_change_pwd: boolean;
  created_at: string;
}
export interface PermissionItem { res_type: string; res_id: string }

export const getUsers = () => api.get('/users').then(r => r.data || [])
export const createUser = (body: {username:string; password:string; display_name:string; role:string}) => api.post('/users', body)
export const updateUser = (id:number, body: {display_name:string; role:string; enabled:boolean}) => api.put(`/users/${id}`, body)
export const deleteUser = (id:number) => api.delete(`/users/${id}`)
export const resetPassword = (id:number, body:{password:string}) => api.put(`/users/${id}/reset-pwd`, body)
export const getUserPermissions = (id:number) => api.get(`/users/${id}/permissions`).then(r => r.data)
export const setUserPermissions = (id:number, perms: PermissionItem[]) => api.put(`/users/${id}/permissions`, {permissions: perms})
```

- [ ] **Step 2: Users 页面**

表格：用户名、显示名、角色标签（admin 绿/operator 蓝/viewer 灰）、状态开关、创建时间、操作列（编辑/权限/重置密码/删除）。

顶部"添加用户"按钮 → 对话框。

编辑自己时：角色和状态置灰。系统唯一 admin 时：该行角色和删除置灰。

- [ ] **Step 3: tsc 验证 + 提交**

```bash
git add web/src/pages/Users/ web/src/api/users.ts
git commit -m "feat(auth): add user management page"
```

---

### Task 14: 权限配置树形页面

**Files:**
- Create: `web/src/pages/Users/PermissionTree.tsx`

- [ ] **Step 1: 实现树形权限配置组件**

路由 `/users/:id/permissions`。

需要加载的数据：
- `GET /groups` — 服务器分组列表
- `GET /servers` — 服务器列表（含 group_id）
- `GET /databases` — 数据库列表
- `GET /probes` — 探测规则列表
- `GET /users/:id/permissions` — 当前权限

树形结构渲染：
- 服务器分组 section → 每组一个 checkbox → 展开子节点（组内服务器，组勾选时子节点置灰已选）
- 未分组服务器 section
- 数据库 section → 每个 RDS 一个 checkbox
- 探测规则 section → 每条规则一个 checkbox

保存按钮 → `PUT /users/:id/permissions`，收集所有勾选项转为 `{res_type, res_id}[]`。

- [ ] **Step 2: 在 App.tsx 注册路由**

```tsx
<Route path="/users/:id/permissions" element={<PermissionTree />} />
```

- [ ] **Step 3: tsc 验证 + npm run build**

- [ ] **Step 4: 提交**

```bash
git add web/src/pages/Users/PermissionTree.tsx web/src/App.tsx
git commit -m "feat(auth): add permission tree configuration page"
```

---

### Task 15: 端到端验证 + 部署

- [ ] **Step 1: 后端全量编译**

Run: `cd server && go build ./...`

- [ ] **Step 2: 后端测试**

Run: `cd server && go test ./... -v`

- [ ] **Step 3: 前端编译**

Run: `cd web && npx tsc --noEmit && npm run build`

- [ ] **Step 4: 本地启动验证**

启动 server，验证：
1. 首次启动迁移创建 admin 用户
2. 用原 admin 账号登录成功
3. 创建 operator 用户，must_change_pwd 流程
4. operator 只能看到分配的资源
5. viewer 无法执行写操作
6. 禁用用户即时踢下线

- [ ] **Step 5: 构建部署到 192.168.10.71**

按部署文档流程：编译 → SCP → 停服 → 替换 → 启动 → 验证。

- [ ] **Step 6: 提交最终版本**

```bash
git add -A
git commit -m "feat(auth): complete multi-user RBAC system"
```
