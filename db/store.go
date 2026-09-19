package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound возвращают методы Get*, когда записи нет.
var ErrNotFound = errors.New("not found")

// Схема целиком аддитивная (IF NOT EXISTS): существующие БД обновляются без миграций.
const schema = `
CREATE TABLE IF NOT EXISTS nodes (
	id           TEXT PRIMARY KEY,
	name         TEXT NOT NULL,
	grpc_url     TEXT NOT NULL,
	token        TEXT NOT NULL,
	cert_pem     TEXT NOT NULL,
	config_json  TEXT,
	created_at   INTEGER NOT NULL,
	updated_at   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_nodes_name ON nodes(name);

CREATE TABLE IF NOT EXISTS users (
	id            TEXT PRIMARY KEY,
	username      TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at    INTEGER NOT NULL,
	updated_at    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	id         TEXT PRIMARY KEY,
	user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS graphs (
	id                TEXT PRIMARY KEY,
	name              TEXT NOT NULL UNIQUE,
	created_at        INTEGER NOT NULL,
	updated_at        INTEGER NOT NULL,
	subscription_name TEXT    DEFAULT '',
	subscription_desc TEXT    DEFAULT '',
	subscription_site TEXT    DEFAULT '',
	subscription_support TEXT  DEFAULT '',
	client_route  TEXT    DEFAULT ''
);

CREATE TABLE IF NOT EXISTS graph_nodes (
	id         TEXT PRIMARY KEY,
	graph_id   TEXT NOT NULL REFERENCES graphs(id) ON DELETE CASCADE,
	node_id    TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
	kind       TEXT NOT NULL CHECK (kind IN ('inbound','outbound','rule','balancer')),
	protocol   TEXT NOT NULL,
	tag        TEXT NOT NULL,
	settings   TEXT NOT NULL DEFAULT '{}',
	pos_x      REAL NOT NULL DEFAULT 0,
	pos_y      REAL NOT NULL DEFAULT 0,
	entry      INTEGER NOT NULL DEFAULT 0,
	"exit"     INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_graph_nodes_tag ON graph_nodes(graph_id, tag);
CREATE INDEX IF NOT EXISTS idx_graph_nodes_graph ON graph_nodes(graph_id);
CREATE INDEX IF NOT EXISTS idx_graph_nodes_node ON graph_nodes(node_id);

CREATE TABLE IF NOT EXISTS graph_edges (
	id         TEXT PRIMARY KEY,
	graph_id   TEXT NOT NULL REFERENCES graphs(id) ON DELETE CASCADE,
	source_id  TEXT NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
	target_id  TEXT NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
	CHECK (source_id <> target_id)
);
CREATE INDEX IF NOT EXISTS idx_graph_edges_graph ON graph_edges(graph_id);

-- Правила маршрутизации: создаются отдельно и назначаются на конкретные inbounds.
-- graph_id = '' означает глобальное правило (доступно для всех графов).
CREATE TABLE IF NOT EXISTS route_rules (
	id          TEXT PRIMARY KEY,
	graph_id    TEXT NOT NULL DEFAULT '',
	name        TEXT NOT NULL,
	rules_json  TEXT NOT NULL DEFAULT '[]',
	is_default  INTEGER NOT NULL DEFAULT 0,
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_route_rules_graph ON route_rules(graph_id);

-- Маппинг: inbound → route_rule (один inbound может иметь несколько правил).
-- Без ON DELETE CASCADE, чтобы сохранять назначения при пересохранении графа (DELETE+INSERT в SaveGraphState).
CREATE TABLE IF NOT EXISTS inbound_routes (
	id           TEXT PRIMARY KEY,
	inbound_id   TEXT NOT NULL REFERENCES graph_nodes(id),
	route_rule_id TEXT NOT NULL REFERENCES route_rules(id)
);
CREATE INDEX IF NOT EXISTS idx_inbound_routes_inbound ON inbound_routes(inbound_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_inbound_routes_unique ON inbound_routes(inbound_id, route_rule_id);

-- VPN-пользователи: креды вшиваются в entry-inbound графа; подписка
-- /sub/{sub_token} отдаёт все entry-inbound как v2ray share-links.
CREATE TABLE IF NOT EXISTS panel_users (
	id           TEXT PRIMARY KEY,
	name         TEXT NOT NULL,
	graph_id     TEXT NOT NULL REFERENCES graphs(id) ON DELETE CASCADE,
	uuid         TEXT NOT NULL,
	password     TEXT NOT NULL,
	flow         TEXT NOT NULL DEFAULT '',
	remark       TEXT NOT NULL DEFAULT '',
	sub_token    TEXT NOT NULL UNIQUE,
	enabled      INTEGER NOT NULL DEFAULT 1,
	used_upload  INTEGER NOT NULL DEFAULT 0,
	used_download INTEGER NOT NULL DEFAULT 0,
	total_traffic INTEGER NOT NULL DEFAULT 0,
	expire_time  INTEGER NOT NULL DEFAULT 0,
	created_at   INTEGER NOT NULL,
	updated_at   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_panel_users_graph ON panel_users(graph_id);
CREATE INDEX IF NOT EXISTS idx_panel_users_sub ON panel_users(sub_token);
CREATE INDEX IF NOT EXISTS idx_graph_edges_source ON graph_edges(source_id);
CREATE INDEX IF NOT EXISTS idx_graph_edges_target ON graph_edges(target_id);
`

