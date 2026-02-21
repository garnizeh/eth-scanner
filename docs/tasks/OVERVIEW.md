# EthScanner Distributed - Project Tasks Overview

**Version:** 1.0  
**Date:** February 14, 2026  
**Status:** Active

---

## Purpose

This document provides a comprehensive overview of all project phases and tasks required to implement the EthScanner Distributed system (MVP). Each task is designed to be small, focused, and independently verifiable.

---

## Task Naming Convention

**Format:** `P{Phase}-T{Task}` for planned phases, `A{Phase}-T{Task}` for adhoc/optimization tasks

- **Phase Number:** 2-digit zero-padded (P01, P02, ..., P99 for planned; A01, A02, ... for adhoc)
- **Task Number:** 3-digit zero-padded with increments of 10 (T010, T020, T030, ...)
- **Subtasks:** Use letter suffixes (T010a, T010b) or single-digit increments (T011, T012)

**Why increments of 10?**  
This allows insertion of new tasks between existing ones without renumbering:
- Insert `P01-T015` between `P01-T010` and `P01-T020`
- Insert `P01-T025` between `P01-T020` and `P01-T030`

**Examples:**
- `P01-T010` → Phase 1, Task 10 (planned phase)
- `P03-T050a` → Phase 3, Task 50, subtask a
- `P05-T025` → Phase 5, Task 25 (inserted between T020 and T030)
- `A01-T010` → Adhoc Phase 1, Task 10 (performance optimization)
- `A02-T020` → Adhoc Phase 2, Task 20 (bug fix or refinement)

**Adhoc Tasks:**
Adhoc tasks (A0X-TXXX) are created on-demand during development to address:
- Performance optimizations discovered during testing
- Bug fixes or refinements not part of the original plan
- Technical debt cleanup
- Improvements to existing features

---

## Task Workflow

1. **Backlog:** New tasks are created in `docs/tasks/backlog/` with filename matching task ID (e.g., `P01-T010.md`)
2. **In Progress:** Add `[IN PROGRESS]` marker to task file header while working
3. **Done:** Move completed task file from `backlog/` to `done/` folder
4. **Sequential Execution:** Work on tasks within each phase sequentially by task number

---

## Project Phases

### Phase 01: Project Foundation & Setup
**Goal:** Establish repository structure, tooling, and development environment.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P01-T010 | Initialize Go module in `go/` folder | High | None | ✅ Completed |
| P01-T020 | Create `internal/` folder structure (api, database, jobs, worker, config) | High | P01-T010 | ✅ Completed |
| P01-T030 | Create `.gitignore` for Go, SQLite, IDE files | High | None | ✅ Completed |
| P01-T040 | Set up `sqlc` configuration file (`sqlc.yaml`) | High | P01-T010 | ✅ Completed |
| P01-T050 | Create `scripts/init-db.sh` (initialize SQLite database with schema) | Medium | None | ✅ Completed |
| P01-T060 | Verify Go toolchain (no CGO requirement for `modernc.org/sqlite`) | High | P01-T010 | ✅ Completed |
| P01-T070 | Create basic `Makefile` or `justfile` for common tasks (build, test, run) | Low | P01-T010 | ✅ Completed |
| P01-T080 | Setup GitHub Actions CI Workflow | High | P01-T070 | ✅ Completed |

---

### Phase 02: Database Layer Implementation
**Goal:** Implement complete database schema with type-safe query layer using `sqlc`.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P02-T010 | Validate `go/internal/database/sql/001_schema.sql` against SDD requirements | High | None | ✅ Completed |
| P02-T020 | Create `go/internal/database/sql/queries.sql` with all sqlc queries | High | P02-T010 | ✅ Completed |
| P02-T030 | Configure `sqlc.yaml` for code generation | High | P01-T040 | ✅ Completed |
| P02-T040 | Run `sqlc generate` and verify generated code in `internal/database/` | High | P02-T020, P02-T030 | ✅ Completed |
| P02-T050 | Implement `internal/database/db.go` (SQLite connection with `modernc.org/sqlite`) | High | P02-T040 | ✅ Completed |
| P02-T060 | Create database initialization function (apply schema on first run) | High | P02-T050 | ✅ Completed |
| P02-T070 | Implement database migration versioning using goose lib | Low | P02-T060 | ✅ Completed |
| P02-T080 | Write unit tests for database layer (connection, schema application) | Medium | P02-T060 | ✅ Completed |

