-- ============================================================================
-- EthScanner Distributed - Database Schema (Dynamic Batching)
-- ============================================================================
-- Database: SQLite (Pure Go - modernc.org/sqlite)
-- Version: 2.0 (Dynamic Batching with Checkpointing)
-- Date: February 14, 2026
-- 
-- This schema supports dynamic batch allocation with worker checkpointing.
-- Key changes from v1.0:
--   - 28-byte prefix stored as BLOB (not TEXT)
--   - Nonce ranges (nonce_start, nonce_end) for batch control
--   - current_nonce for checkpoint recovery
--   - BIGINT for nonce values (supports 2^32 range)
-- 
-- All timestamps are stored in UTC format.
-- ============================================================================

-- +goose Up
-- +goose StatementBegin

-- ============================================================================
-- Jobs Table
-- ============================================================================
-- Manages dynamic batch distribution with checkpointing support.
-- Each job represents a nonce range within a 28-byte prefix.
-- 
-- Lifecycle: pending -> processing (with checkpoints) -> completed
-- Fault Tolerance: Jobs with expired leases return to pending from last checkpoint
-- ============================================================================

CREATE TABLE IF NOT EXISTS jobs (
    -- Primary key (auto-increment)
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    
    -- Global prefix (28 bytes stored as BLOB for efficiency)
    -- This is the fixed portion of the key managed by the Master
    -- Example: x'0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c'
    prefix_28 BLOB NOT NULL,
    
    -- Nonce range assigned to this batch
    -- Nonce is the last 4 bytes (32 bits) of the private key
    nonce_start BIGINT NOT NULL,  -- Starting nonce (0 to 2^32-1)
    nonce_end BIGINT NOT NULL,    -- Ending nonce (exclusive)
    
    -- Current progress checkpoint (allows recovery from failures)
    -- NULL = not started, otherwise = last fully scanned nonce
    current_nonce BIGINT,
    
    -- Current job status
    -- Values: 'pending' | 'processing' | 'completed'
    status TEXT NOT NULL DEFAULT 'pending',
    
    -- Worker ID (UUID or unique identifier) currently holding the lease
    -- NULL when status is 'pending'
    worker_id TEXT,
    
    -- Worker type (for analytics)
    worker_type TEXT,
    
    -- Lease expiration timestamp (UTC)
    -- Format: 'YYYY-MM-DD HH:MM:SS'
    -- NULL when status is 'pending'
    -- Jobs with expires_at < NOW() are automatically available for re-lease
    expires_at DATETIME,
    
    -- Job creation timestamp (UTC, auto-set)
    created_at DATETIME NOT NULL DEFAULT (datetime('now', 'utc')),
    
    -- Job completion timestamp (UTC)
    -- NULL until status becomes 'completed'
    completed_at DATETIME,
    
    -- Number of keys scanned by the worker in this batch
    -- Updated via checkpoint and completion endpoints
    keys_scanned BIGINT DEFAULT 0,
    
    -- Requested batch size (for analytics and monitoring)
    requested_batch_size BIGINT,
    
    -- Last checkpoint timestamp (UTC)
    last_checkpoint_at DATETIME,
    
    -- Constraint: status must be one of the allowed values
    CHECK (status IN ('pending', 'processing', 'completed')),
    
    -- Constraint: nonce_end must be greater than nonce_start
    CHECK (nonce_end > nonce_start),
    
    -- Constraint: current_nonce must be within valid range
    CHECK (current_nonce IS NULL OR 
           (current_nonce >= nonce_start AND current_nonce <= nonce_end)),
    
    -- Unique constraint: prevent overlapping nonce ranges for same prefix
    UNIQUE (prefix_28, nonce_start, nonce_end)
);

-- ============================================================================
-- Indexes for Jobs Table
-- ============================================================================

-- Optimize query: Find available jobs (pending or expired)
-- Used by: POST /api/v1/jobs/lease
CREATE INDEX IF NOT EXISTS idx_jobs_status_expires 
ON jobs(status, expires_at);

