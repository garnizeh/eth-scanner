package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("MASTER_DB_PATH", "/tmp/test.db")
	// ensure other envs unset
	t.Setenv("MASTER_PORT", "")
	t.Setenv("MASTER_LOG_LEVEL", "")
	t.Setenv("MASTER_SHUTDOWN_TIMEOUT", "")
	t.Setenv("MASTER_API_KEY", "")
	t.Setenv("MASTER_TARGET_ADDRESS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Fatalf("expected default Port 8080, got %s", cfg.Port)
	}
	if cfg.DBPath != "/tmp/test.db" {
		t.Fatalf("expected DBPath /tmp/test.db, got %s", cfg.DBPath)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("expected default LogLevel info, got %s", cfg.LogLevel)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Fatalf("expected default ShutdownTimeout 30s, got %v", cfg.ShutdownTimeout)
	}
	if cfg.APIKey != "" {
		t.Fatalf("expected empty APIKey, got %s", cfg.APIKey)
	}
	if cfg.TargetAddress != "0x000000000000000000000000000000000000dEaD" {
		t.Fatalf("expected default TargetAddress dead, got %s", cfg.TargetAddress)
	}
	if cfg.StaleJobThresholdSeconds != 604800 {
		t.Fatalf("expected default StaleJobThresholdSeconds 604800, got %d", cfg.StaleJobThresholdSeconds)
	}
	if cfg.CleanupIntervalSeconds != 21600 {
		t.Fatalf("expected default CleanupIntervalSeconds 21600, got %d", cfg.CleanupIntervalSeconds)
	}
}

func TestLoad_CustomEnv(t *testing.T) {
	t.Setenv("MASTER_DB_PATH", "/tmp/custom.db")
	t.Setenv("MASTER_PORT", "9090")
	t.Setenv("MASTER_LOG_LEVEL", "DEBUG")
	t.Setenv("MASTER_SHUTDOWN_TIMEOUT", "1m30s")
	t.Setenv("MASTER_API_KEY", "secret")
	t.Setenv("MASTER_TARGET_ADDRESS", "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.Port != "9090" {
		t.Fatalf("expected Port 9090, got %s", cfg.Port)
	}
	if cfg.DBPath != "/tmp/custom.db" {
		t.Fatalf("expected DBPath /tmp/custom.db, got %s", cfg.DBPath)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("expected LogLevel debug, got %s", cfg.LogLevel)
	}
	if cfg.ShutdownTimeout != time.Minute+30*time.Second {
		t.Fatalf("expected ShutdownTimeout 90s, got %v", cfg.ShutdownTimeout)
	}
	if cfg.APIKey != "secret" {
		t.Fatalf("expected APIKey secret, got %s", cfg.APIKey)
	}
	if cfg.TargetAddress != "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd" {
		t.Fatalf("expected TargetAddress override, got %s", cfg.TargetAddress)
	}
	// Defaults not set in this test; ensure parsing does not error when unset
	if cfg.StaleJobThresholdSeconds != 604800 {
		t.Fatalf("expected default StaleJobThresholdSeconds when unset, got %d", cfg.StaleJobThresholdSeconds)
	}
}

func TestLoad_InvalidShutdownTimeout(t *testing.T) {
	t.Setenv("MASTER_DB_PATH", "/tmp/test.db")
	t.Setenv("MASTER_SHUTDOWN_TIMEOUT", "notaduration")
	if _, err := Load(); err == nil {
		t.Fatalf("expected error for invalid MASTER_SHUTDOWN_TIMEOUT, got nil")
	}
}

func TestLoad_CustomCleanupEnv(t *testing.T) {
	t.Setenv("MASTER_DB_PATH", "/tmp/test.db")
	t.Setenv("MASTER_STALE_JOB_THRESHOLD", "3600")
	t.Setenv("MASTER_CLEANUP_INTERVAL", "1200")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.StaleJobThresholdSeconds != 3600 {
		t.Fatalf("expected StaleJobThresholdSeconds 3600, got %d", cfg.StaleJobThresholdSeconds)
	}
	if cfg.CleanupIntervalSeconds != 1200 {
		t.Fatalf("expected CleanupIntervalSeconds 1200, got %d", cfg.CleanupIntervalSeconds)
	}
}

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
	if cfg.StaleJobThresholdSeconds != 604800 {
		t.Fatalf("expected default stale threshold when unset, got %d", cfg.StaleJobThresholdSeconds)
	}
	if cfg.CleanupIntervalSeconds != 21600 {
		t.Fatalf("expected default cleanup interval when unset, got %d", cfg.CleanupIntervalSeconds)
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

func TestLoad_InvalidStaleJobThreshold(t *testing.T) {
	t.Parallel()

	// preserve env
	origDB := os.Getenv("MASTER_DB_PATH")
	origStale := os.Getenv("MASTER_STALE_JOB_THRESHOLD")
	defer func() {
		_ = os.Setenv("MASTER_DB_PATH", origDB)
		_ = os.Setenv("MASTER_STALE_JOB_THRESHOLD", origStale)
	}()

	// ensure DBPath is set so Load progresses to parsing the threshold
	_ = os.Setenv("MASTER_DB_PATH", "dummy")
	_ = os.Setenv("MASTER_STALE_JOB_THRESHOLD", "not-an-int")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for invalid MASTER_STALE_JOB_THRESHOLD, got nil")
	}
	if !strings.Contains(err.Error(), "MASTER_STALE_JOB_THRESHOLD") {
		t.Fatalf("error does not contain expected substring; got: %v", err)
	}
}

func TestLoad_InvalidCleanupInterval(t *testing.T) {
	t.Parallel()

	// preserve env
	origDB := os.Getenv("MASTER_DB_PATH")
	origCleanup := os.Getenv("MASTER_CLEANUP_INTERVAL")
	defer func() {
		_ = os.Setenv("MASTER_DB_PATH", origDB)
		_ = os.Setenv("MASTER_CLEANUP_INTERVAL", origCleanup)
	}()

	// ensure DBPath is set so Load progresses to parsing the cleanup interval
	_ = os.Setenv("MASTER_DB_PATH", "dummy")
	_ = os.Setenv("MASTER_CLEANUP_INTERVAL", "not-an-int")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for invalid MASTER_CLEANUP_INTERVAL, got nil")
	}
	if !strings.Contains(err.Error(), "MASTER_CLEANUP_INTERVAL") {
		t.Fatalf("error does not contain expected substring; got: %v", err)
	}
}
