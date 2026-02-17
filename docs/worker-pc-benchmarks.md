# PC Worker Core Benchmarks

This document records the throughput and performance characteristics of the PC Worker scanning engine.

**Last Updated:** February 16, 2026
**Hardware Specs:** Intel(R) Xeon(R) CPU E5-2690 v4 @ 2.60GHz (28 Cores)
**Go Version:** 1.26

---

## Core Operations

| Operation | Latency (ns/op) | Throughput (keys/sec) | Allocations (B/op) | Allocs/op |
|-----------|----------------|----------------------|--------------------|-----------|
| **ConstructPrivateKey** | 1.87 ns | 534,473,543 | 0 | 0 |
| **Derive (Standard)** | 63,844 ns | 15,663 | 641 | 13 |
| **Derive (Fast)**     | 46,916 ns | 21,315 | 80 | 0 |
| **Derive (Parallel)** | 2,758 ns | 362,622 | 0 | 0 |

### How measured

Command used to reproduce core op micro-benchmarks:

```bash
cd /home/user/code/garnizeh/eth-scanner/go && \
	go test -run '^$' -bench='BenchmarkConstructPrivateKey|BenchmarkDerive' -benchmem ./internal/worker
```

Latest run (2026-02-16): measurements were collected on the same Intel E5-2690 v4 machine listed above; values shown in the table are the representative medians from those runs.

Notes:
- These microbenchmarks exercise hot-path functions in `internal/worker` and are single-core measurements unless marked Parallel.
- Use `-run '^$'` and `-benchmem` to reproduce identical measurement conditions.
---

## Scanning Loop (ScanRange)

Measured by scanning various nonce ranges. Values are per-core unless specified as Parallel.

| Range Size | Latency | Single-Threaded (keys/sec) | Parallel (keys/sec) |
|------------|---------|---------------------------|---------------------|
| **1,000**  | 165 ms  | 60,369                    | 52,067              |
| **100,000**| 1.71 s  | 58,340                    | 78,606              |
| **1,000,000**| 17.9 s| 55,866                    | 659,606             |

### Analysis

1. **Parallel Scaling:** On the 28-core test machine, the parallel scanner achieved ~660,000 keys/sec on a range of 1 million nonces, representing a ~11.8x speedup over single-threaded execution for that specific range size. 
2. **Optimizations:** The "Fast" derivation path reduces allocations to zero and provides a ~36% throughput improvement over the standard `go-ethereum` path.
3. **Bottleneck:** Cryptographic scalar multiplication (secp256k1) remains the primary bottleneck. The `ConstructPrivateKey` overhead is negligible (< 2ns).

---

## How to Run Benchmarks

To reproduce these results on your hardware, run the following command from the `go/` directory:

```bash
go test -v -run=^$ -bench=. -benchmem ./internal/worker
```

Measurements are performed with `-benchmem` to ensure zero allocations in the hot loop.

### Scanning loop measurement details

Command used to reproduce scanning-loop numbers:

```bash
cd /home/user/code/garnizeh/eth-scanner/go && \
	go test -run '^$' -bench='BenchmarkScanRange' -benchmem ./internal/worker
```

Latest run (2026-02-16): values in the "Scanning Loop" table were gathered with the above command; parallel numbers were measured using `GOMAXPROCS=28` on the test machine.

Notes & next steps:
- For stable results run multiple iterations with `-benchtime` and average the results.
- When comparing hardware, ensure `GOMAXPROCS` and process affinity are consistent between runs.

## Database: RecordWorkerStats benchmark

This micro-benchmark measures the cost of recording a worker checkpoint into the
SQLite database (the `RecordWorkerStats` code path exercised by server handlers
and tests).

Command used to reproduce the benchmark:

```bash
cd /home/user/code/garnizeh/eth-scanner/go && \
	go test ./internal/database -bench=BenchmarkRecordWorkerStats -benchmem -run '^$'
```

Latest run (2026-02-16):

```
BenchmarkRecordWorkerStats-28               2352            553101 ns/op
	 1128 B/op          27 allocs/op
```

Notes:
- ~553 µs per `RecordWorkerStats` call on the test machine (Intel E5-2690 v4).
- Allocations are modest (≈1.1 KiB, 27 allocs) — most cost is DB I/O and trigger
	execution inside SQLite.

Suggested next steps:
- Add this benchmark to CI for regression tracking.
- Run the benchmark on a dedicated host and average multiple runs (use
	`-benchtime` and repeat) for more stable numbers.
