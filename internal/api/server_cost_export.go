package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// handleGetCostExport streams cost data as CSV. group=day (default) exports the
// daily rollup; group=breakdown exports model, agent and conversation totals.
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
	group := strings.TrimSpace(r.URL.Query().Get("group"))
	if group == "" {
		group = "day"
	}
	if group != "day" && group != "breakdown" {
		writeError(w, http.StatusBadRequest, "invalid_request", "group must be day or breakdown")
		return
	}
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, -(days - 1))
	if group == "breakdown" {
		s.exportCostBreakdownCSV(w, start, now)
		return
	}
	s.exportDailyCostCSV(w, start, now)
}

func (s *Server) exportDailyCostCSV(w http.ResponseWriter, start, now time.Time) {
	byDay, runCount, err := s.costByDay(start, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	filename := fmt.Sprintf("baize-cost-%s-%s.csv", start.Format("20060102"), now.Format("20060102"))
	setCSVHeaders(w, filename)

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

func (s *Server) exportCostBreakdownCSV(w http.ResponseWriter, start, now time.Time) {
	groups, err := s.costGroupsBetween(start, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	modelNames, agentNames, convNames := s.costGroupNames()
	total := 0.0
	for _, v := range groups.model {
		total += v
	}
	filename := fmt.Sprintf("baize-cost-breakdown-%s-%s.csv", start.Format("20060102"), now.Format("20060102"))
	setCSVHeaders(w, filename)

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"group", "key", "name", "cost_usd", "ratio"})
	writeBuckets := func(group string, buckets []costBucketsJSON) {
		for _, b := range buckets {
			_ = cw.Write([]string{
				group,
				b.Key,
				b.Name,
				strconv.FormatFloat(b.CostUSD, 'f', 6, 64),
				strconv.FormatFloat(b.Ratio, 'f', 6, 64),
			})
		}
	}
	writeBuckets("model", buildCostBuckets(groups.model, modelNames, total))
	byProvider, providerNames := s.costByProvider(groups.model)
	writeBuckets("provider", buildCostBuckets(byProvider, providerNames, total))
	writeBuckets("agent", buildCostBuckets(groups.agent, agentNames, total))
	writeBuckets("conversation", buildCostBuckets(groups.conversation, convNames, total))
	byWorkspace, workspaceNames := s.costByWorkspace(groups.conversation)
	writeBuckets("workspace", buildCostBuckets(byWorkspace, workspaceNames, total))
	cw.Flush()
}

func setCSVHeaders(w http.ResponseWriter, filename string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
}
