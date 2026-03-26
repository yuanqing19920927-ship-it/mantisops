package store

import (
	"database/sql"
	"opsboard/server/internal/model"
)

type ProbeStore struct {
	db *sql.DB
}

func NewProbeStore(db *sql.DB) *ProbeStore {
	return &ProbeStore{db: db}
}

func (s *ProbeStore) Create(rule *model.ProbeRule) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO probe_rules (server_id, name, host, port, protocol, interval_sec, timeout_sec, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ServerID, rule.Name, rule.Host, rule.Port, rule.Protocol,
		rule.IntervalSec, rule.TimeoutSec, rule.Enabled)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *ProbeStore) List() ([]model.ProbeRule, error) {
	rows, err := s.db.Query(`SELECT id, server_id, name, host, port, COALESCE(protocol,'tcp'), COALESCE(interval_sec,30), COALESCE(timeout_sec,5), enabled FROM probe_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []model.ProbeRule
	for rows.Next() {
		var r model.ProbeRule
		if err := rows.Scan(&r.ID, &r.ServerID, &r.Name, &r.Host, &r.Port, &r.Protocol, &r.IntervalSec, &r.TimeoutSec, &r.Enabled); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func (s *ProbeStore) ListEnabled() ([]model.ProbeRule, error) {
	rows, err := s.db.Query(`SELECT id, server_id, name, host, port, COALESCE(protocol,'tcp'), COALESCE(interval_sec,30), COALESCE(timeout_sec,5), enabled FROM probe_rules WHERE enabled=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []model.ProbeRule
	for rows.Next() {
		var r model.ProbeRule
		if err := rows.Scan(&r.ID, &r.ServerID, &r.Name, &r.Host, &r.Port, &r.Protocol, &r.IntervalSec, &r.TimeoutSec, &r.Enabled); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func (s *ProbeStore) Update(rule *model.ProbeRule) error {
	_, err := s.db.Exec(
		`UPDATE probe_rules SET name=?, host=?, port=?, protocol=?, interval_sec=?, timeout_sec=?, enabled=? WHERE id=?`,
		rule.Name, rule.Host, rule.Port, rule.Protocol, rule.IntervalSec, rule.TimeoutSec, rule.Enabled, rule.ID)
	return err
}

func (s *ProbeStore) Delete(id int) error {
	_, err := s.db.Exec("DELETE FROM probe_rules WHERE id=?", id)
	return err
}
