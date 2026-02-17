package jobs

import (
	"context"
	"database/sql"
	"encoding/hex"
	"math"
	"path/filepath"
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

	job, err := m.LeaseExistingJob(ctx, "worker-1", "pc")
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

	leased, err := m.LeaseExistingJob(ctx, "worker-1", "pc")
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

	leased, err := m.LeaseExistingJob(ctx, "worker-2", "pc")
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

	job, err := m.LeaseExistingJob(ctx, "worker-1", "pc")
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
	// use uint32 to avoid signed->unsigned conversion warnings
	near := uint32(math.MaxUint32 - 10)
	if _, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, requested_batch_size) VALUES (?, ?, ?, 'completed', ?)`, prefix, 0, int64(near), 1000); err != nil {
		t.Fatalf("insert job: %v", err)
	}

	// After change: allocation should be capped to remaining nonce space
	start, end, err := m.GetNextNonceRange(ctx, prefix, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if start != near+1 {
		t.Fatalf("expected start %d, got %d", near+1, start)
	}
	if end != uint32(math.MaxUint32) {
		t.Fatalf("expected end %d, got %d", uint32(math.MaxUint32), end)
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

func TestCreateBatch_ExpiresAtIsUTC(t *testing.T) {
	ctx := t.Context()
	_, q := setupInMemoryDB(t)
	m := New(q)

	prefix := make([]byte, 28)
	job, err := m.CreateBatch(ctx, prefix, 100)
	if err != nil {
		t.Fatalf("CreateBatch error: %v", err)
	}
	if !job.ExpiresAt.Valid {
		t.Fatalf("expected expires_at to be set")
	}
	if job.ExpiresAt.Time.Location() != time.UTC {
		t.Fatalf("expected expires_at to be UTC, got %v", job.ExpiresAt.Time.Location())
	}
}

func TestFindOrCreateMacroJob_NilManagerReceiver(t *testing.T) {
	ctx := t.Context()
	var m *Manager // nil receiver

	prefix := make([]byte, 28)
	job, err := m.FindOrCreateMacroJob(ctx, prefix, "worker-1")
	if err == nil {
		t.Fatal("expected error when manager receiver is nil")
	}
	if job != nil {
		t.Fatalf("expected nil job when manager is nil, got %+v", job)
	}
}

func TestFindOrCreateMacroJob_NilDB(t *testing.T) {
	ctx := t.Context()
	m := New(nil) // manager with nil db

	prefix := make([]byte, 28)
	job, err := m.FindOrCreateMacroJob(ctx, prefix, "worker-1")
	if err == nil {
		t.Fatal("expected error when manager.db is nil")
	}
	if job != nil {
		t.Fatalf("expected nil job when manager.db is nil, got %+v", job)
	}
}

func TestFindOrCreateMacroJob_InvalidPrefixLength(t *testing.T) {
	ctx := t.Context()
	_, q := setupInMemoryDB(t)
	m := New(q)

	// invalid prefix length
	job, err := m.FindOrCreateMacroJob(ctx, []byte{1, 2, 3}, "worker-1")
	if err == nil {
		t.Fatal("expected error for invalid prefix length")
	}
	if job != nil {
		t.Fatalf("expected nil job for invalid prefix length, got %+v", job)
	}
}

func TestFindOrCreateMacroJob_CreateAndReuse(t *testing.T) {
	ctx := t.Context()
	_, q := setupInMemoryDB(t)
	m := New(q)

	prefix := make([]byte, 28)

	// First call should create and lease a macro job to worker-1
	j1, err := m.FindOrCreateMacroJob(ctx, prefix, "worker-1")
	if err != nil {
		t.Fatalf("FindOrCreateMacroJob error: %v", err)
	}
	if j1 == nil {
		t.Fatal("expected job to be created, got nil")
	}
	if !j1.WorkerID.Valid || j1.WorkerID.String != "worker-1" {
		t.Fatalf("expected worker_id worker-1, got %+v", j1.WorkerID)
	}

	// Second call by same worker should return the same job ID
	j2, err := m.FindOrCreateMacroJob(ctx, prefix, "worker-1")
	if err != nil {
		t.Fatalf("FindOrCreateMacroJob second call error: %v", err)
	}
	if j2 == nil {
		t.Fatal("expected job on second call, got nil")
	}
	if j2.ID != j1.ID {
		t.Fatalf("expected same job ID reused, got %d and %d", j1.ID, j2.ID)
	}
}

func TestFindOrCreateMacroJob_LeaseExpiration(t *testing.T) {
	ctx := t.Context()
	db, q := setupInMemoryDB(t)
	m := New(q)

	// Insert a processing job with an expired lease and a checkpoint
	prefix := make([]byte, 28)
	past := time.Now().UTC().Add(-2 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, current_nonce, status, worker_id, expires_at, requested_batch_size) VALUES (?, ?, ?, ?, 'processing', ?, ?, ?)`, prefix, 0, 4294967295, 12345, "old-worker", past, 1000); err != nil {
		t.Fatalf("insert expired macro job: %v", err)
	}

	// Now request the macro job as a new worker; the manager should re-lease it
	j, err := m.FindOrCreateMacroJob(ctx, prefix, "worker-2")
	if err != nil {
		t.Fatalf("FindOrCreateMacroJob error: %v", err)
	}
	if j == nil {
		t.Fatal("expected job to be returned, got nil")
	}
	if !j.WorkerID.Valid || j.WorkerID.String != "worker-2" {
		t.Fatalf("expected worker_id worker-2, got %+v", j.WorkerID)
	}
	if j.CurrentNonce.Int64 != 12345 {
		t.Fatalf("expected current_nonce to be preserved (12345), got %v", j.CurrentNonce)
	}
}

