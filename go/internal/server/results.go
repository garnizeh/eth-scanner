package server

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/garnizeh/eth-scanner/internal/database"
)

// handleResultSubmit handles POST /api/v1/results
// Request JSON: {"worker_id":"...","job_id":123,"private_key":"...","address":"0x...","nonce":123}
func (s *Server) handleResultSubmit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkerID   string `json:"worker_id"`
		JobID      int64  `json:"job_id"`
		PrivateKey string `json:"private_key"` //nolint:gosec // false positive: descriptive field name, not a hardcoded secret
		Address    string `json:"address"`
		Nonce      int64  `json:"nonce"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.WorkerID == "" {
		http.Error(w, "worker_id is required", http.StatusBadRequest)
		return
	}
	if req.JobID == 0 {
		http.Error(w, "job_id is required", http.StatusBadRequest)
		return
	}
	// validate private key: 64 hex chars
	if len(req.PrivateKey) != 64 {
		http.Error(w, "private_key must be 64 hex characters", http.StatusBadRequest)
		return
	}
	if _, err := hex.DecodeString(req.PrivateKey); err != nil {
		http.Error(w, "private_key must be valid hex", http.StatusBadRequest)
		return
	}
	// validate address: 0x + 40 hex chars
	if !strings.HasPrefix(req.Address, "0x") || len(req.Address) != 42 {
		http.Error(w, "address must be 0x-prefixed 40-hex chars", http.StatusBadRequest)
		return
	}
	if _, err := hex.DecodeString(req.Address[2:]); err != nil {
		http.Error(w, "address must be valid hex", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	q := database.NewQueries(s.db)

	// Heartbeat the worker on match submission
	if req.WorkerID != "" {
		_ = q.UpsertWorker(ctx, database.UpsertWorkerParams{
			ID:         req.WorkerID,
			WorkerType: "unknown", // refined if it exists in the workers table already
			Metadata:   sql.NullString{Valid: false},
		})
	}

	params := database.InsertResultParams{
		PrivateKey: req.PrivateKey,
		Address:    req.Address,
		WorkerID:   req.WorkerID,
		JobID:      req.JobID,
		NonceFound: req.Nonce,
	}
	res, err := q.InsertResult(ctx, params)
	if err != nil {
		http.Error(w, "failed to insert result", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(res)
}
