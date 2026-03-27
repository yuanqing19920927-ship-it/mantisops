package store

import (
	"database/sql"

	"mantisops/server/internal/model"
)

type GroupStore struct {
	db *sql.DB
}

func NewGroupStore(db *sql.DB) *GroupStore {
	return &GroupStore{db: db}
}

func (s *GroupStore) List() ([]model.ServerGroup, error) {
	rows, err := s.db.Query(`
		SELECT g.id, g.name, g.sort_order, g.created_at, COUNT(sv.id) as server_count
		FROM server_groups g
		LEFT JOIN servers sv ON sv.group_id = g.id
		GROUP BY g.id ORDER BY g.sort_order, g.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []model.ServerGroup
	for rows.Next() {
		var g model.ServerGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.SortOrder, &g.CreatedAt, &g.ServerCount); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func (s *GroupStore) Create(name string) (int64, error) {
	result, err := s.db.Exec(`INSERT INTO server_groups (name) VALUES (?)`, name)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *GroupStore) Update(id int, name string, sortOrder int) error {
	_, err := s.db.Exec(`UPDATE server_groups SET name=?, sort_order=? WHERE id=?`, name, sortOrder, id)
	return err
}

func (s *GroupStore) Delete(id int) error {
	s.db.Exec(`UPDATE servers SET group_id=NULL WHERE group_id=?`, id)
	_, err := s.db.Exec(`DELETE FROM server_groups WHERE id=?`, id)
	return err
}

func (s *GroupStore) ListSimple() ([]model.ServerGroup, error) {
	rows, err := s.db.Query(`SELECT id, name, sort_order FROM server_groups ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []model.ServerGroup
	for rows.Next() {
		var g model.ServerGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.SortOrder); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}