---

### Phase 03: Master API - Core Infrastructure
**Goal:** Set up HTTP server with routing, middleware, and basic endpoints.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P03-T010 | Implement `internal/config/config.go` (load from env/file: port, DB path) | High | P01-T020 | ✅ Completed |
| P03-T020 | Implement `internal/server/server.go` (HTTP server setup with `net/http` or `chi`) | High | P03-T010 | ✅ Completed |
| P03-T030 | Implement `internal/server/middleware.go` (logging, CORS, request ID) | Medium | P03-T020 | ✅ Completed |
| P03-T040 | Implement `internal/server/routes.go` (route registration) | High | P03-T020 | ✅ Completed |
| P03-T050 | Create `GET /health` endpoint (basic health check) | High | P03-T040 | ✅ Completed |
| P03-T060 | Create `cmd/master/main.go` entry point (wire dependencies, start server) | High | P03-T020 | ✅ Completed |
| P03-T070 | Test server startup and `/health` endpoint manually | High | P03-T060 | ✅ Completed |
| P03-T080 | Implement graceful shutdown with `context.Context` | Medium | P03-T060 | ✅ Completed |

---

### Phase 04: Master API - Job Management (Dynamic Batching)
**Goal:** Implement all job-related API endpoints with dynamic batching logic.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P04-T010 | Implement `internal/jobs/manager.go` skeleton (job manager struct) | High | P02-T050 | ✅ Completed |
| P04-T020 | Implement job lease logic: find available/expired job from DB | High | P04-T010, P02-T040 | ✅ Completed |
| P04-T030 | Implement nonce range allocation: get next available range for prefix | High | P04-T020 | ✅ Completed |
| P04-T040 | Implement batch creation: create new job with dynamic batch size | High | P04-T030 | ✅ Completed |
| P04-T050 | Create `POST /api/v1/jobs/lease` handler (request validation + lease logic) | High | P04-T040 | ✅ Completed |
| P04-T060 | Implement UTC timestamp handling for `expires_at` (no `time.Local`) | High | P04-T050 | ✅ Completed |
| P04-T070 | Create `PATCH /api/v1/jobs/{id}/checkpoint` handler (update progress) | High | P04-T010, P02-T040 | ✅ Completed |
| P04-T080 | Implement worker_id validation in checkpoint endpoint | High | P04-T070 | ✅ Completed |
| P04-T090 | Create `POST /api/v1/jobs/{id}/complete` handler (mark job completed) | High | P04-T010, P02-T040 | ✅ Completed |
| P04-T100 | Implement final_nonce validation (must equal nonce_end) | High | P04-T090 | ✅ Completed |
| P04-T110 | Create `POST /api/v1/results` handler (submit found private key) | Medium | P04-T010, P02-T040 | ✅ Completed |
| P04-T120 | Create `GET /api/v1/stats` handler (return statistics from view) | Low | P04-T010, P02-T040 | ✅ Completed |
| P04-T130 | Write integration tests for lease endpoint (pending/expired jobs) | High | P04-T050 | ✅ Completed |
| P04-T140 | Write integration tests for checkpoint endpoint | Medium | P04-T070 | ✅ Completed |
| P04-T150 | Write integration tests for complete endpoint | Medium | P04-T090 | ✅ Completed |
| P04-T160 | Add API key middleware for master API | Medium | P04-T150 | ✅ Completed |

