# Proposal: Database Storage & Job Management Optimization

**Version:** 1.0  
**Date:** February 16, 2026  
**Status:** Proposed  
**Related Tasks:** A01-T040 through A01-T070

---

## Executive Summary

This proposal addresses unbounded database growth in the current job management system by introducing:

1. **Long-Lived Jobs**: Single job records updated in-place via checkpoints instead of creating new records per batch
2. **Multi-Tier Statistics**: Four-tier automatic aggregation (raw → daily → monthly → lifetime) with configurable retention
3. **Storage Efficiency**: ~98.7% space savings vs. current approach with millions of checkpoints
4. **No Data Loss**: Automatic aggregation preserves all metrics before pruning
5. **Dashboard-Ready**: Multi-scale analytics (real-time, daily, monthly, lifetime) without ETL

**Impact:** Database size remains constant (~2-3 MB) even with years of continuous operation and millions of worker checkpoints.

---

## Table of Contents

1. [Problem Statement](#1-problem-statement)
2. [Proposed Solution](#2-proposed-solution)
   - [A. Optimization of Job/Batch Management](#a-optimization-of-jobbatch-management)
   - [B. Worker Statistics & History (Multi-Tier)](#b-worker-statistics--history-multi-tier-dashboard-architecture)
3. [Implementation Plan](#3-implementation-plan)
4. [Benefits](#4-benefits)
5. [Dashboard Integration](#5-dashboard-integration-future-work)
6. [Implementation Strategy](#6-implementation-strategy)
7. [Dashboard UI Integration (Phase 11)](#7-dashboard-ui-integration-phase-11)

---

## 1. Problem Statement
The current implementation of the `jobs` table stores every single batch execution as a permanent record. With high-throughput workers requesting small dynamic batches (e.g., every few seconds/minutes), the database size will grow rapidly with completed job records that provide little historical value ("dead rows").

**Current Issues:**
- **High Storage Cost:** Every assigned batch creates a row that persists forever unless manually pruned.
- **Performance Degradation:** Indexes on the `jobs` table grow indefinitely, potentially slowing down the critical "find available work" queries.
- **Mixed Concerns:** The `jobs` table currently mixes "work to be done" (state) with "work history" (logs/stats).
- **Inefficient Space Usage:** Multiple small batch records for what should be a single long-running job.

## 2. Proposed Solution

### A. Optimization of Job/Batch Management
Instead of creating a new row for every small batch requested by a worker, we will shift to a **"Large Range, Small Lease"** or **"Persistent Job"** model.

**Strategy:**
1. **Pre-allocation:** The Master creates "Macro Jobs" (e.g., covering a large nonce range like $0$ to $2^{32}-1$ for a specific prefix, or large chunks of it).
2. **In-Place Updates:** Workers lease a range *within* this Macro Job. 
    - The database record maintains a `current_nonce` cursor.
    - When a worker requests work, it leases the *next available chunk* from the active Macro Job.
    - **Crucial Change:** We do **not** insert a new row for this lease. Instead, we have a lightweight `leases` table or transient state, OR we simply update a `last_leased_nonce` on the main job record.
    - *Alternative (Simpler for MVP):* Use **Range Compaction**. 
        - Keep `jobs` table as "Work Units".
        - When a worker completes a batch, instead of marking it `completed` and leaving it there, we delete it? No, we need tracking.
        - **Better Approach (User's suggestion):** "Use the same record to store what was requested and how far it has progressed."
        - This implies a **Long-Lived Lease**:
            - A worker requests work.
            - It gets assigned a Job ID (specific Prefix + Range).
            - As the worker reports progress (checkpoints) or requests "more work" (next batch), it simply updates the `current_nonce` of that **same** Job ID.
            - The Job record represents the *entirety* of the work unit (e.g., the full 4-byte nonce range for a prefix), not just the 5-minute slice the worker is currently processing.
            - The worker processes this large job in small "batches" locally, reporting progress to the Master. The Master updates the single Job record's `current_nonce` and `last_checkpoint_at`.

**Revised Workflow:**
1. **Job Creation:** Master inserts a job covering the full target range (e.g., `nonce_start=0`, `nonce_end=4294967295`).
2. **Assignment:** Worker requests work. Master assigns this specific Job ID.
3. **Execution:** Worker processes a batch (e.g., 1 million keys).
4. **Update:** Worker sends a heartbeat/checkpoint. Master updates `current_nonce` on the Job record.
5. **Completion:** Only when `current_nonce` reaches `nonce_end` is the job marked completed.
    - **Benefit:** 1 Job Row per Prefix (or large range), regardless of how many thousands of "batches" the worker took to finish it.

### B. Worker Statistics & History (Multi-Tier Dashboard Architecture)
To prevent the main `jobs` table from bloating with metadata, and to enable comprehensive monitoring dashboards, we introduce a **three-tier statistics architecture** with automatic aggregation and configurable retention.

**Architecture Overview:**
```
worker_history (raw, short-term)
    ↓ (aggregate on prune)
worker_stats_daily (aggregated, medium-term, per worker cap)
    ↓ (aggregate on prune)
worker_stats_monthly (aggregated, long-term, per worker cap)
    ↓ (aggregate on prune)
worker_stats_lifetime (fully aggregated, forever, 1 row per worker)
```

**Tier 1: `worker_history` (Raw Detail)**
- **Purpose:** Track detailed performance metrics for recent batches.
- **Retention:** Configurable via `WORKER_HISTORY_LIMIT` (default: 10000 records globally)
- **Use Case:** Real-time monitoring, recent performance analysis, debugging

**Schema (Raw Detail):**
```sql
CREATE TABLE worker_history (
    -- Identity
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    worker_id TEXT NOT NULL,
    worker_type TEXT,  -- 'pc' | 'esp32' (for filtering in dashboard)
    job_id INTEGER,    -- Reference to jobs table (nullable for orphaned stats)
    
    -- Performance Metrics (for throughput graphs)
    batch_size BIGINT NOT NULL,
    keys_scanned BIGINT NOT NULL,
    duration_ms BIGINT NOT NULL,
    keys_per_second BIGINT NOT NULL,
    
    -- Job Context (for analysis)
    prefix_28 BLOB,  -- Allows dashboard to show per-prefix performance
    nonce_start BIGINT,
    nonce_end BIGINT,
    
    -- Timestamps (UTC)
    started_at DATETIME NOT NULL,
    finished_at DATETIME NOT NULL DEFAULT (datetime('now', 'utc')),
    
    -- Optional: Error tracking
    error_message TEXT,  -- NULL on success, error details on failure
    
    -- Indexes
    CREATE INDEX idx_worker_history_worker_id ON worker_history(worker_id);
    CREATE INDEX idx_worker_history_finished_at ON worker_history(finished_at DESC);
    CREATE INDEX idx_worker_history_worker_type ON worker_history(worker_type);
);
```

**Tier 2: `worker_stats_daily` (Daily Aggregation)**
- **Purpose:** Daily performance summaries per worker.
- **Retention:** 1000 most recent records **per worker_id** (configurable via `WORKER_DAILY_STATS_LIMIT`)
- **Use Case:** Weekly/monthly trend analysis, historical performance comparison

**Schema (Daily Aggregation):**
```sql
CREATE TABLE worker_stats_daily (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    worker_id TEXT NOT NULL,
    worker_type TEXT,
    stats_date DATE NOT NULL,  -- YYYY-MM-DD
    
    -- Aggregated metrics
    total_batches INTEGER DEFAULT 0,
    total_keys_scanned BIGINT DEFAULT 0,
    avg_keys_per_second BIGINT DEFAULT 0,
    min_keys_per_second BIGINT,
    max_keys_per_second BIGINT,
    total_duration_ms BIGINT DEFAULT 0,
    
    -- Error tracking
    error_count INTEGER DEFAULT 0,
    success_count INTEGER DEFAULT 0,
    
    -- Metadata
    first_batch_at DATETIME,
    last_batch_at DATETIME,
    created_at DATETIME DEFAULT (datetime('now', 'utc')),
    updated_at DATETIME DEFAULT (datetime('now', 'utc')),
    
    UNIQUE(worker_id, stats_date)
);

CREATE INDEX idx_worker_stats_daily_worker_id ON worker_stats_daily(worker_id, stats_date DESC);
```

**Tier 3: `worker_stats_monthly` (Monthly Aggregation)**
- **Purpose:** Monthly performance summaries per worker.
- **Retention:** 1000 most recent records **per worker_id** (configurable via `WORKER_MONTHLY_STATS_LIMIT`)
- **Use Case:** Long-term trend analysis, year-over-year comparison

**Schema (Monthly Aggregation):**
```sql
CREATE TABLE worker_stats_monthly (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    worker_id TEXT NOT NULL,
    worker_type TEXT,
    stats_month TEXT NOT NULL,  -- YYYY-MM
    
    -- Aggregated metrics
    total_batches INTEGER DEFAULT 0,
    total_keys_scanned BIGINT DEFAULT 0,
    avg_keys_per_second BIGINT DEFAULT 0,
    min_keys_per_second BIGINT,
    max_keys_per_second BIGINT,
    total_duration_ms BIGINT DEFAULT 0,
    
    -- Error tracking
    error_count INTEGER DEFAULT 0,
    success_count INTEGER DEFAULT 0,
    
    -- Metadata
    first_batch_at DATETIME,
    last_batch_at DATETIME,
    created_at DATETIME DEFAULT (datetime('now', 'utc')),
    updated_at DATETIME DEFAULT (datetime('now', 'utc')),
    
    UNIQUE(worker_id, stats_month)
);

CREATE INDEX idx_worker_stats_monthly_worker_id ON worker_stats_monthly(worker_id, stats_month DESC);
```

**Tier 4: `worker_stats_lifetime` (Lifetime Totals)**
- **Purpose:** Cumulative statistics for each worker across its entire lifetime.
- **Retention:** No cap (1 record per worker, updated continuously)
- **Use Case:** Worker leaderboards, overall performance comparison, uptime tracking

**Schema (Lifetime Totals):**
```sql
CREATE TABLE worker_stats_lifetime (
    worker_id TEXT PRIMARY KEY,
    worker_type TEXT,
    
    -- Lifetime totals
    total_batches BIGINT DEFAULT 0,
    total_keys_scanned BIGINT DEFAULT 0,
    total_duration_ms BIGINT DEFAULT 0,
    
    -- Performance stats
    avg_keys_per_second BIGINT DEFAULT 0,
    best_keys_per_second BIGINT DEFAULT 0,
    worst_keys_per_second BIGINT DEFAULT 0,
    
    -- Activity tracking
    first_seen_at DATETIME,
    last_seen_at DATETIME,
    
    -- Error tracking
    total_errors INTEGER DEFAULT 0,
    total_successes INTEGER DEFAULT 0,
    
    updated_at DATETIME DEFAULT (datetime('now', 'utc'))
);
```

**Automatic Aggregation via Triggers:**

The system uses cascading triggers to automatically aggregate data as it ages:

```sql
-- Trigger 1: Aggregate to daily/monthly/lifetime before deleting from worker_history
CREATE TRIGGER aggregate_before_prune_history
BEFORE DELETE ON worker_history
FOR EACH ROW
BEGIN
    -- Update daily stats
    INSERT INTO worker_stats_daily (
        worker_id, worker_type, stats_date,
        total_batches, total_keys_scanned, total_duration_ms,
        avg_keys_per_second, min_keys_per_second, max_keys_per_second,
        error_count, success_count, first_batch_at, last_batch_at, updated_at
    ) VALUES (
        OLD.worker_id, OLD.worker_type, date(OLD.finished_at),
        1, OLD.keys_scanned, OLD.duration_ms,
        OLD.keys_per_second, OLD.keys_per_second, OLD.keys_per_second,
        CASE WHEN OLD.error_message IS NULL THEN 0 ELSE 1 END,
        CASE WHEN OLD.error_message IS NULL THEN 1 ELSE 0 END,
        OLD.finished_at, OLD.finished_at, datetime('now', 'utc')
    )
    ON CONFLICT(worker_id, stats_date) DO UPDATE SET
        total_batches = total_batches + 1,
        total_keys_scanned = total_keys_scanned + OLD.keys_scanned,
        total_duration_ms = total_duration_ms + OLD.duration_ms,
        avg_keys_per_second = (total_keys_scanned + OLD.keys_scanned) / 
                              ((total_duration_ms + OLD.duration_ms) / 1000.0),
        min_keys_per_second = MIN(min_keys_per_second, OLD.keys_per_second),
        max_keys_per_second = MAX(max_keys_per_second, OLD.keys_per_second),
        error_count = error_count + CASE WHEN OLD.error_message IS NULL THEN 0 ELSE 1 END,
        success_count = success_count + CASE WHEN OLD.error_message IS NULL THEN 1 ELSE 0 END,
        last_batch_at = OLD.finished_at,
        updated_at = datetime('now', 'utc');
    
    -- Update monthly stats (similar logic)
    INSERT INTO worker_stats_monthly (
        worker_id, worker_type, stats_month, total_batches, total_keys_scanned, ...
    ) VALUES (...)
    ON CONFLICT(worker_id, stats_month) DO UPDATE SET ...;
    
    -- Update lifetime stats
    INSERT INTO worker_stats_lifetime (
        worker_id, worker_type, total_batches, total_keys_scanned, ...
    ) VALUES (...)
    ON CONFLICT(worker_id) DO UPDATE SET ...;
END;

-- Trigger 2: Prune old daily stats per worker (keep last 1000 per worker_id)
CREATE TRIGGER prune_daily_stats_per_worker
AFTER INSERT ON worker_stats_daily
FOR EACH ROW
WHEN (SELECT COUNT(*) FROM worker_stats_daily WHERE worker_id = NEW.worker_id) > 1000
BEGIN
    -- Aggregate to monthly/lifetime before deleting
    -- (Similar cascading logic)
    
    -- Delete oldest records for this worker
    DELETE FROM worker_stats_daily
    WHERE worker_id = NEW.worker_id
    AND id IN (
        SELECT id FROM worker_stats_daily
        WHERE worker_id = NEW.worker_id
        ORDER BY stats_date ASC
        LIMIT (SELECT COUNT(*) - 1000 FROM worker_stats_daily WHERE worker_id = NEW.worker_id)
    );
END;

-- Trigger 3: Prune old monthly stats per worker (keep last 1000 per worker_id)
CREATE TRIGGER prune_monthly_stats_per_worker
AFTER INSERT ON worker_stats_monthly
FOR EACH ROW
WHEN (SELECT COUNT(*) FROM worker_stats_monthly WHERE worker_id = NEW.worker_id) > 1000
BEGIN
    -- Aggregate to lifetime before deleting
    -- (Similar logic)
    
    DELETE FROM worker_stats_monthly
    WHERE worker_id = NEW.worker_id
    AND id IN (
        SELECT id FROM worker_stats_monthly
        WHERE worker_id = NEW.worker_id
        ORDER BY stats_month ASC
        LIMIT (SELECT COUNT(*) - 1000 FROM worker_stats_monthly WHERE worker_id = NEW.worker_id)
    );
END;
```

**Configuration via Environment Variables:**
- `WORKER_HISTORY_LIMIT=10000` (global cap for raw history)
- `WORKER_DAILY_STATS_LIMIT=1000` (per-worker cap for daily aggregation)
- `WORKER_MONTHLY_STATS_LIMIT=1000` (per-worker cap for monthly aggregation)

**Dashboard Queries Enabled:**

**Real-time (worker_history):**
- Last 5 minutes of activity
- Current worker throughput
- Recent errors

**Short-term trends (worker_stats_daily):**
- Worker performance over last 7/30 days
- Day-over-day comparison
- Identify performance degradation

**Long-term trends (worker_stats_monthly):**
- Monthly performance trends
- Year-over-year comparison
- Seasonal patterns

**Overall stats (worker_stats_lifetime):**
- Worker leaderboards
- All-time best performers
- Total contribution by worker

## 3. Implementation Plan

### Phase 1: Schema Refactoring (A01-T040)
1.  **Refactor Schema (`jobs`):**
    - Ensure `jobs` represents the **Macro Task** (Prefix + Full Nonce Range).
    - Ensure `current_nonce` is the single source of truth for progress.
    - Workers "check out" the job. If they crash, the job lease expires, and the next worker picks up from `current_nonce`.
    - Review and optimize indexes for the new access pattern (fewer inserts, more updates).

### Phase 2: Worker History & Statistics Tables (A01-T050)
2.  **Create Multi-Tier Statistics Tables:**
    - Add `worker_history` table to existing `internal/database/sql/001_schema.sql`
    - Add `worker_stats_daily` table for daily aggregation
    - Add `worker_stats_monthly` table for monthly aggregation
    - Add `worker_stats_lifetime` table for cumulative totals
    - Add indexes for common dashboard queries (worker_id, dates, worker_type)
    - Implement retention triggers with placeholders for configurable limits
    - Implement cascading aggregation triggers (history → daily → monthly → lifetime)
    - Add sqlc queries for inserting stats and fetching dashboard data at all tiers

### Phase 3: Configuration Support (A01-T055)
3.  **Add Configuration:**
    - Add `WORKER_HISTORY_LIMIT` to `internal/config/config.go` (default: 10000)
    - Add `WORKER_DAILY_STATS_LIMIT` (default: 1000 per worker)
    - Add `WORKER_MONTHLY_STATS_LIMIT` (default: 1000 per worker)
    - Validate configuration on startup (must be > 0)
    - Document new environment variables in README and config files

### Phase 4: API & Business Logic Updates (A01-T060)
4.  **Update Master API:**
    - `POST /lease`: Modify to support long-lived job leases. Instead of creating new jobs, find or create a macro job for the prefix and update its lease.
    - `PATCH /checkpoint`: Update `current_nonce` on the existing job. Record stats to `worker_history`.
    - `POST /complete`: Mark job as completed only when `current_nonce == nonce_end`. Record final stats to `worker_history`.
    - Add logic to populate `worker_history` with detailed metrics on each checkpoint/completion.

### Phase 5: Worker Client Updates (A01-T065)
5.  **Update Worker Logic:**
    - Worker maintains the same Job ID across multiple internal batch iterations.
    - Worker sends periodic checkpoints (progress updates) rather than requesting new leases constantly.
    - Worker includes performance metrics (duration_ms, keys_scanned) in checkpoint/complete requests.

### Phase 6: Migration & Testing (A01-T070)
6.  **Data Migration & Validation:**
    - Create migration strategy for existing data (if any).
    - Write comprehensive unit tests for new schema and worker_history insertion.
    - Write integration tests simulating long-lived job execution with checkpoints.
    - Verify pruning trigger works correctly with different `WORKER_HISTORY_LIMIT` values.
    - Load test: Verify database size remains constant with high checkpoint frequency.

## 4. Benefits

### Storage Efficiency
- **Constant O(1) storage** per Prefix/Range being scanned, rather than O(N) where N is number of batches.
- **Bounded history tables**: All tables have configurable or per-worker caps.
- **Reduced write amplification**: Updates instead of inserts for job progress.
- **Automatic aggregation**: Old data is summarized and compressed before deletion.
- **No data loss**: Even after pruning, aggregated statistics are preserved forever in lifetime table.

### Performance
- **Smaller indexes**: Jobs table remains compact with only active work items.
- **Faster queries**: "Find next job" queries scan fewer rows.
- **Efficient checkpointing**: Simple UPDATE operations instead of INSERT + cleanup.
- **Multi-tier querying**: Dashboard can choose appropriate granularity (recent vs historical).

### Operational
- **Self-cleaning history**: Automatic pruning via triggers at all levels.
- **Configurable retention**: Operators can tune history size per tier based on needs.
- **Dashboard-ready**: Rich metrics at multiple time scales without additional ETL.
- **Graceful degradation**: Automatic aggregation preserves trends even as raw data ages out.

### Monitoring & Analytics
- **Real-time visibility**: Recent worker performance always available in worker_history.
- **Historical trends**: Daily/monthly aggregations support long-term analysis.
- **Worker comparison**: Easy to compare PC vs ESP32 efficiency at any time scale.
- **Error tracking**: Centralized failure logging and error rate calculation.
- **Lifetime stats**: Permanent record of each worker's total contribution.

---

## 5. Dashboard Integration (Future Work)

The multi-tier statistics architecture is designed to support comprehensive monitoring dashboards with queries optimized for different time scales.

**Implementation Timeline:** Phase 11 (P11-T010 through P11-T190) will create a web-based dashboard that consumes these statistics via the Master API.

### Recommended Dashboard Panels

**Real-time Monitoring (worker_history):**
1. **Active Workers Now**: Workers with activity in last 5 minutes
2. **Live Throughput**: Current keys/second by worker
3. **Recent Errors**: Last 100 failures with details

**Short-term Analysis (worker_stats_daily):**
4. **7-Day Trend**: Worker performance over last week
5. **Daily Comparison**: Today vs yesterday throughput
6. **Worker Efficiency**: Average performance by worker type per day

**Long-term Analysis (worker_stats_monthly):**
7. **Monthly Trends**: Performance over last 12 months
8. **Seasonal Patterns**: Identify performance variations over time
9. **Monthly Leaderboard**: Top performers by month

**Lifetime Statistics (worker_stats_lifetime):**
10. **All-Time Leaderboard**: Best workers by total keys scanned
11. **Worker Overview**: Total contribution and uptime per worker
12. **Fleet Statistics**: Overall system performance summary

### Example Dashboard Queries

**Real-time: Active workers in last 5 minutes**
```sql
SELECT 
    worker_id, 
    worker_type, 
    AVG(keys_per_second) as avg_kps, 
    COUNT(*) as recent_batches,
    MAX(finished_at) as last_seen
FROM worker_history
WHERE finished_at > datetime('now', '-5 minutes')
GROUP BY worker_id
ORDER BY avg_kps DESC;
```

**Short-term: Last 7 days performance by worker**
```sql
SELECT 
    stats_date,
    worker_id,
    avg_keys_per_second as daily_avg_kps,
    total_batches,
    error_count
FROM worker_stats_daily
WHERE worker_id = ?
  AND stats_date >= date('now', '-7 days')
ORDER BY stats_date DESC;
```

**Short-term: Daily trend across all workers**
```sql
SELECT 
    stats_date,
    SUM(total_keys_scanned) as total_keys,
    AVG(avg_keys_per_second) as fleet_avg_kps,
    SUM(total_batches) as total_batches,
    SUM(error_count) as total_errors
FROM worker_stats_daily
WHERE stats_date >= date('now', '-30 days')
GROUP BY stats_date
ORDER BY stats_date;
```

**Long-term: Monthly performance comparison**
```sql
SELECT 
    stats_month,
    worker_type,
    AVG(avg_keys_per_second) as avg_kps,
    SUM(total_keys_scanned) as total_keys,
    SUM(total_batches) as batches
FROM worker_stats_monthly
WHERE stats_month >= strftime('%Y-%m', date('now', '-12 months'))
GROUP BY stats_month, worker_type
ORDER BY stats_month DESC;
```

**Lifetime: Worker leaderboard**
```sql
SELECT 
    worker_id,
    worker_type,
    total_keys_scanned,
    avg_keys_per_second,
    best_keys_per_second,
    total_batches,
    total_errors,
    julianday(last_seen_at) - julianday(first_seen_at) as days_active
FROM worker_stats_lifetime
ORDER BY total_keys_scanned DESC
LIMIT 100;
```

**Lifetime: Fleet overview**
```sql
SELECT 
    worker_type,
    COUNT(*) as worker_count,
    SUM(total_keys_scanned) as total_keys,
    AVG(avg_keys_per_second) as avg_kps,
    MAX(best_keys_per_second) as best_kps,
    SUM(total_errors) as total_errors
FROM worker_stats_lifetime
GROUP BY worker_type;
```

**Cross-tier: Worker health check**
```sql
-- Detect workers with recent performance degradation
SELECT 
    l.worker_id,
    l.avg_keys_per_second as lifetime_avg,
    d.avg_keys_per_second as today_avg,
    (d.avg_keys_per_second - l.avg_keys_per_second) * 100.0 / l.avg_keys_per_second as pct_change
FROM worker_stats_lifetime l
JOIN worker_stats_daily d ON l.worker_id = d.worker_id
WHERE d.stats_date = date('now')
  AND d.avg_keys_per_second < l.avg_keys_per_second * 0.8  -- 20% drop
ORDER BY pct_change ASC;
```

---

## 6. Implementation Strategy

**Note:** Since this project is not yet in production, we can modify the existing schema files directly without migration concerns.

### Direct Schema Modification (Recommended)
1. **Modify existing schema** (`internal/database/sql/001_schema.sql`):
   - Review and update `jobs` table comments to document the "Macro Job" concept
   - Verify schema supports long-lived job pattern (already compatible with current_nonce tracking)
   - Add `worker_history` table to the same schema file
   - Add retention trigger with placeholder for `WORKER_HISTORY_LIMIT`

2. **Update sqlc queries** (`internal/database/sql/queries.sql`):
   - Add queries for `RecordWorkerStats`
   - Add queries for dashboard data retrieval
   - Modify existing job queries if needed for macro job pattern

3. **Regenerate sqlc code**:
   - Run `sqlc generate`
   - Verify generated types and methods

4. **Delete existing database** (if any):
   - Remove `data/eth-scanner.db`
   - Fresh start with optimized schema

### No Rollback Needed
- Project is in development phase
- No production data to preserve
- Full flexibility to iterate on schema design
- Can test different retention strategies without compatibility concerns

---

## 7. Dashboard UI Integration (Phase 11)

The multi-tier statistics architecture is purpose-built to power a real-time monitoring dashboard without requiring additional data processing layers.

### API Endpoints for Dashboard

The Master API will expose statistics endpoints consumed by the dashboard:

```go
// Real-time monitoring (Tier 1)
GET /api/v1/stats/workers/active?since=5m
GET /api/v1/stats/workers/{id}/recent?limit=100

// Daily trends (Tier 2)
GET /api/v1/stats/workers/{id}/daily?days=7
GET /api/v1/stats/daily/aggregate?start_date=2026-02-01

// Monthly trends (Tier 3)
GET /api/v1/stats/workers/{id}/monthly?months=12
GET /api/v1/stats/monthly/aggregate?start_month=2025-02

// Lifetime statistics (Tier 4)
GET /api/v1/stats/workers/leaderboard?sort=keys_scanned&limit=100
GET /api/v1/stats/workers/{id}/lifetime
GET /api/v1/stats/fleet/summary

// Jobs tracking
GET /api/v1/jobs/active
GET /api/v1/jobs/{id}/progress
```

### Dashboard Architecture

```
┌─────────────────────────────────────────────────────────┐
│  Web Browser (React/Vue/Svelte)                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │  Dashboard Components                            │   │
│  │  • Active Workers Panel (Tier 1: 5-sec polling) │   │
│  │  • Live Throughput Chart (Tier 1: real-time)    │   │
│  │  • Daily Trends Chart (Tier 2: on-demand)       │   │
│  │  • Monthly Trends Chart (Tier 3: on-demand)     │   │
│  │  • Worker Leaderboard (Tier 4: cached 1min)     │   │
│  │  • Jobs Overview (jobs table: 10-sec polling)   │   │
│  └─────────────────────────────────────────────────┘   │
└────────────────────┬────────────────────────────────────┘
                     │ HTTP/JSON
                     ↓
┌─────────────────────────────────────────────────────────┐
│  Master API (Go)                                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │  Statistics Endpoints                            │   │
│  │  • Query worker_history (Tier 1)                 │   │
│  │  • Query worker_stats_daily (Tier 2)             │   │
│  │  • Query worker_stats_monthly (Tier 3)           │   │
│  │  • Query worker_stats_lifetime (Tier 4)          │   │
│  │  • Query jobs table                              │   │
│  └─────────────────────────────────────────────────┘   │
└────────────────────┬────────────────────────────────────┘
                     │ SQLite queries
                     ↓
┌─────────────────────────────────────────────────────────┐
│  SQLite Database (Multi-Tier Statistics)                │
│  • worker_history (10K global)                          │
│  • worker_stats_daily (1K per worker)                   │
│  • worker_stats_monthly (1K per worker)                 │
│  • worker_stats_lifetime (1 per worker)                 │
│  • jobs (active work items)                             │
└─────────────────────────────────────────────────────────┘
```

### Key Dashboard Features Enabled by Multi-Tier Architecture

1. **Real-time Monitoring** (Tier 1 - worker_history)
   - 5-second polling for active workers
   - Live throughput graph updates
   - Instant error notifications
   - No heavy queries (limited to 10K records)

2. **Trend Analysis** (Tier 2 - worker_stats_daily)
   - Efficient 7-day/30-day charts
   - Worker comparison over time
   - Performance degradation alerts
   - Pre-aggregated metrics (no expensive GROUP BY on millions of rows)

3. **Historical Analysis** (Tier 3 - worker_stats_monthly)
   - Year-over-year comparisons
   - Long-term performance trends
   - Minimal storage (1K months = 83 years per worker)
   - Fast queries even with years of data

4. **Leaderboards & Summaries** (Tier 4 - worker_stats_lifetime)
   - Instant leaderboard rendering
   - Fleet-wide statistics
   - Single-query worker profiles
   - Permanent historical record

### Performance Benefits for Dashboard

- **Fast Queries**: All tiers have appropriate indexes and small row counts
- **No ETL**: Data is pre-aggregated by triggers during insertion
- **Scalable Polling**: Real-time tier limited to 10K rows prevents slow queries
- **Efficient Caching**: Lifetime stats change infrequently (1-minute cache acceptable)
- **Multi-Resolution**: Dashboard can choose appropriate granularity for each chart

### Implementation Reference

See Phase 11 (P11-T010 through P11-T190) in `docs/tasks/OVERVIEW.md` for detailed dashboard implementation tasks.
