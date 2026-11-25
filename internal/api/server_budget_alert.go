package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/webhook"
)

const costAlertStateKey = "cost_alert_state"
const defaultBudgetAlertInterval = 5 * time.Minute

type budgetAlertLevels struct {
	DayKey     string `json:"day_key"`
	DayLevel   string `json:"day_level"`
	MonthKey   string `json:"month_key"`
	MonthLevel string `json:"month_level"`
}

type budgetAlertState struct {
	DayKey     string                       `json:"day_key"`
	DayLevel   string                       `json:"day_level"`
	MonthKey   string                       `json:"month_key"`
	MonthLevel string                       `json:"month_level"`
	Scopes     map[string]budgetAlertLevels `json:"scopes,omitempty"`
}

var budgetAlertMu sync.Mutex

func alertRank(level string) int {
	switch level {
	case "exceeded":
		return 2
	case "warning":
		return 1
	}
	return 0
}

// checkBudgetAlerts sends a webhook when the global, an agent's, or a
// conversation's spend newly reaches the warning/exceeded level. State is
// persisted so each target+period+level is announced once.
func (s *Server) checkBudgetAlerts(ctx context.Context) (int, error) {
	if s.Store == nil {
		return 0, nil
	}
	now := time.Now().UTC()
	dayKey := now.Format("2006-01-02")
	monthKey := now.Format("2006-01")
	warnRatio, exceededRatio := s.costAlertThresholds()

	state := budgetAlertState{}
	if raw, ok, serr := s.Store.GetSetting(costAlertStateKey); serr == nil && ok {
		_ = json.Unmarshal(raw, &state)
	}
	if state.DayKey != dayKey {
		state.DayKey, state.DayLevel = dayKey, ""
	}
	if state.MonthKey != monthKey {
		state.MonthKey, state.MonthLevel = monthKey, ""
	}

	sent := 0
	notify := func(target, targetID, scope, period, level string, total, budget, ratio float64) {
		if s.Webhook == nil {
			return
		}
		data := map[string]any{
			"scope": scope, "level": level, "period": period,
			"total_usd": round6(total), "budget_usd": budget, "ratio": round4(ratio),
		}
		if target != "" {
			data["target"] = target
			data["target_id"] = targetID
		}
		err := s.Webhook.SendBudgetAlert(ctx, data)
		if err != nil {
			if !errors.Is(err, webhook.ErrNoURL) {
				log.Printf("budget alert webhook (%s %s %s): %v", target, targetID, level, err)
			}
			return
		}
		sent++
	}
	evaluate := func(st *budgetAlertLevels, target, targetID string, totals budgetAlertTotals, dayBudget, monthBudget, warn, exceed float64) {
		resetLevels(st, dayKey, monthKey)
		dayLevel, dayRatio := budgetAlert(totals.day, dayBudget, warn, exceed)
		monthLevel, monthRatio := budgetAlert(totals.month, monthBudget, warn, exceed)
		if alertRank(dayLevel) > alertRank(st.DayLevel) {
			notify(target, targetID, "day", dayKey, dayLevel, totals.day, dayBudget, dayRatio)
			st.DayLevel = dayLevel
		}
		if alertRank(monthLevel) > alertRank(st.MonthLevel) {
			notify(target, targetID, "month", monthKey, monthLevel, totals.month, monthBudget, monthRatio)
			st.MonthLevel = monthLevel
		}
	}

	dayBudget := s.costBudget()
	monthBudget := s.costBudgetMonth()
	if dayBudget > 0 || monthBudget > 0 {
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		byDay, _, err := s.costByDay(monthStart, now)
		if err != nil {
			return sent, err
		}
		dayTotal := byDay[dayKey]
		var monthTotal float64
		for _, v := range byDay {
			monthTotal += v
		}
		global := budgetAlertLevels{
			DayKey: state.DayKey, DayLevel: state.DayLevel,
			MonthKey: state.MonthKey, MonthLevel: state.MonthLevel,
		}
		evaluate(&global, "", "", budgetAlertTotals{day: dayTotal, month: monthTotal}, dayBudget, monthBudget, warnRatio, exceededRatio)
		state.DayKey, state.DayLevel = global.DayKey, global.DayLevel
		state.MonthKey, state.MonthLevel = global.MonthKey, global.MonthLevel
	}

	scopes := map[string]budgetAlertLevels{}
	for _, agentID := range s.configuredBudgetIDs(agentBudgetDayPrefix, agentBudgetMonthPrefix) {
		dayBudget, monthBudget := s.agentBudgets(agentID)
		if dayBudget <= 0 && monthBudget <= 0 {
			continue
		}
		totalsDay, totalsMonth, err := s.agentCostTotals(agentID, now)
		if err != nil {
			continue // fail open: one bad scope must not block others
		}
		key := "agent:" + agentID
		st := state.Scopes[key]
		aw, ae := s.agentBudgetThresholds(agentID)
		sw, se := mergeAlertThresholds(aw, ae, warnRatio, exceededRatio)
		evaluate(&st, "agent", agentID, budgetAlertTotals{day: totalsDay, month: totalsMonth}, dayBudget, monthBudget, sw, se)
		scopes[key] = st
	}
	for _, convID := range s.configuredBudgetIDs(conversationBudgetDayPrefix, conversationBudgetMonthPrefix) {
		dayBudget, monthBudget := s.conversationBudgets(convID)
		if dayBudget <= 0 && monthBudget <= 0 {
			continue
		}
		totalsDay, totalsMonth, err := s.conversationCostTotals(convID, now)
		if err != nil {
			continue
		}
		key := "conversation:" + convID
		st := state.Scopes[key]
		cw, ce := s.conversationBudgetThresholds(convID)
		sw, se := mergeAlertThresholds(cw, ce, warnRatio, exceededRatio)
		evaluate(&st, "conversation", convID, budgetAlertTotals{day: totalsDay, month: totalsMonth}, dayBudget, monthBudget, sw, se)
		scopes[key] = st
	}
	// Model budgets only alert; routing is not constrained by them yet.
	if modelIDs := s.configuredBudgetIDs(modelBudgetDayPrefix, modelBudgetMonthPrefix); len(modelIDs) > 0 {
		if modelDay, modelMonth, err := s.modelCostTotals(now); err == nil {
			for _, modelID := range modelIDs {
				dayBudget, monthBudget := s.modelBudgets(modelID)
				if dayBudget <= 0 && monthBudget <= 0 {
					continue
				}
				key := "model:" + modelID
				st := state.Scopes[key]
				mw, me := s.modelBudgetThresholds(modelID)
				sw, se := mergeAlertThresholds(mw, me, warnRatio, exceededRatio)
				evaluate(&st, "model", modelID, budgetAlertTotals{
					day: modelDay[modelID], month: modelMonth[modelID],
				}, dayBudget, monthBudget, sw, se)
				scopes[key] = st
			}
		}
	}
	for _, wsID := range s.configuredBudgetIDs(workspaceBudgetDayPrefix, workspaceBudgetMonthPrefix) {
		dayBudget, monthBudget := s.workspaceBudgets(wsID)
		if dayBudget <= 0 && monthBudget <= 0 {
			continue
		}
		totalsDay, totalsMonth, err := s.workspaceCostTotals(wsID, now)
		if err != nil {
			continue
		}
		key := "workspace:" + wsID
		st := state.Scopes[key]
		ww, we := s.workspaceBudgetThresholds(wsID)
		sw, se := mergeAlertThresholds(ww, we, warnRatio, exceededRatio)
		evaluate(&st, "workspace", wsID, budgetAlertTotals{day: totalsDay, month: totalsMonth}, dayBudget, monthBudget, sw, se)
		scopes[key] = st
	}
	state.Scopes = scopes

	raw, _ := json.Marshal(state)
	_ = s.Store.UpsertSetting(costAlertStateKey, raw)
	return sent, nil
}