-- Optimize query: Find jobs by worker (for monitoring and debugging)
CREATE INDEX IF NOT EXISTS idx_jobs_worker 
ON jobs(worker_id);

-- Optimize query: Find jobs by creation time (for analytics)
CREATE INDEX IF NOT EXISTS idx_jobs_created 
ON jobs(created_at DESC);

-- Optimize query: Find jobs by prefix (for nonce range allocation)
-- Used when allocating new nonce ranges within existing prefixes
CREATE INDEX IF NOT EXISTS idx_jobs_prefix 
ON jobs(prefix_28, nonce_start);

-- Optimize query: Find jobs by worker type (for statistics)
CREATE INDEX IF NOT EXISTS idx_jobs_worker_type 
ON jobs(worker_type);

-- ============================================================================
-- Results Table
-- ============================================================================
-- Stores private keys that match the target Ethereum address.
-- In practice, this table will almost certainly remain empty forever due to
-- the astronomical size of the key space (2^256).
-- ============================================================================

CREATE TABLE IF NOT EXISTS results (
    -- Primary key (auto-increment)
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    
    -- The private key that was found (hex-encoded, 64 characters)
    -- Example: "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
    -- Must be unique (a key can only be found once)
    private_key TEXT NOT NULL UNIQUE,
    
    -- The derived Ethereum address (hex-encoded with 0x prefix, 42 characters)
    -- Example: "0x000000000000000000000000000000000000dEaD"
    address TEXT NOT NULL,
    
    -- Worker ID that discovered this result
    worker_id TEXT NOT NULL,
    
    -- Reference to the job that contained this result
    job_id INTEGER NOT NULL,
    
    -- The specific nonce value that produced this result
    -- Useful for verification and analytics
    nonce_found BIGINT NOT NULL,
    
    -- Timestamp when the result was found (UTC, auto-set)
    found_at DATETIME NOT NULL DEFAULT (datetime('now', 'utc')),
    
    -- Foreign key constraint
    FOREIGN KEY (job_id) REFERENCES jobs(id)
);

-- ============================================================================
-- Indexes for Results Table
-- ============================================================================

-- Optimize query: Find results by Ethereum address
CREATE INDEX IF NOT EXISTS idx_results_address 
ON results(address);

-- Optimize query: Find results by worker (for statistics)
CREATE INDEX IF NOT EXISTS idx_results_worker 
ON results(worker_id);

-- Optimize query: Find results by discovery time
CREATE INDEX IF NOT EXISTS idx_results_found_at 
ON results(found_at DESC);

-- ============================================================================
-- Workers Table (Optional - for Monitoring)
-- ============================================================================
-- Tracks worker metadata and health status.
-- This table is optional for MVP but useful for monitoring and debugging.
-- ============================================================================

CREATE TABLE IF NOT EXISTS workers (
    -- Worker unique identifier (UUID recommended)
    id TEXT PRIMARY KEY,
    
    -- Worker type: 'pc' or 'esp32'
    worker_type TEXT NOT NULL,
    
    -- Last heartbeat/activity timestamp (UTC)
    last_seen DATETIME NOT NULL,
    
    -- Total number of keys scanned by this worker (across all jobs)
    total_keys_scanned INTEGER DEFAULT 0,
    
    -- Worker metadata (JSON format)
    -- Example: {"cpu": "Intel i9", "cores": 16, "ram": "32GB"}
    -- Example: {"chip": "ESP32-WROOM-32", "freq_mhz": 240}
    metadata TEXT,
    
    -- First time this worker was seen (UTC, auto-set)
    created_at DATETIME NOT NULL DEFAULT (datetime('now', 'utc')),

    -- Last time worker metadata was updated (UTC)
    updated_at DATETIME NOT NULL DEFAULT (datetime('now','utc')),    
    
    -- Constraint: worker_type must be one of the allowed values
    CHECK (worker_type IN ('pc', 'esp32'))
);

-- ============================================================================
-- Indexes for Workers Table
-- ============================================================================

-- Optimize query: Find workers by type
CREATE INDEX IF NOT EXISTS idx_workers_type 
ON workers(worker_type);

