package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/procfs"
	"github.com/rebornace/baize/internal/skill"
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
	writeJSON(w, http.StatusOK, out)
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
		ID       string `json:"id"`
		Markdown string `json:"markdown"`
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
	pkg, err := s.SkillCatalog.InstallMD("SKILL.md", raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": pkg.ID})
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
