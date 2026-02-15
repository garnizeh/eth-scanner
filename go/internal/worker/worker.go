package worker

import (
	"context"
	"fmt"
	"log"
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

	for {
		// Respect parent context cancellation
		select {
		case <-ctx.Done():
			log.Println("worker: context cancelled, shutting down")
			return ctx.Err()
		default:
		}

		// Calculate batch size (target ~1 hour)
		batchSize := CalculateBatchSize(w.measuredThroughput, 1*time.Hour)
		log.Printf("worker: requesting batch size %d (~1h)", batchSize)

		lease, err := w.client.LeaseBatch(ctx, batchSize)
		if err != nil {
			if err == ErrNoJobsAvailable {
				log.Println("worker: no jobs available, retrying after delay")
				select {
				case <-time.After(30 * time.Second):
					continue
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			if err == ErrUnauthorized {
				return fmt.Errorf("worker: lease failed: %w", err)
			}
			log.Printf("worker: lease request failed: %v; retrying", err)
			select {
			case <-time.After(10 * time.Second):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		log.Printf("worker: leased job %s nonce [%d,%d] expires=%s", lease.JobID, lease.NonceStart, lease.NonceEnd, lease.ExpiresAt)

		if err := w.processBatch(ctx, lease); err != nil {
			// If unauthorized bubbled up, stop worker
			if err == ErrUnauthorized {
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
	// Lease context tied to the lease expiration time
	leaseCtx, cancel := context.WithDeadline(ctx, lease.ExpiresAt)
	defer cancel()

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
				return
			case <-ticker.C:
				// Report checkpoint using parent ctx to avoid being cancelled by leaseCtx
				if err := w.client.UpdateCheckpoint(ctx, lease.JobID, currentNonce, totalKeys); err != nil {
					if err == ErrUnauthorized {
						// fatal
						log.Printf("worker: checkpoint unauthorized")
						return
					}
					log.Printf("worker: checkpoint failed: %v", err)
				} else {
					log.Printf("worker: checkpoint sent job=%s nonce=%d keys=%d", lease.JobID, currentNonce, totalKeys)
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
		return leaseCtx.Err()
	case <-time.After(2 * time.Second):
		// Simulate completion
		currentNonce = lease.NonceEnd
		totalKeys = uint64(lease.NonceEnd - lease.NonceStart + 1)
	}

	// Stop checkpoint goroutine and wait for it
	cancel()
	<-doneCh

	// Complete the batch
	if err := w.client.CompleteBatch(ctx, lease.JobID, lease.NonceEnd, totalKeys); err != nil {
		if err == ErrUnauthorized {
			return ErrUnauthorized
		}
		return fmt.Errorf("failed to complete batch: %w", err)
	}

	return nil
}
