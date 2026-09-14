package auth

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/CascadiaLabs/panel/db"
)

// SessionValidator абстрагирует источник сессий (db.Store).
type SessionValidator interface {
	ValidSession(id string) (userID string, ok bool)
}

const (
	// CookieName — имя сессионной cookie.
	CookieName = "cascadia_session"
	// BypassHeaderAuth защищает login-эндпоинт от передачи в нём bearer-токена
	// (иначе curl с PANEL_TOKEN мог бы получить cookie, это не нужно).
	defaultSessionTTL = 7 * 24 * time.Hour
)

// Authenticator решает, пускать ли запрос: валидная сессия (cookie) или
// Bearer PANEL_TOKEN (machine-to-machine, для скриптов вроде dev/stack.py).
type Authenticator struct {
	sessions SessionValidator
	apiToken string // может быть пустым — тогда остаётся только cookie-аутентификация
}

func NewAuthenticator(sessions SessionValidator, apiToken string) *Authenticator {
	return &Authenticator{sessions: sessions, apiToken: apiToken}
}

// Authenticated возвращает userID (для сессии) или "" (для bearer) и true, если доступ разрешён.
func (a *Authenticator) Authenticated(r *http.Request) (userID string, ok bool) {
	if a == nil {
		return "", false
	}
	if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
		if uid, valid := a.sessions.ValidSession(c.Value); valid {
			return uid, true
		}
	}
	if a.apiToken != "" {
		h := r.Header.Get("Authorization")
		if strings.HasPrefix(h, "Bearer ") {
			got := strings.TrimPrefix(h, "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(a.apiToken)) == 1 {
				return "", true
			}
		}
	}
	return "", false
}

// Middleware отклоняет запросы без валидной сессии/bearer-токена;
// id пользователя кладётся в контекст (пустой для bearer).
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := a.Authenticated(r)
		if !ok {
			writeUnauthorized(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), userID)))
	})
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized"}` + "\n"))
}

// SetSessionCookie выставляет HttpOnly-cookie сессии.
// Secure только при HTTPS — панель обычно за HTTP на закрытом адресе.
func SetSessionCookie(w http.ResponseWriter, sessionID string, expires time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  expires,
	})
}

func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// IsTLS сообщает, пришёл ли запрос по TLS (для флага Secure у cookie).
func IsTLS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// LoginLimiter — простой in-memory rate-limit: не более max неудачных
// попыток входа за окно window с одного IP. Успешный вход сбрасывает счётчик.
type LoginLimiter struct {
	max    int
	window time.Duration

	now     func() time.Time
	buckets map[string][]time.Time
}

func NewLoginLimiter(max int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{max: max, window: window, now: time.Now, buckets: map[string][]time.Time{}}
}

func (l *LoginLimiter) Allow(ip string) bool {
	now := l.now()
	cutoff := now.Add(-l.window)
	hits := l.buckets[ip][:0]
	for _, t := range l.buckets[ip] {
		if t.After(cutoff) {
			hits = append(hits, t)
		}
	}
	l.buckets[ip] = hits
	if len(hits) >= l.max {
		return false
	}
	l.buckets[ip] = append(hits, now)
	return true
}

func (l *LoginLimiter) Reset(ip string) {
	delete(l.buckets, ip)
}

// Assert interfaces.
var _ SessionValidator = (*db.Store)(nil)

// contextKey — приватный тип для ключей контекста.
type contextKey int

const userIDKey contextKey = 0

// WithUserID кладёт id пользователя (пустой для bearer) в контекст.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// UserIDFromContext достаёт id пользователя из контекста ("" для bearer-запросов).
func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userIDKey).(string); ok {
		return v
	}
	return ""
}
