package main

import (
	"log"
	"net/http"

	"github.com/CascadiaLabs/panel/api"
	"github.com/CascadiaLabs/panel/auth"
	"github.com/CascadiaLabs/panel/config"
	"github.com/CascadiaLabs/panel/db"
)

func main() {
	cfg := config.Load()

	store, err := db.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("db init: %v", err)
	}

	// Первый запуск: создаём admin. Пароль задаётся PANEL_ADMIN_PASSWORD
	// или генерируется и печатается один раз.
	if created, generated, err := store.SeedAdmin(cfg.AdminPassword); err != nil {
		log.Fatalf("admin seed: %v", err)
	} else if created {
		if generated != "" {
			log.Printf("=== FIRST LOGIN: username=admin password=%s (change it after login) ===", generated)
		} else {
			log.Println("admin user created with PANEL_ADMIN_PASSWORD")
		}
	}

	if cfg.PanelToken == "" {
		log.Println("WARN: PANEL_TOKEN not set — API-token auth disabled (login page still works)")
	}

	if err := store.DeleteExpiredSessions(); err != nil {
		log.Printf("session cleanup: %v", err)
	}

	handler := api.NewHandler(store)
	authn := auth.NewAuthenticator(store, cfg.PanelToken)
	router := api.NewRouter(handler, authn, cfg.SessionTTL)

	log.Printf("panel listening on %s", cfg.ListenAddr)
	if cfg.TLSCert != "" || cfg.TLSKey != "" {
		if cfg.TLSCert == "" || cfg.TLSKey == "" {
			log.Fatal("PANEL_TLS_CERT and PANEL_TLS_KEY must be set together")
		}
		log.Printf("panel HTTPS enabled (cert=%s key=%s)", cfg.TLSCert, cfg.TLSKey)
		if err := http.ListenAndServeTLS(cfg.ListenAddr, cfg.TLSCert, cfg.TLSKey, router); err != nil {
			log.Fatalf("server: %v", err)
		}
		return
	}
	if err := http.ListenAndServe(cfg.ListenAddr, router); err != nil {
		log.Fatalf("server: %v", err)
	}
}
