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

const userColumns = `id, username, password_hash, display_name, role, enabled, must_change_pwd, token_version, created_at, updated_at`

func scanUser(row interface{ Scan(...interface{}) error }) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Role, &u.Enabled, &u.MustChangePwd, &u.TokenVersion, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *UserStore) GetByID(id int64) (*User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userColumns+` FROM users WHERE id = ?`, id))
}

func (s *UserStore) GetByUsername(username string) (*User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userColumns+` FROM users WHERE username = ?`, username))
}

func (s *UserStore) List() ([]User, error) {
	rows, err := s.db.Query(`SELECT ` + userColumns + ` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *u)
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

func (s *UserStore) RemovePermissionByResource(resType, resID string) error {
	_, err := s.db.Exec(
		`DELETE FROM user_permissions WHERE res_type = ? AND res_id = ?`,
		resType, resID,
	)
	return err
}
