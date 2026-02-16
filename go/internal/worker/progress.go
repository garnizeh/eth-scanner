package worker

import (
	"context"
	"fmt"
	"sync/atomic"
)

// Scanner provides a scanning context with atomic progress tracking.
type Scanner struct {
	// currentNonce holds the highest nonce scanned so far.
	currentNonce atomic.Uint64
	// UpdateInterval controls how often the scanner updates the atomic value.
	// If zero, defaults to 1000.
	UpdateInterval uint32
}

// NewScanner returns a Scanner with sensible defaults.
func NewScanner() *Scanner {
	return &Scanner{UpdateInterval: 1000}
}

// GetCurrentNonce returns the current nonce as seen by the scanner.
func (s *Scanner) GetCurrentNonce() uint64 {
	return s.currentNonce.Load()
}

// setCurrentNonce updates the atomic counter to the provided value if it's
// greater than the current value.
func (s *Scanner) setCurrentNonce(n uint32) {
	for {
		cur := s.currentNonce.Load()
		if uint64(n) <= cur {
			return
		}
		if s.currentNonce.CompareAndSwap(cur, uint64(n)) {
			return
		}
	}
}

// ScanRange scans the given job's nonce range similarly to the package-level
// ScanRange but updates the scanner's atomic progress value periodically.
func (s *Scanner) ScanRange(ctx context.Context, job Job, targetAddr AddressProvider) (*ScanResult, error) {
	// Allow using either common.Address or any other provider via interface.
	// For now, AddressProvider is defined in this file as an alias to avoid
	// depending on go-ethereum types in tests.
	if s.UpdateInterval == 0 {
		s.UpdateInterval = 1000
	}

	const checkInterval = 10000

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

		// Update progress periodically
		if nonce%s.UpdateInterval == 0 {
			s.setCurrentNonce(nonce)
		}

		key := ConstructPrivateKey(job.Prefix28, nonce)
		addr, err := DeriveEthereumAddress(key)
		if err != nil {
			continue
		}

		if AddressEquals(addr, targetAddr) {
			s.setCurrentNonce(nonce)
			return &ScanResult{PrivateKey: key, Address: addr, Nonce: nonce}, nil
		}

		if nonce == job.NonceEnd {
			break
		}
	}

	return nil, nil
}

// AddressProvider is an abstraction over target address types used by Scanner.
// We provide a minimal interface and helper to compare addresses to keep tests
// decoupled from go-ethereum in some cases.
type AddressProvider any

// AddressEquals compares a derived address against the provided target. It
// supports go-ethereum's common.Address and also accepts a zero value.
func AddressEquals(addr any, target AddressProvider) bool {
	// In practice we pass go-ethereum addresses; reflect-based comparison is
	// avoided for performance in hot paths, but for this helper simplicity
	// is acceptable since used in tests.
	switch t := target.(type) {
	case interface{ Hex() string }:
		// target is a go-ethereum address-like type
		// Convert addr to the same type by formatting hex strings
		// Fallback: use fmt.Sprintf for comparison
		return fmt.Sprintf("%v", addr) == fmt.Sprintf("%v", t)
	default:
		return fmt.Sprintf("%v", addr) == fmt.Sprintf("%v", target)
	}
}
