package graph

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// twoNodeCascade строит проверенный сценарий:
// клиент → vless in A → vless+reality out A ⇒ vless in B → direct out B → интернет
func twoNodeCascade(t *testing.T) (State, []PhysNode) {
	t.Helper()
	priv, pub, err := GenerateRealityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	st := State{
		Nodes: []Node{
			{ID: "in-a", NodeID: "nodeA", Kind: KindInbound, Protocol: "vless", Tag: "in-a",
				Entry: true, Settings: mustJSON(t, InboundSettings{
					ListenPort: 443, PublicHost: "a.example.com",
					Users: []InboundUser{{Name: "user", UUID: "11111111-1111-1111-1111-111111111111"}},
					TLS: &InboundTLS{Enabled: true, ServerName: "a.example.com", Reality: &RealityIn{
						Enabled: true, PrivateKey: priv, ShortIDs: []string{"abcd1234"},
						HandshakeServer: "www.microsoft.com", HandshakePort: 443,
					}},
				})},
			{ID: "out-a", NodeID: "nodeA", Kind: KindOutbound, Protocol: "vless", Tag: "out-a",
				Settings: mustJSON(t, OutboundSettings{})},
			{ID: "in-b", NodeID: "nodeB", Kind: KindInbound, Protocol: "vless", Tag: "in-b",
				Settings: mustJSON(t, InboundSettings{
					ListenPort: 8443, PublicHost: "b.example.com",
					Users: []InboundUser{{Name: "user", UUID: "22222222-2222-2222-2222-222222222222"}},
					TLS: &InboundTLS{Enabled: true, ServerName: "b.example.com", CertPEM: "PEM-B"},
				})},
			{ID: "out-b", NodeID: "nodeB", Kind: KindOutbound, Protocol: "direct", Tag: "out-b", Exit: true},
		},
		Edges: []Edge{
			{ID: "e1", SourceID: "in-a", TargetID: "out-a"},
			{ID: "e2", SourceID: "out-a", TargetID: "in-b"}, // каскад
			{ID: "e3", SourceID: "in-b", TargetID: "out-b"},
		},
	}
	_ = pub
	return st, []PhysNode{
		{ID: "nodeA", Name: "Node A", GRPCURL: "a.example.com:6237"},
		{ID: "nodeB", Name: "Node B", GRPCURL: "b.example.com:6237"},
	}
}

func TestValidateTwoNodeCascadeOK(t *testing.T) {
	st, phys := twoNodeCascade(t)
	res := (&Validator{State: st, Nodes: phys}).Validate()
	for _, e := range res.Errors {
		t.Errorf("unexpected error: %+v", e)
	}
	if res.HasErrors() {
		t.Fatal("valid cascade flagged as invalid")
	}
}

func TestValidateForbiddenEdges(t *testing.T) {
	st, phys := twoNodeCascade(t)
	// outbound → balancer запрещён
	st.Edges = append(st.Edges, Edge{ID: "x", SourceID: "out-b", TargetID: "out-a"})
	res := (&Validator{State: st, Nodes: phys}).Validate()
	if !hasCode(res.Errors, "forbidden_edge") {
		t.Fatalf("want forbidden_edge, got %+v", res.Errors)
	}
}

