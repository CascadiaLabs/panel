package graph

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// RouteRuleItem — правило маршрутизации sing-box (соответствует db.RouteRuleItem).
type RouteRuleItem struct {
	Name           string   `json:"name"`
	Action         string   `json:"action"`
	Outbounds      []string `json:"outbounds,omitempty"`
	Domain         []string `json:"domain,omitempty"`
	DomainSuffix   []string `json:"domain_suffix,omitempty"`
	DomainKeyword  []string `json:"domain_keyword,omitempty"`
	DomainRegex    []string `json:"domain_regex,omitempty"`
	IPCIDR         []string `json:"ip_cidr,omitempty"`
	SourceIPCIDR   []string `json:"source_ip_cidr,omitempty"`
	Port           []string `json:"port,omitempty"`
	SourcePort     []string `json:"source_port,omitempty"`
	Network        []string `json:"network,omitempty"`
	Protocol       []string `json:"protocol,omitempty"`
	Process        []string `json:"process,omitempty"`
	ProcessPath    []string `json:"process_path,omitempty"`
	PackageName    []string `json:"package_name,omitempty"`
	UID            []string `json:"uid,omitempty"`
	GID            []string `json:"gid,omitempty"`
	NetworkType    []string `json:"network_type,omitempty"`
	Inbound        []string `json:"inbound,omitempty"`
	Final          bool     `json:"final,omitempty"`
}

// InboundRouteRules — правила маршрутизации, назначенные на inbound'ы.
// Ключ — ID inbound-элемента графа, значение — список правил.
type InboundRouteRules map[string][]RouteRuleItem

// Generate компилирует граф в map[физическая нода]конфиг sing-box (нативный JSON).
// Граф обязан пройти валидацию; при ошибках возвращается список.
// inboundRouteRules — дополнительные правила маршрутизации, назначенные на inbound'ы через БД.
func Generate(st State, phys []PhysNode, inboundRouteRules InboundRouteRules) (map[string]string, error) {
	vd := Validator{State: st, Nodes: phys}
	res := vd.Validate()
	if res.HasErrors() {
		msgs := make([]string, 0, len(res.Errors))
		for _, e := range res.Errors {
			msgs = append(msgs, e.Message)
		}
		return nil, fmt.Errorf("граф невалиден (%d ошибок): %s", len(res.Errors), strings.Join(msgs, "; "))
	}

	// Validate the original topology before inserting compiler-only relay nodes.
	st = expandShorthand(st)
	byID := map[string]Node{}
	for _, n := range st.Nodes {
		byID[n.ID] = n
	}

	// каскадные цели outbound'ов: source element id → inbound
	cascadeTarget := map[string]Node{}
	for _, e := range st.Edges {
		if s, ok := byID[e.SourceID]; ok && s.Kind == KindOutbound {
			if t, ok := byID[e.TargetID]; ok && t.Kind == KindInbound {
				cascadeTarget[s.ID] = t // validation guarantees one relay target
			}
		}
	}

	// физические ноды, задействованные в графе
	physByID := map[string]PhysNode{}
	for _, p := range phys {
		physByID[p.ID] = p
	}
	usedNodes := map[string]bool{}
	for _, n := range st.Nodes {
		usedNodes[n.NodeID] = true
	}

	configs := map[string]string{}
	// стабильный порядок обхода: по имени физической ноды
	usedList := make([]string, 0, len(usedNodes))
	for id := range usedNodes {
		usedList = append(usedList, id)
	}
	sort.Slice(usedList, func(i, j int) bool {
		return physByID[usedList[i]].Name < physByID[usedList[j]].Name
	})

	for _, physID := range usedList {
		cfg, err := generateNodeConfig(st, physID, physByID, byID, cascadeTarget, inboundRouteRules)
		if err != nil {
			return nil, fmt.Errorf("нода %s: %w", physByID[physID].Name, err)
		}
		out, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return nil, err
		}
		configs[physID] = string(out) + "\n"
	}
	return configs, nil
}

