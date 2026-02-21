package worker

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/garnizeh/eth-scanner/internal/config"
	"github.com/garnizeh/eth-scanner/internal/database"
	"github.com/garnizeh/eth-scanner/internal/server"
)

// TestWorkerMasterIntegration performs a full end-to-end integration test
// between the PC Worker and the Master API.
func TestWorkerMasterIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Setup Master API with temporary SQLite DB
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "master_integration.db")

	// Find a free port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

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

	srv := server.New(cfg, db)
	srv.RegisterRoutes()

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- srv.Start(ctx)
	}()

	// Wait for server to be responsive
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	responsive := false
	for range 20 {
		hctx, hcancel := context.WithTimeout(ctx, 500*time.Millisecond)
		req, _ := hctx.Done(), hcancel
		_ = req
		// Simple GET to health
		res, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err == nil {
			res.Close()
			responsive = true
			hcancel()
			break
		}
		hcancel()
		time.Sleep(100 * time.Millisecond)
	}
	if !responsive {
		t.Fatalf("server did not become responsive at %s", healthURL)
	}

	// 2. Setup PC Worker
	workerCfg := &Config{
		APIURL:             fmt.Sprintf("http://127.0.0.1:%d", port),
		WorkerID:           "pc-integration-worker",
		InitialBatchSize:   100, // Tiny batch size for fast test
		InternalBatchSize:  10,
		CheckpointInterval: 1 * time.Second,
		RetryMinDelay:      500 * time.Millisecond,
		RetryMaxDelay:      1 * time.Second,
	}

	w := NewWorker(workerCfg)

	// 3. Run Worker in background
	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()

	workerErrCh := make(chan error, 1)
	go func() {
		workerErrCh <- w.Run(workerCtx)
	}()

	// 4. Verification: Wait for a job to be completed in the DB
	q := database.NewQueries(db)
	success := false
	for range 100 { // 10 seconds timeout
		jobs, err := q.GetJobsByStatus(ctx, database.GetJobsByStatusParams{
			Status: "completed",
			Limit:  1,
		})
		if err == nil && len(jobs) > 0 {
			success = true
			break
		}
		select {
		case err := <-workerErrCh:
			if err != nil && err != context.Canceled {
				t.Fatalf("worker exited with error: %v", err)
			}
		case err := <-serverErrCh:
			if err != nil && err != context.Canceled {
				t.Fatalf("server exited with error: %v", err)
			}
		case <-time.After(100 * time.Millisecond):
			// continue polling
		}
	}

	if !success {
		t.Fatalf("worker did not complete a job within the timeout")
	}

	// 5. Final assertion: check worker history
	// We use a raw query because sqlc GetRecentWorkerHistory might be complex or not what we want
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_history WHERE worker_id = ?", workerCfg.WorkerID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query worker_history: %v", err)
	}
	if count == 0 {
		t.Errorf("expected worker_history records, got 0")
	}
}
