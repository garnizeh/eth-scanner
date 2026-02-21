package server

import (
	"net/http"
	"strings"

	"github.com/garnizeh/eth-scanner/internal/server/ui"
)

// RegisterRoutes registers all HTTP routes and applies global middleware.
// This keeps route registration separate from server bootstrap.
func (s *Server) RegisterRoutes() {
	// Register handlers on the underlying ServeMux
	s.router.HandleFunc("/health", s.handleHealth)

	// API v1 routes (placeholders for now)
	// Specific endpoints where possible
	s.router.HandleFunc("/api/v1/jobs/lease", s.handleJobLease)

	// Generic api v1 base placeholder
	s.router.HandleFunc("/api/v1/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Not Implemented", http.StatusNotImplemented)
	})

	// Use prefix handlers for routes that include path parameters
	s.router.HandleFunc("/api/v1/jobs/", func(w http.ResponseWriter, r *http.Request) {
		// Dispatch to specific handlers under /api/v1/jobs/
		// Support /api/v1/jobs/{id}/complete
		if strings.HasSuffix(r.URL.Path, "/complete") {
			if r.Method == http.MethodPost {
				s.handleJobComplete(w, r)
				return
			}
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Support /api/v1/jobs/{id}/checkpoint
		if strings.HasSuffix(r.URL.Path, "/checkpoint") {
			if r.Method == http.MethodPatch {
				s.handleJobCheckpoint(w, r)
				return
			}
			// Path exists but method is not allowed
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.Error(w, "Not Implemented", http.StatusNotImplemented)
	})

	s.router.HandleFunc("/api/v1/results", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			s.handleResultSubmit(w, r)
			return
		}
		http.Error(w, "Not Implemented", http.StatusNotImplemented)
	})

	s.router.HandleFunc("/api/v1/stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			s.handleStats(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	// UI Dashboard routes
	s.router.HandleFunc("/dashboard", s.handleDashboard)
	s.router.HandleFunc("/dashboard/", s.handleDashboard)

	// Static files serving from embedded FS
	s.router.Handle("/static/", http.FileServer(http.FS(ui.FS)))

	// Apply middleware chain in the required order: APIKey -> RequestID -> Logger -> CORS
	// The ServeMux implements http.Handler so we can wrap it. apiKeyMiddleware
	// is a method on Server so it can access configuration; when the API key
	// is not set the middleware is a no-op to preserve test behavior.
	s.handler = s.apiKeyMiddleware(RequestID(Logger(CORS(s.router))))
}
