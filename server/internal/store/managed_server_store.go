package store

import (
	"database/sql"
	"time"
)

// Install state constants for ManagedServer
const (
	InstallStatePending    = "pending"
	InstallStateTesting    = "testing"
	InstallStateConnected  = "connected"
	InstallStateUploading  = "uploading"
	InstallStateInstalling = "installing"
	InstallStateWaiting    = "waiting"
	InstallStateOnline     = "online"
	InstallStateFailed     = "failed"
)

type ManagedServer struct {
	ID             int       `json:"id"`
	Host           string    `json:"host"`
	SSHPort        int       `json:"ssh_port"`
	SSHUser        string    `json:"ssh_user"`
	CredentialID   int       `json:"credential_id"`
	DetectedArch   string    `json:"detected_arch"`
	SSHHostKey     string    `json:"ssh_host_key"`
	InstallOptions string    `json:"install_options"`
	InstallState   string    `json:"install_state"`
	InstallError   string    `json:"install_error"`
	AgentHostID    string    `json:"agent_host_id"`
	AgentVersion   string    `json:"agent_version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type ManagedServerStore struct {
	db *sql.DB
}

func NewManagedServerStore(db *sql.DB) *ManagedServerStore {
	return &ManagedServerStore{db: db}
}

func (s *ManagedServerStore) Create(ms *ManagedServer) (int, error) {
	res, err := s.db.Exec(`
		INSERT INTO managed_servers (host, ssh_port, ssh_user, credential_id, ssh_host_key, install_options)
		VALUES (?, ?, ?, ?, ?, ?)`,
		ms.Host, ms.SSHPort, ms.SSHUser, ms.CredentialID, ms.SSHHostKey, ms.InstallOptions,
	)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func (s *ManagedServerStore) Get(id int) (*ManagedServer, error) {
	var ms ManagedServer
	err := s.db.QueryRow(`
		SELECT id, host, ssh_port, ssh_user, credential_id, detected_arch, ssh_host_key,
			install_options, install_state, install_error, agent_host_id, agent_version,
			created_at, updated_at
		FROM managed_servers WHERE id = ?`, id,
	).Scan(&ms.ID, &ms.Host, &ms.SSHPort, &ms.SSHUser, &ms.CredentialID,
		&ms.DetectedArch, &ms.SSHHostKey, &ms.InstallOptions, &ms.InstallState,
		&ms.InstallError, &ms.AgentHostID, &ms.AgentVersion, &ms.CreatedAt, &ms.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &ms, nil
}

func (s *ManagedServerStore) List() ([]ManagedServer, error) {
	rows, err := s.db.Query(`
		SELECT id, host, ssh_port, ssh_user, credential_id, detected_arch, ssh_host_key,
			install_options, install_state, install_error, agent_host_id, agent_version,
			created_at, updated_at
		FROM managed_servers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ManagedServer
	for rows.Next() {
		var ms ManagedServer
		if err := rows.Scan(&ms.ID, &ms.Host, &ms.SSHPort, &ms.SSHUser, &ms.CredentialID,
			&ms.DetectedArch, &ms.SSHHostKey, &ms.InstallOptions, &ms.InstallState,
			&ms.InstallError, &ms.AgentHostID, &ms.AgentVersion, &ms.CreatedAt, &ms.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, ms)
	}
	return list, nil
}

// CASUpdateState atomically updates state only if current state matches one of fromStates.
func (s *ManagedServerStore) CASUpdateState(id int, fromStates []string, toState string) (bool, error) {
	placeholders := ""
	args := make([]interface{}, 0, len(fromStates)+2)
	args = append(args, toState)
	args = append(args, id)
	for i, state := range fromStates {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, state)
	}

	res, err := s.db.Exec(
		`UPDATE managed_servers SET install_state=?, install_error='', updated_at=CURRENT_TIMESTAMP
		 WHERE id=? AND install_state IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

func (s *ManagedServerStore) UpdateState(id int, state, errorMsg string) error {
	_, err := s.db.Exec(
		`UPDATE managed_servers SET install_state=?, install_error=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		state, errorMsg, id,
	)
	return err
}

func (s *ManagedServerStore) UpdateDetectedArch(id int, arch string) error {
	_, err := s.db.Exec(
		`UPDATE managed_servers SET detected_arch=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		arch, id,
	)
	return err
}

func (s *ManagedServerStore) UpdateAgentInfo(id int, agentHostID, agentVersion string) error {
	_, err := s.db.Exec(
		`UPDATE managed_servers SET agent_host_id=?, agent_version=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		agentHostID, agentVersion, id,
	)
	return err
}

// Delete removes a managed server record and cleans up orphaned credentials.
func (s *ManagedServerStore) Delete(id int, credStore *CredentialStore) error {
	var credID int
	err := s.db.QueryRow("SELECT credential_id FROM managed_servers WHERE id = ?", id).Scan(&credID)
	if err != nil {
		return err
	}

	_, err = s.db.Exec("DELETE FROM managed_servers WHERE id = ?", id)
	if err != nil {
		return err
	}

	// Clean up orphaned credential (Delete returns error if still referenced)
	if credStore != nil {
		_ = credStore.Delete(credID)
	}

	return nil
}

// ResolveWaiting marks a "waiting" managed_server as "online" if its agent_host_id matches.
func (s *ManagedServerStore) ResolveWaiting(agentHostID string) {
	s.db.Exec(
		`UPDATE managed_servers SET install_state=?, updated_at=CURRENT_TIMESTAMP
		 WHERE agent_host_id=? AND install_state=?`,
		InstallStateOnline, agentHostID, InstallStateWaiting,
	)
}

// ResolveAllWaiting scans all "waiting" records and marks them "online" if the agent is registered.
func (s *ManagedServerStore) ResolveAllWaiting(serverStore *ServerStore) {
	rows, err := s.db.Query("SELECT id, agent_host_id FROM managed_servers WHERE install_state=?", InstallStateWaiting)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var agentID string
		if rows.Scan(&id, &agentID) != nil || agentID == "" {
			continue
		}
		srv, err := serverStore.GetByHostID(agentID)
		if err == nil && srv != nil {
			s.db.Exec("UPDATE managed_servers SET install_state=?, updated_at=CURRENT_TIMESTAMP WHERE id=?",
				InstallStateOnline, id)
		}
	}
}
