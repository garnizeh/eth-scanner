package worker

import (
	"testing"
	"time"
)

func TestMetrics_Throughput_Normal(t *testing.T) {
	m := Metrics{
		StartedAt:   time.Now(),
		DurationMs:  2000, // 2 seconds
		KeysScanned: 4000,
	}
	th := m.Throughput()
	if th <= 0 {
		t.Fatalf("expected positive throughput, got %f", th)
	}
	if th != 2000 {
		t.Fatalf("unexpected throughput: got %f, want 2000", th)
	}
}

func TestMetrics_Throughput_ZeroDuration(t *testing.T) {
	m := Metrics{DurationMs: 0, KeysScanned: 100}
	if m.Throughput() != 0 {
		t.Fatalf("expected zero throughput for zero duration")
	}
}

func TestDurationMsBetween(t *testing.T) {
	start := time.Now()
	end := start.Add(1500 * time.Millisecond)
	ms := DurationMsBetween(start, end)
	if ms != 1500 {
		t.Fatalf("expected 1500ms, got %d", ms)
	}
}
