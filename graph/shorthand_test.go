package graph

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func shorthandCascade(t *testing.T) (State, []PhysNode) {
	t.Helper()
	return State{
		Nodes: []Node{
			{ID: "a", NodeID: "A", Kind: KindInbound, Protocol: "vless", Tag: "a", Entry: true,
				Settings: mustJSON(t, InboundSettings{ListenPort: 443, Users: []InboundUser{{UUID: "client"}}})},
			{ID: "b", NodeID: "B", Kind: KindInbound, Protocol: "trojan", Tag: "b", Exit: true,
				Settings: mustJSON(t, InboundSettings{ListenPort: 8443, Users: []InboundUser{{Password: "relay-secret"}},
					TLS:       &InboundTLS{Enabled: true, ServerName: "b.example", CertPEM: "cert"},
					Transport: &TransportSettings{Type: "grpc", ServiceName: "relay"}})},
		},
		Edges: []Edge{{ID: "ab", SourceID: "a", TargetID: "b"}},
	}, []PhysNode{{ID: "A", GRPCURL: "source.example:6237"}, {ID: "B", GRPCURL: "target.example:6237"}}
}

func decodeConfig(t *testing.T, raw string) map[string]any {
	t.Helper()
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func configOutbound(t *testing.T, cfg map[string]any, tag string) map[string]any {
	t.Helper()
	for _, raw := range cfg["outbounds"].([]any) {
		out := raw.(map[string]any)
		if out["tag"] == tag {
			return out
		}
	}
	t.Fatalf("missing outbound %q: %v", tag, cfg)
	return nil
}

func TestGenerateShorthandCascade(t *testing.T) {
	st, phys := shorthandCascade(t)
	before := string(mustJSON(t, st))
	res := (&Validator{State: st, Nodes: phys}).Validate()
	if res.HasErrors() || len(res.Warnings) != 0 {
		t.Fatalf("unexpected validation: %+v", res)
	}
	configs, err := Generate(st, phys, InboundRouteRules{})
	if err != nil {
		t.Fatal(err)
	}
	a := decodeConfig(t, configs["A"])
	// Находим правило с relay outbound (не cascadia-direct и не dns)
	var rule map[string]any
	for _, r := range a["route"].(map[string]any)["rules"].([]any) {
		m := r.(map[string]any)
		if ob, ok := m["outbound"].(string); ok && ob != "cascadia-direct" && ob != "" && !strings.HasPrefix(ob, "dns-") {
			rule = m
			break
		}
	}
	if rule == nil {
		t.Fatal("no rule with outbound found")
	}
	relay := configOutbound(t, a, rule["outbound"].(string))
	if relay["type"] != "trojan" || relay["server"] != "target.example" || relay["server_port"] != float64(8443) || relay["password"] != "relay-secret" {
		t.Fatalf("relay did not mirror target: %v", relay)
	}
	if relay["transport"].(map[string]any)["service_name"] != "relay" || relay["tls"].(map[string]any)["server_name"] != "b.example" {
		t.Fatalf("missing target TLS/transport: %v", relay)
	}
	b := decodeConfig(t, configs["B"])
	// Находим правило с inbound для ноды b (exit route), пропуская системные правила
	var exitRule map[string]any
	for _, r := range b["route"].(map[string]any)["rules"].([]any) {
		m := r.(map[string]any)
		if ib, ok := m["inbound"].([]any); ok && len(ib) > 0 && ib[0] == "b" {
			exitRule = m
			break
		}
	}
	if exitRule == nil {
		t.Fatal("no exit rule for node b found")
	}
	if exitRule["inbound"].([]any)[0] != "b" || configOutbound(t, b, exitRule["outbound"].(string))["type"] != "direct" {
		t.Fatalf("missing explicit Internet route: %v", exitRule)
	}
	again, err := Generate(st, phys, InboundRouteRules{})
	if err != nil || !reflect.DeepEqual(configs, again) || string(mustJSON(t, st)) != before {
		t.Fatalf("generation was not deterministic/nonmutating: %v", err)
	}
}

func TestGenerateExplicitRelayTargetHost(t *testing.T) {
	st, phys := shorthandCascade(t)
	st.Nodes = append(st.Nodes, Node{ID: "out", NodeID: "A", Kind: KindOutbound, Protocol: "trojan", Tag: "out"})
	st.Edges = []Edge{{SourceID: "a", TargetID: "out"}, {SourceID: "out", TargetID: "b"}}
	configs, err := Generate(st, phys, InboundRouteRules{})
	if err != nil {
		t.Fatal(err)
	}
	if out := configOutbound(t, decodeConfig(t, configs["A"]), "out"); out["server"] != "target.example" {
		t.Fatalf("explicit relay used source host: %v", out)
	}
}

func TestGenerateBalancerInboundCandidates(t *testing.T) {
	for _, protocol := range []string{"urltest", "selector"} {
		t.Run(protocol, func(t *testing.T) {
			st, phys := shorthandCascade(t)
			st.Nodes = append(st.Nodes,
				Node{ID: "bal", NodeID: "A", Kind: KindBalancer, Protocol: protocol, Tag: "bal", Settings: mustJSON(t, BalancerSettings{Default: "b"})},
				Node{ID: "c", NodeID: "C", Kind: KindInbound, Protocol: "vmess", Tag: "c", Exit: true,
					Settings: mustJSON(t, InboundSettings{ListenPort: 9443, Users: []InboundUser{{UUID: "c-user"}}})},
				Node{ID: "local", NodeID: "A", Kind: KindOutbound, Protocol: "direct", Tag: "local"})
			phys = append(phys, PhysNode{ID: "C", GRPCURL: "c.example:6237"})
			st.Edges = []Edge{{SourceID: "a", TargetID: "bal"}, {SourceID: "bal", TargetID: "b"}, {SourceID: "bal", TargetID: "c"}, {SourceID: "bal", TargetID: "local"}}
			before := string(mustJSON(t, st))
			configs, err := Generate(st, phys, InboundRouteRules{})
			if err != nil {
				t.Fatal(err)
			}
			a := decodeConfig(t, configs["A"])
			bal := configOutbound(t, a, "bal")
			members := bal["outbounds"].([]any)
			if len(members) != 3 {
				t.Fatalf("members: %v", members)
			}
			seen := map[any]bool{}
			for _, member := range members {
				out := configOutbound(t, a, member.(string))
				seen[out["type"]] = true
			}
			if !seen["trojan"] || !seen["vmess"] || !seen["direct"] {
				t.Fatalf("candidate protocols: %v", seen)
			}
			if protocol == "selector" && configOutbound(t, a, bal["default"].(string))["server"] != "target.example" {
				t.Fatalf("selector default not remapped: %v", bal)
			}
			for i, j := 0, len(st.Edges)-1; i < j; i, j = i+1, j-1 {
				st.Edges[i], st.Edges[j] = st.Edges[j], st.Edges[i]
			}
			again, err := Generate(st, phys, InboundRouteRules{})
			if err != nil || !reflect.DeepEqual(configs, again) {
				t.Fatalf("edge order changed output: %v", err)
			}
			for i, j := 0, len(st.Edges)-1; i < j; i, j = i+1, j-1 {
				st.Edges[i], st.Edges[j] = st.Edges[j], st.Edges[i]
			}
			if before != string(mustJSON(t, st)) {
				t.Fatal("compiler mutated stored selector settings")
			}
		})
	}
}

func TestGenerateBalancerToInboundSameNode(t *testing.T) {
	st, phys := shorthandCascade(t)
	st.Nodes = append(st.Nodes,
		Node{ID: "bal", NodeID: "A", Kind: KindBalancer, Protocol: "selector", Tag: "bal",
			Settings: mustJSON(t, BalancerSettings{Default: "local"})},
		Node{ID: "local", NodeID: "A", Kind: KindInbound, Protocol: "trojan", Tag: "local",
			Settings: mustJSON(t, InboundSettings{ListenPort: 8444, Users: []InboundUser{{Password: "local-secret"}}})},
	)
	st.Edges = []Edge{
		{SourceID: "a", TargetID: "bal"},
		{SourceID: "bal", TargetID: "local"},
		{SourceID: "local", TargetID: "b"},
	}
	before := string(mustJSON(t, st))
	res := (&Validator{State: st, Nodes: phys}).Validate()
	if res.HasErrors() {
		t.Fatalf("same-node balancer relay must validate: %+v", res.Errors)
	}
	configs, err := Generate(st, phys, InboundRouteRules{})
	if err != nil {
		t.Fatal(err)
	}
	a := decodeConfig(t, configs["A"])
	bal := configOutbound(t, a, "bal")
	members := bal["outbounds"].([]any)
	if len(members) != 1 {
		t.Fatalf("balancer members: %v", members)
	}
	relayTag := members[0].(string)
	if relayTag == "local" {
		t.Fatal("balancer member must be a generated relay outbound")
	}
	if bal["default"] != relayTag {
		t.Fatalf("selector default was not remapped: %v", bal)
	}
	relay := configOutbound(t, a, relayTag)
	if relay["type"] != "trojan" || relay["server"] != "source.example" || relay["server_port"] != float64(8444) || relay["password"] != "local-secret" {
		t.Fatalf("local relay did not mirror target inbound: %v", relay)
	}
	rules := a["route"].(map[string]any)["rules"].([]any)
	var localRule map[string]any
	for _, raw := range rules {
		rule := raw.(map[string]any)
		if ib, ok := rule["inbound"].([]any); ok && len(ib) > 0 && ib[0] == "local" {
			localRule = rule
			break
		}
	}
	if localRule == nil || configOutbound(t, a, localRule["outbound"].(string))["server"] != "target.example" {
		t.Fatalf("local inbound must continue to the next cascade: %v", localRule)
	}
	again, err := Generate(st, phys, InboundRouteRules{})
	if err != nil || !reflect.DeepEqual(configs, again) || string(mustJSON(t, st)) != before {
		t.Fatalf("generation was not deterministic/nonmutating: %v", err)
	}
}

func TestShorthandValidation(t *testing.T) {
	cases := []struct {
		name, code string
		change     func(*State)
	}{
		{"same node inbound", "cascade_same_node", func(st *State) { st.Nodes[1].NodeID = "A" }},
		{"cycle", "cycle", func(st *State) {
			st.Nodes[1].Exit = false
			st.Edges = append(st.Edges, Edge{SourceID: "b", TargetID: "a"})
		}},
		{"exit and relay", "inbound_exit_and_target", func(st *State) { st.Nodes[0].Exit = true }},
		{"multiple defaults", "inbound_multiple_targets", func(st *State) {
			st.Nodes = append(st.Nodes, Node{ID: "d", NodeID: "A", Kind: KindOutbound, Protocol: "direct", Tag: "d"})
			st.Edges = append(st.Edges, Edge{SourceID: "a", TargetID: "d"})
		}},
		{"outbound multiple relays", "outbound_multiple_targets", func(st *State) {
			st.Nodes[0].Kind, st.Nodes[0].Protocol = KindOutbound, "trojan"
			c := st.Nodes[1]
			c.ID, c.Tag = "c", "c"
			st.Nodes = append(st.Nodes, c)
			st.Edges = append(st.Edges, Edge{SourceID: "a", TargetID: "c"})
		}},
		{"exit and catch all", "inbound_exit_and_target", func(st *State) {
			st.Nodes[0].Exit = true
			st.Nodes = append(st.Nodes, Node{ID: "r", NodeID: "A", Kind: KindRule, Protocol: "match", Tag: "r"}, Node{ID: "d", NodeID: "A", Kind: KindOutbound, Protocol: "direct", Tag: "d"})
			st.Edges = []Edge{{SourceID: "a", TargetID: "r"}, {SourceID: "r", TargetID: "d"}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st, phys := shorthandCascade(t)
			tc.change(&st)
			res := (&Validator{State: st, Nodes: phys}).Validate()
			if !hasCode(res.Errors, tc.code) {
				t.Fatalf("want %s: %+v", tc.code, res)
			}
			if _, err := Generate(st, phys, InboundRouteRules{}); err == nil {
				t.Fatal("compiler accepted invalid original graph")
			}
			if tc.code == "cycle" {
				for _, issue := range res.Errors {
					if issue.Code == "cycle" && strings.Contains(issue.Element, "cascadia-") {
						t.Fatalf("cycle references internal graph: %+v", issue)
					}
				}
			}
		})
	}
}

func TestInboundExitWithSpecificRule(t *testing.T) {
	st, phys := shorthandCascade(t)
	st.Nodes[0].Exit = true
	st.Nodes = append(st.Nodes,
		Node{ID: "r", NodeID: "A", Kind: KindRule, Protocol: "match", Tag: "r", PosY: 100,
			Settings: mustJSON(t, RuleSettings{Domain: []string{"example.org"}})},
		Node{ID: "out", NodeID: "A", Kind: KindOutbound, Protocol: "trojan", Tag: "out"})
	st.Edges = []Edge{{SourceID: "a", TargetID: "r"}, {SourceID: "r", TargetID: "out"}, {SourceID: "out", TargetID: "b"}}
	configs, err := Generate(st, phys, InboundRouteRules{})
	if err != nil {
		t.Fatal(err)
	}
	a := decodeConfig(t, configs["A"])
	rules := a["route"].(map[string]any)["rules"].([]any)
	// Находим правило с domain (graph rule), пропуская системные
	var domainRule map[string]any
	for _, r := range rules {
		m := r.(map[string]any)
		if m["domain"] != nil {
			domainRule = m
			break
		}
	}
	if domainRule == nil || domainRule["outbound"] != "out" {
		t.Fatalf("specific route must precede Internet default: %v", rules)
	}
	// Находим catch-all rule (inbound + outbound, без domain/domain_suffix/rule_set/ip_is_private)
	var catchAllRule map[string]any
	for _, r := range rules {
		m := r.(map[string]any)
		if m["domain"] == nil && m["domain_suffix"] == nil && m["rule_set"] == nil && m["ip_is_private"] == nil && m["inbound"] != nil {
			catchAllRule = m
			break
		}
	}
	if catchAllRule == nil || catchAllRule["outbound"] != "cascadia-direct" {
		t.Fatalf("specific route must have direct outbound: %v", rules)
	}
}

func TestInternalTagsAvoidCollisions(t *testing.T) {
	st, phys := shorthandCascade(t)
	expanded := expandShorthand(st)
	internal := expanded.Nodes[len(st.Nodes)]
	st.Nodes = append(st.Nodes,
		Node{ID: internal.ID, Tag: internal.Tag, NodeID: "A", Kind: KindOutbound, Protocol: "direct"},
		Node{ID: "reserved", Tag: DefaultDirectTag, NodeID: "B", Kind: KindRule, Protocol: "match", Settings: mustJSON(t, RuleSettings{Domain: []string{"example.org"}})},
		Node{ID: "exit", Tag: "existing-direct", NodeID: "B", Kind: KindOutbound, Protocol: "direct"})
	st.Edges = append(st.Edges, Edge{SourceID: "b", TargetID: "reserved"}, Edge{SourceID: "reserved", TargetID: "exit"})
	configs, err := Generate(st, phys, InboundRouteRules{})
	if err != nil {
		t.Fatal(err)
	}
	// Проверяем collision на ноде B, где находится reserved rule
	b := decodeConfig(t, configs["B"])
	var route map[string]any
	for _, r := range b["route"].(map[string]any)["rules"].([]any) {
		m := r.(map[string]any)
		if m["domain"] != nil {
			route = m
			break
		}
	}
	if route == nil || route["outbound"] == internal.Tag || configOutbound(t, b, route["outbound"].(string))["type"] != "direct" {
		t.Fatalf("internal relay collision: %v", route)
	}
	// Force an automatic direct next to a user-owned non-direct reserved tag.
	st, phys = shorthandCascade(t)
	st.Nodes[0].Tag = DefaultDirectTag
	configs, err = Generate(st, phys, InboundRouteRules{})
	if err != nil {
		t.Fatal(err)
	}
	a := decodeConfig(t, configs["A"])
	final := a["route"].(map[string]any)["final"].(string)
	if final == DefaultDirectTag || configOutbound(t, a, final)["type"] != "direct" {
		t.Fatalf("automatic direct collision: %v", a)
	}
}

func TestNormalDirectExitHasNoWarning(t *testing.T) {
	st, phys := twoNodeCascade(t)
	res := (&Validator{State: st, Nodes: phys}).Validate()
	if len(res.Warnings) != 0 || res.HasErrors() {
		t.Fatalf("normal explicit direct exit should be clean: %+v", res)
	}
}

func TestRuleToInboundShorthand(t *testing.T) {
	// rule → inbound: правило на ноде A ведёт прямо на inbound ноды B.
	st, phys := shorthandCascade(t)
	st.Nodes = append(st.Nodes,
		Node{ID: "r", NodeID: "A", Kind: KindRule, Protocol: "match", Tag: "ru-rule",
			Settings: mustJSON(t, RuleSettings{DomainSuffix: []string{"ru"}})},
		Node{ID: "od", NodeID: "A", Kind: KindOutbound, Protocol: "direct", Tag: "direct-out"},
	)
	st.Edges = []Edge{
		{ID: "ar", SourceID: "a", TargetID: "r"},
		{ID: "rb", SourceID: "r", TargetID: "b"}, // правило → чужой inbound
		{ID: "ad", SourceID: "a", TargetID: "od"},
	}
	res := (&Validator{State: st, Nodes: phys}).Validate()
	if res.HasErrors() {
		t.Fatalf("rule→inbound must validate: %+v", res.Errors)
	}
	configs, err := Generate(st, phys, InboundRouteRules{})
	if err != nil {
		t.Fatal(err)
	}
	a := decodeConfig(t, configs["A"])
	rules := a["route"].(map[string]any)["rules"].([]any)
	var ruRule, defaultRule map[string]any
	for _, raw := range rules {
		r := raw.(map[string]any)
		if r["domain_suffix"] != nil {
			ruRule = r
		}
		// Default rule: catch-all с inbound + outbound, без domain/domain_suffix/rule_set/ip_is_private
		if r["domain"] == nil && r["domain_suffix"] == nil && r["rule_set"] == nil && r["ip_is_private"] == nil && r["inbound"] != nil && r["outbound"] != nil {
			defaultRule = r
		}
	}
	if ruRule == nil || defaultRule == nil {
		t.Fatalf("want .ru rule + default rule, got: %v", rules)
	}
	if ruRule["domain_suffix"].([]any)[0] != "ru" {
		t.Fatalf("match fields lost: %v", ruRule)
	}
	if defaultRule["outbound"] != "direct-out" {
		t.Fatalf("unconditional default must go to direct: %v", defaultRule)
	}
	// правило ведёт на авто-релей: trojan, порт/секрет от целевого inbound
	relay := configOutbound(t, a, ruRule["outbound"].(string))
	if relay["type"] != "trojan" || relay["server"] != "target.example" || relay["password"] != "relay-secret" {
		t.Fatalf("rule relay did not mirror target inbound: %v", relay)
	}
}

func TestRuleToInboundSameNodeRejected(t *testing.T) {
	st, phys := shorthandCascade(t)
	st.Nodes = append(st.Nodes, Node{ID: "r", NodeID: "A", Kind: KindRule, Protocol: "match", Tag: "r",
		Settings: mustJSON(t, RuleSettings{DomainSuffix: []string{"ru"}})})
	st.Edges = []Edge{
		{SourceID: "a", TargetID: "r"},
		{SourceID: "r", TargetID: "in-a-same"}, // цель на той же ноде A
	}
	st.Nodes = append(st.Nodes, Node{ID: "in-a-same", NodeID: "A", Kind: KindInbound, Protocol: "vless", Tag: "same",
		Settings: mustJSON(t, InboundSettings{ListenPort: 9, Users: []InboundUser{{UUID: "u"}}})})
	res := (&Validator{State: st, Nodes: phys}).Validate()
	if !hasCode(res.Errors, "cascade_same_node") {
		t.Fatalf("want cascade_same_node for rule→inbound on one node, got %+v", res.Errors)
	}
}
