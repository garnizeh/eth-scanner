package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestHandleJobCheckpoint_Success(t *testing.T) {
	s, db, _ := setupServer(t)
	ctx := t.Context()

	// insert processing job
	prefix := make([]byte, 28)
	res, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, current_nonce, requested_batch_size) VALUES (?, ?, ?, 'processing', ?, ?, ?)`, prefix, 0, 999, "worker-1", 0, 1000)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	id, _ := res.LastInsertId()

	req := map[string]any{"worker_id": "worker-1", "current_nonce": 5, "keys_scanned": 5}
	b, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/jobs/"+strconv.FormatInt(id, 10)+"/checkpoint", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		JobID int64 `json:"job_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if out.JobID != id {
		t.Fatalf("unexpected job id: %d", out.JobID)
	}
}

func TestHandleJobCheckpoint_WorkerMismatch(t *testing.T) {
	s, db, _ := setupServer(t)
	ctx := t.Context()
	prefix := make([]byte, 28)
	res, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, current_nonce, requested_batch_size) VALUES (?, ?, ?, 'processing', ?, ?, ?)`, prefix, 0, 999, "worker-1", 0, 1000)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	id, _ := res.LastInsertId()

	req := map[string]any{"worker_id": "other", "current_nonce": 5, "keys_scanned": 5}
	b, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/jobs/"+strconv.FormatInt(id, 10)+"/checkpoint", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleJobCheckpoint_NotFound(t *testing.T) {
	s, _, _ := setupServer(t)
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/jobs/99999/checkpoint", bytes.NewReader([]byte(`{"worker_id":"x","current_nonce":1,"keys_scanned":1}`)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleJobCheckpoint_MethodNotAllowed(t *testing.T) {
	s, _, _ := setupServer(t)
	// Use a non-PATCH method; handler should return 405 before parsing the path
	r := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/123/checkpoint", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 Method Not Allowed, got %d: %s", w.Code, w.Body.String())
	}
}
