package jobs

import (
	"context"
	"database/sql"
	"math"
	"testing"
	"time"

	"github.com/garnizeh/eth-scanner/internal/database"
)

func TestNewManager(t *testing.T) {
	var q *database.Queries
	m := New(q)
	if m == nil {
		t.Fatal("expected non-nil Manager")
	}
	if m.db != q {
		t.Fatalf("expected db to be %v, got %v", q, m.db)
	}
}

func setupInMemoryDB(t *testing.T) (*sql.DB, *database.Queries) {
	t.Helper()
	ctx := context.Background()
	db, err := database.InitDB(ctx, ":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	q := database.NewQueries(db)
	return db, q
}

func TestLeaseExistingJob_NoJobsAvailable(t *testing.T) {
	db, q := setupInMemoryDB(t)
	t.Cleanup(func() {
		if err := database.CloseDB(db); err != nil {
			t.Fatalf("CloseDB: %v", err)
		}
	})

	m := New(q)
	ctx := context.Background()

	job, err := m.LeaseExistingJob(ctx, "worker-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job != nil {
		t.Fatalf("expected no job available, got: %+v", job)
	}
}

func TestLeaseExistingJob_PendingJob(t *testing.T) {
	db, q := setupInMemoryDB(t)
	t.Cleanup(func() {
		if err := database.CloseDB(db); err != nil {
			t.Fatalf("CloseDB: %v", err)
		}
	})

	// insert pending job
	prefix := make([]byte, 28)
	if _, err := db.ExecContext(context.Background(), `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, requested_batch_size) VALUES (?, ?, ?, 'pending', ?)`, prefix, 0, 999, 1000); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}

	m := New(q)
	ctx := context.Background()

	leased, err := m.LeaseExistingJob(ctx, "worker-1")
	if err != nil {
		t.Fatalf("LeaseExistingJob error: %v", err)
	}
	if leased == nil {
		t.Fatal("expected job to be leased, got nil")
	}
	if leased.Status != "processing" {
		t.Fatalf("expected status processing, got %s", leased.Status)
	}
	if !leased.WorkerID.Valid || leased.WorkerID.String != "worker-1" {
		t.Fatalf("expected worker_id worker-1, got %+v", leased.WorkerID)
	}
	if !leased.ExpiresAt.Valid {
		t.Fatalf("expected expires_at to be set")
	}
}

func TestLeaseExistingJob_ExpiredJob(t *testing.T) {
	db, q := setupInMemoryDB(t)
	t.Cleanup(func() {
		if err := database.CloseDB(db); err != nil {
			t.Fatalf("CloseDB: %v", err)
		}
	})

	// insert processing job with expired expires_at
	prefix := make([]byte, 28)
	past := time.Now().UTC().Add(-2 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := db.ExecContext(context.Background(), `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, expires_at, requested_batch_size) VALUES (?, ?, ?, 'processing', ?, ?, ?)`, prefix, 0, 999, "old-worker", past, 1000); err != nil {
		t.Fatalf("insert expired job: %v", err)
	}

	m := New(q)
	ctx := context.Background()

	leased, err := m.LeaseExistingJob(ctx, "worker-2")
	if err != nil {
		t.Fatalf("LeaseExistingJob error: %v", err)
	}
	if leased == nil {
		t.Fatal("expected job to be leased, got nil")
	}
	if !leased.WorkerID.Valid || leased.WorkerID.String != "worker-2" {
		t.Fatalf("expected worker_id worker-2, got %+v", leased.WorkerID)
	}
}

func TestLeaseExistingJob_NilManager(t *testing.T) {
	m := New(nil)
	ctx := context.Background()

	job, err := m.LeaseExistingJob(ctx, "worker-1")
	if err == nil {
		t.Fatal("expected error when manager is nil")
	}
	if job != nil {
		t.Fatalf("expected no job, got: %+v", job)
	}
}

func TestGetNextNonceRange_BatchSizeZero(t *testing.T) {
	db, q := setupInMemoryDB(t)
	t.Cleanup(func() { _ = database.CloseDB(db) })

	m := New(q)
	prefix := make([]byte, 28)

	start, end, err := m.GetNextNonceRange(context.Background(), prefix, 0)
	if err == nil {
		t.Fatalf("expected error for batchSize 0, got nil")
	}
	if start != 0 {
		t.Fatalf("expected start 0, got %d", start)
	}
	if end != 0 {
		t.Fatalf("expected end 0, got %d", end)
	}
}

func TestGetNextNonceRange_FirstAllocation(t *testing.T) {
	db, q := setupInMemoryDB(t)
	t.Cleanup(func() { _ = database.CloseDB(db) })

	m := New(q)
	prefix := make([]byte, 28)

	start, end, err := m.GetNextNonceRange(context.Background(), prefix, 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start != 0 {
		t.Fatalf("expected start 0, got %d", start)
	}
	if end != 999 {
		t.Fatalf("expected end 999, got %d", end)
	}
}

func TestGetNextNonceRange_SubsequentAllocation(t *testing.T) {
	db, q := setupInMemoryDB(t)
	t.Cleanup(func() { _ = database.CloseDB(db) })

	// insert a job with nonce_end = 999
	prefix := make([]byte, 28)
	if _, err := db.ExecContext(context.Background(), `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, requested_batch_size) VALUES (?, ?, ?, 'completed', ?)`, prefix, 0, 999, 1000); err != nil {
		t.Fatalf("insert job: %v", err)
	}

	m := New(q)
	start, end, err := m.GetNextNonceRange(context.Background(), prefix, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start != 1000 {
		t.Fatalf("expected start 1000, got %d", start)
	}
	if end != 1199 {
		t.Fatalf("expected end 1199, got %d", end)
	}
}

func TestGetNextNonceRange_InvalidPrefix(t *testing.T) {
	db, q := setupInMemoryDB(t)
	t.Cleanup(func() { _ = database.CloseDB(db) })

	m := New(q)
	_, _, err := m.GetNextNonceRange(context.Background(), []byte{1, 2, 3}, 100)
	if err == nil {
		t.Fatalf("expected error for invalid prefix length")
	}
}

func TestGetNextNonceRange_Overflow(t *testing.T) {
	db, q := setupInMemoryDB(t)
	t.Cleanup(func() { _ = database.CloseDB(db) })

	// set last nonce_end near max uint32
	prefix := make([]byte, 28)
	near := int64(math.MaxUint32 - 10)
	if _, err := db.ExecContext(context.Background(), `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, requested_batch_size) VALUES (?, ?, ?, 'completed', ?)`, prefix, 0, near, 1000); err != nil {
		t.Fatalf("insert job: %v", err)
	}

	m := New(q)
	_, _, err := m.GetNextNonceRange(context.Background(), prefix, 20)
	if err == nil {
		t.Fatalf("expected overflow error, got nil")
	}
}
