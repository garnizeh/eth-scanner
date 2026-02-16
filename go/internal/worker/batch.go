package worker

import "time"

// CalculateBatchSize computes the optimal batch size (nonce range)
// based on estimated throughput (keys per second) and a target duration.
// Returns the batch size as uint32 (max 2^32 - 1). If targetDuration is
// zero or negative, a default of 1 hour is used. If keysPerSecond is zero,
// a conservative fallback value is returned (1,000,000).
func CalculateBatchSize(keysPerSecond uint64, targetDuration time.Duration) uint32 {
	const (
		maxBatchSize = uint64(0xFFFFFFFF) // 2^32 - 1
		minFallback  = uint64(1000000)    // conservative minimum
		defaultSecs  = uint64(3600)       // 1 hour
	)

	if targetDuration <= 0 {
		targetDuration = time.Duration(defaultSecs) * time.Second
	}

	// Convert duration to seconds using float to avoid int64->uint64 casts
	secsFloat := targetDuration.Seconds()
	var secs uint64
	if secsFloat <= 0 {
		secs = defaultSecs
	} else {
		secs = uint64(secsFloat)
		if secs == 0 {
			secs = defaultSecs
		}
	}

	if keysPerSecond == 0 {
		return uint32(minFallback)
	}

	// Prevent overflow: if keysPerSecond > maxBatchSize/secs then clamp.
	if keysPerSecond > maxBatchSize/secs {
		return uint32(maxBatchSize)
	}

	batch := keysPerSecond * secs
	if batch == 0 {
		return uint32(minFallback)
	}
	if batch > maxBatchSize {
		return uint32(maxBatchSize)
	}
	return uint32(batch)
}

// AdjustBatchSize updates the current batch size based on observed actualDuration
// vs the desired targetDuration. It applies a smoothing factor alpha in [0,1]
// to avoid aggressive jumps. The result is clamped to [minBatchSize, maxBatchSize].
func AdjustBatchSize(current uint32, targetDuration, actualDuration time.Duration, minBatchSize, maxBatchSize uint32, alpha float64) uint32 {
	if current == 0 {
		current = minBatchSize
	}
	if actualDuration <= 0 || targetDuration <= 0 {
		return current
	}

	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}

	actualSec := actualDuration.Seconds()
	targetSec := targetDuration.Seconds()
	if actualSec <= 0 {
		return current
	}

	adjustment := targetSec / actualSec
	// smoothed factor: alpha * adjustment + (1-alpha) * 1.0
	factor := alpha*adjustment + (1-alpha)*1.0

	newf := float64(current) * factor
	if newf < float64(minBatchSize) {
		return minBatchSize
	}
	if newf > float64(maxBatchSize) {
		return maxBatchSize
	}
	return uint32(newf)
}
