// Package database provides helpers to initialize and manage the SQLite
// database connection and run embedded migrations.
package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/garnizeh/eth-scanner/internal/config"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

//go:embed sql/0*.sql
var migrations embed.FS

// InitDB initializes a SQLite database connection
// Returns *sql.DB ready for use with sqlc queries
// Supports both file-based and in-memory databases (:memory:)
func InitDB(ctx context.Context, dbPath string) (*sql.DB, error) {
	var dsn string

	if dbPath == ":memory:" {
		// In-memory database - no file operations needed
		dsn = ":memory:?_pragma=foreign_keys(ON)&_pragma=temp_store(MEMORY)&_pragma=cache_size(-64000)"
	} else {
		// File-based database with optimizations for API usage
		dsn = fmt.Sprintf(
			"file:%s?mode=rwc"+
				"&_pragma=journal_mode(WAL)"+
				"&_pragma=synchronous(NORMAL)"+
				"&_pragma=busy_timeout(10000)"+
				"&_pragma=journal_size_limit(67108864)"+
				"&_pragma=mmap_size(536870912)"+
				"&_pragma=cache_size(-64000)"+
				"&_pragma=foreign_keys(ON)",
			dbPath,
		)
	}

	// Open connection with modernc.org/sqlite
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool to deal with concurrent access patterns (single writer, multiple readers)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		if cerr := db.Close(); cerr != nil {
			return nil, fmt.Errorf("failed to ping database: %w", errors.Join(err, cerr))
		}
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Apply schema migrations
	if err := migrate(ctx, db); err != nil {
		if cerr := db.Close(); cerr != nil {
			return nil, fmt.Errorf("failed to apply database schema: %w", errors.Join(err, cerr))
		}
		return nil, fmt.Errorf("failed to apply database schema: %w", err)
	}

	// Create retention triggers based on configured limits (reads env vars)
	hist, daily, monthly := config.GetRetentionLimits()
	if err := createRetentionTriggers(ctx, db, hist, daily, monthly); err != nil {
		if cerr := db.Close(); cerr != nil {
			return nil, fmt.Errorf("failed to create retention triggers: %w", errors.Join(err, cerr))
		}
		return nil, fmt.Errorf("failed to create retention triggers: %w", err)
	}

	return db, nil
}

// NewQueries creates a Queries instance from database connection
func NewQueries(db *sql.DB) *Queries {
	return New(db)
}

// CloseDB closes the database connection
func CloseDB(db *sql.DB) error {
	if db != nil {
		if err := db.Close(); err != nil {
			return fmt.Errorf("failed to close database: %w", err)
		}
	}
	return nil
}

// ApplySchema applies the database schema using goose migrations
// Safe to run multiple times (idempotent via goose version tracking)
func migrate(ctx context.Context, db *sql.DB) error {
	// Create a sub filesystem for the sql directory
	subFS, err := fs.Sub(migrations, "sql")
	if err != nil {
		return fmt.Errorf("failed to create sub filesystem: %w", err)
	}

	// Use goose.NewProvider to avoid global state race conditions (SetBaseFS/SetDialect)
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, subFS)
	if err != nil {
		return fmt.Errorf("failed to create goose provider: %w", err)
	}

	// Run all up migrations
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("failed to apply schema migrations: %w", err)
	}

	return nil
}

// createRetentionTriggers creates or recreates SQLite triggers that prune
// worker history and per-worker daily/monthly stats according to configured limits.
func createRetentionTriggers(ctx context.Context, db *sql.DB, historyLimit, dailyLimit, monthlyLimit int) error {
	// Drop both the original schema trigger names (prefixed with trg_) and
	// any previously created runtime triggers to ensure a single set of
	// retention triggers exist with the configured limits.
	dropTriggers := []string{
		"DROP TRIGGER IF EXISTS prune_worker_history",
		"DROP TRIGGER IF EXISTS prune_daily_stats_per_worker",
		"DROP TRIGGER IF EXISTS prune_monthly_stats_per_worker",
		"DROP TRIGGER IF EXISTS trg_prune_daily_stats_per_worker",
		"DROP TRIGGER IF EXISTS trg_prune_monthly_stats_per_worker",
	}
	for _, stmt := range dropTriggers {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("drop trigger: %w", err)
		}
	}

	historyTrigger := fmt.Sprintf(`
	CREATE TRIGGER prune_worker_history
	AFTER INSERT ON worker_history
	WHEN (SELECT COUNT(*) FROM worker_history) > %d
	BEGIN
		DELETE FROM worker_history
		WHERE id IN (
			SELECT id FROM worker_history
			ORDER BY id ASC
			LIMIT (SELECT COUNT(*) - %d FROM worker_history)
		);
	END;
	`, historyLimit, historyLimit)

	dailyTrigger := fmt.Sprintf(`
	CREATE TRIGGER prune_daily_stats_per_worker
	AFTER INSERT ON worker_stats_daily
	FOR EACH ROW
	WHEN (SELECT COUNT(*) FROM worker_stats_daily WHERE worker_id = NEW.worker_id) > %d
	BEGIN
		DELETE FROM worker_stats_daily
		WHERE worker_id = NEW.worker_id
		AND id IN (
			SELECT id FROM worker_stats_daily
			WHERE worker_id = NEW.worker_id
			ORDER BY stats_date ASC
			LIMIT (SELECT COUNT(*) - %d FROM worker_stats_daily WHERE worker_id = NEW.worker_id)
		);
	END;
	`, dailyLimit, dailyLimit)

	monthlyTrigger := fmt.Sprintf(`
	CREATE TRIGGER prune_monthly_stats_per_worker
	AFTER INSERT ON worker_stats_monthly
	FOR EACH ROW
	WHEN (SELECT COUNT(*) FROM worker_stats_monthly WHERE worker_id = NEW.worker_id) > %d
	BEGIN
		DELETE FROM worker_stats_monthly
		WHERE worker_id = NEW.worker_id
		AND id IN (
			SELECT id FROM worker_stats_monthly
			WHERE worker_id = NEW.worker_id
			ORDER BY stats_month ASC
			LIMIT (SELECT COUNT(*) - %d FROM worker_stats_monthly WHERE worker_id = NEW.worker_id)
		);
	END;
	`, monthlyLimit, monthlyLimit)

	triggers := []string{historyTrigger, dailyTrigger, monthlyTrigger}
	for _, t := range triggers {
		if _, err := db.ExecContext(ctx, t); err != nil {
			return fmt.Errorf("create trigger: %w", err)
		}
	}

	return nil
}
