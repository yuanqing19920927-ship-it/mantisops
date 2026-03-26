package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"opsboard/server/internal/model"
)

type ServerStore struct {
	db *sql.DB
}

func NewServerStore(db *sql.DB) *ServerStore {
	return &ServerStore{db: db}
}

func (s *ServerStore) Upsert(srv *model.Server) error {
	_, err := s.db.Exec(`
		INSERT INTO servers (host_id, hostname, ip_addresses, os, kernel, arch,
			agent_version, cpu_cores, cpu_model, memory_total, disk_total,
			gpu_model, gpu_memory, boot_time, last_seen, status, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'online', CURRENT_TIMESTAMP)
		ON CONFLICT(host_id) DO UPDATE SET
			hostname=excluded.hostname, ip_addresses=excluded.ip_addresses,
			os=excluded.os, kernel=excluded.kernel, arch=excluded.arch,
			agent_version=excluded.agent_version, cpu_cores=excluded.cpu_cores,
			cpu_model=excluded.cpu_model, memory_total=excluded.memory_total,
			disk_total=excluded.disk_total, gpu_model=excluded.gpu_model,
			gpu_memory=excluded.gpu_memory, boot_time=excluded.boot_time,
			last_seen=excluded.last_seen, status='online',
			updated_at=CURRENT_TIMESTAMP`,
		srv.HostID, srv.Hostname, srv.IPAddresses, srv.OS, srv.Kernel, srv.Arch,
		srv.AgentVersion, srv.CPUCores, srv.CPUModel, srv.MemoryTotal, srv.DiskTotal,
		srv.GPUModel, srv.GPUMemory, srv.BootTime, time.Now().Unix())
	return err
}

func (s *ServerStore) UpdateLastSeen(hostID string) error {
	_, err := s.db.Exec(
		"UPDATE servers SET last_seen=?, status='online', updated_at=CURRENT_TIMESTAMP WHERE host_id=?",
		time.Now().Unix(), hostID)
	return err
}

func (s *ServerStore) List() ([]model.Server, error) {
	rows, err := s.db.Query(`SELECT id, host_id, hostname, COALESCE(ip_addresses,''),
		COALESCE(os,''), COALESCE(kernel,''), COALESCE(arch,''), COALESCE(agent_version,''),
		COALESCE(cpu_cores,0), COALESCE(cpu_model,''), COALESCE(memory_total,0),
		COALESCE(disk_total,0), COALESCE(gpu_model,''), COALESCE(gpu_memory,0),
		COALESCE(boot_time,0), COALESCE(last_seen,0), COALESCE(status,'online'),
		COALESCE(display_name,''), COALESCE(sort_order,0), group_id
		FROM servers ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var servers []model.Server
	for rows.Next() {
		var srv model.Server
		var groupID sql.NullInt64
		if err := rows.Scan(&srv.ID, &srv.HostID, &srv.Hostname, &srv.IPAddresses,
			&srv.OS, &srv.Kernel, &srv.Arch, &srv.AgentVersion,
			&srv.CPUCores, &srv.CPUModel, &srv.MemoryTotal, &srv.DiskTotal,
			&srv.GPUModel, &srv.GPUMemory, &srv.BootTime, &srv.LastSeen,
			&srv.Status, &srv.DisplayName, &srv.SortOrder, &groupID); err != nil {
			return nil, err
		}
		if groupID.Valid {
			gid := int(groupID.Int64)
			srv.GroupID = &gid
		}
		servers = append(servers, srv)
	}
	return servers, nil
}

func (s *ServerStore) GetByHostID(hostID string) (*model.Server, error) {
	var srv model.Server
	row := s.db.QueryRow(`SELECT id, host_id, hostname, COALESCE(ip_addresses,''),
		COALESCE(os,''), COALESCE(kernel,''), COALESCE(arch,''), COALESCE(agent_version,''),
		COALESCE(cpu_cores,0), COALESCE(cpu_model,''), COALESCE(memory_total,0),
		COALESCE(disk_total,0), COALESCE(gpu_model,''), COALESCE(gpu_memory,0),
		COALESCE(boot_time,0), COALESCE(last_seen,0), COALESCE(status,'online'),
		COALESCE(display_name,''), COALESCE(sort_order,0), group_id
		FROM servers WHERE host_id=?`, hostID)
	var groupID sql.NullInt64
	err := row.Scan(
		&srv.ID, &srv.HostID, &srv.Hostname, &srv.IPAddresses,
		&srv.OS, &srv.Kernel, &srv.Arch, &srv.AgentVersion,
		&srv.CPUCores, &srv.CPUModel, &srv.MemoryTotal, &srv.DiskTotal,
		&srv.GPUModel, &srv.GPUMemory, &srv.BootTime, &srv.LastSeen,
		&srv.Status, &srv.DisplayName, &srv.SortOrder, &groupID)
	if err != nil {
		return nil, err
	}
	if groupID.Valid {
		gid := int(groupID.Int64)
		srv.GroupID = &gid
	}
	return &srv, nil
}

func (s *ServerStore) UpdateDisplayName(hostID, displayName string) error {
	_, err := s.db.Exec("UPDATE servers SET display_name=?, updated_at=CURRENT_TIMESTAMP WHERE host_id=?", displayName, hostID)
	return err
}

func (s *ServerStore) MarkOffline(timeoutSec int64) error {
	threshold := time.Now().Unix() - timeoutSec
	_, err := s.db.Exec("UPDATE servers SET status='offline' WHERE last_seen < ? AND status='online'", threshold)
	return err
}

func (s *ServerStore) SetGroupID(hostID string, groupID *int) error {
	_, err := s.db.Exec("UPDATE servers SET group_id=? WHERE host_id=?", groupID, hostID)
	return err
}

func IPListToJSON(ips []string) string {
	data, _ := json.Marshal(ips)
	return string(data)
}
