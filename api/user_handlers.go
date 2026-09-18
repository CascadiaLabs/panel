package api

import (
	"encoding/json"
	"net/http"

	"github.com/CascadiaLabs/panel/db"
	"github.com/CascadiaLabs/panel/graph"
)

// --- VPN-пользователи: CRUD + вшивание кредов в граф + деплой ---

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.ListPanelUsers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, users)
}

type userBody struct {
	Name            *string `json:"name"`
	GraphID         string  `json:"graph_id"`
	Remark          *string `json:"remark"`
	Flow            *string `json:"flow"` // vless reality: xtls-rprx-vision
	Enabled         *bool   `json:"enabled"`
	UsedUpload      *int64  `json:"used_upload"`
	UsedDownload    *int64  `json:"used_download"`
	TotalTraffic    *int64  `json:"total_traffic"`
	ExpireTime      *int64  `json:"expire_time"`
}

// userOpResponse — ответ мутирующих операций: пользователь + результат деплоя
// (деплой может упасть на невалидном/неразвёрнутом графе — это предупреждение,
// а не ошибка создания: креды уже вшиты в состояние графа).
type userOpResponse struct {
	User    db.PanelUser `json:"user"`
	Deploy  *deployOut   `json:"deploy,omitempty"`
	Warning string       `json:"warning,omitempty"`
}

type deployOut struct {
	Deployed bool           `json:"deployed"`
	Results  []deployReport `json:"results,omitempty"`
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var body userBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Name == nil || *body.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if body.GraphID == "" {
		http.Error(w, "graph_id is required", http.StatusBadRequest)
		return
	}
	if _, err := h.store.GetGraph(body.GraphID); err != nil {
		http.Error(w, "граф не найден", http.StatusBadRequest)
		return
	}

	u, err := h.store.CreatePanelUser(*body.Name, body.GraphID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if body.Remark != nil && *body.Remark != u.Remark {
		u.Remark = *body.Remark
		if err := h.store.UpdatePanelUser(u); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if body.Flow != nil && *body.Flow != u.Flow {
		u.Flow = *body.Flow
		if err := h.store.UpdatePanelUser(u); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := h.applyUserToGraph(r, u.GraphID, panelCreds(u), true); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := userOpResponse{User: u}
	resp.Deploy, resp.Warning = h.runDeploy(r, u.GraphID)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, resp)
}

func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	u, err := h.store.GetPanelUser(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var body userBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	prevEnabled := u.Enabled
	flowChanged := body.Flow != nil && *body.Flow != u.Flow

	if body.Name != nil && *body.Name != "" && *body.Name != u.Name {
		u.Name = *body.Name
	}
	if body.Remark != nil && *body.Remark != u.Remark {
		u.Remark = *body.Remark
	}
	if flowChanged {
		u.Flow = *body.Flow
	}
	if body.Enabled != nil {
		u.Enabled = *body.Enabled
	}
	// Обновляем трафик, если переданы новые значения
	if body.UsedUpload != nil {
		u.UsedUpload = *body.UsedUpload
	}
	if body.UsedDownload != nil {
		u.UsedDownload = *body.UsedDownload
	}
	if body.TotalTraffic != nil {
		u.TotalTraffic = *body.TotalTraffic
	}
	if body.ExpireTime != nil {
		u.ExpireTime = *body.ExpireTime
	}

	// переезд на другой граф: вычистить креды из старого, вшить в новый
	moved := false
	if body.GraphID != "" && body.GraphID != u.GraphID {
		if _, err := h.store.GetGraph(body.GraphID); err != nil {
			http.Error(w, "граф не найден", http.StatusBadRequest)
			return
		}
		if prevEnabled {
			if err := h.applyUserToGraph(r, u.GraphID, panelCreds(u), false); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = h.runDeploy(r, u.GraphID)
		}
		if err := h.store.MovePanelUser(id, body.GraphID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		u.GraphID = body.GraphID
		moved = true
	}

	// синхронизация кредов: включение/выключение, переезд или смена flow у активного
	needSync := prevEnabled != u.Enabled || (moved && u.Enabled) || (flowChanged && u.Enabled)
	var resp userOpResponse
	if needSync {
		if flowChanged {
			_ = h.applyUserToGraph(r, u.GraphID, panelCreds(u), false) // убрать по uuid, вшить с новым flow
		}
		if err := h.applyUserToGraph(r, u.GraphID, panelCreds(u), u.Enabled); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp = userOpResponse{User: u}
		resp.Deploy, resp.Warning = h.runDeploy(r, u.GraphID)
	}

	// обновляем трафик, если были изменения
	if body.UsedUpload != nil || body.UsedDownload != nil || body.TotalTraffic != nil || body.ExpireTime != nil {
		if err := h.store.UpdatePanelUserTraffic(id, u.UsedUpload, u.UsedDownload, u.TotalTraffic, u.ExpireTime); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := h.store.UpdatePanelUser(u); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if resp.User.ID == "" {
		resp = userOpResponse{User: u}
	}
	writeJSON(w, resp)
}

func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	u, err := h.store.GetPanelUser(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if u.Enabled {
		if err := h.applyUserToGraph(r, u.GraphID, panelCreds(u), false); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = h.runDeploy(r, u.GraphID)
	}
	if err := h.store.DeletePanelUser(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// applyUserToGraph вшивает (inject=true) либо убирает (inject=false) креды
// пользователя во все entry-inbound графа и сохраняет состояние.
func (h *Handler) applyUserToGraph(r *http.Request, graphID string, creds graph.PanelCreds, inject bool) error {
	state, err := h.store.LoadGraphState(graphID)
	if err != nil {
		return err
	}
	if inject {
		state = graph.InjectUsersIntoState(state, creds)
	} else {
		state = graph.RemoveUserFromState(state, creds)
	}
	return h.store.SaveGraphState(graphID, state)
}

func (h *Handler) runDeploy(r *http.Request, graphID string) (*deployOut, string) {
	reports, deployed, err := h.deployGraph(r, graphID)
	if err != nil {
		return nil, err.Error()
	}
	return &deployOut{Deployed: deployed, Results: reports}, ""
}

func panelCreds(u db.PanelUser) graph.PanelCreds {
	return graph.PanelCreds{Name: u.Name, UUID: u.UUID, Password: u.Password, Flow: u.Flow, Remark: u.Remark}
}
