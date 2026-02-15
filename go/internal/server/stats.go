package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/garnizeh/eth-scanner/internal/database"
)

// handleStats returns aggregated statistics for monitoring dashboards.
// GET /api/v1/stats
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.db == nil {
		http.Error(w, "database not configured", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	q := database.NewQueries(s.db)
	stats, err := q.GetStats(ctx)
	if err != nil {
		http.Error(w, "failed to query stats", http.StatusInternalServerError)
		return
	}

	// Normalize total keys scanned to int64
	var totalKeys int64
	switch v := stats.TotalKeysScanned.(type) {
	case int64:
		totalKeys = v
	case int:
		totalKeys = int64(v)
	case float64:
		totalKeys = int64(v)
	case nil:
		totalKeys = 0
	default:
		totalKeys = 0
	}

	resp := struct {
		TotalJobs        int64            `json:"total_jobs"`
		JobsByStatus     map[string]int64 `json:"jobs_by_status"`
		TotalKeysScanned int64            `json:"total_keys_scanned"`
		ActiveWorkers    int64            `json:"active_workers"`
		ResultsFound     int64            `json:"results_found"`
		Timestamp        string           `json:"timestamp"`
	}{
		TotalJobs: stats.TotalBatches,
		JobsByStatus: map[string]int64{
			"pending":    stats.PendingBatches,
			"processing": stats.ProcessingBatches,
			"completed":  stats.CompletedBatches,
		},
		TotalKeysScanned: totalKeys,
		ActiveWorkers:    stats.ActiveWorkers,
		ResultsFound:     stats.ResultsFound,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}
