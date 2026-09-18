package api

import (
	"encoding/json"
	"net/http"

	"github.com/CascadiaLabs/panel/db"
)

// --- Правила маршрутизации ---

func (h *Handler) ListRouteRules(w http.ResponseWriter, r *http.Request) {
	graphID := r.PathValue("graphId")
	rules, err := h.store.ListRouteRules(graphID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, rules)
}

func (h *Handler) CreateRouteRule(w http.ResponseWriter, r *http.Request) {
	graphID := r.PathValue("graphId")
	var req struct {
		Name     string `json:"name"`
		RulesJSON string `json:"rules_json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	// Проверяем, что граф существует
	if _, err := h.store.GetGraph(graphID); err != nil {
		http.Error(w, "graph not found", http.StatusBadRequest)
		return
	}
	rule := db.RouteRule{
		GraphID:   graphID,
		Name:      req.Name,
		RulesJSON: req.RulesJSON,
	}
	created, err := h.store.CreateRouteRule(rule)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, created)
}

func (h *Handler) UpdateRouteRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name      string `json:"name"`
		RulesJSON string `json:"rules_json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateRouteRule(id, req.Name, req.RulesJSON); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) DeleteRouteRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteRouteRule(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// GetInboundRouteAssignments возвращает все route_rule_id для указанного inbound.
func (h *Handler) GetInboundRouteAssignments(w http.ResponseWriter, r *http.Request) {
	inboundID := r.PathValue("inboundId")
	ruleIDs, err := h.store.ListInboundRouteRuleIDs(inboundID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string][]string{"route_rule_ids": ruleIDs})
}

// AssignInboundRoute назначает route_rule на inbound (можно добавить несколько).
func (h *Handler) AssignInboundRoute(w http.ResponseWriter, r *http.Request) {
	inboundID := r.PathValue("inboundId")
	var req struct {
		RouteRuleID string `json:"route_rule_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.store.AssignInboundRouteRule(inboundID, req.RouteRuleID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// RemoveInboundRoute снимает route_rule с inbound.
func (h *Handler) RemoveInboundRoute(w http.ResponseWriter, r *http.Request) {
	inboundID := r.PathValue("inboundId")
	routeRuleID := r.PathValue("routeRuleId")
	if err := h.store.RemoveInboundRouteRule(inboundID, routeRuleID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}
