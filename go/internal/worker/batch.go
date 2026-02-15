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