-- Optimize query: Find workers by last activity (for health checks)
CREATE INDEX IF NOT EXISTS idx_workers_last_seen 
ON workers(last_seen DESC);

-- ============================================================================
-- Statistics View (Optional - for Dashboard)
-- ============================================================================
-- Provides aggregated statistics for monitoring and dashboards.
-- This is a virtual view, not a physical table.
-- Updated for dynamic batching support.
-- ============================================================================

-- ---------------------------------------------------------------------------
-- Worker statistics tables (four-tier architecture)
-- Tier 1: worker_history (raw detail)
-- Tier 2: worker_stats_daily (per-worker daily aggregates)
-- Tier 3: worker_stats_monthly (per-worker monthly aggregates)
-- Tier 4: worker_stats_lifetime (per-worker lifetime totals)
-- Triggers perform aggregation and per-worker pruning to keep bounded size.
-- ---------------------------------------------------------------------------

-- Tier 1: Raw worker history (recent batches / jobs)
CREATE TABLE IF NOT EXISTS worker_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    worker_id TEXT NOT NULL,
    worker_type TEXT,
    job_id INTEGER,
    batch_size INTEGER,
    keys_scanned INTEGER,
    duration_ms INTEGER,
    keys_per_second REAL,
    prefix_28 BLOB,
    nonce_start BIGINT,
    nonce_end BIGINT,
    finished_at DATETIME NOT NULL DEFAULT (datetime('now','utc')),
    error_message TEXT,
    FOREIGN KEY (job_id) REFERENCES jobs(id)
);

CREATE INDEX IF NOT EXISTS idx_worker_history_worker_finished ON worker_history(worker_id, finished_at DESC);
CREATE INDEX IF NOT EXISTS idx_worker_history_finished ON worker_history(finished_at DESC);

-- Trigger: increment worker total_keys_scanned whenever a worker_history row is inserted
-- This keeps workers.total_keys_scanned consistent and atomic with history inserts
CREATE TRIGGER IF NOT EXISTS trg_inc_workers_total_keys
AFTER INSERT ON worker_history
FOR EACH ROW
BEGIN
    UPDATE workers
    SET total_keys_scanned = total_keys_scanned + COALESCE(NEW.keys_scanned, 0)
    WHERE id = NEW.worker_id;
END;

-- Tier 2: Daily aggregates (one row per worker per date)
CREATE TABLE IF NOT EXISTS worker_stats_daily (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    worker_id TEXT NOT NULL,
    stats_date DATE NOT NULL,
    total_batches INTEGER DEFAULT 0,
    total_keys_scanned INTEGER DEFAULT 0,
    total_duration_ms INTEGER DEFAULT 0,
    keys_per_second_avg REAL DEFAULT 0,
    keys_per_second_min REAL DEFAULT NULL,
    keys_per_second_max REAL DEFAULT NULL,
    error_count INTEGER DEFAULT 0,
    UNIQUE(worker_id, stats_date)
);

CREATE INDEX IF NOT EXISTS idx_worker_stats_daily_worker_date ON worker_stats_daily(worker_id, stats_date DESC);

-- Tier 3: Monthly aggregates (one row per worker per month)
CREATE TABLE IF NOT EXISTS worker_stats_monthly (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    worker_id TEXT NOT NULL,
    stats_month TEXT NOT NULL, -- YYYY-MM
    total_batches INTEGER DEFAULT 0,
    total_keys_scanned INTEGER DEFAULT 0,
    total_duration_ms INTEGER DEFAULT 0,
    keys_per_second_avg REAL DEFAULT 0,
    keys_per_second_min REAL DEFAULT NULL,
    keys_per_second_max REAL DEFAULT NULL,
    error_count INTEGER DEFAULT 0,
    UNIQUE(worker_id, stats_month)
);

CREATE INDEX IF NOT EXISTS idx_worker_stats_monthly_worker_month ON worker_stats_monthly(worker_id, stats_month DESC);

