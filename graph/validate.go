package graph

import (
	"fmt"
	"sort"
	"strings"
)

// Issue — одна проблема валидации. Code — стабильный идентификатор для UI.
type Issue struct {
	Code    string `json:"code"`
	Element string `json:"element,omitempty"` // tag или ID элемента
	Message string `json:"message"`
}

type Validation struct {
	Errors   []Issue `json:"errors"`
	Warnings []Issue `json:"warnings"`
}

func (v *Validation) HasErrors() bool { return len(v.Errors) > 0 }

func (v *Validation) err(code, element, format string, args ...any) {
	v.Errors = append(v.Errors, Issue{Code: code, Element: element, Message: fmt.Sprintf(format, args...)})
}

func (v *Validation) warn(code, element, format string, args ...any) {
	v.Warnings = append(v.Warnings, Issue{Code: code, Element: element, Message: fmt.Sprintf(format, args...)})
}

// Validator проверяет состояние графа.
type Validator struct {
	State State
	Nodes []PhysNode // доступные физические ноды
}

func (vd *Validator) Validate() Validation {
	v := Validation{Errors: []Issue{}, Warnings: []Issue{}}
	byID := map[string]*Node{}
	nodeByID := map[string]PhysNode{} // физические ноды по id
	for _, pn := range vd.Nodes {
		nodeByID[pn.ID] = pn
	}

	// --- элементы ---
	for i := range vd.State.Nodes {
		n := &vd.State.Nodes[i]
		byID[n.ID] = n

		if !KindValid(n.Kind) {
			v.err("invalid_kind", n.ID, "неизвестный тип элемента %q", n.Kind)
			continue
		}
		if !ProtocolValid(n.Kind, n.Protocol) {
			v.err("invalid_protocol", n.ID, "протокол %q недопустим для %s", n.Protocol, n.Kind)
		}
		tag := strings.TrimSpace(n.Tag)
		if tag == "" {
			v.err("empty_tag", n.ID, "пустой tag")
		}
		if pn, ok := nodeByID[n.NodeID]; !ok || pn.ID == "" {
			v.err("unknown_node", n.ID, "элемент %q ссылается на неизвестную ноду", tag)
		}

		switch n.Kind {
		case KindInbound:
			vd.checkInbound(&v, n)
		case KindOutbound:
			vd.checkOutbound(&v, n)
		case KindBalancer:
			vd.checkBalancer(&v, n)
		case KindRule:
			vd.checkRule(&v, n)
		}
	}

	// уникальность тегов в графе
	tags := map[string]string{}
	for _, n := range vd.State.Nodes {
		tag := strings.TrimSpace(n.Tag)
		if prev, dup := tags[tag]; dup && tag != "" {
			v.err("duplicate_tag", n.ID, "tag %q уже занят элементом %s", tag, prev)
		} else if tag != "" {
			tags[tag] = n.ID
		}
	}

	// --- рёбра ---
	type endpoint struct {
		node *Node
		id   string
	}
	endpoints := func(e Edge) (endpoint, endpoint, bool) {
		s, sok := byID[e.SourceID]
		t, tok := byID[e.TargetID]
		return endpoint{s, e.SourceID}, endpoint{t, e.TargetID}, sok && tok
	}

	edgeSeen := map[string]bool{}
	outTargets := map[string][]*Node{} // source element id → targets (для терминальности)

	for _, e := range vd.State.Edges {
		se, te, ok := endpoints(e)
		if !ok {
			v.err("dangling_edge", e.ID, "ребро ссылается на несуществующий элемент")
			continue
		}
		if se.node.ID == te.node.ID {
			v.err("self_loop", e.ID, "ребро в самого себя (%s)", se.node.Tag)
			continue
		}
		key := se.id + ">" + te.id
		if edgeSeen[key] {
			v.err("duplicate_edge", e.ID, "повторное ребро %s → %s", se.node.Tag, te.node.Tag)
			continue
		}
		edgeSeen[key] = true
		outTargets[se.id] = append(outTargets[se.id], te.node)

		src, dst := se.node, te.node
		switch {
		case src.Kind == KindInbound && (dst.Kind == KindRule || dst.Kind == KindBalancer || dst.Kind == KindOutbound):
			vd.sameNode(&v, src, dst)
		case src.Kind == KindRule && (dst.Kind == KindOutbound || dst.Kind == KindBalancer):
			vd.sameNode(&v, src, dst)
		case src.Kind == KindBalancer && dst.Kind == KindOutbound:
			vd.sameNode(&v, src, dst)
		case (src.Kind == KindOutbound || src.Kind == KindInbound || src.Kind == KindBalancer || src.Kind == KindRule) && dst.Kind == KindInbound:
			// каскад: только на ДРУГУЮ физическую ноду
			if src.NodeID == dst.NodeID {
				v.err("cascade_same_node", e.ID, "каскадное ребро %s → %s должно вести на другую ноду (внутри одной ноды используйте rule/balancer)", src.Tag, dst.Tag)
			}
			if src.Kind == KindOutbound && src.Protocol != dst.Protocol {
				v.err("protocol_mismatch", e.ID, "протоколы каскада не совпадают: %s (%s) → %s (%s)", src.Tag, src.Protocol, dst.Tag, dst.Protocol)
			}
			// у целевого inbound должны быть пользователи — outbound подключается как один из них
			in, err := ParseInboundSettings(dst.Settings)
			if err == nil && len(in.Users) == 0 {
				v.err("cascade_no_users", e.ID, "у целевого inbound %s нет пользователей для подключения", dst.Tag)
			}
		default:
			v.err("forbidden_edge", e.ID, "запрещённое соединение: %s → %s", kindLabel(src), kindLabel(dst))
		}
	}

	// --- терминальность и достижимость ---
	entryInbound := false
	for i := range vd.State.Nodes {
		n := &vd.State.Nodes[i]
		switch n.Kind {
		case KindInbound:
			if n.Entry {
				entryInbound = true
			}
			defaults := 0
			for _, target := range outTargets[n.ID] {
				if target.Kind != KindRule {
					defaults++
				} else if rs, err := ParseRuleSettings(target.Settings); err == nil && !ruleHasMatch(rs) {
					defaults++
				}
			}
			if defaults > 1 {
				v.err("inbound_multiple_targets", n.ID, "inbound %s имеет несколько безусловных целей; используйте rule/balancer", n.Tag)
			}
			if n.Exit && defaults > 0 {
				v.err("inbound_exit_and_target", n.ID, "inbound %s одновременно выход в интернет и имеет безусловную цель", n.Tag)
			}
		case KindOutbound:
			relayCount := 0
			exit := n.Exit
			for _, t := range outTargets[n.ID] {
				if t.Kind == KindInbound {
					relayCount++
				}
			}
			relay := relayCount > 0
			if relayCount > 1 {
				v.err("outbound_multiple_targets", n.ID, "outbound %s имеет несколько каскадных целей; используйте balancer", n.Tag)
			}
			if n.Protocol != "direct" && !relay && !exit {
				v.err("unroutable_outbound", n.ID, "outbound %s никуда не ведёт: подключите его к inbound другой ноды или отметьте как выход в интернет", n.Tag)
			}
			if relay && exit {
				v.err("outbound_relay_and_exit", n.ID, "outbound %s одновременно каскад и выход в интернет", n.Tag)
			}
		case KindBalancer:
			if len(outTargets[n.ID]) == 0 {
				v.err("empty_balancer", n.ID, "balancer %s без членов", n.Tag)
			}
		case KindRule:
			if len(outTargets[n.ID]) == 0 {
				v.err("rule_no_target", n.ID, "правило %s не ведёт ни к какому outbound/balancer/inbound", n.Tag)
			}
		}
	}

	if len(vd.State.Nodes) > 0 && !entryInbound {
		v.warn("no_entry", "", "ни один inbound не отмечен как вход от клиента")
	}

	// элементы, недостижимые от входа — предупреждение
	vd.unreachable(&v, byID)

	// --- циклы по межэлементным рёбрам ---
	vd.cycles(&v, byID)

	// сортируем для стабильного вывода
	sortIssues(v.Errors)
	sortIssues(v.Warnings)
	return v
}

