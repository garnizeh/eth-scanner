package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleResultSubmit_Success(t *testing.T) {
	s, db, _ := setupServer(t)
	ctx := t.Context()

	// insert a job to reference
	prefix := make([]byte, 28)
	res, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, current_nonce, requested_batch_size) VALUES (?, ?, ?, 'processing', ?, ?, ?)`, prefix, 0, 999, "worker-1", 0, 1000)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	id, _ := res.LastInsertId()

	req := map[string]any{"worker_id": "worker-1", "job_id": id, "private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "address": "0x0123456789abcdef0123456789abcdef01234567", "nonce": 5}
	b, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/results", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		ID         int64  `json:"id"`
		PrivateKey string `json:"private_key"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if out.PrivateKey == "" {
		t.Fatalf("expected private_key in response")
	}
}

func TestHandleResultSubmit_InvalidPrivateKey(t *testing.T) {
	s, _, _ := setupServer(t)
	req := map[string]any{"worker_id": "worker-1", "job_id": 1, "private_key": "vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv", "address": "0x0123456789abcdef0123456789abcdef01234567", "nonce": 5}
	b, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/results", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleResultSubmit_InvalidAddress(t *testing.T) {
	s, _, _ := setupServer(t)
	req := map[string]any{"worker_id": "worker-1", "job_id": 1, "private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "address": "012345", "nonce": 5}
	b, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/results", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleResultSubmit_MissingWorkerID(t *testing.T) {
	s, _, _ := setupServer(t)
	req := map[string]any{"job_id": 1, "private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "address": "0x0123456789abcdef0123456789abcdef01234567", "nonce": 5}
	b, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/results", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for missing worker_id, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleResultSubmit_PrivateKeyNotHex(t *testing.T) {
	s, db, _ := setupServer(t)
	ctx := t.Context()
	// insert job
	prefix := make([]byte, 28)
	res, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, current_nonce, requested_batch_size) VALUES (?, ?, ?, 'processing', ?, ?, ?)`, prefix, 0, 999, "worker-1", 0, 1000)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	id, _ := res.LastInsertId()

	// 64 chars but contains non-hex 'zz'
	pk := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abzz"
	req := map[string]any{"worker_id": "worker-1", "job_id": id, "private_key": pk, "address": "0x0123456789abcdef0123456789abcdef01234567", "nonce": 5}
	b, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/results", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for non-hex private_key, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleResultSubmit_AddressNotHex(t *testing.T) {
	s, db, _ := setupServer(t)
	ctx := t.Context()
	// insert job
	prefix := make([]byte, 28)
	res, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, worker_id, current_nonce, requested_batch_size) VALUES (?, ?, ?, 'processing', ?, ?, ?)`, prefix, 0, 999, "worker-1", 0, 1000)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	id, _ := res.LastInsertId()

	// address with 0x prefix and correct length but contains non-hex 'zz'
	base := "0123456789abcdef0123456789abcdef01234567" // 40 chars
	addr := "0x" + base[:38] + "zz"                    // replace last two chars with 'zz'
	if len(addr) != 42 {
		t.Fatalf("test setup: addr length=%d, want 42", len(addr))
	}
	req := map[string]any{"worker_id": "worker-1", "job_id": id, "private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "address": addr, "nonce": 5}
	b, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/results", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for non-hex address, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleResultSubmit_MissingJobID(t *testing.T) {
	s, _, _ := setupServer(t)
	req := map[string]any{"worker_id": "worker-1", "private_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "address": "0x0123456789abcdef0123456789abcdef01234567", "nonce": 5}
	b, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/results", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for missing job_id, got %d: %s", w.Code, w.Body.String())
	}
}
