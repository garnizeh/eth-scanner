package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDoRequestWithAPIKeySuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Fatalf("expected X-API-Key header to be present")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("expected Content-Type application/json")
		}
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: "test-key"}
	c := NewClient(cfg)

	var resp map[string]string
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.doRequestWithContext(ctx, "GET", "/api/test", nil, &resp); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("unexpected response: %v", resp)
	}
}

func TestDoRequestWithoutAPIKeySuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// X-API-Key should not be present
		if r.Header.Get("X-API-Key") != "" {
			t.Fatalf("expected no X-API-Key header when client has none")
		}
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: ""}
	c := NewClient(cfg)

	var resp map[string]string
	if err := c.doRequestWithContext(context.Background(), "GET", "/api/test", nil, &resp); err != nil {
		t.Fatalf("request failed: %v", err)
	}
}

func TestDoRequestErrorResponseParsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "bad", "message": "invalid input"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: ""}
	c := NewClient(cfg)

	err := c.doRequestWithContext(context.Background(), "POST", "/api/test", map[string]string{"x": "y"}, nil)
	if err == nil {
		t.Fatalf("expected error for 400 response")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", apiErr.StatusCode)
	}
	if apiErr.Message != "invalid input" {
		t.Fatalf("unexpected api error message: %s", apiErr.Message)
	}
}

func TestDoRequestUnauthorizedReturnsErrUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized", "message": "missing api key"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: ""}
	c := NewClient(cfg)

	err := c.doRequestWithContext(context.Background(), "GET", "/api/test", nil, nil)
	if err == nil {
		t.Fatalf("expected error for 401 response")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %T: %v", err, err)
	}
}

func TestDoRequest_InvalidBaseURL(t *testing.T) {
	// Create a client with an invalid base URL to trigger the parse error path.
	cfg := &Config{APIURL: "http://%41:invalid", WorkerID: "w", APIKey: ""}
	c := NewClient(cfg)

	err := c.doRequestWithContext(context.Background(), "GET", "/api/test", nil, nil)
	if err == nil {
		t.Fatalf("expected error for invalid base URL")
	}
	if !strings.Contains(err.Error(), "invalid base url") {
		t.Fatalf("expected error message to include 'invalid base url', got: %v", err)
	}
	if errors.Unwrap(err) == nil {
		t.Fatalf("expected wrapped underlying error, unwrap returned nil")
	}
}

func TestAPIError_ErrorMethod(t *testing.T) {
	e := &APIError{StatusCode: 422, Message: "unprocessable"}
	want := "api error 422: unprocessable"
	if e.Error() != want {
		t.Fatalf("APIError.Error() = %q, want %q", e.Error(), want)
	}
}

func TestLeaseBatch_Success(t *testing.T) {
	prefix := strings.Repeat("ab", 28) // 56 hex chars -> 28 bytes
	expires := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"job_id":      "job-123",
			"prefix_28":   prefix,
			"nonce_start": 1,
			"nonce_end":   10,
			"expires_at":  expires,
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: "test-key"}
	c := NewClient(cfg)

	lease, err := c.LeaseBatch(context.Background(), 100)
	if err != nil {
		t.Fatalf("LeaseBatch failed: %v", err)
	}
	if lease.JobID != "job-123" {
		t.Fatalf("unexpected JobID: %s", lease.JobID)
	}
	if len(lease.Prefix28) != 28 {
		t.Fatalf("unexpected prefix length: %d", len(lease.Prefix28))
	}
}

func TestLeaseBatch_NoJobs404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "no jobs available"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: ""}
	c := NewClient(cfg)

	_, err := c.LeaseBatch(context.Background(), 1)
	if err == nil {
		t.Fatalf("expected ErrNoJobsAvailable")
	}
	if !errors.Is(err, ErrNoJobsAvailable) {
		t.Fatalf("expected ErrNoJobsAvailable, got %T: %v", err, err)
	}
}

