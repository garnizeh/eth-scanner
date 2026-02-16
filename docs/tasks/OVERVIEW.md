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
| P01-T010 | Initialize Go module in `go/` folder | High | None |
| P01-T020 | Create `internal/` folder structure (api, database, jobs, worker, config) | High | P01-T010 |
| P01-T030 | Create `.gitignore` for Go, SQLite, IDE files | High | None |
| P01-T040 | Set up `sqlc` configuration file (`sqlc.yaml`) | High | P01-T010 |
| P01-T050 | Create `scripts/init-db.sh` (initialize SQLite database with schema) | Medium | None |
| P01-T060 | Verify Go toolchain (no CGO requirement for `modernc.org/sqlite`) | High | P01-T010 |
| P01-T070 | Create basic `Makefile` or `justfile` for common tasks (build, test, run) | Low | P01-T010 |
| P01-T080 | Setup GitHub Actions CI Workflow | High | P01-T070 |

---

### Phase 02: Database Layer Implementation
**Goal:** Implement complete database schema with type-safe query layer using `sqlc`.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P02-T010 | Validate `go/internal/database/sql/001_schema.sql` against SDD requirements | High | None |
| P02-T020 | Create `go/internal/database/sql/queries.sql` with all sqlc queries | High | P02-T010 |
| P02-T030 | Configure `sqlc.yaml` for code generation | High | P01-T040 |
| P02-T040 | Run `sqlc generate` and verify generated code in `internal/database/` | High | P02-T020, P02-T030 |
| P02-T050 | Implement `internal/database/db.go` (SQLite connection with `modernc.org/sqlite`) | High | P02-T040 |
| P02-T060 | Create database initialization function (apply schema on first run) | High | P02-T050 |
| P02-T070 | Implement database migration versioning using goose lib | Low | P02-T060 |
| P02-T080 | Write unit tests for database layer (connection, schema application) | Medium | P02-T060 |

---

### Phase 03: Master API - Core Infrastructure
**Goal:** Set up HTTP server with routing, middleware, and basic endpoints.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P03-T010 | Implement `internal/config/config.go` (load from env/file: port, DB path) | High | P01-T020 |
| P03-T020 | Implement `internal/server/server.go` (HTTP server setup with `net/http` or `chi`) | High | P03-T010 |
| P03-T030 | Implement `internal/server/middleware.go` (logging, CORS, request ID) | Medium | P03-T020 |
| P03-T040 | Implement `internal/server/routes.go` (route registration) | High | P03-T020 |
| P03-T050 | Create `GET /health` endpoint (basic health check) | High | P03-T040 |
| P03-T060 | Create `cmd/master/main.go` entry point (wire dependencies, start server) | High | P03-T020 |
| P03-T070 | Test server startup and `/health` endpoint manually | High | P03-T060 |
| P03-T080 | Implement graceful shutdown with `context.Context` | Medium | P03-T060 |

---

### Phase 04: Master API - Job Management (Dynamic Batching)
**Goal:** Implement all job-related API endpoints with dynamic batching logic.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P04-T010 | Implement `internal/jobs/manager.go` skeleton (job manager struct) | High | P02-T050 |
| P04-T020 | Implement job lease logic: find available/expired job from DB | High | P04-T010, P02-T040 |
| P04-T030 | Implement nonce range allocation: get next available range for prefix | High | P04-T020 |
| P04-T040 | Implement batch creation: create new job with dynamic batch size | High | P04-T030 |
| P04-T050 | Create `POST /api/v1/jobs/lease` handler (request validation + lease logic) | High | P04-T040 |
| P04-T060 | Implement UTC timestamp handling for `expires_at` (no `time.Local`) | High | P04-T050 |
| P04-T070 | Create `PATCH /api/v1/jobs/{id}/checkpoint` handler (update progress) | High | P04-T010, P02-T040 |
| P04-T080 | Implement worker_id validation in checkpoint endpoint | High | P04-T070 |
| P04-T090 | Create `POST /api/v1/jobs/{id}/complete` handler (mark job completed) | High | P04-T010, P02-T040 |
| P04-T100 | Implement final_nonce validation (must equal nonce_end) | High | P04-T090 |
| P04-T110 | Create `POST /api/v1/results` handler (submit found private key) | Medium | P04-T010, P02-T040 |
| P04-T120 | Create `GET /api/v1/stats` handler (return statistics from view) | Low | P04-T010, P02-T040 |
| P04-T130 | Write integration tests for lease endpoint (pending/expired jobs) | High | P04-T050 |
| P04-T140 | Write integration tests for checkpoint endpoint | Medium | P04-T070 |
| P04-T150 | Write integration tests for complete endpoint | Medium | P04-T090 |
| P04-T160 | Add API key middleware for master API | Medium | P04-T150 |

