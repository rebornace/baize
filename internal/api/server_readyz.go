package api

import "net/http"

// handleReadyz reports whether the server can reach its backing store. It is
// intentionally separate from /healthz: liveness stays 200 while the process is
// alive, readiness fails when the database is unavailable.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "no store"})
		return
	}
	// A point lookup keeps probes cheap; an aggregate over runs would scan the
	// whole table on every probe.
	if _, _, err := s.Store.GetSetting("__readyz_probe__"); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "store unavailable",
			"error":  err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}
