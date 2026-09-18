package graph

import (
	"strings"
	"testing"
)

const testUUID = "11111111-2222-3333-4444-555555555555"

func creds() PanelCreds {
	return PanelCreds{Name: "ivan", UUID: testUUID, Password: "p@ss", Flow: "", Remark: "Иван · VIP"}
}

func stateWithInbound(protocol string, settings map[string]any) State {
	return State{
		Nodes: []Node{{
			ID:       "n1",
			NodeID:   "phys1",
			Kind:     KindInbound,
			Protocol: protocol,
			Tag:      "in-1",
			Entry:    true,
		}},
	}
}

func TestShareLinksVlessRealityWS(t *testing.T) {
	priv, _, err := GenerateRealityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	raw := mustJSON(t, map[string]any{
		"listen_port": 8443,
		"public_host": "vpn.example.com",
		"users":       []map[string]any{{"name": "x", "uuid": testUUID}},
		"tls": map[string]any{
			"enabled": true, "server_name": "cdn.example.com",
			"reality": map[string]any{
				"enabled": true, "private_key": priv, "short_ids": []string{"abcd1234"},
			},
		},
		"transport": map[string]any{"type": "ws", "path": "/ws", "host": "cdn.example.com"},
	})
	state := stateWithInbound("vless", nil)
	state.Nodes[0].Settings = raw
	// реальная нода нужна для host(), но public_host уже задан
	links, err := ShareLinks(state, nil, creds())
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Fatalf("want 1 link, got %d", len(links))
	}
	u := links[0]
	for _, want := range []string{
		"vless://" + testUUID + "@vpn.example.com:8443",
		"security=reality",
		"pbk=",
		"sid=abcd1234",
		"type=ws",
		"path=%2Fws",
		// имя профиля — тег inbound из графа, а не имя/remark пользователя
		"#in-1",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("link %q missing %q", u, want)
		}
	}
}

func TestShareLinksVlessDefaultTCP(t *testing.T) {
	// регрессия: inbound без транспорта (tcp) обязан явно задавать type/headerType —
	// v2rayTun иначе создаёт пустую подписку ("transport method cannot be empty").
	raw := mustJSON(t, map[string]any{
		"listen_port": 44300, "public_host": "d.example.com",
		"users": []map[string]any{{"name": "x", "uuid": testUUID}},
		"tls":   map[string]any{"enabled": true, "server_name": "d.example.com"},
	})
	state := stateWithInbound("vless", nil)
	state.Nodes[0].Settings = raw
	links, err := ShareLinks(state, nil, creds())
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Fatalf("want 1 link, got %d", len(links))
	}
	for _, want := range []string{"type=tcp", "headerType=none"} {
		if !strings.Contains(links[0], want) {
			t.Errorf("default tcp link must set type=tcp&headerType=none, got: %s", links[0])
		}
	}
}

func TestShareLinksOrdersInboundsBySubscriptionOrder(t *testing.T) {
	settings := func(order int) []byte {
		return mustJSON(t, map[string]any{
			"listen_port": 443, "public_host": "vpn.example.com",
			"subscription_order": order,
		})
	}
	state := State{Nodes: []Node{
		{ID: "third", Kind: KindInbound, Protocol: "vless", Tag: "z-third", Entry: true, Settings: settings(30)},
		{ID: "first", Kind: KindInbound, Protocol: "vless", Tag: "m-first", Entry: true, Settings: settings(10)},
		{ID: "second", Kind: KindInbound, Protocol: "vless", Tag: "a-second", Entry: true, Settings: settings(20)},
	}}
	links, err := ShareLinks(state, nil, creds())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(links), 3; got != want {
		t.Fatalf("want %d links, got %d", want, got)
	}
	for i, tag := range []string{"#m-first", "#a-second", "#z-third"} {
		if !strings.HasSuffix(links[i], tag) {
			t.Errorf("link %d: want suffix %q, got %q", i, tag, links[i])
		}
	}
}

func TestShareLinksSSTrojanHysteria2(t *testing.T) {
	ss := mustJSON(t, map[string]any{
		"listen_port": 8388, "public_host": "a.example.com", "method": "2022-blake3-aes-128-gcm",
		"users": []map[string]any{{"name": "x", "password": "p@ss"}},
	})
	tr := mustJSON(t, map[string]any{
		"listen_port": 443, "public_host": "b.example.com",
		"users":     []map[string]any{{"name": "x", "password": "p@ss"}},
		"tls":       map[string]any{"enabled": true, "server_name": "t.example.com"},
		"transport": map[string]any{"type": "grpc", "service_name": "svc"},
	})
	hy2 := mustJSON(t, map[string]any{
		"listen_port": 443, "public_host": "c.example.com",
		"obfs_password": "sal", "tls": map[string]any{"enabled": true},
		"users": []map[string]any{{"name": "x", "password": "p@ss"}},
	})

	ssState := stateWithInbound("shadowsocks", nil)
	ssState.Nodes[0].Settings = ss
	links, _ := ShareLinks(ssState, nil, creds())
	if len(links) != 1 || !strings.HasPrefix(links[0], "ss://") || !strings.HasSuffix(links[0], "#in-1") {
		t.Errorf("ss link wrong: %v", links)
	}

	trState := stateWithInbound("trojan", nil)
	trState.Nodes[0].Settings = tr
	links, _ = ShareLinks(trState, nil, creds())
	if len(links) != 1 || !strings.Contains(links[0], "security=tls") || !strings.Contains(links[0], "serviceName=svc") {
		t.Errorf("trojan link wrong: %v", links)
	}

	hy2State := stateWithInbound("hysteria2", nil)
	hy2State.Nodes[0].Settings = hy2
	links, _ = ShareLinks(hy2State, nil, creds())
	if len(links) != 1 || !strings.Contains(links[0], "hysteria2://p@ss@c.example.com:443") || !strings.Contains(links[0], "insecure=1") || !strings.Contains(links[0], "obfs-password=sal") {
		t.Errorf("hy2 link wrong: %v", links)
	}
}

func TestInjectRemoveRoundTrip(t *testing.T) {
	raw := mustJSON(t, map[string]any{
		"listen_port": 443, "public_host": "v.example.com",
		"users": []map[string]any{{"name": "manual", "uuid": "00000000-0000-0000-0000-000000000000"}},
	})
	st := stateWithInbound("vless", nil)
	st.Nodes[0].Settings = raw

	injected := InjectUsersIntoState(st, creds())
	injected2 := InjectUsersIntoState(injected, creds())
	in, _ := ParseInboundSettings(injected2.Nodes[0].Settings)
	if len(in.Users) != 2 {
		t.Fatalf("inject must be idempotent: got %d users", len(in.Users))
	}

	pruned := RemoveUserFromState(injected2, creds())
	in, _ = ParseInboundSettings(pruned.Nodes[0].Settings)
	if len(in.Users) != 1 || in.Users[0].Name != "manual" {
		t.Fatalf("remove wrong: %+v", in.Users)
	}
	// креды другой ноды того же пользователя не должны быть в uid-протоколе затронуты
}