// generateNodeConfig строит конфиг sing-box для одной физической ноды.
func generateNodeConfig(st State, physID string, physByID map[string]PhysNode, byID map[string]Node, cascadeTarget map[string]Node, inboundRouteRules InboundRouteRules) (map[string]any, error) {
	var inbounds, outbounds, rules []map[string]any

	mine := func(n Node) bool { return n.NodeID == physID }

	// порядок элементов детерминирован: pos_y, затем tag
	sorted := func(ns []Node) []Node {
		out := append([]Node{}, ns...)
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].PosY != out[j].PosY {
				return out[i].PosY < out[j].PosY
			}
			return out[i].Tag < out[j].Tag
		})
		return out
	}

	var inboundEls, outboundEls, balancerEls, ruleEls []Node
	for _, n := range st.Nodes {
		if !mine(n) {
			continue
		}
		switch n.Kind {
		case KindInbound:
			inboundEls = append(inboundEls, n)
		case KindOutbound:
			outboundEls = append(outboundEls, n)
		case KindBalancer:
			balancerEls = append(balancerEls, n)
		case KindRule:
			ruleEls = append(ruleEls, n)
		}
	}

	// рёбра ноды
	targetsOf := func(id string) []Node {
		var out []Node
		for _, e := range st.Edges {
			if e.SourceID == id {
				if t, ok := byID[e.TargetID]; ok {
					out = append(out, t)
				}
			}
		}
		return sorted(out)
	}

	// --- inbounds ---
	for _, el := range sorted(inboundEls) {
		in, err := ParseInboundSettings(el.Settings)
		if err != nil {
			return nil, fmt.Errorf("inbound %s: %w", el.Tag, err)
		}
		m := map[string]any{
			"type":        el.Protocol,
			"tag":         el.Tag,
			"listen":      "::",
			"listen_port": in.ListenPort,
		}
		switch el.Protocol {
		case "vless":
			users := make([]map[string]any, 0, len(in.Users))
			for _, u := range inboundUsers(in) {
				uu := map[string]any{"name": u.Name, "uuid": u.UUID}
				if u.Flow != "" {
					uu["flow"] = u.Flow
				}
				users = append(users, uu)
			}
			m["users"] = users
		case "vmess":
			users := make([]map[string]any, 0, len(in.Users))
			for _, u := range inboundUsers(in) {
				users = append(users, map[string]any{"name": u.Name, "uuid": u.UUID})
			}
			m["users"] = users
		case "trojan":
			users := make([]map[string]any, 0, len(in.Users))
			for _, u := range inboundUsers(in) {
				users = append(users, map[string]any{"name": u.Name, "password": u.Password})
			}
			m["users"] = users
		case "shadowsocks":
			m["method"] = in.Method
			if in.Network != "" {
				m["network"] = in.Network
			}
			users := make([]map[string]any, 0, len(in.Users))
			for _, u := range inboundUsers(in) {
				users = append(users, map[string]any{"name": u.Name, "password": u.Password})
			}
			m["users"] = users
		case "hysteria2":
			users := make([]map[string]any, 0, len(in.Users))
			for _, u := range inboundUsers(in) {
				users = append(users, map[string]any{"name": u.Name, "password": u.Password})
			}
			m["users"] = users
			if in.UpMbps > 0 {
				m["up_mbps"] = in.UpMbps
			}
			if in.DownMbps > 0 {
				m["down_mbps"] = in.DownMbps
			}
			if in.ObfsPassword != "" {
				m["obfs"] = map[string]any{"type": "salamander", "password": in.ObfsPassword}
			}
		case "tuic":
			users := make([]map[string]any, 0, len(in.Users))
			for _, u := range inboundUsers(in) {
				users = append(users, map[string]any{"name": u.Name, "uuid": u.UUID, "password": u.Password})
			}
			m["users"] = users
			if in.CongestionControl != "" {
				m["congestion_control"] = in.CongestionControl
			}
		}
		if tls := inboundTLSBlock(in); tls != nil {
			m["tls"] = tls
		}
		if tr := transportBlock(in.Transport); tr != nil {
			m["transport"] = tr
		}
		inbounds = append(inbounds, m)
	}

	// --- balancers (urltest/selector) ---
	for _, el := range sorted(balancerEls) {
		s, err := ParseBalancerSettings(el.Settings)
		if err != nil {
			return nil, fmt.Errorf("balancer %s: %w", el.Tag, err)
		}
		var members []string
		for _, t := range targetsOf(el.ID) {
			if t.Kind == KindOutbound {
				members = append(members, t.Tag)
			}
		}
		if len(members) == 0 {
			return nil, fmt.Errorf("balancer %s без членов", el.Tag)
		}
		m := map[string]any{"type": el.Protocol, "tag": el.Tag, "outbounds": members}
		if el.Protocol == "urltest" {
			if s.URL != "" {
				m["url"] = s.URL
			}
			if s.Interval != "" {
				m["interval"] = s.Interval
			}
			if s.Tolerance > 0 {
				m["tolerance"] = s.Tolerance
			}
		} else {
			if s.Default != "" {
				m["default"] = s.Default
			}
		}
		outbounds = append(outbounds, m)
	}

	// --- outbounds ---
	directTag := ""
	for _, el := range sorted(outboundEls) {
		if el.Protocol == "direct" {
			if directTag == "" {
				directTag = el.Tag
			}
			outbounds = append(outbounds, map[string]any{"type": "direct", "tag": el.Tag})
			continue
		}

		// relay: поля подключения выводятся из целевого inbound
		if target, isRelay := cascadeTarget[el.ID]; isRelay {
			m, err := relayOutboundBlock(el, target, physByID[target.NodeID])
			if err != nil {
				return nil, err
			}
			outbounds = append(outbounds, m)
			continue
		}

		// exit: ручные настройки
		s, err := ParseOutboundSettings(el.Settings)
		if err != nil {
			return nil, fmt.Errorf("outbound %s: %w", el.Tag, err)
		}
		m := map[string]any{
			"type":        el.Protocol,
			"tag":         el.Tag,
			"server":      s.Server,
			"server_port": s.ServerPort,
		}
		switch el.Protocol {
		case "vless":
			m["uuid"] = s.UUID
			if s.Flow != "" {
				m["flow"] = s.Flow
			}
		case "vmess":
			m["uuid"] = s.UUID
			if s.Security != "" {
				m["security"] = s.Security
			}
		case "trojan":
			m["password"] = s.Password
		case "shadowsocks":
			m["method"] = s.Method
			m["password"] = s.Password
		case "hysteria2":
			m["password"] = s.Password
			if s.ObfsPassword != "" {
				m["obfs"] = map[string]any{"type": "salamander", "password": s.ObfsPassword}
			}
			if s.UpMbps > 0 {
				m["up_mbps"] = s.UpMbps
			}
			if s.DownMbps > 0 {
				m["down_mbps"] = s.DownMbps
			}
		case "tuic":
			m["uuid"] = s.UUID
			m["password"] = s.Password
			if s.CongestionControl != "" {
				m["congestion_control"] = s.CongestionControl
			}
		}
		if tls := outboundTLSBlock(s.TLS); tls != nil {
			m["tls"] = tls
		}
		if tr := transportBlock(s.Transport); tr != nil {
			m["transport"] = tr
		}
		outbounds = append(outbounds, m)
	}

	// Reuse a direct outbound or allocate a collision-safe compiler-only one.
	if directTag == "" && len(inbounds) > 0 {
		used := map[string]bool{}
		for _, n := range st.Nodes {
			used[strings.TrimSpace(n.Tag)] = true
		}
		directTag = uniqueGraphName(DefaultDirectTag, used)
		outbounds = append(outbounds, map[string]any{"type": "direct", "tag": directTag})
	}

	// --- route rules (из рёбер) ---
	// порядок: по pos_y inbound'а, затем pos_y правила, затем pos_y цели.
	type ruleLink struct {
		inbound Node
		rule    *Node
		target  Node
	}
	var links []ruleLink
	for _, in := range sorted(inboundEls) {
		if in.Exit {
			links = append(links, ruleLink{inbound: in, target: Node{Tag: directTag}})
		}
		for _, mid := range targetsOf(in.ID) {
			switch mid.Kind {
			case KindRule:
				for _, t := range targetsOf(mid.ID) {
					if t.Kind == KindOutbound || t.Kind == KindBalancer {
						links = append(links, ruleLink{inbound: in, rule: &[]Node{mid}[0], target: t})
					}
				}
			case KindOutbound, KindBalancer:
				links = append(links, ruleLink{inbound: in, target: mid})
			}
		}
	}
	sort.SliceStable(links, func(i, j int) bool {
		if links[i].inbound.PosY != links[j].inbound.PosY {
			return links[i].inbound.PosY < links[j].inbound.PosY
		}
		if links[i].inbound.Tag != links[j].inbound.Tag {
			return links[i].inbound.Tag < links[j].inbound.Tag
		}
		// Specific rules must run before an inbound's unconditional default.
		if (links[i].rule == nil) != (links[j].rule == nil) {
			return links[i].rule != nil
		}
		ri, rj := 0.0, 0.0
		if links[i].rule != nil {
			ri = links[i].rule.PosY
		}
		if links[j].rule != nil {
			rj = links[j].rule.PosY
		}
		if ri != rj {
			return ri < rj
		}
		return links[i].target.PosY < links[j].target.PosY
	})

	for _, l := range links {
		rule := map[string]any{
			"inbound":  []string{l.inbound.Tag},
			"outbound": l.target.Tag,
		}
		if l.rule != nil {
			rs, err := ParseRuleSettings(l.rule.Settings)
			if err != nil {
				return nil, fmt.Errorf("rule %s: %w", l.rule.Tag, err)
			}
			applyRuleMatch(rule, rs)
		}
		rules = append(rules, rule)
	}

	// --- assigned route rules (from DB) ---
	// Добавляем правила, назначенные на inbound'ы через панель маршрутизации.
	// Они идут ПОСЛЕ графовых правил, но ДО финального direct (route.final).
	for _, in := range sorted(inboundEls) {
		assigned := inboundRouteRules[in.ID]
		for _, item := range assigned {
			rule := map[string]any{
				"inbound": []string{in.Tag},
			}
			if item.Action != "" {
				rule["action"] = item.Action
			}
			if len(item.Outbounds) > 0 {
				rule["outbound"] = item.Outbounds
			}
			if len(item.Domain) > 0 {
				rule["domain"] = item.Domain
			}
			if len(item.DomainSuffix) > 0 {
				rule["domain_suffix"] = item.DomainSuffix
			}
			if len(item.DomainKeyword) > 0 {
				rule["domain_keyword"] = item.DomainKeyword
			}
			if len(item.DomainRegex) > 0 {
				rule["domain_regex"] = item.DomainRegex
			}
			if len(item.IPCIDR) > 0 {
				rule["ip_cidr"] = item.IPCIDR
			}
			if len(item.SourceIPCIDR) > 0 {
				rule["source_ip_cidr"] = item.SourceIPCIDR
			}
			if len(item.Port) > 0 {
				rule["port"] = item.Port
			}
			if len(item.SourcePort) > 0 {
				rule["source_port"] = item.SourcePort
			}
			if len(item.Network) > 0 {
				rule["network"] = item.Network
			}
			if len(item.Protocol) > 0 {
				rule["protocol"] = item.Protocol
			}
			if len(item.Process) > 0 {
				rule["process"] = item.Process
			}
			if len(item.ProcessPath) > 0 {
				rule["process_path"] = item.ProcessPath
			}
			if len(item.PackageName) > 0 {
				rule["package_name"] = item.PackageName
			}
			if len(item.UID) > 0 {
				rule["uid"] = item.UID
			}
			if len(item.GID) > 0 {
				rule["gid"] = item.GID
			}
			if len(item.NetworkType) > 0 {
				rule["network_type"] = item.NetworkType
			}
			if len(item.Inbound) > 0 {
				rule["inbound"] = item.Inbound
			}
			if item.Final {
				rule["final"] = true
			}
			rules = append(rules, rule)
		}
	}

	// --- сортировка правил маршрутизации ---
	// Принцип: конкретные правила (с условиями domain/ip/port) ДО catch-all правил.
	// Внутри групп сохраняем исходный порядок (stable sort).
	sort.SliceStable(rules, func(i, j int) bool {
		hasMatchI := ruleHasMatchConditions(rules[i])
		hasMatchJ := ruleHasMatchConditions(rules[j])
		if hasMatchI != hasMatchJ {
			return hasMatchI // правила с условиями идут раньше
		}
		// Если обе имеют условия или обе catch-all: final=true раньше
		finalI := getBool(rules[i], "final")
		finalJ := getBool(rules[j], "final")
		if finalI != finalJ {
			return finalI
		}
		return false // сохраняем исходный порядок
	})

	cfg := map[string]any{
		"log": map[string]any{"level": "info"},
	}
	if len(inbounds) > 0 {
		cfg["inbounds"] = inbounds
	} else {
		cfg["inbounds"] = []map[string]any{}
	}
	if len(outbounds) > 0 {
		cfg["outbounds"] = outbounds
	} else {
		cfg["outbounds"] = []map[string]any{}
	}
	route := map[string]any{"rules": rules}
	if directTag != "" {
		route["final"] = directTag
	}
	cfg["route"] = route

	// --- DNS configuration with split DNS for .ru domains ---
	cfg["dns"] = buildDNSConfig(outbounds, directTag)

	return cfg, nil
}

