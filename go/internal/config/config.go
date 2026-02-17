// Package config provides configuration loading and validation for the
// Master API and worker components.
package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	// Port is the TCP port the server listens on (e.g. "8080").
	Port string

	// DBPath is the filesystem path to the SQLite database file.
	DBPath string

	// LogLevel controls application logging: debug, info, warn, error.
	LogLevel string

	// ShutdownTimeout is the default timeout for graceful shutdown (e.g. "30s").
	ShutdownTimeout time.Duration

	// APIKey is the secret API key required for requests when set. If empty,
	// API key enforcement is disabled (useful for local testing).
	APIKey string

	// TargetAddress is the Ethereum address that workers should search for.
	// Defaults to 0x000000000000000000000000000000000000dEaD if not specified.
	TargetAddress string

	// StaleJobThresholdSeconds is the age in seconds after which a processing
	// job with no recent checkpoints is considered abandoned and eligible for
	// cleanup. Default: 7 days (604800 seconds).
	StaleJobThresholdSeconds int64

	// CleanupIntervalSeconds controls how often the master runs the cleanup
	// background task (default: 6 hours = 21600 seconds).
	CleanupIntervalSeconds int64

	// WorkerHistoryLimit is the global cap for raw history rows (worker_history)
	WorkerHistoryLimit int

	// WorkerDailyStatsLimit is the per-worker cap for daily aggregation
	WorkerDailyStatsLimit int

	// WorkerMonthlyStatsLimit is the per-worker cap for monthly aggregation
	WorkerMonthlyStatsLimit int
}

// Load reads configuration from environment variables, applies defaults and
// validates required values. It returns a configured Config or an error.
func Load() (*Config, error) {
	cfg := &Config{
		Port:     strings.TrimSpace(os.Getenv("MASTER_PORT")),
		DBPath:   strings.TrimSpace(os.Getenv("MASTER_DB_PATH")),
		LogLevel: strings.TrimSpace(os.Getenv("MASTER_LOG_LEVEL")),
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	} else {
		// normalize
		cfg.LogLevel = strings.ToLower(cfg.LogLevel)
	}

	// Shutdown timeout (defaults to 30s)
	st := strings.TrimSpace(os.Getenv("MASTER_SHUTDOWN_TIMEOUT"))
	if st == "" {
		cfg.ShutdownTimeout = 30 * time.Second
	} else {
		d, err := time.ParseDuration(st)
		if err != nil {
			return nil, fmt.Errorf("invalid MASTER_SHUTDOWN_TIMEOUT: %w", err)
		}
		cfg.ShutdownTimeout = d
	}

	// Validate DBPath is present
	if cfg.DBPath == "" {
		return nil, fmt.Errorf("MASTER_DB_PATH is required")
	}

	// Load API key if present.
	if k := strings.TrimSpace(os.Getenv("MASTER_API_KEY")); k != "" {
		cfg.APIKey = k
	}

	cfg.TargetAddress = strings.TrimSpace(os.Getenv("MASTER_TARGET_ADDRESS"))
	if cfg.TargetAddress == "" {
		cfg.TargetAddress = "0x000000000000000000000000000000000000dEaD"
	}

	// Stale job cleanup settings
	if v := strings.TrimSpace(os.Getenv("MASTER_STALE_JOB_THRESHOLD")); v == "" {
		cfg.StaleJobThresholdSeconds = 604800 // 7 days
	} else {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid MASTER_STALE_JOB_THRESHOLD: %w", err)
		}
		cfg.StaleJobThresholdSeconds = n
	}

	if v := strings.TrimSpace(os.Getenv("MASTER_CLEANUP_INTERVAL")); v == "" {
		cfg.CleanupIntervalSeconds = 21600 // 6 hours
	} else {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid MASTER_CLEANUP_INTERVAL: %w", err)
		}
		cfg.CleanupIntervalSeconds = n
	}

	// Retention limits for worker statistics (can be set independently)
	// Defaults: 10000, 1000, 1000
	if v := strings.TrimSpace(os.Getenv("WORKER_HISTORY_LIMIT")); v == "" {
		cfg.WorkerHistoryLimit = 10000
	} else {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_HISTORY_LIMIT: %w", err)
		}
		cfg.WorkerHistoryLimit = n
	}

	if v := strings.TrimSpace(os.Getenv("WORKER_DAILY_STATS_LIMIT")); v == "" {
		cfg.WorkerDailyStatsLimit = 1000
	} else {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_DAILY_STATS_LIMIT: %w", err)
		}
		cfg.WorkerDailyStatsLimit = n
	}

	if v := strings.TrimSpace(os.Getenv("WORKER_MONTHLY_STATS_LIMIT")); v == "" {
		cfg.WorkerMonthlyStatsLimit = 1000
	} else {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_MONTHLY_STATS_LIMIT: %w", err)
		}
		cfg.WorkerMonthlyStatsLimit = n
	}

	// Validate retention values and warn for low sizes
	if cfg.WorkerHistoryLimit <= 0 {
		log.Printf("WARNING: WORKER_HISTORY_LIMIT must be > 0, using default 10000")
		cfg.WorkerHistoryLimit = 10000
	}
	if cfg.WorkerHistoryLimit < 100 {
		log.Printf("WARNING: WORKER_HISTORY_LIMIT is very low (%d), may lose recent history quickly", cfg.WorkerHistoryLimit)
	}
	if cfg.WorkerDailyStatsLimit <= 0 {
		log.Printf("WARNING: WORKER_DAILY_STATS_LIMIT must be > 0, using default 1000")
		cfg.WorkerDailyStatsLimit = 1000
	}
	if cfg.WorkerMonthlyStatsLimit <= 0 {
		log.Printf("WARNING: WORKER_MONTHLY_STATS_LIMIT must be > 0, using default 1000")
		cfg.WorkerMonthlyStatsLimit = 1000
	}

	return cfg, nil
}

// GetRetentionLimits reads only the worker retention related environment
// variables and returns concrete values with defaults. This helper avoids
// requiring a full Config load when callers only need retention limits.
func GetRetentionLimits() (history int, daily int, monthly int) {
	// Defaults
	history = 10000
	daily = 1000
	monthly = 1000

	if v := strings.TrimSpace(os.Getenv("WORKER_HISTORY_LIMIT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			history = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("WORKER_DAILY_STATS_LIMIT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			daily = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("WORKER_MONTHLY_STATS_LIMIT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			monthly = n
		}
	}

	// Basic validation and warnings
	if history <= 0 {
		log.Printf("WARNING: WORKER_HISTORY_LIMIT must be > 0, using default 10000")
		history = 10000
	}
	if history < 100 {
		log.Printf("WARNING: WORKER_HISTORY_LIMIT is very low (%d), may lose recent history quickly", history)
	}
	if daily <= 0 {
		log.Printf("WARNING: WORKER_DAILY_STATS_LIMIT must be > 0, using default 1000")
		daily = 1000
	}
	if monthly <= 0 {
		log.Printf("WARNING: WORKER_MONTHLY_STATS_LIMIT must be > 0, using default 1000")
		monthly = 1000
	}

	return history, daily, monthly
}
