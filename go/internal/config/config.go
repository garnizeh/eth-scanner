// Package config provides configuration loading and validation for the
// Master API and worker components.
package config

import (
	"fmt"
	"os"
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

	return cfg, nil
}
