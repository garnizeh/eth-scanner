package config

import (
    "testing"
)

func TestLoad_Success(t *testing.T) {
    t.Setenv("MASTER_PORT", "9090")
    t.Setenv("MASTER_DB_PATH", "/tmp/test.db")
    t.Setenv("MASTER_LOG_LEVEL", "DEBUG")

    cfg, err := Load()
    if err != nil {
        t.Fatalf("expected no error, got %v", err)
    }

    if cfg.Port != "9090" {
        t.Fatalf("expected port 9090, got %s", cfg.Port)
    }
    if cfg.DBPath != "/tmp/test.db" {
        t.Fatalf("expected dbpath /tmp/test.db, got %s", cfg.DBPath)
    }
    if cfg.LogLevel != "debug" {
        t.Fatalf("expected loglevel debug, got %s", cfg.LogLevel)
    }
}

func TestLoad_MissingDBPath(t *testing.T) {
    t.Setenv("MASTER_PORT", "")
    t.Setenv("MASTER_DB_PATH", "")
    t.Setenv("MASTER_LOG_LEVEL", "")

    _, err := Load()
    if err == nil {
        t.Fatal("expected error when MASTER_DB_PATH is empty")
    }
}
