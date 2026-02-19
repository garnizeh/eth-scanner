# EthScanner Distributed (MVP)

EthScanner Distributed is an educational research project demonstrating a distributed key-scanning architecture (Master API + heterogeneous workers: PC and ESP32).

**Project Status:** 🟢 Phase 8 Complete - Core components (Master API, PC worker, and ESP32 firmware foundations) implemented; starting Phase 9 (Integration, Testing & Validation).

IMPORTANT: This project is for research/educational purposes only. Do NOT use it against real wallets with funds. Brute-forcing private keys is computationally infeasible and unethical when targeting active addresses.

## Quick Links
- **Documentation:** [docs/architecture/system-design-document.md](docs/architecture/system-design-document.md)
- **Database Optimization:** [docs/architecture/db-optimization-proposal.md](docs/architecture/db-optimization-proposal.md)
- **PC Worker Benchmarks:** [docs/worker-pc-benchmarks.md](docs/worker-pc-benchmarks.md)
- **API Reference:** Detailed in [System Design Document](docs/architecture/system-design-document.md#api-endpoints).
- **Tasks (Backlog):** [docs/tasks/backlog](docs/tasks/backlog)
- **Tasks (Done):** [docs/tasks/done](docs/tasks/done)
- **Tasks Overview:** [docs/tasks/OVERVIEW.md](docs/tasks/OVERVIEW.md)

## Getting Started (Linux)

### Prerequisites
- **Go 1.26+**
- **Git**

## Configuration
The Master API and PC Worker are configurable via environment variables. Set them in your shell or use the defaults in the `Makefile`.

Master (server) environment variables

| Variable | Description | Default |
|----------|-------------|---------|
| `MASTER_DB_PATH` | Path to the SQLite database file (Required) | `./data/eth-scanner.db` |
| `MASTER_PORT` | TCP port for the API server | `8080` |
| `MASTER_API_KEY` | Secret key for API authentication (optional) | (disabled if empty) |
| `MASTER_LOG_LEVEL`| Logging verbosity (`debug`, `info`, `warn`, `error`) | `info` |
| `MASTER_SHUTDOWN_TIMEOUT` | Graceful shutdown timeout (duration string) | `30s` |
| `MASTER_STALE_JOB_THRESHOLD` | Stale threshold (seconds) after which a processing job is considered abandoned by the background cleanup | `604800` (7 days) |
| `MASTER_CLEANUP_INTERVAL` | How often (seconds) the master runs the stale-job cleanup background task | `21600` (6 hours) |

Worker (PC) environment variables

| Variable | Description | Default |
|----------|-------------|---------|
| `WORKER_API_URL` | Base URL of the Master API (Required) | - |
| `WORKER_ID` | Worker identifier (auto-generated if empty) | auto-generated |
| `WORKER_API_KEY` | API key to send in `X-API-KEY` header (optional) | - |
| `WORKER_CHECKPOINT_INTERVAL` | Interval between automatic checkpoints (duration string) | `5m` |
| `WORKER_LEASE_GRACE_PERIOD` | Time subtracted from lease expiry to stop scanning early (duration string) | `30s` |

Adaptive batch-sizing (new)

These variables were added to support adaptive batch sizing implemented in the PC worker:

| Variable | Description | Default |
|----------|-------------|---------|
| `WORKER_TARGET_JOB_DURATION` | Desired batch processing time in seconds (target job duration) | `3600` (1 hour) |
| `WORKER_MIN_BATCH_SIZE` | Minimum allowed requested batch size (keys) | `100000` |
| `WORKER_MAX_BATCH_SIZE` | Maximum allowed requested batch size (keys) | `10000000` |
| `WORKER_BATCH_ADJUST_ALPHA` | Smoothing factor in [0,1] for batch-size adjustments (alpha) | `0.5` |
| `WORKER_INITIAL_BATCH_SIZE` | Optional initial batch size to start with (0 = auto-calc) | `0` (auto) |
| `WORKER_INTERNAL_BATCH_SIZE` | Internal chunk size (keys) processed between checkpoints by the worker | `1000000` |

Worker Statistics & Performance Monitoring

These variables control the multi-tier statistics architecture for dashboard analytics and long-term performance tracking:

| Variable | Description | Default |
|----------|-------------|---------|
| `WORKER_HISTORY_LIMIT` | Maximum raw history records to keep globally (Tier 1: real-time monitoring) | `10000` |
| `WORKER_DAILY_STATS_LIMIT` | Maximum daily aggregation records per worker (Tier 2: short-term trends) | `1000` |
| `WORKER_MONTHLY_STATS_LIMIT` | Maximum monthly aggregation records per worker (Tier 3: long-term trends) | `1000` |

**Note:** Worker lifetime statistics (Tier 4) have no cap—one permanent record per worker for leaderboards and cumulative totals.

Warning on very small limits: setting any of the `WORKER_*_LIMIT` values below `100` is supported but will emit a runtime warning and may cause rapid churn of historical data; values <= 0 are ignored and defaults are used.

See [Database Optimization Proposal](docs/architecture/db-optimization-proposal.md) for architecture details.

### Running the Master API
The database is initialized automatically with all necessary migrations on the first run. Ensure the directory for your database file exists.

```bash
cd go
# Optional: Create data directory
mkdir -p data

# Option A: Using Makefile (uses defaults)
make run-master

# Option B: Manual run with env vars
MASTER_DB_PATH=./data/eth-scanner.db go run ./cmd/master
```

### Authentication
Endpoints (except `/health`) require an `X-API-KEY` header if `MASTER_API_KEY` is configured.

```bash
curl -H "X-API-KEY: your-secret-key-here" http://localhost:8080/api/v1/jobs/lease
```

## Repository Layout
```
eth-scanner/
├── README.md
├── docs/                       # Comprehensive documentation
│   ├── architecture/           # System design and API contracts
│   ├── database/               # SQL schema and queries
│   └── tasks/                  # Task board (Backlog/Done)
├── go/                         # Master API & PC Worker (Go)
│   ├── cmd/                    # Entry points (master, worker-pc)
│   ├── internal/               # Core logic (database, config, server, jobs)
│   └── Makefile                # Development shortcuts
└── esp32/                      # ESP32 firmware (C++/Arduino) - PLANNED
```

## Database Architecture & Storage Optimization

EthScanner uses a **multi-tier statistics architecture** to prevent unbounded database growth while preserving comprehensive performance data for monitoring dashboards.

### Architecture Overview

Instead of storing every batch as a permanent job record, the system uses:

1. **Long-lived Jobs**: A single job record represents an entire prefix scan range, updated in-place via checkpoints.
2. **Multi-Tier Statistics**: Performance data automatically cascades through four storage tiers:

```
Tier 1: worker_history (raw detail, 10K global cap)
   ↓ automatic aggregation
Tier 2: worker_stats_daily (daily summaries, 1K per worker)
   ↓ automatic aggregation
Tier 3: worker_stats_monthly (monthly summaries, 1K per worker)
   ↓ automatic aggregation
Tier 4: worker_stats_lifetime (lifetime totals, 1 per worker, permanent)
```

### Benefits

- **Bounded Storage**: Database size remains constant (~2-3 MB) even with millions of checkpoints
- **No Data Loss**: Automatic aggregation preserves metrics before pruning
- **Multi-Scale Analysis**: Dashboard can query real-time, daily, monthly, or lifetime statistics
- **Worker Isolation**: Per-worker caps prevent individual workers from consuming all storage
- **~98.7% Space Savings**: Compared to storing every batch as a permanent record

### Configuration

Control retention limits via environment variables:

```bash
WORKER_HISTORY_LIMIT=10000         # Raw history (global cap)
WORKER_DAILY_STATS_LIMIT=1000      # Daily stats per worker
WORKER_MONTHLY_STATS_LIMIT=1000    # Monthly stats per worker
```

See [Database Optimization Proposal](docs/architecture/db-optimization-proposal.md) for complete technical details.

**Dashboard Integration:**  
The multi-tier statistics architecture is designed to power a comprehensive web-based monitoring dashboard (Phase 11). Each tier serves specific dashboard use cases:
- **Tier 1 (worker_history)**: Real-time monitoring, live throughput graphs, recent errors
- **Tier 2 (worker_stats_daily)**: 7-day trends, day-over-day comparison charts
- **Tier 3 (worker_stats_monthly)**: Long-term trends, year-over-year analysis
- **Tier 4 (worker_stats_lifetime)**: Worker leaderboards, all-time statistics, fleet overview

## Project Progress
- [x] **Phase 1: Foundation** - Repository structure and tooling.
- [x] **Phase 2: Database Layer** - Type-safe SQL with `sqlc` and pure Go SQLite.
- [x] **Phase 3: Core Infrastructure** - HTTP server, logging, and configuration.
- [x] **Phase 4: Job Management** - Lease, Checkpoint, and Complete logic.
- [x] **Phase 5: PC Worker - Core Implementation** - Completed
- [x] **Phase 6: PC Worker - Crypto & Scanning Engine** - Completed
- [x] **Phase 7: ESP32 Worker - Core Infrastructure** - Completed
- [x] **Phase 8: ESP32 Worker - Crypto & Computation** - Completed
- [ ] **Phase 9: Integration, Testing & Validation** - Next (in progress)
- [ ] **Phase 10: Documentation, Deployment & Monitoring** - (Planned)
- [ ] **Phase 11: Dashboard & Monitoring UI** - (Planned)
- [ ] **Phase A01: Performance & Optimization (Adhoc Tasks)** - Ongoing optimizations including:
  - [x] Worker-specific prefix affinity (vertical exhaustion)
  - [x] Master background cleanup for stale jobs
  - [x] Adaptive batch sizing based on throughput
   - [x] Database storage optimization with multi-tier statistics (A01-T040 to A01-T070) — completed; see `docs/tasks/done/A01-T070.md` for details

**A01 Summary:** Tasks A01-T010 through A01-T070 have been implemented and validated (integration tests, load tests, benchmarks, and documentation). The system now uses long-lived jobs and a four-tier statistics architecture to bound DB growth while preserving analytics.

## Development Commands (Go)
Within the `go/` directory:
- `make build`: Build binaries for master and worker.
- `make test`: Run all unit tests.
- `make fmt`: Format Go code.
- `make sqlc`: Re-generate database code from SQL definitions.

## ESP32 Developer Quickstart

Essentials for building, flashing and monitoring the ESP32 firmware (PlatformIO + ESP-IDF targets). These commands assume you're at the repo root.

Common PlatformIO commands (ESP32 DoIt DevKit v1):

```bash
# build for the default environment
pio run -e esp32doit-devkit-v1

# upload/flash to device (auto-detects serial port)
pio run -e esp32doit-devkit-v1 -t upload

# monitor serial output (115200 baud default)
pio device monitor -e esp32doit-devkit-v1

# run the unit tests for the esp32 test environment
pio test -e esp32doit-devkit-v1 -vv

# open menuconfig (sdkconfig)
pio run -e esp32doit-devkit-v1 -t menuconfig
```

Tips & recommendations:

- Use `pio device list` to confirm the serial port before flashing.
- When debugging NVS checkpointing, increase serial log verbosity in `menuconfig` and check `esp32/src/nvs_handler.c` for read/write flows.
- Keep networking tasks on Core 0 and the crypto hot-loop on Core 1. See `esp32/include/core_tasks.h` and `esp32/src/core_tasks.c` for task pinning patterns.
- Use `pio device monitor` while performing manual resets (EN button) to verify NVS restore and checkpoint resume behavior.
- For faster iteration enable `build_flags = -DDEBUG` in `platformio.ini` or toggle logging in `menuconfig`.
- If you encounter watchdog resets under heavy load, ensure the networking/watchdog task yields appropriately and NVS writes are batched (see `esp32/src/nvs_handler.c`).

Helpful platformio targets used in CI/dev flows:

- `pio run -e esp32doit-devkit-v1` — build
- `pio run -e esp32doit-devkit-v1 -t upload` — flash
- `pio test -e esp32doit-devkit-v1` — run unit tests

Hardware tips:

- Use a good USB cable and a reliable 5V supply when flashing multiple times; flaky power causes spurious failures.
- Press and hold BOOT only when required by your board for flashing (PlatformIO autodetect usually handles this).


## Ethics & Disclaimer
- Use only on synthetic or "dead" addresses (e.g., `0x000...dEaD`) for demos.
- This project demonstrates architectural patterns and fault-tolerance; it is not intended to be used for malicious activity.

## License
This project is licensed under the MIT License — see the [LICENSE](LICENSE) file.
