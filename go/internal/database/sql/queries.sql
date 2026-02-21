-- name: FindAvailableBatch :one
-- Find an available batch (pending or expired lease)
SELECT * FROM jobs
WHERE status = 'pending' 
   OR (status = 'processing' AND expires_at < datetime('now', 'utc'))
ORDER BY created_at ASC
LIMIT 1;

-- name: GetNextNonceRange :one
-- Get the next available nonce range for a specific prefix
SELECT MAX(nonce_end) as last_nonce_end
FROM jobs
WHERE prefix_28 = ?
AND status IN ('processing', 'completed');

-- name: CreateBatch :one
-- Create a new batch (job) for a worker
INSERT INTO jobs (
    prefix_28, 
    nonce_start, 
    nonce_end,
    current_nonce,
    status, 
    worker_id,
    worker_type,
    expires_at,
    requested_batch_size
)
VALUES (?, ?, ?, ?, 'processing', ?, ?, datetime('now', 'utc', '+' || :lease_seconds || ' seconds'), ?)
RETURNING *;

-- name: FindIncompleteMacroJob :one
-- Find an existing non-completed (macro) job for a given prefix
SELECT * FROM jobs
WHERE prefix_28 = ?
    AND status != 'completed'
ORDER BY created_at ASC
LIMIT 1;

-- name: CreateMacroJob :one
-- Create a long-lived macro job covering the full nonce space for a prefix
INSERT INTO jobs (
        prefix_28,
        nonce_start,
        nonce_end,
        current_nonce,
        status,
        worker_id,
        worker_type,
        expires_at,
        requested_batch_size
)
VALUES (?, ?, ?, ?, 'processing', ?, ?, datetime('now', 'utc', '+' || :lease_seconds || ' seconds'), ?)
RETURNING *;

-- name: LeaseMacroJob :execrows
-- Lease an existing macro job to a worker (if not completed and available)
UPDATE jobs
SET status = 'processing',
        worker_id = ?,
        worker_type = ?,
        expires_at = datetime('now', 'utc', '+' || :lease_seconds || ' seconds')
WHERE id = ?
    AND status != 'completed'
    AND (worker_id IS NULL OR expires_at < datetime('now', 'utc'));

-- name: LeaseBatch :execrows
-- Lease an existing batch to a worker
UPDATE jobs
SET 
    status = 'processing',
    worker_id = ?,
    worker_type = ?,
    expires_at = datetime('now', 'utc', '+' || :lease_seconds || ' seconds')
WHERE id = ? 
  AND (status = 'pending' OR (status = 'processing' AND (expires_at < datetime('now', 'utc') OR worker_id IS NULL)));

-- name: UpdateCheckpoint :exec
-- Update job progress checkpoint
UPDATE jobs
SET 
    current_nonce = ?,
    keys_scanned = ?,
    duration_ms = ?,
    last_checkpoint_at = datetime('now', 'utc')
WHERE id = ? AND worker_id = ? AND status = 'processing';

-- name: CompleteBatch :exec
-- Mark a batch as completed
UPDATE jobs
SET 
    status = 'completed',
    completed_at = datetime('now', 'utc'),
    keys_scanned = ?,
    duration_ms = ?,
    current_nonce = nonce_end
WHERE id = ? AND worker_id = ?;

-- name: GetJobByID :one
-- Get a specific job by ID
SELECT * FROM jobs
WHERE id = ?;

-- name: GetJobsByWorker :many
-- Get all jobs assigned to a specific worker
SELECT * FROM jobs
WHERE worker_id = ?
ORDER BY created_at DESC;

-- name: GetJobsByStatus :many
-- Get jobs by status
SELECT * FROM jobs
WHERE status = ?
ORDER BY created_at DESC
LIMIT ?;

-- name: InsertResult :one
-- Insert a new result (found key)
INSERT INTO results (private_key, address, worker_id, job_id, nonce_found)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetResultByPrivateKey :one
-- Find a result by private key
SELECT * FROM results
WHERE private_key = ?;

-- name: GetResultsByAddress :many
-- Find results by Ethereum address
SELECT * FROM results
WHERE address = ?
ORDER BY found_at DESC;

-- name: GetAllResults :many
-- Get all results (limited)
SELECT * FROM results
ORDER BY found_at DESC
LIMIT ?;