---

### Phase 05: PC Worker - Core Implementation
**Goal:** Build PC worker foundation with batch management and API client.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P05-T010 | Implement `internal/worker/config.go` (worker config: API URL, worker ID) | High | None | ✅ Completed |
| P05-T020 | Implement `internal/worker/client.go` (HTTP client for Master API) | High | P05-T010 | ✅ Completed |
| P05-T030 | Implement `LeaseBatch()` function (POST /api/v1/jobs/lease) | High | P05-T020 | ✅ Completed |
| P05-T040 | Implement `UpdateCheckpoint()` function (PATCH /api/v1/jobs/{id}/checkpoint) | High | P05-T020 | ✅ Completed |
| P05-T050 | Implement `CompleteBatch()` function (POST /api/v1/jobs/{id}/complete) | High | P05-T020 | ✅ Completed |
| P05-T060 | Implement `SubmitResult()` function (POST /api/v1/results) | Medium | P05-T020 | ✅ Completed |
| P05-T070 | Implement batch size calculator using `runtime.NumCPU()` | High | None | ✅ Completed |
| P05-T080 | Implement worker main loop (lease → process → complete) | High | P05-T030, P05-T050 | ✅ Completed |
| P05-T090 | Implement retry logic with exponential backoff (when no jobs available) | Medium | P05-T080 | ✅ Completed |
| P05-T100 | Create `cmd/worker-pc/main.go` entry point | High | P05-T080 | ✅ Completed |

---

### Phase 06: PC Worker - Crypto & Scanning Engine
**Goal:** Implement cryptographic key generation, scanning, and checkpointing.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P06-T010 | Implement `internal/worker/crypto.go` (import `go-ethereum/crypto`) | High | None | ✅ Completed |
| P06-T020 | Implement `DeriveEthereumAddress()` function (private key → address) | High | P06-T010 | ✅ Completed |
| P06-T030 | Implement key construction (prefix_28 + nonce little-endian) | High | P06-T020 | ✅ Completed |
| P06-T040 | Implement `internal/worker/scanner.go` (nonce range scanning) | High | P06-T030 | ✅ Completed |
| P06-T050 | Implement worker pool with goroutines (`runtime.NumCPU()` workers) | High | P06-T040 | ✅ Completed |
| P06-T060 | Implement nonce range partitioning across workers | High | P06-T050 | ✅ Completed |
| P06-T070 | Implement atomic progress tracking (`atomic.Uint64` for current_nonce) | High | P06-T060 | ✅ Completed |
| P06-T080 | Implement checkpoint goroutine (periodic PATCH every 5 minutes) | High | P06-T070, P05-T040 | ✅ Completed |
| P06-T090 | Implement context cancellation for lease expiration | High | P06-T050 | ✅ Completed |
| P06-T100 | Optimize crypto loop (buffer reuse, minimize allocations) | Medium | P06-T040 | ✅ Completed |
| P06-T110 | Write benchmarks for key scanning throughput (keys/sec) | Medium | P06-T040 | ✅ Completed |
| P06-T120 | Replace worker simulation with real job processing | High | P06-T100, P06-T110 | ✅ Completed |

---

### Phase 07: ESP32 Worker - Core Infrastructure
**Goal:** Set up ESP32 firmware with WiFi, HTTP client, and NVS persistence.

