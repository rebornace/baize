package api

import (
	"net/http"

	"github.com/rebornace/baize/internal/run"
)

func (s *Server) handleGetRunBinding(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.Store.GetRun(id); err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	evs, err := s.Store.ListEvents(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	for _, ev := range evs {
		if ev.Type == run.EventRunBinding {
			writeJSON(w, http.StatusOK, map[string]any{
				"run_id":    id,
				"timestamp": ev.Timestamp,
				"binding":   ev.Data,
			})
			return
		}
	}
	writeError(w, http.StatusNotFound, "binding_not_found", "run has no binding record")
}