---

### Phase 05: PC Worker - Core Implementation
**Goal:** Build PC worker foundation with batch management and API client.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P05-T010 | Implement `internal/worker/config.go` (worker config: API URL, worker ID) | High | None |
| P05-T020 | Implement `internal/worker/client.go` (HTTP client for Master API) | High | P05-T010 |
| P05-T030 | Implement `LeaseBatch()` function (POST /api/v1/jobs/lease) | High | P05-T020 |
| P05-T040 | Implement `UpdateCheckpoint()` function (PATCH /api/v1/jobs/{id}/checkpoint) | High | P05-T020 |
| P05-T050 | Implement `CompleteBatch()` function (POST /api/v1/jobs/{id}/complete) | High | P05-T020 |
| P05-T060 | Implement `SubmitResult()` function (POST /api/v1/results) | Medium | P05-T020 |
| P05-T070 | Implement batch size calculator using `runtime.NumCPU()` | High | None |
| P05-T080 | Implement worker main loop (lease → process → complete) | High | P05-T030, P05-T050 |
| P05-T090 | Implement retry logic with exponential backoff (when no jobs available) | Medium | P05-T080 |
| P05-T100 | Create `cmd/worker-pc/main.go` entry point | High | P05-T080 |

---

### Phase 06: PC Worker - Crypto & Scanning Engine
**Goal:** Implement cryptographic key generation, scanning, and checkpointing.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P06-T010 | Implement `internal/worker/crypto.go` (import `go-ethereum/crypto`) | High | None |
| P06-T020 | Implement `DeriveEthereumAddress()` function (private key → address) | High | P06-T010 |
| P06-T030 | Implement key construction (prefix_28 + nonce little-endian) | High | P06-T020 |
| P06-T040 | Implement `internal/worker/scanner.go` (nonce range scanning) | High | P06-T030 |
| P06-T050 | Implement worker pool with goroutines (`runtime.NumCPU()` workers) | High | P06-T040 |
| P06-T060 | Implement nonce range partitioning across workers | High | P06-T050 |
| P06-T070 | Implement atomic progress tracking (`atomic.Uint64` for current_nonce) | High | P06-T060 |
| P06-T080 | Implement checkpoint goroutine (periodic PATCH every 5 minutes) | High | P06-T070, P05-T040 |
| P06-T090 | Implement context cancellation for lease expiration | High | P06-T050 |
| P06-T100 | Optimize crypto loop (buffer reuse, minimize allocations) | Medium | P06-T040 |
| P06-T110 | Write benchmarks for key scanning throughput (keys/sec) | Medium | P06-T040 |
| P06-T120 | Replace worker simulation with real job processing | High | P06-T100, P06-T110 |

---

### Phase 07: ESP32 Worker - Core Infrastructure
**Goal:** Set up ESP32 firmware with WiFi, HTTP client, and NVS persistence.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P07-T010 | Create `esp32/esp32-worker.ino` skeleton (Arduino project) | High | None |
| P07-T020 | Create `esp32/config.h` (WiFi SSID, password, API URL placeholders) | High | P07-T010 |
| P07-T030 | Implement WiFi connection manager (auto-reconnect on failure) | High | P07-T020 |
| P07-T040 | Implement HTTP client wrapper (POST/PATCH requests to Master API) | High | P07-T030 |
| P07-T050 | Initialize NVS (Non-Volatile Storage) for checkpoint persistence | High | P07-T010 |
| P07-T060 | Implement `saveCheckpoint()` function (write to NVS) | High | P07-T050 |
| P07-T070 | Implement `loadCheckpoint()` function (read from NVS on boot) | High | P07-T050 |
| P07-T080 | Implement performance benchmark on boot (10-second dry run) | High | P07-T010 |
| P07-T090 | Implement batch size calculator (keys/sec × 3600 for 1-hour batch) | High | P07-T080 |
| P07-T100 | Create global job state struct (prefix_28, nonce_start, nonce_end, etc.) | High | P07-T010 |

