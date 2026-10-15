package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/applog"
	"github.com/rebornace/baize/internal/otelx"
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
	DecideProfileID              string  `json:"decide_profile_id"`
	DecideSystemOneBaseURL       string  `json:"decide_systemone_base_url"`
	DecideSystemOneAPIKeySet     bool    `json:"decide_systemone_api_key_set"`
	DecideSystemOneModel         string  `json:"decide_systemone_model"`
	DecideToolRoutingEnabled     bool    `json:"decide_tool_routing_enabled"`
	DecideToolThreshold          int     `json:"decide_tool_threshold"`
	DecideToolPreTopK            int     `json:"decide_tool_pre_topk"`
	DecideToolChoiceEnabled      bool    `json:"decide_tool_choice_enabled"`
	DecideToolPruneEnabled       bool    `json:"decide_tool_prune_enabled"`
	DecideToolPruneThreshold     int     `json:"decide_tool_prune_threshold"`
	DecideToolPruneMaxJudged     int     `json:"decide_tool_prune_max_judged"`
	DecideRouteEnabled           bool    `json:"decide_route_enabled"`
	DecideRouteMinRunes          int     `json:"decide_route_min_runes"`
	ContextProjectionEnabled     bool    `json:"context_projection_enabled"`
	ActiveGoalEnabled            bool    `json:"active_goal_enabled"`
	ActiveGoalMaxDepth           int     `json:"active_goal_max_depth"`
	ActiveGoalMaxTextRunes       int     `json:"active_goal_max_text_runes"`
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
		DecideProfileID:              k.DecideProfileID,
		DecideSystemOneBaseURL:       k.DecideSystemOneBaseURL,
		DecideSystemOneAPIKeySet:     strings.TrimSpace(k.DecideSystemOneAPIKey) != "",
		DecideSystemOneModel:         k.DecideSystemOneModel,
		DecideToolRoutingEnabled:     k.DecideToolRoutingEnabled,
		DecideToolThreshold:          k.DecideToolThreshold,
		DecideToolPreTopK:            k.DecideToolPreTopK,
		DecideToolChoiceEnabled:      k.DecideToolChoiceEnabled,
		DecideToolPruneEnabled:       k.DecideToolPruneEnabled,
		DecideToolPruneThreshold:     k.DecideToolPruneThreshold,
		DecideToolPruneMaxJudged:     k.DecideToolPruneMaxJudged,
		DecideRouteEnabled:           k.DecideRouteEnabled,
		DecideRouteMinRunes:          k.DecideRouteMinRunes,
		ContextProjectionEnabled:     k.ContextProjectionEnabled,
		ActiveGoalEnabled:            k.ActiveGoalEnabled,
		ActiveGoalMaxDepth:           k.ActiveGoalMaxDepth,
		ActiveGoalMaxTextRunes:       k.ActiveGoalMaxTextRunes,
	}
}

func (s *Server) handleGetRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	if s.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime_settings_unavailable", "runtime settings not wired")
		return
	}
	view := s.Settings.KnobsView()
	writeJSON(w, http.StatusOK, s.runtimeSettingsResponse(view))
}

func (s *Server) runtimeObservabilityJSON() map[string]any {
	o := runtimecfg.Observability{LogFormat: "text", OtelServiceName: "baize"}
	var logOver, epOver, svcOver bool
	if s.Settings != nil {
		o = s.Settings.Observability()
		logOver, epOver, svcOver = s.Settings.ObservabilityOverridden()
	}
	return map[string]any{
		"log_format":                    o.LogFormat,
		"otel_otlp_endpoint":            o.OtelOTLPEndpoint,
		"otel_service_name":             o.OtelServiceName,
		"otel_enabled":                  o.OtelOTLPEndpoint != "",
		"log_format_overridden":         logOver,
		"otel_otlp_endpoint_overridden": epOver,
		"otel_service_name_overridden":  svcOver,
		"config_restart_hint":           false,
	}
}

func (s *Server) runtimeSettingsResponse(view runtimecfg.KnobsView) map[string]any {
	out := map[string]any{
		"effective":                  knobsToJSON(view.Effective),
		"overridden":                 view.Overridden,
		"public_base_url":            s.Settings.PublicBaseURL(),
		"public_base_url_overridden": s.Settings.PublicBaseURLOverridden(),
	}
	for k, v := range s.runtimeObservabilityJSON() {
		out[k] = v
	}
	return out
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
	if err := s.Settings.ValidateKnobs(patch); err != nil {
		writeError(w, runtimecfg.HTTPStatus(err), "invalid_settings", err.Error())
		return
	}
	touchOtel := patch.ObservabilityReset || patch.OtelOTLPEndpoint != nil || patch.OtelServiceName != nil
	touchLog := patch.LogFormat != nil || patch.ObservabilityReset
	if cfg, ok := s.otelProbeConfig(patch); ok {
		if err := otelx.Probe(r.Context(), cfg); err != nil {
			writeError(w, http.StatusBadRequest, "otel_unavailable", err.Error())
			return
		}
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
	obs := s.Settings.Observability()
	if touchLog {
		applog.Init(obs.LogFormat)
	}
	if touchOtel && s.Otel != nil {
		if err := s.Otel.Apply(r.Context(), otelx.Config{
			Endpoint:    obs.OtelOTLPEndpoint,
			ServiceName: obs.OtelServiceName,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "otel_apply_failed", err.Error())
			return
		}
	}
	view := s.Settings.KnobsView()
	writeJSON(w, http.StatusOK, s.runtimeSettingsResponse(view))
}

// otelProbeConfig returns the OTel config that would apply after patch, and
// whether a Probe is needed (false when patch does not change OTel).
func (s *Server) otelProbeConfig(patch runtimecfg.KnobsPatch) (otelx.Config, bool) {
	cur := s.Settings.Observability()
	cfg := otelx.Config{Endpoint: cur.OtelOTLPEndpoint, ServiceName: cur.OtelServiceName}
	if patch.ObservabilityReset {
		// Best-effort: Probe with empty endpoint always succeeds; real Apply after
		// persist uses baseline. Skip Probe on reset (Apply may still fail → 500).
		return cfg, false
	}
	changed := false
	if patch.OtelOTLPEndpoint != nil {
		n, _ := runtimecfg.ValidateOtelEndpoint(*patch.OtelOTLPEndpoint)
		cfg.Endpoint = n
		changed = true
	}
	if patch.OtelServiceName != nil {
		n := strings.TrimSpace(*patch.OtelServiceName)
		if n == "" {
			n = "baize"
		}
		cfg.ServiceName = n
		changed = true
	}
	return cfg, changed
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
