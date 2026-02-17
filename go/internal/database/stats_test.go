package database

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func setupDBForTests(t *testing.T) (*sql.DB, *Queries) {
	t.Helper()
	ctx := context.Background()
	db, err := InitDB(ctx, ":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	q := NewQueries(db)
	t.Cleanup(func() {
		if err := CloseDB(db); err != nil {
			t.Fatalf("CloseDB: %v", err)
		}
	})
	return db, q
}

func TestRecordWorkerHistoryAndAggregation(t *testing.T) {
	ctx := context.Background()
	db, q := setupDBForTests(t)

	workerID := "worker-agg-1"
	finished := time.Now().UTC()

	// Record a single worker history row
	err := q.RecordWorkerStats(ctx, RecordWorkerStatsParams{
		WorkerID:      workerID,
		WorkerType:    sql.NullString{String: "pc", Valid: true},
		JobID:         sql.NullInt64{Valid: false},
		BatchSize:     sql.NullInt64{Int64: 1000, Valid: true},
		KeysScanned:   sql.NullInt64{Int64: 1000, Valid: true},
		DurationMs:    sql.NullInt64{Int64: 100, Valid: true},
		KeysPerSecond: sql.NullFloat64{Float64: 10.0, Valid: true},
		Prefix28:      []byte{0x01, 0x02},
		NonceStart:    sql.NullInt64{Int64: 0, Valid: true},
		NonceEnd:      sql.NullInt64{Int64: 999, Valid: true},
		FinishedAt:    finished,
		ErrorMessage:  sql.NullString{Valid: false},
	})
	if err != nil {
		t.Fatalf("RecordWorkerStats error: %v", err)
	}

	// Ensure the history row exists and obtain its id
	var id int64
	row := db.QueryRowContext(ctx, "SELECT id FROM worker_history WHERE worker_id = ? ORDER BY id DESC LIMIT 1", workerID)
	if err := row.Scan(&id); err != nil {
		t.Fatalf("fetch history id: %v", err)
	}

	// Inspect stored finished_at to help debug trigger behavior
	var storedFinished sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT finished_at FROM worker_history WHERE id = ?", id).Scan(&storedFinished); err != nil {
		t.Fatalf("fetch finished_at: %v", err)
	}
	if !storedFinished.Valid {
		t.Fatalf("stored finished_at is NULL for id %d", id)
	}
	t.Logf("stored finished_at for id %d = %s", id, storedFinished.String)

	// Delete the history row to trigger aggregation (BEFORE DELETE trigger)
	if _, err := db.ExecContext(ctx, "DELETE FROM worker_history WHERE id = ?", id); err != nil {
		t.Fatalf("delete history to trigger aggregate: %v", err)
	}
	// Debug: count rows in aggregates
	var cntDaily, cntMonthly, cntLifetime int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_stats_daily WHERE worker_id = ?", workerID).Scan(&cntDaily); err != nil {
		t.Fatalf("count daily query error: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_stats_monthly WHERE worker_id = ?", workerID).Scan(&cntMonthly); err != nil {
		t.Fatalf("count monthly query error: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_stats_lifetime WHERE worker_id = ?", workerID).Scan(&cntLifetime); err != nil {
		t.Fatalf("count lifetime query error: %v", err)
	}
	t.Logf("after delete: daily=%d monthly=%d lifetime=%d", cntDaily, cntMonthly, cntLifetime)

	// Verify daily aggregate created
	// Query using date-only (truncate to midnight UTC) because stats_date stores YYYY-MM-DD
	statsDate := time.Date(finished.Year(), finished.Month(), finished.Day(), 0, 0, 0, 0, time.UTC)
	// Verify daily aggregate created by querying the table directly (string date comparison)
	statsDateStr := statsDate.Format("2006-01-02")
	var totalBatches sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT total_batches FROM worker_stats_daily WHERE worker_id = ? AND stats_date = ?", workerID, statsDateStr).Scan(&totalBatches); err != nil {
		t.Fatalf("select daily aggregate error: %v; counts: daily=%d monthly=%d lifetime=%d", err, cntDaily, cntMonthly, cntLifetime)
	}
	if !totalBatches.Valid || totalBatches.Int64 < 1 {
		t.Fatalf("expected total_batches >= 1, got %+v", totalBatches)
	}

	// Verify lifetime stats updated
	lifetime, err := q.GetWorkerLifetimeStats(ctx, workerID)
	if err != nil {
		t.Fatalf("GetWorkerLifetimeStats error: %v", err)
	}
	if !lifetime.TotalBatches.Valid || lifetime.TotalBatches.Int64 < 1 {
		t.Fatalf("expected lifetime.total_batches >= 1, got %+v", lifetime.TotalBatches)
	}

	_ = db // silence unused in some contexts
}

func TestPruneDailyStatsPerWorker(t *testing.T) {
	ctx := context.Background()
	db, _ := setupDBForTests(t)

	workerA := "worker-prune-A"
	workerB := "worker-prune-B"

	// Insert 1001 daily rows for workerA
	for i := 0; i < 1001; i++ {
		d := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		if _, err := db.ExecContext(ctx, "INSERT INTO worker_stats_daily (worker_id, stats_date, total_batches) VALUES (?, ?, 1)", workerA, d); err != nil {
			t.Fatalf("insert daily row workerA: %v", err)
		}
	}

	// Insert 10 daily rows for workerB
	for i := 0; i < 10; i++ {
		d := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		if _, err := db.ExecContext(ctx, "INSERT INTO worker_stats_daily (worker_id, stats_date, total_batches) VALUES (?, ?, 1)", workerB, d); err != nil {
			t.Fatalf("insert daily row workerB: %v", err)
		}
	}

	// Verify counts: workerA should be trimmed to 1000, workerB remains 10
	var countA, countB int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_stats_daily WHERE worker_id = ?", workerA).Scan(&countA); err != nil {
		t.Fatalf("countA query error: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_stats_daily WHERE worker_id = ?", workerB).Scan(&countB); err != nil {
		t.Fatalf("countB query error: %v", err)
	}

	if countA != 1000 {
		t.Fatalf("expected workerA count 1000 after prune, got %d", countA)
	}
	if countB != 10 {
		t.Fatalf("expected workerB count 10, got %d", countB)
	}
}

func TestPruneMonthlyStatsPerWorker(t *testing.T) {
	ctx := context.Background()
	db, _ := setupDBForTests(t)

	worker := "worker-monthly-prune"

	// Insert 1001 monthly rows (different months) for the worker
	for i := 0; i < 1001; i++ {
		// Use month offsets
		dt := time.Now().UTC().AddDate(0, -i, 0)
		m := dt.Format("2006-01")
		if _, err := db.ExecContext(ctx, "INSERT INTO worker_stats_monthly (worker_id, stats_month, total_batches) VALUES (?, ?, 1)", worker, m); err != nil {
			t.Fatalf("insert monthly row: %v", err)
		}
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_stats_monthly WHERE worker_id = ?", worker).Scan(&count); err != nil {
		t.Fatalf("count monthly query error: %v", err)
	}
	if count != 1000 {
		t.Fatalf("expected monthly count 1000 after prune, got %d", count)
	}
}
