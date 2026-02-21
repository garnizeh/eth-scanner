package server

import (
	"bytes"
	"context"
	"encoding/base64"
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

// TestESP32FullCycleSimulation mimics the behavior of the ESP32 firmware
// (using its exact JSON structures) to verify the Master API's compatibility.
func TestESP32FullCycleSimulation(t *testing.T) {
	ctx := t.Context()

	// 1. Setup Master API server
	lc := &net.ListenConfig{}
	l, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	addr := l.Addr().(*net.TCPAddr)
	port := addr.Port
	_ = l.Close()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "esp32_e2e.db")

	cfg := &config.Config{
		Port:                   fmt.Sprintf("%d", port),
		DBPath:                 dbPath,
		LogLevel:               "debug",
		CleanupIntervalSeconds: 1,
		ShutdownTimeout:        3 * time.Second,
	}

	db, err := database.InitDB(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	queries := database.New(db)

	// Add a job to the database so we have something to lease
	// status defaults to 'pending'
	_, err = db.ExecContext(ctx, "INSERT INTO jobs (prefix_28, nonce_start, nonce_end) VALUES (?, ?, ?)", make([]byte, 28), 0, 1000000)
	if err != nil {
		t.Fatalf("failed to seed job: %v", err)
	}

	srv := New(cfg, db)
	srv.RegisterRoutes()

	runCtx, cancelSrv := context.WithCancel(context.Background())
	defer cancelSrv()
	go func() { _ = srv.Start(runCtx) }()

	// Wait for server to be healthy
	apiURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	ok := false
	for range 20 {
		resp, err := http.Get(apiURL + "/health")
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

	workerID := "esp32-hw-sim-1"
	client := &http.Client{Timeout: 5 * time.Second}

	var jobID int64

	// 2. Lease Job (mimicking api_lease_job in api_client.c)
	t.Run("Lease", func(t *testing.T) {
		leaseReq := map[string]any{
			"worker_id":            workerID,
			"requested_batch_size": 50000,
		}
		body, _ := json.Marshal(leaseReq)
		resp, err := client.Post(apiURL+"/api/v1/jobs/lease", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("lease failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("lease failed with status %d", resp.StatusCode)
		}

		var leaseResp struct {
			JobID           int64    `json:"job_id"`
			Prefix28        string   `json:"prefix_28"`
			NonceStart      uint32   `json:"nonce_start"`
			NonceEnd        uint32   `json:"nonce_end"`
			TargetAddresses []string `json:"target_addresses"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&leaseResp); err != nil {
			t.Fatalf("failed to decode lease response: %v", err)
		}

		if leaseResp.JobID == 0 {
			t.Fatalf("expected non-zero job ID")
		}
		jobID = leaseResp.JobID

		prefixBytes, err := base64.StdEncoding.DecodeString(leaseResp.Prefix28)
		if err != nil || len(prefixBytes) != 28 {
			t.Fatalf("invalid prefix_28 format: %v (len %d)", err, len(prefixBytes))
		}
		t.Logf("Lease successful: JobID=%d, NonceRange=[%d, %d]", jobID, leaseResp.NonceStart, leaseResp.NonceEnd)
	})

	// 3. Checkpoint (mimicking api_checkpoint in api_client.c)
	t.Run("Checkpoint", func(t *testing.T) {
		checkpointReq := map[string]any{
			"worker_id":     workerID,
			"current_nonce": 1234,
			"keys_scanned":  1234,
			"duration_ms":   1000,
			// Note: StartedAt is omitted as per api_client.c:198
		}
		body, _ := json.Marshal(checkpointReq)
		req, _ := http.NewRequest(http.MethodPatch, fmt.Sprintf("%s/api/v1/jobs/%d/checkpoint", apiURL, jobID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("checkpoint failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("checkpoint failed with status %d", resp.StatusCode)
		}

		// Verify database was updated
		job, err := queries.GetJobByID(ctx, jobID)
		if err != nil {
			t.Fatalf("failed to query job: %v", err)
		}
		if !job.CurrentNonce.Valid || job.CurrentNonce.Int64 != 1234 {
			t.Errorf("expected current_nonce 1234, got %v", job.CurrentNonce)
		}
	})

	// 4. Submit Result (mimicking api_submit_result in api_client.c)
	t.Run("SubmitResult", func(t *testing.T) {
		resultReq := map[string]any{
			"worker_id":   workerID,
			"job_id":      jobID,
			"private_key": "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20",
			"address":     "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
		}
		body, _ := json.Marshal(resultReq)
		resp, err := client.Post(apiURL+"/api/v1/results", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("submit result failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("submit result failed with status %d", resp.StatusCode)
		}
	})

	// 5. Complete Job (mimicking api_complete in api_client.c)
	t.Run("Complete", func(t *testing.T) {
		completeReq := map[string]any{
			"worker_id":    workerID,
			"final_nonce":  1000000,
			"keys_scanned": 1000000,
			"duration_ms":  5000,
		}
		body, _ := json.Marshal(completeReq)
		resp, err := client.Post(fmt.Sprintf("%s/api/v1/jobs/%d/complete", apiURL, jobID), "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("complete failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("complete failed with status %d", resp.StatusCode)
		}

		// Verify database status
		job, err := queries.GetJobByID(ctx, jobID)
		if err != nil {
			t.Fatalf("failed to query job: %v", err)
		}
		if job.Status != "completed" {
			t.Errorf("expected job status completed, got %v", job.Status)
		}
	})
}
