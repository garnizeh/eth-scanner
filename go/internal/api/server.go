//nolint:revive
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/garnizeh/eth-scanner/internal/config"
)

// Server is the HTTP server for the Master API.
type Server struct {
	cfg        *config.Config
	db         *sql.DB
	router     *http.ServeMux
	httpServer *http.Server
}

// NewServer constructs a new Server instance. Routes must be registered with
// RegisterRoutes before calling Start.
func NewServer(cfg *config.Config, db *sql.DB) *Server {
	mux := http.NewServeMux()
	s := &Server{
		cfg:    cfg,
		db:     db,
		router: mux,
	}
	return s
}

// RegisterRoutes attaches handlers to the server router.
func (s *Server) RegisterRoutes() {
	// Health endpoint (simple implementation; can be replaced by P03-T050 handler)
	s.router.HandleFunc("/health", s.handleHealth)

	// Placeholder for API routes under /api/v1/ - return 501 for now
	s.router.HandleFunc("/api/v1/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Not Implemented", http.StatusNotImplemented)
	})
}

// Start runs the HTTP server and blocks until context cancellation or server error.
func (s *Server) Start(ctx context.Context) error {
	addr := ":" + s.cfg.Port
	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           s.router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("http listen: %w", err)
		} else {
			errCh <- nil
		}
	}()

	select {
	case <-ctx.Done():
		// Graceful shutdown with timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := s.httpServer.Shutdown(shutdownCtx)
		if err != nil {
			return fmt.Errorf("server shutdown: %w", err)
		}
		return fmt.Errorf("server shutdown: %w", ctx.Err())
	case err := <-errCh:
		return err
	}
}

// handleHealth is a minimal health check handler returning JSON status and UTC timestamp.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	type resp struct {
		Status    string `json:"status"`
		Timestamp string `json:"timestamp"`
	}

	w.Header().Set("Content-Type", "application/json")
	rj := resp{Status: "ok", Timestamp: time.Now().UTC().Format(time.RFC3339)}
	if err := json.NewEncoder(w).Encode(rj); err != nil {
		// best-effort; health endpoint should not fail the server
		_ = err
	}
}
