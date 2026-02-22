package server

import (
	"database/sql"
	"encoding/hex"
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
	prefixProgress, _ := q.GetPrefixProgress(ctx)

	tmpl := "index.html"
	data := map[string]any{
		"CurrentPath":         path,
		"ActiveWorkers":       activeWorkers,
		"PrefixProgress":      prefixProgress,
		"TotalWorkers":        stats.TotalWorkers,
		"ActiveWorkerCount":   stats.ActiveWorkers,
		"TotalKeysScanned":    stats.TotalKeysScanned,
		"CompletedJobCount":   stats.CompletedBatches,
		"ProcessingJobCount":  stats.ProcessingBatches,
		"GlobalKeysPerSecond": stats.GlobalKeysPerSecond,
		"NowTimestamp":        time.Now().UTC().Unix(),
	}

	switch {
	case path == "/dashboard/workers":
		tmpl = "workers.html"
		workerStats, _ := q.GetWorkerStats(ctx, 100)
		data["WorkerStats"] = workerStats
	case path == "/dashboard/settings":
		tmpl = "settings.html"
	case path == "/dashboard/daily":
		tmpl = "daily.html"
		workerID := r.URL.Query().Get("worker_id")
		sevenDaysAgo := time.Now().UTC().AddDate(0, 0, -6).Truncate(24 * time.Hour)
		sinceDate30 := time.Now().UTC().AddDate(0, 0, -30).Truncate(24 * time.Hour) // Look back 30 days to find 10 occurrences

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
				SinceDate: sinceDate30,
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
			dailyStats, err := q.GetGlobalDailyStats(ctx, sinceDate30)
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
			// Only sum for last 7 days for the summary cards
			if s.StatsDate >= sevenDaysAgo.Format("2006-01-02") {
				totalKeys += int64(s.TotalKeysScanned)
				totalDurationMs += int64(s.TotalDurationMs)
				totalBatches += int64(s.TotalBatches)
				totalErrors += int64(s.TotalErrors)
			}
		}

		type dailyPoint struct {
			Date   string
			Keys   int64
			Errors int64
		}
		var points []dailyPoint
		// Chart always shows 7 days (today back to 6 days ago)
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

		// Log shows last 10 entries that HAVE activity (keys or errors)
		var dailyLog []statsRow
		for _, s := range unifiedStats {
			if s.TotalKeysScanned > 0 || s.TotalErrors > 0 {
				dailyLog = append(dailyLog, s)
				if len(dailyLog) >= 10 {
					break
				}
			}
		}

		data["DailyStats"] = dailyLog
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
	case path == "/dashboard/monthly":
		tmpl = "monthly.html"
		workerID := r.URL.Query().Get("worker_id")
		sinceMonth := time.Now().UTC().AddDate(-1, 0, 0).Format("2006-01") // Last 12 months

		type monthlyRow struct {
			StatsMonth       string
			TotalBatches     sql.NullFloat64
			TotalKeysScanned sql.NullFloat64
			TotalDurationMs  sql.NullFloat64
			KeysPerSecondAvg sql.NullFloat64
			TotalErrors      sql.NullFloat64
		}
		var monthlyStats []monthlyRow

		if workerID != "" {
			rows, err := q.GetMonthlyStatsByWorker(ctx, database.GetMonthlyStatsByWorkerParams{
				WorkerID:   workerID,
				SinceMonth: sinceMonth,
			})
			if err != nil {
				// #nosec G105, G703, G706 -- workerID is from verified context
				log.Printf("UI: Error getting monthly stats for worker %s: %v", workerID, err)
			}
			for _, r := range rows {
				monthlyStats = append(monthlyStats, monthlyRow{
					StatsMonth:       r.StatsMonth,
					TotalBatches:     r.TotalBatches,
					TotalKeysScanned: r.TotalKeysScanned,
					TotalDurationMs:  r.TotalDurationMs,
					KeysPerSecondAvg: r.KeysPerSecondAvg,
					TotalErrors:      r.TotalErrors,
				})
			}

			// Get worker specific total keys
			w, err := q.GetWorkerByID(ctx, workerID)
			if err == nil {
				data["TotalKeysScanned"] = w.TotalKeysScanned.Int64
			}
		} else {
			rows, err := q.GetGlobalMonthlyStats(ctx, sinceMonth)
			if err != nil {
				log.Printf("UI: Error getting global monthly stats: %v", err)
			}
			for _, r := range rows {
				monthlyStats = append(monthlyStats, monthlyRow{
					StatsMonth:       r.StatsMonth,
					TotalBatches:     r.TotalBatches,
					TotalKeysScanned: r.TotalKeysScanned,
					TotalDurationMs:  r.TotalDurationMs,
					KeysPerSecondAvg: r.KeysPerSecondAvg,
					TotalErrors:      r.TotalErrors,
				})
			}

			// Global total keys
			stats, _ := q.GetStats(ctx)
			data["TotalKeysScanned"] = stats.TotalKeysScanned
		}

		// Filter for last 10 active months
		var monthlyLog []monthlyRow
		for _, s := range monthlyStats {
			if s.TotalKeysScanned.Float64 > 0 || s.TotalErrors.Float64 > 0 {
				monthlyLog = append(monthlyLog, s)
				if len(monthlyLog) >= 10 {
					break
				}
			}
		}

		data["MonthlyStats"] = monthlyLog
		data["WorkerID"] = workerID

		bestMonth, _ := q.GetBestMonthRecord(ctx)
		data["BestMonth"] = bestMonth

		type monthlyPoint struct {
			Month  string
			Keys   int64
			Errors int64
		}
		var points []monthlyPoint
		// Generate exactly 12 months for the chart (last 12 months)
		for i := 11; i >= 0; i-- {
			m := time.Now().UTC().AddDate(0, -i, 0).Format("2006-01")
			var val int64
			var errCount int64
			for _, s := range monthlyStats {
				if s.StatsMonth == m {
					val = int64(s.TotalKeysScanned.Float64)
					errCount = int64(s.TotalErrors.Float64)
					break
				}
			}
			points = append(points, monthlyPoint{
				Month:  m,
				Keys:   val,
				Errors: errCount,
			})
		}
		data["ChartPoints"] = points

		if r.Header.Get("HX-Request") == "true" {
			_ = s.renderer.RenderFragment(w, "monthly.html", "monthly-content", data)
			return
		}
	case path == "/dashboard/leaderboard":
		tmpl = "leaderboard.html"
		leaderboard, err := q.GetAllWorkerLifetimeStats(ctx)
		if err != nil {
			log.Printf("UI: Error getting leaderboard: %v", err)
		}
		data["Leaderboard"] = leaderboard
		data["TotalWorkers"] = len(leaderboard)

		// Calculate work distribution for pie chart (Top 5 + others)
		type distributionItem struct {
			Label string  `json:"label"`
			Value int64   `json:"value"`
			Color string  `json:"color"`
			Pct   float64 `json:"pct"`
		}
		var dist []distributionItem
		var totalAllTime int64
		for _, w := range leaderboard {
			totalAllTime += w.TotalKeysScanned
		}

		colors := []string{"#3b82f6", "#10b981", "#f59e0b", "#ef4444", "#8b5cf6", "#64748b"}

		//nolint:nestif
		if totalAllTime > 0 {
			for i, w := range leaderboard {
				if i < 5 {
					pct := (float64(w.TotalKeysScanned) / float64(totalAllTime)) * 100
					dist = append(dist, distributionItem{
						Label: w.WorkerID,
						Value: w.TotalKeysScanned,
						Color: colors[i],
						Pct:   pct,
					})
				} else {
					// Add to "Others"
					if len(dist) < 6 {
						dist = append(dist, distributionItem{
							Label: "Others",
							Value: 0,
							Color: colors[5],
						})
					}
					dist[5].Value += w.TotalKeysScanned
				}
			}
			if len(dist) == 6 {
				dist[5].Pct = (float64(dist[5].Value) / float64(totalAllTime)) * 100
			}
		}
		data["Distribution"] = dist

		bestDay, _ := q.GetBestDayRecord(ctx)
		data["BestDay"] = bestDay

		if r.Header.Get("HX-Request") == "true" {
			_ = s.renderer.RenderFragment(w, "leaderboard.html", "leaderboard-content", data)
			return
		}
	case strings.HasPrefix(path, "/dashboard/prefixes/"):
		prefixStr := strings.TrimPrefix(path, "/dashboard/prefixes/")
		prefixStr = strings.TrimPrefix(prefixStr, "0x")
		prefixBytes, err := hex.DecodeString(prefixStr)
		if err == nil {
			tmpl = "prefix_details.html"
			jobs, _ := q.GetJobsByPrefix(ctx, prefixBytes)
			data["Jobs"] = jobs
			data["TargetPrefix"] = "0x" + prefixStr

			if r.Header.Get("HX-Request") == "true" {
				_ = s.renderer.RenderFragment(w, "prefix_details.html", "prefix-content", data)
				return
			}
		} else {
			tmpl = "index.html"
		}
	}

	s.renderer.Handler(tmpl, data).ServeHTTP(w, r)
}