type budgetAlertTotals struct {
	day   float64
	month float64
}

// mergeAlertThresholds applies per-scope overrides on top of the global pair.
// 0 means inherit; an invalid merged pair falls back to the globals.
func mergeAlertThresholds(warn, exceed, globalWarn, globalExceed float64) (float64, float64) {
	if warn <= 0 {
		warn = globalWarn
	}
	if exceed <= 0 {
		exceed = globalExceed
	}
	if warn >= exceed {
		return globalWarn, globalExceed
	}
	return warn, exceed
}

// scopeThresholdsValid accepts 0 (inherit) or (0,5], and requires the merged
// warning ratio to stay below the merged exceeded ratio.
func (s *Server) scopeThresholdsValid(warn, exceed float64) bool {
	if warn < 0 || warn > 5 || exceed < 0 || exceed > 5 {
		return false
	}
	globalWarn, globalExceed := s.costAlertThresholds()
	if warn == 0 {
		warn = globalWarn
	}
	if exceed == 0 {
		exceed = globalExceed
	}
	return warn < exceed
}

// resetScopeAlertState drops one target's dedupe levels so a threshold change
// can alert again in the current period.
func (s *Server) resetScopeAlertState(scopeKey string) {
	if s.Store == nil {
		return
	}
	raw, ok, err := s.Store.GetSetting(costAlertStateKey)
	if err != nil || !ok {
		return
	}
	var state budgetAlertState
	if json.Unmarshal(raw, &state) != nil || state.Scopes == nil {
		return
	}
	delete(state.Scopes, scopeKey)
	out, _ := json.Marshal(state)
	_ = s.Store.UpsertSetting(costAlertStateKey, out)
}

func resetLevels(st *budgetAlertLevels, dayKey, monthKey string) {
	if st.DayKey != dayKey {
		st.DayKey, st.DayLevel = dayKey, ""
	}
	if st.MonthKey != monthKey {
		st.MonthKey, st.MonthLevel = monthKey, ""
	}
}

// configuredBudgetIDs returns ids that have a day or month budget setting.
func (s *Server) configuredBudgetIDs(dayPrefix, monthPrefix string) []string {
	seen := map[string]bool{}
	add := func(m map[string][]byte, prefix string) {
		for key := range m {
			id := strings.TrimPrefix(key, prefix)
			if id != "" {
				seen[id] = true
			}
		}
	}
	if m, err := s.Store.ListSettingsByPrefix(dayPrefix); err == nil {
		add(m, dayPrefix)
	}
	if m, err := s.Store.ListSettingsByPrefix(monthPrefix); err == nil {
		add(m, monthPrefix)
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}

// StartBudgetAlerts checks immediately and then every interval until ctx ends.
func (s *Server) StartBudgetAlerts(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultBudgetAlertInterval
	}
	run := func() {
		if !budgetAlertMu.TryLock() {
			return
		}
		defer budgetAlertMu.Unlock()
		_, _ = s.checkBudgetAlerts(ctx)
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
