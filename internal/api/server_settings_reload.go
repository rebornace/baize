package api

import "net/http"

func (s *Server) handlePostSettingsReload(w http.ResponseWriter, r *http.Request) {
	if s.ReloadConfig == nil {
		writeError(w, http.StatusServiceUnavailable, "reload_unavailable", "config reload is not available")
		return
	}
	if err := s.ReloadConfig(); err != nil {
		writeError(w, http.StatusInternalServerError, "reload_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "reloaded",
		"message": "layered config reloaded into runtime baseline",
	})
}
