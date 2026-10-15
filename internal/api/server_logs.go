package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/rebornace/baize/internal/applog"
)

const (
	defaultLogLimit = 200
	maxLogLimit     = 1000
)

func (s *Server) handleGetRecentLogs(w http.ResponseWriter, r *http.Request) {
	limit := defaultLogLimit
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be a positive integer")
			return
		}
		limit = n
	}
	if limit > maxLogLimit {
		limit = maxLogLimit
	}
	lines := applog.Recent(limit)
	if lines == nil {
		lines = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"lines": lines,
		"limit": limit,
	})
}
