package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// handleGetCostExport streams the daily cost rollup as CSV so finance or
// offline tools can reconcile spend without scraping the dashboard.
func (s *Server) handleGetCostExport(w http.ResponseWriter, r *http.Request) {
	days := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 366 {
			writeError(w, http.StatusBadRequest, "invalid_request", "days must be between 1 and 366")
			return
		}
		days = n
	}
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, -(days - 1))
	byDay, runCount, err := s.costByDay(start, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	filename := fmt.Sprintf("baize-cost-%s-%s.csv", start.Format("20060102"), now.Format("20060102"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"date", "runs", "cost_usd"})
	totalCost := 0.0
	totalRuns := 0
	for d := start; !d.After(now); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		cost := round6(byDay[key])
		_ = cw.Write([]string{
			key,
			strconv.Itoa(runCount[key]),
			strconv.FormatFloat(cost, 'f', 6, 64),
		})
		totalCost += cost
		totalRuns += runCount[key]
	}
	_ = cw.Write([]string{
		"total",
		strconv.Itoa(totalRuns),
		strconv.FormatFloat(round6(totalCost), 'f', 6, 64),
	})
	cw.Flush()
}
