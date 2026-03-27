package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
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

// --------------- Account CRUD ---------------

func (s *CloudStore) CreateAccount(name, provider string, credentialID int, regionIDs []string, autoDiscover bool) (int, error) {
	regions, _ := json.Marshal(regionIDs)
	autoDiscoverInt := 0
	if autoDiscover {
		autoDiscoverInt = 1
	}
	res, err := s.db.Exec(`INSERT INTO cloud_accounts (name, provider, credential_id, region_ids, auto_discover)
		VALUES (?, ?, ?, ?, ?)`, name, provider, credentialID, string(regions), autoDiscoverInt)
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
		a, err := scanAccountRow(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, *a)
	}
	return accounts, rows.Err()
}

func (s *CloudStore) UpdateAccount(id int, name string, regionIDs []string, autoDiscover bool) error {
	regions, _ := json.Marshal(regionIDs)
	autoDiscoverInt := 0
	if autoDiscover {
		autoDiscoverInt = 1
	}
	_, err := s.db.Exec(`UPDATE cloud_accounts SET name=?, region_ids=?, auto_discover=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		name, string(regions), autoDiscoverInt, id)
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

// --------------- Instance CRUD ---------------

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

func (s *CloudStore) GetInstance(id int) (*CloudInstance, error) {
	row := s.db.QueryRow(`SELECT id, cloud_account_id, instance_type, instance_id, host_id,
		COALESCE(instance_name,''), COALESCE(region_id,''), COALESCE(spec,''),
		COALESCE(engine,''), COALESCE(endpoint,''), monitored, COALESCE(extra,'{}'),
		created_at, updated_at
		FROM cloud_instances WHERE id=?`, id)
	var inst CloudInstance
	var monitoredInt int
	err := row.Scan(&inst.ID, &inst.CloudAccountID, &inst.InstanceType, &inst.InstanceID,
		&inst.HostID, &inst.InstanceName, &inst.RegionID, &inst.Spec,
		&inst.Engine, &inst.Endpoint, &monitoredInt, &inst.Extra,
		&inst.CreatedAt, &inst.UpdatedAt)
	if err != nil {
		return nil, err
	}
	inst.Monitored = monitoredInt == 1
	return &inst, nil
}

func (s *CloudStore) UpdateInstanceMonitored(id int, monitored bool) error {
	v := 0
	if monitored {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE cloud_instances SET monitored=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, v, id)
	return err
}

func (s *CloudStore) DeleteInstance(id int) error {
	_, err := s.db.Exec(`DELETE FROM cloud_instances WHERE id=?`, id)
	return err
}

// --------------- ConfirmInstances ---------------

func (s *CloudStore) ConfirmInstances(ids []int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, id := range ids {
		// Set monitored=1
		if _, err := tx.Exec(`UPDATE cloud_instances SET monitored=1, updated_at=CURRENT_TIMESTAMP WHERE id=?`, id); err != nil {
			return err
		}

		// Get instance details
		var instType, instName, hostID string
		err := tx.QueryRow(`SELECT instance_type, instance_name, host_id FROM cloud_instances WHERE id=?`, id).
			Scan(&instType, &instName, &hostID)
		if err != nil {
			return err
		}

		// If ECS, register into servers table
		if instType == "ecs" {
			if _, err := tx.Exec(`INSERT INTO servers (host_id, hostname, ip_addresses, status, last_seen)
				VALUES (?, ?, '', 'unknown', 0)
				ON CONFLICT(host_id) DO UPDATE SET
					hostname=excluded.hostname,
					updated_at=CURRENT_TIMESTAMP`,
				hostID, instName); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// --------------- LoadMonitoredInstances ---------------

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

// --------------- Delete impact & cascade ---------------

func (s *CloudStore) GetDeleteImpact(hostIDs []string) (*DeleteImpact, error) {
	if len(hostIDs) == 0 {
		return &DeleteImpact{}, nil
	}

	placeholders := strings.Repeat("?,", len(hostIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, len(hostIDs))
	for i, h := range hostIDs {
		args[i] = h
	}

	impact := &DeleteImpact{}

	// Count servers
	err := s.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM servers WHERE host_id IN (%s)`, placeholders), args...).Scan(&impact.Servers)
	if err != nil {
		return nil, err
	}

	// Count assets
	err = s.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM assets WHERE server_id IN (SELECT id FROM servers WHERE host_id IN (%s))`, placeholders), args...).Scan(&impact.Assets)
	if err != nil {
		return nil, err
	}

	// Count probe_rules
	err = s.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM probe_rules WHERE server_id IN (SELECT id FROM servers WHERE host_id IN (%s))`, placeholders), args...).Scan(&impact.ProbeRules)
	if err != nil {
		return nil, err
	}

	// Count alert_rules (target_id matches host_id)
	err = s.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM alert_rules WHERE target_id IN (%s)`, placeholders), args...).Scan(&impact.AlertRules)
	if err != nil {
		return nil, err
	}

	// Count alert_events (target_id matches host_id)
	err = s.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM alert_events WHERE target_id IN (%s)`, placeholders), args...).Scan(&impact.AlertEvents)
	if err != nil {
		return nil, err
	}

	return impact, nil
}

func (s *CloudStore) CascadeDeleteServers(tx *sql.Tx, hostIDs []string) error {
	if len(hostIDs) == 0 {
		return nil
	}

	placeholders := strings.Repeat("?,", len(hostIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, len(hostIDs))
	for i, h := range hostIDs {
		args[i] = h
	}

	// Delete alert_notifications via event_id
	_, err := tx.Exec(fmt.Sprintf(`DELETE FROM alert_notifications WHERE event_id IN (
		SELECT id FROM alert_events WHERE target_id IN (%s))`, placeholders), args...)
	if err != nil {
		return err
	}

	// Delete alert_events
	_, err = tx.Exec(fmt.Sprintf(`DELETE FROM alert_events WHERE target_id IN (%s)`, placeholders), args...)
	if err != nil {
		return err
	}

	// Delete alert_rules
	_, err = tx.Exec(fmt.Sprintf(`DELETE FROM alert_rules WHERE target_id IN (%s)`, placeholders), args...)
	if err != nil {
		return err
	}

	// Delete probe_rules (by server_id via servers.host_id)
	_, err = tx.Exec(fmt.Sprintf(`DELETE FROM probe_rules WHERE server_id IN (
		SELECT id FROM servers WHERE host_id IN (%s))`, placeholders), args...)
	if err != nil {
		return err
	}

	// Delete assets (by server_id via servers.host_id)
	_, err = tx.Exec(fmt.Sprintf(`DELETE FROM assets WHERE server_id IN (
		SELECT id FROM servers WHERE host_id IN (%s))`, placeholders), args...)
	if err != nil {
		return err
	}

	// Delete servers
	_, err = tx.Exec(fmt.Sprintf(`DELETE FROM servers WHERE host_id IN (%s)`, placeholders), args...)
	return err
}

// --------------- scan helpers ---------------

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

func scanAccountRow(rows *sql.Rows) (*CloudAccount, error) {
	return scanAccount(rows)
}

func scanInstanceRow(rows *sql.Rows) (*CloudInstance, error) {
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
