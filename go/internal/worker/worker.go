package worker

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// Worker orchestrates leasing jobs, scanning and reporting progress.
type Worker struct {
	client             *Client
	config             *Config
	measuredThroughput uint64
	batchSize          uint32
}

// NewWorker constructs a Worker. measuredThroughput may be zero to use
// conservative defaults in CalculateBatchSize.
func NewWorker(cfg *Config) *Worker {
	return &Worker{
		client:             NewClient(cfg),
		config:             cfg,
		measuredThroughput: 0,
		batchSize:          0,
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

		// Initialize batch size from worker state or config
		if w.batchSize == 0 {
			target := 1 * time.Hour
			if w.config != nil && w.config.TargetJobDurationSeconds > 0 {
				target = time.Duration(w.config.TargetJobDurationSeconds) * time.Second
			}
			if w.config != nil && w.config.InitialBatchSize > 0 {
				w.batchSize = w.config.InitialBatchSize
			} else {
				w.batchSize = CalculateBatchSize(w.measuredThroughput, target)
			}
		}
		log.Printf("worker: requesting batch size %d", w.batchSize)

		lease, err := w.client.LeaseBatch(ctx, w.batchSize)
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

		// Log lease response details (this is the response to the earlier
		// "requesting batch size" log). Include key prefix, target address,
		// nonce range and expiry for observability.
		prefixHex := ""
		if len(lease.Prefix28) > 0 {
			prefixHex = hex.EncodeToString(lease.Prefix28)
		}
		log.Printf("worker: leased job %s prefix=%s target=%s nonce=[%d,%d] expires=%s", lease.JobID, prefixHex, lease.TargetAddress, lease.NonceStart, lease.NonceEnd, lease.ExpiresAt)

		duration, keys, err := w.processBatch(ctx, lease)
		if err != nil {
			// If unauthorized bubbled up, stop worker
			if errors.Is(err, ErrUnauthorized) {
				return err
			}
			log.Printf("worker: processing batch failed: %v", err)
			// Continue loop; job will be re-leased or reassigned by Master after expiry
			continue
		}

		log.Printf("worker: completed job %s (duration=%s keys=%d)", lease.JobID, duration.Round(time.Millisecond), keys)

		// Adjust batch size for next iteration using adaptive controller
		if w.config != nil {
			target := time.Duration(w.config.TargetJobDurationSeconds) * time.Second
			newSize := AdjustBatchSize(w.batchSize, target, duration, w.config.MinBatchSize, w.config.MaxBatchSize, w.config.BatchAdjustAlpha)
			log.Printf("worker: batch size adjusted %d -> %d", w.batchSize, newSize)
			w.batchSize = newSize
			// update measured throughput estimate
			if duration.Seconds() > 0 {
				w.measuredThroughput = uint64(float64(keys) / duration.Seconds())
			}
		}

	}
}

// processBatch handles scanning for a leased job, sending periodic checkpoints
// and completing the job when done. The actual scanning (crypto) is delegated
// to the scanner component (not implemented here); this function contains a
// simple placeholder to simulate work and the checkpointing logic.
func (w *Worker) processBatch(ctx context.Context, lease *JobLease) (time.Duration, uint64, error) {
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
		// unauthorizedFlag is set to 1 when checkpointing returns ErrUnauthorized
		// so the main flow can abort and propagate ErrUnauthorized.
		unauthorizedFlag int32
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
						// mark unauthorized so main flow returns ErrUnauthorized
						atomic.StoreInt32(&unauthorizedFlag, 1)
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
						// fatal: mark flag and cancel lease context so scanning stops.
						atomic.StoreInt32(&unauthorizedFlag, 1)
						log.Printf("worker: checkpoint unauthorized")
						cancel()
						return
					}
					log.Printf("worker: checkpoint failed: %v", err)
				} else {
					log.Printf("worker: checkpoint sent job=%s nonce=%d keys=%d", lease.JobID, cn, tk)
				}
			}
		}
	}()

	// Start real scanning using the parallel scanner. Pass a progress callback
	// that updates atomic counters for checkpointing.
	numWorkers := runtime.NumCPU()
	if numWorkers <= 0 {
		numWorkers = 1
	}
	log.Printf("worker: scanning job %s range [%d,%d] using %d goroutines", lease.JobID, lease.NonceStart, lease.NonceEnd, numWorkers)

	// Track start time to compute throughput (keys/sec) for the scanned range.
	startTime := time.Now()

	// Build scanner job
	var job Job
	copy(job.Prefix28[:], lease.Prefix28)
	job.ID = 0
	job.NonceStart = lease.NonceStart
	job.NonceEnd = lease.NonceEnd
	job.ExpiresAt = lease.ExpiresAt

	// parse target address from lease
	target := common.HexToAddress(lease.TargetAddress)

	progressFn := func(nonce uint32, keys uint64) {
		atomic.StoreUint32(&currentNonce, nonce)
		atomic.AddUint64(&totalKeys, keys)
	}

	result, err := ScanRangeParallel(leaseCtx, job, target, progressFn)

	// Compute throughput from atomic totalKeys and elapsed time.
	elapsed := time.Since(startTime)
	tk := atomic.LoadUint64(&totalKeys)
	cn := atomic.LoadUint32(&currentNonce)
	var rate float64
	if elapsed.Seconds() > 0 {
		rate = float64(tk) / elapsed.Seconds()
	}
	log.Printf("worker: scan completed job=%s final_nonce=%d keys=%d duration=%s rate=%.2f keys/s", lease.JobID, cn, tk, elapsed.Round(time.Millisecond), rate)
	if err != nil {
		// Wait for checkpoint goroutine to finish
		cancel()
		<-doneCh
		return elapsed, tk, fmt.Errorf("scan failed: %w", err)
	}

	// If a result was found, submit it
	if result != nil {
		// Submit result to master
		if err := w.client.SubmitResult(ctx, result.PrivateKey[:], result.Address.Hex()); err != nil {
			if errors.Is(err, ErrUnauthorized) {
				return elapsed, tk, ErrUnauthorized
			}
			log.Printf("worker: failed to submit result: %v", err)
		}
		// Update atomics to final values for completion
		atomic.StoreUint32(&currentNonce, result.Nonce)
		// totalKeys already updated by progressFn
	}

	// Stop checkpoint goroutine and wait for it
	cancel()
	<-doneCh

	// If the checkpoint loop encountered an unauthorized error, propagate it
	// so the worker stops entirely.
	if atomic.LoadInt32(&unauthorizedFlag) == 1 {
		return elapsed, tk, ErrUnauthorized
	}

	// Complete the batch
	if err := w.client.CompleteBatch(ctx, lease.JobID, lease.NonceEnd, totalKeys); err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return elapsed, tk, ErrUnauthorized
		}
		return elapsed, tk, fmt.Errorf("failed to complete batch: %w", err)
	}

	return elapsed, tk, nil
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
