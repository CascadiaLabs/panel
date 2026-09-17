package graph

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestGenerateTwoNodeCascade — golden-сценарий:
// клиент → vless in A → vless+reality out A ⇒ vless in B → direct out B → интернет.
func TestGenerateTwoNodeCascade(t *testing.T) {
	st, phys := twoNodeCascade(t)

	configs, err := Generate(st, phys)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 2 {
		t.Fatalf("want 2 configs, got %d", len(configs))
	}

	// --- нода A ---
	var cfgA map[string]any
	if err := json.Unmarshal([]byte(configs["nodeA"]), &cfgA); err != nil {
		t.Fatal(err)
	}
	inA := cfgA["inbounds"].([]any)
	if len(inA) != 1 {
		t.Fatalf("nodeA inbounds: %v", inA)
	}
	in := inA[0].(map[string]any)
	if in["type"] != "vless" || in["tag"] != "in-a" || in["listen_port"].(float64) != 443 {
		t.Fatalf("nodeA inbound: %v", in)
	}
	users := in["users"].([]any)
	if len(users) != 1 || users[0].(map[string]any)["uuid"] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("users: %v", users)
	}
	tls := in["tls"].(map[string]any)
	reality := tls["reality"].(map[string]any)
	if reality["enabled"] != true || reality["private_key"] == "" {
		t.Fatalf("reality: %v", reality)
	}
	if hs := reality["handshake"].(map[string]any); hs["server"] != "www.microsoft.com" || hs["server_port"].(float64) != 443 {
		t.Fatalf("handshake: %v", hs)
	}

	outsA := cfgA["outbounds"].([]any)
	if len(outsA) != 2 { // out-a + авто-direct
		t.Fatalf("nodeA outbounds: %v", outsA)
	}
	var outA map[string]any
	for _, o := range outsA {
		if o.(map[string]any)["tag"] == "out-a" {
			outA = o.(map[string]any)
		}
	}
	if outA == nil {
		t.Fatal("out-a not found")
	}
	// каскад: server/port из целевого inbound
	if outA["server"] != "b.example.com" || outA["server_port"].(float64) != 8443 {
		t.Fatalf("out-a server fields: %v", outA)
	}
	if outA["uuid"] != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("out-a uuid: %v", outA["uuid"])
	}
	// reality зеркалирован: public_key выведен из private_key, short_id взят
	otls := outA["tls"].(map[string]any)
	if _, hasReality := otls["reality"]; hasReality {
		t.Fatal("out-a must not have reality (target in-b has plain TLS)")
	}

	// правило роутинга: in-a → out-a
	rules := cfgA["route"].(map[string]any)["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("nodeA rules: %v", rules)
	}
	rule := rules[0].(map[string]any)
	if rule["inbound"].([]any)[0] != "in-a" || rule["outbound"] != "out-a" {
		t.Fatalf("rule: %v", rule)
	}
	// final — авто-direct
	if cfgA["route"].(map[string]any)["final"] != DefaultDirectTag {
		t.Fatalf("final: %v", cfgA["route"])
	}

	// --- нода B ---
	var cfgB map[string]any
	if err := json.Unmarshal([]byte(configs["nodeB"]), &cfgB); err != nil {
		t.Fatal(err)
	}
	inB := cfgB["inbounds"].([]any)[0].(map[string]any)
	if inB["tag"] != "in-b" || inB["listen_port"].(float64) != 8443 {
		t.Fatalf("in-b: %v", inB)
	}
	// TLS с сертификатом
	if btls := inB["tls"].(map[string]any); btls["certificate"].([]any)[0] != "PEM-B" {
		t.Fatalf("in-b tls: %v", btls)
	}
	// out-b direct + правило
	var foundRule bool
	for _, rr := range cfgB["route"].(map[string]any)["rules"].([]any) {
		m := rr.(map[string]any)
		if m["inbound"].([]any)[0] == "in-b" && m["outbound"] == "out-b" {
			foundRule = true
		}
	}
	if !foundRule {
		t.Fatalf("in-b → out-b rule missing: %v", cfgB["route"])
	}
	if cfgB["route"].(map[string]any)["final"] != "out-b" {
		t.Fatalf("final should be out-b: %v", cfgB["route"])
	}
}

