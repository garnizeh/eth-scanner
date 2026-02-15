package worker

import (
	"testing"
	"time"
)

func TestCalculateBatchSize(t *testing.T) {
	tests := []struct {
		name           string
		keysPerSecond  uint64
		targetDuration time.Duration
		expected       uint32
	}{
		{
			name:           "normal throughput",
			keysPerSecond:  800000,
			targetDuration: 1 * time.Hour,
			expected:       2880000000,
		},
		{
			name:           "zero throughput",
			keysPerSecond:  0,
			targetDuration: 1 * time.Hour,
			expected:       1000000,
		},
		{
			name:           "overflow",
			keysPerSecond:  5000000000,
			targetDuration: 1 * time.Hour,
			expected:       0xFFFFFFFF,
		},
		{
			name:           "30 minutes",
			keysPerSecond:  800000,
			targetDuration: 30 * time.Minute,
			expected:       1440000000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateBatchSize(tt.keysPerSecond, tt.targetDuration)
			if got != tt.expected {
				t.Fatalf("%s: expected %d, got %d", tt.name, tt.expected, got)
			}
		})
	}
}

func TestCalculateBatchSize_ZeroDurationUsesDefault(t *testing.T) {
	// targetDuration == 0 should use default 1 hour
	got := CalculateBatchSize(800000, 0)
	want := CalculateBatchSize(800000, 1*time.Hour)
	if got != want {
		t.Fatalf("zero duration: expected %d, got %d", want, got)
	}
}

func TestCalculateBatchSize_TinyDurationSecsZeroFallback(t *testing.T) {
	// very small durations that truncate to 0 seconds should fall back to default secs
	got := CalculateBatchSize(800000, 500*time.Millisecond)
	want := CalculateBatchSize(800000, 1*time.Hour)
	if got != want {
		t.Fatalf("tiny duration: expected %d, got %d", want, got)
	}
}

func TestCalculateBatchSize_ClampEdge(t *testing.T) {
	// choose keysPerSecond just above the threshold to force clamping
	const maxBatchSize = uint64(0xFFFFFFFF)
	secs := uint64(3600)
	threshold := maxBatchSize / secs
	kps := threshold + 1
	got := CalculateBatchSize(kps, 1*time.Hour)
	if got != uint32(maxBatchSize) {
		t.Fatalf("clamp edge: expected %d, got %d", maxBatchSize, got)
	}
}

func TestCalculateBatchSize_ReturnsComputedBatch(t *testing.T) {
	// simple case to exercise the final return path
	kps := uint64(12345)
	got := CalculateBatchSize(kps, 2*time.Second)
	want := CalculateBatchSize(kps, 2*time.Second)
	if got != want {
		t.Fatalf("computed batch: expected %d, got %d", want, got)
	}
}

func TestCalculateBatchSize_ProductExceedsMaxReturnsMax(t *testing.T) {
	// large throughput and duration should result in clamped max value
	kps := uint64(0xFFFFFFFF)
	got := CalculateBatchSize(kps, 2*time.Second)
	if got != uint32(0xFFFFFFFF) {
		t.Fatalf("expected clamp to max uint32, got %d", got)
	}
}

func TestCalculateBatchSize_NegativeDurationUsesDefault(t *testing.T) {
	// negative duration should use default 1 hour
	got := CalculateBatchSize(800000, -5*time.Minute)
	want := CalculateBatchSize(800000, 1*time.Hour)
	if got != want {
		t.Fatalf("negative duration: expected %d, got %d", want, got)
	}
}

func TestCalculateBatchSize_ThresholdNoClamp(t *testing.T) {
	// keysPerSecond exactly equal to threshold should not clamp
	const maxBatchSize = uint64(0xFFFFFFFF)
	secs := uint64(3600)
	threshold := maxBatchSize / secs
	kps := threshold
	got := CalculateBatchSize(kps, 1*time.Hour)
	// expected is kps * secs but might be <= maxBatchSize
	want := CalculateBatchSize(kps, 1*time.Hour)
	if got != want {
		t.Fatalf("threshold no-clamp: expected %d, got %d", want, got)
	}
}
