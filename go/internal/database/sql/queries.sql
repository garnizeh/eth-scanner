-- name: FindAvailableBatch :one
-- Find an available batch (pending or expired lease, or already assigned to same worker)
SELECT * FROM jobs
WHERE status = 'pending' 
   OR (status = 'processing' AND (expires_at < datetime('now', 'utc') OR worker_id = :worker_id))
ORDER BY created_at ASC
LIMIT 1;

-- name: GetNextNonceRange :one
-- Get the next available nonce range for a specific prefix
SELECT MAX(nonce_end) as last_nonce_end
FROM jobs
WHERE prefix_28 = :prefix_28
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
VALUES (:prefix_28, :nonce_start, :nonce_end, :nonce_start, 'processing', :worker_id, :worker_type, datetime('now', 'utc', '+' || :lease_seconds || ' seconds'), :requested_batch_size)
RETURNING *;

-- name: FindIncompleteMacroJob :one
-- Find an existing non-completed (macro) job for a given prefix
SELECT * FROM jobs
WHERE prefix_28 = :prefix_28
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
VALUES (:prefix_28, :nonce_start, :nonce_end, :nonce_start, 'processing', :worker_id, :worker_type, datetime('now', 'utc', '+' || :lease_seconds || ' seconds'), :requested_batch_size)
RETURNING *;

-- name: LeaseMacroJob :execrows
-- Lease an existing macro job to a worker (if not completed and available)
UPDATE jobs
SET status = 'processing',
        worker_id = :worker_id,
        worker_type = :worker_type,
        expires_at = datetime('now', 'utc', '+' || :lease_seconds || ' seconds')
WHERE id = :id
    AND status != 'completed'
    AND (worker_id IS NULL OR worker_id = :worker_id OR expires_at < datetime('now', 'utc'));

-- name: LeaseBatch :execrows
-- Lease an existing batch to a worker
UPDATE jobs
SET 
    status = 'processing',
    worker_id = :worker_id,
    worker_type = :worker_type,
    expires_at = datetime('now', 'utc', '+' || :lease_seconds || ' seconds')
WHERE id = :id 
  AND (status = 'pending' OR (status = 'processing' AND (expires_at < datetime('now', 'utc') OR worker_id IS NULL OR worker_id = :worker_id)));

-- name: UpdateCheckpoint :exec
-- Update job progress checkpoint
UPDATE jobs
SET 
    current_nonce = :current_nonce,
    keys_scanned = :keys_scanned,
    duration_ms = :duration_ms,
    last_checkpoint_at = datetime('now', 'utc')
WHERE id = :id AND worker_id = :worker_id AND status = 'processing';

-- name: CompleteBatch :exec
-- Mark a batch as completed
UPDATE jobs
SET 
    status = 'completed',
    completed_at = datetime('now', 'utc'),
    keys_scanned = :keys_scanned,
    duration_ms = :duration_ms,
    current_nonce = nonce_end
WHERE id = :id AND worker_id = :worker_id;

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

-- name: GetActiveWorkerDetails :many
-- Get detailed info about currently active workers for dashboard
SELECT 
    w.id,
    w.worker_type,
    w.last_seen,
    w.total_keys_scanned,
    j.prefix_28 as active_prefix,
    j.current_nonce,
    j.nonce_start,
    j.nonce_end,
    (SELECT h.keys_per_second 
     FROM worker_history h 
     WHERE h.worker_id = w.id 
     ORDER BY h.finished_at DESC LIMIT 1) as last_kps
FROM workers w
LEFT JOIN jobs j ON j.worker_id = w.id AND j.status = 'processing'
WHERE w.last_seen > datetime('now', '-5 minutes')
ORDER BY w.last_seen DESC;

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
-- Get recent worker history records for the last N seconds
SELECT * FROM worker_history
WHERE finished_at > datetime('now', '-' || ? || ' seconds')
ORDER BY finished_at DESC
LIMIT ?;

-- name: GetGlobalDailyStats :many
-- Get daily aggregates for all workers, combining archived and recent history
SELECT 
    stats_date,
    SUM(total_batches) as total_batches,
    SUM(total_keys_scanned) as total_keys_scanned,
    SUM(total_duration_ms) as total_duration_ms,
    AVG(keys_per_second_avg) as keys_per_second_avg,
    SUM(error_count) as total_errors