func mustRealityPair(t *testing.T) (string, string, string) {
	t.Helper()
	priv, pub, err := GenerateRealityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return priv, pub, pub
}

// TestGenerateRealityMirror — reality на целевом inbound зеркалируется в outbound.
func TestGenerateRealityMirror(t *testing.T) {
	priv, pub, _ := mustRealityPair(t)
	st := State{
		Nodes: []Node{
			{ID: "in1", NodeID: "n1", Kind: KindInbound, Protocol: "vless", Tag: "in1", Entry: true,
				Settings: mustJSON(t, InboundSettings{ListenPort: 1000, Users: []InboundUser{{Name: "u", UUID: "u1"}}})},
			{ID: "o1", NodeID: "n1", Kind: KindOutbound, Protocol: "vless", Tag: "o1"},
			{ID: "in2", NodeID: "n2", Kind: KindInbound, Protocol: "vless", Tag: "in2",
				Settings: mustJSON(t, InboundSettings{ListenPort: 2000, PublicHost: "re.example.com",
					Users: []InboundUser{{Name: "u", UUID: "u2"}},
					TLS: &InboundTLS{Enabled: true, ServerName: "re.example.com", Reality: &RealityIn{
						Enabled: true, PrivateKey: priv, ShortIDs: []string{"dead"},
						HandshakeServer: "www.apple.com", HandshakePort: 443,
					}}})},
			{ID: "o2", NodeID: "n2", Kind: KindOutbound, Protocol: "direct", Tag: "o2", Exit: true},
		},
		Edges: []Edge{
			{SourceID: "in1", TargetID: "o1"},
			{SourceID: "o1", TargetID: "in2"},
			{SourceID: "in2", TargetID: "o2"},
		},
	}
	phys := []PhysNode{{ID: "n1", Name: "1", GRPCURL: "n1:6237"}, {ID: "n2", Name: "2", GRPCURL: "n2:6237"}}

	configs, err := Generate(st, phys)
	if err != nil {
		t.Fatal(err)
	}
	var cfgA map[string]any
	if err := json.Unmarshal([]byte(configs["n1"]), &cfgA); err != nil {
		t.Fatal(err)
	}
	var o1 map[string]any
	for _, o := range cfgA["outbounds"].([]any) {
		if o.(map[string]any)["tag"] == "o1" {
			o1 = o.(map[string]any)
		}
	}
	tls := o1["tls"].(map[string]any)
	reality := tls["reality"].(map[string]any)
	if reality["public_key"] != pub {
		t.Fatalf("public_key mismatch: got %v want %v", reality["public_key"], pub)
	}
	if reality["short_id"] != "dead" {
		t.Fatalf("short_id: %v", reality["short_id"])
	}
	// utls-fingerprint по умолчанию chrome при reality
	if utls := tls["utls"].(map[string]any); utls["fingerprint"] != "chrome" {
		t.Fatalf("utls: %v", utls)
	}
	// server_name зеркалирован
	if tls["server_name"] != "re.example.com" {
		t.Fatalf("server_name: %v", tls["server_name"])
	}
}

// TestGenerateRealityHandshakePortDefault — при незаданном handshake_port
// в конфиг уходит 443, а не 0.
func TestGenerateRealityHandshakePortDefault(t *testing.T) {
	priv, _, _ := mustRealityPair(t)
	st := State{
		Nodes: []Node{
			{ID: "in1", NodeID: "n1", Kind: KindInbound, Protocol: "vless", Tag: "in1",
				Settings: mustJSON(t, InboundSettings{
					ListenPort: 443,
					Users:      []InboundUser{{Name: "u", UUID: "u1"}},
					TLS: &InboundTLS{Enabled: true, Reality: &RealityIn{
						Enabled:         true,
						PrivateKey:      priv,
						ShortIDs:        []string{"ab"},
						HandshakeServer: "www.apple.com",
					}},
				})},
			{ID: "o1", NodeID: "n1", Kind: KindOutbound, Protocol: "direct", Tag: "o1", Exit: true},
		},
		Edges: []Edge{{SourceID: "in1", TargetID: "o1"}},
	}
	phys := []PhysNode{{ID: "n1", Name: "1", GRPCURL: "n1:6237"}}

	configs, err := Generate(st, phys)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(configs["n1"]), &cfg); err != nil {
		t.Fatal(err)
	}
	in := cfg["inbounds"].([]any)[0].(map[string]any)
	reality := in["tls"].(map[string]any)["reality"].(map[string]any)
	hs := reality["handshake"].(map[string]any)
	if hs["server_port"].(float64) != 443 {
		t.Fatalf("handshake server_port должен быть 443, got: %v", hs)
	}
}

