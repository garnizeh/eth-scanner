package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/garnizeh/eth-scanner/internal/database"
)

// These tests use setupServerWithDB (file-backed DB using t.TempDir()).

func TestJobComplete_Success(t *testing.T) {
	s, db := setupServerWithDB(t)
	ctx := context.Background()

	prefix := make([]byte, 28)
	// insert processing job with nonce_end 999
	res, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, current_nonce, requested_batch_size) VALUES (?, ?, ?, 'processing', ?, ?, ?)`, prefix, 0, 999, "worker-1", 0, 1000)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	id, _ := res.LastInsertId()

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	req := map[string]any{"worker_id": "worker-1", "final_nonce": 999, "keys_scanned": 100}
	b, _ := json.Marshal(req)
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL+"/api/v1/jobs/"+strconv.FormatInt(id, 10)+"/complete", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(r)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	q := database.NewQueries(db)
	job, err := q.GetJobByID(ctx, id)
	if err != nil {
		t.Fatalf("GetJobByID: %v", err)
	}
	if job.Status != "completed" {
		t.Fatalf("expected status completed, got %s", job.Status)
	}
	if !job.CompletedAt.Valid {
		t.Fatalf("expected completed_at set")
	}
	if job.CompletedAt.Time.Location() != time.UTC {
		t.Fatalf("completed_at not UTC: %v", job.CompletedAt.Time.Location())
	}
	if !job.CurrentNonce.Valid || job.CurrentNonce.Int64 != job.NonceEnd {
		t.Fatalf("expected current_nonce == nonce_end, got %+v (nonce_end=%d)", job.CurrentNonce, job.NonceEnd)
	}
}

func TestJobComplete_WorkerMismatch(t *testing.T) {
	s, db := setupServerWithDB(t)
	ctx := context.Background()
	prefix := make([]byte, 28)
	res, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, current_nonce, requested_batch_size) VALUES (?, ?, ?, 'processing', ?, ?, ?)`, prefix, 0, 999, "worker-1", 0, 1000)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	id, _ := res.LastInsertId()

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	req := map[string]any{"worker_id": "other", "final_nonce": 999, "keys_scanned": 100}
	b, _ := json.Marshal(req)
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL+"/api/v1/jobs/"+strconv.FormatInt(id, 10)+"/complete", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(r)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", resp.StatusCode)
	}
}

func TestJobComplete_FinalNonceMismatch(t *testing.T) {
	s, db := setupServerWithDB(t)
	ctx := context.Background()
	prefix := make([]byte, 28)
	res, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, current_nonce, requested_batch_size) VALUES (?, ?, ?, 'processing', ?, ?, ?)`, prefix, 0, 999, "worker-1", 0, 1000)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	id, _ := res.LastInsertId()

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	req := map[string]any{"worker_id": "worker-1", "final_nonce": 998, "keys_scanned": 100}
	b, _ := json.Marshal(req)
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL+"/api/v1/jobs/"+strconv.FormatInt(id, 10)+"/complete", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(r)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
}

func TestJobComplete_NotFound(t *testing.T) {
	s, _ := setupServerWithDB(t)
	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	r, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/api/v1/jobs/99999/complete", bytes.NewReader([]byte(`{"worker_id":"x","final_nonce":1,"keys_scanned":1}`)))
	r.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(r)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", resp.StatusCode)
	}
}

func TestJobComplete_AlreadyCompletedAndPending(t *testing.T) {
	s, db := setupServerWithDB(t)
	ctx := context.Background()
	prefix := make([]byte, 28)

	// already completed
	res1, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, completed_at, requested_batch_size) VALUES (?, ?, ?, 'completed', ?, datetime('now','utc'), ?)`, prefix, 0, 999, "worker-1", 1000)
	if err != nil {
		t.Fatalf("insert completed job: %v", err)
	}
	id1, _ := res1.LastInsertId()

	// pending job (not assigned)
	res2, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, requested_batch_size) VALUES (?, ?, ?, 'pending', ?)`, prefix, 1000, 1999, 1000)
	if err != nil {
		t.Fatalf("insert pending job: %v", err)
	}
	id2, _ := res2.LastInsertId()

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	// attempt to complete already completed
	req1 := map[string]any{"worker_id": "worker-1", "final_nonce": 999, "keys_scanned": 100}
	b1, _ := json.Marshal(req1)
	r1, _ := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL+"/api/v1/jobs/"+strconv.FormatInt(id1, 10)+"/complete", bytes.NewReader(b1))
	r1.Header.Set("Content-Type", "application/json")
	resp1, err := client.Do(r1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for already completed job, got %d", resp1.StatusCode)
	}

	// attempt to complete pending job
	req2 := map[string]any{"worker_id": "worker-2", "final_nonce": 1999, "keys_scanned": 100}
	b2, _ := json.Marshal(req2)
	r2, _ := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL+"/api/v1/jobs/"+strconv.FormatInt(id2, 10)+"/complete", bytes.NewReader(b2))
	r2.Header.Set("Content-Type", "application/json")
	resp2, err := client.Do(r2)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for pending job completion, got %d", resp2.StatusCode)
	}
}
