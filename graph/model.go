// Package graph — доменная модель каскадного графа: элементы (inbound/outbound/
// rule/balancer), рёбра, валидация и компиляция в нативные конфиги sing-box.
//
// Семантика (направление потока):
//
//		клиент → inbound(нода A) → [rule|balancer] → outbound(A) ⇒ inbound(нода B) → … → outbound → интернет
//
//	  - inbound → rule|balancer|outbound: только в пределах одной физической ноды;
//	  - rule → outbound|balancer, balancer → outbound: там же;
//	  - balancer → inbound: сокращённый каскад на любую ноду, включая ту же;
//	    outbound автоматически выводится из целевого inbound;
//	  - inbound|rule → inbound: сокращённый каскад только на ДРУГУЮ ноду;
//	    outbound автоматически выводится из целевого inbound;
//	  - outbound → inbound: явный каскад на ДРУГУЮ ноду (протоколы должны совпадать);
//	  - entry=true у inbound: вход от клиента; exit=true у inbound: прямой выход
//	    в интернет; exit=true у outbound: выход с явными настройками подключения.
package graph

import (
	"encoding/json"
	"net"
	"strings"
)

// Kind — тип элемента графа.
const (
	KindInbound  = "inbound"
	KindOutbound = "outbound"
	KindRule     = "rule"
	KindBalancer = "balancer"
)

// Протоколы («balancer» в терминах UI = urltest/selector из sing-box:
// balancer'ов в sing-box нет, urltest выбирает адресат с меньшим пингом).
var kindProtocols = map[string][]string{
	KindInbound:  {"vless", "vmess", "trojan", "shadowsocks", "hysteria2", "tuic"},
	KindOutbound: {"vless", "vmess", "trojan", "shadowsocks", "hysteria2", "tuic", "direct"},
	KindBalancer: {"urltest", "selector"},
	KindRule:     {"match"},
}

func ProtocolsForKind(kind string) []string { return kindProtocols[kind] }

func KindValid(kind string) bool {
	_, ok := kindProtocols[kind]
	return ok
}

func ProtocolValid(kind, protocol string) bool {
	for _, p := range kindProtocols[kind] {
		if p == protocol {
			return true
		}
	}
	return false
}

// PhysNode — минимум о физической ноде, нужный графу.
type PhysNode struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	GRPCURL string `json:"grpc_url"`
}

// Host возвращает хост из gRPC-адреса (host:port) или всю строку.
func (p PhysNode) Host() string {
	h, _, err := net.SplitHostPort(p.GRPCURL)
	if err != nil {
		return p.GRPCURL
	}
	return h
}

// Node — элемент графа (не путать с физической нодой NodeID).
type Node struct {
	ID       string          `json:"id"`
	NodeID   string          `json:"node_id"` // физическая нода (db.nodes.id)
	Kind     string          `json:"kind"`
	Protocol string          `json:"protocol"`
	Tag      string          `json:"tag"`
	Settings json.RawMessage `json:"settings"`
	PosX     float64         `json:"pos_x"`
	PosY     float64         `json:"pos_y"`
	Entry    bool            `json:"entry"` // вход от клиента
	Exit     bool            `json:"exit"`  // выход в интернет
}

// Edge — ребро графа.
type Edge struct {
	ID       string `json:"id"`
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
}

// State — снимок графа (то, что редактирует канвас).
type State struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// --- Настройки протоколов (settings JSON элемента) ---

type InboundUser struct {
	Name     string `json:"name"`
	UUID     string `json:"uuid,omitempty"`     // vless/vmess/tuic
	Password string `json:"password,omitempty"` // trojan/ss/hy2/tuic
	Flow     string `json:"flow,omitempty"`     // vless
}

type TransportSettings struct {
	Type        string `json:"type,omitempty"` // ws|grpc|http|httpupgrade ("" = tcp)
	Path        string `json:"path,omitempty"`
	ServiceName string `json:"service_name,omitempty"` // grpc
	Host        string `json:"host,omitempty"`         // ws/http/httpupgrade
}

type RealityIn struct {
	Enabled         bool     `json:"enabled,omitempty"`
	PrivateKey      string   `json:"private_key,omitempty"`
	ShortIDs        []string `json:"short_ids,omitempty"`
	HandshakeServer string   `json:"handshake_server,omitempty"`
	HandshakePort   uint16   `json:"handshake_port,omitempty"`
}

type InboundTLS struct {
	Enabled    bool       `json:"enabled,omitempty"`
	ServerName string     `json:"server_name,omitempty"`
	ALPN       []string   `json:"alpn,omitempty"`
	CertPEM    string     `json:"cert_pem,omitempty"`
	KeyPEM     string     `json:"key_pem,omitempty"`
	Reality    *RealityIn `json:"reality,omitempty"`
}

