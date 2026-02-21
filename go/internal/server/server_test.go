package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/garnizeh/eth-scanner/internal/config"
	"github.com/garnizeh/eth-scanner/internal/database"
)

func TestHandleHealth(t *testing.T) {
	s, err := New(&config.Config{}, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", nil)

	s.handleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var body struct {
		Status    string `json:"status"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("unexpected status: %q", body.Status)
	}
	// timestamp should be RFC3339 and in UTC
	ts, err := time.Parse(time.RFC3339, body.Timestamp)
	if err != nil {
		t.Fatalf("timestamp not RFC3339: %v", err)
	}
	if ts.Location() != time.UTC {
		t.Fatalf("timestamp not in UTC: %v (loc=%v)", ts, ts.Location())
	}
}

func TestRegisterRoutes(t *testing.T) {
	s, err := New(&config.Config{}, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	s.RegisterRoutes()

	// health should return 200
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", nil)
	s.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("/health expected 200 got %d", rr.Code)
	}

	// / should redirect to /dashboard
	rrR := httptest.NewRecorder()
	reqR := httptest.NewRequest("GET", "/", nil)
	s.router.ServeHTTP(rrR, reqR)
	if rrR.Code != http.StatusSeeOther {
		t.Fatalf("/ expected 303 got %d", rrR.Code)
	}
	if loc := rrR.Header().Get("Location"); loc != "/dashboard" {
		t.Fatalf("/ redirect location expected /dashboard got %q", loc)
	}

	// /api/v1/ placeholder should return 501
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/api/v1/", nil)
	s.router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusNotImplemented {
		t.Fatalf("/api/v1/ expected 501 got %d", rr2.Code)
	}
}

// TestStartGracefulShutdown starts the server in a goroutine, cancels the
// context to trigger graceful shutdown and ensures Start returns an error
// wrapping context.Canceled.
func TestStartGracefulShutdown(t *testing.T) {
	cfg := &config.Config{Port: "0"}
	s, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- s.Start(ctx)
	}()

	// Allow the server goroutine to start ListenAndServe.
	time.Sleep(100 * time.Millisecond)

	// Trigger graceful shutdown
	cancel()

	err = <-done
	if err == nil {
		t.Fatalf("expected error from Start after cancel, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled; got: %v", err)
	}
}

// helper to get a free port using ListenConfig
func freePort(t *testing.T) int {
	t.Helper()
	lc := &net.ListenConfig{}
	l, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func TestShutdownWaitsForInFlightRequests(t *testing.T) {
	t.Parallel()

	port := freePort(t)
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "shutdown_wait.db")

	cfg := &config.Config{Port: fmt.Sprintf("%d", port), DBPath: dbPath, ShutdownTimeout: 5 * time.Second}

	ctx := context.Background()
	db, err := database.InitDB(ctx, cfg.DBPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	// don't defer close; server will close DB on shutdown

	srv, err := New(cfg, db)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})

	srv.router.HandleFunc("/long", func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("done"))
	})
	srv.RegisterRoutes()

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(runCtx) }()

	// ensure server is listening before starting request to avoid race
	startWaitCtx, startWaitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer startWaitCancel()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for {
		select {
		case <-startWaitCtx.Done():
			t.Fatal("server did not start listening in time")
		default:
		}
		d := &net.Dialer{Timeout: 200 * time.Millisecond}
		c, err := d.DialContext(context.Background(), "tcp", addr)
		if err == nil {
			_ = c.Close()
			goto startedListening
		}
		time.Sleep(20 * time.Millisecond)
	}
startedListening:

	// start request
	client := &http.Client{Timeout: 10 * time.Second}
	reqDone := make(chan error, 1)
	go func() {
		url := fmt.Sprintf("http://127.0.0.1:%d/long", port)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			reqDone <- err
			return
		}
		//nolint:gosec // false positive: SSRF in test
		resp, err := client.Do(req)
		if err != nil {
			reqDone <- err
			return
		}
		defer resp.Body.Close()
		reqDone <- nil
	}()

	// wait for handler to start
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}

	// initiate shutdown
	cancel()

	// allow handler to finish after shutdown started
	close(release)

	// wait for server to exit
	select {
	case e := <-errCh:
		if e != nil && !errors.Is(e, context.Canceled) {
			t.Fatalf("unexpected server error: %v", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shutdown in time")
	}

	// request should have completed successfully
	select {
	case err := <-reqDone:
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
	default:
		t.Fatal("request did not complete")
	}
}

func TestShutdownRespectsTimeout(t *testing.T) {
	t.Parallel()

	port := freePort(t)
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "shutdown_timeout.db")

	// short shutdown timeout
	cfg := &config.Config{Port: fmt.Sprintf("%d", port), DBPath: dbPath, ShutdownTimeout: 100 * time.Millisecond}

	ctx := context.Background()
	db, err := database.InitDB(ctx, cfg.DBPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	srv, err := New(cfg, db)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// handler that sleeps longer than shutdown timeout
	srv.router.HandleFunc("/sleep", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	srv.RegisterRoutes()

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	start := time.Now().UTC()
	go func() { errCh <- srv.Start(runCtx) }()

	// wait a short moment for server to start
	time.Sleep(50 * time.Millisecond)

	// start a request
	client := &http.Client{Timeout: 2 * time.Second}
	reqDone := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(context.Background(), "GET", fmt.Sprintf("http://127.0.0.1:%d/sleep", port), nil)
		if err != nil {
			reqDone <- err
			return
		}
		//nolint:gosec // false positive: SSRF in test
		resp, err := client.Do(req)
		if resp != nil {
			defer resp.Body.Close()
		}
		reqDone <- err
	}()

	// initiate shutdown
	cancel()

	// wait for server to exit and measure duration
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not exit in time")
	}
	dur := time.Since(start)
	if dur < cfg.ShutdownTimeout || dur > cfg.ShutdownTimeout+500*time.Millisecond {
		t.Fatalf("shutdown duration unexpected: %v (timeout %v)", dur, cfg.ShutdownTimeout)
	}

	// request should likely fail due to forced shutdown; ensure it does not hang
	select {
	case err := <-reqDone:
		if err == nil {
			t.Fatalf("expected request to be aborted, but it succeeded")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("request did not finish after shutdown")
	}
}

func TestDBClosedOnShutdown(t *testing.T) {
	t.Parallel()

	port := freePort(t)
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "shutdown_db.db")

	cfg := &config.Config{Port: fmt.Sprintf("%d", port), DBPath: dbPath, ShutdownTimeout: 2 * time.Second}

	ctx := context.Background()
	db, err := database.InitDB(ctx, cfg.DBPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	srv, err := New(cfg, db)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	srv.RegisterRoutes()

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(runCtx) }()

	// wait a moment then shutdown
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-errCh:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shutdown in time")
	}

	// DB should be closed by server shutdown
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelPing()
	if err := db.PingContext(pingCtx); err == nil {
		t.Fatalf("expected db to be closed after shutdown, ping succeeded")
	}
	// if db is closed, calling CloseDB should be safe
	_ = database.CloseDB(db)
}