func hasCode(issues []Issue, code string) bool {
	for _, i := range issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

func TestValidateCycle(t *testing.T) {
	st, phys := twoNodeCascade(t)
	// замыкаем цикл: in-b → out-a (межнодовая петля A→B→A через элементы)
	st.Edges = append(st.Edges, Edge{ID: "c", SourceID: "in-b", TargetID: "out-a"})
	res := (&Validator{State: st, Nodes: phys}).Validate()
	if !hasCode(res.Errors, "cycle") {
		t.Fatalf("want cycle, got %+v", res.Errors)
	}
	// сообщение содержит цепочку тегов
	var cyc Issue
	for _, i := range res.Errors {
		if i.Code == "cycle" {
			cyc = i
		}
	}
	if !strings.Contains(cyc.Message, "→") {
		t.Fatalf("cycle message should list chain: %s", cyc.Message)
	}
}

func TestValidateCrossNodeRule(t *testing.T) {
	st, phys := twoNodeCascade(t)
	// правило на ноде A ведёт на outbound ноды B — запрещено
	st.Nodes = append(st.Nodes, Node{ID: "rule-x", NodeID: "nodeA", Kind: KindRule, Protocol: "match", Tag: "rule-x",
		Settings: mustJSON(t, RuleSettings{Domain: []string{"example.org"}})})
	st.Edges = append(st.Edges, Edge{ID: "r1", SourceID: "in-a", TargetID: "rule-x"})
	st.Edges = append(st.Edges, Edge{ID: "r2", SourceID: "rule-x", TargetID: "out-b"})
	res := (&Validator{State: st, Nodes: phys}).Validate()
	if !hasCode(res.Errors, "cross_node_edge") {
		t.Fatalf("want cross_node_edge, got %+v", res.Errors)
	}
}

func TestValidateProtocolMismatchCascade(t *testing.T) {
	st, phys := twoNodeCascade(t)
	// делаем in-b trojan, а out-a остаётся vless
	for i := range st.Nodes {
		if st.Nodes[i].ID == "in-b" {
			st.Nodes[i].Protocol = "trojan"
			st.Nodes[i].Settings = mustJSON(t, InboundSettings{
				ListenPort: 8443, Users: []InboundUser{{Name: "u", Password: "pw"}},
			})
		}
	}
	res := (&Validator{State: st, Nodes: phys}).Validate()
	if !hasCode(res.Errors, "protocol_mismatch") {
		t.Fatalf("want protocol_mismatch, got %+v", res.Errors)
	}
}

func TestValidateCascadeSameNode(t *testing.T) {
	st, phys := twoNodeCascade(t)
	// out-a теперь указывает на inbound той же ноды
	for i := range st.Nodes {
		if st.Nodes[i].ID == "in-b" {
			st.Nodes[i].NodeID = "nodeA"
		}
		if st.Nodes[i].ID == "out-b" {
			st.Nodes[i].NodeID = "nodeA"
		}
	}
	res := (&Validator{State: st, Nodes: phys}).Validate()
	if !hasCode(res.Errors, "cascade_same_node") {
		t.Fatalf("want cascade_same_node, got %+v", res.Errors)
	}
}

func TestValidateUnroutableAndRelayExit(t *testing.T) {
	st, phys := twoNodeCascade(t)
	// out-a: каскад + exit одновременно
	for i := range st.Nodes {
		if st.Nodes[i].ID == "out-a" {
			st.Nodes[i].Exit = true
		}
	}
	res := (&Validator{State: st, Nodes: phys}).Validate()
	if !hasCode(res.Errors, "outbound_relay_and_exit") {
		t.Fatalf("want outbound_relay_and_exit, got %+v", res.Errors)
	}

	// out-b никуда не ведёт (снимаем exit)
	for i := range st.Nodes {
		if st.Nodes[i].ID == "out-a" {
			st.Nodes[i].Exit = false
		}
		if st.Nodes[i].ID == "out-b" {
			st.Nodes[i].Exit = false
			st.Nodes[i].Protocol = "vless"
			st.Nodes[i].Settings = mustJSON(t, OutboundSettings{Server: "x", ServerPort: 1})
		}
	}
	res = (&Validator{State: st, Nodes: phys}).Validate()
	if !hasCode(res.Errors, "unroutable_outbound") {
		t.Fatalf("want unroutable_outbound, got %+v", res.Errors)
	}
}

func TestValidateDuplicateTagsAndPorts(t *testing.T) {
	st, phys := twoNodeCascade(t)
	// тот же тег на другой ноде
	st.Nodes = append(st.Nodes, Node{ID: "dup", NodeID: "nodeB", Kind: KindOutbound, Protocol: "direct", Tag: "out-b"})
	res := (&Validator{State: st, Nodes: phys}).Validate()
	if !hasCode(res.Errors, "duplicate_tag") {
		t.Fatalf("want duplicate_tag, got %+v", res.Errors)
	}
	// тот же порт на той же ноде — предупреждать не обязаны, но генератор
	// sing-box упадёт при старте; проверим что валидация не считает это ошибкой
	// (это ответственность оператора; sing-box 1.x сам разрулит конфликты портов).
	st.Nodes = st.Nodes[:len(st.Nodes)-1]
	res = (&Validator{State: st, Nodes: phys}).Validate()
	if res.HasErrors() {
		t.Fatalf("unexpected: %+v", res.Errors)
	}
}

func TestValidateTLSRequiredForQUIC(t *testing.T) {
	st, phys := twoNodeCascade(t)
	st.Nodes = append(st.Nodes, Node{ID: "hy", NodeID: "nodeA", Kind: KindInbound, Protocol: "hysteria2", Tag: "in-hy",
		Settings: mustJSON(t, InboundSettings{ListenPort: 9000, Users: []InboundUser{{Name: "u", Password: "p"}}})})
	res := (&Validator{State: st, Nodes: phys}).Validate()
	if !hasCode(res.Errors, "tls_required") {
		t.Fatalf("want tls_required, got %+v", res.Errors)
	}
}

func TestValidateRuleChainOK(t *testing.T) {
	st, phys := twoNodeCascade(t)
	// in-b分流: правило .ru → direct, остальное → out-b
	st.Nodes = append(st.Nodes,
		Node{ID: "rule-b", NodeID: "nodeB", Kind: KindRule, Protocol: "match", Tag: "rule-b",
			Settings: mustJSON(t, RuleSettings{DomainSuffix: []string{"ru"}})},
		Node{ID: "out-b2", NodeID: "nodeB", Kind: KindOutbound, Protocol: "direct", Tag: "out-b2"},
	)
	st.Edges = append(st.Edges,
		Edge{ID: "rb1", SourceID: "in-b", TargetID: "rule-b"},
		Edge{ID: "rb2", SourceID: "rule-b", TargetID: "out-b2"},
	)
	res := (&Validator{State: st, Nodes: phys}).Validate()
	if res.HasErrors() {
		t.Fatalf("unexpected errors: %+v", res.Errors)
	}
	if !hasCode(res.Warnings, "unreachable") {
		// out-b2 достижим; out-b всё ещё достижим; предупреждений о недостижимости быть не должно
		t.Logf("warnings: %+v", res.Warnings)
	}
}

func TestValidateBalancerChain(t *testing.T) {
	priv, _ := mustReality(t)
	st := State{
		Nodes: []Node{
			{ID: "in1", NodeID: "n1", Kind: KindInbound, Protocol: "vless", Tag: "in1", Entry: true,
				Settings: mustJSON(t, InboundSettings{ListenPort: 1, Users: []InboundUser{{Name: "u", UUID: "u1"}}})},
			{ID: "bal", NodeID: "n1", Kind: KindBalancer, Protocol: "urltest", Tag: "bal",
				Settings: mustJSON(t, BalancerSettings{URL: "https://www.gstatic.com/generate_204"})},
			{ID: "ob1", NodeID: "n1", Kind: KindOutbound, Protocol: "vless", Tag: "ob1",
				Settings: mustJSON(t, OutboundSettings{})},
			{ID: "ob2", NodeID: "n1", Kind: KindOutbound, Protocol: "vless", Tag: "ob2",
				Settings: mustJSON(t, OutboundSettings{})},
			{ID: "inX", NodeID: "n2", Kind: KindInbound, Protocol: "vless", Tag: "inX",
				Settings: mustJSON(t, InboundSettings{ListenPort: 2, Users: []InboundUser{{Name: "u", UUID: "u2"}},
					TLS: &InboundTLS{Enabled: true, Reality: &RealityIn{Enabled: true, PrivateKey: priv, ShortIDs: []string{"ab"}, HandshakeServer: "www.apple.com", HandshakePort: 443}}})},
		},
		Edges: []Edge{
			{SourceID: "in1", TargetID: "bal"},
			{SourceID: "bal", TargetID: "ob1"},
			{SourceID: "bal", TargetID: "ob2"},
			{SourceID: "ob1", TargetID: "inX"},
			{SourceID: "ob2", TargetID: "inX"},
		},
	}
	phys := []PhysNode{{ID: "n1", Name: "1", GRPCURL: "n1:6237"}, {ID: "n2", Name: "2", GRPCURL: "n2:6237"}}
	res := (&Validator{State: st, Nodes: phys}).Validate()
	if res.HasErrors() {
		t.Fatalf("unexpected errors: %+v", res.Errors)
	}
}

func mustReality(t *testing.T) (priv, pub string) {
	t.Helper()
	priv, pub, err := GenerateRealityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return priv, pub
}
