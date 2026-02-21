# PC Worker Core Benchmarks

This document records the throughput and performance characteristics of the PC Worker scanning engine.

**Last Updated:** February 21, 2026
**Hardware Specs:** Intel(R) Xeon(R) CPU E5-2690 v4 @ 2.60GHz (14 Cores / 28 Threads)
**Go Version:** 1.26

---

## Core Operations (Isolated)

Measured by `internal/worker/crypto_bench_test.go`. These measure the raw cryptographic operations without loop overhead.

| Operation | Latency (ns/op) | Throughput (keys/sec) | Allocations (B/op) | Allocs/op |
|-----------|----------------|----------------------|--------------------|-----------|
| **ConstructPrivateKey** | 1.89 ns | 526,777,828 | 0 | 0 |
| **Derive (Standard)** | 61,304 ns | 16,312 | 641 | 13 |
| **Derive (Fast)**     | 50,341 ns | 19,865 | 97 | 0 |
| **Derive (Parallel-28)** | 2,651 ns | 377,147 | 0 | 0 |

---

## Scanning Loop (ScanRange)

Measured by `internal/worker/scanner_bench_test.go` with a **non-zero 28-byte prefix**. Using a zero prefix artificially inflates results due to `secp256k1` optimizations for small scalars.

| Range Size | Single-Threaded (keys/sec) | Parallel (28 Threads) (keys/sec) | Scaling Factor |
|------------|---------------------------|----------------------------------|----------------|
| **1,000**  | 21,307                    | 21,247                           | 1.0x           |
| **100,000**| 23,773                    | 34,924                           | 1.5x           |
| **1,000,000**| 23,533                  | 270,550                          | 11.5x          |

### Analysis

1. **Parallel Scaling:** On the 28-thread test machine (14 physical cores), the parallel scanner achieved ~270,550 keys/sec on a range of 1 million nonces. Scaling is efficient (~82%) relative to physical cores.
2. **Realistic Prefix:** Previous benchmarks (Feb 16) used a zero-prefix and showed ~60k keys/sec single-threaded. Real-world random prefixes result in ~23k keys/sec per core.
3. **Capacity Estimation:** A baseline throughput of 270k keys/sec implies ~972 million keys per hour.
4. **Master API Adjustment:** The Master API `maxBatchSize` has been increased from 10M to 4B to allow workers to request batches that actually cover ~1 hour of work.

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