-- Tier 4: Lifetime totals (one row per worker)
CREATE TABLE IF NOT EXISTS worker_stats_lifetime (
    worker_id TEXT PRIMARY KEY,
    worker_type TEXT,
    total_batches INTEGER DEFAULT 0,
    total_keys_scanned INTEGER DEFAULT 0,
    total_duration_ms INTEGER DEFAULT 0,
    keys_per_second_avg REAL DEFAULT 0,
    keys_per_second_best REAL DEFAULT 0,
    keys_per_second_worst REAL DEFAULT NULL,
    first_seen_at DATETIME NOT NULL DEFAULT (datetime('now','utc')),
    last_seen_at DATETIME NOT NULL DEFAULT (datetime('now','utc'))
);

CREATE INDEX IF NOT EXISTS idx_worker_stats_lifetime_keys ON worker_stats_lifetime(total_keys_scanned DESC);

-- Trigger: aggregate before pruning history rows
CREATE TRIGGER IF NOT EXISTS trg_aggregate_before_prune_history
BEFORE DELETE ON worker_history
FOR EACH ROW
BEGIN
    -- Upsert daily aggregate
    INSERT INTO worker_stats_daily (
        worker_id, stats_date, total_batches, total_keys_scanned, total_duration_ms,
        keys_per_second_avg, keys_per_second_min, keys_per_second_max, error_count
    ) VALUES (
        OLD.worker_id,
        substr(OLD.finished_at, 1, 10),
        1,
        COALESCE(OLD.keys_scanned, 0),
        COALESCE(OLD.duration_ms, 0),
        COALESCE(OLD.keys_per_second, 0),
        OLD.keys_per_second,
        OLD.keys_per_second,
        CASE WHEN OLD.error_message IS NULL OR OLD.error_message = '' THEN 0 ELSE 1 END
    )
    ON CONFLICT(worker_id, stats_date) DO UPDATE SET
        total_batches = worker_stats_daily.total_batches + excluded.total_batches,
        total_keys_scanned = worker_stats_daily.total_keys_scanned + excluded.total_keys_scanned,
        total_duration_ms = worker_stats_daily.total_duration_ms + excluded.total_duration_ms,
        -- approximate avg as rolling mean (simple)
        keys_per_second_avg = (worker_stats_daily.keys_per_second_avg * worker_stats_daily.total_batches + excluded.keys_per_second_avg) / (worker_stats_daily.total_batches + excluded.total_batches),
        keys_per_second_min = MIN(IFNULL(worker_stats_daily.keys_per_second_min, excluded.keys_per_second_avg), excluded.keys_per_second_avg),
        keys_per_second_max = MAX(IFNULL(worker_stats_daily.keys_per_second_max, excluded.keys_per_second_avg), excluded.keys_per_second_avg),
        error_count = worker_stats_daily.error_count + excluded.error_count;

    -- Upsert monthly aggregate
    INSERT INTO worker_stats_monthly (
        worker_id, stats_month, total_batches, total_keys_scanned, total_duration_ms,
        keys_per_second_avg, keys_per_second_min, keys_per_second_max, error_count
    ) VALUES (
        OLD.worker_id,
        substr(OLD.finished_at, 1, 7),
        1,
        COALESCE(OLD.keys_scanned, 0),
        COALESCE(OLD.duration_ms, 0),
        COALESCE(OLD.keys_per_second, 0),
        OLD.keys_per_second,
        OLD.keys_per_second,
        CASE WHEN OLD.error_message IS NULL OR OLD.error_message = '' THEN 0 ELSE 1 END
    )
    ON CONFLICT(worker_id, stats_month) DO UPDATE SET
        total_batches = worker_stats_monthly.total_batches + excluded.total_batches,
        total_keys_scanned = worker_stats_monthly.total_keys_scanned + excluded.total_keys_scanned,
        total_duration_ms = worker_stats_monthly.total_duration_ms + excluded.total_duration_ms,
        keys_per_second_avg = (worker_stats_monthly.keys_per_second_avg * worker_stats_monthly.total_batches + excluded.keys_per_second_avg) / (worker_stats_monthly.total_batches + excluded.total_batches),
        keys_per_second_min = MIN(IFNULL(worker_stats_monthly.keys_per_second_min, excluded.keys_per_second_avg), excluded.keys_per_second_avg),
        keys_per_second_max = MAX(IFNULL(worker_stats_monthly.keys_per_second_max, excluded.keys_per_second_avg), excluded.keys_per_second_avg),
        error_count = worker_stats_monthly.error_count + excluded.error_count;

    -- Upsert lifetime totals
    INSERT INTO worker_stats_lifetime (
        worker_id, worker_type, total_batches, total_keys_scanned, total_duration_ms,
        keys_per_second_avg, keys_per_second_best, keys_per_second_worst, first_seen_at, last_seen_at
    ) VALUES (
        OLD.worker_id,
        OLD.worker_type,
        1,
        COALESCE(OLD.keys_scanned, 0),
        COALESCE(OLD.duration_ms, 0),
        COALESCE(OLD.keys_per_second, 0),
        OLD.keys_per_second,
        OLD.keys_per_second,
        datetime('now','utc'),
        datetime('now','utc')
    )
    ON CONFLICT(worker_id) DO UPDATE SET
        total_batches = worker_stats_lifetime.total_batches + excluded.total_batches,
        total_keys_scanned = worker_stats_lifetime.total_keys_scanned + excluded.total_keys_scanned,
        total_duration_ms = worker_stats_lifetime.total_duration_ms + excluded.total_duration_ms,
        keys_per_second_avg = (worker_stats_lifetime.keys_per_second_avg * worker_stats_lifetime.total_batches + excluded.keys_per_second_avg) / (worker_stats_lifetime.total_batches + excluded.total_batches),
        keys_per_second_best = MAX(worker_stats_lifetime.keys_per_second_best, excluded.keys_per_second_avg),
        keys_per_second_worst = MIN(COALESCE(worker_stats_lifetime.keys_per_second_worst, excluded.keys_per_second_avg), excluded.keys_per_second_avg),
        last_seen_at = datetime('now','utc');
