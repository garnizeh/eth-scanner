package worker

import (
	"crypto/rand"
	"math/big"
	"time"
)

// Backoff implements exponential backoff with jitter.
type Backoff struct {
	minDelay time.Duration
	maxDelay time.Duration
	current  time.Duration
}

// NewBackoff creates a Backoff with provided min and max delays.
func NewBackoff(minDelay, maxDelay time.Duration) *Backoff {
	if minDelay <= 0 {
		minDelay = 1 * time.Second
	}
	if maxDelay <= 0 {
		maxDelay = 5 * time.Minute
	}
	return &Backoff{minDelay: minDelay, maxDelay: maxDelay, current: minDelay}
}

// Next returns the next backoff duration with ±25% jitter and doubles the current delay.
func (b *Backoff) Next() time.Duration {
	// Add jitter ±25% using crypto/rand for deterministic linting
	limit := new(big.Int).Lsh(big.NewInt(1), 53) // 2^53
	n, err := rand.Int(rand.Reader, limit)
	var frac float64
	if err == nil {
		frac = float64(n.Int64()) / float64(1<<53) // [0,1)
	} else {
		frac = 0.5
	}
	jitter := (frac - 0.5) * 0.5
	d := float64(b.current) * (1 + jitter)

	// Prepare next delay
	next := b.current * 2
	if next > b.maxDelay {
		next = b.maxDelay
	}
	b.current = next

	// Ensure returned duration is at least 0
	if d < 0 {
		d = 0
	}
	return time.Duration(d)
}

// Reset sets backoff to its minimum delay.
func (b *Backoff) Reset() {
	b.current = b.minDelay
}
