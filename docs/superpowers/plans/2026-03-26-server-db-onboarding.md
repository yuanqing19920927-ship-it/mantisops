# 服务器与数据库接入管理 — 实现计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 UI 驱动的本地服务器 Agent 自动部署和阿里云账号接入管理，替代手动配置文件编辑。

**Architecture:** 共享基础设施层（AES 加密 + 凭据存储 + 4 张新表），之上构建两个独立子系统：(1) SSH 部署器 + 状态机实现本地 Agent 安装；(2) 云账号管理器实现阿里云 ECS/RDS 自动发现。两者通过 WebSocket 推送实时进度，并重构现有 AliyunCollector/DatabaseHandler/BillingHandler 从数据库读取配置。

**Tech Stack:** Go (Gin, gRPC, golang.org/x/crypto/ssh, pkg/sftp, 阿里云 SDK v6), React 19 + TypeScript + Tailwind CSS

**Spec:** `docs/superpowers/specs/2026-03-26-server-db-onboarding-design.md`

---

## Chunk 1: 基础设施层

### Task 1: SQLite schema 扩展 + 外键启用

**Files:**
- Modify: `server/internal/store/sqlite.go`

- [ ] **Step 1: 启用 SQLite 外键约束**

在 `InitSQLite` 连接串中添加 foreign_keys pragma：

```go
// 修改前
db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")

// 修改后
db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
```

- [ ] **Step 2: 在 migrate() 中添加 4 张新表**

在现有 `stmts` 切片末尾追加：

