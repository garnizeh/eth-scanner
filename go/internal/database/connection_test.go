package database

import (
	"context"
	"os"
	"testing"
)

func TestInitDB(t *testing.T) {
	// Test with in-memory database (no files created)
	db, err := InitDB(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("db.Close failed: %v", err)
		}
	}()

	// Test ping
	if err := db.PingContext(t.Context()); err != nil {
		t.Errorf("Database ping failed: %v", err)
	}

	// Test NewQueries
	queries := NewQueries(db)
	if queries == nil {
		t.Error("NewQueries returned nil")
	}

	// Test a simple query to verify schema is applied
	_, err = queries.GetStats(t.Context())
	if err != nil {
		t.Errorf("Failed to execute GetStats query: %v", err)
	}
}

func TestCloseDB(t *testing.T) {
	// Test with in-memory database (no files created)
	db, err := InitDB(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	// Test CloseDB
	err = CloseDB(db)
	if err != nil {
		t.Errorf("CloseDB failed: %v", err)
	}

	// Test closing already closed connection
	err = CloseDB(db)
	if err != nil {
		t.Errorf("CloseDB on already closed connection failed: %v", err)
	}

	// Test closing nil connection
	err = CloseDB(nil)
	if err != nil {
		t.Errorf("CloseDB on nil connection failed: %v", err)
	}
}

func TestInitDBWithSchema(t *testing.T) {
	ctx := context.Background()
	dbPath := "test_schema.db"
	defer func() {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to remove db file: %v", err)
		}
	}()

	db, err := InitDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("db.Close failed: %v", err)
		}
	}()

	// Test NewQueries
	queries := NewQueries(db)
	if queries == nil {
		t.Error("NewQueries returned nil")
	}

	// Test a simple query to verify schema is applied
	_, err = queries.GetStats(ctx)
	if err != nil {
		t.Errorf("Failed to execute GetStats query: %v", err)
	}
}
