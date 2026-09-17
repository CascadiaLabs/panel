package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/CascadiaLabs/panel/db"
	"github.com/CascadiaLabs/panel/graph"
)

func TestSaveGraphValidatesGeneratedRelayUser(t *testing.T) {
	h := newTestServer(t, "graph-token")
	auth := map[string]string{"Authorization": "Bearer graph-token"}

	createNode := func(name string) db.Node {
		t.Helper()
		rec := do(t, h, http.MethodPost, "/api/nodes/", `{"name":"`+name+`","grpc_url":"`+name+`.example:6237"}`, auth)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create node %s: got %d body=%s", name, rec.Code, rec.Body)
		}
		var n db.Node
		if err := json.Unmarshal(rec.Body.Bytes(), &n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	source := createNode("source")
	target := createNode("target")
	rec := do(t, h, http.MethodPost, "/api/graphs/", `{"name":"cascade"}`, auth)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create graph: got %d body=%s", rec.Code, rec.Body)
	}
	var g db.Graph
	if err := json.Unmarshal(rec.Body.Bytes(), &g); err != nil {
		t.Fatal(err)
	}

	state := graph.State{
		Nodes: []graph.Node{
			{ID: "out", NodeID: source.ID, Kind: graph.KindOutbound, Protocol: "vless", Tag: "relay-out", Settings: json.RawMessage(`{}`)},
			{ID: "in", NodeID: target.ID, Kind: graph.KindInbound, Protocol: "vless", Tag: "relay-in", Settings: json.RawMessage(`{"listen_port":8443}`)},
		},
		Edges: []graph.Edge{{ID: "cascade", SourceID: "out", TargetID: "in"}},
	}
	body, err := json.Marshal(map[string]any{"state": state})
	if err != nil {
		t.Fatal(err)
	}
	rec = do(t, h, http.MethodPut, "/api/graphs/"+g.ID, string(body), auth)
	if rec.Code != http.StatusOK {
		t.Fatalf("save graph: got %d body=%s", rec.Code, rec.Body)
	}

	var response graphResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Validation == nil {
		t.Fatal("save response must include validation")
	}
	for _, issue := range response.Validation.Errors {
		if issue.Code == "cascade_no_users" {
			t.Fatalf("generated relay_user must satisfy cascade credentials: %+v", response.Validation.Errors)
		}
	}
	for _, n := range response.State.Nodes {
		if n.ID != "in" {
			continue
		}
		settings, err := graph.ParseInboundSettings(n.Settings)
		if err != nil {
			t.Fatal(err)
		}
		if settings.RelayUser == nil || settings.RelayUser.Name != "relay" {
			t.Fatalf("target inbound must contain generated relay_user: %+v", settings.RelayUser)
		}
		return
	}
	t.Fatal("target inbound missing from saved graph")
}
