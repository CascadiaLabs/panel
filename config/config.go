package config

import (
	"os"
	"time"
)

type Config struct {
	// PanelToken — machine-to-machine bearer-ключ для /api (dev-скрипты, curl).
	// Пустой — bearer-вход отключён, остаётся логин по паролю.
	PanelToken string
	// AdminPassword задаёт пароль admin при первом старте; пустой — генерируется случайный.
	AdminPassword string
	DBPath        string
	ListenAddr    string
	SessionTTL    time.Duration
}

func Load() Config {
	return Config{
		PanelToken:    getenv("PANEL_TOKEN", ""),
		AdminPassword: getenv("PANEL_ADMIN_PASSWORD", ""),
		DBPath:        getenv("DB_PATH", "./panel.db"),
		ListenAddr:    getenv("LISTEN_ADDR", "0.0.0.0:2083"),
		SessionTTL:    7 * 24 * time.Hour,
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