-- name: GetWorkerLastPrefix :one
-- Tracks the last prefix assigned to a worker to enable vertical exhaustion
SELECT prefix_28, MAX(nonce_end) as highest_nonce
FROM jobs
WHERE worker_id = ?
GROUP BY prefix_28
ORDER BY MAX(created_at) DESC
LIMIT 1;

-- name: UpsertWorker :exec
-- Insert or update worker heartbeat
INSERT INTO workers (id, worker_type, last_seen, metadata, updated_at)
VALUES (?, ?, datetime('now', 'utc'), ?, datetime('now','utc'))
ON CONFLICT(id) DO UPDATE SET
    last_seen = datetime('now', 'utc'),
    metadata = excluded.metadata,
    updated_at = datetime('now','utc');

-- name: UpdateWorkerKeyCount :exec
-- Update worker's total key count
UPDATE workers
SET total_keys_scanned = total_keys_scanned + ?
WHERE id = ?;

-- name: GetWorkerByID :one
-- Get worker information by ID
SELECT * FROM workers
WHERE id = ?;

-- name: GetActiveWorkers :many
-- Get workers active in the last N minutes
SELECT * FROM workers
WHERE last_seen > datetime('now', '-' || ? || ' minutes')
ORDER BY last_seen DESC;

-- name: GetWorkersByType :many
-- Get all workers of a specific type
SELECT * FROM workers
WHERE worker_type = ?
ORDER BY last_seen DESC;

-- name: GetStats :one
-- Get aggregated statistics
SELECT * FROM stats_summary;

-- name: GetPrefixUsage :many
-- Get usage statistics per prefix
SELECT 
    prefix_28,
    COUNT(*) as total_batches,
    SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as completed_batches,
    MAX(nonce_end) as highest_nonce,
    SUM(keys_scanned) as total_keys_scanned
FROM jobs
GROUP BY prefix_28
ORDER BY prefix_28
LIMIT ?;

-- name: RecordWorkerStats :exec
-- Insert a raw worker history record (tier 1)
INSERT INTO worker_history (
    worker_id, worker_type, job_id, batch_size, keys_scanned, duration_ms, keys_per_second, prefix_28, nonce_start, nonce_end, finished_at, error_message
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetRecentWorkerHistory :many
SELECT * FROM worker_history
WHERE finished_at > datetime('now', '-' || ? || ' seconds')
ORDER BY finished_at DESC
LIMIT ?;

-- name: GetWorkerDailyStats :many
-- Accept a full timestamp/time.Time parameter but compare only the date portion (YYYY-MM-DD)
-- This makes the generated sqlc method usable directly with a Go time.Time value.
SELECT * FROM worker_stats_daily
WHERE worker_id = ? AND stats_date >= substr(?, 1, 10)
ORDER BY stats_date DESC;

-- name: GetWorkerMonthlyStats :many
SELECT * FROM worker_stats_monthly
WHERE worker_id = ? AND stats_month >= ?
ORDER BY stats_month DESC;

-- name: GetWorkerLifetimeStats :one
SELECT * FROM worker_stats_lifetime
WHERE worker_id = ? LIMIT 1;

-- name: GetAllWorkerLifetimeStats :many
SELECT * FROM worker_stats_lifetime
ORDER BY total_keys_scanned DESC;

-- name: GetWorkerStats :many
-- Get statistics per worker
SELECT 
    w.id,
    w.worker_type,
    w.total_keys_scanned,
    w.last_seen,
    COUNT(j.id) as total_jobs,
    SUM(CASE WHEN j.status = 'processing' THEN 1 ELSE 0 END) as active_jobs,
    SUM(CASE WHEN j.status = 'completed' THEN 1 ELSE 0 END) as completed_jobs
FROM workers w
LEFT JOIN jobs j ON j.worker_id = w.id
GROUP BY w.id
ORDER BY w.total_keys_scanned DESC
LIMIT ?;

-- name: CleanupStaleJobs :exec
-- Clear worker assignment for long-stale processing jobs so they can be re-leased.
UPDATE jobs
SET worker_id = NULL, status = "pending", expires_at = NULL
WHERE status = "processing"
    AND (
        (last_checkpoint_at IS NOT NULL AND last_checkpoint_at < datetime("now", "utc", "-" || :threshold_seconds || " seconds"))
        OR (last_checkpoint_at IS NULL AND created_at < datetime("now", "utc", "-" || :threshold_seconds || " seconds"))
    );
