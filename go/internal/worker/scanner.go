package worker

import (
	"context"
	"encoding/binary"
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
	// Use a uint64 loop index to avoid uint32 wraparound when NonceEnd is 0xFFFFFFFF.
	start := uint64(job.NonceStart)
	end := uint64(job.NonceEnd)

	const checkInterval = 10000

	for i := start; i <= end; i++ {
		nonce := uint32(i)

		if i%checkInterval == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}

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
	}

	return nil, nil
}

// Helper to extract nonce bytes if needed elsewhere.
func nonceBytesFromUint32(n uint32) [4]byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], n)
	return b
}