```go
// Credentials (加密凭据)
`CREATE TABLE IF NOT EXISTS credentials (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,
    encrypted   TEXT NOT NULL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
)`,

// Managed servers (托管服务器)
`CREATE TABLE IF NOT EXISTS managed_servers (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    host            TEXT NOT NULL,
    ssh_port        INTEGER DEFAULT 22,
    ssh_user        TEXT NOT NULL,
    credential_id   INTEGER NOT NULL REFERENCES credentials(id),
    detected_arch   TEXT DEFAULT '',
    ssh_host_key    TEXT DEFAULT '',
    install_options TEXT DEFAULT '{}',
    install_state   TEXT DEFAULT 'pending',
    install_error   TEXT DEFAULT '',
    agent_host_id   TEXT DEFAULT '',
    agent_version   TEXT DEFAULT '',
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
`CREATE INDEX IF NOT EXISTS idx_managed_servers_host ON managed_servers(host)`,
`CREATE INDEX IF NOT EXISTS idx_managed_servers_agent_host_id ON managed_servers(agent_host_id)`,

// Cloud accounts (云账号)
`CREATE TABLE IF NOT EXISTS cloud_accounts (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    provider        TEXT DEFAULT 'aliyun',
    credential_id   INTEGER NOT NULL REFERENCES credentials(id),
    region_ids      TEXT DEFAULT '[]',
    auto_discover   INTEGER DEFAULT 1,
    sync_state      TEXT DEFAULT 'pending',
    sync_error      TEXT DEFAULT '',
    last_synced_at  DATETIME,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
)`,

// Cloud instances (云实例)
`CREATE TABLE IF NOT EXISTS cloud_instances (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    cloud_account_id INTEGER NOT NULL REFERENCES cloud_accounts(id) ON DELETE CASCADE,
    instance_type    TEXT NOT NULL,
    instance_id      TEXT NOT NULL,
    host_id          TEXT NOT NULL,
    instance_name    TEXT DEFAULT '',
    region_id        TEXT DEFAULT '',
    spec             TEXT DEFAULT '',
    engine           TEXT DEFAULT '',
    endpoint         TEXT DEFAULT '',
    monitored        INTEGER DEFAULT 0,
    extra            TEXT DEFAULT '{}',
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
`CREATE UNIQUE INDEX IF NOT EXISTS idx_cloud_instances_account_instance ON cloud_instances(cloud_account_id, instance_id)`,
`CREATE UNIQUE INDEX IF NOT EXISTS idx_cloud_instances_host_id ON cloud_instances(host_id)`,
`CREATE INDEX IF NOT EXISTS idx_cloud_instances_type ON cloud_instances(instance_type)`,
```

- [ ] **Step 3: 验证现有数据的外键完整性**

启用 foreign_keys 后，确保现有 `assets.server_id` 和 `probe_rules.server_id` 都指向存在的 `servers.id`。用 sqlite3 CLI 或写测试检查：

```sql
SELECT a.id FROM assets a LEFT JOIN servers s ON a.server_id = s.id WHERE s.id IS NULL;
SELECT p.id FROM probe_rules p LEFT JOIN servers s ON p.server_id = s.id WHERE s.id IS NULL;
```

若有孤立行，先清理再启用外键。

- [ ] **Step 4: 编译验证**

Run: `cd /Users/piggy/Projects/opsboard/server && go build ./...`
Expected: 编译通过，无错误

- [ ] **Step 5: Commit**

```bash
git add server/internal/store/sqlite.go
git commit -m "feat(store): add onboarding tables and enable foreign keys"
```

---

### Task 2: AES-256-GCM 加密模块

**Files:**
- Create: `server/internal/crypto/aes.go`
- Create: `server/internal/crypto/aes_test.go`

- [ ] **Step 1: 写测试**

```go
// crypto/aes_test.go
package crypto

import (
    "crypto/rand"
    "encoding/hex"
    "testing"
)

func TestEncryptDecrypt(t *testing.T) {
    key := make([]byte, 32)
    rand.Read(key)
    plaintext := []byte(`{"password":"secret123"}`)

    ciphertext, err := Encrypt(key, plaintext)
    if err != nil {
        t.Fatalf("encrypt: %v", err)
    }

    decrypted, err := Decrypt(key, ciphertext)
    if err != nil {
        t.Fatalf("decrypt: %v", err)
    }

    if string(decrypted) != string(plaintext) {
        t.Errorf("got %q, want %q", decrypted, plaintext)
    }
}

func TestDecryptWrongKey(t *testing.T) {
    key1 := make([]byte, 32)
    key2 := make([]byte, 32)
    rand.Read(key1)
    rand.Read(key2)

    ciphertext, _ := Encrypt(key1, []byte("secret"))
    _, err := Decrypt(key2, ciphertext)
    if err == nil {
        t.Error("expected error decrypting with wrong key")
    }
}

func TestParseKeyHex(t *testing.T) {
    keyHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    key, err := ParseKeyHex(keyHex)
    if err != nil {
        t.Fatalf("parse: %v", err)
    }
    if len(key) != 32 {
        t.Errorf("key length %d, want 32", len(key))
    }
}

func TestGenerateKeyHex(t *testing.T) {
    keyHex, err := GenerateKeyHex()
    if err != nil {
        t.Fatalf("generate: %v", err)
    }
    if len(keyHex) != 64 {
        t.Errorf("hex length %d, want 64", len(keyHex))
    }
    _, err = hex.DecodeString(keyHex)
    if err != nil {
        t.Errorf("invalid hex: %v", err)
    }
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd /Users/piggy/Projects/opsboard/server && go test ./internal/crypto/ -v`
Expected: 编译失败 — 包不存在

- [ ] **Step 3: 实现加密模块**

```go
// crypto/aes.go
package crypto

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "encoding/hex"
    "fmt"
    "io"
    "os"
)

// Encrypt 使用 AES-256-GCM 加密，返回 base64(nonce + ciphertext + tag)
func Encrypt(key, plaintext []byte) (string, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return "", err
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return "", err
    }
    sealed := gcm.Seal(nonce, nonce, plaintext, nil)
    return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt 解密 base64(nonce + ciphertext + tag)
func Decrypt(key []byte, ciphertext string) ([]byte, error) {
    data, err := base64.StdEncoding.DecodeString(ciphertext)
    if err != nil {
        return nil, err
    }
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }
    nonceSize := gcm.NonceSize()
    if len(data) < nonceSize {
        return nil, fmt.Errorf("ciphertext too short")
    }
    return gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
}

// ParseKeyHex 从 64 位 hex 字符串解析 32 字节密钥
func ParseKeyHex(keyHex string) ([]byte, error) {
    key, err := hex.DecodeString(keyHex)
    if err != nil {
        return nil, fmt.Errorf("invalid hex key: %w", err)
    }
    if len(key) != 32 {
        return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
    }
    return key, nil
}

// GenerateKeyHex 生成随机 32 字节密钥并返回 hex 编码
func GenerateKeyHex() (string, error) {
    key := make([]byte, 32)
    if _, err := io.ReadFull(rand.Reader, key); err != nil {
        return "", err
    }
    return hex.EncodeToString(key), nil
}

// LoadKey 按优先级加载加密密钥：环境变量 > 配置文件 > 自动生成
func LoadKey(configKeyHex string) ([]byte, error) {
    // 1. 环境变量
    if envKey := os.Getenv("OPSBOARD_ENCRYPTION_KEY"); envKey != "" {
        return ParseKeyHex(envKey)
    }
    // 2. 配置文件
    if configKeyHex != "" {
        return ParseKeyHex(configKeyHex)
    }
    // 3. 自动生成
    keyHex, err := GenerateKeyHex()
    if err != nil {
        return nil, err
    }
    fmt.Printf("[WARN] encryption_key not configured, auto-generated: %s\n", keyHex)
    fmt.Println("[WARN] Please save this key to server.yaml or OPSBOARD_ENCRYPTION_KEY env var. Key loss = credential loss.")
    return ParseKeyHex(keyHex)
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd /Users/piggy/Projects/opsboard/server && go test ./internal/crypto/ -v`
Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/crypto/
git commit -m "feat(crypto): add AES-256-GCM encryption module"
```

---

### Task 3: Config 结构扩展

**Files:**
- Modify: `server/internal/config/config.go`

- [ ] **Step 1: 新增字段**

在 `Config` struct 中添加：

```go
type Config struct {
    // ... 现有字段不动 ...
    EncryptionKey string         `yaml:"encryption_key"`
    AgentBin      AgentBinConfig `yaml:"agent"`
}

type AgentBinConfig struct {
    BinaryDir       string `yaml:"binary_dir"`
    RegisterTimeout int    `yaml:"register_timeout"` // 秒，默认 120
}
```

- [ ] **Step 2: 编译验证**

Run: `cd /Users/piggy/Projects/opsboard/server && go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add server/internal/config/config.go
git commit -m "feat(config): add encryption_key and agent binary config"
```

---

### Task 4: 凭据 Store

**Files:**
- Create: `server/internal/store/credential_store.go`
- Create: `server/internal/store/credential_store_test.go`

- [ ] **Step 1: 写测试**

测试 CRUD + 加密/解密 + 引用检查。

```go
package store

import (
    "testing"
)

func TestCredentialStore_CreateAndGet(t *testing.T) {
    db := setupTestDB(t) // 复用已有的测试 helper
    cs := NewCredentialStore(db, testEncryptionKey())

    id, err := cs.Create("test-ssh", "ssh_password", map[string]string{"password": "secret"})
    if err != nil {
        t.Fatalf("create: %v", err)
    }

    cred, err := cs.Get(id)
    if err != nil {
        t.Fatalf("get: %v", err)
    }
    if cred.Name != "test-ssh" || cred.Type != "ssh_password" {
        t.Errorf("unexpected: %+v", cred)
    }
    if cred.Data["password"] != "secret" {
        t.Errorf("decrypted password = %q, want %q", cred.Data["password"], "secret")
    }
}

func TestCredentialStore_List_NoSensitiveData(t *testing.T) {
    db := setupTestDB(t)
    cs := NewCredentialStore(db, testEncryptionKey())
    cs.Create("cred1", "ssh_password", map[string]string{"password": "x"})

    list, err := cs.List()
    if err != nil {
        t.Fatalf("list: %v", err)
    }
    if len(list) != 1 || list[0].Name != "cred1" {
        t.Errorf("unexpected list: %+v", list)
    }
    // CredentialSummary should not have Data field
}

func TestCredentialStore_Delete_WithReference(t *testing.T) {
    db := setupTestDB(t)
    cs := NewCredentialStore(db, testEncryptionKey())
    id, _ := cs.Create("ref-test", "ssh_password", map[string]string{"password": "x"})

    // 插入一个引用该凭据的 managed_server
    db.Exec("INSERT INTO managed_servers (host, ssh_user, credential_id) VALUES ('1.2.3.4','root',?)", id)

    err := cs.Delete(id)
    if err == nil {
        t.Error("expected error deleting referenced credential")
    }
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd /Users/piggy/Projects/opsboard/server && go test ./internal/store/ -run TestCredential -v`
Expected: 编译失败

- [ ] **Step 3: 实现 CredentialStore**

```go
package store

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "time"

    "opsboard/server/internal/crypto"
)

type Credential struct {
    ID        int
    Name      string
    Type      string
    Data      map[string]string // 解密后的数据
    CreatedAt time.Time
    UpdatedAt time.Time
}

type CredentialSummary struct {
    ID        int       `json:"id"`
    Name      string    `json:"name"`
    Type      string    `json:"type"`
    CreatedAt time.Time `json:"created_at"`
    UsedBy    int       `json:"used_by"` // 被引用次数
}

type CredentialStore struct {
    db        *sql.DB
    masterKey []byte
}

func NewCredentialStore(db *sql.DB, keyHex string) *CredentialStore {
    key, err := crypto.LoadKey(keyHex)
    if err != nil {
        panic(fmt.Sprintf("encryption key error: %v", err))
    }
    return &CredentialStore{db: db, masterKey: key}
}

func (s *CredentialStore) Create(name, credType string, data map[string]string) (int, error) {
    plaintext, _ := json.Marshal(data)
    encrypted, err := crypto.Encrypt(s.masterKey, plaintext)
    if err != nil {
        return 0, fmt.Errorf("encrypt: %w", err)
    }
    res, err := s.db.Exec(
        "INSERT INTO credentials (name, type, encrypted) VALUES (?, ?, ?)",
        name, credType, encrypted,
    )
    if err != nil {
        return 0, err
    }
    id, _ := res.LastInsertId()
    return int(id), nil
}

func (s *CredentialStore) Get(id int) (*Credential, error) {
    var c Credential
    var encrypted string
    err := s.db.QueryRow(
        "SELECT id, name, type, encrypted, created_at, updated_at FROM credentials WHERE id = ?", id,
    ).Scan(&c.ID, &c.Name, &c.Type, &encrypted, &c.CreatedAt, &c.UpdatedAt)
    if err != nil {
        return nil, err
    }
    plaintext, err := crypto.Decrypt(s.masterKey, encrypted)
    if err != nil {
        return nil, fmt.Errorf("decrypt credential %d: %w", id, err)
    }
    c.Data = make(map[string]string)
    json.Unmarshal(plaintext, &c.Data)
    return &c, nil
}

func (s *CredentialStore) List() ([]CredentialSummary, error) {
    rows, err := s.db.Query(`
        SELECT c.id, c.name, c.type, c.created_at,
            (SELECT COUNT(*) FROM managed_servers WHERE credential_id = c.id) +
            (SELECT COUNT(*) FROM cloud_accounts WHERE credential_id = c.id) AS used_by
        FROM credentials c ORDER BY c.id
    `)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var list []CredentialSummary
    for rows.Next() {
        var cs CredentialSummary
        rows.Scan(&cs.ID, &cs.Name, &cs.Type, &cs.CreatedAt, &cs.UsedBy)
        list = append(list, cs)
    }
    return list, nil
}

func (s *CredentialStore) Update(id int, name string, data map[string]string) error {
    plaintext, _ := json.Marshal(data)
    encrypted, err := crypto.Encrypt(s.masterKey, plaintext)
    if err != nil {
        return err
    }
    _, err = s.db.Exec(
        "UPDATE credentials SET name=?, encrypted=?, updated_at=CURRENT_TIMESTAMP WHERE id=?",
        name, encrypted, id,
    )
    return err
}

func (s *CredentialStore) Delete(id int) error {
    var refCount int
    s.db.QueryRow(`
        SELECT (SELECT COUNT(*) FROM managed_servers WHERE credential_id = ?) +
               (SELECT COUNT(*) FROM cloud_accounts WHERE credential_id = ?)
    `, id, id).Scan(&refCount)
    if refCount > 0 {
        return fmt.Errorf("credential %d is referenced by %d records", id, refCount)
    }
    _, err := s.db.Exec("DELETE FROM credentials WHERE id = ?", id)
    return err
}

func (s *CredentialStore) MasterKey() []byte {
    return s.masterKey
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd /Users/piggy/Projects/opsboard/server && go test ./internal/store/ -run TestCredential -v`
Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/store/credential_store.go server/internal/store/credential_store_test.go
git commit -m "feat(store): add credential store with AES encryption"
```

---

### Task 5: 凭据 API Handler

**Files:**
- Create: `server/internal/api/credential_handler.go`

- [ ] **Step 1: 实现 handler**

```go
package api

import (
    "net/http"
    "strconv"

    "github.com/gin-gonic/gin"
    "opsboard/server/internal/store"
)

type CredentialHandler struct {
    store *store.CredentialStore
}

func NewCredentialHandler(s *store.CredentialStore) *CredentialHandler {
    return &CredentialHandler{store: s}
}

func (h *CredentialHandler) List(c *gin.Context) {
    list, err := h.store.List()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, list)
}

func (h *CredentialHandler) Create(c *gin.Context) {
    var req struct {
        Name string            `json:"name" binding:"required"`
        Type string            `json:"type" binding:"required"`
        Data map[string]string `json:"data" binding:"required"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    id, err := h.store.Create(req.Name, req.Type, req.Data)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *CredentialHandler) Update(c *gin.Context) {
    id, _ := strconv.Atoi(c.Param("id"))
    var req struct {
        Name string            `json:"name" binding:"required"`
        Data map[string]string `json:"data" binding:"required"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    if err := h.store.Update(id, req.Name, req.Data); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *CredentialHandler) Delete(c *gin.Context) {
    id, _ := strconv.Atoi(c.Param("id"))
    if err := h.store.Delete(id); err != nil {
        c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"ok": true})
}
```

- [ ] **Step 2: 编译验证**

Run: `cd /Users/piggy/Projects/opsboard/server && go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add server/internal/api/credential_handler.go
git commit -m "feat(api): add credential CRUD handler"
```

---

### Task 6: Router 重构为 RouterDeps

**Files:**
- Modify: `server/internal/api/router.go`
- Modify: `server/cmd/server/main.go`

- [ ] **Step 1: 定义 RouterDeps 结构体，重构 SetupRouter**

将 `SetupRouter` 的 10 个参数合并为一个结构体。暂时只包含现有字段，后续 task 逐步添加新 handler。

```go
type RouterDeps struct {
    ServerStore     *store.ServerStore
    Hub             *ws.Hub
    MetricsProvider MetricsProvider
    StaticDir       string
    ProbeHandler    *ProbeHandler
    AssetHandler    *AssetHandler
    AuthHandler     *AuthHandler
    DatabaseHandler *DatabaseHandler
    BillingHandler  *BillingHandler
    AlertHandler    *AlertHandler
    // 后续 task 添加：
    // CredentialHandler    *CredentialHandler
    // ManagedServerHandler *ManagedServerHandler
    // CloudHandler         *CloudHandler
}

func SetupRouter(deps RouterDeps) *gin.Engine {
    // 将所有 deps.XXX 替换原来的参数引用
    // 路由定义不变
}
```

- [ ] **Step 2: 更新 main.go 的调用方式**

```go
router := api.SetupRouter(api.RouterDeps{
    ServerStore:     serverStore,
    Hub:             hub,
    ProbeHandler:    probeHandler,
    AssetHandler:    assetHandler,
    AuthHandler:     authHandler,
    DatabaseHandler: dbHandler,
    BillingHandler:  billingHandler,
    AlertHandler:    alertHandler,
    MetricsProvider: metricsProvider,
    StaticDir:       cfg.Server.StaticDir,
})
```

- [ ] **Step 3: 编译验证**

Run: `cd /Users/piggy/Projects/opsboard/server && go build ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add server/internal/api/router.go server/cmd/server/main.go
git commit -m "refactor(api): replace SetupRouter params with RouterDeps struct"
```

---

## Chunk 2: 云账号接入（后端）

### Task 7: Cloud Store

**Files:**
- Create: `server/internal/store/cloud_store.go`
- Create: `server/internal/store/cloud_store_test.go`

- [ ] **Step 1: 写测试**

测试 cloud_accounts CRUD、cloud_instances CRUD、confirm、loadMonitored、影响摘要查询。

- [ ] **Step 2: 跑测试确认失败**

- [ ] **Step 3: 实现 CloudStore**

核心方法：
- `CreateAccount(name, provider string, credentialID int, regionIDs []string, autoDiscover bool) (int, error)`
- `GetAccount(id int) (*CloudAccount, error)`
- `ListAccounts() ([]CloudAccount, error)`
- `UpdateAccountSyncState(id int, state, syncError string) error`
- `DeleteAccount(id int) error` — 仅删 cloud_accounts 行，ON DELETE CASCADE 处理 cloud_instances
- `UpsertInstance(accountID int, inst *CloudInstance) error` — INSERT OR REPLACE
- `ListInstances(accountID int) ([]CloudInstance, error)`
- `ConfirmInstances(ids []int, serverStore *ServerStore) error` — 事务内：设 monitored=1 + ECS 注册 servers
- `LoadMonitoredInstances() (ecs []CloudInstance, rds []CloudInstance, error)` — 联合查询 synced/partial + monitored=1
- `GetDeleteImpact(hostIDs []string) (*DeleteImpact, error)` — 查 assets/probe_rules/alert_rules/alert_events 数量
- `CascadeDeleteServers(hostIDs []string) error` — 事务内按顺序清理所有关联数据

- [ ] **Step 4: 跑测试确认通过**

- [ ] **Step 5: Commit**

```bash
git add server/internal/store/cloud_store.go server/internal/store/cloud_store_test.go
git commit -m "feat(store): add cloud account and instance store"
```

---

### Task 8: Cloud Manager — 验证 + 发现

**Files:**
- Create: `server/internal/cloud/manager.go`

- [ ] **Step 1: 添加新 Go 依赖**

```bash
cd /Users/piggy/Projects/opsboard/server
go get github.com/alibabacloud-go/sts-20150401/v2
go get github.com/alibabacloud-go/rds-20140815/v6
```

- [ ] **Step 2: 实现 Manager**

核心方法：
- `Verify(ak, sk string) (*VerifyResult, error)` — STS GetCallerIdentity + ECS/CMS/RDS 权限探测
- `Sync(accountID int) error` — 异步：发现 ECS + 发现 RDS → 入库 monitored=0 → 更新 sync_state
- `DiscoverECS(ak, sk string, regionIDs []string) ([]CloudInstance, error)`
- `DiscoverRDS(ak, sk string, regionIDs []string) ([]CloudInstance, error)`
- `ConfirmInstances(instanceIDs []int) error` — 委托给 CloudStore.ConfirmInstances
- `DeleteAccount(accountID int, force bool) (impact *DeleteImpact, err error)` — force=false 返回摘要，force=true 执行级联删除
- `DeleteInstance(instanceID int, force bool) (impact *DeleteImpact, err error)`

WebSocket 进度广播集成到 Sync 流程中。

- [ ] **Step 3: 编译验证**

- [ ] **Step 4: Commit**

```bash
git add server/internal/cloud/
git commit -m "feat(cloud): add manager with verify, discover, sync, delete"
```

---

### Task 9: Cloud API Handler

**Files:**
- Create: `server/internal/api/cloud_handler.go`

- [ ] **Step 1: 实现 CloudHandler**

端点映射：
- `GET /cloud-accounts` → `List`
- `POST /cloud-accounts` → `Create`（含内嵌 credential 创建）
- `PUT /cloud-accounts/:id` → `Update`
- `DELETE /cloud-accounts/:id` → `Delete`（?force 参数）
- `POST /cloud-accounts/verify` → `Verify`（dry-run，不落库）
- `POST /cloud-accounts/:id/sync` → `Sync`
- `GET /cloud-accounts/:id/instances` → `ListInstances`
- `POST /cloud-instances/confirm` → `ConfirmInstances`
- `PUT /cloud-instances/:id` → `UpdateInstance`
- `POST /cloud-instances` → `AddInstance`（手动添加）
- `DELETE /cloud-instances/:id` → `DeleteInstance`（?force 参数）

- [ ] **Step 2: 在 RouterDeps 和路由中注册**

在 `router.go` 的 v1 组中添加所有云账号路由。

- [ ] **Step 3: 编译验证**

- [ ] **Step 4: Commit**

```bash
git add server/internal/api/cloud_handler.go server/internal/api/router.go
git commit -m "feat(api): add cloud account and instance endpoints"
```

---

### Task 10: AliyunCollector 重构 + 旧配置迁移

**Files:**
- Modify: `server/internal/collector/aliyun.go`
- Modify: `server/cmd/server/main.go`

- [ ] **Step 1: 重构 AliyunCollector 构造函数**

新增 `cloudStore` 和 `credStore` 参数，保留 `fallbackCfg`。

- [ ] **Step 2: 实现 loadInstances() 双数据源**

优先从数据库加载 `synced`/`partial` 账号下 `monitored=1` 的实例；数据库为空时回退到 `fallbackCfg`。

- [ ] **Step 3: 实现 migrateFromConfig()**

启动时检测 `cloud_accounts` 表是否为空，若为空且配置文件有阿里云配置，自动导入（`monitored=1`）。

- [ ] **Step 4: 更新 collectAll() 使用 loadInstances()**

替换直接读 `ac.cfg.Instances` 和 `ac.cfg.RDS` 的逻辑。

- [ ] **Step 5: 更新 main.go 初始化顺序**

credentialStore 和 cloudStore 需要在 AliyunCollector 之前创建。

- [ ] **Step 6: 编译 + 验证旧配置仍可正常采集**

- [ ] **Step 7: Commit**

```bash
git add server/internal/collector/aliyun.go server/cmd/server/main.go
git commit -m "refactor(collector): dual data source with config migration"
```

---

### Task 11: DatabaseHandler + BillingHandler 重构

**Files:**
- Modify: `server/internal/api/database_handler.go`
- Modify: `server/internal/api/billing_handler.go`

- [ ] **Step 1: DatabaseHandler 改为从 CloudStore 读取 RDS 列表**

替换 `rdsConfig []config.AliyunRDS` 为 `cloudStore *store.CloudStore`。API 响应增加 `account_id`/`account_name` 字段。

- [ ] **Step 2: BillingHandler 改为从 CloudStore + CredentialStore 读取**

替换直接使用 `config.AliyunConfig`，改为从数据库读取各云账号的 AK/SK 后调用阿里云 API。响应增加 `account_id`/`account_name`。

- [ ] **Step 3: 统一 RDS SDK 版本为 v6**

更新 `billing_handler.go` 中的 import。

- [ ] **Step 4: 更新 main.go 中 handler 初始化**

- [ ] **Step 5: 编译验证**

- [ ] **Step 6: Commit**

```bash
git add server/internal/api/database_handler.go server/internal/api/billing_handler.go server/cmd/server/main.go
git commit -m "refactor(api): database and billing handlers read from DB"
```

---

### Task 12: 服务器来源字段 (source)

**Files:**
- Modify: `server/internal/api/server_handler.go`

- [ ] **Step 1: ServerHandler.List 中 LEFT JOIN 判断来源**

```sql
SELECT s.*,
    CASE
        WHEN ms.id IS NOT NULL THEN 'managed'
        WHEN ci.id IS NOT NULL THEN 'cloud'
        ELSE 'agent'
    END AS source
FROM servers s
LEFT JOIN managed_servers ms ON ms.agent_host_id = s.host_id
LEFT JOIN cloud_instances ci ON ci.host_id = s.host_id AND ci.instance_type = 'ecs'
ORDER BY s.sort_order, s.id
```

- [ ] **Step 2: 更新 Server model 和 JSON 响应增加 source 字段**

- [ ] **Step 3: Commit**

```bash
git add server/internal/api/server_handler.go server/internal/model/server.go
git commit -m "feat(api): add source field to server list response"
```

---

## Chunk 3: 本地服务器部署（后端）

### Task 13: SSH 客户端

**Files:**
- Create: `server/internal/deployer/ssh.go`
- Create: `server/internal/deployer/ssh_test.go`

- [ ] **Step 1: 添加依赖**

```bash
cd /Users/piggy/Projects/opsboard/server
go get golang.org/x/crypto/ssh
go get github.com/pkg/sftp
```

- [ ] **Step 2: 实现 SSHClient**

```go
type SSHClient struct {
    host       string
    port       int
    user       string
    authMethod ssh.AuthMethod
    hostKey    ssh.PublicKey // TOFU: nil = 接受任何 key 并返回
    conn       *ssh.Client
}

func NewSSHClient(host string, port int, user string, auth ssh.AuthMethod, hostKeyStr string) *SSHClient
func (c *SSHClient) TestConnection() (latencyMs int, hostKey string, arch string, osName string, err error)
func (c *SSHClient) Connect() error
func (c *SSHClient) Close()
func (c *SSHClient) Execute(cmd string) (stdout, stderr string, err error)
func (c *SSHClient) Upload(localPath, remotePath string) error    // SFTP
func (c *SSHClient) WriteFile(remotePath string, content []byte, perm os.FileMode) error // SFTP
func (c *SSHClient) DetectArch() (string, error) // uname -m → amd64/arm64
```

- [ ] **Step 3: 写单元测试**（mock SSH 或使用 testcontainers 可选，基础测试验证参数处理）

- [ ] **Step 4: Commit**

```bash
git add server/internal/deployer/ssh.go server/internal/deployer/ssh_test.go
git commit -m "feat(deployer): SSH client with SFTP and TOFU host key"
```

---

### Task 14: Managed Server Store

**Files:**
- Create: `server/internal/store/managed_server_store.go`

- [ ] **Step 1: 实现 ManagedServerStore**

```go
type ManagedServer struct {
    ID             int
    Host           string
    SSHPort        int
    SSHUser        string
    CredentialID   int
    DetectedArch   string
    SSHHostKey     string
    InstallOptions string // JSON
    InstallState   string
    InstallError   string
    AgentHostID    string
    AgentVersion   string
    CreatedAt      time.Time
    UpdatedAt      time.Time
}

func (s *ManagedServerStore) Create(ms *ManagedServer) (int, error)
func (s *ManagedServerStore) List() ([]ManagedServer, error)
func (s *ManagedServerStore) Get(id int) (*ManagedServer, error)
func (s *ManagedServerStore) Delete(id int) error
func (s *ManagedServerStore) CASUpdateState(id int, fromStates []string, toState string) (bool, error) // CAS
func (s *ManagedServerStore) UpdateState(id int, state, errorMsg string) error
func (s *ManagedServerStore) UpdateAgentInfo(id int, agentHostID, agentVersion string) error
```

- [ ] **Step 2: Commit**

```bash
git add server/internal/store/managed_server_store.go
git commit -m "feat(store): add managed server store with CAS state updates"
```

---

### Task 15: Deployer 状态机

**Files:**
- Create: `server/internal/deployer/deployer.go`

- [ ] **Step 1: 实现 Deployer**

```go
type Deployer struct {
    managedStore *store.ManagedServerStore
    credStore    *store.CredentialStore
    serverStore  *store.ServerStore
    hub          *ws.Hub
    pskToken     string
    grpcAddr     string
    binaryDir    string
    timeout      time.Duration
    pendingCh    map[string]chan struct{}
    mu           sync.Mutex
}

func NewDeployer(...) *Deployer
func (d *Deployer) Deploy(managedID int) error          // CAS → goroutine runDeploy
func (d *Deployer) NotifyRegistered(hostID string)       // 通知 waiting 状态的部署
func (d *Deployer) TestConnection(req TestConnRequest) (*TestConnResult, error) // dry-run
```

`runDeploy` 实现完整的 testing → connected → uploading → installing → waiting → online 状态机。

- [ ] **Step 2: Commit**

```bash
git add server/internal/deployer/deployer.go
git commit -m "feat(deployer): agent deployment state machine with WS progress"
```

---

### Task 16: Managed Server API Handler

**Files:**
- Create: `server/internal/api/managed_server_handler.go`

- [ ] **Step 1: 实现端点**

- `GET /managed-servers` → List
- `POST /managed-servers` → Create（含内嵌 credential）
- `POST /managed-servers/test-connection` → TestConnection（dry-run）
- `POST /managed-servers/:id/deploy` → Deploy
- `POST /managed-servers/:id/retry` → Retry
- `DELETE /managed-servers/:id` → Delete
- `POST /managed-servers/:id/uninstall` → Uninstall

- [ ] **Step 2: 在 RouterDeps 和路由中注册**

- [ ] **Step 3: Commit**

```bash
git add server/internal/api/managed_server_handler.go server/internal/api/router.go
git commit -m "feat(api): add managed server endpoints with deploy/test-connection"
```

---

### Task 17: gRPC Handler 新增 onRegister 回调

**Files:**
- Modify: `server/internal/grpc/handler.go`
- Modify: `server/cmd/server/main.go`

- [ ] **Step 1: NewHandler 增加 onRegister 参数**

```go
type Handler struct {
    serverStore *store.ServerStore
    onMetrics   func(payload *pb.MetricsPayload)
    onRegister  func(hostID string) // 新增
}

func NewHandler(ss *store.ServerStore, onMetrics func(*pb.MetricsPayload), onRegister func(string)) *Handler
```

- [ ] **Step 2: Register 方法中调用 onRegister**

- [ ] **Step 3: main.go 中传入 deployer.NotifyRegistered**

- [ ] **Step 4: 编译验证全量构建**

Run: `cd /Users/piggy/Projects/opsboard/server && go build ./cmd/server/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/grpc/handler.go server/cmd/server/main.go
git commit -m "feat(grpc): add onRegister callback for deploy notification"
```

---

## Chunk 4: 前端实现

### Task 18: 前端类型定义

**Files:**
- Create: `web/src/types/onboarding.ts`

- [ ] **Step 1: 定义所有接入管理相关类型**

```typescript
// ManagedServer, CloudAccount, CloudInstance, Credential, VerifyResult,
// SSHTestRequest, SSHTestResult, AddServerRequest, AddCloudAccountRequest,
// DeployProgress, CloudSyncProgress, DeleteImpact 等
```

- [ ] **Step 2: Commit**

---

### Task 19: 前端 API 客户端

**Files:**
- Create: `web/src/api/onboarding.ts`

- [ ] **Step 1: 实现所有 API 调用**

复用 `api/client.ts` 中已有的 `get`/`post`/`put`/`del` helper，但函数定义放在独立文件中。

- [ ] **Step 2: Commit**

---

### Task 20: 添加服务器对话框

**Files:**
- Create: `web/src/components/AddServerDialog.tsx`
- Create: `web/src/components/DeployProgress.tsx`
- Modify: `web/src/pages/Servers/index.tsx`
- Modify: `web/src/hooks/useWebSocket.ts`

- [ ] **Step 1: 实现 DeployProgress 组件**

步骤指示器：testing → connected → uploading → installing → waiting → online，根据 WebSocket 消息更新。

- [ ] **Step 2: 实现 AddServerDialog 组件**

表单：host/port/user/auth_type/password|key + 高级选项。测试连接 → 进度面板 → 完成。

- [ ] **Step 3: 修改 Servers/index.tsx 添加按钮**

顶部添加「+ 添加服务器」按钮，点击打开 AddServerDialog。

- [ ] **Step 4: useWebSocket 处理 deploy_progress 消息**

- [ ] **Step 5: Commit**

```bash
git add web/src/components/AddServerDialog.tsx web/src/components/DeployProgress.tsx \
        web/src/pages/Servers/index.tsx web/src/hooks/useWebSocket.ts
git commit -m "feat(web): add server dialog with deploy progress"
```

---

### Task 21: 添加云账号对话框

**Files:**
- Create: `web/src/components/AddCloudAccountDialog.tsx`
- Create: `web/src/components/InstanceSelector.tsx`
- Create: `web/src/components/ManualInstanceForm.tsx`
- Modify: `web/src/pages/Databases/index.tsx`

- [ ] **Step 1: 实现 InstanceSelector 组件**

勾选列表：ECS 区域 + RDS 区域，全选/全不选，确认接入按钮。

- [ ] **Step 2: 实现 ManualInstanceForm 组件**

手动输入 instance_type + instance_id + region_id。

- [ ] **Step 3: 实现 AddCloudAccountDialog 组件**

三阶段：填写 AK → 验证 → 发现/手动添加 → 确认接入。

- [ ] **Step 4: 修改 Databases/index.tsx**

- 顶部添加「+ 添加云账号」按钮
- 删除硬编码的 `RDS_INSTANCES` 映射表
- RDS 卡片从 API 返回的 `name`/`engine`/`spec` 渲染
- 按 `account_name` 分组展示

- [ ] **Step 5: useWebSocket 处理 cloud_sync_progress 消息**

- [ ] **Step 6: Commit**

```bash
git add web/src/components/AddCloudAccountDialog.tsx web/src/components/InstanceSelector.tsx \
        web/src/components/ManualInstanceForm.tsx web/src/pages/Databases/index.tsx \
        web/src/hooks/useWebSocket.ts
git commit -m "feat(web): add cloud account dialog with instance discovery"
```

---

### Task 22: 修改 DatabaseDetail + Billing 页面

**Files:**
- Modify: `web/src/pages/DatabaseDetail/index.tsx`
- Modify: `web/src/pages/Billing/index.tsx`

- [ ] **Step 1: DatabaseDetail 去硬编码**

数据库信息卡从 API 响应的 `name`/`engine`/`spec`/`endpoint` 渲染，不再使用前端映射表。

- [ ] **Step 2: Billing 增加账号列**

表格新增「所属账号」列，显示 `account_name`。

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/DatabaseDetail/index.tsx web/src/pages/Billing/index.tsx
git commit -m "feat(web): remove hardcoded RDS info, add account column"
```

---

## Chunk 5: 集成 + 验证

### Task 23: main.go 完整集成

**Files:**
- Modify: `server/cmd/server/main.go`

- [ ] **Step 1: 完整初始化链路**

```
1. SQLite (with foreign_keys)
2. CredentialStore (encryption key)
3. CloudStore
4. ManagedServerStore
5. CloudManager
6. Deployer
7. AliyunCollector (cloudStore + credStore + fallbackCfg + migration)
8. All handlers
9. gRPC handler (onRegister = deployer.NotifyRegistered)
10. SetupRouter(deps)
```

- [ ] **Step 2: 编译完整 server**

Run: `cd /Users/piggy/Projects/opsboard/server && go build -o ../build/opsboard-server ./cmd/server/`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add server/cmd/server/main.go
git commit -m "feat: integrate all onboarding components in main.go"
```

---

### Task 24: 前端构建验证

**Files:** 无新增，验证已有修改

- [ ] **Step 1: TypeScript 类型检查**

Run: `cd /Users/piggy/Projects/opsboard/web && npx tsc --noEmit`
Expected: 无类型错误

- [ ] **Step 2: 前端构建**

Run: `cd /Users/piggy/Projects/opsboard/web && npm run build`
Expected: 构建成功

- [ ] **Step 3: Commit（如有修复）**

---

### Task 25: 交叉编译 + 部署验证

- [ ] **Step 1: 构建 Linux 二进制**

```bash
cd /Users/piggy/Projects/opsboard
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/opsboard-server-linux ./server/cmd/server/
```

- [ ] **Step 2: 构建前端**

```bash
cd web && npm run build && cp -r dist ../build/web-dist
```

- [ ] **Step 3: 部署到 192.168.10.65 并验证**

- 上传二进制和前端
- 重启服务
- 验证现有功能正常（旧配置自动导入）
- 验证新 API 端点可访问
- 验证前端页面加载正常

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "feat: server and database onboarding management - complete"
```
