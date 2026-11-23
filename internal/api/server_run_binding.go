package api

import (
	"encoding/json"
	"math"
	"net/http"

	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

func (s *Server) handleGetRunBinding(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	runRec, err := s.Store.GetRun(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	evs, err := s.Store.ListEvents(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	usage := s.usageWithCost(runRec, evs)
	for _, ev := range evs {
		if ev.Type == run.EventRunBinding {
			writeJSON(w, http.StatusOK, map[string]any{
				"run_id":    id,
				"timestamp": ev.Timestamp,
				"binding":   ev.Data,
				"usage":     usage,
			})
			return
		}
	}
	writeError(w, http.StatusNotFound, "binding_not_found", "run has no binding record")
}

// usageWithCost sums llm.usage events and prices each call by the model profile
// recorded on that event (falling back to the run's profile for older events).
func (s *Server) usageWithCost(runRec *store.Run, evs []store.Event) map[string]any {
	out := map[string]any{
		"turns":             0,
		"prompt_tokens":     0,
		"completion_tokens": 0,
		"total_tokens":      0,
		"cached_tokens":     0,
		"priced":            false,
	}
	table := s.modelPriceTable()
	profiles := map[string]*store.ModelProfile{}
	var cost float64
	pricedTurns, unpricedTurns := 0, 0
	for _, ev := range evs {
		if ev.Type != run.EventLLMUsage {
			continue
		}
		out["turns"] = out["turns"].(int) + 1
		prompt := intFromEventData(ev.Data["prompt_tokens"])
		completion := intFromEventData(ev.Data["completion_tokens"])
		cached := intFromEventData(ev.Data["cached_tokens"])
		out["prompt_tokens"] = out["prompt_tokens"].(int) + prompt
		out["completion_tokens"] = out["completion_tokens"].(int) + completion
		out["total_tokens"] = out["total_tokens"].(int) + intFromEventData(ev.Data["total_tokens"])
		out["cached_tokens"] = out["cached_tokens"].(int) + cached

		profileID := stringFromEventData(ev.Data["model_profile_id"])
		if profileID == "" && runRec != nil {
			profileID = runRec.ModelProfileID
		}
		profile, ok := profiles[profileID]
		if !ok && profileID != "" {
			if p, err := s.Store.GetModelProfile(profileID); err == nil {
				profile = &p
			}
			profiles[profileID] = profile
		}
		if profile == nil {
			unpricedTurns++
			continue
		}
		callCost, _, ok := table.Cost(profile.Model, prompt, completion, cached)
		if !ok {
			unpricedTurns++
			continue
		}
		cost += callCost
		pricedTurns++
	}
	if pricedTurns > 0 {
		out["priced"] = true
		out["cost_usd"] = math.Round(cost*1e6) / 1e6
		out["currency"] = table.Currency
		out["priced_turns"] = pricedTurns
		out["unpriced_turns"] = unpricedTurns
	}
	return out
}

func intFromEventData(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func stringFromEventData(v any) string {
	s, _ := v.(string)
	return s
}
