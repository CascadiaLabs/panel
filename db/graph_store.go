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
	ID                string `json:"id"`
	Name              string `json:"name"`
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
	SubscriptionName  string `json:"subscription_name"`
	SubscriptionDesc  string `json:"subscription_desc"`
	SubscriptionSite  string `json:"subscription_site"`
	SubscriptionSupport string `json:"subscription_support"`
	ClientRoute       string `json:"client_route"` // JSON-строка с клиентской маршрутизацией
}

type GraphCreate struct {
	Name              string
	SubscriptionName  string
	SubscriptionDesc  string
	SubscriptionSite  string
	SubscriptionSupport string
	ClientRoute       string
}

func (s *Store) CreateGraph(g GraphCreate) (Graph, error) {
	now := time.Now().Unix()
	graph := Graph{
		ID:                mustUUID(),
		Name:              g.Name,
		SubscriptionName:  g.SubscriptionName,
		SubscriptionDesc:  g.SubscriptionDesc,
		SubscriptionSite:  g.SubscriptionSite,
		SubscriptionSupport: g.SubscriptionSupport,
		ClientRoute:       g.ClientRoute,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	_, err := s.db.Exec(`INSERT INTO graphs (id, name, created_at, updated_at, subscription_name, subscription_desc, subscription_site, subscription_support, client_route)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		graph.ID, graph.Name, graph.CreatedAt, graph.UpdatedAt,
		graph.SubscriptionName, graph.SubscriptionDesc, graph.SubscriptionSite, graph.SubscriptionSupport, graph.ClientRoute)
	return graph, err
}

func (s *Store) ListGraphs() ([]Graph, error) {
	rows, err := s.db.Query(`SELECT id, name, created_at, updated_at, subscription_name, subscription_desc, subscription_site, subscription_support, client_route FROM graphs ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	graphs := make([]Graph, 0)
	for rows.Next() {
		var g Graph
		if err := rows.Scan(&g.ID, &g.Name, &g.CreatedAt, &g.UpdatedAt, &g.SubscriptionName, &g.SubscriptionDesc, &g.SubscriptionSite, &g.SubscriptionSupport, &g.ClientRoute); err != nil {
			return nil, err
		}
		graphs = append(graphs, g)
	}
	return graphs, nil
}

func (s *Store) GetGraph(id string) (Graph, error) {
	var g Graph
	err := s.db.QueryRow(`SELECT id, name, created_at, updated_at, subscription_name, subscription_desc, subscription_site, subscription_support, client_route FROM graphs WHERE id = ?`, id).
		Scan(&g.ID, &g.Name, &g.CreatedAt, &g.UpdatedAt, &g.SubscriptionName, &g.SubscriptionDesc, &g.SubscriptionSite, &g.SubscriptionSupport, &g.ClientRoute)
	if errors.Is(err, sql.ErrNoRows) {
		return g, ErrNotFound
	}
	return g, err
}

func (s *Store) RenameGraph(id, name string) error {
	_, err := s.db.Exec(`UPDATE graphs SET name = ?, updated_at = ? WHERE id = ?`, name, time.Now().Unix(), id)
	return err
}

// UpdateGraphSubscription обновляет метаданные подписки и клиентскую маршрутизацию графа.
func (s *Store) UpdateGraphSubscription(id string, sub SubscriptionSettings) error {
	_, err := s.db.Exec(`UPDATE graphs SET subscription_name = ?, subscription_desc = ?, subscription_site = ?, subscription_support = ?, client_route = ?, updated_at = ? WHERE id = ?`,
		sub.SubscriptionName, sub.SubscriptionDesc, sub.SubscriptionSite, sub.SubscriptionSupport, sub.ClientRoute, time.Now().Unix(), id)
	return err
}

type SubscriptionSettings struct {
	SubscriptionName  string
	SubscriptionDesc  string
	SubscriptionSite  string
	SubscriptionSupport string
	ClientRoute       string
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

	state = s.InjectPanelUsers(graphID, state)

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
		// каждому inbound нужен служебный relay-пользователь (для каскадных подключений)
		if n.Kind == graph.KindInbound {
			if in, err := graph.ParseInboundSettings(n.Settings); err == nil && in.RelayUser == nil {
				in.RelayUser = &graph.InboundUser{Name: "relay", UUID: graph.GenerateUUID(), Password: graph.GeneratePassword()}
				if raw, err := json.Marshal(in); err == nil {
					settings = string(raw)
				}
			}
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

// InjectPanelUsers вшивает активных VPN-юзеров графа во все entry-inbound.
// Вызывается при каждом сохранении/деплое графа, чтобы вновь созданные inbound
// сразу содержали клиентов панели (иначе подписка и нода расходятся по uuid).
func (s *Store) InjectPanelUsers(graphID string, state graph.State) graph.State {
	users, err := s.ListPanelUsers()
	if err != nil {
		return state
	}
	for _, u := range users {
		if u.GraphID != graphID || !u.Enabled {
			continue
		}
		state = graph.InjectUsersIntoState(state, graph.PanelCreds{Name: u.Name, UUID: u.UUID, Password: u.Password, Flow: u.Flow, Remark: u.Remark})
	}
	return state
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