| Task ID | Description | Priority | Dependencies | Status |
|---------|-------------|----------|--------------|--------|
| P07-T010 | Initialize PlatformIO project with `framework = espidf` and define `src/main.c`. | High | None | ✅ Completed |
| P07-T020 | Configure `Kconfig.projbuild` for **Menuconfig** integration (WiFi/API URL setup). | High | P07-T010 | ✅ Completed |
| P07-T030 | Implement **ESP-NETIF** WiFi handler with Event Loop (Auto-reconnect & Backoff). | High | P07-T020 | ✅ Completed |
| P07-T040 | Implement `esp_http_client` wrapper for Master API communication (POST/PATCH). | High | P07-T030 | ✅ Completed |
| P07-T050 | Initialize **NVS (Non-Volatile Storage)** and obtain partition handles. | High | P07-T010 | ✅ Completed |
| P07-T060 | Implement `save_checkpoint()` using `nvs_set_blob` for atomic job persistence. | High | P07-T050 | ✅ Completed |
| P07-T070 | Implement `load_checkpoint()` to restore state during `app_main` boot sequence. | High | P07-T050 | ✅ Completed |
| P07-T080 | Implement startup benchmark using `esp_timer_get_time()` for µs precision. | High | P07-T010 | ✅ Completed |
| P07-T090 | Implement Batch Size Calculator (Keys/sec logic for 1-hour lease windows). | High | P07-T080 | ✅ Completed |
| P07-T100 | Define Global State `struct` (prefix, nonce range, targets) in `shared_types.h`. | High | P07-T010 | ✅ Completed |

---

### Phase 08: ESP32 Worker - Crypto & Computation
**Goal:** Implement dual-core FreeRTOS tasks with optimized crypto hot loop.

| Task ID | Description | Priority | Dependencies | Status |
|---------|-------------|----------|--------------|--------|
| P08-T010 | Integrate `trezor-crypto` or `micro-ecc` as a **CMake component** (Xtensa optimized). | High | P07-T010 | ✅ Completed |
| P08-T020 | Implement `keccak256` hashing (utilizing ESP32 SHA Hardware Acceleration). | High | P08-T010 | ✅ Completed |
| P08-T030 | Implement `derive_eth_address()` (secp256k1 point multiplication). | High | P08-T020 | ✅ Completed |
| P08-T040 | Spawn **Core 0 Task** (`xTaskCreatePinnedToCore`) for Networking & Watchdog. | High | P07-T040 | ✅ Completed |
| P08-T050 | Implement Job Lease logic on Core 0 (Inter-task signaling via **Task Notifications**). | High | P08-T040 | ✅ Completed |
| P08-T060 | Set up FreeRTOS Timer for periodic background checkpointing (every 60s). | High | P08-T040 | ✅ Completed |
| P08-T070 | Spawn **Core 1 Task** with highest priority for the computational hot loop. | High | P08-T030 | ✅ Completed |
| P08-T080 | Implement optimized Nonce loop (direct byte manipulation, avoiding `sprintf`). | High | P08-T070 | ✅ Completed |
| P08-T090 | Implement binary address comparison using `memcmp` for zero-overhead validation. | High | P08-T080 | ✅ Completed |
| P08-T100 | Implement result submission (Core 1 notifies Core 0 via **FreeRTOS Queue**). | High | P08-T090 | ✅ Completed |
| P08-T110 | Optimize memory: Use `StaticTask_t` for worker tasks to ensure no heap churn. | Medium | P08-T070 | ✅ Completed |
| P08-T120 | Validate NVS recovery and Task Watchdog (TWDT) resilience under 100% CPU load. | High | P07-T070 | ✅ Completed |
| P08-T130 | Long-haul Stress Test (30-60m running against Master API + manual resets). | High | P08-T120 | ✅ Completed |

## Technical Notes

### Dual-Core Logic
* **Core 0 (System Core):** Handles WiFi, HTTP requests, TCP/IP stack, and NVS writes. This prevents "Watchdog Reset" errors and network dropouts.
* **Core 1 (Worker Core):** Dedicated 100% to the `while(1)` crypto loop. No network calls should be made here to prevent latency spikes.

### Build System
* Projects use `CMakeLists.txt` for dependency management.
* Hardware configuration (Clock speed, Flash mode) is managed via `sdkconfig` generated by `pio run -t menuconfig`.

---

### Phase 09: Integration, Testing & Validation
**Goal:** Ensure all components work together correctly with comprehensive testing.

