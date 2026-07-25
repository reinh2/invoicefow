package app

import "testing"

func TestLoadConfigRequiresExplicitSecretForConfiguredWebhook(t *testing.T) {
	t.Setenv("WEBHOOK_MODE", "strict")
	t.Setenv("WEBHOOK_URL", "https://example.test/webhook")
	t.Setenv("WEBHOOK_SECRET", "")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("strict webhook configuration without an explicit secret was accepted")
	}
}

func TestLoadConfigRejectsUncontrolledControlledDestination(t *testing.T) {
	t.Setenv("WEBHOOK_MODE", "controlled")
	t.Setenv("WEBHOOK_URL", "http://127.0.0.1:8080/webhook")
	t.Setenv("WEBHOOK_SECRET", "test-secret")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("controlled mode accepted an arbitrary private destination")
	}
}

func TestLoadConfigAcceptsExplicitComposeControlledSecret(t *testing.T) {
	t.Setenv("WEBHOOK_MODE", "controlled")
	t.Setenv("WEBHOOK_URL", controlledWebhookURL)
	t.Setenv("WEBHOOK_SECRET", "test-secret")
	config, err := LoadConfig()
	if err != nil || config.WebhookSecret != "test-secret" {
		t.Fatalf("LoadConfig() = %+v, %v", config, err)
	}
}

func TestLoadConfigTreatsAnAbsentWebDirectoryAsAPIOnly(t *testing.T) {
	t.Setenv("WEB_DIR", "")
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.WebDir != "" {
		t.Fatalf("WebDir = %q, want an empty API-only configuration", config.WebDir)
	}

	t.Setenv("WEB_DIR", "/app/web")
	config, err = LoadConfig()
	if err != nil || config.WebDir != "/app/web" {
		t.Fatalf("LoadConfig() = %+v, %v", config, err)
	}
}

// The metrics endpoint has no authentication and reveals traffic volume, so
// sharing the API's address is the one mistake that would silently publish it.
func TestLoadConfigRefusesMetricsOnTheAPIAddress(t *testing.T) {
	t.Setenv("API_ADDR", "0.0.0.0:8080")
	t.Setenv("METRICS_ADDR", "0.0.0.0:8080")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("metrics were accepted on the API address")
	}

	t.Setenv("METRICS_ADDR", "127.0.0.1:9090")
	config, err := LoadConfig()
	if err != nil || config.MetricsAddress != "127.0.0.1:9090" {
		t.Fatalf("LoadConfig() = %+v, %v", config, err)
	}
}

// Off is the default: an unset variable must open no listener at all.
func TestLoadConfigLeavesMetricsDisabledByDefault(t *testing.T) {
	t.Setenv("METRICS_ADDR", "")
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.MetricsAddress != "" {
		t.Fatalf("MetricsAddress = %q, want disabled", config.MetricsAddress)
	}
}
