package store

import "database/sql"

type ScanTemplate struct {
	ID        int    `json:"id"`
	Port      int    `json:"port"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	SortOrder int    `json:"sort_order"`
}

type ScanTemplateStore struct {
	db *sql.DB
}

func NewScanTemplateStore(db *sql.DB) *ScanTemplateStore {
	return &ScanTemplateStore{db: db}
}

func (s *ScanTemplateStore) List() ([]ScanTemplate, error) {
	rows, err := s.db.Query("SELECT id, port, name, enabled, sort_order FROM scan_templates ORDER BY sort_order, port")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ScanTemplate
	for rows.Next() {
		var t ScanTemplate
		if err := rows.Scan(&t.ID, &t.Port, &t.Name, &t.Enabled, &t.SortOrder); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, nil
}

func (s *ScanTemplateStore) ListEnabled() ([]ScanTemplate, error) {
	rows, err := s.db.Query("SELECT id, port, name, enabled, sort_order FROM scan_templates WHERE enabled=1 ORDER BY sort_order, port")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ScanTemplate
	for rows.Next() {
		var t ScanTemplate
		if err := rows.Scan(&t.ID, &t.Port, &t.Name, &t.Enabled, &t.SortOrder); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, nil
}

func (s *ScanTemplateStore) Create(port int, name string) (int64, error) {
	res, err := s.db.Exec("INSERT INTO scan_templates (port, name, sort_order) VALUES (?, ?, ?)", port, name, port)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *ScanTemplateStore) Update(id int, port int, name string, enabled bool) error {
	_, err := s.db.Exec("UPDATE scan_templates SET port=?, name=?, enabled=? WHERE id=?", port, name, enabled, id)
	return err
}

func (s *ScanTemplateStore) Delete(id int) error {
	_, err := s.db.Exec("DELETE FROM scan_templates WHERE id=?", id)
	return err
}
