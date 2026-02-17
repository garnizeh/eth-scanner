# A01-T070: Integration test results and notes

This document captures the artifacts and instructions produced while implementing the integration and load tests for the optimized job management system (A01-T070).

Files added
- `go/internal/database/integration_optimization_test.go` — bounded jobs table, global retention and 15k load test
- `go/internal/database/acceptance_optimization_test.go` — per-worker daily/monthly load tests, aggregation verification, and benchmark for `RecordWorkerStats`
- `go/internal/server/e2e_crash_recovery_test.go` — end-to-end crash-recovery test (lease, checkpoint, lease expiry, resume)

How to run tests

Run all tests (slow):
```sh
cd go
go test ./... -v
```

Run just database acceptance tests:
```sh
cd go
go test ./internal/database -run Test -v
```

Run the crash-recovery e2e test:
```sh
cd go
go test ./internal/server -run TestE2E_CrashRecovery_LeaseAndResume -v
```

Notes
- Tests use in-memory SQLite for database package tests; e2e tests use a temporary file DB under `t.TempDir()`.
- Retention limits are configurable via environment variables: `WORKER_HISTORY_LIMIT`, `WORKER_DAILY_STATS_LIMIT`, `WORKER_MONTHLY_STATS_LIMIT` and the tests set these when needed.
- The benchmark `BenchmarkRecordWorkerStats` measures insert throughput of the `RecordWorkerStats` helper; run with `-bench` to collect metrics.

Next steps
- Add load harnesses for longer-running performance tests and CI integration.
- Update dashboards / queries docs once production metrics are available.
