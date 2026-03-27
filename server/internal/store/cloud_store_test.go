package store

import (
	"database/sql"
	"testing"
)

func setupCloudTest(t *testing.T) (*sql.DB, *CloudStore, int) {
	t.Helper()
	db := setupTestSQLite(t)
	// Insert a dummy credential for FK
	res, err := db.Exec("INSERT INTO credentials (name, type, encrypted) VALUES ('test-ak', 'aliyun_ak', 'enc')")
	if err != nil {
		t.Fatal(err)
	}
	credID64, _ := res.LastInsertId()
	return db, NewCloudStore(db), int(credID64)
}

func TestCloudStore_CreateAndGetAccount(t *testing.T) {
	_, cs, credID := setupCloudTest(t)

	id, err := cs.CreateAccount("my-aliyun", "aliyun", credID, []string{"cn-hangzhou", "cn-beijing"}, true)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id < 1 {
		t.Fatalf("expected positive id, got %d", id)
	}

	a, err := cs.GetAccount(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if a.Name != "my-aliyun" {
		t.Errorf("name = %q", a.Name)
	}
	if a.Provider != "aliyun" {
		t.Errorf("provider = %q", a.Provider)
	}
	if !a.AutoDiscover {
		t.Error("auto_discover should be true")
	}
	if len(a.RegionIDs) != 2 || a.RegionIDs[0] != "cn-hangzhou" {
		t.Errorf("region_ids = %v", a.RegionIDs)
	}
	if a.SyncState != "pending" {
		t.Errorf("sync_state = %q", a.SyncState)
	}
}

func TestCloudStore_ListAccounts(t *testing.T) {
	_, cs, credID := setupCloudTest(t)

	cs.CreateAccount("acc1", "aliyun", credID, []string{"cn-hangzhou"}, true)
	cs.CreateAccount("acc2", "aliyun", credID, []string{"cn-beijing"}, false)

	list, err := cs.ListAccounts()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	if list[0].Name != "acc1" {
		t.Errorf("first = %q", list[0].Name)
	}
}

func TestCloudStore_UpsertInstance(t *testing.T) {
	_, cs, credID := setupCloudTest(t)

	accID, _ := cs.CreateAccount("acc", "aliyun", credID, []string{"cn-hangzhou"}, true)

	inst := &CloudInstance{
		InstanceType: "ecs",
		InstanceID:   "i-abc123",
		HostID:       "cloud-ecs-i-abc123",
		InstanceName: "web-server-1",
		RegionID:     "cn-hangzhou",
		Spec:         "ecs.c6.large",
		Extra:        "{}",
	}

	// Insert
	if err := cs.UpsertInstance(accID, inst); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}

	instances, _ := cs.ListInstances(accID)
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	origID := instances[0].ID
	if instances[0].InstanceName != "web-server-1" {
		t.Errorf("name = %q", instances[0].InstanceName)
	}
	if instances[0].Monitored {
		t.Error("new instance should not be monitored")
	}

	// Set monitored=1 manually to test preservation
	cs.UpdateInstanceMonitored(origID, true)

	// Upsert again with updated name
	inst.InstanceName = "web-server-1-updated"
	inst.Spec = "ecs.c6.xlarge"
	if err := cs.UpsertInstance(accID, inst); err != nil {
		t.Fatalf("upsert update: %v", err)
	}

	instances, _ = cs.ListInstances(accID)
	if len(instances) != 1 {
		t.Fatalf("expected 1 after upsert, got %d", len(instances))
	}
	if instances[0].ID != origID {
		t.Errorf("id changed from %d to %d", origID, instances[0].ID)
	}
	if instances[0].InstanceName != "web-server-1-updated" {
		t.Errorf("name not updated: %q", instances[0].InstanceName)
	}
	if instances[0].Spec != "ecs.c6.xlarge" {
		t.Errorf("spec not updated: %q", instances[0].Spec)
	}
	if !instances[0].Monitored {
		t.Error("monitored should be preserved as true after upsert")
	}
}

