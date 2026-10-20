package run

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/memory"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
)

const (
	EventRunBinding         = "run.binding"
	bindingSchemaVersion    = 1
	errBindingPersistFailed = "binding_persist_failed"
)

// persistRunBinding writes the turn-0 dependency closure. On AppendEvent
// failure it finalizes the run as failed and returns a non-nil error so the
// caller must not call the main model.
func (e *Engine) persistRunBinding(runID string, turn int, specs []llm.ToolSpec, messages []llm.Message) error {
	data := e.buildRunBindingData(runID, turn, specs, messages)
	if err := e.Store.AppendEvent(runID, store.Event{Type: EventRunBinding, Data: data}); err != nil {
		_ = e.finalizeFailedRun(runID, fmt.Errorf("%s: %w", errBindingPersistFailed, err))
		return fmt.Errorf("%s: %w", errBindingPersistFailed, err)
	}
	return nil
}

func (e *Engine) buildRunBindingData(runID string, turn int, specs []llm.ToolSpec, messages []llm.Message) map[string]any {
	runRec, _ := e.Store.GetRun(runID)
	agentID := ""
	input := ""
	convID := ""
	profileID := ""
	thinking := ""
	if runRec != nil {
		agentID = runRec.AgentID
		input = runRec.Input
		convID = runRec.ConversationID
		profileID = runRec.ModelProfileID
		thinking = runRec.ThinkingLevel
	}

	toolBinding := ""
	if ag, err := e.Store.GetAgent(agentID); err == nil {
		toolBinding = ag.ToolBinding
	}

	data := map[string]any{
		"schema_version": bindingSchemaVersion,
		"turn":           turn,
		"agent": map[string]any{
			"id":           agentID,
			"tool_binding": toolBinding,
			"skills":       e.runActivatedSkills(runID),
		},
		"model":           e.bindingModel(profileID, thinking),
		"decide":          e.bindingDecide(),
		"knobs":           e.bindingKnobs(),
		"tools_sent":      e.bindingToolsSent(specs),
		"memory_injected": e.memoryInjectedMeta(e.memoryOwner(convID), input),
		"context":         e.bindingContext(runID, messages),
		"catalog":         e.bindingCatalog(specs),
	}
	return data
}

func (e *Engine) runActivatedSkills(runID string) []string {
	st := e.getRunSkillState(runID)
	if st == nil || len(st.activated) == 0 {
		return []string{}
	}
	return append([]string(nil), st.activated...)
}

func (e *Engine) bindingModel(profileID, thinkingOverride string) map[string]any {
	out := map[string]any{
		"profile_id":     profileID,
		"thinking_level": thinkingOverride,
	}
	if profileID == "" || e.Store == nil {
		return out
	}
	p, err := e.Store.GetModelProfile(profileID)
	if err != nil {
		return out
	}
	level := thinkingOverride
	if level == "" {
		level = p.ThinkingLevel
	}
	out["updated_at"] = p.UpdatedAt.UTC().Format(time.RFC3339)
	out["provider"] = p.Provider
	out["base_url"] = p.BaseURL
	out["model"] = p.Model
	out["thinking_level"] = level
	out["context_tokens"] = p.ContextTokens
	out["auto_tier"] = p.AutoTier
	return out
}

func (e *Engine) bindingDecide() map[string]any {
	out := map[string]any{}
	if e.Settings == nil {
		return out
	}
	k := e.Settings.Knobs()
	out["decide_profile_id"] = k.DecideProfileID
	out["systemone_base_url"] = k.DecideSystemOneBaseURL
	out["systemone_model"] = k.DecideSystemOneModel
	out["api_key_set"] = strings.TrimSpace(k.DecideSystemOneAPIKey) != ""
	return out
}

func (e *Engine) bindingKnobs() map[string]any {
	var k runtimecfg.Knobs
	if e.Settings != nil {
		k = e.Settings.Knobs()
	}
	return map[string]any{
		"max_messages":                 k.MaxMessages,
		"max_steps":                    k.MaxSteps,
		"tool_timeout_sec":             int(k.ToolTimeout / time.Second),
		"compaction_enabled":           k.CompactionEnabled,
		"compact_threshold":            k.CompactThreshold,
		"compact_reserve_tokens":       k.CompactReserveTokens,
		"compact_keep_recent":          k.CompactKeepRecent,
		"memory_enabled":               k.MemoryEnabled,
		"memory_auto_extract":          k.MemoryAutoExtract,
		"decide_enabled":               k.DecideEnabled,
		"decide_memory_enabled":        k.DecideMemoryEnabled,
		"decide_tool_routing_enabled":  k.DecideToolRoutingEnabled,
		"decide_tool_threshold":        k.DecideToolThreshold,
		"decide_tool_pre_topk":         k.DecideToolPreTopK,
		"decide_tool_choice_enabled":   k.DecideToolChoiceEnabled,
		"decide_tool_prune_enabled":    k.DecideToolPruneEnabled,
		"decide_tool_prune_threshold":  k.DecideToolPruneThreshold,
		"decide_tool_prune_max_judged": k.DecideToolPruneMaxJudged,
		"decide_route_enabled":         k.DecideRouteEnabled,
		"decide_route_min_runes":       k.DecideRouteMinRunes,
		"context_projection_enabled":   k.ContextProjectionEnabled,
		"context_offload_enabled":      k.ContextOffloadEnabled,
	}
}