type InboundSettings struct {
	ListenPort uint16 `json:"listen_port"`
	PublicHost string `json:"public_host,omitempty"` // адрес для каскадных подключений; "" → хост gRPC
	// SubscriptionOrder задаёт позицию entry-inbound в подписке: меньшие значения идут раньше.
	// Ноль — обычный приоритет; при одинаковом значении используется тег.
	SubscriptionOrder int                `json:"subscription_order,omitempty"`
	Users             []InboundUser      `json:"users"`
	RelayUser         *InboundUser       `json:"relay_user,omitempty"` // служебные креды для каскадных подключений (генерятся при сохранении)
	TLS               *InboundTLS        `json:"tls,omitempty"`
	Transport         *TransportSettings `json:"transport,omitempty"`

	// протоколо-специфичные
	Method            string `json:"method,omitempty"`             // shadowsocks
	Network           string `json:"network,omitempty"`            // shadowsocks (tcp/udp)
	ObfsPassword      string `json:"obfs_password,omitempty"`      // hysteria2 salamander
	UpMbps            int    `json:"up_mbps,omitempty"`            // hysteria2
	DownMbps          int    `json:"down_mbps,omitempty"`          // hysteria2
	CongestionControl string `json:"congestion_control,omitempty"` // tuic: bbr|cubic|new_reno
}

type RealityOut struct {
	Enabled   bool   `json:"enabled,omitempty"`
	PublicKey string `json:"public_key,omitempty"`
	ShortID   string `json:"short_id,omitempty"`
}

type OutboundTLS struct {
	Enabled         bool        `json:"enabled,omitempty"`
	ServerName      string      `json:"server_name,omitempty"`
	ALPN            []string    `json:"alpn,omitempty"`
	Insecure        bool        `json:"insecure,omitempty"`
	UTLSFingerprint string      `json:"utls_fingerprint,omitempty"` // "" → chrome при reality
	Reality         *RealityOut `json:"reality,omitempty"`
}

type OutboundSettings struct {
	Server     string `json:"server,omitempty"`
	ServerPort uint16 `json:"server_port,omitempty"`

	// учётные данные (для exit-режима; для relay выводятся из целевого inbound)
	UUID     string `json:"uuid,omitempty"`
	Password string `json:"password,omitempty"`
	Flow     string `json:"flow,omitempty"`
	Security string `json:"security,omitempty"` // vmess outbound: auto|none|...

	TLS       *OutboundTLS       `json:"tls,omitempty"`
	Transport *TransportSettings `json:"transport,omitempty"`

	// протоколо-специфичные
	Method            string `json:"method,omitempty"` // shadowsocks
	Network           string `json:"network,omitempty"`
	ObfsPassword      string `json:"obfs_password,omitempty"` // hysteria2
	UpMbps            int    `json:"up_mbps,omitempty"`
	DownMbps          int    `json:"down_mbps,omitempty"`
	CongestionControl string `json:"congestion_control,omitempty"` // tuic
}

type BalancerSettings struct {
	URL       string `json:"url,omitempty"`       // urltest
	Interval  string `json:"interval,omitempty"`  // urltest, например "3m"
	Tolerance uint16 `json:"tolerance,omitempty"` // urltest (мс)
	Default   string `json:"default,omitempty"`   // selector
}

type RuleSettings struct {
	Network       []string `json:"network,omitempty"`  // tcp|udp
	Protocol      []string `json:"protocol,omitempty"` // tls|http|quic|dns…
	Domain        []string `json:"domain,omitempty"`
	DomainSuffix  []string `json:"domain_suffix,omitempty"`
	DomainKeyword []string `json:"domain_keyword,omitempty"`
	IPCIDR        []string `json:"ip_cidr,omitempty"`
	Port          []uint16 `json:"port,omitempty"`
	Invert        bool     `json:"invert,omitempty"`
	// sing-box 1.14 fields
	RuleSet     []string `json:"rule_set,omitempty"`
	IPIsPrivate bool     `json:"ip_is_private,omitempty"`
	ClashMode   string   `json:"clash_mode,omitempty"`
}

func ParseInboundSettings(raw json.RawMessage) (InboundSettings, error) {
	var s InboundSettings
	if len(raw) == 0 {
		return s, nil
	}
	err := json.Unmarshal(raw, &s)
	return s, err
}

func ParseOutboundSettings(raw json.RawMessage) (OutboundSettings, error) {
	var s OutboundSettings
	if len(raw) == 0 {
		return s, nil
	}
	err := json.Unmarshal(raw, &s)
	return s, err
}

