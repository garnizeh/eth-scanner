package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/garnizeh/eth-scanner/internal/config"
	"github.com/garnizeh/eth-scanner/internal/database"
)

// TestCleanupAndReassign verifies that the background cleanup clears a stale
// job's worker_id and that both another worker and the same worker can lease
// the job afterwards via the HTTP lease endpoint.
func TestCleanupAndReassign(t *testing.T) {
	// don't run in parallel because we bind ports and use filesystem DB
	ctx := t.Context()

	// pick an available port
	lc := &net.ListenConfig{}
	l, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	addr := l.Addr().(*net.TCPAddr)
	port := addr.Port
	if err := l.Close(); err != nil {
		t.Fatalf("failed to close listener: %v", err)
	}

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "e2e_cleanup.db")

	cfg := &config.Config{
		Port:                     fmt.Sprintf("%d", port),
		DBPath:                   dbPath,
		LogLevel:                 "debug",
		StaleJobThresholdSeconds: 1, // 1s threshold for fast tests
		CleanupIntervalSeconds:   1, // run cleanup every 1s
		ShutdownTimeout:          3 * time.Second,
	}

	db, err := database.InitDB(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	srv := New(cfg, db)
	srv.RegisterRoutes()

	// start server
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(runCtx) }()

	// wait for /health
	client := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	ok := false
	for range 20 {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			ok = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ok {
		cancel()
		t.Fatalf("server did not become healthy in time")
	}

	q := database.NewQueries(db)

	insertStaleJob := func(workerID string, prefix []byte) int64 {
		// insert processing job with last_checkpoint_at 10s ago
		res, err := db.ExecContext(context.Background(), `
            INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, last_checkpoint_at, created_at)
            VALUES (?, ?, ?, 'processing', ?, datetime('now','-10 seconds'), datetime('now','utc'))
        `, prefix, 0, 1000, workerID)
		if err != nil {
			t.Fatalf("failed to insert stale job: %v", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("failed to get last insert id: %v", err)
		}
		return id
	}

	// Scenario 1: another worker should be able to lease cleared job
	prefix1 := make([]byte, 28)
	for i := range prefix1 {
		prefix1[i] = byte(i + 1)
	}
	job1 := insertStaleJob("worker-a", prefix1)

	// wait for cleanup to clear it (poll)
	var cleaned bool
	for range 30 {
		j, err := q.GetJobByID(context.Background(), job1)
		if err != nil {
			t.Fatalf("GetJobByID failed: %v", err)
		}
		if j.Status == "pending" && !j.WorkerID.Valid {
			cleaned = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !cleaned {
		t.Fatalf("job was not cleaned in time")
	}

	// Now simulate worker-b leasing via HTTP
	leaseURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/jobs/lease", port)
	leaseReq := map[string]any{
		"worker_id":            "worker-b",
		"requested_batch_size": 100,
	}
	b, _ := json.Marshal(leaseReq)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, leaseURL, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("lease request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("lease response status not OK: %d", resp.StatusCode)
	}
	var out struct {
		JobID int64 `json:"job_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("failed to decode lease response: %v", err)
	}
	if out.JobID != job1 {
		t.Fatalf("expected leased job id %d, got %d", job1, out.JobID)
	}

	// verify DB shows worker-b and processing
	leased, err := q.GetJobByID(context.Background(), job1)
	if err != nil {
		t.Fatalf("GetJobByID failed after lease: %v", err)
	}
	if leased.Status != "processing" {
		t.Fatalf("expected processing after lease, got %s", leased.Status)
	}
	if !leased.WorkerID.Valid || leased.WorkerID.String != "worker-b" {
		t.Fatalf("expected worker-b assigned after lease, got %v", leased.WorkerID)
	}

	// Simulate a checkpoint from worker-b so the job is not considered stale
	if err := q.UpdateCheckpoint(context.Background(), database.UpdateCheckpointParams{
		CurrentNonce: sql.NullInt64{Int64: leased.NonceStart, Valid: true},
		KeysScanned:  sql.NullInt64{Int64: 1, Valid: true},
		ID:           job1,
		WorkerID:     sql.NullString{String: "worker-b", Valid: true},
	}); err != nil {
		t.Fatalf("UpdateCheckpoint failed: %v", err)
	}

	// Scenario 2: same worker can also lease after cleanup
	prefix2 := make([]byte, 28)
	for i := range prefix2 {
		prefix2[i] = byte(i + 101)
	}
	job2 := insertStaleJob("worker-x", prefix2)

	// wait for cleanup
	cleaned = false
	for range 30 {
		j, err := q.GetJobByID(context.Background(), job2)
		if err != nil {
			t.Fatalf("GetJobByID failed: %v", err)
		}
		if j.Status == "pending" && !j.WorkerID.Valid {
			cleaned = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !cleaned {
		t.Fatalf("job2 was not cleaned in time")
	}

	// lease as same worker-x
	leaseReq2 := map[string]any{
		"worker_id":            "worker-x",
		"requested_batch_size": 100,
	}
	b2, _ := json.Marshal(leaseReq2)
	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, leaseURL, bytes.NewReader(b2))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("lease request 2 failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("lease response 2 status not OK: %d", resp2.StatusCode)
	}
	var out2 struct {
		JobID int64 `json:"job_id"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&out2); err != nil {
		t.Fatalf("failed to decode lease response 2: %v", err)
	}
	if out2.JobID != job2 {
		t.Fatalf("expected leased job id %d, got %d", job2, out2.JobID)
	}

	// cleanup: stop server
	cancel()
	select {
	case e := <-errCh:
		if e != nil {
			t.Logf("server returned: %v", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("server did not shutdown within timeout")
	}
}
