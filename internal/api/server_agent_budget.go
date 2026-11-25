package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

const agentBudgetDayPrefix = "cost_budget_agent_day:"
const agentBudgetMonthPrefix = "cost_budget_agent_month:"
const agentBudgetWarnPrefix = "cost_budget_agent_warn:"
const agentBudgetExceedPrefix = "cost_budget_agent_exceed:"
const agentCostCacheTTL = 30 * time.Second
const scopePartialScanLimit = 200

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

// scopeRatio reads a 0..5 alert ratio override; 0 means inherit the global value.
func (s *Server) scopeRatio(key string) float64 {
	if s.Store == nil {
		return 0
	}
	raw, ok, err := s.Store.GetSetting(key)
	if err != nil || !ok {
		return 0
	}
	var v float64
	if json.Unmarshal(raw, &v) != nil || v < 0 || v > 5 {
		return 0
	}
	return v
}

func (s *Server) agentBudgetThresholds(agentID string) (warn, exceed float64) {
	return s.scopeRatio(agentBudgetWarnPrefix + agentID),
		s.scopeRatio(agentBudgetExceedPrefix + agentID)
}

func clearAgentCostCache() {
	agentCostCache.mu.Lock()
	agentCostCache.entries = nil
	agentCostCache.mu.Unlock()
}

// agentCostTotals sums terminal run costs via SQL and adds recent in-flight
// usage. Terminal costs are not capped; only the in-flight partial scan is.
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
	month, err := s.Store.SumRunCostsByScope(agentID, "", monthStart, now)
	if err != nil {
		return 0, 0, err
	}
	day, err := s.Store.SumRunCostsByScope(agentID, "", dayStart, now)
	if err != nil {
		return 0, 0, err
	}
	// In-flight runs have no cached cost row yet; add their partial usage.
	runs, err := s.Store.ListRunsCreatedBetweenForScope(monthStart, now, agentID, "", scopePartialScanLimit)
	if err != nil {
		return 0, 0, err
	}
	for _, rec := range runs {
		if rec.AgentID != agentID || isTerminalRunStatus(rec.Status) {
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

type agentBudgetJSON struct {
	AgentID       string  `json:"agent_id"`
	BudgetDay     float64 `json:"budget_usd_per_day"`
	BudgetMonth   float64 `json:"budget_usd_per_month"`
	WarningRatio  float64 `json:"warning_ratio"`
	ExceededRatio float64 `json:"exceeded_ratio"`
	SpentDay      float64 `json:"spent_usd_per_day"`
	SpentMonth    float64 `json:"spent_usd_per_month"`
}

func (s *Server) handleGetAgentCostBudgets(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	items := make([]agentBudgetJSON, 0)
	for _, ag := range s.Store.ListAgents() {
		day, month := s.agentBudgets(ag.ID)
		warn, exceed := s.agentBudgetThresholds(ag.ID)
		item := agentBudgetJSON{
			AgentID: ag.ID, BudgetDay: day, BudgetMonth: month,
			WarningRatio: warn, ExceededRatio: exceed,
		}
		if day > 0 || month > 0 {
			if spentDay, spentMonth, err := s.agentCostTotals(ag.ID, now); err == nil {
				item.SpentDay, item.SpentMonth = spentDay, spentMonth
			}
		}
		items = append(items, item)
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
	if !s.scopeThresholdsValid(body.WarningRatio, body.ExceededRatio) {
		writeError(w, http.StatusBadRequest, "invalid_request",
			"thresholds must be 0 (inherit) or in (0,5], with warning below exceeded")
		return
	}
	dayRaw, _ := json.Marshal(body.BudgetDay)
	monthRaw, _ := json.Marshal(body.BudgetMonth)
	warnRaw, _ := json.Marshal(body.WarningRatio)
	exceedRaw, _ := json.Marshal(body.ExceededRatio)
	if err := s.Store.UpsertSetting(agentBudgetDayPrefix+body.AgentID, dayRaw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := s.Store.UpsertSetting(agentBudgetMonthPrefix+body.AgentID, monthRaw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := s.Store.UpsertSetting(agentBudgetWarnPrefix+body.AgentID, warnRaw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := s.Store.UpsertSetting(agentBudgetExceedPrefix+body.AgentID, exceedRaw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	clearAgentCostCache()
	s.resetScopeAlertState("agent:" + body.AgentID)
	if body.BudgetDay > 0 || body.BudgetMonth > 0 {
		if spentDay, spentMonth, err := s.agentCostTotals(body.AgentID, time.Now().UTC()); err == nil {
			body.SpentDay, body.SpentMonth = spentDay, spentMonth
		}
	}
	writeJSON(w, http.StatusOK, body)
}
