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
	"sync"
	"testing"
	"time"

	"github.com/garnizeh/eth-scanner/internal/config"
	"github.com/garnizeh/eth-scanner/internal/database"
)

// E2E: multiple workers processing different prefixes concurrently
func TestE2E_MultipleWorkers_DifferentPrefixes(t *testing.T) {
	ctx := t.Context()
	lc := &net.ListenConfig{}
	l, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "e2e_multi.db")

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

	var wg sync.WaitGroup
	workers := 3
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		go func(id int) {
			defer wg.Done()
			// create distinct 28-byte prefix
			p := make([]byte, 28)
			for i := range p {
				p[i] = byte(id + 1)
			}
			encoded := base64.StdEncoding.EncodeToString(p)

			leaseReq := map[string]any{"worker_id": fmt.Sprintf("worker-%d", id), "requested_batch_size": 10, "prefix_28": encoded}
			b, _ := json.Marshal(leaseReq)
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, leaseURL, bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				t.Errorf("worker %d lease failed: %v", id, err)
				return
			}
			var out struct {
				JobID      int64 `json:"job_id"`
				NonceStart int64 `json:"nonce_start"`
				NonceEnd   int64 `json:"nonce_end"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				resp.Body.Close()
				t.Errorf("worker %d decode lease failed: %v", id, err)
				return
			}
			resp.Body.Close()

			// checkpoint once and complete
			chkURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/jobs/%d/checkpoint", port, out.JobID)
			chk := map[string]any{"worker_id": fmt.Sprintf("worker-%d", id), "current_nonce": out.NonceStart, "keys_scanned": 1, "started_at": time.Now().UTC().Format(time.RFC3339), "duration_ms": 1}
			cb, _ := json.Marshal(chk)
			r2, _ := http.NewRequestWithContext(context.Background(), http.MethodPatch, chkURL, bytes.NewReader(cb))
			r2.Header.Set("Content-Type", "application/json")
			resp2, err := client.Do(r2)
			if err != nil {
				t.Errorf("worker %d checkpoint failed: %v", id, err)
				return
			}
			resp2.Body.Close()

			completeURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/jobs/%d/complete", port, out.JobID)
			compReq := map[string]any{"worker_id": fmt.Sprintf("worker-%d", id), "final_nonce": out.NonceEnd, "keys_scanned": 1, "started_at": time.Now().UTC().Format(time.RFC3339), "duration_ms": 1}
			cb2, _ := json.Marshal(compReq)
			r3, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, completeURL, bytes.NewReader(cb2))
			r3.Header.Set("Content-Type", "application/json")
			resp3, err := client.Do(r3)
			if err != nil {
				t.Errorf("worker %d complete failed: %v", id, err)
				return
			}
			resp3.Body.Close()
			if resp3.StatusCode != http.StatusOK {
				t.Errorf("worker %d complete status %d", id, resp3.StatusCode)
			}
		}(w)
	}

	wg.Wait()

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