| Task ID | Description | Priority | Dependencies | Status |
|---------|-------------|----------|--------------|--------|
| P09-T010 | Write unit tests for `internal/jobs/manager.go` | High | P04-T010 | ✅ Completed |
| P09-T020 | Write unit tests for `internal/worker/crypto.go` | High | P06-T020 | ✅ Completed |
| P09-T030 | Write unit tests for nonce range allocation logic | High | P04-T030 | ✅ Completed |
| P09-T040 | Write integration test: Master API + SQLite (end-to-end lease flow) | High | P04-T050 | ✅ Completed |
| P09-T050 | Write integration test: PC worker + Master API (full batch cycle) | High | P06-T080, P04-T050 | ✅ Completed |
| P09-T060 | Test lease expiration and job re-assignment | High | P04-T050 | ✅ Completed |
| P09-T070 | Test checkpoint recovery (worker crashes mid-batch) | High | P06-T080 | ✅ Completed |
| P09-T080 | Benchmark PC worker throughput (keys/sec on reference hardware) | Medium | P06-T110 | ✅ Completed |
| P09-T090 | Test ESP32 firmware on actual hardware (full cycle) | High | P08-T120 | ✅ Completed |
| P09-T100 | Test ESP32 NVS checkpoint recovery on power loss | High | P08-T120 | ✅ Completed |
| P09-T110 | Validate all API endpoints with Postman/curl scripts | Medium | P04-T120 | ✅ Completed |
| P09-T120 | Load test: multiple concurrent workers (10+ workers) | Low | P09-T050 | ✅ Completed |

---

### Phase 10: Dashboard & Monitoring UI
**Goal:** Build a lightweight, real-time dashboard using Go Templates, HTMX, and WebSockets. All UI assets (templates, CSS, JS) must be **embedded** into the Go binary using the `embed` package for single-file distribution.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P10-T010 | Setup HTMX and Go Template infrastructure with `embed.FS` in `internal/server/ui/` | High | None |
| P10-T020 | Implement simple UI Authentication (Password via environment variable) | High | P10-T010 |
| P10-T030 | Implement WebSocket hub in Master API for real-time metrics broadcast | High | P10-T020 |
| P10-T040 | Create main dashboard layout (Base template + Navigation) | High | P10-T030 |
| P10-T050 | Implement "Active Workers" component (HTMX swap on WebSocket message) | High | P10-T040 |
| P10-T060 | Implement "Live Throughput" chart using minimal JS (uPlot or Chart.js) | High | P10-T050 |
| P10-T070 | Implement "Daily Performance" view (Server-side rendered charts) | High | P10-T040 |
| P10-T080 | Implement "Monthly & Lifetime" statistics views | Medium | P10-T070 |
| P10-T090 | Implement "Jobs Overview" component with progress bars | High | P10-T040 |
| P10-T100 | Implement "Worker Detail" page (SSR fragment) | Medium | P10-T070 |
| P10-T110 | Add responsive design using Tailwind CSS (via CDN or standalone CLI) | Low | P10-T040 |
| P10-T120 | Implement "Error Log" and alert notifications | Medium | P10-T050 |
| P10-T130 | Add export functionality (CSV/JSON download) | Low | P10-T080 |
| P10-T140 | Write documentation for dashboard setup and local development | Medium | All |

**Dashboard Features Overview:**

**Authentication & Security:**
- **Simple Login**: Password protection using environment variable configured on master server
- **No DB Requirement**: No database needed for user sessions (JWT or simple cookie based)
- **Protected Endpoints**: All statistics and monitoring views behind login

**Real-time Monitoring (Tier 1 - worker_history):**
- Active workers list with last-seen timestamps
- Live throughput graph (keys/second)
- Recent errors and warnings
- Current batch progress

