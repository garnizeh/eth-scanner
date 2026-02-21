package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/garnizeh/eth-scanner/internal/database"
)

// handleDashboard renders the main dashboard page.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	path := strings.TrimSuffix(r.URL.Path, "/")
	if path == "" {
		path = "/dashboard"
	}

	q := database.New(s.db)
	stats, _ := q.GetStats(ctx)
	activeWorkers, _ := q.GetActiveWorkerDetails(ctx)

	tmpl := "index.html"
	switch path {
	case "/dashboard/workers":
		tmpl = "workers.html"
	case "/dashboard/settings":
		tmpl = "settings.html"
	}

	data := map[string]any{
		"CurrentPath":         path,
		"ActiveWorkers":       activeWorkers,
		"TotalWorkers":        stats.TotalWorkers,
		"ActiveWorkerCount":   stats.ActiveWorkers,
		"TotalKeysScanned":    stats.TotalKeysScanned,
		"CompletedJobCount":   stats.CompletedBatches,
		"ProcessingJobCount":  stats.ProcessingBatches,
		"GlobalKeysPerSecond": stats.GlobalKeysPerSecond,
		"NowTimestamp":        time.Now().Unix(),
	}

	s.renderer.Handler(tmpl, data).ServeHTTP(w, r)
}
