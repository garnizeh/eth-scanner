package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Measure DB file size after heavy inserts and ensure it stays bounded (example threshold)
func TestDatabaseFileSizeAfterLoad(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "load.db")

	db, err := InitDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	q := NewQueries(db)
	t.Cleanup(func() { _ = CloseDB(db) })

	// Insert many history records (more than retention limit)
	// Use a transaction for bulk inserts to speed up the test and avoid timeouts
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()
	qtx := q.WithTx(tx)

	total := 20000
	for i := range total {
		if err := qtx.RecordWorkerStats(ctx, RecordWorkerStatsParams{
			WorkerID:      "size-worker",
			WorkerType:    sqlNullString("pc"),
			JobID:         sql.NullInt64{Valid: false},
			BatchSize:     sqlNullInt64(100),
			KeysScanned:   sqlNullInt64(100),
			DurationMs:    sqlNullInt64(10),
			KeysPerSecond: sqlNullFloat64(10.0),
			Prefix28:      []byte{0x01},
			NonceStart:    sqlNullInt64(int64(i * 100)),
			NonceEnd:      sqlNullInt64(int64(i*100 + 99)),
			FinishedAt:    time.Now().UTC(),
		}); err != nil {
			t.Fatalf("RecordWorkerStats failed at %d: %v", i, err)
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	// Give SQLite a moment to flush
	time.Sleep(200 * time.Millisecond)
	fi, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat db file failed: %v", err)
	}
	// Assert file size under 20MB as a sanity bound for this test
	if fi.Size() > 20*1024*1024 {
		t.Fatalf("database file too large: %d bytes", fi.Size())
	}
}

// helpers for nullable types
func sqlNullString(s string) sql.NullString    { return sql.NullString{String: s, Valid: true} }
func sqlNullInt64(v int64) sql.NullInt64       { return sql.NullInt64{Int64: v, Valid: true} }
func sqlNullFloat64(f float64) sql.NullFloat64 { return sql.NullFloat64{Float64: f, Valid: true} }