// TestGenerateRulesFromEdges — правила собираются из цепочек inbound→rule→target.
func TestGenerateRulesFromEdges(t *testing.T) {
	priv, _, _ := mustRealityPair(t)
	st := State{
		Nodes: []Node{
			{ID: "in1", NodeID: "n1", Kind: KindInbound, Protocol: "vless", Tag: "in1", Entry: true,
				Settings: mustJSON(t, InboundSettings{ListenPort: 1, Users: []InboundUser{{Name: "u", UUID: "u1"}}})},
			{ID: "rule1", NodeID: "n1", Kind: KindRule, Protocol: "match", Tag: "rule1",
				Settings: mustJSON(t, RuleSettings{DomainSuffix: []string{".ru"}, Network: []string{"tcp"}})},
			{ID: "od", NodeID: "n1", Kind: KindOutbound, Protocol: "direct", Tag: "od"},
			{ID: "bal", NodeID: "n1", Kind: KindBalancer, Protocol: "urltest", Tag: "bal",
				Settings: mustJSON(t, BalancerSettings{Interval: "3m", Tolerance: 50})},
			{ID: "o1", NodeID: "n1", Kind: KindOutbound, Protocol: "vless", Tag: "o1"},
			{ID: "inX", NodeID: "n2", Kind: KindInbound, Protocol: "vless", Tag: "inX",
				Settings: mustJSON(t, InboundSettings{ListenPort: 2, Users: []InboundUser{{Name: "u", UUID: "u2"}},
					TLS: &InboundTLS{Enabled: true, Reality: &RealityIn{Enabled: true, PrivateKey: priv, ShortIDs: []string{"ab"}, HandshakeServer: "h", HandshakePort: 443}}})},
		},
		Edges: []Edge{
			{SourceID: "in1", TargetID: "rule1"},
			{SourceID: "rule1", TargetID: "bal"},
			{SourceID: "bal", TargetID: "o1"},
			{SourceID: "o1", TargetID: "inX"},
		},
	}
	phys := []PhysNode{{ID: "n1", Name: "1", GRPCURL: "n1:6237"}, {ID: "n2", Name: "2", GRPCURL: "n2:6237"}}

	configs, err := Generate(st, phys)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(configs["n1"]), &cfg); err != nil {
		t.Fatal(err)
	}
	rules := cfg["route"].(map[string]any)["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("rules: %v", rules)
	}
	rule := rules[0].(map[string]any)
	if rule["outbound"] != "bal" {
		t.Fatalf("rule outbound: %v", rule)
	}
	if rule["domain_suffix"].([]any)[0] != ".ru" || rule["network"].([]any)[0] != "tcp" {
		t.Fatalf("rule match: %v", rule)
	}

	// urltest-группа с членом o1
	var bal map[string]any
	for _, o := range cfg["outbounds"].([]any) {
		if o.(map[string]any)["tag"] == "bal" {
			bal = o.(map[string]any)
		}
	}
	if bal == nil || bal["type"] != "urltest" {
		t.Fatalf("bal: %v", bal)
	}
	members := bal["outbounds"].([]any)
	if len(members) != 1 || members[0] != "o1" {
		t.Fatalf("members: %v", members)
	}
	if bal["tolerance"].(float64) != 50 || bal["interval"] != "3m" {
		t.Fatalf("urltest settings: %v", bal)
	}
}

