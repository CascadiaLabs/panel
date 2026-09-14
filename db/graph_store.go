package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/CascadiaLabs/panel/graph"
)

type Graph struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

func (s *Store) CreateGraph(name string) (Graph, error) {
	g := Graph{ID: mustUUID(), Name: name}
	now := time.Now().Unix()
	g.CreatedAt, g.UpdatedAt = now, now
	_, err := s.db.Exec(`INSERT INTO graphs (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		g.ID, g.Name, g.CreatedAt, g.UpdatedAt)
	return g, err
}

func (s *Store) ListGraphs() ([]Graph, error) {
	rows, err := s.db.Query(`SELECT id, name, created_at, updated_at FROM graphs ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	graphs := make([]Graph, 0)
	for rows.Next() {
		var g Graph
		if err := rows.Scan(&g.ID, &g.Name, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		graphs = append(graphs, g)
	}
	return graphs, nil
}

func (s *Store) GetGraph(id string) (Graph, error) {
	var g Graph
	err := s.db.QueryRow(`SELECT id, name, created_at, updated_at FROM graphs WHERE id = ?`, id).
		Scan(&g.ID, &g.Name, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return g, ErrNotFound
	}
	return g, err
}

func (s *Store) RenameGraph(id, name string) error {
	_, err := s.db.Exec(`UPDATE graphs SET name = ?, updated_at = ? WHERE id = ?`, name, time.Now().Unix(), id)
	return err
}

func (s *Store) DeleteGraph(id string) error {
	_, err := s.db.Exec(`DELETE FROM graphs WHERE id = ?`, id)
	return err
}

// SaveGraphState атомарно заменяет элементы и рёбра графа одним payload.
func (s *Store) SaveGraphState(graphID string, state graph.State) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM graph_edges WHERE graph_id = ?`, graphID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM graph_nodes WHERE graph_id = ?`, graphID); err != nil {
		return err
	}

	now := time.Now().Unix()
	for _, n := range state.Nodes {
		settings := "{}"
		if len(n.Settings) > 0 {
			settings = string(n.Settings)
		}
		if _, err := tx.Exec(`INSERT INTO graph_nodes (id, graph_id, node_id, kind, protocol, tag, settings, pos_x, pos_y, entry, "exit", created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			n.ID, graphID, n.NodeID, n.Kind, n.Protocol, n.Tag, settings, n.PosX, n.PosY, btoi(n.Entry), btoi(n.Exit), now, now); err != nil {
			return err
		}
	}
	for _, e := range state.Edges {
		if _, err := tx.Exec(`INSERT INTO graph_edges (id, graph_id, source_id, target_id) VALUES (?, ?, ?, ?)`,
			e.ID, graphID, e.SourceID, e.TargetID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE graphs SET updated_at = ? WHERE id = ?`, now, graphID); err != nil {
		return err
	}
	return tx.Commit()
}

// LoadGraphState возвращает элементы и рёбра графа.
func (s *Store) LoadGraphState(graphID string) (graph.State, error) {
	state := graph.State{Nodes: []graph.Node{}, Edges: []graph.Edge{}}

	rows, err := s.db.Query(`SELECT id, node_id, kind, protocol, tag, settings, pos_x, pos_y, entry, "exit" FROM graph_nodes WHERE graph_id = ? ORDER BY tag`, graphID)
	if err != nil {
		return state, err
	}
	defer rows.Close()
	for rows.Next() {
		var n graph.Node
		var settings string
		var entry, exit int
		if err := rows.Scan(&n.ID, &n.NodeID, &n.Kind, &n.Protocol, &n.Tag, &settings, &n.PosX, &n.PosY, &entry, &exit); err != nil {
			return state, err
		}
		n.Settings = json.RawMessage(settings)
		n.Entry = entry == 1
		n.Exit = exit == 1
		state.Nodes = append(state.Nodes, n)
	}
	if err := rows.Err(); err != nil {
		return state, err
	}

	rows2, err := s.db.Query(`SELECT id, source_id, target_id FROM graph_edges WHERE graph_id = ?`, graphID)
	if err != nil {
		return state, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var e graph.Edge
		if err := rows2.Scan(&e.ID, &e.SourceID, &e.TargetID); err != nil {
			return state, err
		}
		state.Edges = append(state.Edges, e)
	}
	return state, rows2.Err()
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// NormalizeGraphTag чистит тег от символов, которые sing-box использует
// как разделители (запятая и квадратные скобки в rules/inbound-списках).
func NormalizeGraphTag(tag string) string {
	tag = strings.TrimSpace(tag)
	repl := strings.NewReplacer(",", "_", "[", "_", "]", "_")
	return repl.Replace(tag)
}
