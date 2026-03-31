package store

import (
	"database/sql"
	"time"
)

// NetworkSubnet represents a scanned subnet.
type NetworkSubnet struct {
	ID         int        `json:"id"`
	CIDR       string     `json:"cidr"`
	Name       string     `json:"name"`
	Gateway    string     `json:"gateway"`
	TotalHosts int        `json:"total_hosts"`
	AliveHosts int        `json:"alive_hosts"`
	LastScan   *time.Time `json:"last_scan"`
	CreatedAt  time.Time  `json:"created_at"`
}

// NetworkDevice represents a device discovered on the network.
type NetworkDevice struct {
	ID               int        `json:"id"`
	IP               string     `json:"ip"`
	MAC              string     `json:"mac"`
	Vendor           string     `json:"vendor"`
	DeviceType       string     `json:"device_type"`
	Hostname         string     `json:"hostname"`
	SNMPSupported    bool       `json:"snmp_supported"`
	SNMPCredentialID int        `json:"snmp_credential_id"`
	SysDescr         string     `json:"sys_descr"`
	SysName          string     `json:"sys_name"`
	SysObjectID      string     `json:"sys_object_id"`
	Model            string     `json:"model"`
	SubnetID         *int       `json:"subnet_id"`
	Status           string     `json:"status"`
	LastSeen         *time.Time `json:"last_seen"`
	FirstSeen        time.Time  `json:"first_seen"`
	ServerID         int        `json:"server_id"`
	CreatedAt        time.Time  `json:"created_at"`
}

// NetworkLink represents a layer-2 link between two network devices.
type NetworkLink struct {
	ID         int        `json:"id"`
	SourceID   int        `json:"source_id"`
	TargetID   int        `json:"target_id"`
	SourcePort string     `json:"source_port"`
	TargetPort string     `json:"target_port"`
	Protocol   string     `json:"protocol"`
	Bandwidth  string     `json:"bandwidth"`
	LastSeen   *time.Time `json:"last_seen"`
	CreatedAt  time.Time  `json:"created_at"`
}

// TopologyData bundles all devices and links for the topology view.
type TopologyData struct {
	Devices []NetworkDevice `json:"devices"`
	Links   []NetworkLink   `json:"links"`
}

// NetworkStore provides CRUD access to the network topology tables.
type NetworkStore struct {
	db *sql.DB
}

// NewNetworkStore constructs a NetworkStore backed by db.
func NewNetworkStore(db *sql.DB) *NetworkStore {
	return &NetworkStore{db: db}
}

// ---- Subnets ----------------------------------------------------------------