---

### Phase 08: ESP32 Worker - Crypto & Computation
**Goal:** Implement dual-core FreeRTOS tasks with optimized crypto hot loop.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P08-T010 | Integrate `trezor-crypto` or `micro-ecc` library (secp256k1) | High | P07-T010 |
| P08-T020 | Integrate `keccak256` hashing library | High | P08-T010 |
| P08-T030 | Implement `deriveAddress()` function (private key → Ethereum address) | High | P08-T020 |
| P08-T040 | Implement FreeRTOS task for Core 0 (networking + checkpointing) | High | P07-T040, P07-T060 |
| P08-T050 | Implement job lease logic in Core 0 task (call Master API) | High | P08-T040, P07-T090 |
| P08-T060 | Implement checkpoint upload logic in Core 0 (every 60 seconds) | High | P08-T040 |
| P08-T070 | Implement FreeRTOS task for Core 1 (computation hot loop) | High | P08-T030 |
| P08-T080 | Implement nonce iteration loop (prefix_28 + nonce) | High | P08-T070, P08-T030 |
| P08-T090 | Implement address comparison with target address | High | P08-T080 |
| P08-T100 | Implement result submission on match found (notify Core 0) | High | P08-T090 |
| P08-T110 | Optimize memory usage (static buffers, no heap fragmentation) | Medium | P08-T070 |
| P08-T120 | Test checkpoint recovery after power cycle | High | P07-T070, P08-T060 |

---

### Phase 09: Integration, Testing & Validation
**Goal:** Ensure all components work together correctly with comprehensive testing.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P09-T010 | Write unit tests for `internal/jobs/manager.go` | High | P04-T010 |
| P09-T020 | Write unit tests for `internal/worker/crypto.go` | High | P06-T020 |
| P09-T030 | Write unit tests for nonce range allocation logic | High | P04-T030 |
| P09-T040 | Write integration test: Master API + SQLite (end-to-end lease flow) | High | P04-T050 |
| P09-T050 | Write integration test: PC worker + Master API (full batch cycle) | High | P06-T080, P04-T050 |
| P09-T060 | Test lease expiration and job re-assignment | High | P04-T050 |
| P09-T070 | Test checkpoint recovery (worker crashes mid-batch) | High | P06-T080 |
| P09-T080 | Benchmark PC worker throughput (keys/sec on reference hardware) | Medium | P06-T110 |
| P09-T090 | Test ESP32 firmware on actual hardware (full cycle) | High | P08-T120 |
| P09-T100 | Test ESP32 NVS checkpoint recovery on power loss | High | P08-T120 |
| P09-T110 | Validate all API endpoints with Postman/curl scripts | Medium | P04-T120 |
| P09-T120 | Load test: multiple concurrent workers (10+ workers) | Low | P09-T050 |

---

### Phase 10: Documentation, Deployment & Monitoring
**Goal:** Finalize documentation, deployment tooling, and optional monitoring.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P10-T010 | Create API documentation (OpenAPI/Swagger spec in `docs/api/`) | Medium | P04-T120 |
| P10-T020 | Write deployment guide (how to run Master API in production) | Medium | P03-T060 |
| P10-T030 | Write ESP32 flashing guide (Arduino IDE and PlatformIO) | Medium | P08-T120 |
| P10-T040 | Create Docker Compose setup (optional: Master API + SQLite) | Low | P03-T060 |
| P10-T050 | Create systemd service file for Master API (Linux) | Low | P03-T060 |
| P10-T060 | Implement Prometheus metrics endpoint `/metrics` (optional) | Low | P03-T060 |
| P10-T070 | Create Grafana dashboard template (optional) | Low | P10-T060 |
| P10-T080 | Write troubleshooting guide (common issues and solutions) | Medium | All |
| P10-T090 | Create example scripts to populate initial jobs | Low | P02-T060 |
| P10-T100 | Final README.md polish (usage examples, screenshots) | Medium | All |

