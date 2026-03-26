package store

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

func InitSQLite(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		return nil, err
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
			server_id INTEGER NOT NULL REFERENCES servers(id),
			name TEXT NOT NULL, host TEXT NOT NULL,
			port INTEGER NOT NULL,
			protocol TEXT DEFAULT 'tcp',
			interval_sec INTEGER DEFAULT 30,
			timeout_sec INTEGER DEFAULT 5,
			enabled BOOLEAN DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}
