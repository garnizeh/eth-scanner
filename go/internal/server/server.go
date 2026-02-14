// Package server contains HTTP handlers and server bootstrap code for the Master API.
package server

import (
	"context"
	"database/sql"
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
	handler    http.Handler
	httpServer *http.Server
}

// New constructs a new Server instance. Routes must be registered with
// RegisterRoutes before calling Start.
func New(cfg *config.Config, db *sql.DB) *Server {
	mux := http.NewServeMux()
	s := &Server{
		cfg:    cfg,
		db:     db,
		router: mux,
	}
	return s
}

// Start runs the HTTP server and blocks until context cancellation or server error.
func (s *Server) Start(ctx context.Context) error {
	addr := ":" + s.cfg.Port
	h := http.Handler(s.router)
	if s.handler != nil {
		h = s.handler
	}

	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           h,
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