**Short-term Analytics (Tier 2 - worker_stats_daily):**
- 7-day performance trends per worker
- Day-over-day comparison charts
- Worker efficiency metrics
- Daily batch completion counts

**Long-term Analytics (Tier 3 - worker_stats_monthly):**
- Monthly performance trends
- Year-over-year comparisons
- Seasonal pattern detection
- Historical throughput analysis

**Lifetime Statistics (Tier 4 - worker_stats_lifetime):**
- All-time worker leaderboard
- Total keys scanned by worker
- Best performing workers (avg keys/sec)
- Worker uptime and activity history
- Fleet-wide cumulative statistics

**Jobs & Prefix Tracking:**
- Active job list with progress bars
- Prefix completion percentage
- Estimated time to completion
- Job assignment history

**Technology Recommendations:**
- **Backend:** Go Templates (standard library `html/template`)
- **Distribution:** Go `embed` package to bundle all UI assets into a single binary
- **Frontend Interaction:** [HTMX](https://htmx.org/) for AJAX/SSE/WebSocket swaps
- **Real-time:** WebSockets for live metrics broadcast
- **Charting:** [uPlot](https://github.com/leeoniya/uPlot) or [Chart.js](https://www.chartjs.org/) (minimal JS weight)
- **CSS:** Tailwind CSS (via CDN or standalone CLI to avoid Node.js)
- **Security:** CSRF protection and simple password-based session (no DB needed)

---

### Phase 11: Documentation, Deployment & Monitoring
**Goal:** Finalize documentation, deployment tooling, and optional monitoring.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P11-T010 | Create API documentation (OpenAPI/Swagger spec in `go/api/`) using the lib `github.com/go-swagger/go-swagger` | Medium | P04-T120 |
| P11-T020 | Write deployment guide (how to run Master API in production) | Medium | P03-T060 |
| P11-T030 | Write ESP32 flashing guide (Arduino IDE and PlatformIO) | Medium | P08-T120 |
| P11-T040 | Create Docker Compose setup (optional: Master API + SQLite) | Low | P03-T060 |
| P11-T050 | Create systemd service file for Master API (Linux) | Low | P03-T060 |
| P11-T060 | Implement Prometheus metrics endpoint `/metrics` (optional) | Low | P03-T060 |
| P11-T070 | Create Grafana dashboard template (optional) | Low | P11-T060 |
| P11-T080 | Write troubleshooting guide (common issues and solutions) | Medium | All |
| P11-T090 | Create example scripts to populate initial jobs | Low | P02-T060 |
| P11-T100 | Final README.md polish (usage examples, screenshots) | Medium | All |

---

### Phase A01: Performance & Optimization (Adhoc Tasks)
**Goal:** Performance optimizations and refinements discovered during development/testing.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| A01-T010 | Implement worker-specific prefix affinity for vertical nonce exhaustion | High | P04-T050, P05-T030 | ✅ Completed |
| A01-T020 | Master background cleanup for abandoned leases (stale jobs reassignment) | Medium | A01-T010 | ✅ Completed |
| A01-T030 | Worker dynamic batch size adjustment based on target job duration | Medium | None | ✅ Completed |
| A01-T040 | Refactor jobs table schema for long-lived job model (macro jobs) | High | None | ✅ Completed |
| A01-T050 | Implement worker_history table with configurable retention | High | A01-T040 | ✅ Completed |
| A01-T055 | Add WORKER_HISTORY_LIMIT configuration support via env var | Medium | A01-T050 | ✅ Completed |
| A01-T060 | Update Master API to record worker statistics on checkpoint/complete | High | A01-T050, A01-T055 | ✅ Completed |
| A01-T065 | Update PC Worker client to support long-lived jobs and metrics reporting | High | A01-T060 | ✅ Completed |
| A01-T070 | Integration testing and validation of optimized job management | High | A01-T065 | ✅ Completed |
| A01-T080 | Fix Worker Metrics Overcounting | High | A01-T070 | ✅ Completed |
| A01-T090 | Fix in-memory database consistency and schema application | High | None | ✅ Completed |
| A01-T100 | Configure PC worker goroutine count via env var | Medium | None | ✅ Completed |
| A01-T110 | Master API: support list of target addresses | High | None | ✅ Completed |
| A01-T120 | Improve worker checkpointing, progress updates, and hot-path efficiency | High | None | ✅ Completed |

**Note:** Adhoc tasks (A0X-TXXX) are created on-demand to address performance issues, bugs, or optimizations discovered during development. They follow the same workflow as regular phase tasks but are tracked separately for visibility.

---

## Task Status Legend

- **Not Started:** Task file exists in `docs/tasks/backlog/`
- **In Progress:** Task file has `[IN PROGRESS]` header marker
- **Completed:** Task file moved to `docs/tasks/done/`
- **Blocked:** Task cannot proceed due to missing dependency (note in task file)

---

## Adding New Tasks On-The-Fly

### Example: Insert a new task between P02-T020 and P02-T030

1. Choose task number: `P02-T025` (halfway between 020 and 030)
2. Create file: `docs/tasks/backlog/P02-T025.md`
3. Update this OVERVIEW.md to include the new task in the table
4. Proceed with sequential execution

### Example: Add a subtask to P04-T050

1. Create subtasks: `P04-T050a.md` and `P04-T050b.md`
2. Or use single increments: `P04-T051.md`, `P04-T052.md`
3. Update OVERVIEW.md to reflect the split

---

## Task File Template

Each task file in `backlog/` or `done/` should follow this structure:

```markdown
# [Task ID]: [Task Title]

**Phase:** P0X - [Phase Name]  
**Status:** Not Started | In Progress | Completed | Blocked  
**Priority:** High | Medium | Low  
**Dependencies:** [List of task IDs]  
**Estimated Effort:** [Small | Medium | Large]

---

## Description

[Clear description of what this task accomplishes]

## Acceptance Criteria

- [ ] Criterion 1
- [ ] Criterion 2
- [ ] Criterion 3

## Implementation Notes

[Technical details, code snippets, references to SDD sections]

## Testing

[How to verify this task is complete]

## References

- SDD: `docs/architecture/system-design-document.md` (section X.Y)
- Schema: `docs/database/schema.sql`
- Related tasks: [Task IDs]
```

---

## Current Project State

**Last Updated:** February 21, 2026  
**Active Phase:** P10 (Dashboard & Monitoring UI)  
**Next Task:** P10-T010  
**Blockers:** None

---

## Phase Completion Checklist

- [x] **P01:** Project Foundation & Setup
- [x] **P02:** Database Layer Implementation
- [x] **P03:** Master API - Core Infrastructure
- [x] **P04:** Master API - Job Management (Dynamic Batching)
- [x] **P05:** PC Worker - Core Implementation
- [x] **P06:** PC Worker - Crypto & Scanning Engine
- [x] **P07:** ESP32 Worker - Core Infrastructure
- [x] **P08:** ESP32 Worker - Crypto & Computation
- [x] **P09:** Integration, Testing & Validation
- [ ] **P10:** Dashboard & Monitoring UI
- [ ] **P11:** Documentation, Deployment & Monitoring

**Adhoc/Optimization Tasks:**
- [x] **A01:** Performance & Optimization (ongoing)

---

## Notes

- **MVP Scope:** Focus on P01-P08 first; P09-P11 can be parallelized near completion
- **Task Granularity:** Each task should take 15 minutes to 2 hours max
- **Dependencies:** Always check dependencies before starting a task
- **SDD Reference:** All tasks are derived from `docs/architecture/system-design-document.md`
- **Sequential Execution:** Within each phase, work sequentially by task number
- **On-The-Fly Expansion:** Use incremental numbering (P0X-T025) to insert tasks dynamically

---

**End of Overview**