// buildDNSConfig создаёт DNS-конфигурацию с split DNS для .ru доменов.
// Использует системный DNS для bootstrap/direct, а для остального — DNS через прокси (если есть).
func buildDNSConfig(outbounds []map[string]any, directTag string) map[string]any {
	// Находим прокси-outbound для remote DNS (первый не-direct outbound)
	var proxyTag string
	for _, ob := range outbounds {
		if t, _ := ob["tag"].(string); t != "" && t != directTag && ob["type"] != "direct" {
			proxyTag = t
			break
		}
	}
	// Если прокси нет, используем directTag для всего
	if proxyTag == "" {
		proxyTag = directTag
	}

	servers := []map[string]any{
		{
			"tag":        "dns-bootstrap",
			"address":    "local",
			"detour":     directTag,
		},
		{
			"tag":        "dns-direct",
			"address":    "local",
			"detour":     directTag,
		},
		{
			"tag":        "dns-remote",
			"address":    "https://1.1.1.1/dns-query",
			"detour":     proxyTag,
		},
	}

	rules := []map[string]any{
		// .ru domains -> direct DNS
		{
			"action":      "route",
			"domain_suffix": []string{".ru", ".su", ".xn--p1ai"},
			"server":      "dns-direct",
		},
		// geosite:ru -> direct DNS
		{
			"action":   "route",
			"rule_set": []string{"geosite:ru"},
			"server":   "dns-direct",
		},
		// Clash Direct mode -> direct DNS
		{
			"action":     "route",
			"clash_mode": "Direct",
			"server":     "dns-direct",
		},
		// Clash Global mode -> remote DNS
		{
			"action":     "route",
			"clash_mode": "Global",
			"server":     "dns-remote",
		},
	}

	return map[string]any{
		"servers": servers,
		"rules":   rules,
		"default": "dns-bootstrap",
		"final":   "dns-remote",
	}
}

