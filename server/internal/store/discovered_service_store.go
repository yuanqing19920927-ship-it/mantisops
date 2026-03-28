package store

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type DiscoveredService struct {
	ID        int       `json:"id"`
	HostID    string    `json:"host_id"`
	PID       int       `json:"pid"`
	Name      string    `json:"name"`
	CmdLine   string    `json:"cmd_line"`
	Port      int       `json:"port"`
	Protocol  string    `json:"protocol"`
	BindAddr  string    `json:"bind_addr"`
	Status    string    `json:"status"`
	AssetID   *int      `json:"asset_id"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

type DiscoveredServiceStore struct {
	db *sql.DB
}

func NewDiscoveredServiceStore(db *sql.DB) *DiscoveredServiceStore {
	return &DiscoveredServiceStore{db: db}
}

// SyncServices performs a full diff: new→insert, existing→update, missing→stopped.
func (s *DiscoveredServiceStore) SyncServices(hostID string, reported []DiscoveredService, assetStore *AssetStore, serverStore *ServerStore) error {
	now := time.Now()

	existing, err := s.ListByHost(hostID)
	if err != nil {
		return err
	}
	existingMap := make(map[string]*DiscoveredService)
	for i := range existing {
		key := portProtoKey(existing[i].Port, existing[i].Protocol)
		existingMap[key] = &existing[i]
	}

	// Load assets for port matching
	var assets []struct{ ID int; Port string }
	srv, err := serverStore.GetByHostID(hostID)
	if err == nil && srv != nil {
		rows, _ := s.db.Query("SELECT id, COALESCE(port,'') FROM assets WHERE server_id=?", srv.ID)
		if rows != nil {
			for rows.Next() {
				var a struct{ ID int; Port string }
				rows.Scan(&a.ID, &a.Port)
				assets = append(assets, a)
			}
			rows.Close()
		}
	}

	var errs []error
	reportedMap := make(map[string]bool)
	for _, r := range reported {
		key := portProtoKey(r.Port, r.Protocol)
		reportedMap[key] = true

		var assetID *int
		for _, a := range assets {
			if matchPort(a.Port, r.Port) {
				aid := a.ID
				assetID = &aid
				break
			}
		}

		if ex, ok := existingMap[key]; ok {
			if _, err := s.db.Exec(`UPDATE discovered_services SET pid=?, name=?, cmd_line=?, bind_addr=?, status='running', asset_id=?, last_seen=? WHERE id=?`,
				r.PID, r.Name, r.CmdLine, r.BindAddr, assetID, now, ex.ID); err != nil {
				errs = append(errs, fmt.Errorf("update service %d: %w", ex.ID, err))
			}
		} else {
			if _, err := s.db.Exec(`INSERT INTO discovered_services (host_id, pid, name, cmd_line, port, protocol, bind_addr, status, asset_id, first_seen, last_seen)
				VALUES (?, ?, ?, ?, ?, ?, ?, 'running', ?, ?, ?)`,
				hostID, r.PID, r.Name, r.CmdLine, r.Port, r.Protocol, r.BindAddr, assetID, now, now); err != nil {
				errs = append(errs, fmt.Errorf("insert service %s/%d: %w", r.Protocol, r.Port, err))
			}
		}
	}

	for key, ex := range existingMap {
		if !reportedMap[key] && ex.Status == "running" {
			if _, err := s.db.Exec("UPDATE discovered_services SET status='stopped', last_seen=? WHERE id=?", now, ex.ID); err != nil {
				errs = append(errs, fmt.Errorf("mark stopped %d: %w", ex.ID, err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("sync services: %d errors, first: %w", len(errs), errs[0])
	}
	return nil
}

func (s *DiscoveredServiceStore) ListByHost(hostID string) ([]DiscoveredService, error) {
	rows, err := s.db.Query(`SELECT id, host_id, pid, name, cmd_line, port, protocol, bind_addr, status, asset_id, first_seen, last_seen
		FROM discovered_services WHERE host_id=? ORDER BY port`, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDiscoveredRows(rows)
}

func (s *DiscoveredServiceStore) ListAll() (map[string][]DiscoveredService, error) {
	rows, err := s.db.Query(`SELECT id, host_id, pid, name, cmd_line, port, protocol, bind_addr, status, asset_id, first_seen, last_seen
		FROM discovered_services ORDER BY host_id, port`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := scanDiscoveredRows(rows)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]DiscoveredService)
	for _, d := range list {
		result[d.HostID] = append(result[d.HostID], d)
	}
	return result, nil
}

func scanDiscoveredRows(rows *sql.Rows) ([]DiscoveredService, error) {
	var list []DiscoveredService
	for rows.Next() {
		var d DiscoveredService
		var assetID sql.NullInt64
		if err := rows.Scan(&d.ID, &d.HostID, &d.PID, &d.Name, &d.CmdLine, &d.Port, &d.Protocol, &d.BindAddr, &d.Status, &assetID, &d.FirstSeen, &d.LastSeen); err != nil {
			return nil, err
		}
		if assetID.Valid {
			aid := int(assetID.Int64)
			d.AssetID = &aid
		}
		list = append(list, d)
	}
	return list, nil
}

func portProtoKey(port int, protocol string) string {
	return strconv.Itoa(port) + ":" + protocol
}

func matchPort(assetPort string, discoveredPort int) bool {
	dp := strconv.Itoa(discoveredPort)
	for _, p := range strings.Split(assetPort, ",") {
		if strings.TrimSpace(p) == dp {
			return true
		}
	}
	return false
}
