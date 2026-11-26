package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const runRetentionSettingKey = "run_retention_days"
const runRetentionConvPrefix = "run_retention_days:"
const runRetentionAgentPrefix = "run_retention_days_agent:"
const defaultRunRetentionInterval = 6 * time.Hour
const runRetentionBatchSize = 500
const runRetentionMaxPerPass = 5000

// runRetentionMaxOverrides matches the store-side exclusion cap. Above it the
// cleanup switches to per-conversation sweeps to stay correct.
const runRetentionMaxOverrides = 5000

var runRetentionMu sync.Mutex

func (s *Server) runRetentionDays() int {
	if s.Store == nil {
		return 0
	}
	raw, ok, err := s.Store.GetSetting(runRetentionSettingKey)
	if err != nil || !ok {
		return 0
	}
	var days int
	if json.Unmarshal(raw, &days) != nil || days < 0 || days > 3650 {
		return 0
	}
	return days
}

// runRetentionDaysFor resolves conversation > workspace > global for display.
// -1 or a missing value inherits the next level; 0 means forever.
func (s *Server) runRetentionDaysFor(convID string, global int) int {
	if days, ok := s.settingRetentionDays(runRetentionConvPrefix + convID); ok {
		return days
	}
	if ms := s.metaStore(); ms != nil {
		if meta, err := ms.GetMeta(convID); err == nil {
			if days := s.workspaceRetentionSetting(runRetentionWorkspacePrefix, meta.WorkspaceID); days >= 0 {
				return days
			}
		}
	}
	return global
}

// runRetentionDaysForAgent returns an explicit agent override, if any.
func (s *Server) runRetentionDaysForAgent(agentID string) (int, bool) {
	return s.settingRetentionDays(runRetentionAgentPrefix + agentID)
}

func (s *Server) handleGetRunRetention(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"days": s.runRetentionDays()})
}

func (s *Server) handlePutRunRetention(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Days int `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.Days < 0 || body.Days > 3650 {
		writeError(w, http.StatusBadRequest, "invalid_request", "days must be between 0 and 3650")
		return
	}
	raw, _ := json.Marshal(body.Days)
	if err := s.Store.UpsertSetting(runRetentionSettingKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": body.Days})
}

// handleGetConversationRunRetention returns the override and effective policy
// for one conversation (-1 = inherit global).
func (s *Server) handleGetConversationRunRetention(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if err := s.checkConversationAccess(r.Context(), convID); err != nil {
		if errors.Is(err, errConversationForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	global := s.runRetentionDays()
	override := -1
	if raw, ok, err := s.Store.GetSetting(runRetentionConvPrefix + convID); err == nil && ok {
		var v int
		if json.Unmarshal(raw, &v) == nil && v >= -1 && v <= 3650 {
			override = v
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days":           override,
		"effective_days": s.runRetentionDaysFor(convID, global),
		"global_days":    global,
	})
}

func (s *Server) handlePutConversationRunRetention(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if err := s.checkConversationAccess(r.Context(), convID); err != nil {
		if errors.Is(err, errConversationForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	var body struct {
		Days int `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.Days < -1 || body.Days > 3650 {
		writeError(w, http.StatusBadRequest, "invalid_request", "days must be between -1 and 3650")
		return
	}
	raw, _ := json.Marshal(body.Days)
	if err := s.Store.UpsertSetting(runRetentionConvPrefix+convID, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	global := s.runRetentionDays()
	writeJSON(w, http.StatusOK, map[string]any{
		"days":           body.Days,
		"effective_days": s.runRetentionDaysFor(convID, global),
		"global_days":    global,
	})
}

type agentRunRetentionJSON struct {
	AgentID string `json:"agent_id"`
	Days    int    `json:"days"`
}

func (s *Server) handleGetAgentRunRetention(w http.ResponseWriter, r *http.Request) {
	global := s.runRetentionDays()
	agents := s.Store.ListAgents()
	items := make([]agentRunRetentionJSON, 0, len(agents))
	for _, ag := range agents {
		days := -1
		if v, ok := s.runRetentionDaysForAgent(ag.ID); ok {
			days = v
		}
		items = append(items, agentRunRetentionJSON{AgentID: ag.ID, Days: days})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"global_days": global,
		"items":       items,
	})
}

