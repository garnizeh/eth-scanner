package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// Test that worker_history is pruned to WORKER_HISTORY_LIMIT after inserts.
func TestRetention_PruneWorkerHistory(t *testing.T) {
	// small limit for test
	t.Setenv("WORKER_HISTORY_LIMIT", "50")

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "retention_history.db")

	ctx := context.Background()
	db, err := InitDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer func() { _ = CloseDB(db) }()

	// Insert 100 history rows for same worker
	for i := range 100 {
		if _, err := db.ExecContext(ctx, "INSERT INTO worker_history (worker_id) VALUES (?)", "worker-h"); err != nil {
			t.Fatalf("insert worker_history failed at %d: %v", i, err)
		}
	}

	var cnt int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_history").Scan(&cnt); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if cnt != 50 {
		t.Fatalf("expected 50 rows after prune, got %d", cnt)
	}
}

// Test that per-worker daily and monthly tables are pruned to configured limits
func TestRetention_PruneDailyAndMonthlyPerWorker(t *testing.T) {
	t.Setenv("WORKER_DAILY_STATS_LIMIT", "20")
	t.Setenv("WORKER_MONTHLY_STATS_LIMIT", "15")

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "retention_daily_monthly.db")

	ctx := context.Background()
	db, err := InitDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer func() { _ = CloseDB(db) }()

	workerA := "worker-A"
	workerB := "worker-B"

	// Insert 30 daily rows for workerA and 5 for workerB
	for i := range 30 {
		d := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		if _, err := db.ExecContext(ctx, "INSERT INTO worker_stats_daily (worker_id, stats_date, total_batches) VALUES (?, ?, 1)", workerA, d); err != nil {
			t.Fatalf("insert daily row workerA failed at %d: %v", i, err)
		}
	}
	for i := range 5 {
		d := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		if _, err := db.ExecContext(ctx, "INSERT INTO worker_stats_daily (worker_id, stats_date, total_batches) VALUES (?, ?, 1)", workerB, d); err != nil {
			t.Fatalf("insert daily row workerB failed at %d: %v", i, err)
		}
	}

	var countA, countB int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_stats_daily WHERE worker_id = ?", workerA).Scan(&countA); err != nil {
		t.Fatalf("countA query error: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_stats_daily WHERE worker_id = ?", workerB).Scan(&countB); err != nil {
		t.Fatalf("countB query error: %v", err)
	}

	if countA != 20 {
		t.Fatalf("expected workerA daily count 20 after prune, got %d", countA)
	}
	if countB != 5 {
		t.Fatalf("expected workerB daily count 5, got %d", countB)
	}

	// Insert 20 monthly rows for workerM and ensure pruning to 15
	workerM := "worker-M"
	for i := range 20 {
		m := time.Now().UTC().AddDate(0, -i, 0).Format("2006-01")
		if _, err := db.ExecContext(ctx, "INSERT INTO worker_stats_monthly (worker_id, stats_month, total_batches) VALUES (?, ?, 1)", workerM, m); err != nil {
			t.Fatalf("insert monthly row failed at %d: %v", i, err)
		}
	}
	var countM int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_stats_monthly WHERE worker_id = ?", workerM).Scan(&countM); err != nil {
		t.Fatalf("count monthly query error: %v", err)
	}
	if countM != 15 {
		t.Fatalf("expected monthly count 15 after prune, got %d", countM)
	}
}

// Test that re-initializing the DB with a new limit recreates triggers and the
// new limit is respected.
func TestRetention_RecreateTriggersWithNewLimits(t *testing.T) {

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "retention_recreate.db")

	// First init with daily limit = 5
	t.Setenv("WORKER_DAILY_STATS_LIMIT", "5")
	ctx := context.Background()
	db, err := InitDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close failed: %v", err)
	}

	// Re-init with daily limit = 3
	t.Setenv("WORKER_DAILY_STATS_LIMIT", "3")
	db2, err := InitDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("second InitDB failed: %v", err)
	}
	defer func() { _ = CloseDB(db2) }()

	// Insert 10 daily rows for workerR and expect prune to 3
	workerR := "worker-R"
	for i := range 10 {
		d := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		if _, err := db2.ExecContext(ctx, "INSERT INTO worker_stats_daily (worker_id, stats_date, total_batches) VALUES (?, ?, 1)", workerR, d); err != nil {
			t.Fatalf("insert daily row workerR failed at %d: %v", i, err)
		}
	}
	var cnt int
	if err := db2.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_stats_daily WHERE worker_id = ?", workerR).Scan(&cnt); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if cnt != 3 {
		t.Fatalf("expected workerR daily count 3 after recreate, got %d", cnt)
	}
}

// Recreate history limit triggers and ensure new limit is enforced
func TestRetention_RecreateHistoryLimit(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "retention_recreate_history.db")

	// Init with history limit 30
	t.Setenv("WORKER_HISTORY_LIMIT", "30")
	ctx := context.Background()
	db, err := InitDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close failed: %v", err)
	}

	// Re-init with history limit 10
	t.Setenv("WORKER_HISTORY_LIMIT", "10")
	db2, err := InitDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("second InitDB failed: %v", err)
	}
	defer func() { _ = CloseDB(db2) }()

	// Insert 50 history rows and expect final count = 10
	for i := 0; i < 50; i++ {
		if _, err := db2.ExecContext(ctx, "INSERT INTO worker_history (worker_id) VALUES (?)", "worker-h"); err != nil {
			t.Fatalf("insert worker_history failed at %d: %v", i, err)
		}
	}
	var cnt int
	if err := db2.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_history").Scan(&cnt); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if cnt != 10 {
		t.Fatalf("expected 10 rows after recreate prune, got %d", cnt)
	}
}
