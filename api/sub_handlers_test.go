package api

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/CascadiaLabs/panel/graph"
)

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// subscriptionFixture — минимальный граф для тестов клиентского конфига.
func subscriptionFixture(t *testing.T) (graph.State, []graph.PhysNode, graph.PanelCreds) {
	t.Helper()
	priv, _, err := graph.GenerateRealityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	creds := graph.PanelCreds{Name: "u", UUID: "5d156f8f-39c0-470c-9d52-8b70f9ab8a9a", Flow: "xtls-rprx-vision"}
	st := graph.State{
		Nodes: []graph.Node{
			{ID: "in-1", NodeID: "node1", Kind: graph.KindInbound, Protocol: "vless", Tag: "in-1", Entry: true,
				Settings: mustJSON(t, graph.InboundSettings{
					ListenPort: 443, PublicHost: "example.com",
					Users:      []graph.InboundUser{{Name: creds.Name, UUID: creds.UUID, Flow: creds.Flow}},
					TLS: &graph.InboundTLS{Enabled: true, ServerName: "icloud.com", Reality: &graph.RealityIn{
						Enabled: true, PrivateKey: priv, ShortIDs: []string{"022e8c7c"},
						HandshakeServer: "icloud.com", HandshakePort: 443,
					}},
				})},
			{ID: "out-1", NodeID: "node1", Kind: graph.KindOutbound, Protocol: "direct", Tag: "direct", Exit: true},
		},
		Edges: []graph.Edge{{ID: "e1", SourceID: "in-1", TargetID: "out-1"}},
	}
	phys := []graph.PhysNode{{ID: "node1", Name: "Node 1", GRPCURL: "node1.example:6237"}}
	return st, phys, creds
}

