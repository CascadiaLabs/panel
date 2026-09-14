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
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL UNIQUE,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
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
	return &Store{db: db}, nil
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
