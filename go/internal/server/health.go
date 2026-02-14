package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// handleHealth returns service status and optional database connectivity info.
// - If the server has a non-nil DB, it will attempt a PingContext with a 2s timeout.
// - On DB error the handler returns HTTP 503 and status "error" with the error message.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	type resp struct {
		Status    string `json:"status"`
		Timestamp string `json:"timestamp"`
		Database  string `json:"database,omitempty"`
		Error     string `json:"error,omitempty"`
	}

	w.Header().Set("Content-Type", "application/json")

	out := resp{Status: "ok", Timestamp: time.Now().UTC().Format(time.RFC3339)}

	// If a DB is configured, perform a short ping to include connectivity state.
	if s.db != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.db.PingContext(ctx); err != nil {
			out.Status = "error"
			out.Database = "disconnected"
			out.Error = err.Error()
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(out)
			return
		}
		out.Database = "connected"
	}

	// If no DB is configured we omit the database field (optional check).
	if err := json.NewEncoder(w).Encode(out); err != nil {
		http.Error(w, "failed to encode health response", http.StatusInternalServerError)
	}
}