// TestCreateBatch_CapsToRemaining ensures that when the nonce space for a prefix
// has fewer remaining nonces than requested, the manager will allocate only the
// remaining range (i.e. cap the batch to avoid overflow).
func TestCreateBatch_CapsToRemaining(t *testing.T) {
	ctx := context.Background()
	// Create a temporary DB file
	d := t.TempDir()
	dbPath := filepath.Join(d, "eth-scanner.db")

	// init DB
	db, err := database.InitDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	q := database.NewQueries(db)
	m := New(q)

	// Prepare a deterministic 28-byte prefix
	prefix, _ := hex.DecodeString("0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c")
	if len(prefix) != 28 {
		t.Fatalf("prefix length unexpected: %d", len(prefix))
	}

	// Insert an existing completed job that ends near MaxUint32 - 500
	const maxUint32 uint64 = 4294967295
	var lastEnd = int64(maxUint32 - 500)
	insertSQL := `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, created_at, completed_at)
VALUES (?, ?, ?, 'completed', datetime('now','utc'), datetime('now','utc'))`
	if _, err := db.ExecContext(ctx, insertSQL, prefix, 0, lastEnd); err != nil {
		t.Fatalf("failed to seed jobs: %v", err)
	}

	// Now request a batch larger than remaining (e.g., 1000 > 501 remaining)
	requested := uint32(1000)
	job, err := m.CreateBatch(ctx, prefix, requested)
	if err != nil {
		t.Fatalf("CreateBatch failed: %v", err)
	}

	// Assert the allocated range starts at lastEnd+1 and ends at MaxUint32
	if job.NonceStart != lastEnd+1 {
		t.Fatalf("unexpected nonce_start: got %d want %d", job.NonceStart, lastEnd+1)
	}
	if job.NonceEnd != int64(maxUint32) {
		t.Fatalf("unexpected nonce_end: got %d want %d", job.NonceEnd, maxUint32)
	}

	// Ensure the requested_batch_size stored matches the actual allocated size
	allocated := job.NonceEnd - job.NonceStart + 1
	if !job.RequestedBatchSize.Valid {
		t.Fatalf("requested_batch_size not set in created job")
	}
	if job.RequestedBatchSize.Int64 != allocated {
		t.Fatalf("requested_batch_size stored mismatch: got %d want %d", job.RequestedBatchSize.Int64, allocated)
	}
}
