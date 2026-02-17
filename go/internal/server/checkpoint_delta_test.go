package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/garnizeh/eth-scanner/internal/database"
)

// Test that multiple cumulative checkpoints produce delta entries in
// worker_history and that workers.total_keys_scanned is incremented by the
// deltas (trigger behavior).
func TestCheckpointRecordsDeltas(t *testing.T) {
	s, db, q := setupServer(t)
	ctx := t.Context()

	// ensure worker exists
	workerID := "worker-delta-test"
	if err := q.UpsertWorker(ctx, database.UpsertWorkerParams{ID: workerID, WorkerType: "pc", Metadata: sql.NullString{Valid: false}}); err != nil {
		t.Fatalf("UpsertWorker failed: %v", err)
	}

	// insert processing job with zeroed progress
	prefix := make([]byte, 28)
	res, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, current_nonce, keys_scanned, duration_ms) VALUES (?, ?, ?, 'processing', ?, ?, ?, ?)`, prefix, 0, 9999, workerID, 0, 0, 0)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	id, _ := res.LastInsertId()

	// helper to perform checkpoint request
	doCheckpoint := func(keysScanned int64, durationMs int64, currentNonce int64) *httptest.ResponseRecorder {
		req := map[string]any{"worker_id": workerID, "current_nonce": currentNonce, "keys_scanned": keysScanned, "duration_ms": durationMs}
		b, _ := json.Marshal(req)
		r := httptest.NewRequest(http.MethodPatch, "/api/v1/jobs/"+strconv.FormatInt(id, 10)+"/checkpoint", bytesReader(b))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.router.ServeHTTP(w, r)
		return w
	}

	// first cumulative checkpoint: 100 keys, 1000 ms
	w1 := doCheckpoint(100, 1000, 100)
	if w1.Code != http.StatusOK {
		t.Fatalf("first checkpoint failed: %d %s", w1.Code, w1.Body.String())
	}

	// wait for async insert
	var got int
	for i := 0; i < 20; i++ {
		row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_history WHERE worker_id = ?", workerID)
		_ = row.Scan(&got)
		if got >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got < 1 {
		t.Fatalf("expected at least 1 worker_history row, got %d", got)
	}

	// second cumulative checkpoint: 300 keys, 3000 ms -> delta should be 200/2000
	w2 := doCheckpoint(300, 3000, 300)
	if w2.Code != http.StatusOK {
		t.Fatalf("second checkpoint failed: %d %s", w2.Code, w2.Body.String())
	}

	// wait for second async insert
	for i := 0; i < 20; i++ {
		row := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM worker_history WHERE worker_id = ?", workerID)
		_ = row.Scan(&got)
		if got >= 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got < 2 {
		t.Fatalf("expected at least 2 worker_history rows, got %d", got)
	}

	// fetch keys_scanned and duration_ms ordered by id
	rows, err := db.QueryContext(ctx, "SELECT keys_scanned, duration_ms FROM worker_history WHERE worker_id = ? ORDER BY id ASC", workerID)
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	defer rows.Close()
	var vals []struct{ K, D int64 }
	for rows.Next() {
		var k, d sql.NullInt64
		if err := rows.Scan(&k, &d); err != nil {
			t.Fatalf("scan: %v", err)
		}
		kv := int64(0)
		dv := int64(0)
		if k.Valid {
			kv = k.Int64
		}
		if d.Valid {
			dv = d.Int64
		}
		vals = append(vals, struct{ K, D int64 }{kv, dv})
	}

	if len(vals) < 2 {
		t.Fatalf("expected >=2 history rows, got %d", len(vals))
	}
	if vals[0].K != 100 || vals[0].D != 1000 {
		t.Fatalf("first history row mismatch: got %+v, want keys=100 dur=1000", vals[0])
	}
	if vals[1].K != 200 || vals[1].D != 2000 {
		t.Fatalf("second history row mismatch (delta): got %+v, want keys=200 dur=2000", vals[1])
	}

	// verify workers.total_keys_scanned equals cumulative 300
	var total sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT total_keys_scanned FROM workers WHERE id = ?", workerID).Scan(&total); err != nil {
		t.Fatalf("query worker total: %v", err)
	}
	if !total.Valid || total.Int64 != 300 {
		t.Fatalf("worker total mismatch: got %v, want 300", total.Int64)
	}
}

// bytesReader returns an io.Reader for the given bytes slice.
// Included as a tiny helper to avoid importing bytes in multiple tests.
func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
