package server

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garnizeh/eth-scanner/internal/config"
	"github.com/garnizeh/eth-scanner/internal/database"
	"github.com/garnizeh/eth-scanner/internal/jobs"
)

func setupServer(t *testing.T) (*Server, *sql.DB, *database.Queries) {
	t.Helper()
	ctx := t.Context()
	db, err := database.InitDB(ctx, ":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	q := database.NewQueries(db)
	cfg := &config.Config{Port: "0", DBPath: ":memory:"}
	s := New(cfg, db)
	s.RegisterRoutes()
	t.Cleanup(func() {
		if err := database.CloseDB(db); err != nil {
			t.Fatalf("CloseDB: %v", err)
		}
	})
	return s, db, q
}

func TestHandleJobLease_CreateBatchAndLease(t *testing.T) {
	s, _, q := setupServer(t)
	ctx := t.Context()

	// Request a lease with a reasonable batch size
	req := map[string]any{
		"worker_id":            "worker-1",
		"requested_batch_size": 1000,
	}
	b, _ := json.Marshal(req)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/lease", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		JobID      int64  `json:"job_id"`
		Prefix28   string `json:"prefix_28"`
		NonceStart int64  `json:"nonce_start"`
		NonceEnd   int64  `json:"nonce_end"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Validate returned values and DB state
	job, err := q.GetJobByID(ctx, resp.JobID)
	if err != nil {
		t.Fatalf("GetJobByID: %v", err)
	}
	if !job.WorkerID.Valid || job.WorkerID.String != "worker-1" {
		t.Fatalf("expected worker_id set to worker-1, got %+v", job.WorkerID)
	}
	if job.Status != "processing" {
		t.Fatalf("expected status processing, got %s", job.Status)
	}
	if resp.NonceStart != job.NonceStart || resp.NonceEnd != job.NonceEnd {
		t.Fatalf("response nonce range mismatches job: resp %d-%d db %d-%d", resp.NonceStart, resp.NonceEnd, job.NonceStart, job.NonceEnd)
	}
	// ensure prefix decodes
	_, err = base64.StdEncoding.DecodeString(resp.Prefix28)
	if err != nil {
		t.Fatalf("invalid base64 prefix in response: %v", err)
	}
}

func TestHandleJobLease_LeaseExistingPendingJob(t *testing.T) {
	s, db, q := setupServer(t)
	ctx := t.Context()

	// Insert a pending job directly
	prefix := make([]byte, 28)
	if _, err := db.ExecContext(ctx, `INSERT INTO jobs (prefix_28, nonce_start, nonce_end, status, requested_batch_size) VALUES (?, ?, ?, 'pending', ?)`, prefix, 0, 999, 1000); err != nil {
		t.Fatalf("insert pending job: %v", err)
	}

	req := map[string]any{
		"worker_id":            "worker-2",
		"requested_batch_size": 1000,
	}
	b, _ := json.Marshal(req)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/lease", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		JobID int64 `json:"job_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	job, err := q.GetJobByID(ctx, resp.JobID)
	if err != nil {
		t.Fatalf("GetJobByID: %v", err)
	}
	if !job.WorkerID.Valid || job.WorkerID.String != "worker-2" {
		t.Fatalf("expected worker_id worker-2, got %+v", job.WorkerID)
	}
}

func TestHandleJobLease_ValidationErrors(t *testing.T) {
	s, _, _ := setupServer(t)

	// Missing worker_id
	req := map[string]any{"requested_batch_size": 100}
	b, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/lease", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for missing worker_id, got %d", w.Code)
	}

	// Invalid batch size (0)
	req2 := map[string]any{"worker_id": "w", "requested_batch_size": 0}
	b2, _ := json.Marshal(req2)
	r2 := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/lease", bytes.NewReader(b2))
	r2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	s.router.ServeHTTP(w2, r2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for invalid batch size, got %d", w2.Code)
	}
}

func TestCreateAndLeaseBatch_InvalidBase64(t *testing.T) {
	s, _, q := setupServer(t)
	ctx := t.Context()
	m := jobs.New(q)

	invalid := "!!!not_base64!!!"
	job, err := s.createAndLeaseBatch(ctx, m, q, "worker-x", "pc", &invalid, 100)
	if err == nil {
		t.Fatalf("expected error for invalid base64, got job: %+v", job)
	}
}

func TestCreateAndLeaseBatch_WrongLength(t *testing.T) {
	s, _, q := setupServer(t)
	ctx := t.Context()
	m := jobs.New(q)

	// base64 of 10 bytes (not 28)
	short := base64.StdEncoding.EncodeToString(make([]byte, 10))
	job, err := s.createAndLeaseBatch(ctx, m, q, "worker-y", "pc", &short, 100)
	if err == nil {
		t.Fatalf("expected error for wrong length prefix, got job: %+v", job)
	}
}

func TestCreateAndLeaseBatch_Success(t *testing.T) {
	s, _, q := setupServer(t)
	ctx := t.Context()
	m := jobs.New(q)

	prefix := make([]byte, 28)
	// deterministic zeros are fine for test
	enc := base64.StdEncoding.EncodeToString(prefix)
	job, err := s.createAndLeaseBatch(ctx, m, q, "worker-z", "pc", &enc, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job == nil {
		t.Fatal("expected job, got nil")
	}
	if len(job.Prefix28) != 28 {
		t.Fatalf("expected prefix length 28, got %d", len(job.Prefix28))
	}
	if !job.WorkerID.Valid || job.WorkerID.String != "worker-z" {
		t.Fatalf("expected worker_id worker-z, got %+v", job.WorkerID)
	}
}