// TestGetSingBoxSubscriptionConfig114 — клиентский JSON-конфиг subscription
// должен быть валиден для sing-box 1.14 и сохранять маршрутизацию RU-доменов в direct.
func TestGetSingBoxSubscriptionConfig114(t *testing.T) {
	st, phys, creds := subscriptionFixture(t)
	cfg, err := GetSingBoxSubscriptionConfig(st, phys, creds)
	if err != nil {
		t.Fatal(err)
	}

	// --- топ-уровневый http_clients: detour без dial ---
	hc, ok := cfg["http_clients"].([]map[string]any)
	if !ok || len(hc) != 1 {
		t.Fatalf("top-level http_clients missing or wrong: %v", cfg["http_clients"])
	}
	hco := hc[0]
	if hco["detour"] != "direct" {
		t.Fatalf("http_clients[0].detour = %q, want direct", hco["detour"])
	}
	if _, has := hco["dial"]; has {
		t.Fatal("http_clients[0] must not contain 'dial'")
	}

	// --- dns: нет default / default_domain_resolver ---
	dns, ok := cfg["dns"].(map[string]any)
	if !ok {
		t.Fatal("dns missing")
	}
	for _, k := range []string{"default", "default_domain_resolver"} {
		if _, has := dns[k]; has {
			t.Fatalf("dns.%s must not exist", k)
		}
	}
	if dns["final"] != "dns-remote" {
		t.Fatalf("dns.final = %q, want dns-remote", dns["final"])
	}

	// --- route: default_http_client, без http_clients, default_domain_resolver ---
	route, ok := cfg["route"].(map[string]any)
	if !ok {
		t.Fatal("route missing")
	}
	if route["default_http_client"] != "direct-http-client" {
		t.Fatalf("route.default_http_client = %q, want direct-http-client", route["default_http_client"])
	}
	if _, has := route["http_clients"]; has {
		t.Fatal("route.http_clients must not exist (must be top-level)")
	}
	if route["default_domain_resolver"] != "dns-bootstrap" {
		t.Fatalf("route.default_domain_resolver = %q, want dns-bootstrap", route["default_domain_resolver"])
	}
	if route["final"] != "proxy-group" {
		t.Fatalf("route.final = %q, want proxy-group", route["final"])
	}

	// --- rule_set с http_client ---
	rss, ok := route["rule_set"].([]map[string]any)
	if !ok || len(rss) < 2 {
		t.Fatalf("route.rule_set missing: %v", route["rule_set"])
	}
	for _, rs := range rss {
		if rs["http_client"] != "direct-http-client" {
			t.Fatalf("rule_set %v missing http_client", rs["tag"])
		}
		if rs["update_interval"] != "7d" {
			t.Fatalf("rule_set %v missing update_interval=7d", rs["tag"])
		}
	}

	// --- route.rules: sniff, hijack-dns, domain_suffix ru → direct, geoip-ru → direct ---
	rules, ok := route["rules"].([]map[string]any)
	if !ok {
		t.Fatal("route.rules missing")
	}
	haveSniff, haveHijack, haveSuffix, haveGeoip := false, false, false, false
	for _, m := range rules {
		switch m["action"] {
		case "sniff":
			haveSniff = true
		case "hijack-dns":
			haveHijack = true
		case "route":
			ds, _ := m["domain_suffix"].([]string)
			if len(ds) > 0 && ds[0] == "ru" {
				haveSuffix = true
			}
			if rs, ok := m["rule_set"].([]string); ok && len(rs) > 0 && rs[0] == "geoip-ru" {
				haveGeoip = true
			}
		}
	}
	if !haveSniff {
		t.Fatal("missing sniff rule")
	}
	if !haveHijack {
		t.Fatal("missing hijack-dns rule")
	}
	if !haveSuffix {
		t.Fatal("missing domain_suffix [ru] -> direct rule")
	}
	if !haveGeoip {
		t.Fatal("missing geoip-ru -> direct rule")
	}

	// --- tun inbound без legacy sniff ---
	inbounds, ok := cfg["inbounds"].([]map[string]any)
	if !ok || len(inbounds) == 0 {
		t.Fatal("inbounds missing")
	}
	tun := inbounds[0]
	if tun["type"] != "tun" {
		t.Fatalf("inbound[0].type = %q, want tun", tun["type"])
	}
	if _, has := tun["sniff"]; has {
		t.Fatal("tun inbound must not contain legacy 'sniff' field")
	}

	// --- outbounds: selector + direct ---
	outs, ok := cfg["outbounds"].([]map[string]any)
	if !ok || len(outs) < 2 {
		t.Fatalf("outbounds missing: %v", len(outs))
	}
	haveSelector, haveDirect := false, false
	for _, o := range outs {
		if o["type"] == "selector" {
			haveSelector = true
		}
		if o["type"] == "direct" {
			haveDirect = true
		}
	}
	if !haveSelector {
		t.Fatal("missing selector outbound proxy-group")
	}
	if !haveDirect {
		t.Fatal("missing direct outbound")
	}

	// --- round-trip: JSON marshal/unmarshal не падает ---
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal("json.Marshal failed:", err)
	}
	var again map[string]any
	if err := json.Unmarshal(b, &again); err != nil {
		t.Fatal("round-trip failed:", err)
	}
	_ = again
}

// TestSingboxCheckClientConfig — gold-standard: реальный бинарник sing-box
// валидирует сгенерированный клиентский конфиг (SINGBOX_BIN=<path>, иначе skip).
func TestSingboxCheckClientConfig(t *testing.T) {
	bin := os.Getenv("SINGBOX_BIN")
	if bin == "" {
		bin = "sing-box"
	}
	if _, err := exec.LookPath(bin); err != nil {
		t.Skip("sing-box binary not found; set SINGBOX_BIN to enable")
	}
	st, phys, creds := subscriptionFixture(t)
	cfg, err := GetSingBoxSubscriptionConfig(st, phys, creds)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(t.TempDir(), "client.json")
	if err := os.WriteFile(f, b, 0644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, "check", "-c", f).CombinedOutput()
	if err != nil {
		t.Fatalf("sing-box check client config: %v\n%s", err, out)
	}
}
