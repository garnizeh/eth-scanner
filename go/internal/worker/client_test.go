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
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
		if r.Header.Get("X-API-Key") != "" {
			t.Fatalf("expected no X-API-Key header when client has none")
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "bad", "message": "invalid input"})
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: ""}
	c := NewClient(cfg)

	err := c.doRequestWithContext(context.Background(), "POST", "/api/test", map[string]string{"x": "y"}, nil)
	if err == nil {
		t.Fatalf("expected error for 400 response")
	}
	if apiErr, ok := err.(*APIError); ok {
		if apiErr.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", apiErr.StatusCode)
		}
		if apiErr.Message != "invalid input" {
			t.Fatalf("unexpected api error message: %s", apiErr.Message)
		}
	} else {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
}

func TestDoRequestUnauthorizedReturnsErrUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized", "message": "missing api key"})
	}))
	defer srv.Close()

	cfg := &Config{APIURL: srv.URL, WorkerID: "w", APIKey: ""}
	c := NewClient(cfg)

	err := c.doRequestWithContext(context.Background(), "GET", "/api/test", nil, nil)
	if err == nil {
		t.Fatalf("expected error for 401 response")
	}
	if err != ErrUnauthorized {
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
