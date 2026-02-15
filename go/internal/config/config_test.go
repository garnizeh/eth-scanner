package config

import (
	"testing"
	"time"
)

func TestLoad_DefaultsAndRequired(t *testing.T) {
	t.Setenv("MASTER_DB_PATH", ":memory:")
	// ensure other vars unset
	t.Setenv("MASTER_PORT", "")
	t.Setenv("MASTER_LOG_LEVEL", "")
	t.Setenv("MASTER_SHUTDOWN_TIMEOUT", "")
	t.Setenv("MASTER_API_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Fatalf("expected default port 8080, got %q", cfg.Port)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("expected default loglevel info, got %q", cfg.LogLevel)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Fatalf("expected default shutdown timeout 30s, got %v", cfg.ShutdownTimeout)
	}
	if cfg.APIKey != "" {
		t.Fatalf("expected empty APIKey when MASTER_API_KEY unset, got %q", cfg.APIKey)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	t.Setenv("MASTER_DB_PATH", "/tmp/test.db")
	t.Setenv("MASTER_PORT", "12345")
	t.Setenv("MASTER_LOG_LEVEL", "DEBUG")
	t.Setenv("MASTER_SHUTDOWN_TIMEOUT", "45s")
	t.Setenv("MASTER_API_KEY", "s3cr3t")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Port != "12345" {
		t.Fatalf("expected port 12345, got %q", cfg.Port)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("expected loglevel debug, got %q", cfg.LogLevel)
	}
	if cfg.ShutdownTimeout != 45*time.Second {
		t.Fatalf("expected shutdown timeout 45s, got %v", cfg.ShutdownTimeout)
	}
	if cfg.APIKey != "s3cr3t" {
		t.Fatalf("expected APIKey s3cr3t, got %q", cfg.APIKey)
	}
}

func TestLoad_InvalidTimeout(t *testing.T) {
	t.Setenv("MASTER_DB_PATH", ":memory:")
	t.Setenv("MASTER_SHUTDOWN_TIMEOUT", "notaduration")
	t.Setenv("MASTER_API_KEY", "")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for invalid shutdown timeout, got nil")
	}
}

func TestLoad_MissingDBPath(t *testing.T) {
	// Ensure DB path is unset
	t.Setenv("MASTER_DB_PATH", "")
	t.Setenv("MASTER_PORT", "")
	t.Setenv("MASTER_LOG_LEVEL", "")
	t.Setenv("MASTER_SHUTDOWN_TIMEOUT", "")
	t.Setenv("MASTER_API_KEY", "")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error when MASTER_DB_PATH is missing, got nil")
	}
}
