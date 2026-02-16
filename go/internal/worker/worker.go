package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync/atomic"
	"time"
)

// Worker orchestrates leasing jobs, scanning and reporting progress.
type Worker struct {
	client             *Client
	config             *Config
	measuredThroughput uint64
}

// NewWorker constructs a Worker. measuredThroughput may be zero to use
// conservative defaults in CalculateBatchSize.
func NewWorker(cfg *Config) *Worker {
	return &Worker{
		client:             NewClient(cfg),
		config:             cfg,
		measuredThroughput: 0,
	}
}

// Run starts the main worker loop. It returns when ctx is cancelled or a
// fatal error (like ErrUnauthorized) occurs.
func (w *Worker) Run(ctx context.Context) error {
	log.Println("worker: starting")
	// Setup backoff using config (defaults set in LoadConfig)
	backoff := NewBackoff(w.config.RetryMinDelay, w.config.RetryMaxDelay)

	for {
		// Respect parent context cancellation
		select {
		case <-ctx.Done():
			log.Println("worker: context cancelled, shutting down")
			return fmt.Errorf("worker: %w", ctx.Err())
		default:
		}

		// Calculate batch size (target ~1 hour)
		batchSize := CalculateBatchSize(w.measuredThroughput, 1*time.Hour)
		log.Printf("worker: requesting batch size %d (~1h)", batchSize)

		lease, err := w.client.LeaseBatch(ctx, batchSize)
		if err != nil {
			if errors.Is(err, ErrNoJobsAvailable) {
				delay := backoff.Next()
				log.Printf("worker: no jobs available, waiting %v", delay)
				select {
				case <-time.After(delay):
					continue
				case <-ctx.Done():
					return fmt.Errorf("worker: %w", ctx.Err())
				}
			}
			if errors.Is(err, ErrUnauthorized) {
				return fmt.Errorf("worker: lease failed: %w", err)
			}

			if isRetryable(err) {
				delay := backoff.Next()
				log.Printf("worker: lease failed (retryable): %v; waiting %v", err, delay)
				select {
				case <-time.After(delay):
					continue
				case <-ctx.Done():
					return fmt.Errorf("worker: %w", ctx.Err())
				}
			}

			// Non-retryable error: propagate
			return fmt.Errorf("worker: lease failed (non-retryable): %w", err)
		}

		// successful lease -> reset backoff
		backoff.Reset()

		log.Printf("worker: leased job %s nonce [%d,%d] expires=%s", lease.JobID, lease.NonceStart, lease.NonceEnd, lease.ExpiresAt)

		if err := w.processBatch(ctx, lease); err != nil {
			// If unauthorized bubbled up, stop worker
			if errors.Is(err, ErrUnauthorized) {
				return err
			}
			log.Printf("worker: processing batch failed: %v", err)
			// Continue loop; job will be re-leased or reassigned by Master after expiry
			continue
		}

		log.Printf("worker: completed job %s", lease.JobID)
	}
}

// processBatch handles scanning for a leased job, sending periodic checkpoints
// and completing the job when done. The actual scanning (crypto) is delegated
// to the scanner component (not implemented here); this function contains a
// simple placeholder to simulate work and the checkpointing logic.
func (w *Worker) processBatch(ctx context.Context, lease *JobLease) error {
	// Lease context tied to (expires_at - gracePeriod) so we stop scanning
	// slightly before the master-side lease expires to allow time for a final
	// checkpoint and graceful shutdown.
	grace := 30 * time.Second
	if w.config != nil && w.config.LeaseGracePeriod != 0 {
		grace = w.config.LeaseGracePeriod
	}
	deadline := lease.ExpiresAt.Add(-grace)
	leaseCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	// Use atomics for values shared between goroutine and main flow to avoid races.
	var (
		currentNonce = lease.NonceStart
		totalKeys    uint64
	)

	// Start checkpoint goroutine
	ticker := time.NewTicker(w.config.CheckpointInterval)
	defer ticker.Stop()

	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		for {
			select {
			case <-leaseCtx.Done():
				// Send a final checkpoint before exiting. Use a background context
				// with timeout so we don't hang if the API is slow.
				cn := atomic.LoadUint32(&currentNonce)
				tk := atomic.LoadUint64(&totalKeys)
				bgCtx, bgCancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := w.client.UpdateCheckpoint(bgCtx, lease.JobID, cn, tk); err != nil {
					if errors.Is(err, ErrUnauthorized) {
						log.Printf("worker: final checkpoint unauthorized for job=%s", lease.JobID)
					} else {
						log.Printf("worker: final checkpoint failed: %v", err)
					}
				} else {
					log.Printf("worker: final checkpoint sent job=%s nonce=%d keys=%d", lease.JobID, cn, tk)
				}
				bgCancel()
				return
			case <-ticker.C:
				// Report checkpoint using parent ctx to avoid being cancelled by leaseCtx
				// Snapshot atomically to avoid data races
				cn := atomic.LoadUint32(&currentNonce)
				tk := atomic.LoadUint64(&totalKeys)
				if err := w.client.UpdateCheckpoint(ctx, lease.JobID, cn, tk); err != nil {
					if errors.Is(err, ErrUnauthorized) {
						// fatal
						log.Printf("worker: checkpoint unauthorized")
						return
					}
					log.Printf("worker: checkpoint failed: %v", err)
				} else {
					log.Printf("worker: checkpoint sent job=%s nonce=%d keys=%d", lease.JobID, cn, tk)
				}
			}
		}
	}()

	// Placeholder scanning: simulate work until either lease expires or we finish
	log.Printf("worker: scanning job %s range [%d,%d] (simulated)", lease.JobID, lease.NonceStart, lease.NonceEnd)

	select {
	case <-leaseCtx.Done():
		log.Printf("worker: lease context done for job %s: %v", lease.JobID, leaseCtx.Err())
		// Wait for checkpoint goroutine to finish
		cancel()
		<-doneCh
		return fmt.Errorf("worker: %w", leaseCtx.Err())
	case <-time.After(2 * time.Second):
		// Simulate completion
		atomic.StoreUint32(&currentNonce, lease.NonceEnd)
		atomic.StoreUint64(&totalKeys, uint64(lease.NonceEnd-lease.NonceStart+1))
	}

	// Stop checkpoint goroutine and wait for it
	cancel()
	<-doneCh

	// Complete the batch
	if err := w.client.CompleteBatch(ctx, lease.JobID, lease.NonceEnd, totalKeys); err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return ErrUnauthorized
		}
		return fmt.Errorf("failed to complete batch: %w", err)
	}

	return nil
}

// isRetryable determines whether an error should be retried.
func isRetryable(err error) bool {
	// If it's an APIError, retry on 5xx and 429.
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode >= 500 && apiErr.StatusCode < 600 {
			return true
		}
		if apiErr.StatusCode == 429 {
			return true
		}
		return false
	}
	// If it's ErrNoJobsAvailable, treat as retryable (should be handled earlier)
	if errors.Is(err, ErrNoJobsAvailable) {
		return true
	}
	// Non-API errors (network, timeouts) are considered retryable.
	return true
}
