package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/CascadiaLabs/panel/api"
	"github.com/CascadiaLabs/panel/auth"
	"github.com/CascadiaLabs/panel/config"
	"github.com/CascadiaLabs/panel/db"
	"golang.org/x/crypto/bcrypt"
)

// version задаётся при сборке (-X main.version=...); dev-сборки — "dev".
var version = "dev"

func main() {
	log.Printf("Cascadia Panel %s", version)

	// CLI-режим: docker exec cascadia-panel /app/panel panel reset-password [новый пароль].
	// Без пароля — генерируется случайный, печатается в консоль.
	if len(os.Args) > 2 && os.Args[1] == "panel" && os.Args[2] == "reset-password" {
		cfg := config.Load()
		store, err := db.New(cfg.DBPath)
		if err != nil {
			log.Fatalf("reset-password: %v", err)
		}
		pw := ""
		if len(os.Args) > 3 {
			pw = os.Args[3]
		}
		if pw == "" {
			b := make([]byte, 18)
			if _, err := rand.Read(b); err != nil {
				log.Fatalf("reset-password: %v", err)
			}
			pw = base64.RawURLEncoding.EncodeToString(b)
		}
		admin, err := store.GetUserByUsername("admin")
		if err != nil {
			log.Fatalf("reset-password: admin not found: %v", err)
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalf("reset-password: %v", err)
		}
		if err := store.UpdateUserPassword(admin.ID, string(hash)); err != nil {
			log.Fatalf("reset-password: %v", err)
		}
		// Все сессии admin инвалидируются (keepID пустой — удаляем все).
		if err := store.DeleteUserSessionsExcept(admin.ID, ""); err != nil {
			log.Fatalf("reset-password: %v", err)
		}
		fmt.Println(pw)
		return
	}

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