type Store struct {
	db *sql.DB
}

func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	// Аддитивная миграция существующих таблиц
	migrations := []string{
		`ALTER TABLE panel_users ADD COLUMN used_upload INTEGER DEFAULT 0`,
		`ALTER TABLE panel_users ADD COLUMN used_download INTEGER DEFAULT 0`,
		`ALTER TABLE panel_users ADD COLUMN total_traffic INTEGER DEFAULT 0`,
		`ALTER TABLE panel_users ADD COLUMN expire_time INTEGER DEFAULT 0`,
		`ALTER TABLE graphs ADD COLUMN subscription_name TEXT DEFAULT ''`,
		`ALTER TABLE graphs ADD COLUMN subscription_desc TEXT DEFAULT ''`,
		`ALTER TABLE graphs ADD COLUMN subscription_site TEXT DEFAULT ''`,
		`ALTER TABLE graphs ADD COLUMN subscription_support TEXT DEFAULT ''`,
		`ALTER TABLE graphs ADD COLUMN client_route TEXT DEFAULT ''`,
		`UPDATE graphs SET client_route = '' WHERE client_route IS NULL`,
		`ALTER TABLE route_rules ADD COLUMN is_default INTEGER DEFAULT 0`,
		// Миграция: убираем FK на graph_id в route_rules (глобальные правила имеют graph_id = '')
		`CREATE TABLE IF NOT EXISTS route_rules_new (
			id          TEXT PRIMARY KEY,
			graph_id    TEXT NOT NULL DEFAULT '',
			name        TEXT NOT NULL,
			rules_json  TEXT NOT NULL DEFAULT '[]',
			is_default  INTEGER NOT NULL DEFAULT 0,
			created_at  INTEGER NOT NULL,
			updated_at  INTEGER NOT NULL
		)`,
		`INSERT INTO route_rules_new (id, graph_id, name, rules_json, is_default, created_at, updated_at)
			SELECT id, COALESCE(graph_id, ''), name, rules_json, COALESCE(is_default, 0), created_at, updated_at
			FROM route_rules`,
		`DROP TABLE route_rules`,
		`ALTER TABLE route_rules_new RENAME TO route_rules`,
		`CREATE INDEX IF NOT EXISTS idx_route_rules_graph ON route_rules(graph_id)`,
	}
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			_ = err
		}
	}
	store := &Store{db: db}
	// Инициализируем глобальные дефолтные правила маршрутизации (один раз)
	if err := store.EnsureGlobalDefaultRouteRules(); err != nil {
		return nil, err
	}
	return store, nil
}

// dsn включает foreign_keys (каскадные удаления) и busy_timeout (конкурентные записи).
func dsn(path string) string {
	if !strings.HasPrefix(path, "file:") {
		path = "file:" + path
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
}

type Node struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	GRPCURL    string `json:"grpc_url"`
	Token      string `json:"token"`
	CertPEM    string `json:"cert_pem"`
	ConfigJSON string `json:"config_json"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
}

func (s *Store) List() ([]Node, error) {
	rows, err := s.db.Query(`SELECT id, name, grpc_url, token, cert_pem, config_json, created_at, updated_at FROM nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := make([]Node, 0)
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Name, &n.GRPCURL, &n.Token, &n.CertPEM, &n.ConfigJSON, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (s *Store) Get(id string) (Node, error) {
	var n Node
	err := s.db.QueryRow(`SELECT id, name, grpc_url, token, cert_pem, config_json, created_at, updated_at FROM nodes WHERE id = ?`, id).
		Scan(&n.ID, &n.Name, &n.GRPCURL, &n.Token, &n.CertPEM, &n.ConfigJSON, &n.CreatedAt, &n.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return n, ErrNotFound
	}
	return n, err
}

func (s *Store) Create(n Node) (Node, error) {
	n.ID = mustUUID()
	now := time.Now().Unix()
	n.CreatedAt = now
	n.UpdatedAt = now
	_, err := s.db.Exec(`INSERT INTO nodes (id, name, grpc_url, token, cert_pem, config_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.Name, n.GRPCURL, n.Token, n.CertPEM, n.ConfigJSON, n.CreatedAt, n.UpdatedAt)
	return n, err
}

func (s *Store) Update(id string, n Node) error {
	n.UpdatedAt = time.Now().Unix()
	_, err := s.db.Exec(`UPDATE nodes SET name = ?, grpc_url = ?, token = ?, cert_pem = ?, config_json = ?, updated_at = ? WHERE id = ?`,
		n.Name, n.GRPCURL, n.Token, n.CertPEM, n.ConfigJSON, n.UpdatedAt, id)
	return err
}

func (s *Store) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM nodes WHERE id = ?`, id)
	return err
}

func mustUUID() string {
	var u [16]byte
	if _, err := rand.Read(u[:]); err != nil {
		panic(err)
	}
	u[6] = (u[6] & 0x0f) | 0x40
	u[8] = (u[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%s",
		binary.BigEndian.Uint32(u[0:4]),
		binary.BigEndian.Uint16(u[4:6]),
		binary.BigEndian.Uint16(u[6:8]),
		binary.BigEndian.Uint16(u[8:10]),
		hex.EncodeToString(u[10:]),
	)
}

// mustToken возвращает 2*n hex-символов криптослучайных данных.
func mustToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