// TestGenerateHysteria2AndTUIC — генерация QUIC-протоколов с обязательным TLS.
func TestGenerateHysteria2AndTUIC(t *testing.T) {
	st := State{
		Nodes: []Node{
			// нода 1: вход hysteria2 → каскады на ноду 2 + direct-выход
			{ID: "in-h", NodeID: "n1", Kind: KindInbound, Protocol: "hysteria2", Tag: "in-h", Entry: true,
				Settings: mustJSON(t, InboundSettings{
					ListenPort: 3000, UpMbps: 100, DownMbps: 500, ObfsPassword: "obfs-pw",
					Users: []InboundUser{{Name: "u", Password: "pw"}},
					TLS:   &InboundTLS{Enabled: true, CertPEM: "PEM"},
				})},
			{ID: "o-h", NodeID: "n1", Kind: KindOutbound, Protocol: "hysteria2", Tag: "o-h"},
			{ID: "o-t", NodeID: "n1", Kind: KindOutbound, Protocol: "tuic", Tag: "o-t"},
			{ID: "od1", NodeID: "n1", Kind: KindOutbound, Protocol: "direct", Tag: "od1", Exit: true},
			{ID: "quic-rule", NodeID: "n1", Kind: KindRule, Protocol: "match", Tag: "quic-rule",
				Settings: mustJSON(t, RuleSettings{Network: []string{"udp"}})},
			// нода 2: цели каскадов + direct-выход
			{ID: "in-h2", NodeID: "n2", Kind: KindInbound, Protocol: "hysteria2", Tag: "in-h2",
				Settings: mustJSON(t, InboundSettings{
					ListenPort: 5000, ObfsPassword: "obfs-2",
					Users: []InboundUser{{Name: "u", Password: "pw2"}},
					TLS:   &InboundTLS{Enabled: true, CertPEM: "PEM2"},
				})},
			{ID: "in-t", NodeID: "n2", Kind: KindInbound, Protocol: "tuic", Tag: "in-t",
				Settings: mustJSON(t, InboundSettings{
					ListenPort: 4000, CongestionControl: "bbr",
					Users: []InboundUser{{Name: "u", UUID: "u-1", Password: "tpw"}},
					TLS:   &InboundTLS{Enabled: true, CertPEM: "PEEM"},
				})},
			{ID: "od", NodeID: "n2", Kind: KindOutbound, Protocol: "direct", Tag: "od", Exit: true},
		},
		Edges: []Edge{
			{SourceID: "in-h", TargetID: "o-h"},
			{SourceID: "in-h", TargetID: "quic-rule"},
			{SourceID: "quic-rule", TargetID: "o-t"},
			{SourceID: "o-h", TargetID: "in-h2"},
			{SourceID: "o-t", TargetID: "in-t"},
			{SourceID: "in-h2", TargetID: "od"},
			{SourceID: "in-t", TargetID: "od"},
		},
	}
	phys := []PhysNode{{ID: "n1", Name: "1", GRPCURL: "n1:6237"}, {ID: "n2", Name: "2", GRPCURL: "n2:6237"}}

	configs, err := Generate(st, phys)
	if err != nil {
		t.Fatal(err)
	}
	var cfg1 map[string]any
	if err := json.Unmarshal([]byte(configs["n1"]), &cfg1); err != nil {
		t.Fatal(err)
	}
	inH := cfg1["inbounds"].([]any)[0].(map[string]any)
	if inH["type"] != "hysteria2" || inH["up_mbps"].(float64) != 100 {
		t.Fatalf("in-h: %v", inH)
	}
	if obfs := inH["obfs"].(map[string]any); obfs["password"] != "obfs-pw" {
		t.Fatalf("obfs: %v", obfs)
	}
	// o-h (relay на in-h2): password от пользователя цели, obfs зеркалирован
	var oH map[string]any
	for _, o := range cfg1["outbounds"].([]any) {
		if o.(map[string]any)["tag"] == "o-h" {
			oH = o.(map[string]any)
		}
	}
	if oH["password"] != "pw2" {
		t.Fatalf("o-h: %v", oH)
	}
	if obfs := oH["obfs"].(map[string]any); obfs["password"] != "obfs-2" {
		t.Fatalf("o-h obfs: %v", obfs)
	}
	if oH["server_port"].(float64) != 5000 {
		t.Fatalf("o-h port: %v", oH)
	}
	// o-t (relay на tuic): uuid+password от пользователя цели, bbr
	var oT map[string]any
	for _, o := range cfg1["outbounds"].([]any) {
		if o.(map[string]any)["tag"] == "o-t" {
			oT = o.(map[string]any)
		}
	}
	if oT["uuid"] != "u-1" || oT["password"] != "tpw" || oT["congestion_control"] != "bbr" {
		t.Fatalf("o-t: %v", oT)
	}
}