// ListSubnets returns all subnets ordered by id.
// total_hosts and alive_hosts are computed dynamically from the devices table.
func (s *NetworkStore) ListSubnets() ([]NetworkSubnet, error) {
	rows, err := s.db.Query(`
		SELECT s.id, s.cidr, s.name, s.gateway,
		       COALESCE(d.total, 0) AS total_hosts,
		       COALESCE(d.alive, 0) AS alive_hosts,
		       s.last_scan, s.created_at
		FROM network_subnets s
		LEFT JOIN (
			SELECT subnet_id,
			       COUNT(*) AS total,
			       SUM(CASE WHEN status = 'online' THEN 1 ELSE 0 END) AS alive
			FROM network_devices
			GROUP BY subnet_id
		) d ON d.subnet_id = s.id
		ORDER BY s.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []NetworkSubnet{}
	for rows.Next() {
		var sub NetworkSubnet
		var lastScan sql.NullString
		if err := rows.Scan(
			&sub.ID, &sub.CIDR, &sub.Name, &sub.Gateway,
			&sub.TotalHosts, &sub.AliveHosts, &lastScan, &sub.CreatedAt,
		); err != nil {
			return nil, err
		}
		if lastScan.Valid {
			if t, err := time.Parse(time.RFC3339, lastScan.String); err == nil {
				sub.LastScan = &t
			}
		}
		list = append(list, sub)
	}
	return list, nil
}

// UpsertSubnet inserts or updates a subnet row identified by cidr.
// Returns the row id.
func (s *NetworkStore) UpsertSubnet(cidr, name, gateway string, totalHosts, aliveHosts int) (int, error) {
	now := time.Now().Format(time.RFC3339)
	res, err := s.db.Exec(`
		INSERT INTO network_subnets (cidr, name, gateway, total_hosts, alive_hosts, last_scan)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(cidr) DO UPDATE SET
			name        = excluded.name,
			gateway     = excluded.gateway,
			total_hosts = excluded.total_hosts,
			alive_hosts = excluded.alive_hosts,
			last_scan   = excluded.last_scan
	`, cidr, name, gateway, totalHosts, aliveHosts, now)
	if err != nil {
		return 0, err
	}
	// ON CONFLICT UPDATE does not change LastInsertId reliably; look it up.
	id, err := res.LastInsertId()
	if err != nil || id == 0 {
		if err2 := s.db.QueryRow(`SELECT id FROM network_subnets WHERE cidr = ?`, cidr).Scan(&id); err2 != nil {
			return 0, err2
		}
	}
	return int(id), nil
}

// ---- Devices ----------------------------------------------------------------

// ListDevices returns devices. If subnetID > 0 it filters by subnet.
func (s *NetworkStore) ListDevices(subnetID int) ([]NetworkDevice, error) {
	query := `
		SELECT id, ip, mac, vendor, device_type, hostname,
		       snmp_supported, snmp_credential_id,
		       sys_descr, sys_name, sys_object_id, model,
		       subnet_id, status, last_seen, first_seen, server_id, created_at
		FROM network_devices`
	var args []interface{}
	if subnetID > 0 {
		query += " WHERE subnet_id = ?"
		args = append(args, subnetID)
	}
	query += " ORDER BY id"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []NetworkDevice{}
	for rows.Next() {
		d, err := scanNetworkDevice(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *d)
	}
	return list, nil
}

// GetDevice returns a single device by id.
func (s *NetworkStore) GetDevice(id int) (*NetworkDevice, error) {
	row := s.db.QueryRow(`
		SELECT id, ip, mac, vendor, device_type, hostname,
		       snmp_supported, snmp_credential_id,
		       sys_descr, sys_name, sys_object_id, model,
		       subnet_id, status, last_seen, first_seen, server_id, created_at
		FROM network_devices WHERE id = ?
	`, id)
	return scanNetworkDevice(row)
}

// GetAllDevices returns all devices regardless of status.
func (s *NetworkStore) GetAllDevices() ([]NetworkDevice, error) {
	return s.ListDevices(0)
}


// UpsertDevice inserts or updates a device row identified by ip.
// Business rules applied on conflict:
//   - preserve existing device_type when the incoming value is "unknown"
//   - preserve existing snmp_credential_id when the incoming value is 0
//
// Returns the row id.
func (s *NetworkStore) UpsertDevice(
	ip, mac, vendor, deviceType, hostname string,
	snmpSupported bool, snmpCredID int,
	sysDescr, sysName, sysObjID, model string,
	subnetID, serverID int,
) (int, error) {
	now := time.Now().Format(time.RFC3339)

	var subnetArg interface{}
	if subnetID > 0 {
		subnetArg = subnetID
	}

	res, err := s.db.Exec(`
		INSERT INTO network_devices
			(ip, mac, vendor, device_type, hostname,
			 snmp_supported, snmp_credential_id,
			 sys_descr, sys_name, sys_object_id, model,
			 subnet_id, status, last_seen, server_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'online', ?, ?)
		ON CONFLICT(ip) DO UPDATE SET
			mac               = excluded.mac,
			vendor            = excluded.vendor,
			device_type       = CASE WHEN excluded.device_type = 'unknown'
			                         THEN device_type
			                         ELSE excluded.device_type END,
			hostname          = excluded.hostname,
			snmp_supported    = excluded.snmp_supported,
			snmp_credential_id = CASE WHEN excluded.snmp_credential_id = 0
			                          THEN snmp_credential_id
			                          ELSE excluded.snmp_credential_id END,
			sys_descr         = excluded.sys_descr,
			sys_name          = excluded.sys_name,
			sys_object_id     = excluded.sys_object_id,
			model             = excluded.model,
			subnet_id         = excluded.subnet_id,
			status            = 'online',
			last_seen         = excluded.last_seen,
			server_id         = excluded.server_id
	`, ip, mac, vendor, deviceType, hostname,
		snmpSupported, snmpCredID,
		sysDescr, sysName, sysObjID, model,
		subnetArg, now, serverID)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil || id == 0 {
		if err2 := s.db.QueryRow(`SELECT id FROM network_devices WHERE ip = ?`, ip).Scan(&id); err2 != nil {
			return 0, err2
		}
	}
	return int(id), nil
}

// UpdateDevice updates mutable user-facing fields of a device.
func (s *NetworkStore) UpdateDevice(id int, deviceType, hostname string) error {
	_, err := s.db.Exec(`
		UPDATE network_devices SET device_type = ?, hostname = ? WHERE id = ?
	`, deviceType, hostname, id)
	return err
}

// UpdateDeviceStatus sets the status and refreshes last_seen.
func (s *NetworkStore) UpdateDeviceStatus(id int, status string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(`
		UPDATE network_devices SET status = ?, last_seen = ? WHERE id = ?
	`, status, now, id)
	return err
}

// DeleteDevice removes a device by id.
func (s *NetworkStore) DeleteDevice(id int) error {
	_, err := s.db.Exec("DELETE FROM network_devices WHERE id = ?", id)
	return err
}

// ---- Links ------------------------------------------------------------------

// UpsertLink inserts or updates a link. source_id is always stored as the
// smaller id to prevent duplicate (A→B) / (B→A) pairs.
func (s *NetworkStore) UpsertLink(sourceID, targetID int, sourcePort, targetPort, protocol, bandwidth string) error {
	// Normalise direction: smaller id is always source.
	if sourceID > targetID {
		sourceID, targetID = targetID, sourceID
		sourcePort, targetPort = targetPort, sourcePort
	}
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO network_links
			(source_id, target_id, source_port, target_port, protocol, bandwidth, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id, target_id, source_port) DO UPDATE SET
			target_port = excluded.target_port,
			protocol    = excluded.protocol,
			bandwidth   = excluded.bandwidth,
			last_seen   = excluded.last_seen
	`, sourceID, targetID, sourcePort, targetPort, protocol, bandwidth, now)
	return err
}

