package api

import (
	"encoding/json"
	"net/http"
	"time"
)

const modelBudgetDayPrefix = "cost_budget_model_day:"
const modelBudgetMonthPrefix = "cost_budget_model_month:"
const modelBudgetWarnPrefix = "cost_budget_model_warn:"
const modelBudgetExceedPrefix = "cost_budget_model_exceed:"

func (s *Server) modelBudget(profileID string, month bool) float64 {
	if s.Store == nil || profileID == "" {
		return 0
	}
	key := modelBudgetDayPrefix
	if month {
		key = modelBudgetMonthPrefix
	}
	raw, ok, err := s.Store.GetSetting(key + profileID)
	if err != nil || !ok {
		return 0
	}
	var v float64
	if json.Unmarshal(raw, &v) != nil || v < 0 || v > 1_000_000 {
		return 0
	}
	return v
}

func (s *Server) modelBudgets(profileID string) (day, month float64) {
	return s.modelBudget(profileID, false), s.modelBudget(profileID, true)
}

func (s *Server) modelBudgetThresholds(profileID string) (warn, exceed float64) {
	return s.scopeRatio(modelBudgetWarnPrefix + profileID),
		s.scopeRatio(modelBudgetExceedPrefix + profileID)
}

// modelCostTotals returns per-model-profile day and month spend. Terminal costs
// come from the cached cost table in two grouped queries; recent in-flight runs
// are added from their partial usage.
func (s *Server) modelCostTotals(now time.Time) (map[string]float64, map[string]float64, error) {
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	month, err := s.Store.SumRunCostsGrouped("model", monthStart, now)
	if err != nil {
		return nil, nil, err
	}
	day, err := s.Store.SumRunCostsGrouped("model", dayStart, now)
	if err != nil {
		return nil, nil, err
	}
	runs, err := s.Store.ListRunsCreatedBetweenForScope(monthStart, now, "", "", scopePartialScanLimit)
	if err != nil {
		return nil, nil, err
	}
	for _, rec := range runs {
		if rec.ModelProfileID == "" || isTerminalRunStatus(rec.Status) {
			continue
		}
		cost := s.runCostCached(rec)
		if cost == 0 {
			continue
		}
		month[rec.ModelProfileID] += cost
		if !rec.CreatedAt.Before(dayStart) {
			day[rec.ModelProfileID] += cost
		}
	}
	return day, month, nil
}

type modelBudgetJSON struct {
	ModelProfileID string  `json:"model_profile_id"`
	Name           string  `json:"name"`
	Model          string  `json:"model"`
	BudgetDay      float64 `json:"budget_usd_per_day"`
	BudgetMonth    float64 `json:"budget_usd_per_month"`
	WarningRatio   float64 `json:"warning_ratio"`
	ExceededRatio  float64 `json:"exceeded_ratio"`
	SpentDay       float64 `json:"spent_usd_per_day"`
	SpentMonth     float64 `json:"spent_usd_per_month"`
}

func (s *Server) handleGetModelCostBudgets(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	dayByModel, monthByModel, err := s.modelCostTotals(now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	profiles, err := s.Store.ListModelProfiles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	items := make([]modelBudgetJSON, 0, len(profiles))
	for _, p := range profiles {
		day, month := s.modelBudgets(p.ID)
		warn, exceed := s.modelBudgetThresholds(p.ID)
		item := modelBudgetJSON{
			ModelProfileID: p.ID, Name: p.Name, Model: p.Model,
			BudgetDay: day, BudgetMonth: month,
			WarningRatio: warn, ExceededRatio: exceed,
		}
		if day > 0 || month > 0 {
			item.SpentDay = round6(dayByModel[p.ID])
			item.SpentMonth = round6(monthByModel[p.ID])
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handlePutModelCostBudget(w http.ResponseWriter, r *http.Request) {
	var body modelBudgetJSON
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.ModelProfileID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "model_profile_id is required")
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
	profile, err := s.Store.GetModelProfile(body.ModelProfileID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "model profile not found")
		return
	}
	dayRaw, _ := json.Marshal(body.BudgetDay)
	monthRaw, _ := json.Marshal(body.BudgetMonth)
	warnRaw, _ := json.Marshal(body.WarningRatio)
	exceedRaw, _ := json.Marshal(body.ExceededRatio)
	for key, raw := range map[string][]byte{
		modelBudgetDayPrefix + body.ModelProfileID:    dayRaw,
		modelBudgetMonthPrefix + body.ModelProfileID:  monthRaw,
		modelBudgetWarnPrefix + body.ModelProfileID:   warnRaw,
		modelBudgetExceedPrefix + body.ModelProfileID: exceedRaw,
	} {
		if err := s.Store.UpsertSetting(key, raw); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}
	s.resetScopeAlertState("model:" + body.ModelProfileID)
	body.Name = profile.Name
	body.Model = profile.Model
	if body.BudgetDay > 0 || body.BudgetMonth > 0 {
		dayByModel, monthByModel, cerr := s.modelCostTotals(time.Now().UTC())
		if cerr == nil {
			body.SpentDay = round6(dayByModel[body.ModelProfileID])
			body.SpentMonth = round6(monthByModel[body.ModelProfileID])
		}
	}
	writeJSON(w, http.StatusOK, body)
}
