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

// State represents the worker-local state for a long-lived job assignment.
type State struct {
	JobID        string
	Prefix28     []byte
	NonceStart   uint32
	NonceEnd     uint32
	CurrentNonce uint32
	BatchStarted time.Time
	KeysScanned  uint64
}

// RuntimeConfig exposes runtime-configurable worker knobs used by higher-level
// orchestration code or tests.
type RuntimeConfig struct {
	CheckpointInterval       time.Duration
	InternalBatchSize        uint32
	TargetJobDurationSeconds int64
	MinBatchSize             uint32
	MaxBatchSize             uint32
	BatchAdjustAlpha         float64
	InitialBatchSize         uint32
}

// Worker orchestrates leasing jobs, scanning and reporting progress.
type Worker struct {
	client             *Client
	config             *Config
	measuredThroughput uint64
	batchSize          uint32
	numWorkers         int
}

// NewWorker constructs a Worker. measuredThroughput may be zero to use
// conservative defaults in CalculateBatchSize.
func NewWorker(cfg *Config) *Worker {
	// Determine goroutine count once at construction time. If the config
	// specifies a positive override use it, otherwise fallback to
	// runtime.NumCPU(). Ensure at least 1 worker.
	nw := runtime.NumCPU()
	if cfg != nil && cfg.WorkerNumGoroutines > 0 {
		nw = cfg.WorkerNumGoroutines
	}
	if nw <= 0 {
		nw = 1
	}
	return &Worker{
		client:             NewClient(cfg),
		config:             cfg,
		measuredThroughput: 0,
		batchSize:          0,
		numWorkers:         nw,
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
		log.Printf("worker: leased job %s prefix=%s targets=%v nonce=[%d,%d] expires=%s", lease.JobID, prefixHex, lease.TargetAddresses, lease.NonceStart, lease.NonceEnd, lease.ExpiresAt)

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

	// ErrLeaseExpired is returned when the Master API reports the worker's lease
	// has expired (HTTP 410 Gone) while updating a checkpoint. The worker should
	// stop processing the current lease and re-request work.
	var ErrLeaseExpired = errors.New("lease expired")

	// Start checkpoint goroutine
	ticker := time.NewTicker(w.config.CheckpointInterval)
	defer ticker.Stop()

	// Track start time to compute throughput (keys/sec) for the scanned range.
	startTime := time.Now()

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
				durationMs := time.Since(startTime).Milliseconds()
				if err := w.client.UpdateCheckpoint(bgCtx, lease.JobID, cn, tk, startTime, durationMs); err != nil {
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
				durationMs := time.Since(startTime).Milliseconds()
				if err := w.client.UpdateCheckpoint(ctx, lease.JobID, cn, tk, startTime, durationMs); err != nil {
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

	// Start real scanning using the parallel scanner in smaller internal
	// chunks. Use the cached `w.numWorkers` value determined at startup to
	// avoid repeated runtime/config checks inside the hot path.
	numWorkers := w.numWorkers
	log.Printf("worker: scanning job %s range [%d,%d] using %d goroutines", lease.JobID, lease.NonceStart, lease.NonceEnd, numWorkers)

	// Build scanner job template
	var job Job
	copy(job.Prefix28[:], lease.Prefix28)
	job.ID = 0
	job.ExpiresAt = lease.ExpiresAt

	// parse target addresses from lease
	targets := make([]common.Address, 0, len(lease.TargetAddresses))
	for _, a := range lease.TargetAddresses {
		targets = append(targets, common.HexToAddress(a))
	}

	progressFn := func(nonce uint32, keys uint64) {
		atomic.StoreUint32(&currentNonce, nonce)
		atomic.AddUint64(&totalKeys, keys)
	}

	// Determine internal chunk size
	internalBatch := uint32(1000000)
	if w.config != nil && w.config.InternalBatchSize > 0 {
		internalBatch = w.config.InternalBatchSize
	}

	// Iterate over the lease range in chunks.
	start := lease.NonceStart
	var foundResult *ScanResult
	stopEarly := false
	for start <= lease.NonceEnd {
		// Respect lease/context cancellation
		select {
		case <-leaseCtx.Done():
			// signal outer loop to stop
			stopEarly = true
		default:
		}
		if stopEarly {
			break
		}

		end := start + internalBatch - 1
		if end < start || end > lease.NonceEnd {
			end = lease.NonceEnd
		}

		// Prepare sub-job
		subJob := job
		subJob.NonceStart = start
		subJob.NonceEnd = end

		// Snapshot total keys before scanning this chunk
		beforeKeys := atomic.LoadUint64(&totalKeys)
		batchStart := time.Now()
		res, err := ScanRangeParallel(leaseCtx, subJob, targets, progressFn, numWorkers)
		batchDurationMs := time.Since(batchStart).Milliseconds()
		afterKeys := atomic.LoadUint64(&totalKeys)
		keysThisChunk := afterKeys - beforeKeys

		// If scanning returned an error, stop and propagate
		if err != nil {
			// Wait for checkpoint goroutine to finish
			cancel()
			<-doneCh
			elapsed := time.Since(startTime)
			return elapsed, afterKeys, fmt.Errorf("scan failed: %w", err)
		}

		// If a result was found, submit it
		if res != nil {
			if err := w.client.SubmitResult(ctx, res.PrivateKey[:], res.Address.Hex()); err != nil {
				if errors.Is(err, ErrUnauthorized) {
					cancel()
					<-doneCh
					elapsed := time.Since(startTime)
					return elapsed, afterKeys, ErrUnauthorized
				}
				log.Printf("worker: failed to submit result: %v", err)
			}
			atomic.StoreUint32(&currentNonce, res.Nonce)
			foundResult = res
		}

		// Send a checkpoint for this chunk (reporting chunk-level metrics).
		if err := w.client.UpdateCheckpoint(ctx, lease.JobID, atomic.LoadUint32(&currentNonce), keysThisChunk, batchStart, batchDurationMs); err != nil {
			if errors.Is(err, ErrUnauthorized) {
				// fatal: stop processing and propagate
				cancel()
				<-doneCh
				elapsed := time.Since(startTime)
				return elapsed, afterKeys, ErrUnauthorized
			}
			var apiErr *APIError
			if errors.As(err, &apiErr) && apiErr.StatusCode == 410 {
				// Lease expired on master side: stop and re-request work
				cancel()
				<-doneCh
				elapsed := time.Since(startTime)
				return elapsed, afterKeys, ErrLeaseExpired
			}
			// Non-fatal checkpoint failure: log and continue. The ticker will
			// continue attempting periodic checkpoints as well.
			log.Printf("worker: checkpoint failed for chunk [%d,%d]: %v", start, end, err)
		} else {
			log.Printf("worker: chunk checkpoint sent job=%s nonce=%d keys_chunk=%d", lease.JobID, atomic.LoadUint32(&currentNonce), keysThisChunk)
		}

		// If a result was found we can stop scanning further chunks.
		if foundResult != nil {
			break
		}

		// Advance to next chunk
		if end == lease.NonceEnd {
			break
		}
		start = end + 1
	}

	// Compute overall elapsed and totals
	elapsed := time.Since(startTime)
	tk := atomic.LoadUint64(&totalKeys)

	// Stop checkpoint goroutine and wait for it
	cancel()
	<-doneCh

	// If the checkpoint loop encountered an unauthorized error, propagate it
	// so the worker stops entirely.
	if atomic.LoadInt32(&unauthorizedFlag) == 1 {
		return elapsed, tk, ErrUnauthorized
	}

	// If we exited early due to lease expiry, the caller will handle re-request.
	// Otherwise, complete the batch on master with overall metrics.
	if err := w.client.CompleteBatch(ctx, lease.JobID, lease.NonceEnd, tk, startTime, elapsed.Milliseconds()); err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return elapsed, tk, ErrUnauthorized
		}
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 410 {
			return elapsed, tk, ErrLeaseExpired
		}
		return elapsed, tk, fmt.Errorf("failed to complete batch: %w", err)
	}

	return elapsed, tk, nil
}

// isRetryable determines whether an error should be retried.
func isRetryable(err error) bool {
	// If it's an APIError, retry on 5xx and 429.
	if apiErr, ok := errors.AsType[*APIError](err); ok {
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
