package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"
)

const conversationBudgetDayPrefix = "cost_budget_conv_day:"
const conversationBudgetMonthPrefix = "cost_budget_conv_month:"
const conversationCostCacheTTL = 30 * time.Second

type conversationCostEntry struct {
	at    time.Time
	day   float64
	month float64
}

var conversationCostCache struct {
	mu      sync.Mutex
	entries map[string]conversationCostEntry
}

func (s *Server) conversationBudget(convID string, month bool) float64 {
	if s.Store == nil || convID == "" {
		return 0
	}
	key := conversationBudgetDayPrefix
	if month {
		key = conversationBudgetMonthPrefix
	}
	raw, ok, err := s.Store.GetSetting(key + convID)
	if err != nil || !ok {
		return 0
	}
	var v float64
	if json.Unmarshal(raw, &v) != nil || v < 0 || v > 1_000_000 {
		return 0
	}
	return v
}

func (s *Server) conversationBudgets(convID string) (day, month float64) {
	return s.conversationBudget(convID, false), s.conversationBudget(convID, true)
}

func clearConversationCostCache() {
	conversationCostCache.mu.Lock()
	conversationCostCache.entries = nil
	conversationCostCache.mu.Unlock()
}

// conversationCostTotals sums this conversation's cached run costs for today
// and this month. It scans at most 500 runs in the month (documented limit).
func (s *Server) conversationCostTotals(convID string, now time.Time) (float64, float64, error) {
	if convID == "" || s.Store == nil {
		return 0, 0, nil
	}
	conversationCostCache.mu.Lock()
	if e, ok := conversationCostCache.entries[convID]; ok && now.Sub(e.at) < conversationCostCacheTTL {
		conversationCostCache.mu.Unlock()
		return e.day, e.month, nil
	}
	conversationCostCache.mu.Unlock()

	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	runs, err := s.Store.ListRunsCreatedBetween(monthStart, now, "", 500)
	if err != nil {
		return 0, 0, err
	}
	var day, month float64
	for _, rec := range runs {
		if rec.ConversationID != convID {
			continue
		}
		cost := s.runCostCached(rec)
		month += cost
		if !rec.CreatedAt.Before(dayStart) {
			day += cost
		}
	}
	day, month = round6(day), round6(month)
	conversationCostCache.mu.Lock()
	if conversationCostCache.entries == nil {
		conversationCostCache.entries = map[string]conversationCostEntry{}
	}
	conversationCostCache.entries[convID] = conversationCostEntry{at: now, day: day, month: month}
	conversationCostCache.mu.Unlock()
	return day, month, nil
}

// requireCostBudgetFor checks the global budget plus the optional agent and
// conversation budgets. The global enforce switch gates all of them.
func (s *Server) requireCostBudgetFor(agentID, convID string) error {
	if err := s.requireCostBudget(); err != nil {
		return err
	}
	if !s.costBudgetEnforced() {
		return nil
	}
	agentDay, agentMonth := s.agentBudgets(agentID)
	if agentID != "" && (agentDay > 0 || agentMonth > 0) {
		day, month, err := s.agentCostTotals(agentID, time.Now().UTC())
		if err == nil && ((agentDay > 0 && day >= agentDay) || (agentMonth > 0 && month >= agentMonth)) {
			return ErrCostBudgetExceeded
		}
	}
	convDay, convMonth := s.conversationBudgets(convID)
	if convID != "" && (convDay > 0 || convMonth > 0) {
		day, month, err := s.conversationCostTotals(convID, time.Now().UTC())
		if err == nil && ((convDay > 0 && day >= convDay) || (convMonth > 0 && month >= convMonth)) {
			return ErrCostBudgetExceeded
		}
	}
	return nil
}

func (s *Server) handleGetConversationCostBudget(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if err := s.checkConversationAccess(r.Context(), convID); err != nil {
		if errors.Is(err, errConversationForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	day, month := s.conversationBudgets(convID)
	writeJSON(w, http.StatusOK, map[string]any{
		"budget_usd_per_day":   day,
		"budget_usd_per_month": month,
	})
}

func (s *Server) handlePutConversationCostBudget(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if err := s.checkConversationAccess(r.Context(), convID); err != nil {
		if errors.Is(err, errConversationForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	var body struct {
		BudgetUSDPerDay   float64 `json:"budget_usd_per_day"`
		BudgetUSDPerMonth float64 `json:"budget_usd_per_month"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.BudgetUSDPerDay < 0 || body.BudgetUSDPerDay > 1_000_000 ||
		body.BudgetUSDPerMonth < 0 || body.BudgetUSDPerMonth > 1_000_000 {
		writeError(w, http.StatusBadRequest, "invalid_request", "budgets must be between 0 and 1000000")
		return
	}
	dayRaw, _ := json.Marshal(body.BudgetUSDPerDay)
	monthRaw, _ := json.Marshal(body.BudgetUSDPerMonth)
	if err := s.Store.UpsertSetting(conversationBudgetDayPrefix+convID, dayRaw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := s.Store.UpsertSetting(conversationBudgetMonthPrefix+convID, monthRaw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	clearConversationCostCache()
	writeJSON(w, http.StatusOK, map[string]any{
		"budget_usd_per_day":   body.BudgetUSDPerDay,
		"budget_usd_per_month": body.BudgetUSDPerMonth,
	})
}
