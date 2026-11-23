package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const costBudgetSettingKey = "cost_budget_usd_per_day"

func (s *Server) handleGetCostSummary(w http.ResponseWriter, r *http.Request) {
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
	today := now.Format("2006-01-02")
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, -(days - 1))
	runs, err := s.Store.ListRunsCreatedBetween(start, now, "", 500)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	byDay := map[string]float64{}
	runCount := map[string]int{}
	for _, runRec := range runs {
		evs, err := s.Store.ListEvents(runRec.ID)
		if err != nil {
			continue
		}
		usage := s.usageWithCost(runRec, evs)
		cost, _ := usage["cost_usd"].(float64)
		day := runRec.CreatedAt.UTC().Format("2006-01-02")
		byDay[day] += cost
		runCount[day]++
	}
	outDays := make([]map[string]any, 0, days)
	for d := start; !d.After(now); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		outDays = append(outDays, map[string]any{
			"date":      key,
			"total_usd": round6(byDay[key]),
			"runs":      runCount[key],
		})
	}
	budget := s.costBudget()
	todayTotal := round6(byDay[today])
	writeJSON(w, http.StatusOK, map[string]any{
		"currency":           "USD",
		"days":               outDays,
		"today_total_usd":    todayTotal,
		"budget_usd_per_day": budget,
		"over_budget":        budget > 0 && todayTotal > budget,
	})
}

func (s *Server) handlePutCostBudget(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BudgetUSDPerDay float64 `json:"budget_usd_per_day"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.BudgetUSDPerDay < 0 || body.BudgetUSDPerDay > 1_000_000 {
		writeError(w, http.StatusBadRequest, "invalid_request", "budget_usd_per_day must be between 0 and 1000000")
		return
	}
	raw, _ := json.Marshal(body.BudgetUSDPerDay)
	if err := s.Store.UpsertSetting(costBudgetSettingKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"budget_usd_per_day": body.BudgetUSDPerDay})
}

func (s *Server) costBudget() float64 {
	if s.Store == nil {
		return 0
	}
	raw, ok, err := s.Store.GetSetting(costBudgetSettingKey)
	if err != nil || !ok {
		return 0
	}
	var v float64
	if json.Unmarshal(raw, &v) != nil || v < 0 {
		return 0
	}
	return v
}

func round6(v float64) float64 {
	return float64(int64(v*1e6+0.5)) / 1e6
}
