package server

import (
	"net/http"
)

// handleDashboard renders the main dashboard page.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	// For now it's just a simple SSR page.
	// In the future P10-T020 will add auth middleware here or in RegisterRoutes.
	s.renderer.Handler("index.html", nil).ServeHTTP(w, r)
}