func (vd *Validator) sameNode(v *Validation, src, dst *Node) {
	if src.NodeID != dst.NodeID {
		v.err("cross_node_edge", dst.ID, "%s и %s должны находиться на одной физической ноде", src.Tag, dst.Tag)
	}
}

func (vd *Validator) checkInbound(v *Validation, n *Node) {
	in, err := ParseInboundSettings(n.Settings)
	if err != nil {
		v.err("bad_settings", n.ID, "настройки inbound %s: %v", n.Tag, err)
		return
	}
	if in.ListenPort == 0 {
		v.err("no_listen_port", n.ID, "inbound %s: не задан listen_port", n.Tag)
	}
	needsUsers := n.Protocol == "vless" || n.Protocol == "vmess" || n.Protocol == "trojan" ||
		n.Protocol == "shadowsocks" || n.Protocol == "hysteria2" || n.Protocol == "tuic"
	if needsUsers && len(in.Users) == 0 {
		v.err("no_users", n.ID, "inbound %s: добавьте хотя бы одного пользователя", n.Tag)
	}
	// TLS обязателен для QUIC-протоколов; для остальных — опционален.
	if (n.Protocol == "hysteria2" || n.Protocol == "tuic") && (in.TLS == nil || !in.TLS.Enabled) {
		v.err("tls_required", n.ID, "inbound %s: %s требует TLS", n.Tag, n.Protocol)
	}
	if in.TLS != nil && in.TLS.Reality != nil && in.TLS.Reality.Enabled {
		if in.TLS.Reality.PrivateKey == "" {
			v.err("reality_no_key", n.ID, "inbound %s: reality требует private_key", n.Tag)
		}
		if len(in.TLS.Reality.ShortIDs) == 0 {
			v.err("reality_no_shortid", n.ID, "inbound %s: reality требует short_id", n.Tag)
		}
		if in.TLS.Reality.HandshakeServer == "" {
			v.err("reality_no_handshake", n.ID, "inbound %s: reality требует handshake-сервер (маскировка)", n.Tag)
		}
		// reality несовместим с сертификатами
		if in.TLS.CertPEM != "" {
			v.err("reality_and_cert", n.ID, "inbound %s: reality не использует свой сертификат — уберите cert/key", n.Tag)
		}
		if n.Protocol != "vless" && n.Protocol != "trojan" {
			v.err("reality_protocol", n.ID, "inbound %s: reality поддерживается для vless/trojan", n.Tag)
		}
	} else if in.TLS != nil && in.TLS.Enabled {
		// обычный TLS: либо свои сертификаты, либо ACME не поддерживаем в v1 — предупреждение
		if in.TLS.CertPEM == "" {
			v.warn("tls_selfsigned", n.ID, "inbound %s: без cert/key sing-box сгенерирует самоподписанный сертификат", n.Tag)
		}
	}
	if in.Transport != nil && in.Transport.Type != "" {
		switch in.Transport.Type {
		case "ws", "grpc", "http", "httpupgrade":
		default:
			v.err("bad_transport", n.ID, "inbound %s: неизвестный transport %q", n.Tag, in.Transport.Type)
		}
	}
}