// DefaultDirectTag — тег неявного direct-outbound (route.final).
const DefaultDirectTag = "cascadia-direct"

// inboundTLSBlock строит серверный tls-блок sing-box.
func inboundTLSBlock(in InboundSettings) map[string]any {
	if in.TLS == nil || !in.TLS.Enabled {
		return nil
	}
	m := map[string]any{"enabled": true}
	if in.TLS.ServerName != "" {
		m["server_name"] = in.TLS.ServerName
	}
	if len(in.TLS.ALPN) > 0 {
		m["alpn"] = in.TLS.ALPN
	}
	if in.TLS.Reality != nil && in.TLS.Reality.Enabled {
		hsPort := in.TLS.Reality.HandshakePort
		if hsPort == 0 {
			hsPort = 443 // дефолт reality handshake, если не задан
		}
		m["reality"] = map[string]any{
			"enabled":     true,
			"handshake":   map[string]any{"server": in.TLS.Reality.HandshakeServer, "server_port": hsPort},
			"private_key": in.TLS.Reality.PrivateKey,
			"short_id":    in.TLS.Reality.ShortIDs,
		}
		return m
	}
	if in.TLS.CertPEM != "" {
		m["certificate"] = []string{in.TLS.CertPEM}
		if in.TLS.KeyPEM != "" {
			m["key"] = []string{in.TLS.KeyPEM}
		}
	} else {
		// sing-box: без сертификата и с insecure=true генерируется
		// самоподписанный сертификат на лету.
		m["insecure"] = true
	}
	return m
}

