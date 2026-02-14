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

DROP VIEW IF EXISTS stats_summary;

DROP INDEX IF EXISTS idx_workers_last_seen;
DROP INDEX IF EXISTS idx_workers_type;

DROP INDEX IF EXISTS idx_results_found_at;
DROP INDEX IF EXISTS idx_results_worker;
DROP INDEX IF EXISTS idx_results_address;

DROP INDEX IF EXISTS idx_jobs_worker_type;
DROP INDEX IF EXISTS idx_jobs_prefix;
DROP INDEX IF EXISTS idx_jobs_created;
DROP INDEX IF EXISTS idx_jobs_worker;
DROP INDEX IF EXISTS idx_jobs_status_expires;

DROP TABLE IF EXISTS results;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS workers;

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
