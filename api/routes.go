package api

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/CascadiaLabs/panel/auth"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed static
var staticFS embed.FS

func NewRouter(h *Handler, authn *auth.Authenticator, sessionTTL time.Duration) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)

	ah := &authHandlers{
		store:   h.store,
		authn:   authn,
		limiter: auth.NewLoginLimiter(5, time.Minute),
		ttl:     sessionTTL,
	}

	r.Route("/api", func(r chi.Router) {
		// Логин — единственный открытый эндпоинт.
		r.Post("/auth/login", ah.Login)

		// Всё остальное — под аутентификацией (cookie-сессия или Bearer PANEL_TOKEN).
		r.Group(func(r chi.Router) {
			r.Use(authn.Middleware)
			r.Post("/auth/logout", ah.Logout)
			r.Get("/auth/me", ah.Me)
			r.Put("/auth/password", ah.ChangePassword)

			r.Route("/nodes", func(r chi.Router) {
				r.Get("/", h.ListNodes)
				r.Post("/", h.CreateNode)
				// Важно: /statuses регистрируется до /{id} (статический путь
				// приоритетнее параметра, но так читаемее).
				r.Get("/statuses", h.AllStatuses)
				r.Get("/{id}", h.GetNode)
				r.Put("/{id}", h.UpdateNode)
				r.Delete("/{id}", h.DeleteNode)
				r.Get("/{id}/status", h.GetStatus)
				r.Post("/{id}/push", h.PushConfig)
			})

			r.Route("/graphs", func(r chi.Router) {
				r.Get("/", h.ListGraphs)
				r.Post("/", h.CreateGraph)
				r.Get("/{id}", h.GetGraph)
				r.Put("/{id}", h.SaveGraph)
				r.Delete("/{id}", h.DeleteGraph)
				r.Post("/{id}/validate", h.ValidateGraph)
				r.Get("/{id}/configs", h.GraphConfigs)
				r.Post("/{id}/deploy", h.DeployGraph)
			})

			r.Post("/util/generate", h.GenerateSecret)
		})
	})

	// Serve the embedded frontend: real files under /assets/, SPA fallback
	// to index.html for everything else.
	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	r.Handle("/assets/*", http.FileServer(http.FS(static)))

	index, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		panic(err)
	}
	serveIndex := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(index)
	}

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		// Don't serve HTML for missing assets — the browser would refuse to
		// execute it as a module script and the app would stay blank.
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			http.NotFound(w, r)
			return
		}
		serveIndex(w, r)
	})

	return r
}
