package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/CascadiaLabs/panel/db"
	"github.com/CascadiaLabs/panel/graph"
	"github.com/CascadiaLabs/panel/node"
)

// --- Список/CRUD графов ---

func (h *Handler) ListGraphs(w http.ResponseWriter, r *http.Request) {
	graphs, err := h.store.ListGraphs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, graphs)
}

func (h *Handler) CreateGraph(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name              string `json:"name"`
		SubscriptionName  string `json:"subscription_name"`
		SubscriptionDesc  string `json:"subscription_desc"`
		SubscriptionSite  string `json:"subscription_site"`
		SubscriptionSupport string `json:"subscription_support"`
		ClientRoute       string `json:"client_route"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	g, err := h.store.CreateGraph(db.GraphCreate{
		Name:              body.Name,
		SubscriptionName:  body.SubscriptionName,
		SubscriptionDesc:  body.SubscriptionDesc,
		SubscriptionSite:  body.SubscriptionSite,
		SubscriptionSupport: body.SubscriptionSupport,
		ClientRoute:       body.ClientRoute,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, g)
}

type graphResponse struct {
	Graph      db.Graph          `json:"graph"`
	State      graph.State       `json:"state"`
	Validation *graph.Validation `json:"validation,omitempty"`
}

func (h *Handler) GetGraph(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	g, err := h.store.GetGraph(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	state, err := h.store.LoadGraphState(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, graphResponse{Graph: g, State: state})
}

// PUT /api/graphs/{id} — сохранение состояния канваса одним payload.
// Имя графа можно поменять полем name. Валидация выполняется, но ошибки не
// блокируют сохранение черновика (валидность нужна только для deploy/конфигов);
// ответ содержит результат валидации.
func (h *Handler) SaveGraph(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := h.store.GetGraph(id); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var body struct {
		Name  *string      `json:"name"`
		State *graph.State `json:"state"`
		// Поля подписки (перезаписывают существующие при наличии)
		SubscriptionName  *string `json:"subscription_name"`
		SubscriptionDesc  *string `json:"subscription_desc"`
		SubscriptionSite  *string `json:"subscription_site"`
		SubscriptionSupport *string `json:"subscription_support"`
		ClientRoute       *string `json:"client_route"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Name != nil && *body.Name != "" {
		if err := h.store.RenameGraph(id, *body.Name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	// Обновляем метаданные подписки и клиентскую маршрутизацию, если переданы
	if body.SubscriptionName != nil || body.SubscriptionDesc != nil ||
		body.SubscriptionSite != nil || body.SubscriptionSupport != nil ||
		body.ClientRoute != nil {
		sub := db.SubscriptionSettings{}
			if body.SubscriptionName != nil {
				sub.SubscriptionName = *body.SubscriptionName
			}
			if body.SubscriptionDesc != nil {
				sub.SubscriptionDesc = *body.SubscriptionDesc
			}
			if body.SubscriptionSite != nil {
				sub.SubscriptionSite = *body.SubscriptionSite
			}
			if body.SubscriptionSupport != nil {
				sub.SubscriptionSupport = *body.SubscriptionSupport
			}
			if body.ClientRoute != nil {
				sub.ClientRoute = *body.ClientRoute
			}
		if err := h.store.UpdateGraphSubscription(id, sub); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	validation := (*graph.Validation)(nil)
	if body.State != nil {
		if err := h.validateGraphState(*body.State); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := h.store.SaveGraphState(id, *body.State); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// SaveGraphState добавляет отсутствующий служебный relay_user в inbound.
		// Валидируем сохранённое состояние, иначе первый ответ после сохранения
		// ошибочно сообщает, что каскадному inbound не хватает пользователей.
		state, err := h.store.LoadGraphState(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		vd := graph.Validator{State: state, Nodes: h.physNodes(r)}
		val := vd.Validate()
		validation = &val
	}
	g, err := h.store.GetGraph(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	state, err := h.store.LoadGraphState(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, graphResponse{Graph: g, State: state, Validation: validation})
}

func (h *Handler) DeleteGraph(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteGraph(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Валидация / конфиги / деплой ---

func (h *Handler) physNodes(_ *http.Request) []graph.PhysNode {
	nodes, err := h.store.List()
	if err != nil {
		return nil
	}
	out := make([]graph.PhysNode, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, graph.PhysNode{ID: n.ID, Name: n.Name, GRPCURL: n.GRPCURL})
	}
	return out
}

// validateGraphState проверяет базовую целостность payload (не семантику графа).
func (h *Handler) validateGraphState(state graph.State) error {
	ids := map[string]bool{}
	for _, n := range state.Nodes {
		if n.ID == "" {
			return errBad("элемент без id")
		}
		if ids[n.ID] {
			return errBad("дублирующийся id элемента: " + n.ID)
		}
		ids[n.ID] = true
	}
	for _, e := range state.Edges {
		if e.SourceID == "" || e.TargetID == "" {
			return errBad("ребро без source/target")
		}
		if !ids[e.SourceID] || !ids[e.TargetID] {
			return errBad("ребро ссылается на несуществующий элемент")
		}
	}
	return nil
}

type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

func errBad(msg string) error { return &validationError{msg: msg} }

func (h *Handler) ValidateGraph(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := h.store.GetGraph(id); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	state, err := h.store.LoadGraphState(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	vd := graph.Validator{State: state, Nodes: h.physNodes(r)}
	writeJSON(w, vd.Validate())
}

func (h *Handler) GraphConfigs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := h.store.GetGraph(id); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	state, err := h.store.LoadGraphState(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	inboundRouteRules, err := h.buildInboundRouteRules(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	configs, err := graph.Generate(state, h.physNodes(r), inboundRouteRules)
	if err != nil {
		writeErr422(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"configs": configs})
}

type deployReport struct {
	NodeID string `json:"node_id"`
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

// DeployGraph применяет сгенерированные конфиги ко всем задействованным нодам
// параллельно. Ошибка одной ноды не мешает остальным.
func (h *Handler) DeployGraph(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := h.store.GetGraph(id); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	reports, deployed, err := h.deployGraph(r, id)
	if err != nil {
		writeErr422(w, err.Error())
		return
	}
	if !deployed {
		w.WriteHeader(http.StatusMultiStatus)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	writeJSON(w, map[string]any{"deployed": deployed, "results": reports})
}

// deployGraph генерирует конфиги графа из актуального состояния и пушит их
// на ноды параллельно. Переиспользуется и user-операциями (инжект кредов).
func (h *Handler) deployGraph(r *http.Request, id string) ([]deployReport, bool, error) {
	state, err := h.store.LoadGraphState(id)
	if err != nil {
		return nil, false, err
	}

	// вшиваем активных клиентов панели: новый entry-inbound без юзера в стейте
	// дал бы ноде конфиг без uuid из подписки
	state = h.store.InjectPanelUsers(id, state)

	inboundRouteRules, err := h.buildInboundRouteRules(id)
	if err != nil {
		return nil, false, err
	}

	configs, err := graph.Generate(state, h.physNodes(r), inboundRouteRules)
	if err != nil {
		return nil, false, err
	}

	nodes, err := h.store.List()
	if err != nil {
		return nil, false, err
	}
	byID := map[string]db.Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	reports := make([]deployReport, 0, len(configs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for nodeID, cfg := range configs {
		n, ok := byID[nodeID]
		if !ok {
			reports = append(reports, deployReport{NodeID: nodeID, Name: nodeID, Error: "нода удалена из панели"})
			continue
		}
		wg.Add(1)
		go func(n db.Node, cfg string) {
			defer wg.Done()
			rep := deployReport{NodeID: n.ID, Name: n.Name}
			pctx, pcancel := context.WithTimeout(ctx, 15*time.Second)
			defer pcancel()
			if err := node.PushConfig(pctx, n.GRPCURL, n.Token, n.CertPEM, cfg); err != nil {
				rep.Error = err.Error()
			} else {
				rep.OK = true
				// конфиг применён — фиксируем его в панели
				n.ConfigJSON = cfg
				if err := h.store.Update(n.ID, n); err != nil {
					rep.Error = "применён, но не сохранён в панели: " + err.Error()
					rep.OK = false
				}
			}
			mu.Lock()
			reports = append(reports, rep)
			mu.Unlock()
		}(n, cfg)
	}
	wg.Wait()

	sort.Slice(reports, func(i, j int) bool { return reports[i].Name < reports[j].Name })
	deployed := true
	for _, rep := range reports {
		if !rep.OK {
			deployed = false
		}
	}
	return reports, deployed, nil
}

func writeErr422(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	json.NewEncoder(w).Encode(map[string]any{"error": msg})
}

// --- Утилиты генерации секретов ---

func (h *Handler) GenerateSecret(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind string `json:"kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var value string
	var err error
	switch body.Kind {
	case "uuid":
		value = graph.GenerateUUID()
	case "password":
		value = graph.GeneratePassword()
	case "shortid":
		value = graph.GenerateShortID()
	case "reality":
		var pub string
		value, pub, err = graph.GenerateRealityKeyPair()
		if err == nil {
			writeJSON(w, map[string]string{"private_key": value, "public_key": pub})
			return
		}
	default:
		http.Error(w, "kind must be one of: uuid, password, shortid, reality", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"value": value})
}

// buildInboundRouteRules строит карту inbound_id -> []RouteRuleItem из назначений в БД.
func (h *Handler) buildInboundRouteRules(graphID string) (graph.InboundRouteRules, error) {
	assignments, err := h.store.ListInboundRouteAssignments(graphID)
	if err != nil {
		return nil, err
	}
	if len(assignments) == 0 {
		return graph.InboundRouteRules{}, nil
	}

	result := make(graph.InboundRouteRules)
	for inboundID, ruleIDs := range assignments {
		var items []graph.RouteRuleItem
		for _, ruleID := range ruleIDs {
			rule, err := h.store.GetRouteRule(ruleID)
			if err != nil {
				// Пропускаем несуществующие правила, но логируем
				continue
			}
			ruleItems, err := graph.ParseRouteRuleItems([]byte(rule.RulesJSON))
			if err != nil {
				continue
			}
			items = append(items, ruleItems...)
		}
		if len(items) > 0 {
			result[inboundID] = items
		}
	}
	return result, nil
}

// --- Утилиты генерации секретов ---