func (vd *Validator) checkOutbound(v *Validation, n *Node) {
	if n.Protocol == "direct" {
		return // терминальный, без настроек
	}
	out, err := ParseOutboundSettings(n.Settings)
	if err != nil {
		v.err("bad_settings", n.ID, "настройки outbound %s: %v", n.Tag, err)
	}
	// server/server_port проверяются в relay-режиме при генерации; для exit — здесь
	if n.Exit {
		if out.Server == "" || out.ServerPort == 0 {
			v.err("exit_no_server", n.ID, "outbound %s (выход в интернет): задайте server и server_port", n.Tag)
		}
	}
	if out.TLS != nil && out.TLS.Reality != nil && out.TLS.Reality.Enabled {
		if out.TLS.Reality.PublicKey == "" {
			v.err("reality_no_pubkey", n.ID, "outbound %s: reality требует public_key", n.Tag)
		}
		if n.Protocol != "vless" && n.Protocol != "trojan" {
			v.err("reality_protocol", n.ID, "outbound %s: reality поддерживается для vless/trojan", n.Tag)
		}
	}
}

func (vd *Validator) checkBalancer(v *Validation, n *Node) {
	if _, err := ParseBalancerSettings(n.Settings); err != nil {
		v.err("bad_settings", n.ID, "настройки balancer %s: %v", n.Tag, err)
	}
}

