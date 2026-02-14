package server

import (
	"encoding/json"
	"net/http"
	"time"
)

// handleHealth is a minimal health check handler returning JSON status and UTC timestamp.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	type resp struct {
		Status    string `json:"status"`
		Timestamp string `json:"timestamp"`
	}

	w.Header().Set("Content-Type", "application/json")
	rj := resp{Status: "ok", Timestamp: time.Now().UTC().Format(time.RFC3339)}
	// Best-effort: ignore encoding errors for health endpoint
	_ = json.NewEncoder(w).Encode(rj)
}
