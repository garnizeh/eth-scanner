package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/garnizeh/eth-scanner/internal/database"
)

// setupServerWithDB is defined in jobs_lease_test.go; reuse it to get a file-backed DB

// Test: Successful checkpoint update (current_nonce and keys_scanned updated)
func TestJobCheckpoint_Success(t *testing.T) {
	s, db := setupServerWithDB(t)
	ctx := context.Background()

	// insert processing job
	prefix := make([]byte, 28)
	res, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, current_nonce, requested_batch_size) VALUES (?, ?, ?, 'processing', ?, ?, ?)`, prefix, 0, 999, "worker-1", 0, 1000)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	id, _ := res.LastInsertId()

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	req := map[string]any{"worker_id": "worker-1", "current_nonce": 5, "keys_scanned": 5}
	b, _ := json.Marshal(req)
	r, _ := http.NewRequestWithContext(ctx, http.MethodPatch, ts.URL+"/api/v1/jobs/"+strconv.FormatInt(id, 10)+"/checkpoint", bytes.NewReader(b))
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
	var out struct {
		JobID        int64  `json:"job_id"`
		CurrentNonce int64  `json:"current_nonce"`
		KeysScanned  int64  `json:"keys_scanned"`
		UpdatedAt    string `json:"updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if out.JobID != id {
		t.Fatalf("unexpected job id: %d", out.JobID)
	}

	q := database.NewQueries(db)
	job, err := q.GetJobByID(ctx, id)
	if err != nil {
		t.Fatalf("GetJobByID: %v", err)
	}
	if !job.CurrentNonce.Valid || job.CurrentNonce.Int64 != 5 {
		t.Fatalf("expected current_nonce 5, got %+v", job.CurrentNonce)
	}
	if !job.KeysScanned.Valid || job.KeysScanned.Int64 != 5 {
		t.Fatalf("expected keys_scanned 5, got %+v", job.KeysScanned)
	}
	if !job.LastCheckpointAt.Valid {
		t.Fatalf("expected last_checkpoint_at to be set")
	}
	// parse updated_at from response and compare to DB value (UTC)
	if out.UpdatedAt == "" {
		t.Fatalf("expected updated_at in response")
	}
	parsed, err := time.Parse(time.RFC3339, out.UpdatedAt)
	if err != nil {
		t.Fatalf("failed to parse updated_at: %v", err)
	}
	if !job.LastCheckpointAt.Time.UTC().Equal(parsed.UTC()) {
		t.Fatalf("updated_at mismatch: db=%v resp=%v", job.LastCheckpointAt.Time.UTC(), parsed.UTC())
	}
}

// Test: Worker_id mismatch returns 403 Forbidden
func TestJobCheckpoint_WorkerMismatch(t *testing.T) {
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

	req := map[string]any{"worker_id": "other", "current_nonce": 5, "keys_scanned": 5}
	b, _ := json.Marshal(req)
	r, _ := http.NewRequestWithContext(ctx, http.MethodPatch, ts.URL+"/api/v1/jobs/"+strconv.FormatInt(id, 10)+"/checkpoint", bytes.NewReader(b))
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

// Test: Checkpointing non-existent job returns 404
func TestJobCheckpoint_NotFound(t *testing.T) {
	s, _ := setupServerWithDB(t)
	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	r, _ := http.NewRequestWithContext(context.Background(), http.MethodPatch, ts.URL+"/api/v1/jobs/99999/checkpoint", bytes.NewReader([]byte(`{"worker_id":"x","current_nonce":1,"keys_scanned":1}`)))
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

// Test: Checkpointing completed job returns 400 Bad Request
func TestJobCheckpoint_CompletedJob(t *testing.T) {
	s, db := setupServerWithDB(t)
	ctx := context.Background()
	prefix := make([]byte, 28)
	res, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, current_nonce, requested_batch_size, completed_at) VALUES (?, ?, ?, 'completed', ?, ?, ?, datetime('now','utc'))`, prefix, 0, 999, "worker-1", 0, 1000)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	id, _ := res.LastInsertId()

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	req := map[string]any{"worker_id": "worker-1", "current_nonce": 5, "keys_scanned": 5}
	b, _ := json.Marshal(req)
	r, _ := http.NewRequestWithContext(ctx, http.MethodPatch, ts.URL+"/api/v1/jobs/"+strconv.FormatInt(id, 10)+"/checkpoint", bytes.NewReader(b))
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

// Test: Concurrent checkpoints from same worker (last write wins)
func TestJobCheckpoint_Concurrent(t *testing.T) {
	s, db := setupServerWithDB(t)
	ctx := context.Background()
	prefix := make([]byte, 28)
	res, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, current_nonce, requested_batch_size) VALUES (?, ?, ?, 'processing', ?, ?, ?)`, prefix, 0, 99999, "worker-1", 0, 100000)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	id, _ := res.LastInsertId()

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	var wg sync.WaitGroup
	values := []int64{100, 200, 300, 400, 500}
	for _, v := range values {
		wg.Add(1)
		go func(v int64) {
			defer wg.Done()
			req := map[string]any{"worker_id": "worker-1", "current_nonce": v, "keys_scanned": v}
			b, _ := json.Marshal(req)
			r, _ := http.NewRequestWithContext(ctx, http.MethodPatch, ts.URL+"/api/v1/jobs/"+strconv.FormatInt(id, 10)+"/checkpoint", bytes.NewReader(b))
			r.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(r)
			if err != nil {
				t.Logf("concurrent req failed: %v", err)
				return
			}
			resp.Body.Close()
		}(v)
	}
	wg.Wait()

	q := database.NewQueries(db)
	job, err := q.GetJobByID(ctx, id)
	if err != nil {
		t.Fatalf("GetJobByID: %v", err)
	}
	if !job.CurrentNonce.Valid {
		t.Fatalf("expected current_nonce to be set")
	}
	found := false
	for _, v := range values {
		if job.CurrentNonce.Int64 == v {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("current_nonce not one of submitted values: %d", job.CurrentNonce.Int64)
	}
	if !job.LastCheckpointAt.Valid {
		t.Fatalf("expected last_checkpoint_at set")
	}
}

// Test: current_nonce cannot exceed nonce_end (validation)
func TestJobCheckpoint_CurrentNonceTooLarge(t *testing.T) {
	s, db := setupServerWithDB(t)
	ctx := context.Background()
	prefix := make([]byte, 28)
	// nonce_end = 100
	res, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, requested_batch_size) VALUES (?, ?, ?, 'processing', ?, ?)`, prefix, 0, 100, "worker-1", 100)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	id, _ := res.LastInsertId()

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	req := map[string]any{"worker_id": "worker-1", "current_nonce": 200, "keys_scanned": 200}
	b, _ := json.Marshal(req)
	client := &http.Client{Timeout: 5 * time.Second}
	r, _ := http.NewRequestWithContext(ctx, http.MethodPatch, ts.URL+"/api/v1/jobs/"+strconv.FormatInt(id, 10)+"/checkpoint", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(r)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Request should not succeed; either 4xx or 5xx
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("expected failure for oversized current_nonce, got 200 OK")
	}

	q := database.NewQueries(db)
	job, err := q.GetJobByID(ctx, id)
	if err != nil {
		t.Fatalf("GetJobByID: %v", err)
	}
	// Ensure DB did not accept invalid current_nonce
	if job.CurrentNonce.Valid && job.CurrentNonce.Int64 == 200 {
		t.Fatalf("database accepted invalid current_nonce > nonce_end")
	}
}