func ParseBalancerSettings(raw json.RawMessage) (BalancerSettings, error) {
	var s BalancerSettings
	if len(raw) == 0 {
		return s, nil
	}
	err := json.Unmarshal(raw, &s)
	return s, err
}

func ParseRuleSettings(raw json.RawMessage) (RuleSettings, error) {
	var s RuleSettings
	if len(raw) == 0 {
		return s, nil
	}
	err := json.Unmarshal(raw, &s)
	return s, err
}

// LegacyRouteRuleItem — устаревшая структура для чтения старых записей из БД.
// Поля Outbounds, Final, ip_cidr с geoip:/geosite: — legacy формат sing-box < 1.12.
type LegacyRouteRuleItem struct {
	Name          string   `json:"name"`
	Action        string   `json:"action"`
	Outbounds     []string `json:"outbounds,omitempty"`
	Domain        []string `json:"domain,omitempty"`
	DomainSuffix  []string `json:"domain_suffix,omitempty"`
	DomainKeyword []string `json:"domain_keyword,omitempty"`
	DomainRegex   []string `json:"domain_regex,omitempty"`
	IPCIDR        []string `json:"ip_cidr,omitempty"`
	SourceIPCIDR  []string `json:"source_ip_cidr,omitempty"`
	Port          []string `json:"port,omitempty"`
	SourcePort    []string `json:"source_port,omitempty"`
	Network       []string `json:"network,omitempty"`
	Protocol      []string `json:"protocol,omitempty"`
	Process       []string `json:"process,omitempty"`
	ProcessPath   []string `json:"process_path,omitempty"`
	PackageName   []string `json:"package_name,omitempty"`
	UID           []string `json:"uid,omitempty"`
	GID           []string `json:"gid,omitempty"`
	NetworkType   []string `json:"network_type,omitempty"`
	Inbound       []string `json:"inbound,omitempty"`
	Final         bool     `json:"final,omitempty"`
}

// ConvertToRouteRuleItem конвертирует legacy RouteRuleItem в новую модель sing-box 1.14+.
// Обрабатывает: block→reject, geoip:→rule_set/ip_is_private, geosite:→rule_set, outbounds[]→outbound.
func (l LegacyRouteRuleItem) ConvertToRouteRuleItem() RouteRuleItem {
	r := RouteRuleItem{
		Name:   l.Name,
		Action: l.Action,
	}
	if l.Action == "block" {
		r.Action = "reject"
	}
	if len(l.Outbounds) > 0 {
		r.Outbound = l.Outbounds[0]
	}
	// Конвертация ip_cidr: geoip:xx → rule_set, geoip:private → ip_is_private
	for _, cidr := range l.IPCIDR {
		switch cidr {
		case "geoip:private":
			r.IPIsPrivate = true
		default:
			if strings.HasPrefix(cidr, "geoip:") {
				r.RuleSet = append(r.RuleSet, "geoip-"+strings.TrimPrefix(cidr, "geoip:"))
			}
		}
	}
	// Конвертация domain: geosite:xx → rule_set
	for _, dom := range l.Domain {
		if strings.HasPrefix(dom, "geosite:") {
			r.RuleSet = append(r.RuleSet, strings.TrimPrefix(dom, "geosite:"))
		}
	}
	// Копируем остальные поля как есть
	r.Domain = l.Domain
	r.DomainSuffix = l.DomainSuffix
	r.DomainKeyword = l.DomainKeyword
	r.DomainRegex = l.DomainRegex
	r.SourceIPCIDR = l.SourceIPCIDR
	r.Port = l.Port
	r.SourcePort = l.SourcePort
	r.Network = l.Network
	r.Protocol = l.Protocol
	r.Process = l.Process
	r.ProcessPath = l.ProcessPath
	r.PackageName = l.PackageName
	r.UID = l.UID
	r.GID = l.GID
	r.NetworkType = l.NetworkType
	r.Inbound = l.Inbound
	return r
}

// ParseRouteRuleItems парсит JSON массив из rules_json, поддерживая оба формата:
// legacy (Outbounds, Final, geoip:/geosite:) и новый (Outbound, RuleSet, IPIsPrivate).
func ParseRouteRuleItems(data []byte) ([]RouteRuleItem, error) {
	var result []RouteRuleItem
	// Сначала попробуем legacy формат
	var legacy []LegacyRouteRuleItem
	if err := json.Unmarshal(data, &legacy); err == nil && len(legacy) > 0 {
		for _, l := range legacy {
			result = append(result, l.ConvertToRouteRuleItem())
		}
		return result, nil
	}
	// Fallback: новый формат
	err := json.Unmarshal(data, &result)
	return result, err
}