---

### Phase 11: Dashboard & Monitoring UI
**Goal:** Build a web-based dashboard for real-time monitoring and analytics using the multi-tier statistics architecture.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| P11-T010 | Choose frontend stack (React/Vue/Svelte + charting library) | High | None |
| P11-T020 | Create `dashboard/` folder structure and initialize project | High | P11-T010 |
| P11-T030 | Implement API client for Master API statistics endpoints | High | P11-T020 |
| P11-T040 | Create dashboard layout (sidebar, header, main content area) | High | P11-T030 |
| P11-T050 | Implement "Active Workers" panel (Tier 1: real-time from worker_history) | High | P11-T040 |
| P11-T060 | Implement "Live Throughput" chart (keys/sec over last 5-10 minutes) | High | P11-T050 |
| P11-T070 | Implement "Daily Performance" chart (Tier 2: worker_stats_daily trends) | High | P11-T040 |
| P11-T080 | Implement "Monthly Trends" chart (Tier 3: worker_stats_monthly long-term) | Medium | P11-T040 |
| P11-T090 | Implement "Worker Leaderboard" table (Tier 4: worker_stats_lifetime rankings) | Medium | P11-T040 |
| P11-T100 | Implement "Jobs Overview" panel (active jobs, progress bars per prefix) | High | P11-T040 |
| P11-T110 | Implement "Error Log" panel (recent failures from worker_history) | Medium | P11-T050 |
| P11-T120 | Implement "Fleet Statistics" panel (total keys scanned, avg throughput, worker types) | Medium | P11-T090 |
| P11-T130 | Add auto-refresh/polling for real-time data updates | High | P11-T050 |
| P11-T140 | Implement worker detail view (click worker → see individual history/stats) | Medium | P11-T090 |
| P11-T150 | Add responsive design for mobile/tablet viewing | Low | P11-T040 |
| P11-T160 | Implement dark/light theme toggle | Low | P11-T040 |
| P11-T170 | Add export functionality (CSV/JSON for stats) | Low | P11-T090 |
| P11-T180 | Create production build configuration and deployment guide | High | P11-T130 |
| P11-T190 | Write documentation for dashboard setup and usage | Medium | P11-T180 |

**Dashboard Features Overview:**

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
- **Frontend:** React/Next.js or Vue/Nuxt for SSR capabilities
- **Charting:** Recharts, Chart.js, or D3.js for visualizations
- **State:** React Query or SWR for API data fetching/caching
- **UI:** Tailwind CSS or Material-UI for rapid development
- **Real-time:** WebSocket or polling (every 5-10 seconds) for live updates

---

### Phase A01: Performance & Optimization (Adhoc Tasks)
**Goal:** Performance optimizations and refinements discovered during development/testing.

| Task ID | Description | Priority | Dependencies |
|---------|-------------|----------|--------------|
| A01-T010 | Implement worker-specific prefix affinity for vertical nonce exhaustion | High | P04-T050, P05-T030 |
| A01-T020 | Master background cleanup for abandoned leases (stale jobs reassignment) | Medium | A01-T010 |
| A01-T030 | Worker dynamic batch size adjustment based on target job duration | Medium | None |
| A01-T040 | Refactor jobs table schema for long-lived job model (macro jobs) | High | None |
| A01-T050 | Implement worker_history table with configurable retention | High | A01-T040 |
| A01-T055 | Add WORKER_HISTORY_LIMIT configuration support via env var | Medium | A01-T050 |
| A01-T060 | Update Master API to record worker statistics on checkpoint/complete | High | A01-T050, A01-T055 |
| A01-T065 | Update PC Worker client to support long-lived jobs and metrics reporting | High | A01-T060 |
| A01-T070 | Integration testing and validation of optimized job management | High | A01-T065 |

**Note:** Adhoc tasks (A0X-TXXX) are created on-demand to address performance issues, bugs, or optimizations discovered during development. They follow the same workflow as regular phase tasks but are tracked separately for visibility.

---

### A01 Deep Dive: Database Storage Optimization

