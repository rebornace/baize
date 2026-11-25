package api

import (
	"net/http"
	"sort"
	"strconv"
	"time"
)

type costBucketsJSON struct {
	Key     string  `json:"key"`
	Name    string  `json:"name"`
	CostUSD float64 `json:"cost_usd"`
	Ratio   float64 `json:"ratio"`
}

// handleGetCostBreakdown returns cached terminal costs grouped by model and by
// agent for the last N days, newest range first.
func (s *Server) handleGetCostBreakdown(w http.ResponseWriter, r *http.Request) {
	days := 30
	if raw := r.URL.Query().Get("days"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			days = n
		}
	}
	if days < 1 {
		days = 1
	}
	if days > 365 {
		days = 365
	}
	now := time.Now().UTC()
	from := now.AddDate(0, 0, -days)
	byModel, err := s.Store.SumRunCostsGrouped("model", from, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	byAgent, err := s.Store.SumRunCostsGrouped("agent", from, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	modelNames := map[string]string{}
	if profiles, err := s.Store.ListModelProfiles(); err == nil {
		for _, p := range profiles {
			modelNames[p.ID] = p.Name
		}
	}
	agentNames := map[string]string{}
	for _, ag := range s.Store.ListAgents() {
		agentNames[ag.ID] = ag.ID
	}
	total := 0.0
	for _, v := range byModel {
		total += v
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days":      days,
		"from":      from.Format(time.RFC3339),
		"to":        now.Format(time.RFC3339),
		"total_usd": round6(total),
		"by_model":  buildCostBuckets(byModel, modelNames, total),
		"by_agent":  buildCostBuckets(byAgent, agentNames, total),
	})
}

func buildCostBuckets(sums map[string]float64, names map[string]string, total float64) []costBucketsJSON {
	items := make([]costBucketsJSON, 0, len(sums))
	for key, cost := range sums {
		ratio := 0.0
		if total > 0 {
			ratio = cost / total
		}
		name := names[key]
		items = append(items, costBucketsJSON{
			Key: key, Name: name, CostUSD: round6(cost), Ratio: round4(ratio),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CostUSD == items[j].CostUSD {
			return items[i].Key < items[j].Key
		}
		return items[i].CostUSD > items[j].CostUSD
	})
	return items
}