// ListLinks returns all links ordered by id.
func (s *NetworkStore) ListLinks() ([]NetworkLink, error) {
	rows, err := s.db.Query(`
		SELECT id, source_id, target_id, source_port, target_port,
		       protocol, bandwidth, last_seen, created_at
		FROM network_links ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []NetworkLink{}
	for rows.Next() {
		lk, err := scanNetworkLink(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *lk)
	}
	return list, nil
}

// GetTopology returns all devices and all links for the topology view.
func (s *NetworkStore) GetTopology() (*TopologyData, error) {
	devices, err := s.GetAllDevices()
	if err != nil {
		return nil, err
	}
	links, err := s.ListLinks()
	if err != nil {
		return nil, err
	}
	return &TopologyData{Devices: devices, Links: links}, nil
}

// ---- scan helpers -----------------------------------------------------------

func scanNetworkDevice(row rowScanner) (*NetworkDevice, error) {
	var d NetworkDevice
	var lastSeen, firstSeen sql.NullString
	var createdAt sql.NullString
	var subnetID sql.NullInt64
	if err := row.Scan(
		&d.ID, &d.IP, &d.MAC, &d.Vendor, &d.DeviceType, &d.Hostname,
		&d.SNMPSupported, &d.SNMPCredentialID,
		&d.SysDescr, &d.SysName, &d.SysObjectID, &d.Model,
		&subnetID, &d.Status, &lastSeen, &firstSeen, &d.ServerID, &createdAt,
	); err != nil {
		return nil, err
	}
	if subnetID.Valid {
		v := int(subnetID.Int64)
		d.SubnetID = &v
	}
	if lastSeen.Valid {
		if t, err := time.Parse(time.RFC3339, lastSeen.String); err == nil {
			d.LastSeen = &t
		}
	}
	if firstSeen.Valid {
		if t, err := time.Parse(time.RFC3339, firstSeen.String); err == nil {
			d.FirstSeen = t
		}
	}
	if createdAt.Valid {
		if t, err := time.Parse(time.RFC3339, createdAt.String); err == nil {
			d.CreatedAt = t
		}
	}
	return &d, nil
}

func scanNetworkLink(row rowScanner) (*NetworkLink, error) {
	var lk NetworkLink
	var lastSeen, createdAt sql.NullString
	if err := row.Scan(
		&lk.ID, &lk.SourceID, &lk.TargetID,
		&lk.SourcePort, &lk.TargetPort,
		&lk.Protocol, &lk.Bandwidth,
		&lastSeen, &createdAt,
	); err != nil {
		return nil, err
	}
	if lastSeen.Valid {
		if t, err := time.Parse(time.RFC3339, lastSeen.String); err == nil {
			lk.LastSeen = &t
		}
	}
	if createdAt.Valid {
		if t, err := time.Parse(time.RFC3339, createdAt.String); err == nil {
			lk.CreatedAt = t
		}
	}
	return &lk, nil
}
