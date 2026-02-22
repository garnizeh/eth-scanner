package worker

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Job describes a scanning job allocated by the master.
type Job struct {
	ID         int64
	Prefix28   [28]byte
	NonceStart uint32
	NonceEnd   uint32
	ExpiresAt  time.Time
}

// ScanResult is the result of a successful scan.
type ScanResult struct {
	PrivateKey [32]byte //nolint:gosec // false positive
	Address    common.Address
	Nonce      uint32
}

// ScanRange scans the nonce range [job.NonceStart, job.NonceEnd] (inclusive)
// for a private key whose derived address matches any of the targetAddresses.
// It periodically checks ctx for cancellation and returns ctx.Err() if canceled.
func ScanRange(ctx context.Context, job Job, targetAddresses []common.Address) (*ScanResult, error) {
	const checkInterval = 10000

	// If the start is greater than the end, nothing to scan.
	if job.NonceStart > job.NonceEnd {
		return nil, nil
	}

	// Hot loop optimization: pre-allocate buffers and hasher to avoid allocations
	// inside the iteration.
	hasher := crypto.NewKeccakState()
	var pubBuf [64]byte
	var hashBuf [32]byte
	var key [32]byte

	// Map lookup is fast, but for 1-3 addresses, a simple array iterate might be faster.
	// However, a map is more general and scales better if the list grows.
	targets := make(map[common.Address]bool, len(targetAddresses))
	for _, a := range targetAddresses {
		targets[a] = true
	}

	// Use a uint32 loop variable to avoid unsafe downcasts; maintain a
	// separate counter for periodic context checks so we don't overflow.
	var counter uint64
	for n := job.NonceStart; ; n++ {
		nonce := n

		if counter%checkInterval == 0 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("scan canceled: %w", ctx.Err())
			default:
			}
		}
		counter++

		// Reuse key buffer
		copy(key[:28], job.Prefix28[:])
		binary.BigEndian.PutUint32(key[28:], nonce)

		// Use fast, allocation-free derivation path
		addr, err := DeriveEthereumAddressFast(key, hasher, &pubBuf, &hashBuf)
		if err != nil {
			// skip invalid keys (zero or overflow)
			continue
		}

		if targets[addr] {
			return &ScanResult{
				PrivateKey: key,
				Address:    addr,
				Nonce:      nonce,
			}, nil
		}

		// If we've reached the inclusive end, stop the loop.
		if nonce == job.NonceEnd {
			break
		}
	}

	return nil, nil
}

// ScanRangeParallel partitions the job's nonce range and scans it using multiple
// goroutines (one per CPU core). It returns the first result found and cancels
// all other workers immediately.
// progressFn, if non-nil, is called to report progress where the first
// argument is the last scanned nonce (inclusive) and the second is the
// number of keys scanned in that chunk.
func ScanRangeParallel(ctx context.Context, job Job, targetAddresses []common.Address, progressFn func(nonce uint32, keys uint64), numWorkers int) (*ScanResult, error) {
	if numWorkers <= 0 {
		numWorkers = 1
	}

	if job.NonceStart > job.NonceEnd {
		return nil, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	const chunkSize uint32 = 1 << 16

	jobsCh := make(chan Job, numWorkers)
	resultCh := make(chan *ScanResult, 1)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup

	for range numWorkers {
		wg.Go(func() {
			for subJob := range jobsCh {
				result, err := ScanRange(ctx, subJob, targetAddresses)
				if err != nil {
					select {
					case errCh <- err:
					default:
					}
					cancel()
					return
				}
				// report progress for this chunk
				if progressFn != nil && result == nil {
					keys := uint64(subJob.NonceEnd - subJob.NonceStart + 1)
					progressFn(subJob.NonceEnd, keys)
				}
				if result != nil {
					// report progress up to found nonce
					if progressFn != nil {
						keys := uint64(result.Nonce - subJob.NonceStart + 1)
						progressFn(result.Nonce, keys)
					}
					select {
					case resultCh <- result:
					default:
					}
					cancel()
					return
				}
			}
		})
	}

	go func() {
		defer close(jobsCh)
		start := job.NonceStart
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			end := start + chunkSize - 1
			if end < start || end > job.NonceEnd {
				end = job.NonceEnd
			}

			subJob := job
			subJob.NonceStart = start
			subJob.NonceEnd = end

			select {
			case jobsCh <- subJob:
			case <-ctx.Done():
				return
			}

			if end == job.NonceEnd {
				return
			}
			start = end + 1
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	for {
		select {
		case result := <-resultCh:
			if result != nil {
				return result, nil
			}
		case err := <-errCh:
			if err != nil {
				return nil, err
			}
		case <-done:
			select {
			case result := <-resultCh:
				if result != nil {
					return result, nil
				}
			default:
			}

			select {
			case err := <-errCh:
				if err != nil {
					return nil, err
				}
			default:
			}

			if cause := context.Cause(ctx); cause != nil {
				return nil, fmt.Errorf("scan canceled: %w", cause)
			}
			return nil, nil
		}
	}
}

// Helper to extract nonce bytes if needed elsewhere.
func nonceBytesFromUint32(n uint32) [4]byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], n)
	return b
}
