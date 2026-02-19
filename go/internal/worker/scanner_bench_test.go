package worker

import (
	"context"
	"runtime"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

type scanBenchCase struct {
	name     string
	nonceEnd uint32
}

var scanBenchCases = []scanBenchCase{
	{name: "small_1k", nonceEnd: 1_000},
	{name: "medium_100k", nonceEnd: 100_000},
	{name: "large_1m", nonceEnd: 1_000_000},
}

func BenchmarkScanRange_Single(b *testing.B) {
	target := common.Address{0x1} // practically no match; exercises full scan path
	ctx := context.Background()

	for _, tc := range scanBenchCases {
		b.Run(tc.name, func(b *testing.B) {
			job := Job{NonceStart: 0, NonceEnd: tc.nonceEnd}
			numKeys := uint64(tc.nonceEnd) + 1
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_, _ = ScanRange(ctx, job, target)
			}
			b.StopTimer()
			// Avoid integer overflow when converting b.N to uint64; compute in float64
			keysPerSec := float64(b.N) * float64(numKeys) / b.Elapsed().Seconds()
			b.ReportMetric(keysPerSec, "keys/sec")
		})
	}
}

func BenchmarkScanRange_Parallel(b *testing.B) {
	target := common.Address{0x1} // practically no match; exercises full scan path
	ctx := context.Background()

	for _, tc := range scanBenchCases {
		b.Run(tc.name, func(b *testing.B) {
			job := Job{NonceStart: 0, NonceEnd: tc.nonceEnd}
			numKeys := uint64(tc.nonceEnd) + 1
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_, _ = ScanRangeParallel(ctx, job, target, nil, runtime.NumCPU())
			}
			b.StopTimer()
			// Avoid integer overflow when converting b.N to uint64; compute in float64
			keysPerSec := float64(b.N) * float64(numKeys) / b.Elapsed().Seconds()
			b.ReportMetric(keysPerSec, "keys/sec")
		})
	}
}
