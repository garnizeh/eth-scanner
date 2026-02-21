package jobs

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
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

func TestUpdateCheckpoint_Success(t *testing.T) {
	ctx := t.Context()
	db, q := setupInMemoryDB(t)
	m := New(q)

	prefix := make([]byte, 28)
	// Create a processing job for worker-1
	res, err := db.ExecContext(ctx, "INSERT INTO jobs (prefix_28, nonce_start, nonce_end, current_nonce, status, worker_id, requested_batch_size) VALUES (?, 0, 999, 0, 'processing', 'worker-1', 1000)", prefix)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	id, _ := res.LastInsertId()

	err = m.UpdateCheckpoint(ctx, id, "worker-1", 500, 500, 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated, _ := q.GetJobByID(ctx, id)
	if updated.CurrentNonce.Int64 != 500 {
		t.Errorf("expected current_nonce 500, got %d", updated.CurrentNonce.Int64)
	}
	if updated.KeysScanned.Int64 != 500 {
		t.Errorf("expected keys_scanned 500, got %d", updated.KeysScanned.Int64)
	}
}

func TestUpdateCheckpoint_Errors(t *testing.T) {
	ctx := t.Context()
	db, q := setupInMemoryDB(t)
	m := New(q)

	prefix := make([]byte, 28)
	// Create a processing job for worker-1 [1000, 1999], current = 1000
	res, err := db.ExecContext(ctx, "INSERT INTO jobs (prefix_28, nonce_start, nonce_end, current_nonce, status, worker_id, requested_batch_size) VALUES (?, 1000, 1999, 1000, 'processing', 'worker-1', 1000)", prefix)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	id, _ := res.LastInsertId()

	t.Run("NotFound", func(t *testing.T) {
		err := m.UpdateCheckpoint(ctx, id+999, "worker-1", 1500, 500, 1000)
		if err == nil || !errors.Is(err, ErrJobNotFound) {
			t.Errorf("expected ErrJobNotFound, got %v", err)
		}
	})

	t.Run("WorkerMismatch", func(t *testing.T) {
		err := m.UpdateCheckpoint(ctx, id, "wrong-worker", 1500, 500, 1000)
		if err == nil || !errors.Is(err, ErrWorkerMismatch) {
			t.Errorf("expected ErrWorkerMismatch, got %v", err)
		}
	})

	t.Run("InvalidNonceRange_TooSmall", func(t *testing.T) {
		err := m.UpdateCheckpoint(ctx, id, "worker-1", 500, 500, 1000)
		if err == nil || !errors.Is(err, ErrInvalidNonce) {
			t.Errorf("expected ErrInvalidNonce, got %v", err)
		}
	})

	t.Run("InvalidNonceRange_TooLarge", func(t *testing.T) {
		err := m.UpdateCheckpoint(ctx, id, "worker-1", 2500, 500, 1000)
		if err == nil || !errors.Is(err, ErrInvalidNonce) {
			t.Errorf("expected ErrInvalidNonce, got %v", err)
		}
	})

	t.Run("BackwardNonce", func(t *testing.T) {
		// Set current nonce to 1500
		_, _ = db.ExecContext(ctx, "UPDATE jobs SET current_nonce = 1500 WHERE id = ?", id)
		err := m.UpdateCheckpoint(ctx, id, "worker-1", 1200, 500, 1000)
		if err == nil || !errors.Is(err, ErrInvalidNonce) {
			t.Errorf("expected ErrInvalidNonce (backward), got %v", err)
		}
	})

	t.Run("NotProcessing", func(t *testing.T) {
		_, _ = db.ExecContext(ctx, "UPDATE jobs SET status = 'completed' WHERE id = ?", id)
		err := m.UpdateCheckpoint(ctx, id, "worker-1", 1800, 500, 1000)
		if err == nil || !errors.Is(err, ErrJobNotProcessing) {
			t.Errorf("expected ErrJobNotProcessing, got %v", err)
		}
	})
}

func TestCompleteJob_Success(t *testing.T) {
	ctx := t.Context()
	db, q := setupInMemoryDB(t)
	m := New(q)

	prefix := make([]byte, 28)
	// Create a processing job for worker-1
	res, err := db.ExecContext(ctx, "INSERT INTO jobs (prefix_28, nonce_start, nonce_end, current_nonce, status, worker_id, requested_batch_size) VALUES (?, 0, 999, 500, 'processing', 'worker-1', 1000)", prefix)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	id, _ := res.LastInsertId()

	err = m.CompleteJob(ctx, id, "worker-1", 1000, 2000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated, _ := q.GetJobByID(ctx, id)
	if updated.Status != "completed" {
		t.Errorf("expected status completed, got %s", updated.Status)
	}
	if updated.CurrentNonce.Int64 != updated.NonceEnd {
		t.Errorf("expected current_nonce %d, got %d", updated.NonceEnd, updated.CurrentNonce.Int64)
	}
	if !updated.CompletedAt.Valid {
		t.Errorf("expected completed_at to be valid")
	}
}

func TestCompleteJob_Errors(t *testing.T) {
	ctx := t.Context()
	db, q := setupInMemoryDB(t)
	m := New(q)

	prefix := make([]byte, 28)
	res, err := db.ExecContext(ctx, "INSERT INTO jobs (prefix_28, nonce_start, nonce_end, current_nonce, status, worker_id, requested_batch_size) VALUES (?, 0, 999, 0, 'processing', 'worker-1', 1000)", prefix)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	id, _ := res.LastInsertId()

	t.Run("NotFound", func(t *testing.T) {
		err := m.CompleteJob(ctx, id+999, "worker-1", 1000, 2000)
		if err == nil || !errors.Is(err, ErrJobNotFound) {
			t.Errorf("expected ErrJobNotFound, got %v", err)
		}
	})

	t.Run("WorkerMismatch", func(t *testing.T) {
		err := m.CompleteJob(ctx, id, "wrong-worker", 1000, 2000)
		if err == nil || !errors.Is(err, ErrWorkerMismatch) {
			t.Errorf("expected ErrWorkerMismatch, got %v", err)
		}
	})

	t.Run("NotProcessing", func(t *testing.T) {
		_, _ = db.ExecContext(ctx, "UPDATE jobs SET status = 'completed' WHERE id = ?", id)
		err := m.CompleteJob(ctx, id, "worker-1", 1000, 2000)
		if err == nil || !errors.Is(err, ErrJobNotProcessing) {
			t.Errorf("expected ErrJobNotProcessing, got %v", err)
		}
	})
}

// TestGetNextNonceRange_TableDriven covers sequential ranges with varying sizes
// and ensures no gaps between allocations.
func TestGetNextNonceRange_TableDriven(t *testing.T) {
ctx := t.Context()
db, q := setupInMemoryDB(t)
m := New(q)

prefix := make([]byte, 28)
copy(prefix, []byte("test-prefix-1234567890123456"))

tests := []struct {
name      string
batchSize uint32
wantStart uint32
wantEnd   uint32
}{
{"First-10k", 10000, 0, 9999},
{"Second-5k", 5000, 10000, 14999},
{"Third-1k", 1000, 15000, 15999},
{"Fourth-50k", 50000, 16000, 65999},
}

for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
start, end, err := m.GetNextNonceRange(ctx, prefix, tt.batchSize)
if err != nil {
t.Fatalf("unexpected error: %v", err)
}
if start != tt.wantStart {
t.Errorf("start mismatch: got %d, want %d", start, tt.wantStart)
}
if end != tt.wantEnd {
t.Errorf("end mismatch: got %d, want %d", end, tt.wantEnd)
}
// Insert a job to persist the allocation so the next call sees the state
if _, err := db.ExecContext(ctx, "INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status) VALUES (?, ?, ?, 'pending')", prefix, int64(start), int64(end)); err != nil {
t.Fatalf("failed to insert job: %v", err)
}
})
}
}

