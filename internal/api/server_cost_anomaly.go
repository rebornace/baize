package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/rebornace/baize/internal/webhook"
)

const costAnomalyStateKey = "cost_anomaly_state"
const costAnomalyHistoryKey = "cost_anomaly_history"
const costAnomalyHistoryLimit = 200
const costAnomalyDays = 7
const costAnomalyMultiplier = 2.0
const costAnomalyMinBaselineUSD = 0.50
const costAnomalyMinTodayUSD = 1.00
const costAnomalyMaxConversations = 5

type costAnomalyState struct {
	DayKey        string          `json:"day_key"`
	Global        bool            `json:"global,omitempty"`
	Conversations map[string]bool `json:"conversations,omitempty"`
}

type costAnomalyHistoryEntry struct {
	At             time.Time `json:"at"`
	Scope          string    `json:"scope"`
	ConversationID string    `json:"conversation_id,omitempty"`
	TodayUSD       float64   `json:"today_usd"`
	BaselineUSD    float64   `json:"baseline_usd"`
	Ratio          float64   `json:"ratio"`
}

// recordCostAnomaly appends one anomaly to the capped in-app history, even when
// no webhook is configured.
func (s *Server) recordCostAnomaly(entry costAnomalyHistoryEntry) {
	if s.Store == nil {
		return
	}
	var list []costAnomalyHistoryEntry
	if raw, ok, err := s.Store.GetSetting(costAnomalyHistoryKey); err == nil && ok {
		_ = json.Unmarshal(raw, &list)
	}
	list = append(list, entry)
	if len(list) > costAnomalyHistoryLimit {
		list = list[len(list)-costAnomalyHistoryLimit:]
	}
	raw, _ := json.Marshal(list)
	_ = s.Store.UpsertSetting(costAnomalyHistoryKey, raw)
}

func (s *Server) handleGetCostAnomalies(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > costAnomalyHistoryLimit {
		limit = costAnomalyHistoryLimit
	}
	var list []costAnomalyHistoryEntry
	if raw, ok, err := s.Store.GetSetting(costAnomalyHistoryKey); err == nil && ok {
		_ = json.Unmarshal(raw, &list)
	}
	out := make([]costAnomalyHistoryEntry, 0, limit)
	for i := len(list) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, list[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "limit": limit})
}

func costAnomalyTripped(baselineAvg, today float64) bool {
	if baselineAvg < costAnomalyMinBaselineUSD || today < costAnomalyMinTodayUSD {
		return false
	}
	return today >= baselineAvg*costAnomalyMultiplier
}

// checkCostAnomalies compares today's spend with the previous 7-day average,
// both globally and per conversation, and announces each new spike once per day.
// It shares the budget alert silence window: maintenance mode suppresses it too.
func (s *Server) checkCostAnomalies(ctx context.Context) (int, error) {
	if s.Store == nil {
		return 0, nil
	}
	now := time.Now().UTC()
	if s.budgetAlertsSilenced(now) {
		return 0, nil
	}
	dayKey := now.Format("2006-01-02")
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	fromDay := todayStart.AddDate(0, 0, -costAnomalyDays).Format("2006-01-02")
	toDay := todayStart.AddDate(0, 0, -1).Format("2006-01-02")

	state := costAnomalyState{DayKey: dayKey}
	if raw, ok, err := s.Store.GetSetting(costAnomalyStateKey); err == nil && ok {
		_ = json.Unmarshal(raw, &state)
	}
	if state.DayKey != dayKey {
		state = costAnomalyState{DayKey: dayKey}
	}
	if state.Conversations == nil {
		state.Conversations = map[string]bool{}
	}

	baseline, err := s.Store.SumRunCostsGroupedByDay("conversation", fromDay, toDay)
	if err != nil {
		return 0, err
	}
	today, err := s.Store.SumRunCostsGroupedByDay("conversation", dayKey, dayKey)
	if err != nil {
		return 0, err
	}

	sent := 0
	notify := func(scope, convID string, todayUSD, baselineAvg float64) {
		ratio := 0.0
		if baselineAvg > 0 {
			ratio = todayUSD / baselineAvg
		}
		s.recordCostAnomaly(costAnomalyHistoryEntry{
			At: time.Now().UTC(), Scope: scope, ConversationID: convID,
			TodayUSD: round6(todayUSD), BaselineUSD: round6(baselineAvg), Ratio: round4(ratio),
		})
		if s.Webhook == nil {
			return
		}
		data := map[string]any{
			"scope":        scope,
			"day":          dayKey,
			"today_usd":    round6(todayUSD),
			"baseline_usd": round6(baselineAvg),
			"ratio":        round4(ratio),
			"multiplier":   costAnomalyMultiplier,
			"days":         costAnomalyDays,
		}
		if convID != "" {
			data["conversation_id"] = convID
		}
		if err := s.Webhook.SendCostAnomaly(ctx, data); err != nil {
			if !errors.Is(err, webhook.ErrNoURL) {
				log.Printf("cost anomaly webhook (%s %s): %v", scope, convID, err)
			}
			return
		}
		sent++
	}

	if !state.Global {
		var baselineTotal, todayTotal float64
		for _, v := range baseline {
			baselineTotal += v
		}
		for _, v := range today {
			todayTotal += v
		}
		baselineAvg := baselineTotal / float64(costAnomalyDays)
		if costAnomalyTripped(baselineAvg, todayTotal) {
			notify("global", "", todayTotal, baselineAvg)
			state.Global = true
		}
	}

	type candidate struct {
		id    string
		today float64
		base  float64
	}
	candidates := make([]candidate, 0)
	for id, todayUSD := range today {
		if id == "" || state.Conversations[id] {
			continue
		}
		baselineAvg := baseline[id] / float64(costAnomalyDays)
		if costAnomalyTripped(baselineAvg, todayUSD) {
			candidates = append(candidates, candidate{id: id, today: todayUSD, base: baselineAvg})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].today == candidates[j].today {
			return candidates[i].id < candidates[j].id
		}
		return candidates[i].today > candidates[j].today
	})
	if len(candidates) > costAnomalyMaxConversations {
		candidates = candidates[:costAnomalyMaxConversations]
	}
	for _, c := range candidates {
		notify("conversation", c.id, c.today, c.base)
		state.Conversations[c.id] = true
	}

	raw, _ := json.Marshal(state)
	_ = s.Store.UpsertSetting(costAnomalyStateKey, raw)
	return sent, nil
}
