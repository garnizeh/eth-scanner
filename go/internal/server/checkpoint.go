package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"strconv"

	"github.com/garnizeh/eth-scanner/internal/database"
)

// handleJobCheckpoint handles PATCH /api/v1/jobs/{id}/checkpoint
// Request JSON: {"worker_id":"...","current_nonce":1234,"keys_scanned":100}
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
		WorkerID     string `json:"worker_id"`
		CurrentNonce int64  `json:"current_nonce"`
		KeysScanned  int64  `json:"keys_scanned"`
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

	params := database.UpdateCheckpointParams{
		CurrentNonce: sql.NullInt64{Int64: req.CurrentNonce, Valid: true},
		KeysScanned:  sql.NullInt64{Int64: req.KeysScanned, Valid: true},
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
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
