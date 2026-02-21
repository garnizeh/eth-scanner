package worker

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"runtime"
	"sync"
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

// ErrLeaseExpired is returned when the Master API reports the worker's lease
// has expired (HTTP 410 Gone).
var ErrLeaseExpired = errors.New("lease expired")

// Worker orchestrates leasing jobs, scanning and reporting progress.
type Worker struct {
	client             *Client
	config             *Config
	measuredThroughput uint64
	batchSize          uint32
	numWorkers         int
}

// NewWorker constructs a Worker. measuredThroughput may be zero to use
// conservative defaults in CalculateBatchSize. Config MUST not be nil.
func NewWorker(cfg *Config) *Worker {
	if cfg == nil {
		panic("worker: Nil configuration provided")
	}
	// Determine goroutine count once at construction time. If the config
	// specifies a positive override use it, otherwise fallback to
	// runtime.NumCPU(). Ensure at least 1 worker.
	nw := runtime.NumCPU()
	if cfg.WorkerNumGoroutines > 0 {
		nw = cfg.WorkerNumGoroutines
	}
	if nw <= 0 {
		nw = 1
	}

	// Apply sensible defaults if not set (common in tests using struct literals).
	// Most of these are normally set by LoadConfig().
	if cfg.CheckpointInterval <= 0 {
		cfg.CheckpointInterval = 5 * time.Minute
	}
	if cfg.InternalBatchSize == 0 {
		cfg.InternalBatchSize = 1_000_000
	}
	if cfg.CheckpointTimeout == 0 {
		cfg.CheckpointTimeout = 10 * time.Second
	}
	if cfg.ProgressThrottleMS == 0 {
		cfg.ProgressThrottleMS = 100 // default to 100ms if not specified
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

		if !w.config.LogSampling {
			log.Printf("worker: completed job %s (duration=%s keys=%d)", lease.JobID, duration.Round(time.Millisecond), keys)
		}

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
	startNonce := lease.NonceStart
	if lease.CurrentNonce != nil {
		startNonce = *lease.CurrentNonce
	}

	var (
		currentNonce = startNonce
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
	var lastCheckpointTime time.Time
	const minCheckpointInterval = 10 * time.Second

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
					if !w.config.LogSampling {
						log.Printf("worker: final checkpoint sent job=%s nonce=%d keys=%d", lease.JobID, cn, tk)
					}
				}
				bgCancel()
				return
			case <-ticker.C:
				// Report checkpoint using parent ctx to avoid being cancelled by leaseCtx
				// Snapshot atomically to avoid data races
				cn := atomic.LoadUint32(&currentNonce)
				tk := atomic.LoadUint64(&totalKeys)
				durationMs := time.Since(startTime).Milliseconds()

				// Per-call timeout for periodic checkpoint
				cctx, ccancel := context.WithTimeout(ctx, w.config.CheckpointTimeout)
				if err := w.client.UpdateCheckpoint(cctx, lease.JobID, cn, tk, startTime, durationMs); err != nil {
					ccancel()
					if errors.Is(err, ErrUnauthorized) {
						// fatal: mark flag and cancel lease context so scanning stops.
						atomic.StoreInt32(&unauthorizedFlag, 1)
						log.Printf("worker: checkpoint unauthorized")
						cancel()
						return
					}
					log.Printf("worker: checkpoint failed: %v", err)
				} else {
					ccancel()
					if !w.config.LogSampling {
						log.Printf("worker: checkpoint sent job=%s nonce=%d keys=%d", lease.JobID, cn, tk)
					}
				}
			}
		}
	}()

	// Start real scanning using the parallel scanner in smaller internal
	// chunks. Use the cached `w.numWorkers` value determined at startup to
	// avoid repeated runtime/config checks inside the hot path.
	numWorkers := w.numWorkers
	if !w.config.LogSampling {
		log.Printf("worker: scanning job %s range [%d,%d] using %d goroutines", lease.JobID, lease.NonceStart, lease.NonceEnd, numWorkers)
	}

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

	// Wrap progress updates in a throttler to reduce atomic overhead.
	// We use a local non-atomic variable to accumulate keys between updates
	// and update the shared atomics only periodically.
	var (
		progressMu         sync.Mutex
		lastProgressUpdate time.Time
		localKeys          uint64
		latestNonce        uint32
	)
	progressThrottle := time.Duration(w.config.ProgressThrottleMS) * time.Millisecond

	progressFn := func(nonce uint32, keys uint64) {
		progressMu.Lock()
		defer progressMu.Unlock()
		localKeys += keys
		if nonce > latestNonce {
			latestNonce = nonce
		}
		now := time.Now()
		// Only update shared atomics if enough time has passed to reduce synchronization overhead.
		if now.Sub(lastProgressUpdate) >= progressThrottle {
			atomic.StoreUint32(&currentNonce, latestNonce)
			atomic.AddUint64(&totalKeys, localKeys)
			localKeys = 0
			lastProgressUpdate = now
		}
	}

	// flushProgress ensures all accumulated keys are reported to atomics.
	// Must be called after ScanRangeParallel returns.
	flushProgress := func(finalNonce uint32) {
		progressMu.Lock()
		defer progressMu.Unlock()
		if localKeys > 0 {
			atomic.AddUint64(&totalKeys, localKeys)
			localKeys = 0
		}
		// update latestNonce to final if provided, then store in atomic
		if finalNonce > latestNonce {
			latestNonce = finalNonce
		}
		atomic.StoreUint32(&currentNonce, latestNonce)
	}

	// Determine internal chunk size
	internalBatch := uint32(1000000)
	if w.config.InternalBatchSize > 0 {
		internalBatch = w.config.InternalBatchSize
	}

	// Iterate over the lease range in chunks, starting from the last checkpoint
	// if this is a resumption.
	start := startNonce
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

		res, err := ScanRangeParallel(leaseCtx, subJob, targets, progressFn, numWorkers)
		flushProgress(end) // Flush any pending keys from this chunk

		// If scanning returned an error, stop and propagate
		if err != nil {
			// Wait for checkpoint goroutine to finish
			cancel()
			<-doneCh
			elapsed := time.Since(startTime)
			afterKeys := atomic.LoadUint64(&totalKeys)
			return elapsed, afterKeys, fmt.Errorf("scan failed: %w", err)
		}

		// If a result was found, submit it
		if res != nil {
			// Snapshot final nonce if a result was found
			atomic.StoreUint32(&currentNonce, res.Nonce)

			// Submit with per-call timeout
			sctx, scancel := context.WithTimeout(ctx, w.config.CheckpointTimeout)
			if err := w.client.SubmitResult(sctx, res.PrivateKey[:], res.Address.Hex()); err != nil {
				scancel()
				if errors.Is(err, ErrUnauthorized) {
					cancel()
					<-doneCh
					elapsed := time.Since(startTime)
					afterKeys := atomic.LoadUint64(&totalKeys)
					return elapsed, afterKeys, ErrUnauthorized
				}
				log.Printf("worker: failed to submit result: %v", err)
			} else {
				scancel()
			}
			foundResult = res
		}

		// Send a checkpoint for this chunk (reporting cumulative job-level metrics).
		// We use a 10s throttle to avoid flooding the server on fast PCs.
		if time.Since(lastCheckpointTime) >= minCheckpointInterval {
			err := w.sendChunkCheckpoint(ctx, lease.JobID, startTime, &currentNonce, &totalKeys)
			if err != nil {
				cancel()
				<-doneCh
				elapsed := time.Since(startTime)
				currentTk := atomic.LoadUint64(&totalKeys)
				return elapsed, currentTk, err
			}
			lastCheckpointTime = time.Now()
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
	// Use a background context with 10s timeout for final completion.
	bgCtx, bgCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer bgCancel()
	if err := w.client.CompleteBatch(bgCtx, lease.JobID, lease.NonceEnd, tk, startTime, elapsed.Milliseconds()); err != nil {
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

// sendChunkCheckpoint sends a checkpoint for a chunk and handles errors.
// It returns an error if the worker should stop processing the current lease.
func (w *Worker) sendChunkCheckpoint(ctx context.Context, jobID string, startTime time.Time, currentNonce *uint32, totalKeys *uint64) error {
	cctx, ccancel := context.WithTimeout(ctx, w.config.CheckpointTimeout)
	defer ccancel()

	currentTk := atomic.LoadUint64(totalKeys)
	currentDuration := time.Since(startTime).Milliseconds()
	currentNonceVal := atomic.LoadUint32(currentNonce)

	if err := w.client.UpdateCheckpoint(cctx, jobID, currentNonceVal, currentTk, startTime, currentDuration); err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return ErrUnauthorized
		}
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 410 {
			return ErrLeaseExpired
		}
		// Non-fatal checkpoint failure: log and continue.
		log.Printf("worker: checkpoint failed for job %s: %v", jobID, err)
		return nil
	}

	if !w.config.LogSampling {
		log.Printf("worker: checkpoint sent job=%s nonce=%d total_keys=%d", jobID, currentNonceVal, currentTk)
	}
	return nil
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
