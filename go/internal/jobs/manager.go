package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/garnizeh/eth-scanner/internal/database"
)

// Manager encapsulates job management operations.
type Manager struct {
	db *database.Queries
}

var (
	ErrPrefixExhausted  = errors.New("requested prefix is already fully scanned or unavailable")
	ErrJobNotFound      = errors.New("job not found")
	ErrJobNotProcessing = errors.New("job not processing")
	ErrWorkerMismatch   = errors.New("worker mismatch")
	ErrInvalidNonce     = errors.New("invalid nonce: outside range or smaller than current")
)

// New constructs a new Manager with the provided database queries.
func New(db *database.Queries) *Manager {
	return &Manager{db: db}
}

// LeaseExistingJob attempts to find an available (pending or expired) job
// and lease it to the provided workerID.
// It also checks if the worker already has an active, unexpired job they
// are already assigned to, in case they are resuming after a crash.
// If no job is available, returns (nil, nil).
// Lease duration defaults to 1 hour.
func (m *Manager) LeaseExistingJob(ctx context.Context, workerID, workerType string) (*database.Job, error) {
	if m == nil || m.db == nil {
		return nil, fmt.Errorf("manager or db is nil")
	}

	// First, check if this worker already has an active, unexpired lease.
	// This supports worker crash recovery before the lease expires.
	userJobs, err := m.db.GetJobsByWorker(ctx, sql.NullString{String: workerID, Valid: true})
	if err == nil {
		for _, j := range userJobs {
			if j.Status == "processing" && j.ExpiresAt.Valid && j.ExpiresAt.Time.UTC().After(time.Now().UTC()) {
				// Extend the lease duration slightly to ensure they have enough time to actually resume.
				// This is optional but good practice.
				leaseSeconds := int64((1 * time.Hour).Seconds())
				p := database.LeaseBatchParams{
					WorkerID:     sql.NullString{String: workerID, Valid: true},
					WorkerType:   sql.NullString{String: workerType, Valid: workerType != ""},
					LeaseSeconds: sql.NullString{String: fmt.Sprintf("%d", leaseSeconds), Valid: true},
					ID:           j.ID,
				}
				_, _ = m.db.LeaseBatch(ctx, p)

				// Re-load to return current record
				updated, err := m.db.GetJobByID(ctx, j.ID)
				if err == nil {
					return &updated, nil
				}
				return &j, nil
			}
		}
	}

	// Lease duration
	leaseSeconds := int64((1 * time.Hour).Seconds())

	// Try up to 3 times to find and lease an existing job to handle concurrency
	for range 3 {
		// Find an available batch (pending or expired)
		job, err := m.db.FindAvailableBatch(ctx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil
			}
			return nil, fmt.Errorf("find available batch: %w", err)
		}

		// Lease the batch (update worker_id, status, expires_at)
		p := database.LeaseBatchParams{
			WorkerID:     sql.NullString{String: workerID, Valid: true},
			WorkerType:   sql.NullString{String: workerType, Valid: workerType != ""},
			LeaseSeconds: sql.NullString{String: fmt.Sprintf("%d", leaseSeconds), Valid: true},
			ID:           job.ID,
		}
		rowsAffected, err := m.db.LeaseBatch(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("lease batch: %w", err)
		}

		if rowsAffected == 0 {
			// Someone else leased it between our Find and Lease, try again
			continue
		}

		// Re-load the job to return the up-to-date record
		updated, err := m.db.GetJobByID(ctx, job.ID)
		if err != nil {
			return nil, fmt.Errorf("get job after lease: %w", err)
		}
		return &updated, nil
	}

	return nil, nil // Fallback if we fail to lease after retries
}

