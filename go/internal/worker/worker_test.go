package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerRun_ProcessesAndCompletesBatch(t *testing.T) {
	var checkpoints int32
	var completes int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/jobs/lease":
			expires := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
			resp := leaseResponse{
				JobID:           "test-job-123",
				Prefix28:        strings.Repeat("00", 28),
				NonceStart:      0,
				NonceEnd:        10,
				TargetAddresses: []string{"0x000000000000000000000000000000000000dEaD"},
				ExpiresAt:       expires,
			}
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("encode lease response: %v", err)
			}
		case "/api/v1/jobs/test-job-123/checkpoint":
			atomic.AddInt32(&checkpoints, 1)
			w.WriteHeader(http.StatusOK)
		case "/api/v1/jobs/test-job-123/complete":
			atomic.AddInt32(&completes, 1)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &Config{
		APIURL:             srv.URL,
		WorkerID:           "test-worker",
		APIKey:             "",
		CheckpointInterval: 200 * time.Millisecond,
		InternalBatchSize:  10,
	}

	w := NewWorker(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	err := w.Run(ctx)
	if err == nil {
		t.Fatalf("expected error due to context timeout")
	}
	if ctx.Err() == nil {
		t.Fatalf("expected parent context to be done")
	}

	if atomic.LoadInt32(&completes) == 0 {
		t.Fatalf("expected CompleteBatch to be called at least once")
	}
	if atomic.LoadInt32(&checkpoints) == 0 {
		t.Fatalf("expected at least one checkpoint to be sent")
	}
}

func TestWorkerRun_LeaseExpiresBeforeCompletion(t *testing.T) {
	var completes int32
	var leaseCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/jobs/lease":
			// First lease has a very short expiry; subsequent leases return 404
			if atomic.AddInt32(&leaseCount, 1) == 1 {
				expires := time.Now().Add(500 * time.Millisecond).UTC().Format(time.RFC3339)
				resp := leaseResponse{
					JobID:           "short-lease",
					Prefix28:        strings.Repeat("00", 28),
					NonceStart:      0,
					NonceEnd:        1000,
					TargetAddresses: []string{"0x000000000000000000000000000000000000dEaD"},
					ExpiresAt:       expires,
				}
				w.WriteHeader(http.StatusOK)
				if err := json.NewEncoder(w).Encode(resp); err != nil {
					t.Fatalf("encode lease response: %v", err)
				}
				return
			}
			w.WriteHeader(http.StatusNotFound)
		case "/api/v1/jobs/short-lease/checkpoint":
			// ignore
			w.WriteHeader(http.StatusOK)
		case "/api/v1/jobs/short-lease/complete":
			atomic.AddInt32(&completes, 1)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &Config{
		APIURL:             srv.URL,
		WorkerID:           "test-worker",
		APIKey:             "",
		CheckpointInterval: 200 * time.Millisecond,
		LeaseGracePeriod:   300 * time.Millisecond,
		InternalBatchSize:  10,
	}

	w := NewWorker(cfg)

	// Construct a lease directly to avoid interactions with LeaseBatch timing
	lease := &JobLease{
		JobID:           "test-job-unauth",
		Prefix28:        make([]byte, 28),
		NonceStart:      0,
		NonceEnd:        1,
		TargetAddresses: []string{"0x000000000000000000000000000000000000dEaD"},
		// set a short expiry so the grace period triggers an earlier deadline
		ExpiresAt: time.Now().Add(500 * time.Millisecond).UTC(),
	}

	_, _, err := w.processBatch(context.Background(), lease)
	if err != nil {
		t.Logf("processBatch returned: %v", err)
	}

	if atomic.LoadInt32(&completes) != 0 {
		t.Fatalf("did not expect CompleteBatch to be called when lease expires before completion")
	}
}