FROM (
    -- Archived historical data
    SELECT 
        stats_date,
        total_batches,
        total_keys_scanned,
        total_duration_ms,
        keys_per_second_avg,
        error_count
    FROM worker_stats_daily
    WHERE stats_date >= substr(:since_date, 1, 10)

    UNION ALL

    -- Recent history data (not yet pruned/archived)
    SELECT 
        date(finished_at) as stats_date,
        1 as total_batches,
        keys_scanned as total_keys_scanned,
        duration_ms as total_duration_ms,
        keys_per_second as keys_per_second_avg,
        CASE WHEN error_message IS NOT NULL THEN 1 ELSE 0 END as error_count
    FROM worker_history
    WHERE finished_at >= substr(:since_date, 1, 10)
)
GROUP BY stats_date
ORDER BY stats_date DESC;

-- name: GetWorkerDailyStats :many
-- Get daily aggregates for a worker, combining archived and recent history
-- We select and group by stats_date only, as worker_id is filtered to a single value
SELECT 
    stats_date,
    SUM(total_batches) as total_batches,
    SUM(total_keys_scanned) as total_keys_scanned,
    SUM(total_duration_ms) as total_duration_ms,
    AVG(keys_per_second_avg) as keys_per_second_avg,
    SUM(error_count) as total_errors
FROM (
    -- Archived historical data
    SELECT 
        stats_date,
        total_batches,
        total_keys_scanned,
        total_duration_ms,
        keys_per_second_avg,
        error_count
    FROM worker_stats_daily wsd
    WHERE wsd.worker_id = :worker_id AND wsd.stats_date >= substr(:since_date, 1, 10)

    UNION ALL

    -- Recent history data (not yet pruned/archived)
    SELECT 
        date(finished_at) as stats_date,
        1 as total_batches,
        keys_scanned as total_keys_scanned,
        duration_ms as total_duration_ms,
        keys_per_second as keys_per_second_avg,
        CASE WHEN error_message IS NOT NULL THEN 1 ELSE 0 END as error_count
    FROM worker_history wh
    WHERE wh.worker_id = :worker_id AND wh.finished_at >= substr(:since_date, 1, 10)
) AS combined
GROUP BY stats_date
ORDER BY stats_date DESC;

-- name: GetMonthlyStatsByWorker :many
-- Get monthly aggregates for a specific worker, combining archived and recent history
SELECT 
    stats_month,
    SUM(total_batches) as total_batches,
    SUM(total_keys_scanned) as total_keys_scanned,
    SUM(total_duration_ms) as total_duration_ms,
    AVG(keys_per_second_avg) as keys_per_second_avg,
    SUM(error_count) as total_errors
FROM (
    -- Archived monthly data
    SELECT 
        stats_month,
        total_batches,
        total_keys_scanned,
        total_duration_ms,
        keys_per_second_avg,
        error_count
    FROM worker_stats_monthly wsm
    WHERE wsm.worker_id = :worker_id AND wsm.stats_month >= substr(:since_month, 1, 7)

    UNION ALL

    -- Recent history data (not yet pruned)
    SELECT 
        substr(finished_at, 1, 7) as stats_month,
        1 as total_batches,
        keys_scanned as total_keys_scanned,
        duration_ms as total_duration_ms,
        keys_per_second as keys_per_second_avg,
        CASE WHEN error_message IS NOT NULL AND error_message != '' THEN 1 ELSE 0 END as error_count
    FROM worker_history wh
    WHERE wh.worker_id = :worker_id AND wh.finished_at >= substr(:since_month, 1, 7)
) AS combined
GROUP BY stats_month
ORDER BY stats_month DESC;

-- name: GetGlobalMonthlyStats :many
-- Get monthly aggregates for all workers, combining archived and recent history
SELECT 
    stats_month,
    SUM(total_batches) as total_batches,
    SUM(total_keys_scanned) as total_keys_scanned,
    SUM(total_duration_ms) as total_duration_ms,
    AVG(keys_per_second_avg) as keys_per_second_avg,
    SUM(error_count) as total_errors
