package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/procfs"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
)

type skillDraftTimelineJSON struct {
	RunID     string `json:"run_id"`
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Summary   string `json:"summary"`
}

type skillDraftResponseJSON struct {
	SourceRunID    string                   `json:"source_run_id"`
	ConversationID string                   `json:"conversation_id,omitempty"`
	Timeline       []skillDraftTimelineJSON `json:"timeline"`
	SuggestedID    string                   `json:"suggested_id"`
	Markdown       string                   `json:"markdown"`
}

func (s *Server) handleGetEvalRunSkillDraft(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("id"))
	if runID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "run id is required")
		return
	}
	out, err := s.buildSkillDraft(runID)
	if err != nil {
		writeSkillDraftErr(w, err)
		return
	}
	s.appendHumanSkillDraft(out.SourceRunID, procfs.EventHumanSkillDraftOpened, map[string]any{
		"source": "eval",
	})
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handlePostEvalRunSkillDraftPolish(w http.ResponseWriter, r *http.Request) {
	if s.LLM == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "llm not configured")
		return
	}
	runID := strings.TrimSpace(r.PathValue("id"))
	if runID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "run id is required")
		return
	}
	var body struct {
		Markdown string `json:"markdown"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	ruleMD := strings.TrimSpace(body.Markdown)
	tl, err := procfs.BuildTimeline(s.Store, runID)
	if err != nil {
		writeSkillDraftErr(w, err)
		return
	}
	if ruleMD == "" {
		draft, err := procfs.RenderDraft(tl, tl.SourceRunID)
		if err != nil {
			writeSkillDraftErr(w, err)
			return
		}
		raw, err := skill.BuildSKILLMD(draft)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		ruleMD = string(raw)
	}
	polished, err := procfs.PolishMarkdown(r.Context(), s.LLM, tl, ruleMD)
	if err != nil {
		writeError(w, http.StatusBadGateway, "polish_failed", err.Error())
		return
	}
	s.appendHumanSkillDraft(tl.SourceRunID, procfs.EventHumanSkillDraftPolished, map[string]any{
		"had_body": strings.TrimSpace(body.Markdown) != "",
	})
	writeJSON(w, http.StatusOK, map[string]string{"markdown": polished})
}

func (s *Server) handlePostEvalRunSkillDraftInstall(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "skill catalog not configured")
		return
	}
	runID := strings.TrimSpace(r.PathValue("id"))
	if runID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "run id is required")
		return
	}
	var body struct {
		ID             string `json:"id"`
		Markdown       string `json:"markdown"`
		CreateEvalStub bool   `json:"create_eval_stub"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	raw := []byte(strings.TrimSpace(body.Markdown))
	if len(raw) == 0 {
		draft, err := s.buildSkillDraft(runID)
		if err != nil {
			writeSkillDraftErr(w, err)
			return
		}
		raw = []byte(draft.Markdown)
	}
	if id := strings.TrimSpace(body.ID); id != "" {
		pkg, err := skill.ParseSKILLMD(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		if pkg.Name != id {
			writeError(w, http.StatusBadRequest, "invalid_request", "id does not match markdown name")
			return
		}
	}
	edited := false
	if rule, err := s.buildSkillDraft(runID); err == nil {
		edited = strings.TrimSpace(rule.Markdown) != strings.TrimSpace(string(raw))
	}
	pkg, err := s.SkillCatalog.InstallMD("SKILL.md", raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	s.appendHumanSkillDraft(runID, procfs.EventHumanSkillDraftInstalled, map[string]any{
		"skill_id":         pkg.ID,
		"create_eval_stub": body.CreateEvalStub,
		"edited":           edited,
	})
	resp := map[string]string{"id": pkg.ID}
	if body.CreateEvalStub {
		resp["eval_stub"] = s.applyEvalStub(pkg.ID)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) appendHumanSkillDraft(runID, typ string, data map[string]any) {
	if s.Store == nil || runID == "" || typ == "" {
		return
	}
	_ = s.Store.AppendEvent(runID, store.Event{Type: typ, Data: data})
}

// applyEvalStub writes an empty evidence-eval EVAL when none exists.
// Returns created | skipped_exists | error.
func (s *Server) applyEvalStub(skillID string) string {
	if _, ok := s.SkillCatalog.GetEval(skillID); ok {
		return "skipped_exists"
	}
	err := s.SkillCatalog.PutEval(skillID, skill.EvalDoc{
		Harness: "evidence-eval",
		Cases:   []skill.EvalCase{},
	})
	if err != nil {
		return "error"
	}
	return "created"
}

func (s *Server) buildSkillDraft(runID string) (skillDraftResponseJSON, error) {
	if _, err := s.Store.GetRun(runID); err != nil {
		return skillDraftResponseJSON{}, errRunNotFound
	}
	tl, err := procfs.BuildTimeline(s.Store, runID)
	if err != nil {
		return skillDraftResponseJSON{}, err
	}
	draft, err := procfs.RenderDraft(tl, tl.SourceRunID)
	if err != nil {
		return skillDraftResponseJSON{}, err
	}
	raw, err := skill.BuildSKILLMD(draft)
	if err != nil {
		return skillDraftResponseJSON{}, err
	}
	timeline := make([]skillDraftTimelineJSON, 0, len(tl.Entries))
	for _, e := range tl.Entries {
		timeline = append(timeline, skillDraftTimelineJSON{
			RunID:     e.RunID,
			Type:      e.Type,
			Timestamp: formatTimeUTC(e.Timestamp),
			Summary:   e.Summary,
		})
	}
	return skillDraftResponseJSON{
		SourceRunID:    tl.SourceRunID,
		ConversationID: tl.ConversationID,
		Timeline:       timeline,
		SuggestedID:    procfs.SuggestedSkillID(tl.SourceRunID),
		Markdown:       string(raw),
	}, nil
}

var errRunNotFound = errors.New("run not found")

func writeSkillDraftErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errRunNotFound):
		writeError(w, http.StatusNotFound, "not_found", "run not found")
	case errors.Is(err, store.ErrNoPriorConversationRun):
		writeError(w, http.StatusNotFound, "not_found", "no run for conversation")
	case errors.Is(err, procfs.ErrNoEvents):
		writeError(w, http.StatusUnprocessableEntity, "no_events", err.Error())
	default:
		if err != nil && err.Error() == "run not found" {
			writeError(w, http.StatusNotFound, "not_found", "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
	}
}
