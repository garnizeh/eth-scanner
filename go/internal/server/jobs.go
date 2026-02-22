package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/garnizeh/eth-scanner/internal/database"
	"github.com/garnizeh/eth-scanner/internal/jobs"
)

const (
	// maxBatchSize is a conservative upper bound for requested batch sizes.
	// We allow up to 4 billion keys to accommodate fast PC workers (1 hour @ 1M keys/sec).
	maxBatchSize  = 4_000_000_000
	leaseDuration = time.Hour
)

// handleJobLease handles POST /api/v1/jobs/lease
// Request JSON: {"worker_id":"...","requested_batch_size":12345, "prefix_28":"base64..."}
func (s *Server) handleJobLease(w http.ResponseWriter, r *http.Request) {
	type reqBody struct {
		WorkerID           string  `json:"worker_id"`
		WorkerType         string  `json:"worker_type,omitempty"`
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

	var job *database.Job
	var err error

	// If Win Scenario is active, we ensure the "win job" (zero prefix, nonce 1)
	// exists and is available for this worker. This works by resetting any
	// existing job for the zero prefix/nonce 0 range and clearing siblings.
	if s.cfg.WinScenario {
		log.Printf("[WIN-SCENARIO] Forcing Win job for worker %s", req.WorkerID)
		zeroPrefix := make([]byte, 28)
		// 1. Delete all other jobs for this prefix to avoid "running away" nonces.
		if err := q.ResetWinScenarioPrefix(ctx, zeroPrefix); err != nil {
			log.Printf("[WIN-SCENARIO] error resetting win prefix: %v", err)
		}
		// 2. Reset the main job [0, 99] to pending so it can be re-leased.
		if err := q.ResetWinScenarioJob(ctx, zeroPrefix); err != nil {
			log.Printf("[WIN-SCENARIO] error resetting win job: %v", err)
		}
	}

	// Try to lease an existing available job first (pass worker type so the
	// database record can be annotated).
	job, err = m.LeaseExistingJob(ctx, req.WorkerID, req.WorkerType)
	if err != nil {
		http.Error(w, "failed to lease existing job", http.StatusInternalServerError)
		return
	}

	// If none available (or forced by win-scenario if first time), create and lease a new batch
	if job == nil {
		job, err = s.createAndLeaseBatch(ctx, m, q, req.WorkerID, req.WorkerType, req.Prefix28, req.RequestedBatchSize)
		if err != nil {
			http.Error(w, "failed to create and lease batch", http.StatusInternalServerError)
			return
		}
	}

	// Always heartbeat the worker if a type is provided
	// This ensures the dashboard sees the worker as active.
	if req.WorkerType != "" {
		_ = q.UpsertWorker(ctx, database.UpsertWorkerParams{
			ID:         req.WorkerID,
			WorkerType: req.WorkerType,
			Metadata:   sql.NullString{Valid: false},
		})
	}

	// Build response
	type resp struct {
		JobID           int64    `json:"job_id"`
		Prefix28        string   `json:"prefix_28"`
		NonceStart      int64    `json:"nonce_start"`
		NonceEnd        int64    `json:"nonce_end"`
		TargetAddresses []string `json:"target_addresses"`
		CurrentNonce    *int64   `json:"current_nonce,omitempty"`
		ExpiresAt       *string  `json:"expires_at,omitempty"`
	}

	targets := s.cfg.TargetAddresses
	if s.cfg.WinScenario {
		// Ensure the winner address is in the targets list for this job
		winAddr := "0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf"
		found := false
		for _, a := range targets {
			if strings.EqualFold(a, winAddr) {
				found = true
				break
			}
		}
		if !found {
			targets = append([]string{winAddr}, targets...)
		}
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
		JobID:           job.ID,
		Prefix28:        base64.StdEncoding.EncodeToString(job.Prefix28),
		NonceStart:      job.NonceStart,
		NonceEnd:        job.NonceEnd,
		TargetAddresses: targets,
		CurrentNonce:    cur,
		ExpiresAt:       exp,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}

// createAndLeaseBatch encapsulates the logic to create a new batch for the
// given prefix (optionally provided as base64) and lease it to workerID.
func (s *Server) createAndLeaseBatch(ctx context.Context, m *jobs.Manager, q *database.Queries, workerID, workerType string, prefixOpt *string, batchSize uint32) (*database.Job, error) {
	var prefix28 []byte

	// Win Scenario override: always use 28 bytes of zeros and small nonce range
	if s.cfg.WinScenario {
		prefix28 = make([]byte, 28) // zeros
		batchSize = 100             // Ensure it doesn't take long to find (0-100 contains nonce 1)
		log.Printf("[WIN-SCENARIO] Forcing zero-prefix and small batch for worker %s", workerID)
	} else if prefixOpt != nil {
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
		if s.cfg.WinScenario {
			return nil // Don't use worker's last prefix in win mode
		}
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
	var created *database.Job
	var createErr error
	// Retry on transient constraint violations (concurrent allocs) a few times
	for attempt := range 3 {
		// If no prefix, generate a new random one.
		if prefix28 == nil {
			prefix28 = make([]byte, 28)
			if _, err := rand.Read(prefix28); err != nil {
				return nil, fmt.Errorf("failed to generate prefix: %w", err)
			}
		}

		created, createErr = m.CreateBatch(ctx, prefix28, batchSize)
		if createErr == nil {
			break
		}

		log.Printf("create batch attempt %d failed: %v", attempt+1, createErr)

		// If prefix is exhausted, don't retry with same prefix; switch to random immediately
		if errors.Is(createErr, jobs.ErrPrefixExhausted) {
			prefix28 = nil
			continue
		}

		// If error looks like a constraint/unique conflict, retry after a tiny backoff
		if strings.Contains(createErr.Error(), "UNIQUE constraint") || strings.Contains(createErr.Error(), "constraint failed") {
			time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
			continue
		}
		// Non-retriable error
		return nil, fmt.Errorf("create batch: %w", createErr)
	}
	if createErr != nil {
		return nil, fmt.Errorf("create batch: %w", createErr)
	}

	leaseSeconds := int64(leaseDuration.Seconds())
	lb := database.LeaseBatchParams{
		WorkerID:     sql.NullString{String: workerID, Valid: true},
		WorkerType:   sql.NullString{String: workerType, Valid: workerType != ""},
		LeaseSeconds: sql.NullString{String: fmt.Sprintf("%d", leaseSeconds), Valid: true},
		ID:           created.ID,
	}
	if _, err := q.LeaseBatch(ctx, lb); err != nil {
		log.Printf("lease created batch failed: %v", err)
		return nil, fmt.Errorf("lease created batch: %w", err)
	}
	updated, err := q.GetJobByID(ctx, created.ID)
	if err != nil {
		log.Printf("get job by id failed after create: %v", err)
		return nil, fmt.Errorf("get job by id: %w", err)
	}
	return &updated, nil
}
