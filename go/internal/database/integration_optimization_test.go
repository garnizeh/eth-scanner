package database

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"
)

// Integration-style tests validating retention and bounded storage behaviors

func TestJobsTableBoundedStorage(t *testing.T) {
	ctx := context.Background()
	db, q := setupDBForTests(t)

	prefix := []byte{0x01, 0x02}
	res, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, current_nonce, requested_batch_size) VALUES (?, ?, ?, 'processing', ?, ?, ?)`, prefix, 0, 999, "worker-A", 0, 1000)
	if err != nil {
		t.Fatalf("insert job failed: %v", err)
	}
	jid, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}

	// Record many checkpoints (worker_history rows) referencing the same job
	for i := 0; i < 200; i++ {
		if err := q.RecordWorkerStats(ctx, RecordWorkerStatsParams{
			WorkerID:      "worker-A",
			WorkerType:    sql.NullString{String: "pc", Valid: true},
			JobID:         sql.NullInt64{Int64: jid, Valid: true},
			BatchSize:     sql.NullInt64{Int64: 1000, Valid: true},
			KeysScanned:   sql.NullInt64{Int64: 1000, Valid: true},
			DurationMs:    sql.NullInt64{Int64: 100, Valid: true},
			KeysPerSecond: sql.NullFloat64{Float64: 10.0, Valid: true},
			Prefix28:      prefix,
			NonceStart:    sql.NullInt64{Int64: 0, Valid: true},
			NonceEnd:      sql.NullInt64{Int64: 999, Valid: true},
			FinishedAt:    time.Now().UTC(),
		}); err != nil {
			t.Fatalf("RecordWorkerStats failed: %v", err)
		}
	}

	// Jobs table must remain bounded (1 row)
	var cnt int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs").Scan(&cnt); err != nil {
		t.Fatalf("count jobs failed: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("expected jobs count 1 after many checkpoints, got %d", cnt)
	}
}

func TestWorkerHistoryGlobalRetention_SmallLimit(t *testing.T) {
	// Configure a small global history limit to exercise pruning trigger
	if err := os.Setenv("WORKER_HISTORY_LIMIT", "100"); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	defer os.Unsetenv("WORKER_HISTORY_LIMIT")

	ctx := context.Background()
	db, q := setupDBForTests(t)

	// Insert more rows than the limit and verify trimming
	total := 150
	for i := 0; i < total; i++ {
		if err := q.RecordWorkerStats(ctx, RecordWorkerStatsParams{
			WorkerID:      "worker-glob",
			WorkerType:    sql.NullString{String: "pc", Valid: true},
			JobID:         sql.NullInt64{Valid: false},
			BatchSize:     sql.NullInt64{Int64: 100, Valid: true},
			KeysScanned:   sql.NullInt64{Int64: 100, Valid: true},
			DurationMs:    sql.NullInt64{Int64: 10, Valid: true},
			KeysPerSecond: sql.NullFloat64{Float64: 10.0, Valid: true},
			Prefix28:      []byte{0x01},
			NonceStart:    sql.NullInt64{Int64: int64(i * 100), Valid: true},
			NonceEnd:      sql.NullInt64{Int64: int64(i*100 + 99), Valid: true},
			FinishedAt:    time.Now().UTC(),
		}); err != nil {
			t.Fatalf("RecordWorkerStats failed: %v", err)
		}
	}

	var cnt int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_history").Scan(&cnt); err != nil {
		t.Fatalf("count worker_history failed: %v", err)
	}
	if cnt != 100 {
		t.Fatalf("expected worker_history count 100 after pruning, got %d", cnt)
	}
}

func TestWorkerHistoryGlobalRetention_Load15000(t *testing.T) {
	// This load-style test inserts 15k rows and verifies global cap (default 10000)
	// It may be somewhat heavy but exercises pruning logic.
	ctx := context.Background()
	db, q := setupDBForTests(t)

	total := 15000
	for i := 0; i < total; i++ {
		if err := q.RecordWorkerStats(ctx, RecordWorkerStatsParams{
			WorkerID:      "worker-load",
			WorkerType:    sql.NullString{String: "pc", Valid: true},
			JobID:         sql.NullInt64{Valid: false},
			BatchSize:     sql.NullInt64{Int64: 100, Valid: true},
			KeysScanned:   sql.NullInt64{Int64: 100, Valid: true},
			DurationMs:    sql.NullInt64{Int64: 10, Valid: true},
			KeysPerSecond: sql.NullFloat64{Float64: 10.0, Valid: true},
			Prefix28:      []byte{0x02},
			NonceStart:    sql.NullInt64{Int64: int64(i * 100), Valid: true},
			NonceEnd:      sql.NullInt64{Int64: int64(i*100 + 99), Valid: true},
			FinishedAt:    time.Now().UTC(),
		}); err != nil {
			t.Fatalf("RecordWorkerStats failed at %d: %v", i, err)
		}
	}

	var cnt int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_history").Scan(&cnt); err != nil {
		t.Fatalf("count worker_history failed: %v", err)
	}
	if cnt != 10000 {
		t.Fatalf("expected worker_history count 10000 after load prune, got %d", cnt)
	}
}