END;

-- Prune daily stats per worker (keep latest 1000 per worker)
CREATE TRIGGER IF NOT EXISTS trg_prune_daily_stats_per_worker
AFTER INSERT ON worker_stats_daily
FOR EACH ROW
WHEN (SELECT COUNT(*) FROM worker_stats_daily WHERE worker_id = NEW.worker_id) > 1000
BEGIN
    DELETE FROM worker_stats_daily
    WHERE id IN (
        SELECT id FROM worker_stats_daily WHERE worker_id = NEW.worker_id ORDER BY stats_date ASC LIMIT (
            SELECT COUNT(*) - 1000 FROM worker_stats_daily WHERE worker_id = NEW.worker_id
        )
    );
END;

-- Prune monthly stats per worker (keep latest 1000 per worker)
CREATE TRIGGER IF NOT EXISTS trg_prune_monthly_stats_per_worker
AFTER INSERT ON worker_stats_monthly
FOR EACH ROW
WHEN (SELECT COUNT(*) FROM worker_stats_monthly WHERE worker_id = NEW.worker_id) > 1000
BEGIN
    DELETE FROM worker_stats_monthly
    WHERE id IN (
        SELECT id FROM worker_stats_monthly WHERE worker_id = NEW.worker_id ORDER BY stats_month ASC LIMIT (
            SELECT COUNT(*) - 1000 FROM worker_stats_monthly WHERE worker_id = NEW.worker_id
        )
    );
END;

-- ============================================================================
-- Statistics View (Optional - for Dashboard)
-- ============================================================================
-- Provides aggregated statistics for monitoring and dashboards.
-- This is a virtual view, not a physical table.
-- Updated for dynamic batching support.
-- ============================================================================

