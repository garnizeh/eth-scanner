package server

import (
	"net/http"
	"strings"
)

// handleDashboard renders the main dashboard page.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	if path == "/dashboard" || path == "" {
		path = "/dashboard"
	}

	tmpl := "index.html"
	if path == "/dashboard/workers" {
		tmpl = "workers.html"
	} else if path == "/dashboard/settings" {
		tmpl = "settings.html"
	}

	data := map[string]any{
		"CurrentPath": path,
	}

	s.renderer.Handler(tmpl, data).ServeHTTP(w, r)
}
