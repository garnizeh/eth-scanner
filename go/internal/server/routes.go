package server

import (
	"net/http"
	"strings"
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

	s.router.HandleFunc("/api/v1/results", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Not Implemented", http.StatusNotImplemented)
	})

	s.router.HandleFunc("/api/v1/stats", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Not Implemented", http.StatusNotImplemented)
	})

	// Apply middleware chain in the required order: RequestID -> Logger -> CORS
	// The ServeMux implements http.Handler so we can wrap it.
	s.handler = RequestID(Logger(CORS(s.router)))
}
