package db

import (
	"testing"

	"github.com/CascadiaLabs/panel/graph"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func seedNode(t *testing.T, s *Store, name string) Node {
	t.Helper()
	n, err := s.Create(Node{Name: name, GRPCURL: name + ":6237", Token: "tok", CertPEM: "pem"})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestGraphRoundtrip(t *testing.T) {
	s := newTestStore(t)
	phys := seedNode(t, s, "node-1")

	g, err := s.CreateGraph("test-graph")
	if err != nil {
		t.Fatal(err)
	}

	state := graph.State{
		Nodes: []graph.Node{
			{ID: "in1", NodeID: phys.ID, Kind: "inbound", Protocol: "vless", Tag: "in1",
				Settings: []byte(`{"listen_port":443,"users":[{"name":"u","uuid":"x"}]}`),
				PosX:     100, PosY: 200, Entry: true},
			{ID: "out1", NodeID: phys.ID, Kind: "outbound", Protocol: "direct", Tag: "out1",
				Settings: []byte(`{}`), PosX: 300, PosY: 200, Exit: true},
		},
		Edges: []graph.Edge{{ID: "e1", SourceID: "in1", TargetID: "out1"}},
	}
	if err := s.SaveGraphState(g.ID, state); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadGraphState(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Nodes) != 2 || len(loaded.Edges) != 1 {
		t.Fatalf("loaded: %+v", loaded)
	}
	in := loaded.Nodes[0]
	if in.Tag != "in1" || !in.Entry || in.PosX != 100 || string(in.Settings) == "" {
		t.Fatalf("in1: %+v", in)
	}
	if loaded.Edges[0].SourceID != "in1" || loaded.Edges[0].TargetID != "out1" {
		t.Fatalf("edge: %+v", loaded.Edges[0])
	}

	// повторное сохранение заменяет состояние (не дублирует)
	state.Nodes = state.Nodes[:1]
	state.Edges = nil
	if err := s.SaveGraphState(g.ID, state); err != nil {
		t.Fatal(err)
	}
	loaded, err = s.LoadGraphState(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Nodes) != 1 || len(loaded.Edges) != 0 {
		t.Fatalf("after replace: %+v", loaded)
	}
}

// SaveGraphState докидывает служебный relay_user каждому inbound без него
// и сохраняет уже существующий.
func TestGraphSaveGeneratesRelayUser(t *testing.T) {
	s := newTestStore(t)
	phys := seedNode(t, s, "node-1")
	g, err := s.CreateGraph("relay")
	if err != nil {
		t.Fatal(err)
	}
	state := graph.State{
		Nodes: []graph.Node{
			{ID: "in1", NodeID: phys.ID, Kind: "inbound", Protocol: "trojan", Tag: "in1",
				Settings: []byte(`{"listen_port":8443}`)},
			{ID: "out1", NodeID: phys.ID, Kind: "outbound", Protocol: "direct", Tag: "out1", Settings: []byte(`{}`)},
		},
	}
	if err := s.SaveGraphState(g.ID, state); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadGraphState(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	in, err := graph.ParseInboundSettings(loaded.Nodes[0].Settings)
	if err != nil {
		t.Fatal(err)
	}
	if in.RelayUser == nil || in.RelayUser.Name != "relay" || in.RelayUser.UUID == "" || in.RelayUser.Password == "" {
		t.Fatalf("want generated relay_user, got %+v", in.RelayUser)
	}
	ru := *in.RelayUser

	// повторное сохранение (как делают редактор и операции пользователей —
	// грузят состояние, мутируют, сохраняют) не перегенеряет relay_user
	loaded2, err := s.LoadGraphState(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveGraphState(g.ID, loaded2); err != nil {
		t.Fatal(err)
	}
	ru2 := mustLoadRelayUser(t, s, g.ID, "in1")
	if ru2.UUID != ru.UUID || ru2.Password != ru.Password {
		t.Fatalf("relay_user must be stable across saves: %+v → %+v", ru, ru2)
	}
}

func mustLoadRelayUser(t *testing.T, s *Store, graphID, nodeID string) *graph.InboundUser {
	t.Helper()
	st, err := s.LoadGraphState(graphID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range st.Nodes {
		if n.ID != nodeID {
			continue
		}
		in, err := graph.ParseInboundSettings(n.Settings)
		if err != nil {
			t.Fatal(err)
		}
		return in.RelayUser
	}
	t.Fatalf("node %s not found", nodeID)
	return nil
}

func TestGraphCascadeDelete(t *testing.T) {
	s := newTestStore(t)
	phys := seedNode(t, s, "node-1")
	g, err := s.CreateGraph("cascade")
	if err != nil {
		t.Fatal(err)
	}
	state := graph.State{
		Nodes: []graph.Node{
			{ID: "in1", NodeID: phys.ID, Kind: "inbound", Protocol: "vless", Tag: "in1", Settings: []byte(`{}`)},
		},
	}
	if err := s.SaveGraphState(g.ID, state); err != nil {
		t.Fatal(err)
	}

	// удаление графа каскадно удаляет элементы
	if err := s.DeleteGraph(g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetGraph(g.ID); err != ErrNotFound {
		t.Fatalf("graph must be gone, got err=%v", err)
	}
	loaded, err := s.LoadGraphState(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Nodes) != 0 {
		t.Fatalf("state must be gone after graph delete, got %+v", loaded.Nodes)
	}

	// удаление физической ноды каскадно удаляет её элементы графа
	g2, _ := s.CreateGraph("cascade2")
	state2 := graph.State{Nodes: []graph.Node{
		{ID: "in2", NodeID: phys.ID, Kind: "inbound", Protocol: "vless", Tag: "in2", Settings: []byte(`{}`)},
	}}
	if err := s.SaveGraphState(g2.ID, state2); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(phys.ID); err != nil {
		t.Fatal(err)
	}
	loaded2, err := s.LoadGraphState(g2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded2.Nodes) != 0 {
		t.Fatalf("elements of deleted node must cascade: %+v", loaded2.Nodes)
	}
}

func TestGraphListAndRename(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateGraph("alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateGraph("beta"); err != nil {
		t.Fatal(err)
	}
	graphs, err := s.ListGraphs()
	if err != nil {
		t.Fatal(err)
	}
	if len(graphs) != 2 || graphs[0].Name != "alpha" {
		t.Fatalf("list: %+v", graphs)
	}
	if err := s.RenameGraph(graphs[0].ID, "gamma"); err != nil {
		t.Fatal(err)
	}
	graphs, _ = s.ListGraphs()
	if graphs[0].Name != "beta" || graphs[1].Name != "gamma" {
		t.Fatalf("after rename: %+v", graphs)
	}
	if err := s.DeleteGraph(graphs[1].ID); err != nil {
		t.Fatal(err)
	}
	graphs, _ = s.ListGraphs()
	if len(graphs) != 1 {
		t.Fatalf("after delete: %+v", graphs)
	}
}

func TestDuplicateGraphNameFails(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateGraph("same"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateGraph("same"); err == nil {
		t.Fatal("duplicate graph name must fail (UNIQUE)")
	}
}