func (s *Server) handlePutAgentRunRetention(w http.ResponseWriter, r *http.Request) {
	var body agentRunRetentionJSON
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.AgentID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "agent_id is required")
		return
	}
	if body.Days < -1 || body.Days > 3650 {
		writeError(w, http.StatusBadRequest, "invalid_request", "days must be between -1 and 3650")
		return
	}
	if _, err := s.Store.GetAgent(body.AgentID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "agent not found")
		return
	}
	raw, _ := json.Marshal(body.Days)
	if err := s.Store.UpsertSetting(runRetentionAgentPrefix+body.AgentID, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, body)
}

// cleanupExpiredRuns deletes terminal runs older than the configured window.
// It never touches queued/running/waiting_human runs. Precedence is
// conversation override > agent override > global; 0 means forever.
func (s *Server) cleanupExpiredRuns() (int, error) {
	if s.Store == nil {
		return 0, nil
	}
	global := s.runRetentionDays()
	now := time.Now().UTC()

	convSettings, err := s.Store.ListSettingsByPrefix(runRetentionConvPrefix)
	if err != nil {
		return 0, err
	}
	convDays := parseRetentionOverrides(convSettings, runRetentionConvPrefix)
	agentSettings, err := s.Store.ListSettingsByPrefix(runRetentionAgentPrefix)
	if err != nil {
		return 0, err
	}
	agentDays := parseRetentionOverrides(agentSettings, runRetentionAgentPrefix)
	workspaceDays := s.workspaceRetentionDays(runRetentionWorkspacePrefix)
	workspaceByConv := s.workspaceByConversation()

	workspaceConvs := map[string]bool{}
	convsByWorkspace := map[string][]string{}
	for convID, wsID := range workspaceByConv {
		if _, ok := workspaceDays[wsID]; !ok {
			continue
		}
		workspaceConvs[convID] = true
		convsByWorkspace[wsID] = append(convsByWorkspace[wsID], convID)
	}

	total := 0
	// Very large override sets cannot fit the exclusion lists. With agent
	// overrides a per-conversation sweep cannot resolve per-run precedence, so
	// skip this pass rather than risk deleting runs that should be kept.
	if len(convDays)+len(agentDays)+len(workspaceConvs) > runRetentionMaxOverrides {
		if len(agentDays) > 0 {
			log.Printf("run retention: %d overrides exceed the exclusion limit; skipping pass",
				len(convDays)+len(agentDays)+len(workspaceConvs))
			return 0, nil
		}
		convs, cerr := s.Store.ListRunConversationIDs()
		if cerr != nil {
			return total, cerr
		}
		for _, convID := range convs {
			days := resolveRunRetentionDays(convID, global, convDays, workspaceDays, workspaceByConv)
			n, derr := s.deleteRunsForConversation(convID, days, now)
			if derr != nil {
				return total, derr
			}
			total += n
			if total >= runRetentionMaxPerPass {
				break
			}
		}
		if global > 0 && total < runRetentionMaxPerPass {
			cutoff := now.AddDate(0, 0, -global)
			for {
				n, derr := s.Store.DeleteTerminalRunsBeforeExcluding(cutoff, runRetentionBatchSize, convs, nil)
				if derr != nil {
					return total, derr
				}
				total += n
				if n < runRetentionBatchSize || total >= runRetentionMaxPerPass {
					break
				}
			}
		}
		logRunRetention(total)
		return total, nil
	}

	scopedSet := map[string]bool{}
	scopedConvs := make([]string, 0, len(convDays)+len(workspaceConvs))
	for convID, days := range convDays {
		scopedSet[convID] = true
		scopedConvs = append(scopedConvs, convID)
		if days <= 0 {
			continue
		}
		n, derr := s.deleteRunsForConversation(convID, days, now)
		if derr != nil {
			return total, derr
		}
		total += n
		if total >= runRetentionMaxPerPass {
			logRunRetention(total)
			return total, nil
		}
	}

	// Workspace overrides apply to its conversations unless the conversation
	// has its own explicit override.
	for wsID, days := range workspaceDays {
		if days <= 0 {
			continue
		}
		for _, convID := range convsByWorkspace[wsID] {
			if _, has := convDays[convID]; has {
				continue
			}
			n, derr := s.deleteRunsForConversation(convID, days, now)
			if derr != nil {
				return total, derr
			}
			total += n
			if total >= runRetentionMaxPerPass {
				logRunRetention(total)
				return total, nil
			}
		}
	}
	for convID := range workspaceConvs {
		if !scopedSet[convID] {
			scopedSet[convID] = true
			scopedConvs = append(scopedConvs, convID)
		}
	}

	scopedAgents := make([]string, 0, len(agentDays))
	for agentID, days := range agentDays {
		scopedAgents = append(scopedAgents, agentID)
		if days <= 0 {
			continue
		}
		n, derr := s.deleteRunsForAgent(agentID, days, now, scopedConvs)
		if derr != nil {
			return total, derr
		}
		total += n
		if total >= runRetentionMaxPerPass {
			logRunRetention(total)
			return total, nil
		}
	}

	if global > 0 && total < runRetentionMaxPerPass {
		cutoff := now.AddDate(0, 0, -global)
		for {
			n, derr := s.Store.DeleteTerminalRunsBeforeExcluding(
				cutoff, runRetentionBatchSize, scopedConvs, scopedAgents,
			)
			if derr != nil {
				return total, derr
			}
			total += n
			if n < runRetentionBatchSize || total >= runRetentionMaxPerPass {
				break
			}
		}
	}
	logRunRetention(total)
	return total, nil
}

