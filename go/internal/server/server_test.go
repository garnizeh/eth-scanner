package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/garnizeh/eth-scanner/internal/config"
)

func TestHandleHealth(t *testing.T) {
	s := NewServer(&config.Config{}, nil)

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
	s := NewServer(&config.Config{}, nil)
	s.RegisterRoutes()

	// health should return 200
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", nil)
	s.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("/health expected 200 got %d", rr.Code)
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
	s := NewServer(cfg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- s.Start(ctx)
	}()

	// Allow the server goroutine to start ListenAndServe.
	time.Sleep(100 * time.Millisecond)

	// Trigger graceful shutdown
	cancel()

	err := <-done
	if err == nil {
		t.Fatalf("expected error from Start after cancel, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled; got: %v", err)
	}
}
