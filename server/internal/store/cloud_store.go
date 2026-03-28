package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Sync state constants for CloudAccount
const (
	SyncStatePending = "pending"
	SyncStateSyncing = "syncing"
	SyncStateSynced  = "synced"
	SyncStatePartial = "partial"
	SyncStateFailed  = "failed"
)

type CloudAccount struct {
	ID           int        `json:"id"`
	Name         string     `json:"name"`
	Provider     string     `json:"provider"`
	CredentialID int        `json:"credential_id"`
	RegionIDs    []string   `json:"region_ids"`
	AutoDiscover bool       `json:"auto_discover"`
	SyncState    string     `json:"sync_state"`
	SyncError    string     `json:"sync_error"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type CloudInstance struct {
	ID             int       `json:"id"`
	CloudAccountID int       `json:"cloud_account_id"`
	InstanceType   string    `json:"instance_type"`
	InstanceID     string    `json:"instance_id"`
	HostID         string    `json:"host_id"`
	InstanceName   string    `json:"instance_name"`
	RegionID       string    `json:"region_id"`
	Spec           string    `json:"spec"`
	Engine         string    `json:"engine"`
	Endpoint       string    `json:"endpoint"`
	Monitored      bool      `json:"monitored"`
	Extra          string    `json:"extra"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type DeleteImpact struct {
	Servers     int `json:"servers"`
	Assets      int `json:"assets"`
	ProbeRules  int `json:"probe_rules"`
	AlertRules  int `json:"alert_rules"`
	AlertEvents int `json:"alert_events"`
}

type CloudStore struct {
	db *sql.DB
}

func NewCloudStore(db *sql.DB) *CloudStore {
	return &CloudStore{db: db}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}


func (s *CloudStore) CreateAccount(name, provider string, credentialID int, regionIDs []string, autoDiscover bool) (int, error) {
	regions, err := json.Marshal(regionIDs)
	if err != nil {
		return 0, fmt.Errorf("marshal region_ids: %w", err)
	}
	res, err := s.db.Exec(`INSERT INTO cloud_accounts (name, provider, credential_id, region_ids, auto_discover)
		VALUES (?, ?, ?, ?, ?)`, name, provider, credentialID, string(regions), boolToInt(autoDiscover))
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return int(id), err
}

func (s *CloudStore) GetAccount(id int) (*CloudAccount, error) {
	row := s.db.QueryRow(`SELECT id, name, provider, credential_id, region_ids,
		auto_discover, sync_state, COALESCE(sync_error,''), last_synced_at, created_at, updated_at
		FROM cloud_accounts WHERE id=?`, id)
	return scanAccount(row)
}

func (s *CloudStore) ListAccounts() ([]CloudAccount, error) {
	rows, err := s.db.Query(`SELECT id, name, provider, credential_id, region_ids,
		auto_discover, sync_state, COALESCE(sync_error,''), last_synced_at, created_at, updated_at
		FROM cloud_accounts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []CloudAccount
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, *a)
	}
	return accounts, rows.Err()
}

func (s *CloudStore) UpdateAccount(id int, name string, regionIDs []string, autoDiscover bool) error {
	regions, err := json.Marshal(regionIDs)
	if err != nil {
		return fmt.Errorf("marshal region_ids: %w", err)
	}
	_, err = s.db.Exec(`UPDATE cloud_accounts SET name=?, region_ids=?, auto_discover=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		name, string(regions), boolToInt(autoDiscover), id)
	return err
}

func (s *CloudStore) UpdateAccountSyncState(id int, state, syncError string) error {
	_, err := s.db.Exec(`UPDATE cloud_accounts SET sync_state=?, sync_error=?, last_synced_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		state, syncError, id)
	return err
}

func (s *CloudStore) DeleteAccountRow(tx *sql.Tx, id int) error {
	_, err := tx.Exec(`DELETE FROM cloud_accounts WHERE id=?`, id)
	return err
}


func (s *CloudStore) UpsertInstance(accountID int, inst *CloudInstance) error {
	_, err := s.db.Exec(`INSERT INTO cloud_instances (cloud_account_id, instance_type, instance_id, host_id, instance_name, region_id, spec, engine, endpoint, extra)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(cloud_account_id, instance_id) DO UPDATE SET
			instance_name=excluded.instance_name, region_id=excluded.region_id, spec=excluded.spec,
			engine=excluded.engine, endpoint=excluded.endpoint, extra=excluded.extra,
			updated_at=CURRENT_TIMESTAMP`,
		accountID, inst.InstanceType, inst.InstanceID, inst.HostID,
		inst.InstanceName, inst.RegionID, inst.Spec, inst.Engine, inst.Endpoint, inst.Extra)
	return err
}

func (s *CloudStore) ListInstances(accountID int) ([]CloudInstance, error) {
	rows, err := s.db.Query(`SELECT id, cloud_account_id, instance_type, instance_id, host_id,
		COALESCE(instance_name,''), COALESCE(region_id,''), COALESCE(spec,''),
		COALESCE(engine,''), COALESCE(endpoint,''), monitored, COALESCE(extra,'{}'),
		created_at, updated_at
		FROM cloud_instances WHERE cloud_account_id=? ORDER BY id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var instances []CloudInstance
	for rows.Next() {
		inst, err := scanInstanceRow(rows)
		if err != nil {
			return nil, err
		}
		instances = append(instances, *inst)
	}
	return instances, rows.Err()
}

// ListAllInstances returns all cloud instances across all accounts.
func (s *CloudStore) ListAllInstances() ([]CloudInstance, error) {
	rows, err := s.db.Query(`SELECT id, cloud_account_id, instance_type, instance_id, host_id,
		COALESCE(instance_name,''), COALESCE(region_id,''), COALESCE(spec,''),
		COALESCE(engine,''), COALESCE(endpoint,''), monitored, COALESCE(extra,'{}'),
		created_at, updated_at
		FROM cloud_instances ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var instances []CloudInstance
	for rows.Next() {
		inst, err := scanInstanceRow(rows)
		if err != nil {
			return nil, err
		}
		instances = append(instances, *inst)
	}
	return instances, rows.Err()
}

func (s *CloudStore) GetInstance(id int) (*CloudInstance, error) {
	row := s.db.QueryRow(`SELECT id, cloud_account_id, instance_type, instance_id, host_id,
		COALESCE(instance_name,''), COALESCE(region_id,''), COALESCE(spec,''),
		COALESCE(engine,''), COALESCE(endpoint,''), monitored, COALESCE(extra,'{}'),
		created_at, updated_at
		FROM cloud_instances WHERE id=?`, id)
	return scanInstanceRow(row)
}

func (s *CloudStore) UpdateInstanceMonitored(id int, monitored bool) error {
	_, err := s.db.Exec(`UPDATE cloud_instances SET monitored=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, boolToInt(monitored), id)
	return err
}

func (s *CloudStore) DeleteInstance(id int) error {
	_, err := s.db.Exec(`DELETE FROM cloud_instances WHERE id=?`, id)
	return err
}

func (s *CloudStore) ConfirmInstances(ids []int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, id := range ids {
		if _, err := tx.Exec(`UPDATE cloud_instances SET monitored=1, updated_at=CURRENT_TIMESTAMP WHERE id=?`, id); err != nil {
			return err
		}

		var instType, instName, hostID string
		err := tx.QueryRow(`SELECT instance_type, instance_name, host_id FROM cloud_instances WHERE id=?`, id).
			Scan(&instType, &instName, &hostID)
		if err != nil {
			return err
		}

		if instType == "ecs" {
			// Parse extra JSON for system info from cloud API
			var extra string
			tx.QueryRow(`SELECT COALESCE(extra,'{}') FROM cloud_instances WHERE id=?`, id).Scan(&extra)
			var info struct {
				OSName string   `json:"os_name"`
				CPU    int      `json:"cpu"`
				Memory int      `json:"memory"` // MB
				IPs    []string `json:"ips"`
			}
			json.Unmarshal([]byte(extra), &info)
			ipsJSON, _ := json.Marshal(info.IPs)

			if _, err := tx.Exec(`INSERT INTO servers (host_id, hostname, ip_addresses, os, cpu_cores, memory_total, status, last_seen)
				VALUES (?, ?, ?, ?, ?, ?, 'unknown', 0)
				ON CONFLICT(host_id) DO UPDATE SET
					hostname=excluded.hostname,
					ip_addresses=CASE WHEN excluded.ip_addresses != '[]' AND excluded.ip_addresses != 'null' THEN excluded.ip_addresses ELSE servers.ip_addresses END,
					os=CASE WHEN excluded.os != '' THEN excluded.os ELSE servers.os END,
					cpu_cores=CASE WHEN excluded.cpu_cores > 0 THEN excluded.cpu_cores ELSE servers.cpu_cores END,
					memory_total=CASE WHEN excluded.memory_total > 0 THEN excluded.memory_total ELSE servers.memory_total END,
					updated_at=CURRENT_TIMESTAMP`,
				hostID, instName, string(ipsJSON), info.OSName, info.CPU, int64(info.Memory)*1024*1024); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (s *CloudStore) LoadMonitoredInstances() (ecs []CloudInstance, rds []CloudInstance, err error) {
	rows, err := s.db.Query(`SELECT ci.id, ci.cloud_account_id, ci.instance_type, ci.instance_id, ci.host_id,
		COALESCE(ci.instance_name,''), COALESCE(ci.region_id,''), COALESCE(ci.spec,''),
		COALESCE(ci.engine,''), COALESCE(ci.endpoint,''), ci.monitored, COALESCE(ci.extra,'{}'),
		ci.created_at, ci.updated_at
		FROM cloud_instances ci
		JOIN cloud_accounts ca ON ci.cloud_account_id = ca.id
		WHERE ca.sync_state IN ('synced','partial') AND ci.monitored = 1
		ORDER BY ci.id`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		inst, err := scanInstanceRow(rows)
		if err != nil {
			return nil, nil, err
		}
		switch inst.InstanceType {
		case "ecs":
			ecs = append(ecs, *inst)
		case "rds":
			rds = append(rds, *inst)
		}
	}
	return ecs, rds, rows.Err()
}

// SyncServersFromCloud updates servers table with system info from cloud_instances extra data
// for all monitored ECS instances of a given account.
func (s *CloudStore) SyncServersFromCloud(accountID int) error {
	rows, err := s.db.Query(`SELECT host_id, instance_name, COALESCE(extra,'{}') FROM cloud_instances
		WHERE cloud_account_id=? AND instance_type='ecs' AND monitored=1`, accountID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var hostID, instName, extra string
		if err := rows.Scan(&hostID, &instName, &extra); err != nil {
			return err
		}
		var info struct {
			OSName string   `json:"os_name"`
			CPU    int      `json:"cpu"`
			Memory int      `json:"memory"`
			IPs    []string `json:"ips"`
		}
		json.Unmarshal([]byte(extra), &info)
		ipsJSON, _ := json.Marshal(info.IPs)

		s.db.Exec(`UPDATE servers SET
			hostname=?,
			ip_addresses=CASE WHEN ? != '[]' AND ? != 'null' AND ? != '' THEN ? ELSE ip_addresses END,
			os=CASE WHEN ? != '' THEN ? ELSE os END,
			cpu_cores=CASE WHEN ? > 0 THEN ? ELSE cpu_cores END,
			memory_total=CASE WHEN ? > 0 THEN ? ELSE memory_total END,
			updated_at=CURRENT_TIMESTAMP
			WHERE host_id=?`,
			instName,
			string(ipsJSON), string(ipsJSON), string(ipsJSON), string(ipsJSON),
			info.OSName, info.OSName,
			info.CPU, info.CPU,
			int64(info.Memory)*1024*1024, int64(info.Memory)*1024*1024,
			hostID)
	}
	return rows.Err()
}

// --------------- helpers ---------------

func buildPlaceholders(hostIDs []string) (string, []interface{}) {
	placeholders := strings.Repeat("?,", len(hostIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, len(hostIDs))
	for i, h := range hostIDs {
		args[i] = h
	}
	return placeholders, args
}

// --------------- Delete impact & cascade ---------------

func (s *CloudStore) GetDeleteImpact(hostIDs []string) (*DeleteImpact, error) {
	if len(hostIDs) == 0 {
		return &DeleteImpact{}, nil
	}

	placeholders, args := buildPlaceholders(hostIDs)

	impact := &DeleteImpact{}

	queries := []struct {
		query string
		dest  *int
	}{
		{`SELECT COUNT(*) FROM servers WHERE host_id IN (%s)`, &impact.Servers},
		{`SELECT COUNT(*) FROM assets WHERE server_id IN (SELECT id FROM servers WHERE host_id IN (%s))`, &impact.Assets},
		{`SELECT COUNT(*) FROM probe_rules WHERE server_id IN (SELECT id FROM servers WHERE host_id IN (%s))`, &impact.ProbeRules},
		{`SELECT COUNT(*) FROM alert_rules WHERE target_id IN (%s)`, &impact.AlertRules},
		{`SELECT COUNT(*) FROM alert_events WHERE target_id IN (%s)`, &impact.AlertEvents},
	}
	for _, q := range queries {
		if err := s.db.QueryRow(fmt.Sprintf(q.query, placeholders), args...).Scan(q.dest); err != nil {
			return nil, err
		}
	}

	return impact, nil
}

func (s *CloudStore) CascadeDeleteServers(tx *sql.Tx, hostIDs []string) error {
	if len(hostIDs) == 0 {
		return nil
	}

	placeholders, args := buildPlaceholders(hostIDs)

	// Order matters: delete child records before parents
	deletes := []string{
		`DELETE FROM alert_notifications WHERE event_id IN (SELECT id FROM alert_events WHERE target_id IN (%s))`,
		`DELETE FROM alert_events WHERE target_id IN (%s)`,
		`DELETE FROM alert_rules WHERE target_id IN (%s)`,
		`DELETE FROM probe_rules WHERE server_id IN (SELECT id FROM servers WHERE host_id IN (%s))`,
		`DELETE FROM assets WHERE server_id IN (SELECT id FROM servers WHERE host_id IN (%s))`,
		`DELETE FROM servers WHERE host_id IN (%s)`,
	}
	for _, tmpl := range deletes {
		if _, err := tx.Exec(fmt.Sprintf(tmpl, placeholders), args...); err != nil {
			return err
		}
	}
	return nil
}


type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanAccount(row rowScanner) (*CloudAccount, error) {
	var a CloudAccount
	var regionsJSON string
	var autoDiscoverInt int
	var lastSynced sql.NullTime
	err := row.Scan(&a.ID, &a.Name, &a.Provider, &a.CredentialID, &regionsJSON,
		&autoDiscoverInt, &a.SyncState, &a.SyncError, &lastSynced, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	a.AutoDiscover = autoDiscoverInt == 1
	if lastSynced.Valid {
		a.LastSyncedAt = &lastSynced.Time
	}
	if err := json.Unmarshal([]byte(regionsJSON), &a.RegionIDs); err != nil {
		a.RegionIDs = []string{}
	}
	return &a, nil
}

func scanInstanceRow(rows rowScanner) (*CloudInstance, error) {
	var inst CloudInstance
	var monitoredInt int
	err := rows.Scan(&inst.ID, &inst.CloudAccountID, &inst.InstanceType, &inst.InstanceID,
		&inst.HostID, &inst.InstanceName, &inst.RegionID, &inst.Spec,
		&inst.Engine, &inst.Endpoint, &monitoredInt, &inst.Extra,
		&inst.CreatedAt, &inst.UpdatedAt)
	if err != nil {
		return nil, err
	}
	inst.Monitored = monitoredInt == 1
	return &inst, nil
}