// GetNextNonceRange returns the next available nonce range [nonceStart, nonceEnd]
// for a given 28-byte prefix and requested batch size. Nonces are uint32.
func (m *Manager) GetNextNonceRange(ctx context.Context, prefix28 []byte, batchSize uint32) (uint32, uint32, error) {
	if m == nil || m.db == nil {
		return 0, 0, fmt.Errorf("manager or db is nil")
	}
	if len(prefix28) != 28 {
		return 0, 0, fmt.Errorf("prefix_28 must be 28 bytes")
	}
	if batchSize == 0 {
		return 0, 0, fmt.Errorf("batchSize must be > 0")
	}
	// Use GetPrefixUsage to determine whether we've seen this prefix before
	// and obtain the highest nonce_end if present. This avoids ambiguity
	// between MAX(...) returning 0 vs NULL when no rows exist.
	usage, err := m.db.GetPrefixUsage(ctx, 1000)
	if err != nil {
		return 0, 0, fmt.Errorf("get prefix usage: %w", err)
	}

	var found bool
	var lastEnd uint64
	for _, row := range usage {
		if len(row.Prefix28) != len(prefix28) {
			continue
		}
		equal := true
		for i := range prefix28 {
			if row.Prefix28[i] != prefix28[i] {
				equal = false
				break
			}
		}
		if !equal {
			continue
		}
		found = true
		if row.HighestNonce == nil {
			lastEnd = 0
			break
		}
		switch v := row.HighestNonce.(type) {
		case int64:
			if v < 0 {
				return 0, 0, fmt.Errorf("invalid negative highest_nonce: %d", v)
			}
			lastEnd = uint64(v)
		default:
			return 0, 0, fmt.Errorf("unexpected type for highest_nonce: %T", v)
		}
		break
	}

	if !found {
		// No previous batches for this prefix: start at 0
		nonceStart := uint64(0)
		// remaining slots including nonceStart up to MaxUint32
		remaining := uint64(math.MaxUint32) - nonceStart + 1
		if remaining == 0 {
			return 0, 0, fmt.Errorf("nonce space exhausted for this prefix")
		}
		// If requested batch is larger than remaining, cap to remaining
		var alloc = uint64(batchSize)
		if alloc > remaining {
			alloc = remaining
		}
		nonceEnd64 := nonceStart + alloc
		// ensure values fit in uint32 before converting
		if nonceStart > uint64(math.MaxUint32) || (nonceEnd64-1) > uint64(math.MaxUint32) {
			return 0, 0, fmt.Errorf("nonce range overflow")
		}
		return uint32(nonceStart), uint32(nonceEnd64), nil
	}

	if lastEnd > uint64(math.MaxUint32) {
		return 0, 0, ErrPrefixExhausted
	}

	nonceStart := lastEnd
	if nonceStart > uint64(math.MaxUint32) {
		return 0, 0, fmt.Errorf("nonceStart overflow")
	}
	// remaining slots including nonceStart up to MaxUint32
	remaining := uint64(math.MaxUint32) - nonceStart + 1
	if remaining == 0 {
		return 0, 0, ErrPrefixExhausted
	}
	// cap allocation to remaining if requested is larger
	var alloc = uint64(batchSize)
	if alloc > remaining {
		alloc = remaining
	}
	nonceEnd64 := nonceStart + alloc

	// ensure values fit in uint32 before converting
	if nonceStart > uint64(math.MaxUint32) || (nonceEnd64-1) > uint64(math.MaxUint32) {
		return 0, 0, fmt.Errorf("nonce range overflow")
	}
	return uint32(nonceStart), uint32(nonceEnd64), nil
}

// CreateBatch creates a new job (batch) for the given prefix and batchSize.
// It computes the next nonce range and inserts a job record returning the created Job.
func (m *Manager) CreateBatch(ctx context.Context, prefix28 []byte, batchSize uint32) (*database.Job, error) {
	if m == nil || m.db == nil {
		return nil, fmt.Errorf("manager or db is nil")
	}
	if len(prefix28) != 28 {
		return nil, fmt.Errorf("prefix_28 must be 28 bytes")
	}
	if batchSize == 0 {
		return nil, fmt.Errorf("batchSize must be > 0")
	}

	// Determine nonce range
	start, end, err := m.GetNextNonceRange(ctx, prefix28, batchSize)
	if err != nil {
		return nil, fmt.Errorf("get next nonce range: %w", err)
	}

	// Prepare params for CreateBatch (sqlc generated)
	// Ensure expires_at is set using UTC-based lease duration (1 hour)
	leaseSeconds := int64((1 * time.Hour).Seconds())
	// Actual allocated batch size may be smaller than requested if near nonce space end
	allocated := uint64(end) - uint64(start) + 1
	// safe cast to int64 after explicit bounds check to satisfy static analyzers
	if allocated > uint64(math.MaxInt64) {
		return nil, fmt.Errorf("allocated batch size too large: %d", allocated)
	}
	allocatedInt := int64(allocated)
	params := database.CreateBatchParams{
		Prefix28:           prefix28,
		NonceStart:         int64(start),
		NonceEnd:           int64(end),
		CurrentNonce:       sql.NullInt64{Int64: int64(start), Valid: true},
		WorkerID:           sql.NullString{Valid: false},
		WorkerType:         sql.NullString{Valid: false},
		LeaseSeconds:       sql.NullString{String: fmt.Sprintf("%d", leaseSeconds), Valid: true},
		RequestedBatchSize: sql.NullInt64{Int64: allocatedInt, Valid: true},
	}

	job, err := m.db.CreateBatch(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("create batch: %w", err)
	}
	return &job, nil
}

