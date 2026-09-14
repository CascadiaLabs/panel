package api

import (
	"net/http"
	"strings"

	"github.com/CascadiaLabs/panel/graph"
)

// Sub — публичная подписка VPN-пользователя: GET /sub/{sub_token}.
// Генерируется live из актуального состояния графа → подписка автоматически
// обновляется при любом изменении графа (добавлении/удалении inbound и т.п.).
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
	state, err := h.store.LoadGraphState(u.GraphID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	links, err := graph.ShareLinks(state, h.physNodes(r), panelCreds(u))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if len(links) == 0 {
		http.Error(w, "в графе нет entry-inbound", http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(strings.Join(links, "\n") + "\n"))
}
