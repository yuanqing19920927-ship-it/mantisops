package store

import (
	"database/sql"
	"mantisops/server/internal/model"
)

type ProbeStore struct {
	db *sql.DB
}

func NewProbeStore(db *sql.DB) *ProbeStore {
	return &ProbeStore{db: db}
}

func (s *ProbeStore) Create(rule *model.ProbeRule) (int64, error) {
	src := rule.Source
	if src == "" {
		src = "manual"
	}
	result, err := s.db.Exec(
		`INSERT INTO probe_rules (server_id, name, host, port, protocol, url, expect_status, expect_body, interval_sec, timeout_sec, enabled, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ServerID, rule.Name, rule.Host, rule.Port, rule.Protocol,
		rule.URL, rule.ExpectStatus, rule.ExpectBody,
		rule.IntervalSec, rule.TimeoutSec, rule.Enabled, src)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *ProbeStore) List() ([]model.ProbeRule, error) {
	rows, err := s.db.Query(`SELECT id, server_id, name, host, port,
		COALESCE(protocol,'tcp'), COALESCE(url,''), COALESCE(expect_status,200),
		COALESCE(expect_body,''), COALESCE(interval_sec,30), COALESCE(timeout_sec,5), enabled, COALESCE(source,'manual')
		FROM probe_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []model.ProbeRule
	for rows.Next() {
		var r model.ProbeRule
		var serverID sql.NullInt64
		if err := rows.Scan(&r.ID, &serverID, &r.Name, &r.Host, &r.Port,
			&r.Protocol, &r.URL, &r.ExpectStatus, &r.ExpectBody,
			&r.IntervalSec, &r.TimeoutSec, &r.Enabled, &r.Source); err != nil {
			return nil, err
		}
		if serverID.Valid {
			sid := int(serverID.Int64)
			r.ServerID = &sid
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func (s *ProbeStore) ListEnabled() ([]model.ProbeRule, error) {
	rows, err := s.db.Query(`SELECT id, server_id, name, host, port,
		COALESCE(protocol,'tcp'), COALESCE(url,''), COALESCE(expect_status,200),
		COALESCE(expect_body,''), COALESCE(interval_sec,30), COALESCE(timeout_sec,5), enabled, COALESCE(source,'manual')
		FROM probe_rules WHERE enabled=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []model.ProbeRule
	for rows.Next() {
		var r model.ProbeRule
		var serverID sql.NullInt64
		if err := rows.Scan(&r.ID, &serverID, &r.Name, &r.Host, &r.Port,
			&r.Protocol, &r.URL, &r.ExpectStatus, &r.ExpectBody,
			&r.IntervalSec, &r.TimeoutSec, &r.Enabled, &r.Source); err != nil {
			return nil, err
		}
		if serverID.Valid {
			sid := int(serverID.Int64)
			r.ServerID = &sid
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func (s *ProbeStore) Update(rule *model.ProbeRule) error {
	_, err := s.db.Exec(
		`UPDATE probe_rules SET server_id=?, name=?, host=?, port=?, protocol=?, url=?, expect_status=?, expect_body=?, interval_sec=?, timeout_sec=?, enabled=? WHERE id=?`,
		rule.ServerID, rule.Name, rule.Host, rule.Port, rule.Protocol,
		rule.URL, rule.ExpectStatus, rule.ExpectBody,
		rule.IntervalSec, rule.TimeoutSec, rule.Enabled, rule.ID)
	return err
}

func (s *ProbeStore) Delete(id int) error {
	_, err := s.db.Exec("DELETE FROM probe_rules WHERE id=?", id)
	return err
}
