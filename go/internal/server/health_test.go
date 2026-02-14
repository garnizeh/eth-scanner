package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/garnizeh/eth-scanner/internal/config"
)

func TestHandleHealth_File(t *testing.T) {
	s := New(&config.Config{}, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", nil)

	s.handleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var body struct {
		Status    string `json:"status"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("unexpected status: %q", body.Status)
	}
	ts, err := time.Parse(time.RFC3339, body.Timestamp)
	if err != nil {
		t.Fatalf("timestamp not RFC3339: %v", err)
	}
	if ts.Location() != time.UTC {
		t.Fatalf("timestamp not in UTC: %v (loc=%v)", ts, ts.Location())
	}
}
