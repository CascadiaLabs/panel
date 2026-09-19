package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// RouteRule — набор правил маршрутизации, который можно назначить на любой inbound.
type RouteRule struct {
	ID        string `json:"id"`
	GraphID   string `json:"graph_id"`
	Name      string `json:"name"`
	RulesJSON string `json:"rules_json"`
	IsDefault bool   `json:"is_default"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// RouteRuleItem — отдельное правило внутри массива rules_json.
type RouteRuleItem struct {
	Name          string   `json:"name"`
	Action        string   `json:"action"`
	Outbound      string   `json:"outbound,omitempty"`
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
	// sing-box 1.14 fields
	RuleSet     []string `json:"rule_set,omitempty"`
	IPIsPrivate bool     `json:"ip_is_private,omitempty"`
	ClashMode   string   `json:"clash_mode,omitempty"`
}

// DefaultRouteRules возвращает список дефолтных правил маршрутизации.
func DefaultRouteRules() []RouteRuleItem {
	return []RouteRuleItem{
		{
			Name:        "ru-direct",
			Action:      "route",
			Outbound:    "direct",
			DomainSuffix: []string{"ru", "su", "xn--p1ai"},
			RuleSet:     []string{"geoip-ru"},
		},
		{
			Name:    "ads-blocker",
			Action:  "reject",
			RuleSet: []string{"geosite-category-ads-all"},
		},
		{
			Name:       "private-ip",
			Action:     "reject",
			IPIsPrivate: true,
		},
		{
			Name:     "global-route",
			Action:   "route",
			Outbound: "proxy",
		},
	}
}

// EnsureGlobalDefaultRouteRules создаёт глобальные дефолтные правила маршрутизации,
// если их ещё нет. Глобальные правила имеют graph_id = ” и доступны для всех графов.
func (s *Store) EnsureGlobalDefaultRouteRules() error {
	// Проверяем, существуют ли уже глобальные дефолтные правила
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM route_rules WHERE graph_id = '' AND is_default = 1`).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil // уже существуют
	}

	items := DefaultRouteRules()
	now := time.Now().Unix()
	for _, item := range items {
		rulesJSON, err := json.Marshal([]RouteRuleItem{item})
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(`INSERT INTO route_rules (id, graph_id, name, rules_json, is_default, created_at, updated_at)
			VALUES (?, '', ?, ?, 1, ?, ?)`,
			mustUUID(), item.Name, string(rulesJSON), now, now); err != nil {
			return err
		}
	}
	return nil
}

const routeRuleCols = `id, graph_id, name, rules_json, is_default, created_at, updated_at`

// CreateRouteRule создаёт новое правило маршрутизации.
func (s *Store) CreateRouteRule(gr RouteRule) (RouteRule, error) {
	now := time.Now().Unix()
	gr.ID = mustUUID()
	gr.CreatedAt = now
	gr.UpdatedAt = now
	_, err := s.db.Exec(`INSERT INTO route_rules (id, graph_id, name, rules_json, is_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		gr.ID, gr.GraphID, gr.Name, gr.RulesJSON, gr.IsDefault, gr.CreatedAt, gr.UpdatedAt)
	return gr, err
}

// ListRouteRules возвращает правила для графа + глобальные дефолтные правила.
// Если graphID пустой — возвращает все правила (включая глобальные).
func (s *Store) ListRouteRules(graphID string) ([]RouteRule, error) {
	var rows *sql.Rows
	var err error
	if graphID == "" {
		rows, err = s.db.Query(`SELECT ` + routeRuleCols + ` FROM route_rules ORDER BY is_default DESC, name`)
	} else {
		// Получаем правила графа + глобальные дефолтные
		rows, err = s.db.Query(`SELECT `+routeRuleCols+` FROM route_rules 
			WHERE graph_id = ? OR (graph_id = '' AND is_default = 1)
			ORDER BY is_default DESC, name`, graphID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []RouteRule
	for rows.Next() {
		var r RouteRule
		if err := rows.Scan(&r.ID, &r.GraphID, &r.Name, &r.RulesJSON, &r.IsDefault, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// GetRouteRule возвращает правило по ID.
func (s *Store) GetRouteRule(id string) (RouteRule, error) {
	var rr RouteRule
	err := s.db.QueryRow(`SELECT `+routeRuleCols+` FROM route_rules WHERE id = ?`, id).
		Scan(&rr.ID, &rr.GraphID, &rr.Name, &rr.RulesJSON, &rr.IsDefault, &rr.CreatedAt, &rr.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return rr, ErrNotFound
	}
	return rr, err
}

// UpdateRouteRule обновляет правило (запрет для дефолтных).
func (s *Store) UpdateRouteRule(id string, name, rulesJSON string) error {
	// Проверяем, не является ли правило дефольным
	var isDefault bool
	err := s.db.QueryRow(`SELECT is_default FROM route_rules WHERE id = ?`, id).Scan(&isDefault)
	if err != nil {
		return err
	}
	if isDefault {
		return errors.New("нельзя изменить дефольное правило")
	}
	_, err = s.db.Exec(`UPDATE route_rules SET name = ?, rules_json = ?, updated_at = ? WHERE id = ?`,
		name, rulesJSON, time.Now().Unix(), id)
	return err
}

// DeleteRouteRule удаляет правило и все привязки к inbounds (запрет для дефольных).
func (s *Store) DeleteRouteRule(id string) error {
	var isDefault bool
	err := s.db.QueryRow(`SELECT is_default FROM route_rules WHERE id = ?`, id).Scan(&isDefault)
	if err != nil {
		return err
	}
	if isDefault {
		return errors.New("нельзя удалить дефольное правило")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`DELETE FROM inbound_routes WHERE route_rule_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM route_rules WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ListInboundRouteRuleIDs возвращает все route_rule_id для указанного inbound.
func (s *Store) ListInboundRouteRuleIDs(inboundID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT route_rule_id FROM inbound_routes WHERE inbound_id = ?`, inboundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// AssignInboundRouteRule назначает route_rule на inbound (можно назначить несколько).
func (s *Store) AssignInboundRouteRule(inboundID, routeRuleID string) error {
	_, err := s.db.Exec(`INSERT INTO inbound_routes (id, inbound_id, route_rule_id) VALUES (?, ?, ?)`,
		mustUUID(), inboundID, routeRuleID)
	return err
}

// RemoveInboundRouteRule снимает конкретный route_rule с inbound.
func (s *Store) RemoveInboundRouteRule(inboundID, routeRuleID string) error {
	_, err := s.db.Exec(`DELETE FROM inbound_routes WHERE inbound_id = ? AND route_rule_id = ?`,
		inboundID, routeRuleID)
	return err
}

// ListInboundRouteAssignments возвращает все привязки для графа: inbound_id -> []route_rule_id.
func (s *Store) ListInboundRouteAssignments(graphID string) (map[string][]string, error) {
	rows, err := s.db.Query(`
		SELECT ir.inbound_id, ir.route_rule_id
		FROM inbound_routes ir
		JOIN graph_nodes gn ON gn.id = ir.inbound_id
		WHERE gn.graph_id = ?`, graphID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]string)
	for rows.Next() {
		var inboundID, routeRuleID string
		if err := rows.Scan(&inboundID, &routeRuleID); err != nil {
			return nil, err
		}
		out[inboundID] = append(out[inboundID], routeRuleID)
	}
	return out, rows.Err()
}