FROM (
    -- Archived monthly data
    SELECT 
        stats_month,
        total_batches,
        total_keys_scanned,
        total_duration_ms,
        keys_per_second_avg,
        error_count
    FROM worker_stats_monthly
    WHERE stats_month >= substr(:since_month, 1, 7)

    UNION ALL

    -- Recent history data (not yet pruned)
    SELECT 
        substr(finished_at, 1, 7) as stats_month,
        1 as total_batches,
        keys_scanned as total_keys_scanned,
        duration_ms as total_duration_ms,
        keys_per_second as keys_per_second_avg,
        CASE WHEN error_message IS NOT NULL AND error_message != '' THEN 1 ELSE 0 END as error_count
    FROM worker_history
    WHERE finished_at >= substr(:since_month, 1, 7)
)
GROUP BY stats_month
ORDER BY stats_month DESC;

-- name: GetBestDayRecord :one
-- Get the day with highest volume across all workers
SELECT stats_date, SUM(total_keys_scanned) as total_keys
FROM (
    SELECT stats_date, total_keys_scanned FROM worker_stats_daily
    UNION ALL
    SELECT date(finished_at) as stats_date, keys_scanned as total_keys_scanned FROM worker_history
)
GROUP BY stats_date
ORDER BY total_keys DESC
LIMIT 1;

-- name: GetBestMonthRecord :one
-- Get the month with highest volume across all workers
SELECT stats_month, SUM(total_keys_scanned) as total_keys
FROM (
    SELECT stats_month, total_keys_scanned FROM worker_stats_monthly
    UNION ALL
    SELECT substr(finished_at, 1, 7) as stats_month, keys_scanned as total_keys_scanned FROM worker_history
)
GROUP BY stats_month
ORDER BY total_keys DESC
LIMIT 1;

-- name: GetWorkerLifetimeStats :one
-- Get lifetime stats for a worker
SELECT * FROM worker_stats_lifetime
WHERE worker_id = ? LIMIT 1;

-- name: GetAllWorkerLifetimeStats :many
-- Get unified lifetime stats for all workers, combining archived tier 4 and recent tier 1
SELECT 
    worker_id,
    CAST(MAX(worker_type) AS TEXT) as worker_type,
    CAST(SUM(total_batches) AS INTEGER) as total_batches,
    CAST(SUM(total_keys_scanned) AS INTEGER) as total_keys_scanned,
    CAST(SUM(total_duration_ms) AS INTEGER) as total_duration_ms,
    AVG(keys_per_second_avg) as keys_per_second_avg,
    CAST(MAX(keys_per_second_best) AS REAL) as keys_per_second_best,
    CAST(MIN(IFNULL(keys_per_second_worst, 999999999)) AS REAL) as keys_per_second_worst,
    MIN(first_seen_at) as first_seen_at,
    MAX(last_seen_at) as last_seen_at
FROM (
    -- Archived Tier 4
    SELECT 
        worker_id, worker_type, total_batches, total_keys_scanned, total_duration_ms,
        keys_per_second_avg, keys_per_second_best, keys_per_second_worst, first_seen_at, last_seen_at
    FROM worker_stats_lifetime
    
    UNION ALL
    
    -- Recent Tier 1
    SELECT 
        worker_id, worker_type, 1 as total_batches, COALESCE(keys_scanned, 0) as total_keys_scanned, COALESCE(duration_ms, 0) as total_duration_ms,
        keys_per_second as keys_per_second_avg, keys_per_second as keys_per_second_best, keys_per_second as keys_per_second_worst,
        finished_at as first_seen_at, finished_at as last_seen_at
    FROM worker_history
) AS combined
GROUP BY worker_id
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
SET worker_id = NULL, status = 'pending', expires_at = NULL
WHERE status = 'processing'
    AND (
        (last_checkpoint_at IS NOT NULL AND last_checkpoint_at < datetime('now', 'utc', '-' || :threshold_seconds || ' seconds'))
        OR (last_checkpoint_at IS NULL AND created_at < datetime('now', 'utc', '-' || :threshold_seconds || ' seconds'))
    );
