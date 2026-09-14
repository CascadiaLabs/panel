package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CascadiaLabs/panel/auth"
	"github.com/CascadiaLabs/panel/db"
)

// newTestServer поднимает роутер с temp-БД, admin/adminpass и заданным api-token.
func newTestServer(t *testing.T, apiToken string) http.Handler {
	t.Helper()
	store, err := db.New(t.TempDir() + "/panel.db")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.SeedAdmin("adminpass"); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(store)
	authn := auth.NewAuthenticator(store, apiToken)
	return NewRouter(h, authn, time.Hour)
}

func do(t *testing.T, h http.Handler, method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLoginFlow(t *testing.T) {
	h := newTestServer(t, "secret-token")

	// неверный пароль → 401
	rec := do(t, h, "POST", "/api/auth/login", `{"username":"admin","password":"wrong"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: got %d", rec.Code)
	}

	// верный логин → cookie
	rec = do(t, h, "POST", "/api/auth/login", `{"username":"admin","password":"adminpass"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: got %d body=%s", rec.Code, rec.Body)
	}
	var resp struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.User.Username != "admin" {
		t.Fatalf("want admin, got %q", resp.User.Username)
	}

	cookies := rec.Result().Cookies()
	var sessionID string
	for _, c := range cookies {
		if c.Name == auth.CookieName {
			sessionID = c.Value
			if !c.HttpOnly {
				t.Fatal("session cookie must be HttpOnly")
			}
			if c.SameSite != http.SameSiteLaxMode {
				t.Fatal("session cookie must be SameSite=Lax")
			}
		}
	}
	if sessionID == "" {
		t.Fatal("login must set session cookie")
	}

	// cookie работает на защищённом эндпоинте
	rec = do(t, h, "GET", "/api/auth/me", "", map[string]string{"Cookie": auth.CookieName + "=" + sessionID})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "admin") {
		t.Fatalf("me with cookie: got %d body=%s", rec.Code, rec.Body)
	}

	// logout → сессия удалена
	rec = do(t, h, "POST", "/api/auth/logout", "", map[string]string{"Cookie": auth.CookieName + "=" + sessionID})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: got %d", rec.Code)
	}
	rec = do(t, h, "GET", "/api/auth/me", "", map[string]string{"Cookie": auth.CookieName + "=" + sessionID})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout: got %d", rec.Code)
	}
}

func TestBearerTokenWorks(t *testing.T) {
	h := newTestServer(t, "secret-token")
	hdr := map[string]string{"Authorization": "Bearer secret-token"}

	rec := do(t, h, "GET", "/api/nodes", "", hdr)
	if rec.Code != http.StatusOK {
		t.Fatalf("bearer on /api/nodes: got %d", rec.Code)
	}
	rec = do(t, h, "GET", "/api/auth/me", "", hdr)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "api-token") {
		t.Fatalf("bearer me: got %d body=%s", rec.Code, rec.Body)
	}

	// неверный bearer → 401
	rec = do(t, h, "GET", "/api/nodes", "", map[string]string{"Authorization": "Bearer nope"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong bearer: got %d", rec.Code)
	}
}

func TestAnonymousRejected(t *testing.T) {
	h := newTestServer(t, "secret-token")
	for _, path := range []string{"/api/nodes", "/api/auth/me", "/api/graphs"} {
		rec := do(t, h, "GET", path, "", nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s anonymous: got %d", path, rec.Code)
		}
	}
}

func TestChangePassword(t *testing.T) {
	h := newTestServer(t, "secret-token")

	login := do(t, h, "POST", "/api/auth/login", `{"username":"admin","password":"adminpass"}`, nil)
	cookie := ""
	for _, c := range login.Result().Cookies() {
		if c.Name == auth.CookieName {
			cookie = c.Value
		}
	}
	hdr := map[string]string{"Cookie": auth.CookieName + "=" + cookie}

	// вторая сессия с другого «браузера»
	login2 := do(t, h, "POST", "/api/auth/login", `{"username":"admin","password":"adminpass"}`, nil)
	cookie2 := ""
	for _, c := range login2.Result().Cookies() {
		if c.Name == auth.CookieName {
			cookie2 = c.Value
		}
	}

	// неверный текущий пароль → 401
	rec := do(t, h, "PUT", "/api/auth/password", `{"current_password":"bad","new_password":"newpass123"}`, hdr)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong current: got %d", rec.Code)
	}

	// короткий новый пароль → 400
	rec = do(t, h, "PUT", "/api/auth/password", `{"current_password":"adminpass","new_password":"short"}`, hdr)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("short new: got %d", rec.Code)
	}

	// смена пароля
	rec = do(t, h, "PUT", "/api/auth/password", `{"current_password":"adminpass","new_password":"newpass123"}`, hdr)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("change: got %d body=%s", rec.Code, rec.Body)
	}

	// старый пароль больше не работает
	rec = do(t, h, "POST", "/api/auth/login", `{"username":"admin","password":"adminpass"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old password after change: got %d", rec.Code)
	}

	// другая сессия инвалидирована, текущая — жива
	rec = do(t, h, "GET", "/api/auth/me", "", map[string]string{"Cookie": auth.CookieName + "=" + cookie2})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("other session must be invalidated: got %d", rec.Code)
	}
	rec = do(t, h, "GET", "/api/auth/me", "", hdr)
	if rec.Code != http.StatusOK {
		t.Fatalf("current session must survive: got %d", rec.Code)
	}

	// bearer не может менять пароль
	rec = do(t, h, "PUT", "/api/auth/password", `{"current_password":"x","new_password":"y12345678"}`,
		map[string]string{"Authorization": "Bearer secret-token"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("bearer password change: got %d", rec.Code)
	}
}

func TestLoginRateLimit(t *testing.T) {
	h := newTestServer(t, "secret-token")
	// 5 неудачных попыток → 429
	for i := 0; i < 5; i++ {
		rec := do(t, h, "POST", "/api/auth/login", `{"username":"admin","password":"wrong"}`, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: got %d", i, rec.Code)
		}
	}
	rec := do(t, h, "POST", "/api/auth/login", `{"username":"admin","password":"adminpass"}`, nil)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("after limit: got %d", rec.Code)
	}
}

func TestSeedAdminGenerated(t *testing.T) {
	store, err := db.New(t.TempDir() + "/panel.db")
	if err != nil {
		t.Fatal(err)
	}
	created, generated, err := store.SeedAdmin("")
	if err != nil || !created || generated == "" {
		t.Fatalf("seed: created=%v generated=%q err=%v", created, generated, err)
	}
	if len(generated) < 16 {
		t.Fatalf("generated password too short: %q", generated)
	}
	// повторный сидинг не создаёт второго пользователя и не меняет пароль
	created2, _, err := store.SeedAdmin("other")
	if err != nil || created2 {
		t.Fatalf("re-seed must be no-op: created=%v err=%v", created2, err)
	}
	_, err = store.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("admin missing after re-seed: %v", err)
	}
}
