package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"database/sql"

	"github.com/garnizeh/eth-scanner/internal/config"
	"github.com/garnizeh/eth-scanner/internal/database"
)

// helper to init DB and server
func setupServerWithDB(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	ctx := context.Background()
	db, err := database.InitDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	s := New(&config.Config{}, db)
	s.RegisterRoutes()
	t.Cleanup(func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("failed to close DB: %v", err)
		}
	})
	return s, db
}

func postLease(t *testing.T, serverURL string, body any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, serverURL+"/api/v1/jobs/lease", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("post lease failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestLeasePendingJob(t *testing.T) {
	s, db := setupServerWithDB(t)

	// insert a pending job
	prefix := make([]byte, 28)
	ctx := context.Background()
	_, err := db.ExecContext(ctx, "INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, created_at) VALUES (?, ?, ?, 'pending', datetime('now','utc'))", prefix, 0, 100)
	if err != nil {
		t.Fatalf("failed to insert pending job: %v", err)
	}

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	httpStatus, out := postLease(t, ts.URL, map[string]any{"worker_id": "worker-1", "requested_batch_size": 10})
	if httpStatus != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%v", httpStatus, out)
	}
	// verify returned job fields
	if out["job_id"] == nil {
		t.Fatalf("expected job_id in response")
	}
	if out["expires_at"] == nil {
		t.Fatalf("expected expires_at in response")
	}

	// verify job in DB is now processing and assigned
	var status string
	var workerID string
	row := db.QueryRowContext(ctx, "SELECT status, worker_id FROM jobs LIMIT 1")
	if err := row.Scan(&status, &workerID); err != nil {
		t.Fatalf("query job failed: %v", err)
	}
	if status != "processing" {
		t.Fatalf("expected job status processing, got %s", status)
	}
	if workerID != "worker-1" {
		t.Fatalf("expected worker_id worker-1, got %s", workerID)
	}
}

func TestLeaseExpiredJob_Reassigned(t *testing.T) {
	s, db := setupServerWithDB(t)

	prefix := make([]byte, 28)
	ctx := context.Background()
	// insert processing job with expired lease
	_, err := db.ExecContext(ctx, "INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, expires_at, created_at) VALUES (?, ?, ?, 'processing', ?, datetime('now','utc','-1 hour'), datetime('now','utc'))", prefix, 0, 100, "old-worker")
	if err != nil {
		t.Fatalf("failed to insert expired job: %v", err)
	}

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	httpStatus, out := postLease(t, ts.URL, map[string]any{"worker_id": "new-worker", "requested_batch_size": 10})
	if httpStatus != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%v", httpStatus, out)
	}

	// verify DB worker_id updated
	var workerID string
	row := db.QueryRowContext(ctx, "SELECT worker_id FROM jobs LIMIT 1")
	if err := row.Scan(&workerID); err != nil {
		t.Fatalf("query job failed: %v", err)
	}
	if workerID != "new-worker" {
		t.Fatalf("expected worker_id new-worker, got %s", workerID)
	}
}

func TestNoJobsCreatesNewBatch(t *testing.T) {
	s, db := setupServerWithDB(t)

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	// supply a known prefix (base64)
	prefix := make([]byte, 28)
	prefixB64 := base64.StdEncoding.EncodeToString(prefix)

	httpStatus, out := postLease(t, ts.URL, map[string]any{"worker_id": "worker-x", "requested_batch_size": 5, "prefix_28": prefixB64})
	if httpStatus != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%v", httpStatus, out)
	}
	if out["prefix_28"] != prefixB64 {
		t.Fatalf("expected prefix_28 %s, got %v", prefixB64, out["prefix_28"])
	}

	// ensure job persisted
	ctx := context.Background()
	var count int
	row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count == 0 {
		t.Fatalf("expected at least one job created")
	}
}

func TestConcurrentLeaseRequests_NoDuplicates(t *testing.T) {
	s, db := setupServerWithDB(t)

	ctx := context.Background()
	// insert 5 pending jobs
	prefix := make([]byte, 28)
	for i := range 5 {
		if _, err := db.ExecContext(ctx, "INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, created_at) VALUES (?, ?, ?, 'pending', datetime('now','utc'))", prefix, i*100, (i+1)*100); err != nil {
			t.Fatalf("failed insert pending: %v", err)
		}
	}

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	var wg sync.WaitGroup
	mu := sync.Mutex{}
	ids := make(map[int64]struct{})
	errs := make([]error, 0)

	for i := range 5 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, out := postLease(t, ts.URL, map[string]any{"worker_id": fmt.Sprintf("w-%d", i), "requested_batch_size": 10})
			if jid, ok := out["job_id"].(float64); ok {
				mu.Lock()
				ids[int64(jid)] = struct{}{}
				mu.Unlock()
			} else {
				mu.Lock()
				errs = append(errs, nil)
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if len(ids) != 5 {
		t.Fatalf("expected 5 unique job assignments, got %d, errs=%v", len(ids), errs)
	}
}

func TestLeaseRequestValidation(t *testing.T) {
	s, _ := setupServerWithDB(t)

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	// missing worker_id
	status1, _ := postLease(t, ts.URL, map[string]any{"requested_batch_size": 10})
	if status1 != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing worker_id, got %d", status1)
	}

	// invalid batch size (0)
	status2, _ := postLease(t, ts.URL, map[string]any{"worker_id": "x", "requested_batch_size": 0})
	if status2 != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid batch size, got %d", status2)
	}
}

func TestWorkerPrefixAffinity(t *testing.T) {
	s, _ := setupServerWithDB(t)

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	// first lease for worker-a
	status1, out1 := postLease(t, ts.URL, map[string]any{"worker_id": "worker-a", "requested_batch_size": 100})
	if status1 != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%v", status1, out1)
	}
	prefix1, ok := out1["prefix_28"].(string)
	if !ok {
		t.Fatalf("expected prefix_28 string in resp1")
	}
	end1f, ok := out1["nonce_end"].(float64)
	if !ok {
		t.Fatalf("expected nonce_end in resp1")
	}
	end1 := int64(end1f)

	// second lease for same worker should continue same prefix
	status2, out2 := postLease(t, ts.URL, map[string]any{"worker_id": "worker-a", "requested_batch_size": 100})
	if status2 != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%v", status2, out2)
	}
	prefix2, ok := out2["prefix_28"].(string)
	if !ok {
		t.Fatalf("expected prefix_28 string in resp2")
	}
	start2f, ok := out2["nonce_start"].(float64)
	if !ok {
		t.Fatalf("expected nonce_start in resp2")
	}
	start2 := int64(start2f)

	if prefix1 != prefix2 {
		t.Fatalf("expected same prefix for same worker, got %s and %s", prefix1, prefix2)
	}
	if start2 != end1+1 {
		t.Fatalf("expected second lease start = first end + 1 (%d), got %d", end1+1, start2)
	}

	// another worker should get a different prefix
	status3, out3 := postLease(t, ts.URL, map[string]any{"worker_id": "worker-b", "requested_batch_size": 100})
	if status3 != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%v", status3, out3)
	}
	prefix3, ok := out3["prefix_28"].(string)
	if !ok {
		t.Fatalf("expected prefix_28 string in resp3")
	}
	if prefix3 == prefix1 {
		t.Fatalf("expected different prefix for different worker, both got %s", prefix3)
	}
}
