package worker

import (
	"context"
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
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_, _ = ScanRange(ctx, job, target)
			}
		})
	}
}

func BenchmarkScanRange_Parallel(b *testing.B) {
	target := common.Address{0x1} // practically no match; exercises full scan path
	ctx := context.Background()

	for _, tc := range scanBenchCases {
		b.Run(tc.name, func(b *testing.B) {
			job := Job{NonceStart: 0, NonceEnd: tc.nonceEnd}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_, _ = ScanRangeParallel(ctx, job, target)
			}
		})
	}
}