// TestRelayUserNoClients — каскад со служебным relay_user:
// целевой inbound без клиентских пользователей (их дают «Пользователи»), relay
// подключается креденитью relay_user, а не Users[0].
func TestRelayUserNoClients(t *testing.T) {
	st, phys := twoNodeCascade(t)
	st.Nodes[2].Settings = mustJSON(t, InboundSettings{
		ListenPort: 8443, PublicHost: "b.example.com",
		RelayUser: &InboundUser{Name: "relay", UUID: "rrrrrrrr-rrrr-rrrr-rrrr-rrrrrrrrrrrr", Password: "relay-secret"},
		TLS:       &InboundTLS{Enabled: true, ServerName: "b.example.com", CertPEM: "PEM-B"},
	})

	configs, err := Generate(st, phys)
	if err != nil {
		t.Fatal(err)
	}
	var cfgA, cfgB map[string]any
	if err := json.Unmarshal([]byte(configs["nodeA"]), &cfgA); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(configs["nodeB"]), &cfgB); err != nil {
		t.Fatal(err)
	}

	// relay outbound подключается как relay_user
	var outA map[string]any
	for _, o := range cfgA["outbounds"].([]any) {
		if o.(map[string]any)["tag"] == "out-a" {
			outA = o.(map[string]any)
		}
	}
	if outA == nil || outA["uuid"] != "rrrrrrrr-rrrr-rrrr-rrrr-rrrrrrrrrrrr" {
		t.Fatalf("relay creds must come from relay_user, got %v", outA)
	}

	// целевой inbound пускает relay_user
	inB := cfgB["inbounds"].([]any)[0].(map[string]any)
	users := inB["users"].([]any)
	found := false
	for _, u := range users {
		if u.(map[string]any)["uuid"] == "rrrrrrrr-rrrr-rrrr-rrrr-rrrrrrrrrrrr" {
			found = true
		}
	}
	if !found {
		t.Fatalf("target inbound must include relay_user: %v", users)
	}

	// валидация каскада без клиентских users не падает (relay_user закрывает креды)
	res := (&Validator{State: st, Nodes: phys}).Validate()
	if res.HasErrors() {
		t.Fatalf("cascade with relay_user must validate: %+v", res.Errors)
	}
}

func TestGenerateRejectsInvalid(t *testing.T) {
	st := State{Nodes: []Node{
		{ID: "x", NodeID: "n1", Kind: KindOutbound, Protocol: "vless", Tag: "x"},
	}}
	_, err := Generate(st, []PhysNode{{ID: "n1", Name: "1", GRPCURL: "n:1"}})
	if err == nil || !strings.Contains(err.Error(), "невалиден") {
		t.Fatalf("want validation error, got %v", err)
	}
}

func TestRealityPublicKey(t *testing.T) {
	priv, pub, err := GenerateRealityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	derived, err := RealityPublicKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	if derived != pub {
		t.Fatalf("derived %s != generated %s", derived, pub)
	}
	if _, err := RealityPublicKey("!!!not base64!!!"); err == nil {
		t.Fatal("invalid base64 must fail")
	}
}

func TestPhysNodeHost(t *testing.T) {
	p := PhysNode{GRPCURL: "10.0.0.1:6237"}
	if p.Host() != "10.0.0.1" {
		t.Fatalf("host: %q", p.Host())
	}
	p = PhysNode{GRPCURL: "[2001:db8::1]:6237"}
	if p.Host() != "2001:db8::1" {
		t.Fatalf("IPv6 host: %q", p.Host())
	}
	p = PhysNode{GRPCURL: "2001:db8::1"}
	if p.Host() != "2001:db8::1" {
		t.Fatalf("bare IPv6 fallback: %q", p.Host())
	}
	p = PhysNode{GRPCURL: "no-port"}
	if p.Host() != "no-port" {
		t.Fatalf("host fallback: %q", p.Host())
	}
}
