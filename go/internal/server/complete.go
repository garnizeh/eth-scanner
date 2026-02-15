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

// handleJobComplete handles POST /api/v1/jobs/{id}/complete
// Request JSON: {"worker_id":"...","final_nonce":999,"keys_scanned":100}
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

	var req struct {
		WorkerID    string `json:"worker_id"`
		FinalNonce  int64  `json:"final_nonce"`
		KeysScanned int64  `json:"keys_scanned"`
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

	// Validate final nonce equals job's nonce_end (enforced here)
	if req.FinalNonce != job.NonceEnd {
		http.Error(w, "final_nonce does not match job nonce_end", http.StatusBadRequest)
		return
	}

	params := database.CompleteBatchParams{
		KeysScanned: sql.NullInt64{Int64: req.KeysScanned, Valid: true},
		ID:          id,
		WorkerID:    sql.NullString{String: req.WorkerID, Valid: true},
	}
	if err := q.CompleteBatch(ctx, params); err != nil {
		http.Error(w, "failed to complete job", http.StatusInternalServerError)
		return
	}

	updated, err := q.GetJobByID(ctx, id)
	if err != nil {
		http.Error(w, "failed to fetch updated job", http.StatusInternalServerError)
		return
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
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