func TestLeaseBatch_InvalidPrefixLength(t *testing.T) {
	// prefix too short
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"job_id":      "job-1",
			"prefix_28":   "abcd", // too short
			"nonce_start": 0,
			"nonce_end":   1,
			"expires_at":  time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339),
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: ""}
	c := NewClient(cfg)

	_, err := c.LeaseBatch(context.Background(), 1)
	if err == nil {
		t.Fatalf("expected error for invalid prefix length")
	}
	if !strings.Contains(err.Error(), "invalid prefix_28") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLeaseBatch_InvalidExpiresAt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"job_id":      "job-1",
			"prefix_28":   strings.Repeat("ab", 28),
			"nonce_start": 0,
			"nonce_end":   1,
			"expires_at":  "not-a-time",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: ""}
	c := NewClient(cfg)

	_, err := c.LeaseBatch(context.Background(), 1)
	if err == nil {
		t.Fatalf("expected error for invalid expires_at")
	}
	if !strings.Contains(err.Error(), "invalid expires_at") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLeaseBatch_UnauthorizedReturnsErrUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized", "message": "invalid api key"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: "bad"}
	c := NewClient(cfg)

	_, err := c.LeaseBatch(context.Background(), 1)
	if err == nil {
		t.Fatalf("expected ErrUnauthorized")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %T: %v", err, err)
	}
}

func TestLeaseBatch_APIErrorWrapped(t *testing.T) {
	// Master API returns 500 with an error message; LeaseBatch should wrap the APIError
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "server", "message": "oops"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: ""}
	c := NewClient(cfg)

	_, err := c.LeaseBatch(context.Background(), 1)
	if err == nil {
		t.Fatalf("expected wrapped API error")
	}
	if !strings.Contains(err.Error(), "lease request failed") {
		t.Fatalf("expected top-level error to mention lease request failed, got: %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected underlying APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status 500 inside APIError, got %d", apiErr.StatusCode)
	}
}

func TestLeaseBatch_InvalidPrefixHex(t *testing.T) {
	// prefix contains invalid hex characters -> hex.DecodeString should fail
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"job_id":      "job-1",
			"prefix_28":   strings.Repeat("zz", 28), // invalid hex
			"nonce_start": 0,
			"nonce_end":   1,
			"expires_at":  time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339),
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: ""}
	c := NewClient(cfg)

	_, err := c.LeaseBatch(context.Background(), 1)
	if err == nil {
		t.Fatalf("expected error for invalid prefix hex")
	}
	if !strings.Contains(err.Error(), "invalid prefix_28 hex") {
		t.Fatalf("unexpected error: %v", err)
	}
	if errors.Unwrap(err) == nil {
		t.Fatalf("expected underlying hex error to be wrapped")
	}
}

func TestLeaseBatch_LeaseRequestInvalidBaseURLWrapped(t *testing.T) {
	// Create a client with an invalid base URL to trigger doRequestWithContext parse error
	cfg := &Config{APIURL: "http://%41:invalid", WorkerID: "w", APIKey: ""}
	c := NewClient(cfg)

	_, err := c.LeaseBatch(context.Background(), 1)
	if err == nil {
		t.Fatalf("expected lease request to fail due to invalid base URL")
	}
	if !strings.Contains(err.Error(), "lease request failed") {
		t.Fatalf("expected top-level lease request failed error, got: %v", err)
	}
	if errors.Unwrap(err) == nil {
		t.Fatalf("expected underlying error to be wrapped")
	}
}

