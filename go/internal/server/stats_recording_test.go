package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckpoint_RecordsWorkerHistory(t *testing.T) {
	s, db, _ := setupServer(t)

	// insert a processing job
	prefix := make([]byte, 28)
	res, err := db.ExecContext(context.Background(), `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, worker_type, requested_batch_size) VALUES (?, ?, ?, 'processing', ?, ?, ?)`, prefix, 0, 999, "worker-1", "pc", 1000)
	if err != nil {
		t.Fatalf("insert job failed: %v", err)
	}
	jobID, _ := res.LastInsertId()

	// prepare checkpoint request
	now := time.Now().UTC()
	reqBody := map[string]any{
		"worker_id":     "worker-1",
		"current_nonce": 500,
		"keys_scanned":  500,
		"started_at":    now.Format(time.RFC3339),
		"duration_ms":   1000,
	}
	b, _ := json.Marshal(reqBody)

	r := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/jobs/%d/checkpoint", jobID), bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	// allow goroutine to insert
	time.Sleep(20 * time.Millisecond)

	var cnt int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM worker_history WHERE worker_id = ?", "worker-1").Scan(&cnt); err != nil {
		t.Fatalf("count worker_history failed: %v", err)
	}
	if cnt < 1 {
		t.Fatalf("expected at least 1 worker_history row, got %d", cnt)
	}
}

func TestComplete_RecordsWorkerHistory(t *testing.T) {
	s, db, _ := setupServer(t)

	// insert a processing job
	prefix := make([]byte, 28)
	res, err := db.ExecContext(context.Background(), `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, worker_type, requested_batch_size) VALUES (?, ?, ?, 'processing', ?, ?, ?)`, prefix, 0, 999, "worker-2", "pc", 2000)
	if err != nil {
		t.Fatalf("insert job failed: %v", err)
	}
	jobID, _ := res.LastInsertId()

	reqBody := map[string]any{
		"worker_id":    "worker-2",
		"final_nonce":  999,
		"keys_scanned": 2000,
		"started_at":   time.Now().UTC().Format(time.RFC3339),
		"duration_ms":  2000,
	}
	b, _ := json.Marshal(reqBody)

	r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/jobs/%d/complete", jobID), bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	// allow goroutine to insert
	time.Sleep(20 * time.Millisecond)

	var cnt int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM worker_history WHERE worker_id = ?", "worker-2").Scan(&cnt); err != nil {
		t.Fatalf("count worker_history failed: %v", err)
	}
	if cnt < 1 {
		t.Fatalf("expected at least 1 worker_history row, got %d", cnt)
	}
}