// outboundTLSBlock строит клиентский tls-блок sing-box.
func outboundTLSBlock(t *OutboundTLS) map[string]any {
	if t == nil || !t.Enabled {
		return nil
	}
	m := map[string]any{"enabled": true}
	if t.ServerName != "" {
		m["server_name"] = t.ServerName
	}
	if len(t.ALPN) > 0 {
		m["alpn"] = t.ALPN
	}
	if t.Insecure {
		m["insecure"] = true
	}
	fingerprint := t.UTLSFingerprint
	if t.Reality != nil && t.Reality.Enabled && fingerprint == "" {
		fingerprint = "chrome"
	}
	if fingerprint != "" {
		m["utls"] = map[string]any{"enabled": true, "fingerprint": fingerprint}
	}
	if t.Reality != nil && t.Reality.Enabled {
		m["reality"] = map[string]any{
			"enabled":    true,
			"public_key": t.Reality.PublicKey,
			"short_id":   t.Reality.ShortID,
		}
	}
	return m
}

func transportBlock(tr *TransportSettings) map[string]any {
	if tr == nil || tr.Type == "" {
		return nil
	}
	switch tr.Type {
	case "ws":
		m := map[string]any{"type": "ws"}
		if tr.Path != "" {
			m["path"] = tr.Path
		}
		if tr.Host != "" {
			m["headers"] = map[string]any{"Host": tr.Host}
		}
		return m
	case "grpc":
		m := map[string]any{"type": "grpc"}
		if tr.ServiceName != "" {
			m["service_name"] = tr.ServiceName
		}
		return m
	case "http":
		m := map[string]any{"type": "http"}
		if tr.Path != "" {
			m["path"] = tr.Path
		}
		if tr.Host != "" {
			m["host"] = []string{tr.Host}
		}
		return m
	case "httpupgrade":
		m := map[string]any{"type": "httpupgrade"}
		if tr.Path != "" {
			m["path"] = tr.Path
		}
		if tr.Host != "" {
			m["host"] = tr.Host
		}
		return m
	}
	return nil
}

