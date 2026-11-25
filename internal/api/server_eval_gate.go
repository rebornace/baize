package api

import (
	"encoding/json"
	"net/http"

	"github.com/rebornace/baize/internal/eval"
	"github.com/rebornace/baize/internal/store"
)

const evalGateLastResultKey = "eval_gate_last_result"

func (s *Server) handleGetEvalGate(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"min_pass_rate":     s.evalGateMinRate(),
		"auto_hour":         s.evalGateAutoHour(),
		"score_history_cap": store.EvalScoreHistoryCapFor(s.Store),
		"last":              s.evalGateLastResult(),
	})
}

func (s *Server) handlePutEvalGate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MinPassRate     float64 `json:"min_pass_rate"`
		AutoHour        *int    `json:"auto_hour"`
		ScoreHistoryCap *int    `json:"score_history_cap"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.MinPassRate < 0 || body.MinPassRate > 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "min_pass_rate must be between 0 and 1")
		return
	}
	if body.AutoHour != nil && (*body.AutoHour < -1 || *body.AutoHour > 23) {
		writeError(w, http.StatusBadRequest, "invalid_request", "auto_hour must be -1 (off) or 0..23")
		return
	}
	if body.ScoreHistoryCap != nil &&
		(*body.ScoreHistoryCap < store.EvalScoreHistoryCapMin || *body.ScoreHistoryCap > store.EvalScoreHistoryCapMax) {
		writeError(w, http.StatusBadRequest, "invalid_request", "score_history_cap must be between 10 and 5000")
		return
	}
	raw, _ := json.Marshal(body.MinPassRate)
	if err := s.Store.UpsertSetting(eval.GateMinRateSettingKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	hour := s.evalGateAutoHour()
	if body.AutoHour != nil {
		hour = *body.AutoHour
		hourRaw, _ := json.Marshal(hour)
		if err := s.Store.UpsertSetting(evalGateAutoHourKey, hourRaw); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}
	cap := store.EvalScoreHistoryCapFor(s.Store)
	if body.ScoreHistoryCap != nil {
		cap = *body.ScoreHistoryCap
		capRaw, _ := json.Marshal(cap)
		if err := s.Store.UpsertSetting(store.SettingKeyEvalScoreHistoryCap, capRaw); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"min_pass_rate":     body.MinPassRate,
		"auto_hour":         hour,
		"score_history_cap": cap,
	})
}

func (s *Server) handleRunEvalGate(w http.ResponseWriter, r *http.Request) {
	res, err := eval.RunGate(s.Store, s.evalGateMinRate(), 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if raw, merr := json.Marshal(res); merr == nil {
		_ = s.Store.UpsertSetting(evalGateLastResultKey, raw)
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) evalGateMinRate() float64 {
	if s.Store == nil {
		return eval.GateDefaultMinRate
	}
	raw, ok, err := s.Store.GetSetting(eval.GateMinRateSettingKey)
	if err != nil || !ok {
		return eval.GateDefaultMinRate
	}
	var v float64
	if json.Unmarshal(raw, &v) != nil || v < 0 || v > 1 {
		return eval.GateDefaultMinRate
	}
	return v
}

func (s *Server) evalGateLastResult() *eval.GateResult {
	if s.Store == nil {
		return nil
	}
	raw, ok, err := s.Store.GetSetting(evalGateLastResultKey)
	if err != nil || !ok {
		return nil
	}
	var res eval.GateResult
	if json.Unmarshal(raw, &res) != nil {
		return nil
	}
	if res.Failures == nil {
		res.Failures = []eval.GateFailure{}
	}
	return &res
}
