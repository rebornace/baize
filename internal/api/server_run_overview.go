package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/store"
)

const runOverviewLimit = 500
const runOverviewTopErrors = 5

// handleGetRunOverview returns a small, storage-backed activity summary for
// admins (no Prometheus dependency).
func (s *Server) handleGetRunOverview(w http.ResponseWriter, r *http.Request) {
	days := 7
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 31 {
			writeError(w, http.StatusBadRequest, "invalid_request", "days must be between 1 and 31")
			return
		}
		days = n
	}
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, -(days - 1))
	runs, err := s.Store.ListRunsCreatedBetween(start, now, "", runOverviewLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	byStatus := map[string]int{
		string(store.StatusQueued):       0,
		string(store.StatusRunning):      0,
		string(store.StatusWaitingHuman): 0,
		string(store.StatusSucceeded):    0,
		string(store.StatusFailed):       0,
		string(store.StatusCancelled):    0,
		string(store.StatusRejected):     0,
	}
	terminal := 0
	active := 0
	waiting := 0
	errorsByMessage := map[string]int{}
	for _, runRec := range runs {
		status := string(runRec.Status)
		byStatus[status]++
		switch runRec.Status {
		case store.StatusSucceeded:
			terminal++
		case store.StatusFailed, store.StatusCancelled, store.StatusRejected:
			terminal++
		case store.StatusQueued, store.StatusRunning:
			active++
		case store.StatusWaitingHuman:
			active++
			waiting++
		}
		if runRec.Status == store.StatusFailed || runRec.Status == store.StatusRejected {
			if msg := firstErrorLine(runRec.Error); msg != "" {
				errorsByMessage[msg]++
			}
		}
	}
	successRate := 0.0
	if terminal > 0 {
		successRate = round4(float64(byStatus[string(store.StatusSucceeded)]) / float64(terminal))
	}
	type errorCount struct {
		Message string `json:"message"`
		Count   int    `json:"count"`
	}
	topErrors := make([]errorCount, 0, len(errorsByMessage))
	for msg, count := range errorsByMessage {
		topErrors = append(topErrors, errorCount{Message: msg, Count: count})
	}
	sort.Slice(topErrors, func(i, j int) bool {
		if topErrors[i].Count == topErrors[j].Count {
			return topErrors[i].Message < topErrors[j].Message
		}
		return topErrors[i].Count > topErrors[j].Count
	})
	if len(topErrors) > runOverviewTopErrors {
		topErrors = topErrors[:runOverviewTopErrors]
	}

	byDay, _, cerr := s.costByDay(start, now)
	if cerr != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", cerr.Error())
		return
	}
	var rangeCost float64
	for _, v := range byDay {
		rangeCost += v
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days":           days,
		"total":          len(runs),
		"by_status":      byStatus,
		"terminal":       terminal,
		"success_rate":   successRate,
		"active":         active,
		"waiting_human":  waiting,
		"range_cost_usd": round6(rangeCost),
		"today_cost_usd": round6(byDay[now.Format("2006-01-02")]),
		"top_errors":     topErrors,
		"truncated":      len(runs) >= runOverviewLimit,
	})
}

func firstErrorLine(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	line := raw
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimSpace(line)
	if len(line) > 160 {
		line = strings.TrimSpace(line[:160]) + "…"
	}
	return line
}