// inboundUsers возвращает клиентских пользователей + служебного relay-пользователя
// (каскадный outbound подключается как relay_user, а не как клиентский).
func inboundUsers(in InboundSettings) []InboundUser {
	users := in.Users
	if in.RelayUser == nil {
		return users
	}
	for _, u := range users {
		if u.UUID != "" && u.UUID == in.RelayUser.UUID {
			return users
		}
	}
	return append(users, *in.RelayUser)
}

// relayOutboundBlock строит outbound для каскадной связи: серверные поля и
// учётные данные берутся из целевого inbound (единый источник правды).
func relayOutboundBlock(ob Node, target Node, targetPhys PhysNode) (map[string]any, error) {
	in, err := ParseInboundSettings(target.Settings)
	if err != nil {
		return nil, fmt.Errorf("inbound %s: %w", target.Tag, err)
	}
	host := in.PublicHost
	if host == "" {
		host = targetPhys.Host()
	}
	var user InboundUser
	if in.RelayUser != nil {
		user = *in.RelayUser
	} else if len(in.Users) > 0 {
		user = in.Users[0] // legacy: графы, сохранённые до введения relay_user
	}
	// utls fingerprint может быть задан в настройках outbound
	utlsFingerprint := ""
	if out, err := ParseOutboundSettings(ob.Settings); err == nil && out.TLS != nil {
		utlsFingerprint = out.TLS.UTLSFingerprint
	}
	return clientOutboundBlock(ob.Protocol, ob.Tag, in, host, user, utlsFingerprint), nil
}

// clientOutboundBlock — общий билдер клиентского outbound'а.
// protocol/tag берутся из source, настройки — из target inbound, креды — из user.
func clientOutboundBlock(protocol, tag string, in InboundSettings, host string, user InboundUser, utlsFingerprint string) map[string]any {
	m := map[string]any{
		"type":        protocol,
		"tag":         tag,
		"server":      host,
		"server_port": in.ListenPort,
	}
	switch protocol {
	case "vless":
		m["uuid"] = user.UUID
		if user.Flow != "" {
			m["flow"] = user.Flow
		}
	case "vmess":
		m["uuid"] = user.UUID
		m["security"] = "auto"
	case "trojan":
		m["password"] = user.Password
	case "shadowsocks":
		m["method"] = in.Method
		m["password"] = user.Password
	case "hysteria2":
		m["password"] = user.Password
		if in.ObfsPassword != "" {
			m["obfs"] = map[string]any{"type": "salamander", "password": in.ObfsPassword}
		}
	case "tuic":
		m["uuid"] = user.UUID
		m["password"] = user.Password
		if in.CongestionControl != "" {
			m["congestion_control"] = in.CongestionControl
		}
	}
	// TLS зеркалирует inbound
	if tls := clientTLSFromInbound(in, utlsFingerprint); tls != nil {
		m["tls"] = tls
	}
	if tr := transportBlock(in.Transport); tr != nil {
		m["transport"] = tr
	}
	return m
}

