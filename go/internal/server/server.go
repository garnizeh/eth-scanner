// Package server contains HTTP handlers and server bootstrap code for the Master API.
package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
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
	mu         sync.Mutex
	conns      map[net.Conn]struct{}
}

// New constructs a new Server instance. Routes must be registered with
// RegisterRoutes before calling Start.
func New(cfg *config.Config, db *sql.DB) *Server {
	mux := http.NewServeMux()
	s := &Server{
		cfg:    cfg,
		db:     db,
		router: mux,
		conns:  make(map[net.Conn]struct{}),
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

	// Track connections so we can force-close them if graceful shutdown
	// exceeds the configured timeout.
	s.httpServer.ConnState = func(c net.Conn, state http.ConnState) {
		s.mu.Lock()
		defer s.mu.Unlock()
		switch state {
		case http.StateNew, http.StateActive:
			s.conns[c] = struct{}{}
		case http.StateClosed, http.StateHijacked:
			delete(s.conns, c)
		case http.StateIdle:
			// keep in map until closed/hijacked
		}
	}

	// Ensure database is closed when server is shutting down
	s.httpServer.RegisterOnShutdown(func() {
		if s.db != nil {
			if err := s.db.Close(); err != nil {
				log.Printf("failed to close db on shutdown: %v", err)
			} else {
				log.Printf("database connection closed")
			}
		}
	})

	// Create listener first so we reliably know the server is bound before
	// returning from Start. Use ListenConfig.Listen with a context-aware
	// API to satisfy linters recommending context-aware listeners.
	lc := &net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("http serve: %w", err)
		} else {
			errCh <- nil
		}
	}()

	select {
	case <-ctx.Done():
		// Graceful shutdown with configurable timeout
		timeout := 30 * time.Second
		if s.cfg != nil && s.cfg.ShutdownTimeout > 0 {
			timeout = s.cfg.ShutdownTimeout
		}
		log.Printf("shutdown initiated, waiting up to %s for active connections to finish", timeout)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		// Small grace period to allow recently-started requests from clients
		// (who may have concurrent scheduling) to reach the server before we
		// trigger Shutdown. This reduces flakiness in tests that start a
		// request and immediately cancel the server context.
		time.Sleep(20 * time.Millisecond)
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			// If shutdown timed out, force-close active connections so
			// long-running handlers are aborted.
			if errors.Is(err, context.DeadlineExceeded) {
				log.Printf("shutdown timed out, force-closing active connections")
				s.mu.Lock()
				for c := range s.conns {
					_ = c.Close()
				}
				s.mu.Unlock()
			}
			return fmt.Errorf("server shutdown: %w", err)
		}

		// Ensure DB is closed before Start returns so callers/tests can rely on
		// the DB being shut down when Start exits.
		if s.db != nil {
			if err := s.db.Close(); err != nil {
				log.Printf("failed to close db on shutdown: %v", err)
			} else {
				log.Printf("database connection closed")
			}
		}

		log.Printf("shutdown complete")
		return fmt.Errorf("server shutdown: %w", ctx.Err())
	case err := <-errCh:
		return err
	}
}
