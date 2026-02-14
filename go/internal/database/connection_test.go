package database

import (
	"testing"
)

func TestInitDB(t *testing.T) {
	// Test with in-memory database (no files created)
	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// Test ping
	if err := db.Ping(); err != nil {
		t.Errorf("Database ping failed: %v", err)
	}

	// Test NewQueries
	queries := NewQueries(db)
	if queries == nil {
		t.Error("NewQueries returned nil")
	}
}

func TestCloseDB(t *testing.T) {
	// Create temporary database file in test temp directory
	tempDir := t.TempDir()
	dbPath := tempDir + "/test_close.db"

	db, err := InitDB(dbPath)
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
