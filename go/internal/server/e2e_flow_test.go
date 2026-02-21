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

// TestE2EFlow performs a full sequence: Lease → Checkpoint → Complete → Submit Result.
func TestE2EFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Initializing server
	lc := &net.ListenConfig{}
	l, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "e2e_flow.db")

	cfg := &config.Config{
		Port:     fmt.Sprintf("%d", port),
		DBPath:   dbPath,
		LogLevel: "debug",
	}

	db, err := database.InitDB(ctx, cfg.DBPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer func() { _ = database.CloseDB(db) }()

	srv := New(cfg, db)
	srv.RegisterRoutes()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	client := &http.Client{Timeout: 5 * time.Second}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// 1. Wait for server to be healthy
	ok := false
	for range 20 {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
		//nolint:gosec // false positive
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			ok = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ok {
		t.Fatalf("server did not become healthy in time")
	}

	workerID := "e2e-worker-1"

	// 2. Lease a job
	leaseReq := map[string]any{
		"worker_id":            workerID,
		"requested_batch_size": 1000,
	}
	body, _ := json.Marshal(leaseReq)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/jobs/lease", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	//nolint:gosec // false positive
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("lease request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("lease request returned status %d", resp.StatusCode)
	}

	var leaseResp struct {
		JobID      int64  `json:"job_id"`
		Prefix28   string `json:"prefix_28"`
		NonceStart int64  `json:"nonce_start"`
		NonceEnd   int64  `json:"nonce_end"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&leaseResp); err != nil {
		resp.Body.Close()
		t.Fatalf("failed to decode lease response: %v", err)
	}
	resp.Body.Close()

	if leaseResp.JobID == 0 {
		t.Fatal("lease response returned job_id 0")
	}

	// 3. Update Checkpoint
	checkpointReq := map[string]any{
		"worker_id":     workerID,
		"current_nonce": leaseResp.NonceStart + 500,
		"keys_scanned":  500,
		"started_at":    time.Now().UTC().Format(time.RFC3339),
		"duration_ms":   1000,
	}
	body, _ = json.Marshal(checkpointReq)
	checkpointURL := fmt.Sprintf("%s/api/v1/jobs/%d/checkpoint", baseURL, leaseResp.JobID)
	req, _ = http.NewRequestWithContext(ctx, http.MethodPatch, checkpointURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	//nolint:gosec // base URL is local and trusted
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("checkpoint request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("checkpoint request returned status %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 4. Submit Result
	resultReq := map[string]any{
		"worker_id":   workerID,
		"job_id":      leaseResp.JobID,
		"private_key": "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20",
		"address":     "0x0123456789012345678901234567890123456789",
		"nonce":       leaseResp.NonceStart + 123,
	}
	body, _ = json.Marshal(resultReq)
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/results", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	//nolint:gosec // base URL is local and trusted
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("submit result request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("submit result request returned status %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 5. Complete Job
	completeReq := map[string]any{
		"worker_id":    workerID,
		"final_nonce":  leaseResp.NonceEnd,
		"keys_scanned": 1000,
		"started_at":   time.Now().UTC().Format(time.RFC3339),
		"duration_ms":  2000,
	}
	body, _ = json.Marshal(completeReq)
	completeURL := fmt.Sprintf("%s/api/v1/jobs/%d/complete", baseURL, leaseResp.JobID)
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, completeURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	//nolint:gosec // base URL is local and trusted
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("complete request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("complete request returned status %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 6. Final DB Verification
	q := database.NewQueries(db)
	job, err := q.GetJobByID(ctx, leaseResp.JobID)
	if err != nil {
		t.Fatalf("failed to fetch job from DB: %v", err)
	}
	if job.Status != "completed" {
		t.Errorf("expected job status 'completed', got %q", job.Status)
	}

	results, err := q.GetAllResults(ctx, 10)
	if err != nil {
		t.Fatalf("failed to fetch results from DB: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result in DB, got %d", len(results))
	} else if results[0].PrivateKey != "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20" {
		t.Errorf("result private key mismatch")
	}
}
