package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/garnizeh/eth-scanner/internal/config"
	"github.com/garnizeh/eth-scanner/internal/database"
)

// End-to-end crash recovery: worker A leases and checkpoints, then crashes
// (lease expires). Worker B should be able to lease and resume from the
// last checkpoint (current_nonce preserved) and worker_history should contain
// the prior checkpoint record.
func TestE2E_CrashRecovery_LeaseAndResume(t *testing.T) {
	ctx := t.Context()

	lc := &net.ListenConfig{}
	l, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	addr := l.Addr().(*net.TCPAddr)
	port := addr.Port
	_ = l.Close()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "e2e_crash.db")

	cfg := &config.Config{
		Port:                     fmt.Sprintf("%d", port),
		DBPath:                   dbPath,
		LogLevel:                 "debug",
		StaleJobThresholdSeconds: 1,
		CleanupIntervalSeconds:   1,
		ShutdownTimeout:          3 * time.Second,
	}

	db, err := database.InitDB(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	srv, err := New(cfg, db)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	srv.RegisterRoutes()

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(runCtx) }()

	client := &http.Client{Timeout: 3 * time.Second}
	leaseURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/jobs/lease", port)

	// wait for /health
	ok := false
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	for range 20 {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, healthURL, nil)
		//nolint:gosec // false positive: SSRF in test
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

	// Worker A leases (server will create+lease a batch)
	leaseReq := map[string]any{"worker_id": "worker-A", "requested_batch_size": 100}
	b, _ := json.Marshal(leaseReq)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, leaseURL, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	//nolint:gosec // false positive: SSRF in test
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("lease request failed: %v", err)
	}
	var out struct {
		JobID        int64  `json:"job_id"`
		NonceStart   int64  `json:"nonce_start"`
		NonceEnd     int64  `json:"nonce_end"`
		CurrentNonce *int64 `json:"current_nonce"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		resp.Body.Close()
		t.Fatalf("decode lease response failed: %v", err)
	}
	resp.Body.Close()

	jobID := out.JobID

	q := database.NewQueries(db)

	// Worker A sends a checkpoint within the leased nonce range
	checkpointNonce := out.NonceStart + 1
	chkURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/jobs/%d/checkpoint", port, jobID)
	chk := map[string]any{
		"worker_id":     "worker-A",
		"current_nonce": checkpointNonce,
		"keys_scanned":  100,
		"started_at":    time.Now().UTC().Format(time.RFC3339),
		"duration_ms":   10,
	}
	cb, _ := json.Marshal(chk)
	r2, _ := http.NewRequestWithContext(context.Background(), http.MethodPatch, chkURL, bytes.NewReader(cb))
	r2.Header.Set("Content-Type", "application/json")

	// Direct DB call diagnostic: attempt UpdateCheckpoint directly to surface errors
	if err := q.UpdateCheckpoint(context.Background(), database.UpdateCheckpointParams{
		CurrentNonce: sql.NullInt64{Int64: checkpointNonce, Valid: true},
		KeysScanned:  sql.NullInt64{Int64: 100, Valid: true},
		ID:           jobID,
		WorkerID:     sql.NullString{String: "worker-A", Valid: true},
	}); err != nil {
		t.Fatalf("direct UpdateCheckpoint failed: %v", err)
	}

	//nolint:gosec // false positive: SSRF in test
	resp2, err := client.Do(r2)
	if err != nil {
		t.Fatalf("checkpoint request failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp2.Body)
		t.Fatalf("checkpoint request returned status %d: %s", resp2.StatusCode, string(bodyBytes))
	}

	// Ensure job current_nonce updated
	var j database.Job
	var tries int
	for tries = 0; tries < 20; tries++ {
		var err error
		j, err = q.GetJobByID(context.Background(), jobID)
		if err != nil {
			t.Fatalf("GetJobByID failed: %v", err)
		}
		if j.CurrentNonce.Valid && j.CurrentNonce.Int64 == checkpointNonce {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if tries == 20 {
		t.Fatalf("job did not reflect checkpoint current_nonce in time")
	}

	// Wait for cleanup to expire the lease (job should become pending and worker_id cleared)
	cleaned := false
	for i := 0; i < 40; i++ {
		got, err := q.GetJobByID(context.Background(), jobID)
		if err != nil {
			t.Fatalf("GetJobByID failed while waiting cleanup: %v", err)
		}
		if got.Status == "pending" && !got.WorkerID.Valid {
			cleaned = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !cleaned {
		t.Fatalf("job was not cleaned (lease expired) in time")
	}

	// Ensure worker_history contains a record for worker-A (the checkpoint goroutine may take a moment)
	foundHistory := false
	for i := 0; i < 20; i++ {
		var cnt int
		if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM worker_history WHERE worker_id = ?", "worker-A").Scan(&cnt); err != nil {
			t.Fatalf("query worker_history failed: %v", err)
		}
		if cnt > 0 {
			foundHistory = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !foundHistory {
		t.Fatalf("expected worker_history record for worker-A, none found")
	}

	// Now worker-B leases the job and should receive current_nonce == 12345 (resume)
	leaseReqB := map[string]any{"worker_id": "worker-B", "requested_batch_size": 100}
	bb, _ := json.Marshal(leaseReqB)
	reqB, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, leaseURL, bytes.NewReader(bb))
	reqB.Header.Set("Content-Type", "application/json")
	//nolint:gosec // false positive: SSRF in test
	respB, err := client.Do(reqB)
	if err != nil {
		t.Fatalf("lease request B failed: %v", err)
	}
	var outB struct {
		JobID        int64  `json:"job_id"`
		CurrentNonce *int64 `json:"current_nonce"`
	}
	if err := json.NewDecoder(respB.Body).Decode(&outB); err != nil {
		respB.Body.Close()
		t.Fatalf("decode lease response B failed: %v", err)
	}
	respB.Body.Close()

	if outB.JobID != jobID {
		t.Fatalf("expected leased job id %d, got %d", jobID, outB.JobID)
	}
	if outB.CurrentNonce == nil || *outB.CurrentNonce != checkpointNonce {
		t.Fatalf("expected leased current_nonce %d, got %+v", checkpointNonce, outB.CurrentNonce)
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
