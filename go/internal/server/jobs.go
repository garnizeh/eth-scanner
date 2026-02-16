package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/garnizeh/eth-scanner/internal/database"
	"github.com/garnizeh/eth-scanner/internal/jobs"
)

const (
	// maxBatchSize is a conservative upper bound for requested batch sizes.
	// Workers must request a positive value <= this limit.
	maxBatchSize  = 10_000_000
	leaseDuration = time.Hour
)

// handleJobLease handles POST /api/v1/jobs/lease
// Request JSON: {"worker_id":"...","requested_batch_size":12345, "prefix_28":"base64..."}
func (s *Server) handleJobLease(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		WorkerID           string  `json:"worker_id"`
		RequestedBatchSize uint32  `json:"requested_batch_size"`
		Prefix28           *string `json:"prefix_28,omitempty"`
	}

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var req reqBody
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.WorkerID == "" {
		http.Error(w, "worker_id is required", http.StatusBadRequest)
		return
	}
	if req.RequestedBatchSize == 0 || req.RequestedBatchSize > maxBatchSize {
		http.Error(w, "requested_batch_size must be >0 and <= max allowed", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// build manager backed by queries
	q := database.NewQueries(s.db)
	m := jobs.New(q)

	// Try to lease an existing available job first
	job, err := m.LeaseExistingJob(ctx, req.WorkerID)
	if err != nil {
		http.Error(w, "failed to lease existing job", http.StatusInternalServerError)
		return
	}

	// If none available, create and lease a new batch (extracted to helper to
	// reduce nesting and cyclomatic complexity).
	if job == nil {
		job, err = s.createAndLeaseBatch(ctx, m, q, req.WorkerID, req.Prefix28, req.RequestedBatchSize)
		if err != nil {
			http.Error(w, "failed to create and lease batch", http.StatusInternalServerError)
			return
		}
	}

	// Build response
	type resp struct {
		JobID         int64   `json:"job_id"`
		Prefix28      string  `json:"prefix_28"`
		NonceStart    int64   `json:"nonce_start"`
		NonceEnd      int64   `json:"nonce_end"`
		TargetAddress string  `json:"target_address"`
		CurrentNonce  *int64  `json:"current_nonce,omitempty"`
		ExpiresAt     *string `json:"expires_at,omitempty"`
	}

	var cur *int64
	if job.CurrentNonce.Valid {
		v := job.CurrentNonce.Int64
		cur = &v
	}
	var exp *string
	if job.ExpiresAt.Valid {
		t := job.ExpiresAt.Time.UTC().Format(time.RFC3339)
		exp = &t
	}

	out := resp{
		JobID:         job.ID,
		Prefix28:      base64.StdEncoding.EncodeToString(job.Prefix28),
		NonceStart:    job.NonceStart,
		NonceEnd:      job.NonceEnd,
		TargetAddress: s.cfg.TargetAddress,
		CurrentNonce:  cur,
		ExpiresAt:     exp,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

// createAndLeaseBatch encapsulates the logic to create a new batch for the
// given prefix (optionally provided as base64) and lease it to workerID.
func (s *Server) createAndLeaseBatch(ctx context.Context, m *jobs.Manager, q *database.Queries, workerID string, prefixOpt *string, batchSize uint32) (*database.Job, error) {
	var prefix28 []byte

	// If client provided a prefix, validate and use it.
	if prefixOpt != nil {
		decoded, err := base64.StdEncoding.DecodeString(*prefixOpt)
		if err != nil {
			return nil, fmt.Errorf("invalid base64 prefix_28: %w", err)
		}
		if len(decoded) != 28 {
			return nil, fmt.Errorf("prefix_28 must decode to 28 bytes")
		}
		prefix28 = decoded
	}

	// Helper: attempt to find a worker-specific prefix with remaining nonces.
	getWorkerAvailablePrefix := func() []byte {
		last, err := q.GetWorkerLastPrefix(ctx, sql.NullString{String: workerID, Valid: true})
		if err != nil || last.HighestNonce == nil {
			return nil
		}
		// Safely convert the returned highest_nonce to uint64 only if non-negative
		var highest uint64
		switch v := last.HighestNonce.(type) {
		case int64:
			if v >= 0 {
				highest = uint64(v)
			}
		case int32:
			if v >= 0 {
				highest = uint64(v)
			}
		case int:
			if v >= 0 {
				highest = uint64(v)
			}
		case float64:
			if v >= 0 {
				highest = uint64(v)
			}
		default:
			return nil
		}
		if highest < math.MaxUint32 {
			return last.Prefix28
		}
		return nil
	}

	if prefix28 == nil {
		prefix28 = getWorkerAvailablePrefix()
	}

	// If still no prefix, generate a new random one.
	if prefix28 == nil {
		prefix28 = make([]byte, 28)
		if _, err := rand.Read(prefix28); err != nil {
			return nil, fmt.Errorf("failed to generate prefix: %w", err)
		}
	}

	created, err := m.CreateBatch(ctx, prefix28, batchSize)
	if err != nil {
		return nil, fmt.Errorf("create batch: %w", err)
	}

	leaseSeconds := int64(leaseDuration.Seconds())
	lb := database.LeaseBatchParams{
		WorkerID:     sql.NullString{String: workerID, Valid: true},
		WorkerType:   sql.NullString{Valid: false},
		LeaseSeconds: sql.NullString{String: fmt.Sprintf("%d", leaseSeconds), Valid: true},
		ID:           created.ID,
	}
	if _, err := q.LeaseBatch(ctx, lb); err != nil {
		return nil, fmt.Errorf("lease created batch: %w", err)
	}
	updated, err := q.GetJobByID(ctx, created.ID)
	if err != nil {
		return nil, fmt.Errorf("get job by id: %w", err)
	}
	return &updated, nil
}