CREATE VIEW IF NOT EXISTS stats_summary AS
SELECT
    -- Batch statistics
    COUNT(CASE WHEN status = 'pending' THEN 1 END) AS pending_batches,
    COUNT(CASE WHEN status = 'processing' THEN 1 END) AS processing_batches,
    COUNT(CASE WHEN status = 'completed' THEN 1 END) AS completed_batches,
    COUNT(*) AS total_batches,
    
    -- Key scanning statistics
    COALESCE(SUM(keys_scanned), 0) AS total_keys_scanned,
    
    -- Batch size statistics (average requested sizes by worker type)
    AVG(CASE WHEN worker_type = 'pc' THEN requested_batch_size END) AS avg_pc_batch_size,
    AVG(CASE WHEN worker_type = 'esp32' THEN requested_batch_size END) AS avg_esp32_batch_size,
    
    -- Result statistics
    (SELECT COUNT(*) FROM results) AS results_found,
    
    -- Worker statistics
    (SELECT COUNT(*) FROM workers) AS total_workers,
    (SELECT COUNT(*) FROM workers WHERE last_seen > datetime('now', '-5 minutes')) AS active_workers,
    (SELECT COUNT(*) FROM workers WHERE worker_type = 'pc') AS pc_workers,
    (SELECT COUNT(*) FROM workers WHERE worker_type = 'esp32') AS esp32_workers,
    
    -- Prefix progress (distinct prefixes being worked on)
    COUNT(DISTINCT prefix_28) AS active_prefixes
FROM jobs;

-- +goose StatementEnd


-- +goose Down
-- +goose StatementBegin

-- 1. Drop View
DROP VIEW IF EXISTS stats_summary;

-- 2. Drop Triggers (Created last in Up, so drop first among new objects)
DROP TRIGGER IF EXISTS trg_prune_monthly_stats_per_worker;
DROP TRIGGER IF EXISTS trg_prune_daily_stats_per_worker;
DROP TRIGGER IF EXISTS trg_aggregate_before_prune_history;
DROP TRIGGER IF EXISTS trg_inc_workers_total_keys;

-- 3. Drop New Statistics Tables & Indexes (Reverse order of creation)

-- Tier 4: Lifetime
DROP INDEX IF EXISTS idx_worker_stats_lifetime_keys;
DROP TABLE IF EXISTS worker_stats_lifetime;

-- Tier 3: Monthly
DROP INDEX IF EXISTS idx_worker_stats_monthly_worker_month;
DROP TABLE IF EXISTS worker_stats_monthly;

-- Tier 2: Daily
DROP INDEX IF EXISTS idx_worker_stats_daily_worker_date;
DROP TABLE IF EXISTS worker_stats_daily;

-- Tier 1: History
DROP INDEX IF EXISTS idx_worker_history_finished;
DROP INDEX IF EXISTS idx_worker_history_worker_finished;
DROP TABLE IF EXISTS worker_history;

-- 4. Drop Base Tables & Indexes (Reverse order of creation)

-- Workers
DROP INDEX IF EXISTS idx_workers_last_seen;
DROP INDEX IF EXISTS idx_workers_type;
DROP TABLE IF EXISTS workers;

-- Results
DROP INDEX IF EXISTS idx_results_found_at;
DROP INDEX IF EXISTS idx_results_worker;
DROP INDEX IF EXISTS idx_results_address;
DROP TABLE IF EXISTS results;

-- Jobs
DROP INDEX IF EXISTS idx_jobs_worker_type;
DROP INDEX IF EXISTS idx_jobs_prefix;
DROP INDEX IF EXISTS idx_jobs_created;
DROP INDEX IF EXISTS idx_jobs_worker;
DROP INDEX IF EXISTS idx_jobs_status_expires;
DROP TABLE IF EXISTS jobs;

-- +goose StatementEnd

-- ============================================================================
-- Sample Queries (for sqlc code generation)
-- ============================================================================

-- Query: Find an available batch or allocate new nonce range
-- This query finds either:
--   1. A pending job, OR
--   2. An expired job (for re-lease), OR
--   3. Returns NULL (caller must create new batch)
-- name: FindAvailableBatch :one
-- SELECT * FROM jobs
-- WHERE status = 'pending' 
--    OR (status = 'processing' AND expires_at < datetime('now', 'utc'))
-- ORDER BY created_at ASC
-- LIMIT 1;

