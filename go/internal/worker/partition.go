package worker

import "math"

// NonceRange represents an inclusive nonce interval [Start, End].
type NonceRange struct {
	Start uint32
	End   uint32
}

// PartitionNonceRange divides the inclusive nonce range [start, end] into at
// most numWorkers contiguous partitions. If numWorkers <= 0 it defaults to 1.
// If the range is empty (start > end) it returns nil. When the range size
// is smaller than numWorkers the function reduces the worker count to the
// number of nonces so each partition has at most one nonce.
func PartitionNonceRange(start, end uint32, numWorkers int) []NonceRange {
	if start > end {
		return nil
	}
	if numWorkers <= 0 {
		numWorkers = 1
	}

	total := int64(end) - int64(start) + 1
	if total <= 0 {
		return nil
	}

	if int64(numWorkers) > total {
		numWorkers = int(total)
	}

	base := total / int64(numWorkers)
	rem := total % int64(numWorkers) // first `rem` partitions get an extra nonce

	parts := make([]NonceRange, 0, numWorkers)
	var offset int64
	for i := range numWorkers {
		add := base
		if int64(i) < rem {
			add++
		}
		s := int64(start) + offset
		e := s + add - 1

		// Defensive bounds check before narrowing to uint32
		if s < 0 || e < 0 || s > int64(math.MaxUint32) || e > int64(math.MaxUint32) {
			// Should not happen for valid uint32 inputs, but guard defensively
			return nil
		}

		parts = append(parts, NonceRange{Start: uint32(s), End: uint32(e)})
		offset += add
	}

	return parts
}
