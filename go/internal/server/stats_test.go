package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/garnizeh/eth-scanner/internal/config"
	"github.com/garnizeh/eth-scanner/internal/database"
)

func TestHandleStats(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "stats.db")

	ctx := context.Background()
	db, err := database.InitDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	s, err := New(&config.Config{}, db)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	s.RegisterRoutes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var body struct {
		TotalJobs        int64            `json:"total_jobs"`
		JobsByStatus     map[string]int64 `json:"jobs_by_status"`
		TotalKeysScanned int64            `json:"total_keys_scanned"`
		ActiveWorkers    int64            `json:"active_workers"`
		ResultsFound     int64            `json:"results_found"`
		Timestamp        string           `json:"timestamp"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode stats response: %v", err)
	}

	// Expect zeroed counters on a fresh DB
	if body.TotalJobs != 0 {
		t.Fatalf("expected total_jobs 0, got %d", body.TotalJobs)
	}
	if body.TotalKeysScanned != 0 {
		t.Fatalf("expected total_keys_scanned 0, got %d", body.TotalKeysScanned)
	}
	if body.ResultsFound != 0 {
		t.Fatalf("expected results_found 0, got %d", body.ResultsFound)
	}

	// Timestamp should parse and be UTC
	ts, err := time.Parse(time.RFC3339, body.Timestamp)
	if err != nil {
		t.Fatalf("timestamp not RFC3339: %v", err)
	}
	if ts.Location() != time.UTC {
		t.Fatalf("timestamp not in UTC: %v (loc=%v)", ts, ts.Location())
	}
}

func TestHandleStats_NoDB(t *testing.T) {
	s, err := New(&config.Config{}, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	s.RegisterRoutes()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when DB is nil, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "database not configured") {
		t.Fatalf("expected error message about database not configured, got %q", rr.Body.String())
	}
}
