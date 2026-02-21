package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/garnizeh/eth-scanner/internal/config"
	"github.com/garnizeh/eth-scanner/internal/database"
)

func TestHandleHealth_File(t *testing.T) {
	t.Run("no db configured", func(t *testing.T) {
		s, err := New(&config.Config{}, nil)
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/health", nil)

		s.handleHealth(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rr.Code)
		}

		var body struct {
			Status    string `json:"status"`
			Timestamp string `json:"timestamp"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode health response: %v", err)
		}
		if body.Status != "ok" {
			t.Fatalf("unexpected status: %q", body.Status)
		}
		ts, err := time.Parse(time.RFC3339, body.Timestamp)
		if err != nil {
			t.Fatalf("timestamp not RFC3339: %v", err)
		}
		if ts.Location() != time.UTC {
			t.Fatalf("timestamp not in UTC: %v (loc=%v)", ts, ts.Location())
		}
	})

	t.Run("db connected", func(t *testing.T) {
		ctx := context.Background()
		db, err := database.InitDB(ctx, ":memory:")
		if err != nil {
			t.Fatalf("failed to init in-memory database: %v", err)
		}
		defer db.Close()

		s, err := New(&config.Config{}, db)
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/health", nil)

		s.handleHealth(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rr.Code)
		}

		var body struct {
			Status    string `json:"status"`
			Timestamp string `json:"timestamp"`
			Database  string `json:"database"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode health response: %v", err)
		}
		if body.Status != "ok" {
			t.Fatalf("unexpected status: %q", body.Status)
		}
		if body.Database != "connected" {
			t.Fatalf("expected database connected, got %q", body.Database)
		}
	})

	t.Run("db disconnected", func(t *testing.T) {
		ctx := context.Background()
		db, err := database.InitDB(ctx, ":memory:")
		if err != nil {
			t.Fatalf("failed to init in-memory database: %v", err)
		}
		// Close to simulate an unavailable DB for PingContext
		if err := db.Close(); err != nil {
			t.Fatalf("failed to close db: %v", err)
		}

		s, err := New(&config.Config{}, db)
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/health", nil)

		s.handleHealth(rr, req)

		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected status 503, got %d", rr.Code)
		}

		var body struct {
			Status    string `json:"status"`
			Timestamp string `json:"timestamp"`
			Database  string `json:"database"`
			Error     string `json:"error"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode health response: %v", err)
		}
		if body.Status != "error" {
			t.Fatalf("unexpected status: %q", body.Status)
		}
		if body.Database != "disconnected" {
			t.Fatalf("expected database disconnected, got %q", body.Database)
		}
		if body.Error == "" {
			t.Fatalf("expected error message, got empty")
		}
	})
}
