package api

import (
	"encoding/json"
	"net/http"

	"github.com/rebornace/baize/internal/runtimecfg"
)

// runtimeKnobsJSON is the wire shape for engine knobs (duration as seconds).
type runtimeKnobsJSON struct {
	MaxMessages                  int     `json:"max_messages"`
	MaxSteps                     int     `json:"max_steps"`
	ToolTimeoutSeconds           int     `json:"tool_timeout_seconds"`
	CompactionEnabled            bool    `json:"compaction_enabled"`
	CompactThreshold             float64 `json:"compact_threshold"`
	CompactReserveTokens         int     `json:"compact_reserve_tokens"`
	CompactKeepRecent            int     `json:"compact_keep_recent"`
	CompactSummaryTimeoutSeconds int     `json:"compact_summary_timeout_seconds"`
	MemoryEnabled                bool    `json:"memory_enabled"`
	MemoryAutoExtract            bool    `json:"memory_auto_extract"`
	DecideEnabled                bool    `json:"decide_enabled"`
	DecideMemoryEnabled          bool    `json:"decide_memory_enabled"`
}

func knobsToJSON(k runtimecfg.Knobs) runtimeKnobsJSON {
	return runtimeKnobsJSON{
		MaxMessages:                  k.MaxMessages,
		MaxSteps:                     k.MaxSteps,
		ToolTimeoutSeconds:           int(k.ToolTimeout.Seconds()),
		CompactionEnabled:            k.CompactionEnabled,
		CompactThreshold:             k.CompactThreshold,
		CompactReserveTokens:         k.CompactReserveTokens,
		CompactKeepRecent:            k.CompactKeepRecent,
		CompactSummaryTimeoutSeconds: int(k.CompactSummaryTimeout.Seconds()),
		MemoryEnabled:                k.MemoryEnabled,
		MemoryAutoExtract:            k.MemoryAutoExtract,
		DecideEnabled:                k.DecideEnabled,
		DecideMemoryEnabled:          k.DecideMemoryEnabled,
	}
}

func (s *Server) handleGetRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	if s.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime_settings_unavailable", "runtime settings not wired")
		return
	}
	view := s.Settings.KnobsView()
	writeJSON(w, http.StatusOK, map[string]any{
		"effective":                  knobsToJSON(view.Effective),
		"overridden":                 view.Overridden,
		"public_base_url":            s.Settings.PublicBaseURL(),
		"public_base_url_overridden": s.Settings.PublicBaseURLOverridden(),
	})
}

func (s *Server) handlePatchRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	if s.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime_settings_unavailable", "runtime settings not wired")
		return
	}
	var patch runtimecfg.KnobsPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := s.Settings.ApplyKnobs(r.Context(), s.Store, patch); err != nil {
		if writeIfSettingsKeyRequired(w, err) {
			return
		}
		writeError(w, runtimecfg.HTTPStatus(err), "invalid_settings", err.Error())
		return
	}
	// Keep the OAuth / Apply path field in sync without restart.
	s.CallbackPublicBase = s.Settings.PublicBaseURL()
	view := s.Settings.KnobsView()
	writeJSON(w, http.StatusOK, map[string]any{
		"effective":                  knobsToJSON(view.Effective),
		"overridden":                 view.Overridden,
		"public_base_url":            s.Settings.PublicBaseURL(),
		"public_base_url_overridden": s.Settings.PublicBaseURLOverridden(),
	})
}

func (s *Server) handleGetCredentials(w http.ResponseWriter, r *http.Request) {
	if s.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime_settings_unavailable", "runtime settings not wired")
		return
	}
	writeJSON(w, http.StatusOK, s.Settings.CredentialsView())
}

func (s *Server) handlePatchCredentials(w http.ResponseWriter, r *http.Request) {
	if s.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime_settings_unavailable", "runtime settings not wired")
		return
	}
	var patch runtimecfg.CredsPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := s.Settings.ApplyCreds(r.Context(), s.Store, patch); err != nil {
		if writeIfSettingsKeyRequired(w, err) {
			return
		}
		writeError(w, runtimecfg.HTTPStatus(err), "invalid_credentials", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.Settings.CredentialsView())
}
