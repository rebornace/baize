package api

import (
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/procfs"
	"github.com/rebornace/baize/internal/store"
)

func (s *Server) handleGetConversationSkillDraft(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "conversation id is required")
		return
	}
	run, err := s.Store.LatestConversationRun(convID)
	if err != nil {
		if err == store.ErrNoPriorConversationRun {
			writeSkillDraftErr(w, store.ErrNoPriorConversationRun)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	out, err := s.buildSkillDraft(run.ID)
	if err != nil {
		writeSkillDraftErr(w, err)
		return
	}
	s.appendHumanSkillDraft(out.SourceRunID, procfs.EventHumanSkillDraftOpened, map[string]any{
		"source":          "chat",
		"conversation_id": convID,
	})
	writeJSON(w, http.StatusOK, out)
}
