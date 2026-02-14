package server

import (
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

// TestServerE2E performs an end-to-end smoke test: start server, call /health,
// verify JSON and db connectivity, then perform graceful shutdown.
func TestServerE2E(t *testing.T) {
	t.Parallel()

	// Choose an available port using ListenConfig with context (lint: noctx)
	lc := &net.ListenConfig{}
	l, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	addr := l.Addr().(*net.TCPAddr)
	port := addr.Port
	if err := l.Close(); err != nil {
		t.Fatalf("failed to close listener: %v", err)
	}

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "e2e.db")

	cfg := &config.Config{
		Port:     fmt.Sprintf("%d", port),
		DBPath:   dbPath,
		LogLevel: "debug",
	}

	// Initialize database (applies migrations)
	ctx := t.Context()
	db, err := database.InitDB(ctx, cfg.DBPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer func() {
		if err := database.CloseDB(db); err != nil {
			t.Errorf("CloseDB failed: %v", err)
		}
	}()

	srv := New(cfg, db)
	srv.RegisterRoutes()

	// Start server in background
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(runCtx)
	}()

	// Wait for server to become responsive
	client := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	var resp *http.Response
	var body struct {
		Status    string `json:"status"`
		Timestamp string `json:"timestamp"`
		Database  string `json:"database"`
	}
	ok := false
	for range 20 {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		resp, err = client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			if err := json.NewDecoder(resp.Body).Decode(&body); err == nil {
				ok = true
				resp.Body.Close()
				break
			}
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ok {
		cancel()
		t.Fatalf("server did not respond with healthy /health in time: last err=%v", err)
	}

	if body.Status != "ok" {
		t.Fatalf("expected status ok, got %q", body.Status)
	}
	if body.Database != "connected" {
		t.Fatalf("expected database connected, got %q", body.Database)
	}

	// Test graceful shutdown
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
