package api

import (
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/CascadiaLabs/panel/auth"
	"github.com/CascadiaLabs/panel/db"
	"golang.org/x/crypto/bcrypt"
)

type authHandlers struct {
	store   *db.Store
	authn   *auth.Authenticator
	limiter *auth.LoginLimiter
	ttl     time.Duration
}

// POST /api/auth/login {username, password}
func (h *authHandlers) Login(w http.ResponseWriter, r *http.Request) {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		ip = r.RemoteAddr
	}
	if !h.limiter.Allow(ip) {
		http.Error(w, "too many attempts, try again later", http.StatusTooManyRequests)
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	user, err := h.store.GetUserByUsername(body.Username)
	if err != nil {
		// Одинаковая ошибка для «нет пользователя» и «неверный пароль».
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)) != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	sess, err := h.store.CreateSession(user.ID, h.ttl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.limiter.Reset(ip)
	auth.SetSessionCookie(w, sess.ID, time.Unix(sess.ExpiresAt, 0), auth.IsTLS(r))
	writeJSON(w, map[string]any{"user": map[string]string{"id": user.ID, "username": user.Username}})
}

// POST /api/auth/logout
func (h *authHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.CookieName); err == nil && c.Value != "" {
		_ = h.store.DeleteSession(c.Value)
	}
	auth.ClearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/auth/me
func (h *authHandlers) Me(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFromContext(r.Context())
	if uid == "" {
		// bearer-токен: сессии нет, но пользователь — системный.
		writeJSON(w, map[string]any{"user": map[string]string{"id": "", "username": "api-token"}, "bearer": true})
		return
	}
	user, err := h.store.GetUserByID(uid)
	if err != nil {
		http.Error(w, "not found", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{"user": map[string]string{"id": user.ID, "username": user.Username}})
}

// PUT /api/auth/password {current_password, new_password}
func (h *authHandlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFromContext(r.Context())
	if uid == "" {
		http.Error(w, "password change requires a logged-in session, not an API token", http.StatusForbidden)
		return
	}

	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(body.NewPassword) < 8 {
		http.Error(w, "new password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	user, err := h.store.GetUserByID(uid)
	if err != nil {
		http.Error(w, "not found", http.StatusUnauthorized)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.CurrentPassword)) != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.store.UpdateUserPassword(uid, string(hash)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Все прочие сессии пользователя инвалидируются; текущая — сохраняется.
	if c, err := r.Cookie(auth.CookieName); err == nil {
		_ = h.store.DeleteUserSessionsExcept(uid, c.Value)
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(r *http.Request, v any) error {
	// 8 КБ хватает для логина/пароля с запасом.
	r.Body = http.MaxBytesReader(nil, r.Body, 8<<10)
	return json.NewDecoder(r.Body).Decode(v)
}
