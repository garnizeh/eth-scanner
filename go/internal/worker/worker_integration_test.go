package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Integration-style test: Lease -> multiple chunk checkpoints -> Complete
func TestIntegration_WorkerLifecycle_MultipleCheckpoints(t *testing.T) {
	var checkpoints int32
	var completes int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/jobs/lease":
			// large range so multiple internal chunks occur (100 keys, chunk=10 -> 10 chunks)
			expires := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)
			resp := leaseResponse{
				JobID:      "integration-job",
				Prefix28:   strings.Repeat("00", 28),
				NonceStart: 0,
				NonceEnd:   99,
				ExpiresAt:  expires,
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
		case "/api/v1/jobs/integration-job/checkpoint":
			atomic.AddInt32(&checkpoints, 1)
			w.WriteHeader(http.StatusOK)
		case "/api/v1/jobs/integration-job/complete":
			atomic.AddInt32(&completes, 1)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := &Config{
		APIURL:             srv.URL,
		WorkerID:           "integration-worker",
		APIKey:             "",
		CheckpointInterval: 5 * time.Second, // rely on chunk-level checkpoints
		InternalBatchSize:  10,
	}

	w := NewWorker(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	err := w.Run(ctx)
	if err == nil {
		t.Fatalf("expected Run to exit via context timeout")
	}

	got := atomic.LoadInt32(&checkpoints)
	if got < 5 {
		t.Fatalf("expected at least 5 checkpoints from chunking, got %d", got)
	}
	if atomic.LoadInt32(&completes) == 0 {
		t.Fatalf("expected complete to be called at least once")
	}
}
