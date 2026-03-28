package store

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

func InitSQLite(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		return nil, err
	}
	// Ensure new columns exist (safe to run repeatedly — silently ignores duplicates)
	for _, col := range []string{
		"ALTER TABLE servers ADD COLUMN collect_docker BOOLEAN",
		"ALTER TABLE servers ADD COLUMN collect_gpu BOOLEAN",
	} {
		db.Exec(col) // ignore "duplicate column" errors
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS servers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			host_id TEXT UNIQUE NOT NULL,
			hostname TEXT NOT NULL,
			ip_addresses TEXT,
			os TEXT, kernel TEXT, arch TEXT,
			agent_version TEXT,
			cpu_cores INTEGER, cpu_model TEXT,
			memory_total INTEGER, disk_total INTEGER,
			gpu_model TEXT, gpu_memory INTEGER,
			boot_time INTEGER, last_seen INTEGER,
			status TEXT DEFAULT 'online',
			display_name TEXT, sort_order INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS assets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id INTEGER NOT NULL REFERENCES servers(id),
			name TEXT NOT NULL, category TEXT, description TEXT,
			tech_stack TEXT, path TEXT, port TEXT,
			status TEXT DEFAULT 'active', extra_info TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS probe_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id INTEGER REFERENCES servers(id),
			name TEXT NOT NULL, host TEXT NOT NULL,
			port INTEGER NOT NULL DEFAULT 0,
			protocol TEXT DEFAULT 'tcp',
			url TEXT DEFAULT '',
			expect_status INTEGER DEFAULT 200,
			expect_body TEXT DEFAULT '',
			interval_sec INTEGER DEFAULT 30,
			timeout_sec INTEGER DEFAULT 5,
			enabled BOOLEAN DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS alert_rules (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			name        TEXT NOT NULL,
			type        TEXT NOT NULL,
			target_id   TEXT DEFAULT '',
			operator    TEXT DEFAULT '>',
			threshold   REAL DEFAULT 0,
			unit        TEXT DEFAULT '%',
			duration    INTEGER DEFAULT 3,
			level       TEXT DEFAULT 'warning',
			enabled     BOOLEAN DEFAULT 1,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS notification_channels (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			name        TEXT NOT NULL,
			type        TEXT NOT NULL,
			config      TEXT NOT NULL,
			enabled     BOOLEAN DEFAULT 1,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS alert_events (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			rule_id       INTEGER NOT NULL,
			rule_name     TEXT NOT NULL,
			target_id     TEXT NOT NULL,
			target_label  TEXT NOT NULL DEFAULT '',
			level         TEXT NOT NULL,
			status        TEXT DEFAULT 'firing',
			silenced      BOOLEAN DEFAULT 0,
			value         REAL,
			message       TEXT,
			fired_at      DATETIME NOT NULL,
			resolved_at   DATETIME,
			resolve_type  TEXT DEFAULT '',
			acked_at      DATETIME,
			acked_by      TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS alert_notifications (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id      INTEGER NOT NULL,
			channel_id    INTEGER NOT NULL,
			notify_type   TEXT NOT NULL DEFAULT 'firing',
			status        TEXT DEFAULT 'pending',
			retry_count   INTEGER DEFAULT 0,
			last_error    TEXT DEFAULT '',
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
			claimed_at    DATETIME,
			sent_at       DATETIME
		)`,
		// Alert indexes
		`CREATE INDEX IF NOT EXISTS idx_alert_events_status ON alert_events(status)`,
		`CREATE INDEX IF NOT EXISTS idx_alert_events_rule_status ON alert_events(rule_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_alert_events_fired_at ON alert_events(fired_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_alert_notifications_status ON alert_notifications(status)`,
		`CREATE INDEX IF NOT EXISTS idx_alert_notifications_event_id ON alert_notifications(event_id)`,
		`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`,

		// Credentials (encrypted credentials)
		`CREATE TABLE IF NOT EXISTS credentials (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			name        TEXT NOT NULL,
			type        TEXT NOT NULL,
			encrypted   TEXT NOT NULL,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// Managed servers (UI-deployed servers)
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

		// Cloud accounts
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

		// Cloud instances
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

		// Platform settings (key-value)
		`CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,

		// NAS devices
		`CREATE TABLE IF NOT EXISTS nas_devices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			nas_type TEXT NOT NULL,
			host TEXT NOT NULL,
			port INTEGER NOT NULL DEFAULT 22,
			ssh_user TEXT NOT NULL DEFAULT 'root',
			credential_id INTEGER NOT NULL REFERENCES credentials(id),
			collect_interval INTEGER DEFAULT 60,
			status TEXT DEFAULT 'unknown',
			last_seen INTEGER,
			system_info TEXT DEFAULT '{}',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_nas_devices_host_port ON nas_devices(host, port)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}

	// --- Version-based migrations ---
	if err := migrateV1(db); err != nil {
		return err
	}

	return nil
}

func migrateV1(db *sql.DB) error {
	// Ensure schema_version table exists (idempotent)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return err
	}

	var version int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&version); err != nil {
		return err
	}
	if version >= 1 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Check if probe_rules has the 'url' column (new schema indicator)
	hasURL := false
	rows, err := tx.Query(`PRAGMA table_info(probe_rules)`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "url" {
			hasURL = true
		}
	}
	rows.Close()

	// If old schema, rebuild probe_rules
	if !hasURL {
		if _, err := tx.Exec(`ALTER TABLE probe_rules RENAME TO probe_rules_old`); err != nil {
			return err
		}
		if _, err := tx.Exec(`CREATE TABLE probe_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id INTEGER REFERENCES servers(id),
			name TEXT NOT NULL, host TEXT NOT NULL,
			port INTEGER NOT NULL DEFAULT 0,
			protocol TEXT DEFAULT 'tcp',
			url TEXT DEFAULT '',
			expect_status INTEGER DEFAULT 200,
			expect_body TEXT DEFAULT '',
			interval_sec INTEGER DEFAULT 30,
			timeout_sec INTEGER DEFAULT 5,
			enabled BOOLEAN DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO probe_rules (id, server_id, name, host, port, protocol, interval_sec, timeout_sec, enabled, created_at)
			SELECT id, server_id, name, host, port, protocol, interval_sec, timeout_sec, enabled, created_at FROM probe_rules_old`); err != nil {
			return err
		}
		if _, err := tx.Exec(`DROP TABLE probe_rules_old`); err != nil {
			return err
		}
	}

	// Create server_groups table
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS server_groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		sort_order INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return err
	}

	// Add group_id column to servers (ignore duplicate column error)
	if _, err := tx.Exec(`ALTER TABLE servers ADD COLUMN group_id INTEGER REFERENCES server_groups(id)`); err != nil {
		// Ignore "duplicate column" error — column may already exist
		if err.Error() != "duplicate column name: group_id" {
			return err
		}
	}

	// Mark migration complete
	if _, err := tx.Exec(`INSERT INTO schema_version VALUES(1)`); err != nil {
		return err
	}

	return tx.Commit()
}