// ClientOutbound строит клиентский outbound к entry-inbound для подписки:
// адрес/порт/TLS/transport зеркалятся из inbound, креды берутся из панели.
func ClientOutbound(el Node, in InboundSettings, host string, c PanelCreds) map[string]any {
	user := InboundUser{Name: c.Name, UUID: c.UUID, Password: c.Password, Flow: c.Flow}
	// uTLS fingerprint для Reality — стандартно "chrome" (не путать с flow!)
	return clientOutboundBlock(el.Protocol, el.Tag, in, host, user, "chrome")
}

// clientTLSFromInbound зеркалирует TLS inbound'а на сторону клиента (outbound).
func clientTLSFromInbound(in InboundSettings, utlsFingerprint string) map[string]any {
	if in.TLS == nil || !in.TLS.Enabled {
		return nil
	}
	t := OutboundTLS{
		Enabled:    true,
		ServerName: in.TLS.ServerName,
		ALPN:       in.TLS.ALPN,
	}
	if t.ServerName == "" && in.PublicHost != "" {
		t.ServerName = in.PublicHost
	}
	if in.TLS.Reality != nil && in.TLS.Reality.Enabled {
		pub, err := RealityPublicKey(in.TLS.Reality.PrivateKey)
		if err != nil {
			// Валидация уже проверила формат; здесь ошибка маловероятна.
			return map[string]any{"enabled": true}
		}
		shortID := ""
		if len(in.TLS.Reality.ShortIDs) > 0 {
			shortID = in.TLS.Reality.ShortIDs[0]
		}
		t.Reality = &RealityOut{Enabled: true, PublicKey: pub, ShortID: shortID}
	} else if in.TLS.CertPEM == "" {
		// самоподписанный сертификат сервера — клиент пропускает проверку
		t.Insecure = true
	}
	t.UTLSFingerprint = utlsFingerprint
	return outboundTLSBlock(&t)
}

func applyRuleMatch(rule map[string]any, rs RuleSettings) {
	if len(rs.Network) > 0 {
		rule["network"] = rs.Network
	}
	if len(rs.Protocol) > 0 {
		rule["protocol"] = rs.Protocol
	}
	if len(rs.Domain) > 0 {
		rule["domain"] = rs.Domain
	}
	if len(rs.DomainSuffix) > 0 {
		rule["domain_suffix"] = rs.DomainSuffix
	}
	if len(rs.DomainKeyword) > 0 {
		rule["domain_keyword"] = rs.DomainKeyword
	}
	if len(rs.IPCIDR) > 0 {
		rule["ip_cidr"] = rs.IPCIDR
	}
	if len(rs.Port) > 0 {
		rule["port"] = rs.Port
	}
	if rs.Invert {
		rule["invert"] = true
	}
}

// ruleHasMatchConditions проверяет, есть ли у правила условия совпадения (domain, ip, port и т.д.).
// Правила без условий — это catch-all (final/fallback).
func ruleHasMatchConditions(rule map[string]any) bool {
	matchKeys := []string{"domain", "domain_suffix", "domain_keyword", "ip_cidr", "ip", "port", "network", "protocol", "source", "source_port", "process", "process_path", "package_name", "uid", "gid", "network_type", "inbound"}
	for _, k := range matchKeys {
		if v, ok := rule[k]; ok {
			switch vv := v.(type) {
			case []any:
				if len(vv) > 0 {
					return true
				}
			case []string:
				if len(vv) > 0 {
					return true
				}
			case string:
				if vv != "" {
					return true
				}
			}
		}
	}
	return false
}

func getBool(m map[string]any, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
