package server

import (
	"bytes"
	"context"
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

// E2E: single worker completes a full job with multiple checkpoints
func TestE2E_SingleWorker_MultipleCheckpoints(t *testing.T) {
	ctx := t.Context()
	lc := &net.ListenConfig{}
	l, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "e2e_single.db")

	cfg := &config.Config{
		Port:                     fmt.Sprintf("%d", port),
		DBPath:                   dbPath,
		LogLevel:                 "debug",
		StaleJobThresholdSeconds: 60,
		CleanupIntervalSeconds:   60,
		ShutdownTimeout:          3 * time.Second,
	}

	db, err := database.InitDB(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	srv := New(cfg, db)
	srv.RegisterRoutes()

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(runCtx) }()

	client := &http.Client{Timeout: 3 * time.Second}
	leaseURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/jobs/lease", port)

	// wait for health
	ok := false
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	for range 20 {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, healthURL, nil)
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			ok = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ok {
		cancel()
		t.Fatalf("server did not become healthy in time")
	}

	// Lease a job
	leaseReq := map[string]any{"worker_id": "single-worker", "requested_batch_size": 10}
	b, _ := json.Marshal(leaseReq)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, leaseURL, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("lease request failed: %v", err)
	}
	var out struct {
		JobID      int64 `json:"job_id"`
		NonceStart int64 `json:"nonce_start"`
		NonceEnd   int64 `json:"nonce_end"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		resp.Body.Close()
		t.Fatalf("decode lease response failed: %v", err)
	}
	resp.Body.Close()

	jobID := out.JobID

	// Send multiple checkpoints within the range
	for i := int64(0); i < 5; i++ {
		chkURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/jobs/%d/checkpoint", port, jobID)
		nonce := out.NonceStart + i
		chk := map[string]any{"worker_id": "single-worker", "current_nonce": nonce, "keys_scanned": 1, "started_at": time.Now().UTC().Format(time.RFC3339), "duration_ms": 1}
		cb, _ := json.Marshal(chk)
		r2, _ := http.NewRequestWithContext(context.Background(), http.MethodPatch, chkURL, bytes.NewReader(cb))
		r2.Header.Set("Content-Type", "application/json")
		resp2, err := client.Do(r2)
		if err != nil {
			t.Fatalf("checkpoint %d request failed: %v", i, err)
		}
		resp2.Body.Close()
		if resp2.StatusCode != http.StatusOK {
			t.Fatalf("checkpoint %d returned status %d", i, resp2.StatusCode)
		}
	}

	// Complete the job (use nonce_end as final_nonce)
	completeURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/jobs/%d/complete", port, jobID)
	compReq := map[string]any{"worker_id": "single-worker", "final_nonce": out.NonceEnd, "keys_scanned": 5, "started_at": time.Now().UTC().Format(time.RFC3339), "duration_ms": 10}
	cb2, _ := json.Marshal(compReq)
	r3, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, completeURL, bytes.NewReader(cb2))
	r3.Header.Set("Content-Type", "application/json")
	resp3, err := client.Do(r3)
	if err != nil {
		t.Fatalf("complete request failed: %v", err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("complete returned status %d", resp3.StatusCode)
	}

	// shutdown server
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