// deleteRunsForConversation removes terminal runs for one conversation at the
// given retention window. days <= 0 keeps everything.
func (s *Server) deleteRunsForConversation(convID string, days int, now time.Time) (int, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := now.AddDate(0, 0, -days)
	total := 0
	for {
		n, err := s.Store.DeleteTerminalRunsBeforeForConversation(convID, cutoff, runRetentionBatchSize)
		if err != nil {
			return total, err
		}
		total += n
		if n < runRetentionBatchSize {
			return total, nil
		}
	}
}

func (s *Server) deleteRunsForAgent(
	agentID string, days int, now time.Time, excludeConversations []string,
) (int, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := now.AddDate(0, 0, -days)
	total := 0
	for {
		n, err := s.Store.DeleteTerminalRunsBeforeForAgent(
			agentID, cutoff, runRetentionBatchSize, excludeConversations,
		)
		if err != nil {
			return total, err
		}
		total += n
		if n < runRetentionBatchSize {
			return total, nil
		}
	}
}

func logRunRetention(total int) {
	if total > 0 {
		log.Printf("run retention: removed %d terminal runs", total)
	}
}

// resolveRunRetentionDays applies conversation > workspace > agent > global.
// Agent overrides are handled by their own sweep, so this helper stops at the
// workspace level.
func resolveRunRetentionDays(
	convID string, global int, convDays, workspaceDays map[string]int,
	workspaceByConv map[string]string,
) int {
	if days, ok := convDays[convID]; ok {
		return days
	}
	if wsID, ok := workspaceByConv[convID]; ok {
		if days, ok := workspaceDays[wsID]; ok {
			return days
		}
	}
	return global
}

// parseRetentionOverrides keeps only explicit per-conversation values (>= 0);
// -1 and malformed settings inherit the global policy.
func parseRetentionOverrides(settings map[string][]byte, prefix string) map[string]int {
	out := make(map[string]int, len(settings))
	for key, raw := range settings {
		convID := strings.TrimPrefix(key, prefix)
		if convID == "" || convID == key {
			continue
		}
		var days int
		if json.Unmarshal(raw, &days) != nil || days < 0 || days > 3650 {
			continue
		}
		out[convID] = days
	}
	return out
}

// StartRunRetention runs an immediate cleanup and then repeats until ctx ends.
func (s *Server) StartRunRetention(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultRunRetentionInterval
	}
	run := func() {
		if !runRetentionMu.TryLock() {
			return
		}
		defer runRetentionMu.Unlock()
		if _, err := s.cleanupExpiredRuns(); err != nil {
			log.Printf("run retention: cleanup failed: %v", err)
		}
	}
	run()
	for {
		interval = s.retentionInterval(interval)
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			run()
		}
	}
}
