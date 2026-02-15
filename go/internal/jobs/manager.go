package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
func (m *Manager) LeaseExistingJob(ctx context.Context, workerID string) (*database.Job, error) {
	if m == nil || m.db == nil {
		return nil, fmt.Errorf("manager or db is nil")
	}

	// Find an available batch (pending or expired)
	job, err := m.db.FindAvailableBatch(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find available batch: %w", err)
	}

	// Lease parameters
	leaseSeconds := int64((1 * time.Hour).Seconds())

	// Lease the batch (update worker_id, status, expires_at)
	p := database.LeaseBatchParams{
		WorkerID:   sql.NullString{String: workerID, Valid: true},
		WorkerType: sql.NullString{Valid: false},
		Column3:    sql.NullString{String: fmt.Sprintf("%d", leaseSeconds), Valid: true},
		ID:         job.ID,
	}
	if err := m.db.LeaseBatch(ctx, p); err != nil {
		return nil, fmt.Errorf("lease batch: %w", err)
	}

	// Re-load the job to return the up-to-date record
	updated, err := m.db.GetJobByID(ctx, job.ID)
	if err != nil {
		return nil, fmt.Errorf("get job after lease: %w", err)
	}
	return &updated, nil
}