func TestCloudStore_ConfirmInstances(t *testing.T) {
	db, cs, credID := setupCloudTest(t)

	accID, _ := cs.CreateAccount("acc", "aliyun", credID, []string{"cn-hangzhou"}, true)

	inst := &CloudInstance{
		InstanceType: "ecs",
		InstanceID:   "i-confirm1",
		HostID:       "cloud-ecs-i-confirm1",
		InstanceName: "confirm-server",
		RegionID:     "cn-hangzhou",
		Spec:         "ecs.c6.large",
		Extra:        "{}",
	}
	cs.UpsertInstance(accID, inst)
	instances, _ := cs.ListInstances(accID)
	instID := instances[0].ID

	if err := cs.ConfirmInstances([]int{instID}); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// Verify monitored=1
	got, _ := cs.GetInstance(instID)
	if !got.Monitored {
		t.Error("instance should be monitored after confirm")
	}

	// Verify server row created
	var hostname string
	err := db.QueryRow("SELECT hostname FROM servers WHERE host_id=?", "cloud-ecs-i-confirm1").Scan(&hostname)
	if err != nil {
		t.Fatalf("server not found: %v", err)
	}
	if hostname != "confirm-server" {
		t.Errorf("hostname = %q", hostname)
	}
}

func TestCloudStore_LoadMonitoredInstances(t *testing.T) {
	_, cs, credID := setupCloudTest(t)

	accID, _ := cs.CreateAccount("acc", "aliyun", credID, []string{"cn-hangzhou"}, true)
	cs.UpdateAccountSyncState(accID, "synced", "")

	// ECS monitored
	cs.UpsertInstance(accID, &CloudInstance{
		InstanceType: "ecs", InstanceID: "i-ecs1", HostID: "cloud-ecs-i-ecs1",
		InstanceName: "ecs1", RegionID: "cn-hangzhou", Spec: "ecs.c6.large", Extra: "{}",
	})
	// RDS monitored
	cs.UpsertInstance(accID, &CloudInstance{
		InstanceType: "rds", InstanceID: "rm-rds1", HostID: "cloud-rds-rm-rds1",
		InstanceName: "rds1", RegionID: "cn-hangzhou", Engine: "MySQL 8.0", Extra: "{}",
	})
	// ECS not monitored
	cs.UpsertInstance(accID, &CloudInstance{
		InstanceType: "ecs", InstanceID: "i-ecs2", HostID: "cloud-ecs-i-ecs2",
		InstanceName: "ecs2", RegionID: "cn-hangzhou", Spec: "ecs.c6.large", Extra: "{}",
	})

	insts, _ := cs.ListInstances(accID)
	for _, inst := range insts {
		if inst.InstanceID == "i-ecs1" || inst.InstanceID == "rm-rds1" {
			cs.UpdateInstanceMonitored(inst.ID, true)
		}
	}

	ecs, rds, err := cs.LoadMonitoredInstances()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(ecs) != 1 {
		t.Errorf("expected 1 ecs, got %d", len(ecs))
	}
	if len(rds) != 1 {
		t.Errorf("expected 1 rds, got %d", len(rds))
	}

	// Account with pending state should not return instances
	accID2, _ := cs.CreateAccount("acc2", "aliyun", credID, []string{"cn-beijing"}, true)
	cs.UpsertInstance(accID2, &CloudInstance{
		InstanceType: "ecs", InstanceID: "i-ecs3", HostID: "cloud-ecs-i-ecs3",
		InstanceName: "ecs3", RegionID: "cn-beijing", Spec: "ecs.c6.large", Extra: "{}",
	})
	pendingInsts, _ := cs.ListInstances(accID2)
	cs.UpdateInstanceMonitored(pendingInsts[0].ID, true)

	ecs2, _, err := cs.LoadMonitoredInstances()
	if err != nil {
		t.Fatalf("load2: %v", err)
	}
	if len(ecs2) != 1 {
		t.Errorf("expected 1 ecs after pending account, got %d", len(ecs2))
	}
}

