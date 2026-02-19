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
	// adaptive batch sizing defaults
	if cfg.TargetJobDurationSeconds != 3600 {
		t.Fatalf("unexpected TargetJobDurationSeconds default: %d", cfg.TargetJobDurationSeconds)
	}
	if cfg.MinBatchSize != 100000 {
		t.Fatalf("unexpected MinBatchSize default: %d", cfg.MinBatchSize)
	}
	if cfg.MaxBatchSize != 10000000 {
		t.Fatalf("unexpected MaxBatchSize default: %d", cfg.MaxBatchSize)
	}
	if cfg.BatchAdjustAlpha != 0.5 {
		t.Fatalf("unexpected BatchAdjustAlpha default: %f", cfg.BatchAdjustAlpha)
	}
	if cfg.InitialBatchSize != 0 {
		t.Fatalf("unexpected InitialBatchSize default: %d", cfg.InitialBatchSize)
	}
	// internal batch default
	if cfg.InternalBatchSize != 1000000 {
		t.Fatalf("unexpected InternalBatchSize default: %d", cfg.InternalBatchSize)
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
	// adaptive batch sizing defaults are still applied
	if cfg.TargetJobDurationSeconds != 3600 {
		t.Fatalf("unexpected TargetJobDurationSeconds default: %d", cfg.TargetJobDurationSeconds)
	}
}

func TestLoadConfig_AdaptiveEnvOverrides(t *testing.T) {
	os.Setenv("WORKER_API_URL", "http://localhost:8080")
	defer os.Unsetenv("WORKER_API_URL")
	os.Setenv("WORKER_TARGET_JOB_DURATION", "1800")
	defer os.Unsetenv("WORKER_TARGET_JOB_DURATION")
	os.Setenv("WORKER_MIN_BATCH_SIZE", "12345")
	defer os.Unsetenv("WORKER_MIN_BATCH_SIZE")
	os.Setenv("WORKER_MAX_BATCH_SIZE", "54321")
	defer os.Unsetenv("WORKER_MAX_BATCH_SIZE")
	os.Setenv("WORKER_BATCH_ADJUST_ALPHA", "0.25")
	defer os.Unsetenv("WORKER_BATCH_ADJUST_ALPHA")
	os.Setenv("WORKER_INITIAL_BATCH_SIZE", "77777")
	defer os.Unsetenv("WORKER_INITIAL_BATCH_SIZE")
	os.Setenv("WORKER_INTERNAL_BATCH_SIZE", "250000")
	defer os.Unsetenv("WORKER_INTERNAL_BATCH_SIZE")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed with adaptive overrides: %v", err)
	}
	if cfg.TargetJobDurationSeconds != 1800 {
		t.Fatalf("expected TargetJobDurationSeconds 1800, got %d", cfg.TargetJobDurationSeconds)
	}
	if cfg.MinBatchSize != 12345 {
		t.Fatalf("expected MinBatchSize 12345, got %d", cfg.MinBatchSize)
	}
	if cfg.MaxBatchSize != 54321 {
		t.Fatalf("expected MaxBatchSize 54321, got %d", cfg.MaxBatchSize)
	}
	if cfg.BatchAdjustAlpha != 0.25 {
		t.Fatalf("expected BatchAdjustAlpha 0.25, got %f", cfg.BatchAdjustAlpha)
	}
	if cfg.InitialBatchSize != 77777 {
		t.Fatalf("expected InitialBatchSize 77777, got %d", cfg.InitialBatchSize)
	}
	if cfg.InternalBatchSize != 250000 {
		t.Fatalf("expected InternalBatchSize 250000, got %d", cfg.InternalBatchSize)
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

func TestLoadConfig_WorkerNumGoroutines_SetUnsetInvalidZero(t *testing.T) {
	// Base required env
	os.Setenv("WORKER_API_URL", "http://localhost:8080")
	defer os.Unsetenv("WORKER_API_URL")

	// Unset -> default (0) which indicates fallback to runtime.NumCPU()
	os.Unsetenv("WORKER_NUM_GOROUTINES")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.WorkerNumGoroutines != 0 {
		t.Fatalf("expected WorkerNumGoroutines 0 when unset, got %d", cfg.WorkerNumGoroutines)
	}

	// Set to a positive integer
	os.Setenv("WORKER_NUM_GOROUTINES", "4")
	cfg, err = LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed with WORKER_NUM_GOROUTINES set: %v", err)
	}
	if cfg.WorkerNumGoroutines != 4 {
		t.Fatalf("expected WorkerNumGoroutines 4, got %d", cfg.WorkerNumGoroutines)
	}
	os.Unsetenv("WORKER_NUM_GOROUTINES")

	// Invalid value -> should fallback to 0 (and not return error)
	os.Setenv("WORKER_NUM_GOROUTINES", "notanint")
	cfg, err = LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed for invalid WORKER_NUM_GOROUTINES: %v", err)
	}
	if cfg.WorkerNumGoroutines != 0 {
		t.Fatalf("expected WorkerNumGoroutines 0 for invalid value, got %d", cfg.WorkerNumGoroutines)
	}
	os.Unsetenv("WORKER_NUM_GOROUTINES")

	// Zero value explicitly set -> treated as unset/fallback
	os.Setenv("WORKER_NUM_GOROUTINES", "0")
	cfg, err = LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed for WORKER_NUM_GOROUTINES=0: %v", err)
	}
	if cfg.WorkerNumGoroutines != 0 {
		t.Fatalf("expected WorkerNumGoroutines 0 for explicit zero, got %d", cfg.WorkerNumGoroutines)
	}
	os.Unsetenv("WORKER_NUM_GOROUTINES")
}
