package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

const agentBudgetDayPrefix = "cost_budget_agent_day:"
const agentBudgetMonthPrefix = "cost_budget_agent_month:"
const agentCostCacheTTL = 30 * time.Second

type agentCostEntry struct {
	at    time.Time
	day   float64
	month float64
}

var agentCostCache struct {
	mu      sync.Mutex
	entries map[string]agentCostEntry
}

func (s *Server) agentBudget(agentID string, month bool) float64 {
	if s.Store == nil || agentID == "" {
		return 0
	}
	key := agentBudgetDayPrefix
	if month {
		key = agentBudgetMonthPrefix
	}
	raw, ok, err := s.Store.GetSetting(key + agentID)
	if err != nil || !ok {
		return 0
	}
	var v float64
	if json.Unmarshal(raw, &v) != nil || v < 0 || v > 1_000_000 {
		return 0
	}
	return v
}

func (s *Server) agentBudgets(agentID string) (day, month float64) {
	return s.agentBudget(agentID, false), s.agentBudget(agentID, true)
}

func clearAgentCostCache() {
	agentCostCache.mu.Lock()
	agentCostCache.entries = nil
	agentCostCache.mu.Unlock()
}

// agentCostTotals sums this agent's cached run costs for today and this month.
// It scans at most 500 runs in the month (documented limit).
func (s *Server) agentCostTotals(agentID string, now time.Time) (float64, float64, error) {
	if agentID == "" || s.Store == nil {
		return 0, 0, nil
	}
	agentCostCache.mu.Lock()
	if e, ok := agentCostCache.entries[agentID]; ok && now.Sub(e.at) < agentCostCacheTTL {
		agentCostCache.mu.Unlock()
		return e.day, e.month, nil
	}
	agentCostCache.mu.Unlock()

	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	runs, err := s.Store.ListRunsCreatedBetween(monthStart, now, "", 500)
	if err != nil {
		return 0, 0, err
	}
	var day, month float64
	for _, rec := range runs {
		if rec.AgentID != agentID {
			continue
		}
		cost := s.runCostCached(rec)
		month += cost
		if !rec.CreatedAt.Before(dayStart) {
			day += cost
		}
	}
	day, month = round6(day), round6(month)
	agentCostCache.mu.Lock()
	if agentCostCache.entries == nil {
		agentCostCache.entries = map[string]agentCostEntry{}
	}
	agentCostCache.entries[agentID] = agentCostEntry{at: now, day: day, month: month}
	agentCostCache.mu.Unlock()
	return day, month, nil
}

// requireCostBudgetFor checks the global budget plus this agent's optional
// daily/monthly budgets. The global enforce switch gates all of them.
type agentBudgetJSON struct {
	AgentID     string  `json:"agent_id"`
	BudgetDay   float64 `json:"budget_usd_per_day"`
	BudgetMonth float64 `json:"budget_usd_per_month"`
}

func (s *Server) handleGetAgentCostBudgets(w http.ResponseWriter, r *http.Request) {
	items := make([]agentBudgetJSON, 0)
	for _, ag := range s.Store.ListAgents() {
		day, month := s.agentBudgets(ag.ID)
		items = append(items, agentBudgetJSON{AgentID: ag.ID, BudgetDay: day, BudgetMonth: month})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handlePutAgentCostBudget(w http.ResponseWriter, r *http.Request) {
	var body agentBudgetJSON
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.AgentID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "agent_id is required")
		return
	}
	if body.BudgetDay < 0 || body.BudgetDay > 1_000_000 ||
		body.BudgetMonth < 0 || body.BudgetMonth > 1_000_000 {
		writeError(w, http.StatusBadRequest, "invalid_request", "budgets must be between 0 and 1000000")
		return
	}
	dayRaw, _ := json.Marshal(body.BudgetDay)
	monthRaw, _ := json.Marshal(body.BudgetMonth)
	if err := s.Store.UpsertSetting(agentBudgetDayPrefix+body.AgentID, dayRaw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := s.Store.UpsertSetting(agentBudgetMonthPrefix+body.AgentID, monthRaw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	clearAgentCostCache()
	writeJSON(w, http.StatusOK, body)
}