// FindOrCreateMacroJob finds an existing long-lived (macro) job for the given
// prefix and leases it to the provided workerID. If no such job exists, a new
// macro job covering the full nonce space is created and returned.
// Lease duration defaults to 1 hour.
func (m *Manager) FindOrCreateMacroJob(ctx context.Context, prefix28 []byte, workerID string) (*database.Job, error) {
	if m == nil || m.db == nil {
		return nil, fmt.Errorf("manager or db is nil")
	}
	if len(prefix28) != 28 {
		return nil, fmt.Errorf("prefix_28 must be 28 bytes")
	}

	leaseSeconds := int64((1 * time.Hour).Seconds())

	// Try to find an existing incomplete macro job for this prefix
	job, err := m.db.FindIncompleteMacroJob(ctx, prefix28)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("find incomplete macro job: %w", err)
		}

		// No existing macro job — create one that spans the full 32-bit nonce space
		params := database.CreateMacroJobParams{
			Prefix28:           prefix28,
			NonceStart:         int64(0),
			NonceEnd:           int64(math.MaxUint32),
			CurrentNonce:       sql.NullInt64{Int64: 0, Valid: true},
			WorkerID:           sql.NullString{String: workerID, Valid: true},
			WorkerType:         sql.NullString{Valid: false},
			LeaseSeconds:       sql.NullString{String: fmt.Sprintf("%d", leaseSeconds), Valid: true},
			RequestedBatchSize: sql.NullInt64{Valid: false},
		}
		created, err := m.db.CreateMacroJob(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("create macro job: %w", err)
		}
		return &created, nil
	}

	// Existing job found — attempt to lease it to the caller
	p := database.LeaseMacroJobParams{
		WorkerID:     sql.NullString{String: workerID, Valid: true},
		WorkerType:   sql.NullString{Valid: false},
		LeaseSeconds: sql.NullString{String: fmt.Sprintf("%d", leaseSeconds), Valid: true},
		ID:           job.ID,
	}
	rowsAffected, err := m.db.LeaseMacroJob(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("lease macro job: %w", err)
	}
	if rowsAffected == 0 {
		// Race: someone else holds the lease — reload and return
		updated, err := m.db.GetJobByID(ctx, job.ID)
		if err != nil {
			return nil, fmt.Errorf("get job after lease race: %w", err)
		}
		return &updated, nil
	}

	updated, err := m.db.GetJobByID(ctx, job.ID)
	if err != nil {
		return nil, fmt.Errorf("get job after lease: %w", err)
	}
	return &updated, nil
}

// UpdateCheckpoint validates and updates job progress.
func (m *Manager) UpdateCheckpoint(ctx context.Context, jobID int64, workerID string, currentNonce int64, keysScanned int64, durationMs int64) error {
	if m == nil || m.db == nil {
		return fmt.Errorf("manager or db is nil")
	}

	job, err := m.db.GetJobByID(ctx, jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrJobNotFound
		}
		return fmt.Errorf("get job: %w", err)
	}

	if job.Status != "processing" {
		return ErrJobNotProcessing
	}

	if !job.WorkerID.Valid || job.WorkerID.String != workerID {
		return ErrWorkerMismatch
	}

	// Nonce validation
	if currentNonce < job.NonceStart || currentNonce > job.NonceEnd {
		return fmt.Errorf("%w: %d is outside range [%d, %d]", ErrInvalidNonce, currentNonce, job.NonceStart, job.NonceEnd)
	}
	// Maintain monotonicity: nonce should not go backwards
	if job.CurrentNonce.Valid && currentNonce < job.CurrentNonce.Int64 {
		return fmt.Errorf("%w: %d is smaller than current %d", ErrInvalidNonce, currentNonce, job.CurrentNonce.Int64)
	}

	params := database.UpdateCheckpointParams{
		CurrentNonce: sql.NullInt64{Int64: currentNonce, Valid: true},
		KeysScanned:  sql.NullInt64{Int64: keysScanned, Valid: true},
		DurationMs:   sql.NullInt64{Int64: durationMs, Valid: true},
		ID:           jobID,
		WorkerID:     sql.NullString{String: workerID, Valid: true},
	}
	if err := m.db.UpdateCheckpoint(ctx, params); err != nil {
		return fmt.Errorf("update checkpoint: %w", err)
	}

	return nil
}

// CompleteJob validates and marks a job as completed.
func (m *Manager) CompleteJob(ctx context.Context, jobID int64, workerID string, keysScanned int64, durationMs int64) error {
	if m == nil || m.db == nil {
		return fmt.Errorf("manager or db is nil")
	}

	job, err := m.db.GetJobByID(ctx, jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrJobNotFound
		}
		return fmt.Errorf("get job: %w", err)
	}

	if job.Status != "processing" {
		return ErrJobNotProcessing
	}

	if !job.WorkerID.Valid || job.WorkerID.String != workerID {
		return ErrWorkerMismatch
	}

	// Set complete status using sqcl-generated method
	params := database.CompleteBatchParams{
		KeysScanned: sql.NullInt64{Int64: keysScanned, Valid: true},
		DurationMs:  sql.NullInt64{Int64: durationMs, Valid: true},
		ID:          jobID,
		WorkerID:    sql.NullString{String: workerID, Valid: true},
	}
	if err := m.db.CompleteBatch(ctx, params); err != nil {
		return fmt.Errorf("complete batch: %w", err)
	}

	return nil
}
