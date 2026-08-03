package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	HTTPPort      string
	DBDriver      string
	DatabaseURL   string
	SyncInterval  time.Duration
	SessionSecret string
	Location      *time.Location
}

const minSessionSecretLen = 32

func Load() (Config, error) {
	cfg := Config{
		HTTPPort:      getenv("HTTP_PORT", "8083"),
		DBDriver:      getenv("DB_DRIVER", "sqlite"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		SessionSecret: os.Getenv("SESSION_SECRET"),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("config: DATABASE_URL is required")
	}
	if cfg.DBDriver != "mysql" && cfg.DBDriver != "sqlite" {
		return Config{}, fmt.Errorf("config: DB_DRIVER must be mysql or sqlite, got %q", cfg.DBDriver)
	}
	if len(cfg.SessionSecret) < minSessionSecretLen {
		return Config{}, fmt.Errorf("config: SESSION_SECRET is required and must be at least %d characters (generate with: openssl rand -hex 32)", minSessionSecretLen)
	}

	interval, err := time.ParseDuration(getenv("SYNC_INTERVAL", "15m"))
	if err != nil {
		return Config{}, fmt.Errorf("config: invalid SYNC_INTERVAL: %w", err)
	}
	cfg.SyncInterval = interval

	loc, err := time.LoadLocation(getenv("TIMEZONE", "UTC"))
	if err != nil {
		return Config{}, fmt.Errorf("config: invalid TIMEZONE: %w", err)
	}
	cfg.Location = loc

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
