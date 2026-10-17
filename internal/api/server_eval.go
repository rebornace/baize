package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/eval"
	"github.com/rebornace/baize/internal/store"
)

type evalCandidateJSON struct {
	ID           string  `json:"id"`
	AgentID      string  `json:"agent_id"`
	Status       string  `json:"status"`
	CreatedAt    string  `json:"created_at"`
	EndedAt      *string `json:"ended_at,omitempty"`
	DurationMs   *int64  `json:"duration_ms,omitempty"`
	InputPreview string  `json:"input_preview"`
}

type evalCaseJSON struct {
	ID                string                  `json:"id"`
	SourceRunID       string                  `json:"source_run_id"`
	Status            store.EvalCaseStatus    `json:"status"`
	Protocol          string                  `json:"protocol,omitempty"`
	Input             string                  `json:"input"`
	RequiredReads     []string                `json:"required_reads,omitempty"`
	Consequential     []string                `json:"consequential,omitempty"`
	ExpectConseqAny   []string                `json:"expect_consequential_any,omitempty"`
	Chain             []store.EvalChainLink   `json:"chain,omitempty"`
	RunStatusAtIngest string                  `json:"run_status_at_ingest,omitempty"`
	LastScorePass     *bool                   `json:"last_score_pass,omitempty"`
	LastScoreCodes    []string                `json:"last_score_codes,omitempty"`
	LastScoreNotes    string                  `json:"last_score_notes,omitempty"`
	LastScoredAt      *string                 `json:"last_scored_at,omitempty"`
	HumanAgree        *bool                   `json:"human_agree,omitempty"`
	HumanNote         string                  `json:"human_note,omitempty"`
	CalibratedAt      string                  `json:"calibrated_at,omitempty"`
	ForbidTools       []string                `json:"forbid_tools,omitempty"`
	ArgContains       []store.EvalArgContains `json:"arg_contains,omitempty"`
	ExpectState       []store.EvalExpectState `json:"expect_state,omitempty"`
	HasStateSnapshot  bool                    `json:"has_state_snapshot"`
	StateSnapshot     json.RawMessage         `json:"state_snapshot,omitempty"`
	CreatedAt         string                  `json:"created_at"`
	UpdatedAt         string                  `json:"updated_at"`
	Events            []store.Event           `json:"events,omitempty"`
}

type evalCalibrationSummaryJSON struct {
	ScoredCount             int      `json:"scored_count"`
	CalibratedCount         int      `json:"calibrated_count"`
	AgreeCount              int      `json:"agree_count"`
	DisagreeCount           int      `json:"disagree_count"`
	AgreementRate           *float64 `json:"agreement_rate"`
	UncalibratedScoredCount int      `json:"uncalibrated_scored_count"`
}

type evalScoreJSON struct {
	Pass  bool     `json:"pass"`
	Codes []string `json:"codes"`
	Notes string   `json:"notes,omitempty"`
}

func formatTimeUTC(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func formatTimePtrUTC(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := formatTimeUTC(*t)
	return &s
}

func evalCaseToJSON(c store.EvalCase, includeEvents, includeSnapshot bool) (evalCaseJSON, error) {
	out := evalCaseJSON{
		ID:                c.ID,
		SourceRunID:       c.SourceRunID,
		Status:            c.Status,
		Protocol:          c.Protocol,
		Input:             c.Input,
		RequiredReads:     nonNilStrings(c.RequiredReads),
		Consequential:     nonNilStrings(c.Consequential),
		ExpectConseqAny:   c.ExpectConseqAny,
		Chain:             c.Chain,
		RunStatusAtIngest: c.RunStatusAtIngest,
		LastScorePass:     c.LastScorePass,
		LastScoreCodes:    c.LastScoreCodes,
		LastScoreNotes:    c.LastScoreNotes,
		LastScoredAt:      formatTimePtrUTC(c.LastScoredAt),
		HumanAgree:        c.HumanAgree,
		HumanNote:         c.HumanNote,
		ForbidTools:       nonNilStrings(c.ForbidTools),
		ArgContains:       c.ArgContains,
		ExpectState:       c.ExpectState,
		HasStateSnapshot:  len(c.StateSnapshotJSON) > 0,
		CreatedAt:         formatTimeUTC(c.CreatedAt),
		UpdatedAt:         formatTimeUTC(c.UpdatedAt),
	}
	if c.CalibratedAt != nil {
		out.CalibratedAt = formatTimeUTC(*c.CalibratedAt)
	}
	if includeSnapshot && len(c.StateSnapshotJSON) > 0 {
		out.StateSnapshot = json.RawMessage(c.StateSnapshotJSON)
	}
	if includeEvents && len(c.EventsJSON) > 0 {
		var evs []store.Event
		if err := json.Unmarshal(c.EventsJSON, &evs); err != nil {
			return evalCaseJSON{}, err
		}
		out.Events = evs
	}
	return out, nil
}

func candidateToJSON(c eval.Candidate) evalCandidateJSON {
	out := evalCandidateJSON{
		ID:           c.ID,
		AgentID:      c.AgentID,
		Status:       string(c.Status),
		CreatedAt:    formatTimeUTC(c.CreatedAt),
		InputPreview: c.InputPreview,
	}
	if c.EndedAt != nil {
		s := formatTimeUTC(*c.EndedAt)
		out.EndedAt = &s
	}
	if c.DurationMs != nil {
		ms := *c.DurationMs
		out.DurationMs = &ms
	}
	return out
}

func parseEvalRFC3339(name, raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errors.New(name + " is required")
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, errors.New(name + " must be RFC3339")
	}
	return t.UTC(), nil
}

func writeEvalErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, eval.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, eval.ErrRunNotFound), errors.Is(err, eval.ErrCaseNotFound), errors.Is(err, store.ErrEvalCaseNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, eval.ErrNotScorable):
		writeError(w, http.StatusBadRequest, "not_scorable", err.Error())
	case errors.Is(err, eval.ErrNotCalibratable):
		writeError(w, http.StatusBadRequest, "not_calibratable", err.Error())
	case errors.Is(err, eval.ErrNoteTooLong):
		writeError(w, http.StatusBadRequest, "note_too_long", err.Error())
	case errors.Is(err, eval.ErrMockBaseRequired):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
	}
}

func (s *Server) handleGetEvalRuns(w http.ResponseWriter, r *http.Request) {
	from, err := parseEvalRFC3339("from", r.URL.Query().Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	to, err := parseEvalRFC3339("to", r.URL.Query().Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	q := eval.Query{From: from, To: to}
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		q.Status = store.Status(raw)
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("min_duration_ms")); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "min_duration_ms must be a non-negative integer")
			return
		}
		q.MinDurationMs = n
	}
	q.EventType = strings.TrimSpace(r.URL.Query().Get("event_type"))
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be a positive integer")
			return
		}
		q.Limit = n
	}
	cands, err := eval.QueryCandidates(s.Store, q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	items := make([]evalCandidateJSON, 0, len(cands))
	for _, c := range cands {
		items = append(items, candidateToJSON(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handlePostEvalCase(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RunID           string                  `json:"run_id"`
		Replace         bool                    `json:"replace"`
		Protocol        string                  `json:"protocol"`
		RequiredReads   []string                `json:"required_reads"`
		Consequential   []string                `json:"consequential"`
		ExpectConseqAny []string                `json:"expect_consequential_any"`
		Chain           []store.EvalChainLink   `json:"chain"`
		ForbidTools     []string                `json:"forbid_tools"`
		ArgContains     []store.EvalArgContains `json:"arg_contains"`
		ExpectState     []store.EvalExpectState `json:"expect_state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	runID := strings.TrimSpace(body.RunID)
	if runID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "run_id is required")
		return
	}
	in := eval.IngestInput{
		RunID:           runID,
		Replace:         body.Replace,
		Protocol:        body.Protocol,
		RequiredReads:   body.RequiredReads,
		Consequential:   body.Consequential,
		ExpectConseqAny: body.ExpectConseqAny,
		Chain:           body.Chain,
		ForbidTools:     body.ForbidTools,
		ArgContains:     body.ArgContains,
		ExpectState:     body.ExpectState,
	}
	c, err := eval.IngestFromRun(s.Store, in)
	if err != nil {
		writeEvalErr(w, err)
		return
	}
	out, err := evalCaseToJSON(c, false, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleListEvalCases(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be a positive integer")
			return
		}
		limit = n
	}
	list, err := s.Store.ListEvalCases(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	items := make([]evalCaseJSON, 0, len(list))
	for _, c := range list {
		out, err := evalCaseToJSON(c, false, false)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		items = append(items, out)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleGetEvalCase(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	c, err := s.Store.GetEvalCase(id)
	if errors.Is(err, store.ErrEvalCaseNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "eval case not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	out, err := evalCaseToJSON(c, true, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handlePatchEvalCase(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var body struct {
		Protocol           *string                 `json:"protocol"`
		RequiredReads      []string                `json:"required_reads"`
		Consequential      []string                `json:"consequential"`
		ExpectConseqAny    []string                `json:"expect_consequential_any"`
		Chain              []store.EvalChainLink   `json:"chain"`
		Status             *store.EvalCaseStatus   `json:"status"`
		SetRequiredReads   bool                    `json:"-"`
		SetConsequential   bool                    `json:"-"`
		SetExpectConseqAny bool                    `json:"-"`
		SetChain           bool                    `json:"-"`
		ForbidTools        []string                `json:"forbid_tools"`
		SetForbidTools     bool                    `json:"-"`
		ArgContains        []store.EvalArgContains `json:"arg_contains"`
		SetArgContains     bool                    `json:"-"`
		ExpectState        []store.EvalExpectState `json:"expect_state"`
		SetExpectState     bool                    `json:"-"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	raw := map[string]json.RawMessage{}
	if err := dec.Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if v, ok := raw["protocol"]; ok {
		if err := json.Unmarshal(v, &body.Protocol); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
	}
	if v, ok := raw["required_reads"]; ok {
		body.SetRequiredReads = true
		if err := json.Unmarshal(v, &body.RequiredReads); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
	}
	if v, ok := raw["consequential"]; ok {
		body.SetConsequential = true
		if err := json.Unmarshal(v, &body.Consequential); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
	}
	if v, ok := raw["expect_consequential_any"]; ok {
		body.SetExpectConseqAny = true
		if err := json.Unmarshal(v, &body.ExpectConseqAny); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
	}
	if v, ok := raw["chain"]; ok {
		body.SetChain = true
		if err := json.Unmarshal(v, &body.Chain); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
	}
	if v, ok := raw["status"]; ok {
		if err := json.Unmarshal(v, &body.Status); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
	}
	if v, ok := raw["forbid_tools"]; ok {
		body.SetForbidTools = true
		if err := json.Unmarshal(v, &body.ForbidTools); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
	}
	if v, ok := raw["arg_contains"]; ok {
		body.SetArgContains = true
		if err := json.Unmarshal(v, &body.ArgContains); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
	}
	if v, ok := raw["expect_state"]; ok {
		body.SetExpectState = true
		if err := json.Unmarshal(v, &body.ExpectState); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
	}
	patch := eval.PatchInput{
		SetRequiredReads:   body.SetRequiredReads,
		RequiredReads:      body.RequiredReads,
		SetConsequential:   body.SetConsequential,
		Consequential:      body.Consequential,
		SetExpectConseqAny: body.SetExpectConseqAny,
		ExpectConseqAny:    body.ExpectConseqAny,
		SetChain:           body.SetChain,
		Chain:              body.Chain,
		SetForbidTools:     body.SetForbidTools,
		ForbidTools:        body.ForbidTools,
		SetArgContains:     body.SetArgContains,
		ArgContains:        body.ArgContains,
		SetExpectState:     body.SetExpectState,
		ExpectState:        body.ExpectState,
	}
	if body.Protocol != nil {
		patch.SetProtocol = true
		patch.Protocol = *body.Protocol
	}
	if body.Status != nil {
		patch.SetStatus = true
		patch.Status = *body.Status
	}
	c, err := eval.PatchCase(s.Store, id, patch)
	if err != nil {
		writeEvalErr(w, err)
		return
	}
	out, err := evalCaseToJSON(c, false, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handlePostEvalCaseScore(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	res, err := eval.ScoreCase(s.Store, id)
	if err != nil {
		writeEvalErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, evalScoreJSON{
		Pass:  res.Pass,
		Codes: nonNilStrings(res.Codes),
		Notes: res.Notes,
	})
}

func (s *Server) handlePostEvalCaseCheckState(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var body struct {
		MockBaseURL string `json:"mock_base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	c, _, err := eval.CheckState(s.Store, id, body.MockBaseURL, nil)
	if err != nil {
		writeEvalErr(w, err)
		return
	}
	out, err := evalCaseToJSON(c, false, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetEvalCaseExport(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	raw, err := eval.Export(s.Store, id)
	if err != nil {
		writeEvalErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (s *Server) handlePatchEvalCaseCalibration(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	raw := map[string]json.RawMessage{}
	if err := dec.Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	agreeRaw, ok := raw["agree"]
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "agree is required")
		return
	}
	var agree *bool
	if string(agreeRaw) == "null" {
		agree = nil
	} else {
		var b bool
		if err := json.Unmarshal(agreeRaw, &b); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
		agree = &b
	}
	note := ""
	if v, ok := raw["note"]; ok {
		if err := json.Unmarshal(v, &note); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}
	}
	c, err := eval.SetCalibration(s.Store, id, agree, note)
	if err != nil {
		writeEvalErr(w, err)
		return
	}
	out, err := evalCaseToJSON(c, false, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetEvalCalibrationSummary(w http.ResponseWriter, r *http.Request) {
	sum, err := eval.CalibrationSummary(s.Store)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, evalCalibrationSummaryJSON{
		ScoredCount:             sum.ScoredCount,
		CalibratedCount:         sum.CalibratedCount,
		AgreeCount:              sum.AgreeCount,
		DisagreeCount:           sum.DisagreeCount,
		AgreementRate:           sum.AgreementRate,
		UncalibratedScoredCount: sum.UncalibratedScoredCount,
	})
}

func (s *Server) handleDeleteEvalCase(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if err := s.Store.DeleteEvalCase(id); err != nil {
		if errors.Is(err, store.ErrEvalCaseNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "eval case not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
