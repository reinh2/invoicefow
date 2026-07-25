// Package app contains process configuration and dependency composition.
package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
	// Extractor selects the structured-extraction provider. "fake" (the default)
	// is the deterministic offline demo path; "openai" opts in to a live
	// provider and requires OpenAIAPIKey. Only the worker consumes these.
	Extractor     string
	OpenAIAPIKey  string
	OpenAIModel   string
	OpenAIBaseURL string
	// PublicDemo marks a deployment that anyone on the internet can reach. It
	// changes no authority — there is still no authentication — it only lets the
	// interface state plainly that the instance is a shared demo whose data is
	// periodically erased. Default false keeps the local demo unchanged.
	PublicDemo bool
	// UploadRatePerMinute bounds uploads per client address. Zero disables
	// limiting, which is the default for the local demo.
	UploadRatePerMinute int
	// MetricsAddress is the listen address of the operational metrics endpoint
	// (ADR-017). Empty — the default — means no metrics listener is opened at
	// all. The endpoint has no authentication of its own and reveals traffic
	// volume, so it must never share the public API's address.
	MetricsAddress string
}

func LoadConfig() (Config, error) {
	c := Config{
		DatabaseURL:    envOr("DATABASE_URL", defaultDatabaseURL),
		APIAddress:     envOr("API_ADDR", defaultAPIAddress),
		MigrationDir:   envOr("MIGRATIONS_DIR", "db/migrations"),
		DemoActor:      envOr("DEMO_ACTOR", defaultDemoActor),
		DBTimeout:      10 * time.Second,
		StorageDir:     envOr("STORAGE_DIR", "./var/storage"),
		WebhookSecret:  os.Getenv("WEBHOOK_SECRET"),
		WebhookURL:     os.Getenv("WEBHOOK_URL"),
		WebhookMode:    envOr("WEBHOOK_MODE", "strict"),
		WebDir:         os.Getenv("WEB_DIR"),
		Extractor:      envOr("EXTRACTOR", "fake"),
		OpenAIAPIKey:   os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:    os.Getenv("OPENAI_MODEL"),
		OpenAIBaseURL:  os.Getenv("OPENAI_BASE_URL"),
		MetricsAddress: strings.TrimSpace(os.Getenv("METRICS_ADDR")),
	}
	publicDemo, err := envBool("PUBLIC_DEMO")
	if err != nil {
		return Config{}, err
	}
	c.PublicDemo = publicDemo
	uploadRate, err := envInt("UPLOAD_RATE_PER_MINUTE")
	if err != nil {
		return Config{}, err
	}
	c.UploadRatePerMinute = uploadRate
	if c.DemoActor == "" {
		return Config{}, fmt.Errorf("demo actor must not be empty")
	}
	if c.Extractor != "fake" && c.Extractor != "openai" {
		return Config{}, fmt.Errorf("EXTRACTOR must be fake or openai")
	}
	if c.Extractor == "openai" && c.OpenAIAPIKey == "" {
		return Config{}, fmt.Errorf("EXTRACTOR=openai requires a non-empty OPENAI_API_KEY")
	}
	// Refusing the API's own address is the one configuration mistake that would
	// silently publish operational volume on a public instance.
	if c.MetricsAddress != "" && c.MetricsAddress == c.APIAddress {
		return Config{}, fmt.Errorf("METRICS_ADDR must not be the same address as API_ADDR")
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

// envBool accepts only the two unambiguous spellings. A typo must not be read
// as "false" and quietly disable a protection an operator believed was on.
func envBool(key string) (bool, error) {
	switch strings.TrimSpace(os.Getenv(key)) {
	case "":
		return false, nil
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", key)
	}
}

func envInt(key string) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", key)
	}
	return value, nil
}
