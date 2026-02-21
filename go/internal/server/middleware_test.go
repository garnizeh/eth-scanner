package server

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/garnizeh/eth-scanner/internal/config"
	"github.com/garnizeh/eth-scanner/internal/database"
)

func TestRequestIDMiddleware(t *testing.T) {
	var captured string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = GetRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(captured))
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/foo", nil)
	wrapped := RequestID(h)
	wrapped.ServeHTTP(rr, req)

	gotHeader := rr.Header().Get("X-Request-ID")
	if gotHeader == "" {
		t.Fatalf("missing X-Request-ID header")
	}
	if captured != gotHeader {
		t.Fatalf("request id in context and header differ: ctx=%q header=%q", captured, gotHeader)
	}
	// ensure returned id looks like a 16-byte hex (32 chars) or a timestamp fallback
	if len(gotHeader) != 32 && !strings.Contains(gotHeader, "T") {
		t.Fatalf("unexpected request id format: %q", gotHeader)
	}
}

func TestCORSPreflight(t *testing.T) {
	called := false
	h := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/bar", nil)
	wrapped := CORS(h)
	wrapped.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204 for preflight, got %d", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected Access-Control-Allow-Origin=* got=%q", got)
	}
	if called {
		t.Fatalf("handler should not be called for preflight OPTIONS")
	}
}

func TestCORSNormal(t *testing.T) {
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(http.StatusOK) })

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/baz", nil)
	wrapped := CORS(h)
	wrapped.ServeHTTP(rr, req)

	if !called {
		t.Fatalf("handler was not called for normal request")
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected Access-Control-Allow-Origin=* got=%q", got)
	}
}

func TestLoggerMiddleware(t *testing.T) {
	var buf bytes.Buffer
	// capture logs
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/logme", nil)
	wrapped := Logger(h)
	wrapped.ServeHTTP(rr, req)

	out := buf.String()
	if !strings.Contains(out, "method=\"GET\"") {
		t.Fatalf("log output missing method: %q", out)
	}
	if !strings.Contains(out, "status=201") {
		t.Fatalf("log output missing status=201: %q", out)
	}
}

// helper to create a server with a temporary DB and provided config
func newServerWithCfg(t *testing.T, cfg *config.Config) *Server {
	t.Helper()
	ctx := t.Context()
	dbPath := t.TempDir() + "/test.db"
	db, err := database.InitDB(ctx, dbPath)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	s, err := New(cfg, db)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	s.RegisterRoutes()
	return s
}

func TestAPIKeyMiddleware_NoConfig_Allows(t *testing.T) {
	cfg := &config.Config{Port: "0", DBPath: ":memory:", APIKey: ""}
	s := newServerWithCfg(t, cfg)

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/health", nil)
	cli := &http.Client{}
	//nolint:gosec // false positive: SSRF in test
	resp, err := cli.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK when API key not configured, got %d", resp.StatusCode)
	}
}

func TestAPIKeyMiddleware_RejectsMissingOrInvalid(t *testing.T) {
	cfg := &config.Config{Port: "0", DBPath: ":memory:", APIKey: "secret"}
	s := newServerWithCfg(t, cfg)

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	// missing header
	req1, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/api/v1/jobs/lease", nil)
	cli := &http.Client{}
	//nolint:gosec // false positive: SSRF in test
	resp, err := cli.Do(req1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for missing key, got %d", resp.StatusCode)
	}

	// invalid header
	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/api/v1/jobs/lease", nil)
	req2.Header.Set("X-API-KEY", "wrong")
	//nolint:gosec // false positive: SSRF in test
	resp2, err := cli.Do(req2)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for invalid key, got %d", resp2.StatusCode)
	}
}

func TestAPIKeyMiddleware_AllowsValid(t *testing.T) {
	cfg := &config.Config{Port: "0", DBPath: ":memory:", APIKey: "secret"}
	s := newServerWithCfg(t, cfg)

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/health", nil)
	req.Header.Set("X-API-KEY", "secret")
	cli := &http.Client{}
	//nolint:gosec // false positive: SSRF in test
	resp, err := cli.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for valid key, got %d", resp.StatusCode)
	}
}

func TestAPIKeyMiddleware_AllowsOptions(t *testing.T) {
	// Ensure that when an API key is configured, preflight OPTIONS requests
	// are still allowed through (apiKeyMiddleware should call next.ServeHTTP and return).
	cfg := &config.Config{Port: "0", DBPath: ":memory:", APIKey: "secret"}
	s := newServerWithCfg(t, cfg)

	ts := httptest.NewServer(s.handler)
	defer ts.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodOptions, ts.URL+"/health", nil)
	cli := &http.Client{}
	//nolint:gosec // false positive: SSRF in test
	resp, err := cli.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for OPTIONS preflight, got %d", resp.StatusCode)
	}
}
