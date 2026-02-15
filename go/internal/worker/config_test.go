package worker

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadConfig_Valid(t *testing.T) {
	// set env
	os.Setenv("WORKER_API_URL", "http://localhost:8080")
	defer os.Unsetenv("WORKER_API_URL")
	os.Setenv("WORKER_API_KEY", "test-key")
	defer os.Unsetenv("WORKER_API_KEY")
	os.Setenv("WORKER_ID", "test-worker-01")
	defer os.Unsetenv("WORKER_ID")
	os.Setenv("WORKER_CHECKPOINT_INTERVAL", "2s")
	defer os.Unsetenv("WORKER_CHECKPOINT_INTERVAL")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.APIURL != "http://localhost:8080" {
		t.Fatalf("unexpected APIURL: %s", cfg.APIURL)
	}
	if cfg.APIKey != "test-key" {
		t.Fatalf("unexpected APIKey: %s", cfg.APIKey)
	}
	if cfg.WorkerID != "test-worker-01" {
		t.Fatalf("unexpected WorkerID: %s", cfg.WorkerID)
	}
	if cfg.CheckpointInterval != 2*time.Second {
		t.Fatalf("unexpected CheckpointInterval: %v", cfg.CheckpointInterval)
	}
}

func TestLoadConfig_AutoGenerateID(t *testing.T) {
	os.Setenv("WORKER_API_URL", "http://localhost:8080")
	defer os.Unsetenv("WORKER_API_URL")
	os.Setenv("WORKER_API_KEY", "test-key")
	defer os.Unsetenv("WORKER_API_KEY")
	os.Unsetenv("WORKER_ID")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.WorkerID == "" {
		t.Fatalf("expected auto-generated WorkerID, got empty")
	}
	if !strings.HasPrefix(cfg.WorkerID, "worker-pc-") {
		t.Fatalf("unexpected WorkerID format: %s", cfg.WorkerID)
	}
}

func TestLoadConfig_MissingAPIURL(t *testing.T) {
	os.Unsetenv("WORKER_API_URL")
	os.Setenv("WORKER_API_KEY", "test-key")
	defer os.Unsetenv("WORKER_API_KEY")

	_, err := LoadConfig()
	if err == nil {
		t.Fatalf("expected error when WORKER_API_URL missing")
	}
}

func TestLoadConfig_InvalidInterval(t *testing.T) {
	os.Setenv("WORKER_API_URL", "http://localhost:8080")
	defer os.Unsetenv("WORKER_API_URL")
	os.Setenv("WORKER_API_KEY", "test-key")
	defer os.Unsetenv("WORKER_API_KEY")
	os.Setenv("WORKER_CHECKPOINT_INTERVAL", "notaduration")
	defer os.Unsetenv("WORKER_CHECKPOINT_INTERVAL")

	_, err := LoadConfig()
	if err == nil {
		t.Fatalf("expected error for invalid checkpoint interval")
	}
}

func TestLoadConfig_InvalidAPIURLWrapping(t *testing.T) {
	os.Setenv("WORKER_API_URL", "not-a-url://")
	defer os.Unsetenv("WORKER_API_URL")
	os.Setenv("WORKER_API_KEY", "test-key")
	defer os.Unsetenv("WORKER_API_KEY")

	_, err := LoadConfig()
	if err == nil {
		t.Fatalf("expected error when WORKER_API_URL is invalid")
	}
	// top-level message should include our prefix
	if !strings.Contains(err.Error(), "invalid WORKER_API_URL") {
		t.Fatalf("expected wrapped error to include 'invalid WORKER_API_URL', got: %v", err)
	}
	// ensure the error is wrapped (unwrap should return underlying error)
	if errors.Unwrap(err) == nil {
		t.Fatalf("expected underlying error to be wrapped, but unwrap returned nil: %v", err)
	}
}

func TestLoadConfig_MissingAPIKeyAllowed(t *testing.T) {
	os.Setenv("WORKER_API_URL", "http://localhost:8080")
	defer os.Unsetenv("WORKER_API_URL")
	os.Unsetenv("WORKER_API_KEY")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected LoadConfig to succeed without WORKER_API_KEY, got error: %v", err)
	}
	if cfg.APIKey != "" {
		t.Fatalf("expected empty APIKey when WORKER_API_KEY not set, got: %s", cfg.APIKey)
	}
}
