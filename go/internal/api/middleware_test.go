package api

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
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
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

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
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(http.StatusOK) })

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

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/logme", nil)
	wrapped := Logger(h)
	wrapped.ServeHTTP(rr, req)

	out := buf.String()
	if !strings.Contains(out, "method=GET") {
		t.Fatalf("log output missing method: %q", out)
	}
	if !strings.Contains(out, "status=201") {
		t.Fatalf("log output missing status=201: %q", out)
	}
}
