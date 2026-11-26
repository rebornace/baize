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
const costAnomalyMaxAgents = 3
const costAnomalyMaxModels = 3

type costAnomalyState struct {
	DayKey string          `json:"day_key"`
	Global bool            `json:"global,omitempty"`
	Scopes map[string]bool `json:"scopes,omitempty"`
	// Conversations is the pre-scope-map format; read for compatibility.
	Conversations map[string]bool `json:"conversations,omitempty"`
}

type costAnomalyHistoryEntry struct {
	At             time.Time `json:"at"`
	Scope          string    `json:"scope"`
	ConversationID string    `json:"conversation_id,omitempty"`
	AgentID        string    `json:"agent_id,omitempty"`
	ModelID        string    `json:"model_id,omitempty"`
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

type costAnomalyHit struct {
	scope string
	id    string
	key   string
	today float64
	base  float64
}

// collectCostAnomalies returns at most `limit` new spikes for one dimension.
func collectCostAnomalies(
	scope string, today, baseline map[string]float64, seen map[string]bool, limit int,
) []costAnomalyHit {
	hits := make([]costAnomalyHit, 0)
	for id, todayUSD := range today {
		if id == "" {
			continue
		}
		key := scope + ":" + id
		if seen[key] {
			continue
		}
		baselineAvg := baseline[id] / float64(costAnomalyDays)
		if costAnomalyTripped(baselineAvg, todayUSD) {
			hits = append(hits, costAnomalyHit{
				scope: scope, id: id, key: key, today: todayUSD, base: baselineAvg,
			})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].today == hits[j].today {
			return hits[i].id < hits[j].id
		}
		return hits[i].today > hits[j].today
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// checkCostAnomalies compares today's spend with the previous 7-day average
// globally, per conversation, per agent and per model. Specific spikes are
// preferred over the global roll-up so one incident does not produce a global
// alert plus every narrower scope in the same run.
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
	if state.Scopes == nil {
		state.Scopes = map[string]bool{}
	}
	for id := range state.Conversations {
		state.Scopes["conversation:"+id] = true
	}

	baselineConv, err := s.Store.SumRunCostsGroupedByDay("conversation", fromDay, toDay)
	if err != nil {
		return 0, err
	}
	todayConv, err := s.Store.SumRunCostsGroupedByDay("conversation", dayKey, dayKey)
	if err != nil {
		return 0, err
	}
	baselineAgent, err := s.Store.SumRunCostsGroupedByDay("agent", fromDay, toDay)
	if err != nil {
		return 0, err
	}
	todayAgent, err := s.Store.SumRunCostsGroupedByDay("agent", dayKey, dayKey)
	if err != nil {
		return 0, err
	}
	baselineModel, err := s.Store.SumRunCostsGroupedByDay("model", fromDay, toDay)
	if err != nil {
		return 0, err
	}
	todayModel, err := s.Store.SumRunCostsGroupedByDay("model", dayKey, dayKey)
	if err != nil {
		return 0, err
	}

	hits := collectCostAnomalies("conversation", todayConv, baselineConv, state.Scopes, costAnomalyMaxConversations)
	hits = append(hits, collectCostAnomalies("agent", todayAgent, baselineAgent, state.Scopes, costAnomalyMaxAgents)...)
	hits = append(hits, collectCostAnomalies("model", todayModel, baselineModel, state.Scopes, costAnomalyMaxModels)...)

	sent := 0
	notify := func(scope, id string, todayUSD, baselineAvg float64) {
		ratio := 0.0
		if baselineAvg > 0 {
			ratio = todayUSD / baselineAvg
		}
		entry := costAnomalyHistoryEntry{
			At: time.Now().UTC(), Scope: scope,
			TodayUSD: round6(todayUSD), BaselineUSD: round6(baselineAvg), Ratio: round4(ratio),
		}
		switch scope {
		case "conversation":
			entry.ConversationID = id
		case "agent":
			entry.AgentID = id
		case "model":
			entry.ModelID = id
		}
		s.recordCostAnomaly(entry)
		data := map[string]any{
			"scope":        scope,
			"day":          dayKey,
			"today_usd":    round6(todayUSD),
			"baseline_usd": round6(baselineAvg),
			"ratio":        round4(ratio),
			"multiplier":   costAnomalyMultiplier,
			"days":         costAnomalyDays,
		}
		switch scope {
		case "conversation":
			data["conversation_id"] = id
		case "agent":
			data["agent_id"] = id
		case "model":
			data["model_id"] = id
		}
		if s.Webhook != nil {
			if err := s.Webhook.SendCostAnomaly(ctx, data); err != nil {
				if !errors.Is(err, webhook.ErrNoURL) {
					log.Printf("cost anomaly webhook (%s %s): %v", scope, id, err)
				}
			} else {
				sent++
			}
		}
		s.sendAlertChannels(ctx, "cost.anomaly", data)
	}

	for _, hit := range hits {
		notify(hit.scope, hit.id, hit.today, hit.base)
		state.Scopes[hit.key] = true
	}

	// Global alert only when no narrower scope fired (including scopes already
	// announced earlier today); otherwise the specific alerts explain the spike.
	if len(hits) == 0 && len(state.Scopes) == 0 && !state.Global {
		var baselineTotal, todayTotal float64
		for _, v := range baselineConv {
			baselineTotal += v
		}
		for _, v := range todayConv {
			todayTotal += v
		}
		baselineAvg := baselineTotal / float64(costAnomalyDays)
		if costAnomalyTripped(baselineAvg, todayTotal) {
			notify("global", "", todayTotal, baselineAvg)
			state.Global = true
		}
	}

	raw, _ := json.Marshal(state)
	_ = s.Store.UpsertSetting(costAnomalyStateKey, raw)
	return sent, nil
}
