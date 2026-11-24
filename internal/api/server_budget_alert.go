package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/webhook"
)

const costAlertStateKey = "cost_alert_state"
const defaultBudgetAlertInterval = 5 * time.Minute

type budgetAlertState struct {
	DayKey     string `json:"day_key"`
	DayLevel   string `json:"day_level"`
	MonthKey   string `json:"month_key"`
	MonthLevel string `json:"month_level"`
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

// checkBudgetAlerts sends a webhook when today's or this month's spend newly
// reaches the warning/exceeded level. State is persisted so each period+level
// is announced once.
func (s *Server) checkBudgetAlerts(ctx context.Context) (int, error) {
	dayBudget := s.costBudget()
	monthBudget := s.costBudgetMonth()
	if (dayBudget <= 0 && monthBudget <= 0) || s.Store == nil {
		return 0, nil
	}
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	byDay, _, err := s.costByDay(monthStart, now)
	if err != nil {
		return 0, err
	}
	dayTotal := byDay[now.Format("2006-01-02")]
	var monthTotal float64
	for _, v := range byDay {
		monthTotal += v
	}
	dayLevel, dayRatio := budgetAlert(dayTotal, dayBudget)
	monthLevel, monthRatio := budgetAlert(monthTotal, monthBudget)
	dayKey := now.Format("2006-01-02")
	monthKey := now.Format("2006-01")

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
	notify := func(scope, level, period string, total, budget, ratio float64) {
		if s.Webhook == nil {
			return
		}
		err := s.Webhook.SendBudgetAlert(ctx, map[string]any{
			"scope": scope, "level": level, "period": period,
			"total_usd": round6(total), "budget_usd": budget, "ratio": round4(ratio),
		})
		if err != nil {
			if !errors.Is(err, webhook.ErrNoURL) {
				log.Printf("budget alert webhook (%s %s): %v", scope, level, err)
			}
			return
		}
		sent++
	}
	if alertRank(dayLevel) > alertRank(state.DayLevel) {
		notify("day", dayLevel, dayKey, dayTotal, dayBudget, dayRatio)
		state.DayLevel = dayLevel
	}
	if alertRank(monthLevel) > alertRank(state.MonthLevel) {
		notify("month", monthLevel, monthKey, monthTotal, monthBudget, monthRatio)
		state.MonthLevel = monthLevel
	}
	raw, _ := json.Marshal(state)
	_ = s.Store.UpsertSetting(costAlertStateKey, raw)
	return sent, nil
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
