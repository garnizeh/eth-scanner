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

// New constructs a new Manager with the provided database queries.
func New(db *database.Queries) *Manager {
	return &Manager{db: db}
}

// LeaseExistingJob attempts to find an available (pending or expired) job
// and lease it to the provided workerID. If no job is available, returns (nil, nil).
// Lease duration defaults to 1 hour.
func (m *Manager) LeaseExistingJob(ctx context.Context, workerID, workerType string) (*database.Job, error) {
	if m == nil || m.db == nil {
		return nil, fmt.Errorf("manager or db is nil")
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
		nonceEnd64 := nonceStart + uint64(batchSize) - 1
		if nonceEnd64 > uint64(math.MaxUint32) {
			return 0, 0, fmt.Errorf("batch size causes nonce_end overflow")
		}
		return uint32(nonceStart), uint32(nonceEnd64), nil
	}

	if lastEnd >= uint64(math.MaxUint32) {
		return 0, 0, fmt.Errorf("nonce space exhausted for this prefix")
	}

	nonceStart := lastEnd + 1
	// Check overflow when adding batchSize-1
	if uint64(batchSize) > 0 && nonceStart > uint64(math.MaxUint32) {
		return 0, 0, fmt.Errorf("nonceStart overflow")
	}
	nonceEnd64 := nonceStart + uint64(batchSize) - 1
	if nonceEnd64 > uint64(math.MaxUint32) {
		return 0, 0, fmt.Errorf("batch size causes nonce_end overflow")
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
	params := database.CreateBatchParams{
		Prefix28:           prefix28,
		NonceStart:         int64(start),
		NonceEnd:           int64(end),
		CurrentNonce:       sql.NullInt64{Int64: int64(start), Valid: true},
		WorkerID:           sql.NullString{Valid: false},
		WorkerType:         sql.NullString{Valid: false},
		LeaseSeconds:       sql.NullString{String: fmt.Sprintf("%d", leaseSeconds), Valid: true},
		RequestedBatchSize: sql.NullInt64{Int64: int64(batchSize), Valid: true},
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
