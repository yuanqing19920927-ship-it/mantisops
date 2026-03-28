package store

import (
	"database/sql"
	"fmt"
	"time"
)

type NasDevice struct {
	ID              int        `json:"id"`
	Name            string     `json:"name"`
	NasType         string     `json:"nas_type"`
	Host            string     `json:"host"`
	Port            int        `json:"port"`
	SSHUser         string     `json:"ssh_user"`
	CredentialID    int        `json:"credential_id"`
	CollectInterval int        `json:"collect_interval"`
	Status          string     `json:"status"`
	LastSeen        *time.Time `json:"last_seen"`
	SystemInfo      string     `json:"system_info"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type NasStore struct {
	db *sql.DB
}

func NewNasStore(db *sql.DB) *NasStore {
	return &NasStore{db: db}
}

func (s *NasStore) List() ([]NasDevice, error) {
	rows, err := s.db.Query(`
		SELECT id, name, nas_type, host, port, ssh_user, credential_id,
		       collect_interval, status, last_seen, system_info, created_at, updated_at
		FROM nas_devices ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []NasDevice
	for rows.Next() {
		d, err := scanNasDevice(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *d)
	}
	return list, nil
}

func (s *NasStore) Get(id int) (*NasDevice, error) {
	row := s.db.QueryRow(`
		SELECT id, name, nas_type, host, port, ssh_user, credential_id,
		       collect_interval, status, last_seen, system_info, created_at, updated_at
		FROM nas_devices WHERE id = ?
	`, id)
	return scanNasDevice(row)
}

func (s *NasStore) Create(name, nasType, host string, port int, sshUser string, credentialID, collectInterval int) (int, error) {
	if sshUser == "" {
		sshUser = "root"
	}
	if collectInterval < 30 {
		collectInterval = 30
	}
	res, err := s.db.Exec(`
		INSERT INTO nas_devices (name, nas_type, host, port, ssh_user, credential_id, collect_interval)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, name, nasType, host, port, sshUser, credentialID, collectInterval)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func (s *NasStore) Update(id int, name, nasType, host string, port int, sshUser string, credentialID, collectInterval int) error {
	if sshUser == "" {
		sshUser = "root"
	}
	if collectInterval < 30 {
		collectInterval = 30
	}
	_, err := s.db.Exec(`
		UPDATE nas_devices SET name=?, nas_type=?, host=?, port=?, ssh_user=?, credential_id=?, collect_interval=?,
		       updated_at=CURRENT_TIMESTAMP
		WHERE id=?
	`, name, nasType, host, port, sshUser, credentialID, collectInterval, id)
	return err
}

func (s *NasStore) Delete(id int) error {
	_, err := s.db.Exec("DELETE FROM nas_devices WHERE id = ?", id)
	return err
}

func (s *NasStore) UpdateStatus(id int, status string) error {
	if status == "online" || status == "degraded" {
		_, err := s.db.Exec(`
			UPDATE nas_devices SET status=?, last_seen=?, updated_at=CURRENT_TIMESTAMP WHERE id=?
		`, status, time.Now().Unix(), id)
		return err
	}
	_, err := s.db.Exec(`
		UPDATE nas_devices SET status=?, updated_at=CURRENT_TIMESTAMP WHERE id=?
	`, status, id)
	return err
}

func (s *NasStore) UpdateSystemInfo(id int, info string) error {
	_, err := s.db.Exec(`
		UPDATE nas_devices SET system_info=?, updated_at=CURRENT_TIMESTAMP WHERE id=?
	`, info, id)
	return err
}

func scanNasDevice(row rowScanner) (*NasDevice, error) {
	var d NasDevice
	var lastSeen sql.NullInt64
	if err := row.Scan(
		&d.ID, &d.Name, &d.NasType, &d.Host, &d.Port, &d.SSHUser, &d.CredentialID,
		&d.CollectInterval, &d.Status, &lastSeen, &d.SystemInfo, &d.CreatedAt, &d.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if lastSeen.Valid {
		t := time.Unix(lastSeen.Int64, 0)
		d.LastSeen = &t
	}
	return &d, nil
}

func (s *NasStore) GetByHostPort(host string, port int) (*NasDevice, error) {
	row := s.db.QueryRow(`
		SELECT id, name, nas_type, host, port, ssh_user, credential_id,
		       collect_interval, status, last_seen, system_info, created_at, updated_at
		FROM nas_devices WHERE host = ? AND port = ?
	`, host, port)
	d, err := scanNasDevice(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("nas device not found: %s:%d", host, port)
		}
		return nil, err
	}
	return d, nil
}
