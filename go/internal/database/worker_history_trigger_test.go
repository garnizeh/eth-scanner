package database

import (
	"context"
	"database/sql"
	"testing"
)

func TestWorkerHistoryTriggerIncrementsWorkerTotal(t *testing.T) {
	ctx := context.Background()
	db, err := InitDB(ctx, ":memory:")
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	q := NewQueries(db)

	// Ensure worker exists
	workerID := "worker-trigger-test"
	if err := q.UpsertWorker(ctx, UpsertWorkerParams{ID: workerID, WorkerType: "pc", Metadata: sql.NullString{Valid: false}}); err != nil {
		t.Fatalf("UpsertWorker failed: %v", err)
	}

	// Record a worker_history row with keys_scanned = 42
	if err := q.RecordWorkerStats(ctx, RecordWorkerStatsParams{
		WorkerID:      workerID,
		WorkerType:    sql.NullString{String: "pc", Valid: true},
		JobID:         sql.NullInt64{Valid: false},
		BatchSize:     sql.NullInt64{Int64: 0, Valid: false},
		KeysScanned:   sql.NullInt64{Int64: 42, Valid: true},
		DurationMs:    sql.NullInt64{Int64: 0, Valid: true},
		KeysPerSecond: sql.NullFloat64{Valid: false},
		Prefix28:      nil,
		NonceStart:    sql.NullInt64{Valid: false},
		NonceEnd:      sql.NullInt64{Valid: false},
	}); err != nil {
		t.Fatalf("RecordWorkerStats failed: %v", err)
	}

	// Query worker and confirm total_keys_scanned updated
	w, err := q.GetWorkerByID(ctx, workerID)
	if err != nil {
		t.Fatalf("GetWorkerByID failed: %v", err)
	}
	if !w.TotalKeysScanned.Valid || w.TotalKeysScanned.Int64 != 42 {
		t.Fatalf("expected total_keys_scanned 42, got %v", w.TotalKeysScanned)
	}
}
