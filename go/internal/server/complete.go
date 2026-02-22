package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/garnizeh/eth-scanner/internal/database"
)

// handleJobComplete handles POST /api/v1/jobs/{id}/complete
// Request JSON: {"worker_id":"...","final_nonce":999,"keys_scanned":100, "started_at":"2024-01-01T12:00:00Z","duration_ms":5000}
func (s *Server) handleJobComplete(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if path.Base(p) != "complete" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	parent := path.Dir(p)
	idStr := path.Base(parent)
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid job id", http.StatusBadRequest)
		return
	}

	// Read and log raw body for debugging ESP32 payloads
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}
	// Restore body after reading
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var req struct {
		WorkerID    string    `json:"worker_id"`
		FinalNonce  int64     `json:"final_nonce"`
		KeysScanned int64     `json:"keys_scanned"`
		StartedAt   time.Time `json:"started_at"`
		DurationMs  int64     `json:"duration_ms"`
	}
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

	// Always heartbeat even if the job doesn't exist for better visibility.
	if req.WorkerID != "" {
		_ = q.UpsertWorker(ctx, database.UpsertWorkerParams{
			ID:         req.WorkerID,
			WorkerType: "unknown", // placeholder that will be refined if job is found
			Metadata:   sql.NullString{Valid: false},
		})
	}

	job, err := q.GetJobByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// #nosec G706: logging raw body for debugging, even on decode failure
			log.Printf("complete failed: job %d not found", id)
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		// #nosec G706: logging raw body for debugging, even on decode failure
		log.Printf("complete failed: failed to fetch job %d: %v", id, err)
		http.Error(w, "failed to fetch job", http.StatusInternalServerError)
		return
	}

	if job.Status != "processing" {
		// #nosec G706: logging raw body for debugging, even on decode failure
		log.Printf("complete failed: job %d status is %s, expected processing. Worker: %q", id, job.Status, req.WorkerID)
		http.Error(w, "job no longer active", http.StatusGone) // 410
		return
	}
	if !job.WorkerID.Valid || job.WorkerID.String != req.WorkerID {
		// #nosec G706: logging raw body for debugging, even on decode failure
		log.Printf("complete failed: job %d owned by %v, but complete from %q", id, job.WorkerID.String, req.WorkerID)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Calculate deltas and range for worker_history before final update
	deltaKeys := req.KeysScanned - job.KeysScanned.Int64
	deltaDuration := req.DurationMs - job.DurationMs.Int64

	// Nonce range for THIS final period (since last checkpoint)
	rangeStart := job.NonceStart
	if job.CurrentNonce.Valid && job.KeysScanned.Int64 > 0 {
		rangeStart = job.CurrentNonce.Int64 + 1
	}
	rangeEnd := req.FinalNonce

	// Sanity check: if deltas are negative, fallback to full reported values
	if deltaKeys < 0 {
		deltaKeys = req.KeysScanned
		rangeStart = job.NonceStart
	}
	if deltaDuration < 0 {
		deltaDuration = req.DurationMs
	}

	// Validate final nonce equals job's nonce_end (enforced here)
	if req.FinalNonce != job.NonceEnd {
		http.Error(w, "final_nonce does not match job nonce_end", http.StatusBadRequest)
		return
	}

	params := database.CompleteBatchParams{
		KeysScanned: sql.NullInt64{Int64: req.KeysScanned, Valid: true},
		DurationMs:  sql.NullInt64{Int64: req.DurationMs, Valid: true},
		ID:          id,
		WorkerID:    sql.NullString{String: req.WorkerID, Valid: true},
	}
	if err := q.CompleteBatch(ctx, params); err != nil {
		s.BroadcastEvent(req.WorkerID, "Completion Failure", fmt.Sprintf("Failed to complete job %d: %v", id, err), "error")
		http.Error(w, "failed to complete job", http.StatusInternalServerError)
		return
	}

	s.BroadcastEvent(req.WorkerID, "Job Done", fmt.Sprintf("Batch %d completed: scanned %d keys", id, req.KeysScanned), "success")

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
		JobID       int64   `json:"job_id"`
		Status      string  `json:"status"`
		FinalNonce  int64   `json:"final_nonce"`
		KeysScanned int64   `json:"keys_scanned"`
		CompletedAt *string `json:"completed_at,omitempty"`
	}
	var ca *string
	if updated.CompletedAt.Valid {
		t := updated.CompletedAt.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
		ca = &t
	}
	out := resp{
		JobID:       updated.ID,
		Status:      updated.Status,
		FinalNonce:  updated.CurrentNonce.Int64,
		KeysScanned: updated.KeysScanned.Int64,
		CompletedAt: ca,
	}
	// Record worker history asynchronously (best-effort)
	go func(dk, dd int64) {
		var kps float64
		if dd > 0 {
			kps = float64(dk) / (float64(dd) / 1000.0)
		}

		var batchSize any
		if updated.RequestedBatchSize.Valid {
			batchSize = updated.RequestedBatchSize.Int64
		} else {
			batchSize = dk
		}

		ctx := context.Background()
		_, err := s.db.ExecContext(ctx, `INSERT INTO worker_history (worker_id, worker_type, job_id, batch_size, keys_scanned, duration_ms, keys_per_second, prefix_28, nonce_start, nonce_end, finished_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now','utc'))`,
			req.WorkerID,
			updated.WorkerType.String,
			updated.ID,
			batchSize,
			dk, // delta keys
			dd, // delta duration
			kps,
			updated.Prefix28,
			rangeStart,
			rangeEnd,
		)
		if err != nil {
			log.Printf("WARNING: failed to record worker stats on complete: %v", err)
		}
		// Trigger real-time broadcast of refreshed fleet stats
		s.broadcastStats(ctx)
	}(deltaKeys, deltaDuration)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