// TestGetNextNonceRange_Exhaustion specifically tests the rollover (exhausted)
// case when the prefix nonce space is already full.
func TestGetNextNonceRange_Exhaustion(t *testing.T) {
ctx := t.Context()
db, q := setupInMemoryDB(t)
m := New(q)

prefix := make([]byte, 28)
// Seed one job that covers the entire range up to MaxUint32
if _, err := db.ExecContext(ctx, "INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status) VALUES (?, 0, ?, 'completed')", prefix, int64(math.MaxUint32)); err != nil {
t.Fatalf("seed error: %v", err)
}

start, end, err := m.GetNextNonceRange(ctx, prefix, 1000)
if !errors.Is(err, ErrPrefixExhausted) {
t.Fatalf("expected ErrPrefixExhausted, got (start=%d, end=%d, err=%v)", start, end, err)
}
}

// TestGetNextNonceRange_CapToMaxUint32 ensures that when we request a range
// that would exceed MaxUint32, the manager returns a capped range.
func TestGetNextNonceRange_CapToMaxUint32(t *testing.T) {
ctx := t.Context()
db, q := setupInMemoryDB(t)
m := New(q)

prefix := make([]byte, 28)
// Seed a job that ends near the end
last := uint32(math.MaxUint32 - 50)
if _, err := db.ExecContext(ctx, "INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status) VALUES (?, 0, ?, 'completed')", prefix, int64(last)); err != nil {
t.Fatalf("seed error: %v", err)
}

// Request more than remains (100 > 50)
start, end, err := m.GetNextNonceRange(ctx, prefix, 100)
if err != nil {
t.Fatalf("unexpected error: %v", err)
}
if start != last+1 {
t.Errorf("unexpected start: %d", start)
}
if end != uint32(math.MaxUint32) {
t.Errorf("unexpected end (capped): %d", end)
}

// Now it should definitely be exhausted
if _, err := db.ExecContext(ctx, "INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status) VALUES (?, ?, ?, 'completed')", prefix, int64(start), int64(end)); err != nil {
t.Fatalf("insert error: %v", err)
}
_, _, err = m.GetNextNonceRange(ctx, prefix, 10)
if !errors.Is(err, ErrPrefixExhausted) {
t.Fatalf("expected ErrPrefixExhausted, got %v", err)
}
}
