package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

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
		// File-based database with optimizations for single-writer API usage
		dsn = fmt.Sprintf("file:%s?mode=rwc&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(30000)&_pragma=temp_store(MEMORY)&_pragma=mmap_size(268435456)&_pragma=cache_size(-64000)", dbPath)
	}

	// Open connection with modernc.org/sqlite
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool for single-writer SQLite
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Apply schema migrations
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to apply database schema: %w", err)
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

	// Set goose to use the embedded filesystem for migrations
	goose.SetBaseFS(subFS)

	// Set dialect for SQLite
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	// Run all up migrations
	if err := goose.UpContext(ctx, db, "."); err != nil {
		return fmt.Errorf("failed to apply schema migrations: %w", err)
	}

	return nil
}
