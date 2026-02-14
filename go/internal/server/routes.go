package server

import (
	"net/http"
)

// RegisterRoutes registers all HTTP routes and applies global middleware.
// This keeps route registration separate from server bootstrap.
func (s *Server) RegisterRoutes() {
	// Register handlers on the underlying ServeMux
	s.router.HandleFunc("/health", s.handleHealth)

	// API v1 routes (placeholders for now)
	// Specific endpoints where possible
	s.router.HandleFunc("/api/v1/jobs/lease", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Not Implemented", http.StatusNotImplemented)
	})

	// Generic api v1 base placeholder
	s.router.HandleFunc("/api/v1/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Not Implemented", http.StatusNotImplemented)
	})

	// Use prefix handlers for routes that include path parameters
	s.router.HandleFunc("/api/v1/jobs/", func(w http.ResponseWriter, _ *http.Request) {
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
