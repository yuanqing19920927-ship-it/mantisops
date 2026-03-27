package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"opsboard/server/internal/crypto"
)

type Credential struct {
	ID        int               `json:"id"`
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Data      map[string]string `json:"-"` // decrypted, never serialized
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type CredentialSummary struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	UsedBy    int       `json:"used_by"`
}

type CredentialStore struct {
	db        *sql.DB
	masterKey []byte
}

func NewCredentialStore(db *sql.DB, masterKey []byte) *CredentialStore {
	return &CredentialStore{db: db, masterKey: masterKey}
}

func (s *CredentialStore) MasterKey() []byte {
	return s.masterKey
}

func (s *CredentialStore) Create(name, credType string, data map[string]string) (int, error) {
	plaintext, err := json.Marshal(data)
	if err != nil {
		return 0, fmt.Errorf("marshal credential data: %w", err)
	}
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
	if err := json.Unmarshal(plaintext, &c.Data); err != nil {
		return nil, fmt.Errorf("unmarshal credential %d: %w", id, err)
	}
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
		if err := rows.Scan(&cs.ID, &cs.Name, &cs.Type, &cs.CreatedAt, &cs.UsedBy); err != nil {
			return nil, err
		}
		list = append(list, cs)
	}
	return list, nil
}

func (s *CredentialStore) Update(id int, name string, data map[string]string) error {
	plaintext, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal credential data: %w", err)
	}
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
	if err := s.db.QueryRow(`
		SELECT (SELECT COUNT(*) FROM managed_servers WHERE credential_id = ?) +
		       (SELECT COUNT(*) FROM cloud_accounts WHERE credential_id = ?)
	`, id, id).Scan(&refCount); err != nil {
		return fmt.Errorf("check credential references: %w", err)
	}
	if refCount > 0 {
		return fmt.Errorf("credential %d is referenced by %d records, cannot delete", id, refCount)
	}
	_, err := s.db.Exec("DELETE FROM credentials WHERE id = ?", id)
	return err
}
