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
