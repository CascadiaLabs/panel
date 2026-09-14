package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/CascadiaLabs/panel/db"
	"github.com/CascadiaLabs/panel/node"
)

type Handler struct {
	store *db.Store
}

func NewHandler(store *db.Store) *Handler {
	return &Handler{store: store}
}

func (h *Handler) ListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.store.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, nodes)
}

func (h *Handler) GetNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, err := h.store.Get(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, n)
}

func (h *Handler) CreateNode(w http.ResponseWriter, r *http.Request) {
	var n db.Node
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	created, err := h.store.Create(n)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, created)
}

func (h *Handler) UpdateNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var n db.Node
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.store.Update(id, n); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, n)
}

func (h *Handler) DeleteNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, err := h.store.Get(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	st, err := node.FetchStatus(ctx, n.GRPCURL, n.Token, n.CertPEM)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, st)
}

// StatusResult — статус одной ноды: либо данные, либо причина недоступности.
type StatusResult struct {
	Status *node.Status `json:"status,omitempty"`
	Error  string       `json:"error,omitempty"`
}

// AllStatuses опрашивает все ноды параллельно и возвращает карту id → статус.
// Недоступная нода не валит запрос — её статус содержит ошибку.
func (h *Handler) AllStatuses(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.store.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	results := make(map[string]StatusResult, len(nodes))
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for _, n := range nodes {
		wg.Add(1)
		go func(n db.Node) {
			defer wg.Done()
			nctx, ncancel := context.WithTimeout(ctx, 3*time.Second)
			defer ncancel()

			st, err := node.FetchStatus(nctx, n.GRPCURL, n.Token, n.CertPEM)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				results[n.ID] = StatusResult{Error: err.Error()}
			} else {
				results[n.ID] = StatusResult{Status: &st}
			}
		}(n)
	}
	wg.Wait()
	writeJSON(w, results)
}

func (h *Handler) PushConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, err := h.store.Get(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var body struct {
		ConfigJSON string `json:"config_json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.ConfigJSON == "" {
		body.ConfigJSON = n.ConfigJSON
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := node.PushConfig(ctx, n.GRPCURL, n.Token, n.CertPEM, body.ConfigJSON); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	// Preserve the existing record — only the config changes.
	n.ConfigJSON = body.ConfigJSON
	if err := h.store.Update(id, n); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
