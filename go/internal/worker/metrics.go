package worker

import "time"

// Metrics captures perf metrics reported by the worker for a batch or chunk.
type Metrics struct {
	StartedAt   time.Time
	DurationMs  int64
	KeysScanned uint64
}

// Throughput returns keys per second computed from KeysScanned and DurationMs.
// If DurationMs is zero or negative, throughput is 0 to avoid division by zero.
func (m Metrics) Throughput() float64 {
	if m.DurationMs <= 0 {
		return 0
	}
	secs := float64(m.DurationMs) / 1000.0
	return float64(m.KeysScanned) / secs
}

// DurationMsBetween returns the duration in milliseconds between two times.
func DurationMsBetween(start, end time.Time) int64 {
	return end.Sub(start).Milliseconds()
}
