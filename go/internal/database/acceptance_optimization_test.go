package database

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"
)

// Per-worker daily retention load test (1500 -> trimmed to 1000)
func TestPerWorkerDailyRetention_Load(t *testing.T) {
	if err := os.Setenv("WORKER_DAILY_STATS_LIMIT", "1000"); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	defer os.Unsetenv("WORKER_DAILY_STATS_LIMIT")

	ctx := context.Background()
	db, _ := setupDBForTests(t)

	workerA := "worker-load-A"
	workerB := "worker-load-B"

	// Insert 1500 daily rows for workerA and 800 for workerB
	for i := range 1500 {
		d := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		if _, err := db.ExecContext(ctx, "INSERT INTO worker_stats_daily (worker_id, stats_date, total_batches) VALUES (?, ?, 1)", workerA, d); err != nil {
			t.Fatalf("insert daily workerA: %v", err)
		}
	}
	for i := range 800 {
		d := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		if _, err := db.ExecContext(ctx, "INSERT INTO worker_stats_daily (worker_id, stats_date, total_batches) VALUES (?, ?, 1)", workerB, d); err != nil {
			t.Fatalf("insert daily workerB: %v", err)
		}
	}

	var countA, countB int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_stats_daily WHERE worker_id = ?", workerA).Scan(&countA); err != nil {
		t.Fatalf("countA query error: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_stats_daily WHERE worker_id = ?", workerB).Scan(&countB); err != nil {
		t.Fatalf("countB query error: %v", err)
	}

	if countA != 1000 {
		t.Fatalf("expected workerA daily count 1000 after prune, got %d", countA)
	}
	if countB != 800 {
		t.Fatalf("expected workerB daily count 800, got %d", countB)
	}
}

// Per-worker monthly retention load test (1500 -> trimmed to 1000)
func TestPerWorkerMonthlyRetention_Load(t *testing.T) {
	if err := os.Setenv("WORKER_MONTHLY_STATS_LIMIT", "1000"); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	defer os.Unsetenv("WORKER_MONTHLY_STATS_LIMIT")

	ctx := context.Background()
	db, _ := setupDBForTests(t)

	worker := "worker-month-load"

	// Insert 1500 monthly rows (unique months)
	for i := range 1500 {
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

// Automatic aggregation: insert many history rows for same date and verify
// daily and monthly aggregates accumulate correctly after deleting history
func TestAutomaticAggregationFromHistory(t *testing.T) {
	ctx := context.Background()
	db, q := setupDBForTests(t)

	workerID := "worker-agg-2"
	finished := time.Now().UTC()

	// Insert 100 history rows
	for i := range 100 {
		if err := q.RecordWorkerStats(ctx, RecordWorkerStatsParams{
			WorkerID:      workerID,
			WorkerType:    sql.NullString{String: "pc", Valid: true},
			JobID:         sql.NullInt64{Valid: false},
			BatchSize:     sql.NullInt64{Int64: 100, Valid: true},
			KeysScanned:   sql.NullInt64{Int64: 100, Valid: true},
			DurationMs:    sql.NullInt64{Int64: 10, Valid: true},
			KeysPerSecond: sql.NullFloat64{Float64: 10.0, Valid: true},
			Prefix28:      []byte{0x09},
			NonceStart:    sql.NullInt64{Int64: int64(i * 100), Valid: true},
			NonceEnd:      sql.NullInt64{Int64: int64(i*100 + 99), Valid: true},
			FinishedAt:    finished,
		}); err != nil {
			t.Fatalf("RecordWorkerStats failed: %v", err)
		}
	}

	// Delete all history rows for worker to force aggregation via BEFORE DELETE trigger
	if _, err := db.ExecContext(ctx, "DELETE FROM worker_history WHERE worker_id = ?", workerID); err != nil {
		t.Fatalf("delete history failed: %v", err)
	}

	// Verify daily aggregate contains 100 batches
	statsDateStr := finished.Format("2006-01-02")
	var totalBatches sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT total_batches FROM worker_stats_daily WHERE worker_id = ? AND stats_date = ?", workerID, statsDateStr).Scan(&totalBatches); err != nil {
		t.Fatalf("select daily aggregate error: %v", err)
	}
	if !totalBatches.Valid || totalBatches.Int64 != 100 {
		t.Fatalf("expected daily total_batches 100, got %+v", totalBatches)
	}

	// Verify monthly aggregate has at least 100
	monthStr := finished.Format("2006-01")
	var monthlyBatches sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT total_batches FROM worker_stats_monthly WHERE worker_id = ? AND stats_month = ?", workerID, monthStr).Scan(&monthlyBatches); err != nil {
		t.Fatalf("select monthly aggregate error: %v", err)
	}
	if !monthlyBatches.Valid || monthlyBatches.Int64 < 100 {
		t.Fatalf("expected monthly total_batches >= 100, got %+v", monthlyBatches)
	}

	// Verify lifetime stats updated
	lifetime, err := q.GetWorkerLifetimeStats(ctx, workerID)
	if err != nil {
		t.Fatalf("GetWorkerLifetimeStats error: %v", err)
	}
	if lifetime.TotalBatches < 100 {
		t.Fatalf("expected lifetime.total_batches >= 100, got %d", lifetime.TotalBatches)
	}
}

// Benchmark: record worker stats throughput
func BenchmarkRecordWorkerStats(b *testing.B) {
	ctx := context.Background()
	_, q := setupDBForBench(b)

	for i := 0; b.Loop(); i++ {
		_ = q.RecordWorkerStats(ctx, RecordWorkerStatsParams{
			WorkerID:      "bench-worker",
			WorkerType:    sql.NullString{String: "pc", Valid: true},
			JobID:         sql.NullInt64{Valid: false},
			BatchSize:     sql.NullInt64{Int64: 100, Valid: true},
			KeysScanned:   sql.NullInt64{Int64: 100, Valid: true},
			DurationMs:    sql.NullInt64{Int64: 10, Valid: true},
			KeysPerSecond: sql.NullFloat64{Float64: 10.0, Valid: true},
			Prefix28:      []byte{0x0a},
			NonceStart:    sql.NullInt64{Int64: int64(i * 100), Valid: true},
			NonceEnd:      sql.NullInt64{Int64: int64(i*100 + 99), Valid: true},
			FinishedAt:    time.Now().UTC(),
		})
	}
}

// helper for benchmark: uses setup but returns *Queries
func setupDBForBench(tb testing.TB) (*sql.DB, *Queries) {
	tb.Helper()
	ctx := context.Background()
	db, err := InitDB(ctx, ":memory:")
	if err != nil {
		tb.Fatalf("InitDB: %v", err)
	}
	q := NewQueries(db)
	tb.Cleanup(func() { _ = CloseDB(db) })
	return db, q
}
