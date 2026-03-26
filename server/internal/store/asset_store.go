package store

import (
	"database/sql"
	"opsboard/server/internal/model"
)

type AssetStore struct {
	db *sql.DB
}

func NewAssetStore(db *sql.DB) *AssetStore {
	return &AssetStore{db: db}
}

func (s *AssetStore) List() ([]model.Asset, error) {
	rows, err := s.db.Query(`SELECT id, server_id, name, COALESCE(category,''), COALESCE(description,''), COALESCE(tech_stack,''), COALESCE(path,''), COALESCE(port,''), COALESCE(status,'active'), COALESCE(extra_info,'') FROM assets ORDER BY server_id, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var assets []model.Asset
	for rows.Next() {
		var a model.Asset
		if err := rows.Scan(&a.ID, &a.ServerID, &a.Name, &a.Category, &a.Description, &a.TechStack, &a.Path, &a.Port, &a.Status, &a.ExtraInfo); err != nil {
			return nil, err
		}
		assets = append(assets, a)
	}
	return assets, nil
}

func (s *AssetStore) ListByServer(serverID int) ([]model.Asset, error) {
	rows, err := s.db.Query(`SELECT id, server_id, name, COALESCE(category,''), COALESCE(description,''), COALESCE(tech_stack,''), COALESCE(path,''), COALESCE(port,''), COALESCE(status,'active'), COALESCE(extra_info,'') FROM assets WHERE server_id=? ORDER BY id`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var assets []model.Asset
	for rows.Next() {
		var a model.Asset
		if err := rows.Scan(&a.ID, &a.ServerID, &a.Name, &a.Category, &a.Description, &a.TechStack, &a.Path, &a.Port, &a.Status, &a.ExtraInfo); err != nil {
			return nil, err
		}
		assets = append(assets, a)
	}
	return assets, nil
}

func (s *AssetStore) Create(a *model.Asset) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO assets (server_id, name, category, description, tech_stack, path, port, status, extra_info)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ServerID, a.Name, a.Category, a.Description, a.TechStack, a.Path, a.Port, a.Status, a.ExtraInfo)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *AssetStore) Update(a *model.Asset) error {
	_, err := s.db.Exec(
		`UPDATE assets SET name=?, category=?, description=?, tech_stack=?, path=?, port=?, status=?, extra_info=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		a.Name, a.Category, a.Description, a.TechStack, a.Path, a.Port, a.Status, a.ExtraInfo, a.ID)
	return err
}

func (s *AssetStore) Delete(id int) error {
	_, err := s.db.Exec("DELETE FROM assets WHERE id=?", id)
	return err
}
