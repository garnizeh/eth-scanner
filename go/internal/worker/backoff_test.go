package worker

import (
	"fmt"
	"testing"
	"time"
)

func TestBackoff_NextAndReset(t *testing.T) {
	b := NewBackoff(1*time.Second, 10*time.Second)

	d1 := b.Next()
	if d1 < 750*time.Millisecond || d1 > 1250*time.Millisecond {
		t.Fatalf("expected ~1s ±25%%, got %v", d1)
	}

	d2 := b.Next()
	if d2 < 1500*time.Millisecond || d2 > 2500*time.Millisecond {
		t.Fatalf("expected ~2s ±25%%, got %v", d2)
	}

	// advance several times to cap
	for range 10 {
		_ = b.Next()
	}
	dc := b.Next()
	if dc > 12500*time.Millisecond {
		t.Fatalf("expected capped near 10s ±25%%, got %v", dc)
	}

	b.Reset()
	dr := b.Next()
	if dr < 750*time.Millisecond || dr > 1250*time.Millisecond {
		t.Fatalf("expected ~1s after reset, got %v", dr)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"500", &APIError{StatusCode: 500}, true},
		{"503", &APIError{StatusCode: 503}, true},
		{"429", &APIError{StatusCode: 429}, true},
		{"400", &APIError{StatusCode: 400}, false},
		{"404", &APIError{StatusCode: 404}, false},
		{"network", fmt.Errorf("dial tcp: connection refused"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryable(tt.err)
			if got != tt.want {
				t.Fatalf("isRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
