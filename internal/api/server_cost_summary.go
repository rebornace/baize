package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/store"
)

const costBudgetSettingKey = "cost_budget_usd_per_day"
const costBudgetEnforceKey = "cost_budget_enforce"
const costSummaryCacheTTL = 30 * time.Second

var (
	ErrCostBudgetExceeded = errors.New("cost budget exceeded")
	costCache             struct {
		mu    sync.Mutex
		at    time.Time
		today float64
	}
)

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
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, -(days - 1))
	byDay, runCount, err := s.costByDay(start, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
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
	todayTotal := round6(byDay[now.Format("2006-01-02")])
	writeJSON(w, http.StatusOK, map[string]any{
		"currency":           "USD",
		"days":               outDays,
		"today_total_usd":    todayTotal,
		"budget_usd_per_day": budget,
		"budget_enforce":     s.costBudgetEnforced(),
		"over_budget":        budget > 0 && todayTotal > budget,
	})
}

// costRollupDayLimit bounds how many runs a single day rollup will read. When
// a day hits the cap it is never marked final, so the total stays conservative
// instead of silently truncating.
const costRollupDayLimit = 500

// costByDay returns per-day cost totals and run counts for [start, now].
// Finished days come from the cost_days rollup in one query; today (and days
// that still have a running job) are computed from runs and then finalized
// once they can no longer change.
func (s *Server) costByDay(start, now time.Time) (map[string]float64, map[string]int, error) {
	startDay := start.UTC().Format("2006-01-02")
	todayDay := now.UTC().Format("2006-01-02")
	rolled, err := s.Store.ListCostDays(startDay, todayDay)
	if err != nil {
		return nil, nil, err
	}
	finished := make(map[string]store.CostDay, len(rolled))
	for _, d := range rolled {
		if d.Finalized {
			finished[d.Day] = d
		}
	}
	byDay := map[string]float64{}
	runCount := map[string]int{}
	for d := start.UTC(); !d.After(now); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		if c, ok := finished[key]; ok {
			byDay[key] = c.CostUSD
			runCount[key] = c.Runs
			continue
		}
		total, n, final, err := s.rollupDay(d)
		if err != nil {
			return nil, nil, err
		}
		byDay[key] = total
		runCount[key] = n
		if final {
			_ = s.Store.PutCostDay(store.CostDay{Day: key, CostUSD: total, Runs: n, Finalized: true})
		}
	}
	return byDay, runCount, nil
}

// rollupDay sums one UTC day's run costs. A day is final only when every run
// created that day is terminal and the day is entirely in the past.
func (s *Server) rollupDay(dayStart time.Time) (float64, int, bool, error) {
	dayStart = dayStart.UTC().Truncate(24 * time.Hour)
	dayEnd := dayStart.AddDate(0, 0, 1).Add(-time.Nanosecond)
	runs, err := s.Store.ListRunsCreatedBetween(dayStart, dayEnd, "", costRollupDayLimit)
	if err != nil {
		return 0, 0, false, err
	}
	var total float64
	pending := false
	for _, runRec := range runs {
		total += s.runCostCached(runRec)
		if !isTerminalRunStatus(runRec.Status) {
			pending = true
		}
	}
	final := !pending && len(runs) < costRollupDayLimit && dayEnd.Before(time.Now().UTC())
	return round6(total), len(runs), final, nil
}

func isTerminalRunStatus(st store.Status) bool {
	switch st {
	case store.StatusSucceeded, store.StatusFailed, store.StatusCancelled, store.StatusRejected:
		return true
	}
	return false
}

// todayCostUSD sums today's estimated cost. A short cache keeps budget
// enforcement from rescanning on every new run.
func (s *Server) todayCostUSD() float64 {
	now := time.Now().UTC()
	costCache.mu.Lock()
	defer costCache.mu.Unlock()
	if !costCache.at.IsZero() && now.Sub(costCache.at) < costSummaryCacheTTL {
		return costCache.today
	}
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	total, _, _, err := s.rollupDay(start)
	if err != nil {
		return costCache.today
	}
	costCache.at = now
	costCache.today = total
	return total
}

func (s *Server) requireCostBudget() error {
	budget := s.costBudget()
	if budget <= 0 || !s.costBudgetEnforced() {
		return nil
	}
	if s.todayCostUSD() >= budget {
		return ErrCostBudgetExceeded
	}
	return nil
}

// runCostCached returns a run's estimated cost, memoizing terminal runs so
// summaries and budget checks stop rescanning their events.
func (s *Server) runCostCached(runRec *store.Run) float64 {
	day := runRec.CreatedAt.UTC().Format("2006-01-02")
	terminal := isTerminalRunStatus(runRec.Status)
	if terminal {
		if c, ok, err := s.Store.GetRunCost(runRec.ID); err == nil && ok && c.Day == day {
			if c.Priced {
				return c.CostUSD
			}
			return 0
		}
	}
	evs, err := s.Store.ListEvents(runRec.ID)
	if err != nil {
		return 0
	}
	usage := s.usageWithCost(runRec, evs)
	cost, _ := usage["cost_usd"].(float64)
	priced, _ := usage["priced"].(bool)
	if terminal {
		_ = s.Store.PutRunCost(store.RunCost{
			RunID: runRec.ID, Day: day, CostUSD: cost, Priced: priced,
		})
	}
	return cost
}

func (s *Server) costBudgetEnforced() bool {
	if s.Store == nil {
		return false
	}
	raw, ok, err := s.Store.GetSetting(costBudgetEnforceKey)
	if err != nil || !ok {
		return false
	}
	var v bool
	return json.Unmarshal(raw, &v) == nil && v
}

func (s *Server) handlePutCostBudget(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BudgetUSDPerDay float64 `json:"budget_usd_per_day"`
		Enforce         bool    `json:"enforce"`
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
	enforceRaw, _ := json.Marshal(body.Enforce)
	if err := s.Store.UpsertSetting(costBudgetEnforceKey, enforceRaw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	costCache.mu.Lock()
	costCache.at = time.Time{}
	costCache.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"budget_usd_per_day": body.BudgetUSDPerDay,
		"enforce":            body.Enforce,
	})
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