func (vd *Validator) checkRule(v *Validation, n *Node) {
	s, err := ParseRuleSettings(n.Settings)
	if err != nil {
		v.err("bad_settings", n.ID, "настройки rule %s: %v", n.Tag, err)
		return
	}
	if !ruleHasMatch(s) {
		v.warn("rule_catch_all", n.ID, "правило %s без условий — ловит весь трафик (catch-all)", n.Tag)
	}
}

func ruleHasMatch(s RuleSettings) bool {
	return len(s.Network) > 0 || len(s.Protocol) > 0 || len(s.Domain) > 0 ||
		len(s.DomainSuffix) > 0 || len(s.DomainKeyword) > 0 || len(s.IPCIDR) > 0 || len(s.Port) > 0
}

// unreachable помечает элементы, до которых нельзя дойти от entry-inbound'ов
// (двигаясь по направлению потока).
func (vd *Validator) unreachable(v *Validation, byID map[string]*Node) {
	adj := map[string][]string{}
	for _, e := range vd.State.Edges {
		adj[e.SourceID] = append(adj[e.SourceID], e.TargetID)
	}
	reachable := map[string]bool{}
	var visit func(id string)
	visit = func(id string) {
		if reachable[id] {
			return
		}
		reachable[id] = true
		for _, next := range adj[id] {
			visit(next)
		}
	}
	for _, n := range vd.State.Nodes {
		if n.Kind == KindInbound && n.Entry {
			visit(n.ID)
		}
	}
	anyEntry := false
	for _, n := range vd.State.Nodes {
		if n.Kind == KindInbound && n.Entry {
			anyEntry = true
			break
		}
	}
	if !anyEntry {
		return // уже предупредили no_entry
	}
	for _, n := range vd.State.Nodes {
		if !reachable[n.ID] {
			v.warn("unreachable", n.ID, "элемент %s недостижим от входа", n.Tag)
		}
	}
}

// cycles ищет направленные циклы DFS (по элементам графа; включает межнодовые петли).
func (vd *Validator) cycles(v *Validation, byID map[string]*Node) {
	adj := map[string][]string{}
	for _, e := range vd.State.Edges {
		adj[e.SourceID] = append(adj[e.SourceID], e.TargetID)
	}

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var stack []string
	var cycle []string

	var dfs func(id string) bool
	dfs = func(id string) bool {
		color[id] = gray
		stack = append(stack, id)
		for _, next := range adj[id] {
			switch color[next] {
			case gray:
				// нашли цикл: от next до конца стека
				idx := 0
				for i, s := range stack {
					if s == next {
						idx = i
						break
					}
				}
				cycle = append([]string{}, stack[idx:]...)
				return true
			case white:
				if dfs(next) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return false
	}

	for _, n := range vd.State.Nodes {
		if color[n.ID] == white {
			if dfs(n.ID) {
				tags := make([]string, 0, len(cycle))
				for _, id := range cycle {
					if node, ok := byID[id]; ok {
						tags = append(tags, node.Tag)
					}
				}
				v.err("cycle", strings.Join(cycle, ","), "обнаружен цикл: %s", strings.Join(tags, " → "))
				return // достаточно первого цикла
			}
		}
	}
}

func kindLabel(n *Node) string {
	return n.Kind + " " + n.Tag
}

func sortIssues(issues []Issue) {
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Code != issues[j].Code {
			return issues[i].Code < issues[j].Code
		}
		return issues[i].Element < issues[j].Element
	})
}
