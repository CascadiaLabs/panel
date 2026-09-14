package graph

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// ShareLinks собирает v2ray share-links для всех entry-inbound графа —
// по одной подписке клиент получает все входы разом.
func ShareLinks(st State, phys []PhysNode, c PanelCreds) ([]string, error) {
	physByID := map[string]PhysNode{}
	for _, p := range phys {
		physByID[p.ID] = p
	}
	var links []string
	for _, el := range st.Nodes {
		if el.Kind != KindInbound || !el.Entry {
			continue
		}
		in, err := ParseInboundSettings(el.Settings)
		if err != nil {
			return nil, fmt.Errorf("inbound %s: %w", el.Tag, err)
		}
		host := in.PublicHost
		if host == "" {
			if p, ok := physByID[el.NodeID]; ok {
				host = p.Host()
			}
		}
		link, err := buildShareLink(el, in, host, c)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, nil
}

func buildShareLink(el Node, in InboundSettings, host string, c PanelCreds) (string, error) {
	frag := c.Name
	if c.Remark != "" {
		frag = c.Remark
	}
	// фрагмент — имя профиля у клиента; не-ascii кодируем как в реальных панелях
	frag = url.QueryEscape(strings.ReplaceAll(frag, "#", ""))
	addr := host + ":" + itoa(in.ListenPort)

	switch el.Protocol {
	case "vless":
		q := url.Values{}
		q.Set("encryption", "none")
		applyClientTLS(q, c, host, in)
		if c.Flow != "" {
			q.Set("flow", c.Flow)
		}
		if tr := in.Transport; tr != nil {
			applyTransport(q, tr)
		}
		return "vless://" + c.UUID + "@" + addr + "?" + q.Encode() + "#" + frag, nil

	case "vmess":
		net := "tcp"
		typ := ""
		vmhost := ""
		path := ""
		if tr := in.Transport; tr != nil {
			net = tr.Type
			vmhost, path = tr.Host, tr.Path
			if tr.Type == "grpc" {
				vmhost = tr.ServiceName
			}
			if tr.Type == "http" {
				typ = "http"
				vmhost = tr.Host
			}
		}
		payload := map[string]any{
			"v":    "2",
			"ps":   frag,
			"add":  host,
			"port": in.ListenPort,
			"id":   c.UUID,
			"aid":  "0",
			"scy":  "auto",
			"net":  net,
			"type": typ,
			"host": vmhost,
			"path": path,
		}
		applyVMessTLS(payload, host, in)
		raw, err := json.Marshal(payload)
		if err != nil {
			return "", err
		}
		return "vmess://" + base64.RawURLEncoding.EncodeToString(raw), nil

	case "trojan":
		q := url.Values{}
		applyClientTLS(q, c, host, in)
		if tr := in.Transport; tr != nil {
			applyTransport(q, tr)
		}
		return "trojan://" + c.Password + "@" + addr + "?" + q.Encode() + "#" + frag, nil

	case "shadowsocks":
		method := in.Method
		if method == "" {
			method = "2022-blake3-aes-128-gcm"
		}
		cipher := base64.RawURLEncoding.EncodeToString([]byte(method + ":" + c.Password))
		return "ss://" + cipher + "@" + addr + "#" + frag, nil

	case "hysteria2":
		q := url.Values{}
		if in.TLS != nil && in.TLS.ServerName != "" {
			q.Set("sni", in.TLS.ServerName)
		}
		if in.TLS == nil || in.TLS.CertPEM == "" {
			q.Set("insecure", "1")
		}
		if in.ObfsPassword != "" {
			q.Set("obfs", "salamander")
			q.Set("obfs-password", in.ObfsPassword)
		}
		return "hysteria2://" + c.Password + "@" + addr + "?" + q.Encode() + "#" + frag, nil

	case "tuic":
		q := url.Values{}
		if in.CongestionControl != "" {
			q.Set("congestion_control", in.CongestionControl)
		} else {
			q.Set("congestion_control", "bbr")
		}
		if in.TLS != nil {
			if in.TLS.ServerName != "" {
				q.Set("sni", in.TLS.ServerName)
			}
			if len(in.TLS.ALPN) > 0 {
				q.Set("alpn", strings.Join(in.TLS.ALPN, ","))
			}
		}
		q.Set("udp_relay_mode", "native")
		q.Set("reduce_rtt", "true")
		return "tuic://" + c.UUID + ":" + c.Password + "@" + addr + "?" + q.Encode() + "#" + frag, nil
	}
	return "", fmt.Errorf("inbound %s: протокол %s не поддерживается в подписке", el.Tag, el.Protocol)
}

// applyClientTLS заполняет query-параметры TLS/Reality для vless/trojan.
func applyClientTLS(q url.Values, c PanelCreds, host string, in InboundSettings) {
	tls := in.TLS
	if tls != nil && tls.Reality != nil && tls.Reality.Enabled {
		q.Set("security", "reality")
		sni := tls.ServerName
		if sni == "" {
			sni = host
		}
		q.Set("sni", sni)
		q.Set("fp", "chrome")
		if pub, err := RealityPublicKey(tls.Reality.PrivateKey); err == nil {
			q.Set("pbk", pub)
		}
		if len(tls.Reality.ShortIDs) > 0 {
			q.Set("sid", tls.Reality.ShortIDs[0])
		}
		return
	}
	if tls != nil && tls.Enabled {
		q.Set("security", "tls")
		sni := tls.ServerName
		if sni == "" {
			sni = host
		}
		if sni != "" {
			q.Set("sni", sni)
		}
		if len(tls.ALPN) > 0 {
			q.Set("alpn", strings.Join(tls.ALPN, ","))
		}
		q.Set("fp", "")
		return
	}
	q.Set("security", "none")
}

// applyVMessTLS заполняет поля tls/reality для JSON-конфига vmess.
func applyVMessTLS(payload map[string]any, host string, in InboundSettings) {
	tls := in.TLS
	if tls != nil && tls.Reality != nil && tls.Reality.Enabled {
		payload["tls"] = "reality"
		sni := tls.ServerName
		if sni == "" {
			sni = host
		}
		payload["sni"] = sni
		payload["fp"] = "chrome"
		if pub, err := RealityPublicKey(tls.Reality.PrivateKey); err == nil {
			payload["pbk"] = pub
		}
		if len(tls.Reality.ShortIDs) > 0 {
			payload["sid"] = tls.Reality.ShortIDs[0]
		}
		return
	}
	if tls != nil && tls.Enabled {
		payload["tls"] = "tls"
		sni := tls.ServerName
		if sni == "" {
			sni = host
		}
		if sni != "" {
			payload["sni"] = sni
		}
		if len(tls.ALPN) > 0 {
			payload["alpn"] = strings.Join(tls.ALPN, ",")
		}
	}
}

// applyTransport заполняет query-параметры транспорта для vless/trojan.
func applyTransport(q url.Values, tr *TransportSettings) {
	if tr == nil || tr.Type == "" {
		return
	}
	switch tr.Type {
	case "ws", "http", "httpupgrade":
		q.Set("type", tr.Type)
		if tr.Path != "" {
			q.Set("path", tr.Path)
		}
		if tr.Host != "" {
			q.Set("host", tr.Host)
		}
	case "grpc":
		q.Set("type", "grpc")
		if tr.ServiceName != "" {
			q.Set("serviceName", tr.ServiceName)
		}
	}
}

func itoa(n uint16) string {
	if n == 0 {
		return "0"
	}
	var buf [5]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
