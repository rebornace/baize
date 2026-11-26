package api

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/store"
)

// auditExcludedPrefixes covers high-volume workload and unauthenticated
// callback traffic. Administrative configuration changes remain audited.
var auditExcludedPrefixes = []string{
	"/v0/runs",
	"/v0/hitl",
	"/v0/inbox",
	"/v0/auth/hosted",
	"/v0/mcp/export",
}

func (s *Server) shouldAuditRequest(r *http.Request) bool {
	if s.Store == nil {
		return false
	}
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}
	path := r.URL.Path
	if !strings.HasPrefix(path, "/v0/") {
		return false
	}
	for _, prefix := range auditExcludedPrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return false
		}
	}
	if strings.HasSuffix(path, "/inbound") || strings.HasSuffix(path, "/plugin-callbacks") {
		return false
	}
	// Read receipts are high-frequency user actions, not administrative ones.
	if strings.HasPrefix(path, "/v0/conversations/") && strings.HasSuffix(path, "/read") {
		return false
	}
	return true
}

type auditStatusWriter struct {
	http.ResponseWriter
	status int
}

func (w *auditStatusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *auditStatusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}

func (w *auditStatusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *auditStatusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

const auditRetentionSettingKey = "audit_retention_rows"
const auditRetentionMin = 100

func (s *Server) auditRetentionRows() int {
	if s.Store == nil {
		return store.AuditEventRetention
	}
	raw, ok, err := s.Store.GetSetting(auditRetentionSettingKey)
	if err != nil || !ok {
		return store.AuditEventRetention
	}
	var rows int
	if json.Unmarshal(raw, &rows) != nil || rows < auditRetentionMin || rows > store.AuditEventRetention {
		return store.AuditEventRetention
	}
	return rows
}

func (s *Server) handleGetAuditRetention(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"rows": s.auditRetentionRows(),
		"min":  auditRetentionMin,
		"max":  store.AuditEventRetention,
	})
}

func (s *Server) handlePutAuditRetention(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Rows int `json:"rows"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.Rows < auditRetentionMin || body.Rows > store.AuditEventRetention {
		writeError(w, http.StatusBadRequest, "invalid_request", "rows must be between 100 and 5000")
		return
	}
	raw, _ := json.Marshal(body.Rows)
	if err := s.Store.UpsertSetting(auditRetentionSettingKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	// Apply the new cap immediately so the change is visible in the list.
	if err := s.Store.TrimAuditEvents(body.Rows); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": body.Rows})
}

// recordAudit persists one audit row. Failures are ignored on purpose: an audit
// write must never turn a successful administrative action into an error.
func (s *Server) recordAudit(r *http.Request, status int) {
	if s.Store == nil {
		return
	}
	principal := s.principalFrom(r.Context())
	action := strings.TrimSpace(r.Pattern)
	if action == "" {
		action = r.Method + " " + r.URL.Path
	}
	_ = s.Store.AppendAuditEvent(store.AuditEvent{
		OperatorID: principal.OperatorID,
		Role:       string(principal.Role),
		Method:     r.Method,
		Action:     action,
		Target:     r.URL.Path,
		Status:     status,
		RemoteAddr: r.RemoteAddr,
	})
	if keep := s.auditRetentionRows(); keep < store.AuditEventRetention {
		_ = s.Store.TrimAuditEvents(keep)
	}
}

// handleListAuditEvents returns newest-first audit rows. Admin-only by the
// control-plane default for unmatched /v0 paths.
func (s *Server) handleListAuditEvents(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	if limit > 500 {
		limit = 500
	}
	filter, err := auditFilterFromQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	filter.Limit = limit
	items, err := s.Store.ListAuditEvents(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if items == nil {
		items = []store.AuditEvent{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"limit": limit,
	})
}

// auditFilterFromQuery parses the shared list/export filters.
func auditFilterFromQuery(r *http.Request) (store.AuditFilter, error) {
	q := r.URL.Query()
	filter := store.AuditFilter{
		Action:     strings.TrimSpace(q.Get("action")),
		OperatorID: strings.TrimSpace(q.Get("operator_id")),
		Query:      strings.TrimSpace(q.Get("q")),
	}
	if raw := strings.TrimSpace(q.Get("from")); raw != "" {
		t, err := parseFlexibleTime(raw)
		if err != nil {
			return filter, errors.New("from must be an RFC3339 timestamp")
		}
		filter.From = t
	}
	if raw := strings.TrimSpace(q.Get("to")); raw != "" {
		t, err := parseFlexibleTime(raw)
		if err != nil {
			return filter, errors.New("to must be an RFC3339 timestamp")
		}
		filter.To = t
	}
	return filter, nil
}

const auditExportLimit = store.AuditEventRetention

// handleExportAuditEvents streams the filtered audit trail as CSV. The export
// stays authenticated like the list endpoint, so it is admin-only.
func (s *Server) handleExportAuditEvents(w http.ResponseWriter, r *http.Request) {
	filter, err := auditFilterFromQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	filter.Limit = auditExportLimit
	items, err := s.Store.ListAuditEvents(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		`attachment; filename="baize-audit-`+time.Now().UTC().Format("20060102-150405")+`.csv"`)
	// BOM keeps Excel from mangling Chinese action names.
	_, _ = w.Write([]byte("\ufeff"))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{
		"time", "operator_id", "role", "method", "action", "target", "status", "remote_addr",
	})
	for _, e := range items {
		_ = cw.Write([]string{
			e.CreatedAt.UTC().Format(time.RFC3339),
			e.OperatorID, e.Role, e.Method, e.Action, e.Target,
			strconv.Itoa(e.Status), e.RemoteAddr,
		})
	}
	cw.Flush()
}