func (e *Engine) bindingToolsSent(specs []llm.ToolSpec) []map[string]any {
	out := make([]map[string]any, 0, len(specs))
	for _, sp := range specs {
		row := map[string]any{
			"name":          sp.Name,
			"contract_hash": "",
		}
		if e.Store != nil {
			if t, err := e.Store.GetTool(sp.Name); err == nil {
				row["connector_id"] = t.ConnectorID
				row["method"] = t.Method
				row["path"] = t.Path
				row["operation_id"] = t.OperationID
				row["contract_hash"] = store.ContractHash(t)
			} else {
				// Built-in / non-catalog tools: fingerprint the LLM spec surface.
				row["contract_hash"] = specContractHash(sp)
			}
		}
		out = append(out, row)
	}
	return out
}

func specContractHash(sp llm.ToolSpec) string {
	h := sha256.New()
	_, _ = h.Write([]byte(sp.Name))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(sp.Source))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(sp.Method))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(sp.Path))
	raw, _ := json.Marshal(sp.InputSchema)
	_, _ = h.Write(raw)
	sum := hex.EncodeToString(h.Sum(nil))
	if len(sum) > 16 {
		return sum[:16]
	}
	return sum
}

func (e *Engine) memoryInjectedMeta(ownerID, query string) []map[string]any {
	if !e.effectiveMemoryEnabled() || e.Memory == nil {
		return []map[string]any{}
	}
	ownerID = strings.TrimSpace(ownerID)
	query = strings.TrimSpace(query)
	if ownerID == "" || query == "" {
		return []map[string]any{}
	}
	entries, err := e.Memory.Search(ownerID, query, memory.DefaultTopK)
	if err != nil || len(entries) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(entries))
	n := 0
	const headerRunes = 40 // approximate header budget already applied in memoryBlock
	n = headerRunes
	for _, ent := range entries {
		text := strings.TrimSpace(ent.Text)
		if text == "" {
			continue
		}
		lineN := len([]rune("\n- " + text))
		if n+lineN > memory.DefaultMaxInjectChars {
			break
		}
		n += lineN
		sum := sha256.Sum256([]byte(text))
		out = append(out, map[string]any{
			"id":   ent.ID,
			"hash": hex.EncodeToString(sum[:])[:16],
		})
	}
	return out
}

func (e *Engine) bindingContext(runID string, messages []llm.Message) map[string]any {
	compacted := false
	if evs, err := e.Store.ListEvents(runID); err == nil {
		for _, ev := range evs {
			if ev.Type == EventContextCompacted {
				compacted = true
				break
			}
		}
	}
	_ = messages
	return map[string]any{
		"compacted":        compacted,
		"projection_armed": e.contextArmed(runID),
	}
}

func (e *Engine) bindingCatalog(specs []llm.ToolSpec) map[string]int64 {
	out := map[string]int64{}
	if e.Store == nil {
		return out
	}
	seen := map[string]bool{}
	for _, sp := range specs {
		t, err := e.Store.GetTool(sp.Name)
		if err != nil || t.ConnectorID == "" || seen[t.ConnectorID] {
			continue
		}
		seen[t.ConnectorID] = true
		if c, err := e.Store.GetConnector(t.ConnectorID); err == nil {
			out[t.ConnectorID] = c.CatalogRevision
		}
	}
	return out
}

// toolContractFields returns non-secret audit fields for a catalog tool.
func (e *Engine) toolContractFields(name string) (hash string, revision int64, connectorID string) {
	if e.Store == nil {
		return "", 0, ""
	}
	t, err := e.Store.GetTool(name)
	if err != nil {
		return "", 0, ""
	}
	hash = store.ContractHash(t)
	connectorID = t.ConnectorID
	if connectorID != "" {
		if c, err := e.Store.GetConnector(connectorID); err == nil {
			revision = c.CatalogRevision
		}
	}
	return hash, revision, connectorID
}

// sentToolAudit builds decide.tool_narrow sent entries with contract hashes.
func (e *Engine) sentToolAudit(specs []llm.ToolSpec) []map[string]any {
	out := make([]map[string]any, 0, len(specs))
	for _, sp := range specs {
		hash, _, _ := e.toolContractFields(sp.Name)
		if hash == "" {
			hash = specContractHash(sp)
		}
		out = append(out, map[string]any{"name": sp.Name, "contract_hash": hash})
	}
	return out
}
