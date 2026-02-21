-- +goose Up
-- Update stats_summary view to includes global_keys_per_second
DROP VIEW IF EXISTS stats_summary;

CREATE VIEW stats_summary AS
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
    
    -- Global throughput (sum of latest KPS for each worker active in last 10m)
    (SELECT COALESCE(SUM(keys_per_second), 0) FROM (
        SELECT keys_per_second, MAX(finished_at)
        FROM worker_history
        WHERE finished_at > datetime('now', '-10 minutes')
        GROUP BY worker_id
    )) AS global_keys_per_second,

    -- Prefix progress (distinct prefixes being worked on)
    COUNT(DISTINCT prefix_28) AS active_prefixes
FROM jobs;

-- +goose Down
-- Revert view (not strictly necessary but follow convention)
DROP VIEW IF EXISTS stats_summary;
