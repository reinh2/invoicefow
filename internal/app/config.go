// Package app contains process configuration and dependency composition.
package app

import (
	"fmt"
	"os"
	"time"
)

const (
	defaultDatabaseURL = "postgres://invoiceflow:invoiceflow@127.0.0.1:5432/invoiceflow?sslmode=disable"
	defaultAPIAddress  = "127.0.0.1:8080"
	defaultDemoActor   = "local-demo"
)

// Config is deliberately small at the foundation stage. Values that affect
// authority are server configuration, never request data.
type Config struct {
	DatabaseURL  string
	APIAddress   string
	MigrationDir string
	DemoActor    string
	DBTimeout    time.Duration
	StorageDir   string
}

func LoadConfig() (Config, error) {
	c := Config{
		DatabaseURL:  envOr("DATABASE_URL", defaultDatabaseURL),
		APIAddress:   envOr("API_ADDR", defaultAPIAddress),
		MigrationDir: envOr("MIGRATIONS_DIR", "db/migrations"),
		DemoActor:    envOr("DEMO_ACTOR", defaultDemoActor),
		DBTimeout:    10 * time.Second,
		StorageDir:   envOr("STORAGE_DIR", "./var/storage"),
	}
	if c.DemoActor == "" {
		return Config{}, fmt.Errorf("demo actor must not be empty")
	}
	return c, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