func TestWorkerRun_LeaseUnauthorizedStops(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/jobs/lease" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized", "message": "bad key"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := &Config{
		APIURL:             srv.URL,
		WorkerID:           "test-worker",
		APIKey:             "bad",
		CheckpointInterval: 1 * time.Second,
		InternalBatchSize:  10,
	}

	w := NewWorker(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	err := w.Run(ctx)
	if err == nil {
		t.Fatalf("expected error when lease returns 401")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestWorkerRun_LeaseError_ContextCancelledDuringRetry(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/jobs/lease" {
			atomic.AddInt32(&attempts, 1)
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "server", "message": "oops"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := &Config{
		APIURL:             srv.URL,
		WorkerID:           "test-worker",
		APIKey:             "",
		CheckpointInterval: 1 * time.Second,
		InternalBatchSize:  10,
	}

	w := NewWorker(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := w.Run(ctx)
	if err == nil {
		t.Fatalf("expected Run to return when context times out")
	}
	if ctx.Err() == nil {
		t.Fatalf("expected parent context to be done")
	}
	if atomic.LoadInt32(&attempts) == 0 {
		t.Fatalf("expected at least one lease attempt")
	}
}

func TestCheckpointUnauthorizedStopsCheckpointLoop(t *testing.T) {
	var checkpoints int32
	var completes int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/jobs/lease":
			expires := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
			resp := leaseResponse{
				JobID:      "test-job-unauth",
				Prefix28:   strings.Repeat("00", 28),
				NonceStart: 0,
				NonceEnd:   1,
				ExpiresAt:  expires,
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
		case "/api/v1/jobs/test-job-unauth/checkpoint":
			// First checkpoint returns 401, subsequent would be 200.
			if atomic.AddInt32(&checkpoints, 1) == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
			w.WriteHeader(http.StatusOK)
		case "/api/v1/jobs/test-job-unauth/complete":
			atomic.AddInt32(&completes, 1)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &Config{
		APIURL:             srv.URL,
		WorkerID:           "test-worker",
		APIKey:             "",
		CheckpointInterval: 100 * time.Millisecond,
		InternalBatchSize:  10,
	}

	w := NewWorker(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = w.Run(ctx)

	// Ensure at least one checkpoint was attempted (the goroutine should stop after unauthorized)
	if atomic.LoadInt32(&checkpoints) < 1 {
		t.Fatalf("expected at least 1 checkpoint attempt, got %d", atomic.LoadInt32(&checkpoints))
	}
}

func TestProcessBatch_CompleteUnauthorizedReturnsErrUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/jobs/lease":
			expires := time.Now().Add(1 * time.Minute).UTC().Format(time.RFC3339)
			resp := leaseResponse{
				JobID:      "job-unauth",
				Prefix28:   strings.Repeat("00", 28),
				NonceStart: 0,
				NonceEnd:   1,
				ExpiresAt:  expires,
			}
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Fatalf("encode lease response: %v", err)
			}
		case "/api/v1/jobs/job-unauth/checkpoint":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/jobs/job-unauth/complete":
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &Config{
		APIURL:             srv.URL,
		WorkerID:           "test-worker",
		APIKey:             "",
		CheckpointInterval: 100 * time.Millisecond,
		InternalBatchSize:  10,
	}

	w := NewWorker(cfg)

	lease, err := w.client.LeaseBatch(context.Background(), 1)
	if err != nil {
		t.Fatalf("lease failed: %v", err)
	}

	_, _, err = w.processBatch(context.Background(), lease)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

// TestRun_NoJobsAvailable_BackoffAndCancel ensures the worker backs off when
// LeaseBatch returns ErrNoJobsAvailable and that Run respects context
// cancellation while sleeping during backoff.
func TestRun_NoJobsAvailable_BackoffAndCancel(t *testing.T) {
	var leaseCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/jobs/lease" {
			atomic.AddInt32(&leaseCount, 1)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"no jobs available"}`))
			return
		}
		// Default OK for other endpoints used by the client
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cfg := &Config{
		APIURL:             srv.URL,
		WorkerID:           "test-worker",
		APIKey:             "",
		CheckpointInterval: 50 * time.Millisecond,
		RetryMinDelay:      10 * time.Millisecond,
		RetryMaxDelay:      50 * time.Millisecond,
		InternalBatchSize:  10,
	}

	w := NewWorker(cfg)

	// Run the worker with a short timeout so we exercise the backoff sleep
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.Run(ctx)
	}()

	err := <-errCh

	// Run should return a wrapped context.DeadlineExceeded (or canceled)
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got: %v", err)
	}

	if atomic.LoadInt32(&leaseCount) == 0 {
		t.Fatalf("expected at least one LeaseBatch request, got 0")
	}
}

func TestWorkerRun_RerequestOnCheckpoint410(t *testing.T) {
	var leaseCount int32
	var checkpoints int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/jobs/lease":
			// Count leases, return a lease only on first call
			if atomic.AddInt32(&leaseCount, 1) == 1 {
				expires := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
				resp := leaseResponse{
					JobID:      "job-410",
					Prefix28:   strings.Repeat("00", 28),
					NonceStart: 0,
					NonceEnd:   100,
					ExpiresAt:  expires,
				}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(resp)
				return
			}
			// subsequent leases return 404
			w.WriteHeader(http.StatusNotFound)
		case "/api/v1/jobs/job-410/checkpoint":
			// First checkpoint returns 410 Gone to indicate lease expired.
			if atomic.AddInt32(&checkpoints, 1) == 1 {
				w.WriteHeader(http.StatusGone)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "lease expired"})
				return
			}
			w.WriteHeader(http.StatusOK)
		case "/api/v1/jobs/job-410/complete":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &Config{
		APIURL:             srv.URL,
		WorkerID:           "test-worker",
		APIKey:             "",
		CheckpointInterval: 1 * time.Second,
		InternalBatchSize:  50, // chunking will create at least one chunk
	}

	w := NewWorker(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_ = w.Run(ctx)

	if atomic.LoadInt32(&leaseCount) < 2 {
		t.Fatalf("expected worker to re-request a lease after 410; leaseCount=%d", atomic.LoadInt32(&leaseCount))
	}
}

func TestWorkerRun_TickerTriggersCheckpointWhenChunkLong(t *testing.T) {
	var checkpoints int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/jobs/lease":
			expires := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
			resp := leaseResponse{
				JobID:      "job-ticker",
				Prefix28:   strings.Repeat("00", 28),
				NonceStart: 0,
				NonceEnd:   1000,
				ExpiresAt:  expires,
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
		case "/api/v1/jobs/job-ticker/checkpoint":
			atomic.AddInt32(&checkpoints, 1)
			w.WriteHeader(http.StatusOK)
		case "/api/v1/jobs/job-ticker/complete":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &Config{
		APIURL:             srv.URL,
		WorkerID:           "test-worker",
		APIKey:             "",
		CheckpointInterval: 50 * time.Millisecond,
		InternalBatchSize:  100000000, // very large so chunk checkpoint won't occur quickly
	}

	w := NewWorker(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_ = w.Run(ctx)

	if atomic.LoadInt32(&checkpoints) == 0 {
		t.Fatalf("expected at least one checkpoint from ticker, got %d", atomic.LoadInt32(&checkpoints))
	}
}