-- Query: Get the next available nonce range for a prefix
-- Used when creating a new batch within an existing prefix
-- name: GetNextNonceRange :one
-- SELECT MAX(nonce_end) as last_nonce_end
-- FROM jobs
-- WHERE prefix_28 = ?
-- AND status IN ('processing', 'completed');

-- Query: Create a new batch (lease)
-- name: CreateBatch :one
-- INSERT INTO jobs (
--     prefix_28, 
--     nonce_start, 
--     nonce_end,
--     current_nonce,
--     status, 
--     worker_id,
--     worker_type,
--     expires_at,
--     requested_batch_size
-- )
-- VALUES (?, ?, ?, ?, 'processing', ?, ?, datetime('now', 'utc', '+' || ? || ' seconds'), ?)
-- RETURNING *;

-- Query: Lease an existing batch to a worker
-- name: LeaseBatch :exec
-- UPDATE jobs
-- SET 
--     status = 'processing',
--     worker_id = ?,
--     worker_type = ?,
--     expires_at = datetime('now', 'utc', '+' || ? || ' seconds')
-- WHERE id = ?;

-- Query: Update checkpoint (progress reporting)
-- name: UpdateCheckpoint :exec
-- UPDATE jobs
-- SET 
--     current_nonce = ?,
--     keys_scanned = ?,
--     last_checkpoint_at = datetime('now', 'utc')
-- WHERE id = ? AND worker_id = ? AND status = 'processing';

-- Query: Complete a batch
-- name: CompleteBatch :exec
-- UPDATE jobs
-- SET 
--     status = 'completed',
--     completed_at = datetime('now', 'utc'),
--     keys_scanned = ?,
--     current_nonce = nonce_end
-- WHERE id = ? AND worker_id = ?;

-- Query: Insert a result
-- name: InsertResult :one
-- INSERT INTO results (private_key, address, worker_id, job_id, nonce_found)
-- VALUES (?, ?, ?, ?, ?)
-- RETURNING *;

-- Query: Get statistics
-- name: GetStats :one
-- SELECT * FROM stats_summary;

-- Query: Upsert worker heartbeat
-- name: UpsertWorker :exec
-- INSERT INTO workers (id, worker_type, last_seen, metadata)
-- VALUES (?, ?, datetime('now', 'utc'), ?)
-- ON CONFLICT(id) DO UPDATE SET
--     last_seen = datetime('now', 'utc'),
--     metadata = excluded.metadata;

-- Query: Update worker key count
-- name: UpdateWorkerKeyCount :exec
-- UPDATE workers
-- SET total_keys_scanned = total_keys_scanned + ?
-- WHERE id = ?;

-- Query: Get current global prefix and usage
-- name: GetPrefixUsage :many
-- SELECT 
--     prefix_28,
--     COUNT(*) as total_batches,
--     SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as completed_batches,
--     MAX(nonce_end) as highest_nonce,
--     SUM(keys_scanned) as total_keys_scanned
-- FROM jobs
-- GROUP BY prefix_28
-- ORDER BY prefix_28
-- LIMIT 100;

-- ============================================================================
-- Initial Data Seeding (Optional)
-- ============================================================================
-- Uncomment to pre-populate the database with initial batches.
-- This is useful for testing and initial deployment.
-- 
-- Note: With dynamic batching, batches are created on-demand by the API
-- when workers request them. Pre-seeding is optional.
-- ============================================================================

-- Example: Create initial batches for the first prefix (all zeros)
-- This creates small test batches of 1M keys each
-- 
-- INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status) 
-- VALUES (
--     x'000000000000000000000000000000000000000000000000000000000000',
--     0,
--     1000000,
--     'pending'
-- );
-- 
-- INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status) 
-- VALUES (
--     x'000000000000000000000000000000000000000000000000000000000000',
--     1000000,
--     2000000,
--     'pending'
-- );
-- 
-- ... (continue for desired range) ...

-- ============================================================================
-- End of Schema
-- ============================================================================
