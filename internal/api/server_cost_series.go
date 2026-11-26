package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/conversation"
)

// handleGetCostSeries returns a per-day cost series for one conversation or
// workspace so the cost page can drill into a single scope.
func (s *Server) handleGetCostSeries(w http.ResponseWriter, r *http.Request) {
	days := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 90 {
			writeError(w, http.StatusBadRequest, "invalid_request", "days must be between 1 and 90")
			return
		}
		days = n
	}
	convID := strings.TrimSpace(r.URL.Query().Get("conversation_id"))
	wsID := strings.TrimSpace(r.URL.Query().Get("workspace_id"))
	if (convID == "") == (wsID == "") {
		writeError(w, http.StatusBadRequest, "invalid_request",
			"provide exactly one of conversation_id or workspace_id")
		return
	}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	start := today.AddDate(0, 0, -(days - 1))

	var convSet map[string]bool
	if wsID != "" {
		wsID = conversation.NormalizeWorkspaceID(wsID)
		var err error
		convSet, err = s.workspaceConversationIDs(wsID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}

	// One grouped query for the whole window instead of one query per day.
	perDay, err := s.Store.SumRunCostsPerDayGrouped(
		"conversation", start.Format("2006-01-02"), today.Format("2006-01-02"),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	series := make([]map[string]any, 0, days)
	total := 0.0
	for d := start; !d.After(today); d = d.AddDate(0, 0, 1) {
		byConv := perDay[d.Format("2006-01-02")]
		cost := 0.0
		if convID != "" {
			cost = byConv[convID]
		} else {
			for id := range convSet {
				cost += byConv[id]
			}
		}
		cost = round6(cost)
		total += cost
		series = append(series, map[string]any{
			"date":      d.Format("2006-01-02"),
			"total_usd": cost,
		})
	}

	// Today's active runs have no cached cost row yet; add their partial usage.
	if runs, err := s.Store.ListRunsCreatedBetweenForScope(
		today, now, "", "", scopePartialScanLimit,
	); err == nil {
		for _, rec := range runs {
			if isTerminalRunStatus(rec.Status) {
				continue
			}
			if convID != "" {
				if rec.ConversationID != convID {
					continue
				}
			} else if !convSet[rec.ConversationID] {
				continue
			}
			cost := s.runCostCached(rec)
			if cost == 0 {
				continue
			}
			total += cost
			last := series[len(series)-1]
			last["total_usd"] = round6(last["total_usd"].(float64) + cost)
		}
	}
	out := map[string]any{
		"scope":     "conversation",
		"days":      days,
		"total_usd": round6(total),
		"series":    series,
	}
	if convID != "" {
		out["conversation_id"] = convID
	} else {
		out["scope"] = "workspace"
		out["workspace_id"] = wsID
	}
	writeJSON(w, http.StatusOK, out)
}
