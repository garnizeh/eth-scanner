package database

import (
	"context"
	"testing"
	"time"
)

// Simple dashboard query perf measurement: exercise GetStats and record duration
func TestDashboardQueryPerf(t *testing.T) {
	ctx := context.Background()
	db, q := setupDBForTests(t)

	// Insert some jobs to make stats non-trivial
	for i := 0; i < 100; i++ {
		start := i * 10
		end := start + 9
		_, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, requested_batch_size, created_at) VALUES (?, ?, ?, 'pending', ?, datetime('now','utc'))`, []byte{0x01}, start, end, 10)
		if err != nil {
			t.Fatalf("insert job failed: %v", err)
		}
	}

	// Run GetStats a few times and measure
	runs := 5
	var total time.Duration
	for i := 0; i < runs; i++ {
		start := time.Now()
		_, err := q.GetStats(ctx)
		if err != nil {
			t.Fatalf("GetStats failed: %v", err)
		}
		total += time.Since(start)
	}
	avg := total / time.Duration(runs)
	t.Logf("GetStats average duration: %s", avg)
}
