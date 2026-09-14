package db

import (
	"database/sql"
	"errors"
	"time"
)

// PanelUser — VPN-клиент панели (не путать с admin-пользователем из users).
// Креды пользователя вшиваются во все entry-inbound его графа.
type PanelUser struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	GraphID   string `json:"graph_id"`
	GraphName string `json:"graph_name,omitempty"`
	UUID      string `json:"uuid"`
	Password  string `json:"password"`
	Flow      string `json:"flow"`
	Remark    string `json:"remark"`
	SubToken  string `json:"sub_token"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

const panelUserCols = `id, name, graph_id, uuid, password, flow, remark, sub_token, enabled, created_at, updated_at`

func (s *Store) CreatePanelUser(name, graphID string) (PanelUser, error) {
	u := PanelUser{
		ID:       mustUUID(),
		Name:     name,
		GraphID:  graphID,
		UUID:     mustUUID(),
		Password: mustToken(24),
		SubToken: mustToken(32),
		Enabled:  true,
	}
	now := time.Now().Unix()
	u.CreatedAt, u.UpdatedAt = now, now
	_, err := s.db.Exec(`INSERT INTO panel_users (id, name, graph_id, uuid, password, flow, remark, sub_token, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Name, u.GraphID, u.UUID, u.Password, u.Flow, u.Remark, u.SubToken, 1, u.CreatedAt, u.UpdatedAt)
	return u, err
}

func (s *Store) ListPanelUsers() ([]PanelUser, error) {
	rows, err := s.db.Query(`SELECT pu.id, pu.name, pu.graph_id, g.name, pu.uuid, pu.password, pu.flow, pu.remark, pu.sub_token, pu.enabled, pu.created_at, pu.updated_at
		FROM panel_users pu LEFT JOIN graphs g ON g.id = pu.graph_id ORDER BY pu.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PanelUser, 0)
	for rows.Next() {
		u, err := scanPanelUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) GetPanelUser(id string) (PanelUser, error) {
	row := s.db.QueryRow(`SELECT pu.id, pu.name, pu.graph_id, g.name, pu.uuid, pu.password, pu.flow, pu.remark, pu.sub_token, pu.enabled, pu.created_at, pu.updated_at
		FROM panel_users pu LEFT JOIN graphs g ON g.id = pu.graph_id WHERE pu.id = ?`, id)
	return scanPanelUser(row)
}

func (s *Store) GetPanelUserBySubToken(token string) (PanelUser, error) {
	row := s.db.QueryRow(`SELECT pu.id, pu.name, pu.graph_id, g.name, pu.uuid, pu.password, pu.flow, pu.remark, pu.sub_token, pu.enabled, pu.created_at, pu.updated_at
		FROM panel_users pu LEFT JOIN graphs g ON g.id = pu.graph_id WHERE pu.sub_token = ?`, token)
	return scanPanelUser(row)
}

// UpdatePanelUser обновляет изменяемые поля (name/remark/flow/enabled).
func (s *Store) UpdatePanelUser(u PanelUser) error {
	_, err := s.db.Exec(`UPDATE panel_users SET name = ?, remark = ?, flow = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		u.Name, u.Remark, u.Flow, btoi(u.Enabled), time.Now().Unix(), u.ID)
	return err
}

// MovePanelUser перепривязывает пользователя на другой граф.
func (s *Store) MovePanelUser(id, graphID string) error {
	_, err := s.db.Exec(`UPDATE panel_users SET graph_id = ?, updated_at = ? WHERE id = ?`,
		graphID, time.Now().Unix(), id)
	return err
}

func (s *Store) DeletePanelUser(id string) error {
	_, err := s.db.Exec(`DELETE FROM panel_users WHERE id = ?`, id)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPanelUser(row rowScanner) (PanelUser, error) {
	var u PanelUser
	var enabled int
	err := row.Scan(&u.ID, &u.Name, &u.GraphID, &u.GraphName, &u.UUID, &u.Password, &u.Flow, &u.Remark, &u.SubToken, &enabled, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	u.Enabled = enabled == 1
	return u, err
}