**Overview:**  
Tasks A01-T040 through A01-T070 implement a comprehensive database optimization to prevent unbounded growth while enabling rich analytics. This is a critical architectural improvement for long-term system scalability.

**Problem:**  
The current implementation creates a new job record for every batch request. With high-throughput workers submitting checkpoints every few minutes, the database would grow to hundreds of megabytes within weeks, with most data being low-value historical records.

**Solution - Two-Part Optimization:**

**Part 1: Long-Lived Jobs (A01-T040)**
- Replace "one job per batch" with "one job per prefix range"
- Workers update the same job record via checkpoints instead of creating new records
- Results: O(1) storage per active prefix instead of O(N) per batch

**Part 2: Multi-Tier Statistics (A01-T050 to A01-T070)**

Introduce a four-tier automatic aggregation architecture:

```
Tier 1: worker_history (raw detail)
  • Retention: 10,000 records globally
  • Use case: Real-time monitoring (last 5 minutes)
  • Auto-prune: Aggregate to Tier 2 before deletion
     ↓
Tier 2: worker_stats_daily (daily aggregation)
  • Retention: 1,000 records per worker
  • Use case: Weekly/monthly trends
  • Auto-prune: Aggregate to Tier 3 before deletion
     ↓
Tier 3: worker_stats_monthly (monthly aggregation)
  • Retention: 1,000 records per worker
  • Use case: Year-over-year analysis
  • Auto-prune: Aggregate to Tier 4 before deletion
     ↓
Tier 4: worker_stats_lifetime (cumulative totals)
  • Retention: No cap (1 record per worker, permanent)
  • Use case: Leaderboards, overall statistics
```

**Key Features:**
- **Automatic Aggregation**: SQLite triggers cascade data through tiers before deletion
- **No Data Loss**: Metrics are preserved in aggregated form forever
- **Per-Worker Isolation**: Each worker maintains independent caps for daily/monthly stats
- **Dashboard-Ready**: Multi-scale queries without ETL (real-time, daily, monthly, lifetime)

**Configuration:**
```bash
WORKER_HISTORY_LIMIT=10000         # Global cap (Tier 1)
WORKER_DAILY_STATS_LIMIT=1000      # Per-worker cap (Tier 2)
WORKER_MONTHLY_STATS_LIMIT=1000    # Per-worker cap (Tier 3)
```

**Impact:**
- Storage: ~98.7% reduction vs. naive approach
- Database size: Constant ~2-3 MB even with millions of checkpoints
- Performance: Database queries remain fast regardless of uptime
- Analytics: Rich multi-scale insights from seconds to years

**Reference:**  
See `docs/architecture/db-optimization-proposal.md` for complete technical specification, schema definitions, trigger logic, and example dashboard queries.

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

**Last Updated:** February 14, 2026  
**Active Phase:** P01 (Project Foundation & Setup)  
**Next Task:** P01-T010  
**Blockers:** None

---

## Phase Completion Checklist

- [ ] **P01:** Project Foundation & Setup
- [ ] **P02:** Database Layer Implementation
- [ ] **P03:** Master API - Core Infrastructure
- [ ] **P04:** Master API - Job Management (Dynamic Batching)
- [ ] **P05:** PC Worker - Core Implementation
- [ ] **P06:** PC Worker - Crypto & Scanning Engine
- [ ] **P07:** ESP32 Worker - Core Infrastructure
- [ ] **P08:** ESP32 Worker - Crypto & Computation
- [ ] **P09:** Integration, Testing & Validation
- [ ] **P10:** Documentation, Deployment & Monitoring
- [ ] **P11:** Dashboard & Monitoring UI

**Adhoc/Optimization Tasks:**
- [ ] **A01:** Performance & Optimization (ongoing)

---

## Notes

- **MVP Scope:** Focus on P01-P08 first; P09-P10 can be parallelized near completion
- **Task Granularity:** Each task should take 15 minutes to 2 hours max
- **Dependencies:** Always check dependencies before starting a task
- **SDD Reference:** All tasks are derived from `docs/architecture/system-design-document.md`
- **Sequential Execution:** Within each phase, work sequentially by task number
- **On-The-Fly Expansion:** Use incremental numbering (P0X-T025) to insert tasks dynamically

---

**End of Overview**