func TestCloudStore_GetDeleteImpact(t *testing.T) {
	db := setupTestSQLite(t)
	cs := NewCloudStore(db)

	db.Exec(`INSERT INTO servers (host_id, hostname, status, last_seen) VALUES ('h1', 'srv1', 'online', 0)`)
	var serverID int
	db.QueryRow("SELECT id FROM servers WHERE host_id='h1'").Scan(&serverID)

	db.Exec("INSERT INTO assets (server_id, name) VALUES (?, 'asset1')", serverID)
	db.Exec("INSERT INTO probe_rules (server_id, name, host, port) VALUES (?, 'probe1', '1.2.3.4', 80)", serverID)
	db.Exec("INSERT INTO alert_rules (name, type, target_id) VALUES ('rule1', 'cpu', 'h1')")
	db.Exec("INSERT INTO alert_events (rule_id, rule_name, target_id, level, fired_at) VALUES (1, 'rule1', 'h1', 'warning', CURRENT_TIMESTAMP)")

	impact, err := cs.GetDeleteImpact([]string{"h1"})
	if err != nil {
		t.Fatalf("impact: %v", err)
	}
	if impact.Servers != 1 {
		t.Errorf("servers = %d", impact.Servers)
	}
	if impact.Assets != 1 {
		t.Errorf("assets = %d", impact.Assets)
	}
	if impact.ProbeRules != 1 {
		t.Errorf("probe_rules = %d", impact.ProbeRules)
	}
	if impact.AlertRules != 1 {
		t.Errorf("alert_rules = %d", impact.AlertRules)
	}
	if impact.AlertEvents != 1 {
		t.Errorf("alert_events = %d", impact.AlertEvents)
	}
}

func TestCloudStore_CascadeDeleteServers(t *testing.T) {
	db := setupTestSQLite(t)
	cs := NewCloudStore(db)

	db.Exec(`INSERT INTO servers (host_id, hostname, status, last_seen) VALUES ('h1', 'srv1', 'online', 0)`)
	var serverID int
	db.QueryRow("SELECT id FROM servers WHERE host_id='h1'").Scan(&serverID)

	db.Exec("INSERT INTO assets (server_id, name) VALUES (?, 'asset1')", serverID)
	db.Exec("INSERT INTO probe_rules (server_id, name, host, port) VALUES (?, 'probe1', '1.2.3.4', 80)", serverID)
	db.Exec("INSERT INTO alert_rules (name, type, target_id) VALUES ('rule1', 'cpu', 'h1')")
	db.Exec("INSERT INTO alert_events (rule_id, rule_name, target_id, level, fired_at) VALUES (1, 'rule1', 'h1', 'warning', CURRENT_TIMESTAMP)")
	db.Exec("INSERT INTO alert_notifications (event_id, channel_id, notify_type) VALUES (1, 1, 'firing')")

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}

	if err := cs.CascadeDeleteServers(tx, []string{"h1"}); err != nil {
		tx.Rollback()
		t.Fatalf("cascade: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM servers WHERE host_id='h1'").Scan(&count)
	if count != 0 {
		t.Errorf("servers remaining: %d", count)
	}
	db.QueryRow("SELECT COUNT(*) FROM assets WHERE server_id=?", serverID).Scan(&count)
	if count != 0 {
		t.Errorf("assets remaining: %d", count)
	}
	db.QueryRow("SELECT COUNT(*) FROM probe_rules WHERE server_id=?", serverID).Scan(&count)
	if count != 0 {
		t.Errorf("probe_rules remaining: %d", count)
	}
	db.QueryRow("SELECT COUNT(*) FROM alert_rules WHERE target_id='h1'").Scan(&count)
	if count != 0 {
		t.Errorf("alert_rules remaining: %d", count)
	}
	db.QueryRow("SELECT COUNT(*) FROM alert_events WHERE target_id='h1'").Scan(&count)
	if count != 0 {
		t.Errorf("alert_events remaining: %d", count)
	}
	db.QueryRow("SELECT COUNT(*) FROM alert_notifications").Scan(&count)
	if count != 0 {
		t.Errorf("alert_notifications remaining: %d", count)
	}
}
