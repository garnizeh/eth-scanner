package jobs

import (
	"context"
	"database/sql"
	"math"
	"testing"
	"time"

	"github.com/garnizeh/eth-scanner/internal/database"
)

func setupInMemoryDB(t *testing.T) (*sql.DB, *database.Queries) {
	t.Helper()
	ctx := t.Context()
	db, err := database.InitDB(ctx, ":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	q := database.NewQueries(db)
	t.Cleanup(func() {
		if err := database.CloseDB(db); err != nil {
			t.Fatalf("CloseDB: %v", err)
		}
	})
	return db, q
}

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

func TestLeaseExistingJob_NoJobsAvailable(t *testing.T) {
	ctx := t.Context()
	_, q := setupInMemoryDB(t)
	m := New(q)

	job, err := m.LeaseExistingJob(ctx, "worker-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job != nil {
		t.Fatalf("expected no job available, got: %+v", job)
	}
}

func TestLeaseExistingJob_PendingJob(t *testing.T) {
	ctx := t.Context()
	db, q := setupInMemoryDB(t)
	m := New(q)

	// insert pending job
	prefix := make([]byte, 28)
	if _, err := db.ExecContext(context.Background(), `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, requested_batch_size) VALUES (?, ?, ?, 'pending', ?)`, prefix, 0, 999, 1000); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}

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
	ctx := t.Context()
	db, q := setupInMemoryDB(t)
	m := New(q)

	// insert processing job with expired expires_at
	prefix := make([]byte, 28)
	past := time.Now().UTC().Add(-2 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := db.ExecContext(context.Background(), `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, expires_at, requested_batch_size) VALUES (?, ?, ?, 'processing', ?, ?, ?)`, prefix, 0, 999, "old-worker", past, 1000); err != nil {
		t.Fatalf("insert expired job: %v", err)
	}

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
	ctx := t.Context()
	m := New(nil)

	job, err := m.LeaseExistingJob(ctx, "worker-1")
	if err == nil {
		t.Fatal("expected error when manager is nil")
	}
	if job != nil {
		t.Fatalf("expected no job, got: %+v", job)
	}
}

func TestGetNextNonceRange_BatchSizeZero(t *testing.T) {
	ctx := t.Context()
	_, q := setupInMemoryDB(t)
	m := New(q)

	prefix := make([]byte, 28)
	_, _, err := m.GetNextNonceRange(ctx, prefix, 0)
	if err == nil {
		t.Fatalf("expected error for batchSize 0, got nil")
	}
}

func TestGetNextNonceRange_FirstAllocation(t *testing.T) {
	ctx := t.Context()
	_, q := setupInMemoryDB(t)
	m := New(q)

	prefix := make([]byte, 28)
	start, end, err := m.GetNextNonceRange(ctx, prefix, 1000)
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
	ctx := t.Context()
	db, q := setupInMemoryDB(t)

	// insert a job with nonce_end = 999
	prefix := make([]byte, 28)
	if _, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, requested_batch_size) VALUES (?, ?, ?, 'completed', ?)`, prefix, 0, 999, 1000); err != nil {
		t.Fatalf("insert job: %v", err)
	}

	m := New(q)
	start, end, err := m.GetNextNonceRange(ctx, prefix, 200)
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
	ctx := t.Context()
	_, q := setupInMemoryDB(t)
	m := New(q)

	_, _, err := m.GetNextNonceRange(ctx, []byte{1, 2, 3}, 100)
	if err == nil {
		t.Fatalf("expected error for invalid prefix length")
	}
}

func TestGetNextNonceRange_Overflow(t *testing.T) {
	ctx := t.Context()
	db, q := setupInMemoryDB(t)
	m := New(q)

	// set last nonce_end near max uint32
	prefix := make([]byte, 28)
	near := int64(math.MaxUint32 - 10)
	if _, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, requested_batch_size) VALUES (?, ?, ?, 'completed', ?)`, prefix, 0, near, 1000); err != nil {
		t.Fatalf("insert job: %v", err)
	}

	_, _, err := m.GetNextNonceRange(ctx, prefix, 20)
	if err == nil {
		t.Fatalf("expected overflow error, got nil")
	}
}

func TestGetNextNonceRange_NilManager(t *testing.T) {
	ctx := t.Context()
	m := New(nil)

	prefix := make([]byte, 28)
	_, _, err := m.GetNextNonceRange(ctx, prefix, 100)
	if err == nil {
		t.Fatal("expected error when manager is nil")
	}
}

func TestCreateBatch_Success(t *testing.T) {
	ctx := t.Context()
	_, q := setupInMemoryDB(t)
	m := New(q)

	prefix := make([]byte, 28)
	job, err := m.CreateBatch(ctx, prefix, 1000)
	if err != nil {
		t.Fatalf("CreateBatch error: %v", err)
	}
	if job.NonceStart != 0 || job.NonceEnd != 999 {
		t.Fatalf("unexpected nonce range: %d-%d", job.NonceStart, job.NonceEnd)
	}
}

func TestCreateBatch_Subsequent(t *testing.T) {
	ctx := t.Context()
	_, q := setupInMemoryDB(t)
	m := New(q)

	prefix := make([]byte, 28)
	j1, err := m.CreateBatch(ctx, prefix, 1000)
	if err != nil {
		t.Fatalf("CreateBatch 1 error: %v", err)
	}
	j2, err := m.CreateBatch(ctx, prefix, 200)
	if err != nil {
		t.Fatalf("CreateBatch 2 error: %v", err)
	}
	if j2.NonceStart <= j1.NonceEnd {
		t.Fatalf("expected non-overlapping ranges, got %d <= %d", j2.NonceStart, j1.NonceEnd)
	}
}

func TestCreateBatch_InvalidPrefix(t *testing.T) {
	ctx := t.Context()
	_, q := setupInMemoryDB(t)
	m := New(q)

	_, err := m.CreateBatch(ctx, []byte{1, 2, 3}, 100)
	if err == nil {
		t.Fatalf("expected error for invalid prefix length")
	}
}

func TestCreateBatch_NilManager(t *testing.T) {
	ctx := t.Context()
	m := New(nil)

	prefix := make([]byte, 28)
	job, err := m.CreateBatch(ctx, prefix, 100)
	if err == nil {
		t.Fatal("expected error when manager is nil")
	}
	if job != nil {
		t.Fatalf("expected no job, got: %+v", job)
	}
}

func TestCreateBatch_BatchSizeZero(t *testing.T) {
	ctx := t.Context()
	_, q := setupInMemoryDB(t)
	m := New(q)

	prefix := make([]byte, 28)
	job, err := m.CreateBatch(ctx, prefix, 0)
	if err == nil {
		t.Fatalf("expected error for batchSize 0, got nil")
	}
	if job != nil {
		t.Fatalf("expected no job, got: %+v", job)
	}
}
