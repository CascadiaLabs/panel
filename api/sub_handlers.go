package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/CascadiaLabs/panel/db"
	"github.com/CascadiaLabs/panel/graph"
)

// SubscriptionInfoResp — метаданные подписки для заголовков и JSON-ответа.
type SubscriptionInfoResp struct {
	Name                 string   `json:"name"`
	Desc                 string   `json:"desc,omitempty"`
	Site                 string   `json:"site,omitempty"`
	Support              string   `json:"support,omitempty"`
	SubscriptionID       string   `json:"subscription_id"`
	Links                []string `json:"links,omitempty"`
	Userinfo             string   `json:"-"`     // передаётся только в заголовке
	ProfileTitle         string   `json:"-"`     // base64 заголовок
	ProfileUpdateInterval int     `json:"-"`
	ProfileWebPageURL    string   `json:"-"`
	SupportURL           string   `json:"-"`
	Config               map[string]any `json:"config,omitempty"` // Sing-box JSON конфиг (для JSON формата)
}

// Sub — публичная подписка VPN-пользователя: GET /sub/{sub_token}.
// Поддерживает форматы: Base64 (v2rayN/v2rayNG) и Sing-box JSON (Happ).
func (h *Handler) Sub(w http.ResponseWriter, r *http.Request) {
	u, err := h.store.GetPanelUserBySubToken(r.PathValue("token"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !u.Enabled {
		http.Error(w, "подписка отключена", http.StatusForbidden)
		return
	}

	format := detectFormat(r)
	resp, err := h.buildSubscriptionResponse(u.GraphID, u, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	// Устанавливаем HTTP-заголовки метаданных
	setSubscriptionHeaders(w, resp)

	if format == "json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		writeJSON(w, resp.Config)
		return
	}

	// Base64 format (по умолчанию)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	combined := strings.Join(resp.Links, "\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(combined))
	w.Write([]byte(encoded))
}

// SubscriptionInfo — GET /api/graphs/{id}/subscription — возвращает метаданные
// подписки и конфиг. Требует аутентификации (admin).
func (h *Handler) SubscriptionInfo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	users, err := h.store.ListPanelUsers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var targetUser db.PanelUser
	found := false
	for _, u := range users {
		if u.GraphID == id && u.Enabled {
			targetUser = u
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "нет активных пользователей для этого графа", http.StatusUnprocessableEntity)
		return
	}
	resp, err := h.buildSubscriptionResponse(id, targetUser, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	setSubscriptionHeaders(w, resp)
	format := detectFormat(r)
	if format == "json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		writeJSON(w, resp.Config)
		return
	}
	writeJSON(w, resp)
}

// detectFormat определяет формат ответа по User-Agent и query-параметру.
// base64 (share links) — для Xray-клиентов (Happ, v2rayNG, v2rayTun и др.).
// json (sing-box JSON) — для нативных sing-box клиентов.
func detectFormat(r *http.Request) string {
	if format := r.URL.Query().Get("format"); format != "" {
		return format
	}
	ua := strings.ToLower(r.UserAgent())
	// Sing-box нативные клиенты (включая iOS sing-box)
	if strings.Contains(ua, "sing-box") ||
		strings.Contains(ua, "nekobox") ||
		strings.Contains(ua, "hiddify") ||
		strings.Contains(ua, "streisand") {
		return "json"
	}
	// Clash-клиенты обычно умеют и то, и другое — отдаём JSON (они его поймут)
	if strings.Contains(ua, "clash") {
		return "json"
	}
	// Happ, v2rayNG, v2rayTun и остальные Xray-клиенты — base64 share links
	return "base64"
}

// buildSubscriptionResponse формирует полную структуру подписки.
func (h *Handler) buildSubscriptionResponse(graphID string, u db.PanelUser, r *http.Request) (*SubscriptionInfoResp, error) {
	state, err := h.store.LoadGraphState(graphID)
	if err != nil {
		return nil, err
	}
	links, err := graph.ShareLinks(state, h.physNodes(r), panelCreds(u))
	if err != nil {
		return nil, err
	}
	g, err := h.store.GetGraph(graphID)
	if err != nil {
		return nil, err
	}

	resp := &SubscriptionInfoResp{
		Name:           g.SubscriptionName,
		Desc:           g.SubscriptionDesc,
		Site:           g.SubscriptionSite,
		Support:        g.SubscriptionSupport,
		SubscriptionID: g.ID,
		Links:          links,
		Userinfo:       buildUserinfo(u),
		ProfileTitle:   buildProfileTitle(g.SubscriptionName),
		ProfileUpdateInterval: 12,
		ProfileWebPageURL: g.SubscriptionSite,
		SupportURL:     g.SubscriptionSupport,
	}

	// Если запрос от Sing-box клиента или format=json — генерируем полноценный JSON
	format := detectFormat(r)
	if format == "json" {
		singBoxConfig, err := GetSingBoxSubscriptionConfig(state, h.physNodes(r), panelCreds(u))
		if err != nil {
			return nil, err
		}
		// Если задан client_route, подставляем его как маршрутизацию
		if g.ClientRoute != "" {
			var customRoute map[string]any
			if err := json.Unmarshal([]byte(g.ClientRoute), &customRoute); err == nil {
				singBoxConfig["route"] = customRoute
			}
		}
		resp.Config = singBoxConfig
		resp.Links = nil // JSON format не содержит links
	}

	return resp, nil
}

// setSubscriptionHeaders устанавливает все метаданные в HTTP-заголовках ответа.
func setSubscriptionHeaders(w http.ResponseWriter, resp *SubscriptionInfoResp) {
	w.Header().Set("Subscription-Userinfo", resp.Userinfo)
	w.Header().Set("profile-title", resp.ProfileTitle)
	w.Header().Set("profile-update-interval", fmt.Sprintf("%d", resp.ProfileUpdateInterval))
	if resp.ProfileWebPageURL != "" {
		w.Header().Set("profile-web-page-url", resp.ProfileWebPageURL)
	}
	if resp.SupportURL != "" {
		w.Header().Set("support-url", resp.SupportURL)
	}
}

// buildUserinfo формирует строку Subscription-Userinfo.
func buildUserinfo(u db.PanelUser) string {
	parts := []string{
		fmt.Sprintf("upload=%d", u.UsedUpload),
		fmt.Sprintf("download=%d", u.UsedDownload),
	}
	if u.TotalTraffic > 0 {
		parts = append(parts, fmt.Sprintf("total=%d", u.TotalTraffic))
	}
	if u.ExpireTime > 0 {
		parts = append(parts, fmt.Sprintf("expire=%d", u.ExpireTime))
	}
	return strings.Join(parts, "; ")
}

// buildProfileTitle кодирует имя профиля в base64.
func buildProfileTitle(name string) string {
	if name == "" {
		name = "VPN Panel"
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(name))
	return "base64:" + encoded
}

// GetSingBoxSubscriptionConfig генерирует полноценный sing-box JSON конфиг
// для клиентов Happ/ClashMeta на основе графа и физических нод.
// Клиентский конфиг содержит: tun-вход, outbound'ы по каждому entry-inbound,
// selector proxy-group, direct, block, DNS и маршрутизацию с final=proxy-group.
func GetSingBoxSubscriptionConfig(st graph.State, phys []graph.PhysNode, creds graph.PanelCreds) (map[string]any, error) {
	// Вшиваем учётные данные панели во все entry-inbound.
	st = graph.InjectUsersIntoState(st, creds)

	// Собираем entry-inbound'ы в порядке подписки.
	entries, err := graph.EntryInbounds(st, phys)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("в графе нет entry-inbound для подписки")
	}

	// Клиентские outbound'ы: по одному на entry-inbound.
	// Это и есть серверы, которые выбирает пользователь в клиенте.
	var serverTags []string
	outbounds := make([]map[string]any, 0, len(entries)+3)
	for _, e := range entries {
		ob := graph.ClientOutbound(e.Node, e.In, e.Host, creds)
		outbounds = append(outbounds, ob)
		if tag, ok := ob["tag"].(string); ok {
			serverTags = append(serverTags, tag)
		}
	}

	// proxy-group selector (всегда есть, ссылается на него DNS и route)
	outbounds = append(outbounds, map[string]any{
		"type":      "selector",
		"tag":       "proxy-group",
		"outbounds": serverTags,
		"default":   serverTags[0],
	})

	// Direct (без block — legacy special outbound, не используется)
	outbounds = append(outbounds,
		map[string]any{"type": "direct", "tag": "direct"},
	)

	// DNS (sing-box 1.14+ формат)
	dns := map[string]any{
		"servers": []map[string]any{
			{
				"type":       "https",
				"tag":        "dns-remote",
				"server":     "1.1.1.1",
				"server_port": 443,
				"path":       "/dns-query",
				"detour":     "proxy-group",
				"tls": map[string]any{
					"enabled":     true,
					"server_name": "cloudflare-dns.com",
				},
			},
			{
				"type":       "https",
				"tag":        "dns-direct",
				"server":     "dns.google",
				"server_port": 443,
				"path":       "/dns-query",
				"tls": map[string]any{
					"enabled":     true,
					"server_name": "dns.google",
				},
			},
		},
		"rules": []map[string]any{
			{"action": "route", "clash_mode": "Direct", "server": "dns-direct"},
			{"action": "route", "clash_mode": "Global", "server": "dns-remote"},
		},
		"final": "dns-remote",
	}

	// Route rules (современный формат sing-box 1.14+ с action: "route")
	routeRules := []map[string]any{
		{"action": "sniff"},
		{"protocol": "dns", "action": "hijack-dns"},
		{"action": "resolve"},
		{"action": "route", "clash_mode": "Direct", "outbound": "direct"},
		{"action": "route", "clash_mode": "Global", "outbound": "proxy-group"},
		{"action": "route", "ip_is_private": true, "outbound": "direct"},
	}

	// Russian domain rules
	routeRules = append(routeRules, map[string]any{
		"action":      "route",
		"outbound":    "direct",
		"domain_suffix": []string{
			".ru", ".su", ".xn--p1ai", "vk.com", "yandex.ru",
			"gosuslugi.ru", "sberbank.ru",
		},
	})

	// Sing-box geoip/geosite rule sets (актуальные теги)
	routeRules = append(routeRules, map[string]any{
		"action":     "route",
		"outbound":   "direct",
		"rule_set":   []string{"geosite-category-ru", "geoip-ru"},
	})

	route := map[string]any{
		"rule_set": []map[string]any{
			{
				"tag":      "geosite-category-ru",
				"type":     "remote",
				"format":   "binary",
				"url":      "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-category-ru.srs",
			},
			{
				"tag":      "geoip-ru",
				"type":     "remote",
				"format":   "binary",
				"url":      "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-ru.srs",
			},
		},
		"rules":                  routeRules,
		"final":                  "proxy-group",
		"auto_detect_interface":  true,
		"default_domain_resolver": "dns-remote",
	}

	// Клиентский inbound: TUN (sing-box 1.14+ формат)
	inbounds := []map[string]any{
		{
			"type":          "tun",
			"tag":           "tun-in",
			"address":       []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
			"auto_route":    true,
			"strict_route":  true,
		},
	}

	result := map[string]any{
		"log":       map[string]any{"level": "info"},
		"dns":       dns,
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"route":     route,
	}

	return result, nil
}

// sanitizeTag превращает имя ноды в безопасный сегмент тега:
// заменяет пробелы и спецсимволы на дефис, убирает верхний регистр.
func sanitizeTag(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", "-"))
}
