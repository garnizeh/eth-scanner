package worker

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
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
	PrivateKey [32]byte
	Address    common.Address
	Nonce      uint32
}

// ScanRange scans the nonce range [job.NonceStart, job.NonceEnd] (inclusive)
// for a private key whose derived address equals targetAddr. It periodically
// checks ctx for cancellation and returns ctx.Err() if canceled.
func ScanRange(ctx context.Context, job Job, targetAddr common.Address) (*ScanResult, error) {
	const checkInterval = 10000

	// If the start is greater than the end, nothing to scan.
	if job.NonceStart > job.NonceEnd {
		return nil, nil
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

		key := ConstructPrivateKey(job.Prefix28, nonce)
		addr, err := DeriveEthereumAddress(key)
		if err != nil {
			// skip invalid/private keys
			continue
		}

		if addr == targetAddr {
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

// Helper to extract nonce bytes if needed elsewhere.
func nonceBytesFromUint32(n uint32) [4]byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], n)
	return b
}
