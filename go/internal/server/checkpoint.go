package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/garnizeh/eth-scanner/internal/database"
)

// handleJobCheckpoint handles PATCH /api/v1/jobs/{id}/checkpoint
// Request JSON: {"worker_id":"...","current_nonce":1234,"keys_scanned":100, "started_at":"2024-01-01T12:00:00Z","duration_ms":5000}
func (s *Server) handleJobCheckpoint(w http.ResponseWriter, r *http.Request) {
	// Expect path like /api/v1/jobs/{id}/checkpoint
	// Trim prefix handled by ServeMux and parse remaining segments
	// Use path.Base and path.Dir to extract id and suffix
	p := r.URL.Path
	// get last element, should be "checkpoint"
	if path.Base(p) != "checkpoint" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	// remove trailing /checkpoint to get /api/v1/jobs/{id}
	parent := path.Dir(p)
	idStr := path.Base(parent)
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid job id", http.StatusBadRequest)
		return
	}

	type reqBody struct {
		WorkerID     string    `json:"worker_id"`
		CurrentNonce int64     `json:"current_nonce"`
		KeysScanned  int64     `json:"keys_scanned"`
		StartedAt    time.Time `json:"started_at"`
		DurationMs   int64     `json:"duration_ms"`
	}
	var req reqBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.WorkerID == "" {
		http.Error(w, "worker_id is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	q := database.NewQueries(s.db)

	// Always heartbeat even if the job doesn't exist
	// This helps with visibility when a worker is stuck in an old job after a master reset.
	if req.WorkerID != "" {
		_ = q.UpsertWorker(ctx, database.UpsertWorkerParams{
			ID:         req.WorkerID,
			WorkerType: "unknown", // can't accurately know type from body yet, but it beats 0
			Metadata:   sql.NullString{Valid: false},
		})
	}

	job, err := q.GetJobByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to fetch job", http.StatusInternalServerError)
		return
	}

	if job.Status != "processing" {
		http.Error(w, "job not processing", http.StatusBadRequest)
		return
	}
	if !job.WorkerID.Valid || job.WorkerID.String != req.WorkerID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Calculate deltas for worker_history before updating job state
	deltaKeys := req.KeysScanned - job.KeysScanned.Int64
	deltaDuration := req.DurationMs - job.DurationMs.Int64

	// Sanity check: if deltas are negative, fallback to full reported values
	if deltaKeys < 0 {
		deltaKeys = req.KeysScanned
	}
	if deltaDuration < 0 {
		deltaDuration = req.DurationMs
	}

	params := database.UpdateCheckpointParams{
		CurrentNonce: sql.NullInt64{Int64: req.CurrentNonce, Valid: true},
		KeysScanned:  sql.NullInt64{Int64: req.KeysScanned, Valid: true},
		DurationMs:   sql.NullInt64{Int64: req.DurationMs, Valid: true},
		ID:           id,
		WorkerID:     sql.NullString{String: req.WorkerID, Valid: true},
	}
	if err := q.UpdateCheckpoint(ctx, params); err != nil {
		http.Error(w, "failed to update checkpoint", http.StatusInternalServerError)
		return
	}

	updated, err := q.GetJobByID(ctx, id)
	if err != nil {
		http.Error(w, "failed to fetch updated job", http.StatusInternalServerError)
		return
	}

	// Register or heartbeat this worker in workers table
	if updated.WorkerType.Valid {
		_ = q.UpsertWorker(ctx, database.UpsertWorkerParams{
			ID:         req.WorkerID,
			WorkerType: updated.WorkerType.String,
			Metadata:   sql.NullString{Valid: false},
		})
	}

	type resp struct {
		JobID        int64   `json:"job_id"`
		CurrentNonce int64   `json:"current_nonce"`
		KeysScanned  int64   `json:"keys_scanned"`
		UpdatedAt    *string `json:"updated_at,omitempty"`
	}
	var up *string
	if updated.LastCheckpointAt.Valid {
		t := updated.LastCheckpointAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
		up = &t
	}
	out := resp{
		JobID:        updated.ID,
		CurrentNonce: updated.CurrentNonce.Int64,
		KeysScanned:  updated.KeysScanned.Int64,
		UpdatedAt:    up,
	}
	// Record worker history (best-effort; do not fail the request on error)
	go func(dk, dd int64) {
		// compute keys per second based on delta
		var kps float64
		if dd > 0 {
			kps = float64(dk) / (float64(dd) / 1000.0)
		}

		// choose batch size: prefers requested_batch_size if present
		var batchSize any
		if updated.RequestedBatchSize.Valid {
			batchSize = updated.RequestedBatchSize.Int64
		} else {
			batchSize = dk
		}

		ctx := context.Background()

		// Insert into worker_history (finished_at uses UTC now)
		_, err := s.db.ExecContext(ctx, `INSERT INTO worker_history (worker_id, worker_type, job_id, batch_size, keys_scanned, duration_ms, keys_per_second, prefix_28, nonce_start, nonce_end, finished_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now','utc'))`,
			req.WorkerID,
			updated.WorkerType.String,
			updated.ID,
			batchSize,
			dk, // delta keys
			dd, // delta duration
			kps,
			updated.Prefix28,
			updated.NonceStart,
			req.CurrentNonce,
		)
		if err != nil {
			log.Printf("WARNING: failed to record worker stats on checkpoint: %v", err)
		}
		// Trigger real-time broadcast of refreshed fleet stats
		s.broadcastStats(ctx)
	}(deltaKeys, deltaDuration)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