func TestUpdateCheckpoint_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("expected PATCH, got %s", r.Method)
		}
		expectedPath := "/api/v1/jobs/test-job-123/checkpoint"
		if r.URL.Path != expectedPath {
			t.Fatalf("expected path %s, got %s", expectedPath, r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req checkpointRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.WorkerID != "test-worker" {
			t.Fatalf("unexpected worker id: %s", req.WorkerID)
		}
		if req.CurrentNonce != 12345 {
			t.Fatalf("unexpected current nonce: %d", req.CurrentNonce)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(&Config{APIURL: server.URL, WorkerID: "test-worker", APIKey: "test-key"})
	if err := c.UpdateCheckpoint(context.Background(), "test-job-123", 12345, 12345, time.Now(), 1000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateCheckpoint_UnauthorizedReturnsErrUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized", "message": "missing api key"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: "bad"}
	c := NewClient(cfg)

	err := c.UpdateCheckpoint(context.Background(), "job-1", 0, 0, time.Now(), 0)
	if err == nil {
		t.Fatalf("expected ErrUnauthorized")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %T: %v", err, err)
	}
}

func TestUpdateCheckpoint_APIErrorWrapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "worker_id mismatch", "message": "job is assigned to different worker"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: ""}
	c := NewClient(cfg)

	err := c.UpdateCheckpoint(context.Background(), "job-1", 0, 0, time.Now(), 0)
	if err == nil {
		t.Fatalf("expected wrapped API error")
	}
	if !strings.Contains(err.Error(), "checkpoint update failed") {
		t.Fatalf("expected top-level error to mention checkpoint update failed, got: %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected underlying APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusConflict {
		t.Fatalf("expected status 409 inside APIError, got %d", apiErr.StatusCode)
	}
}

func TestCompleteBatch_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		expectedPath := "/api/v1/jobs/test-job-456/complete"
		if r.URL.Path != expectedPath {
			t.Fatalf("expected path %s, got %s", expectedPath, r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req completeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.WorkerID != "test-worker" {
			t.Fatalf("unexpected worker id: %s", req.WorkerID)
		}
		if req.FinalNonce != 4294967295 {
			t.Fatalf("unexpected final nonce: %d", req.FinalNonce)
		}
		if req.KeysScanned != 4294967296 {
			t.Fatalf("unexpected keys scanned: %d", req.KeysScanned)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(&Config{APIURL: server.URL, WorkerID: "test-worker", APIKey: "test-key"})
	if err := c.CompleteBatch(context.Background(), "test-job-456", 4294967295, 4294967296, time.Now(), 1000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompleteBatch_UnauthorizedReturnsErrUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized", "message": "missing api key"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: "bad"}
	c := NewClient(cfg)

	err := c.CompleteBatch(context.Background(), "job-1", 0, 0, time.Now(), 0)
	if err == nil {
		t.Fatalf("expected ErrUnauthorized")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %T: %v", err, err)
	}
}

func TestCompleteBatch_APIErrorWrapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "invalid final_nonce", "message": "final_nonce must equal nonce_end"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: ""}
	c := NewClient(cfg)

	err := c.CompleteBatch(context.Background(), "job-1", 0, 0, time.Now(), 0)
	if err == nil {
		t.Fatalf("expected wrapped API error")
	}
	if !strings.Contains(err.Error(), "complete batch failed") {
		t.Fatalf("expected top-level error to mention complete batch failed, got: %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected underlying APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400 inside APIError, got %d", apiErr.StatusCode)
	}
}

func TestSubmitResult_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/results" {
			t.Fatalf("expected /api/v1/results, got %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req resultRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.WorkerID != "test-worker" {
			t.Fatalf("unexpected worker id: %s", req.WorkerID)
		}
		if len(req.PrivateKey) != 64 {
			t.Fatalf("unexpected private key length: %d", len(req.PrivateKey))
		}
		if req.EthereumAddress == "" {
			t.Fatalf("missing ethereum address")
		}
		if _, err := time.Parse(time.RFC3339, req.FoundAt); err != nil {
			t.Fatalf("invalid found_at timestamp: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	c := NewClient(&Config{APIURL: server.URL, WorkerID: "test-worker", APIKey: "test-key"})
	privateKey := make([]byte, 32)
	if err := c.SubmitResult(context.Background(), privateKey, "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSubmitResult_InvalidPrivateKeyLength(t *testing.T) {
	c := NewClient(&Config{APIURL: "http://example.com", WorkerID: "w", APIKey: ""})
	err := c.SubmitResult(context.Background(), make([]byte, 16), "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb")
	if err == nil {
		t.Fatalf("expected error for invalid private key length")
	}
	if !strings.Contains(err.Error(), "invalid private key length") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSubmitResult_UnauthorizedReturnsErrUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized", "message": "missing api key"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: "bad"}
	c := NewClient(cfg)

	err := c.SubmitResult(context.Background(), make([]byte, 32), "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb")
	if err == nil {
		t.Fatalf("expected ErrUnauthorized")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %T: %v", err, err)
	}
}

func TestSubmitResult_APIErrorWrapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "invalid private_key", "message": "private_key must be 64 hex characters"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: ""}
	c := NewClient(cfg)

	err := c.SubmitResult(context.Background(), make([]byte, 32), "0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb")
	if err == nil {
		t.Fatalf("expected wrapped API error")
	}
	if !strings.Contains(err.Error(), "result submission failed") {
		t.Fatalf("expected top-level error to mention result submission failed, got: %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected underlying APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400 inside APIError, got %d", apiErr.StatusCode)
	}
}
