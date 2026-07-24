// Package app contains process configuration and dependency composition.
package app

import (
	"fmt"
	"os"
	"time"
)

const (
	defaultDatabaseURL   = "postgres://invoiceflow:invoiceflow@127.0.0.1:5432/invoiceflow?sslmode=disable"
	defaultAPIAddress    = "127.0.0.1:8080"
	defaultDemoActor     = "local-demo"
	controlledWebhookURL = "http://receiver:8090/webhook"
)

// Config is deliberately small at the foundation stage. Values that affect
// authority are server configuration, never request data.
type Config struct {
	DatabaseURL   string
	APIAddress    string
	MigrationDir  string
	DemoActor     string
	DBTimeout     time.Duration
	StorageDir    string
	WebhookSecret string
	WebhookURL    string
	WebhookMode   string
	// WebDir points at one pre-built browser bundle (ADR-013). Empty means the
	// process serves the JSON API only.
	WebDir string
}

func LoadConfig() (Config, error) {
	c := Config{
		DatabaseURL:   envOr("DATABASE_URL", defaultDatabaseURL),
		APIAddress:    envOr("API_ADDR", defaultAPIAddress),
		MigrationDir:  envOr("MIGRATIONS_DIR", "db/migrations"),
		DemoActor:     envOr("DEMO_ACTOR", defaultDemoActor),
		DBTimeout:     10 * time.Second,
		StorageDir:    envOr("STORAGE_DIR", "./var/storage"),
		WebhookSecret: os.Getenv("WEBHOOK_SECRET"),
		WebhookURL:    os.Getenv("WEBHOOK_URL"),
		WebhookMode:   envOr("WEBHOOK_MODE", "strict"),
		WebDir:        os.Getenv("WEB_DIR"),
	}
	if c.DemoActor == "" {
		return Config{}, fmt.Errorf("demo actor must not be empty")
	}
	if c.WebhookMode != "strict" && c.WebhookMode != "controlled" {
		return Config{}, fmt.Errorf("WEBHOOK_MODE must be strict or controlled")
	}
	if c.WebhookURL != "" && c.WebhookSecret == "" {
		return Config{}, fmt.Errorf("webhook delivery requires an explicit non-empty WEBHOOK_SECRET")
	}
	if c.WebhookMode == "controlled" && c.WebhookURL != controlledWebhookURL {
		return Config{}, fmt.Errorf("controlled webhook mode only permits %s", controlledWebhookURL)
	}
	return c, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
