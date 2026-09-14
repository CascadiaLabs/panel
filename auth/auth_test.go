package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeSessions map[string]string

func (f fakeSessions) ValidSession(id string) (string, bool) {
	uid, ok := f[id]
	return uid, ok
}

func testAuth(token string) *Authenticator {
	return NewAuthenticator(fakeSessions{"sess-alice": "user-alice"}, token)
}

func ok(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		next.ServeHTTP(w, r)
	})
}

func TestSessionCookieAuth(t *testing.T) {
	a := testAuth("tok")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "sess-alice"})
	if uid, ok := a.Authenticated(req); !ok || uid != "user-alice" {
		t.Fatalf("want user-alice ok, got %q %v", uid, ok)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "sess-unknown"})
	if _, ok := a.Authenticated(req); ok {
		t.Fatal("unknown session must not authenticate")
	}
}

func TestBearerAuth(t *testing.T) {
	a := testAuth("tok")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer tok")
	if uid, ok := a.Authenticated(req); !ok || uid != "" {
		t.Fatalf("bearer must pass with empty uid, got %q %v", uid, ok)
	}

	req.Header.Set("Authorization", "Bearer wrong")
	if _, ok := a.Authenticated(req); ok {
		t.Fatal("wrong token must fail")
	}

	req.Header.Set("Authorization", "Basic dG9r")
	if _, ok := a.Authenticated(req); ok {
		t.Fatal("non-bearer scheme must fail")
	}
}

func TestNoTokenDisablesBearer(t *testing.T) {
	a := testAuth("")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer ")
	if _, ok := a.Authenticated(req); ok {
		t.Fatal("empty api token must disable bearer auth")
	}
	// но сессии работают
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "sess-alice"})
	if _, ok := a.Authenticated(req); !ok {
		t.Fatal("session must still work when token unset")
	}
}

func TestMiddlewareUnauthorized(t *testing.T) {
	a := testAuth("tok")
	called := false
	h := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/nodes", nil))
	if rec.Code != http.StatusUnauthorized || called {
		t.Fatalf("want 401 not called, got %d called=%v", rec.Code, called)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "sess-alice"})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !called {
		t.Fatalf("want 200 called, got %d called=%v", rec.Code, called)
	}
}

func TestLoginLimiter(t *testing.T) {
	l := NewLoginLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.Allow("1.2.3.4") {
			t.Fatal("attempt within limit rejected")
		}
	}
	if l.Allow("1.2.3.4") {
		t.Fatal("4th attempt must be rejected")
	}
	if !l.Allow("5.6.7.8") {
		t.Fatal("other ip must be unaffected")
	}
	l.Reset("1.2.3.4")
	if !l.Allow("1.2.3.4") {
		t.Fatal("reset must clear the bucket")
	}
}

func TestTLSDetection(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if IsTLS(req) {
		t.Fatal("plain request is not TLS")
	}
	req.Header.Set("X-Forwarded-Proto", "https")
	if !IsTLS(req) {
		t.Fatal("X-Forwarded-Proto: https must count as TLS")
	}
}
