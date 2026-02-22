package server

import (
	"log"
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
	data := map[string]any{
		"CurrentPath":         path,
		"ActiveWorkers":       activeWorkers,
		"TotalWorkers":        stats.TotalWorkers,
		"ActiveWorkerCount":   stats.ActiveWorkers,
		"TotalKeysScanned":    stats.TotalKeysScanned,
		"CompletedJobCount":   stats.CompletedBatches,
		"ProcessingJobCount":  stats.ProcessingBatches,
		"GlobalKeysPerSecond": stats.GlobalKeysPerSecond,
		"NowTimestamp":        time.Now().UTC().Unix(),
	}

	switch path {
	case "/dashboard/workers":
		tmpl = "workers.html"
		workerStats, _ := q.GetWorkerStats(ctx, 100)
		data["WorkerStats"] = workerStats
	case "/dashboard/settings":
		tmpl = "settings.html"
	case "/dashboard/daily":
		tmpl = "daily.html"
		workerID := r.URL.Query().Get("worker_id")
		sevenDaysAgo := time.Now().UTC().AddDate(0, 0, -6).Truncate(24 * time.Hour)

		type statsRow struct {
			StatsDate        string
			TotalBatches     float64
			TotalKeysScanned float64
			TotalDurationMs  float64
			TotalErrors      float64
		}
		var unifiedStats []statsRow

		if workerID != "" {
			dailyStats, err := q.GetWorkerDailyStats(ctx, database.GetWorkerDailyStatsParams{
				WorkerID:  workerID,
				SinceDate: sevenDaysAgo,
			})
			if err != nil {
				log.Printf("UI: Error getting worker daily stats: %v", err)
			}
			for _, s := range dailyStats {
				unifiedStats = append(unifiedStats, statsRow{
					StatsDate:        s.StatsDate,
					TotalBatches:     s.TotalBatches.Float64,
					TotalKeysScanned: s.TotalKeysScanned.Float64,
					TotalDurationMs:  s.TotalDurationMs.Float64,
					TotalErrors:      s.TotalErrors.Float64,
				})
			}
			data["WorkerID"] = workerID
		} else {
			dailyStats, err := q.GetGlobalDailyStats(ctx, sevenDaysAgo)
			if err != nil {
				log.Printf("UI: Error getting global daily stats: %v", err)
			}
			for _, s := range dailyStats {
				unifiedStats = append(unifiedStats, statsRow{
					StatsDate:        s.StatsDate,
					TotalBatches:     s.TotalBatches.Float64,
					TotalKeysScanned: s.TotalKeysScanned.Float64,
					TotalDurationMs:  s.TotalDurationMs.Float64,
					TotalErrors:      s.TotalErrors.Float64,
				})
			}
		}

		var totalKeys, totalDurationMs, totalBatches, totalErrors int64
		for _, s := range unifiedStats {
			totalKeys += int64(s.TotalKeysScanned)
			totalDurationMs += int64(s.TotalDurationMs)
			totalBatches += int64(s.TotalBatches)
			totalErrors += int64(s.TotalErrors)
		}

		type dailyPoint struct {
			Date   string
			Keys   int64
			Errors int64
		}
		var points []dailyPoint
		for i := 6; i >= 0; i-- {
			d := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
			var val int64
			var errCount int64
			for _, s := range unifiedStats {
				if s.StatsDate == d {
					val = int64(s.TotalKeysScanned)
					errCount = int64(s.TotalErrors)
					break
				}
			}
			points = append(points, dailyPoint{Date: d, Keys: val, Errors: errCount})
		}

		data["DailyStats"] = unifiedStats
		data["TotalKeys7Days"] = totalKeys
		data["TotalBatches7Days"] = totalBatches
		data["TotalErrors7Days"] = totalErrors
		if totalDurationMs > 0 {
			data["AvgThroughput7Days"] = float64(totalKeys) / (float64(totalDurationMs) / 1000.0)
		} else {
			data["AvgThroughput7Days"] = 0.0
		}
		data["ChartPoints"] = points

		if r.Header.Get("HX-Request") == "true" {
			_ = s.renderer.RenderFragment(w, "daily.html", "daily-content", data)
			return
		}
	}

	s.renderer.Handler(tmpl, data).ServeHTTP(w, r)
}
