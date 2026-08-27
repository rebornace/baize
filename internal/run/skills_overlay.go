package run

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/skill"
)

type runSkillState struct {
	activated       []string
	defaultNonEmpty bool
	baseSystem      string

	// workflow pipeline mode: set once when an activated skill carries a
	// workflow.yaml; workflowResults is the template data tree (input +
	// "<step_id>.result" nodes).
	workflowStarted bool
	workflowSkill   string
	workflowResults map[string]any
}

func (e *Engine) ensureRuns() {
	if e.runs == nil {
		e.runs = make(map[string]*runSkillState)
	}
}

func (e *Engine) beginRunSkills(runID string, defaultSkills []string, baseSystem string, input ...map[string]any) {
	e.runMu.Lock()
	defer e.runMu.Unlock()
	e.ensureRuns()

	state := &runSkillState{
		defaultNonEmpty: len(defaultSkills) > 0,
		baseSystem:      baseSystem,
		workflowResults: map[string]any{"input": map[string]any{}},
	}
	if len(input) > 0 && input[0] != nil {
		state.workflowResults["input"] = input[0]
	}
	if e.Skills != nil {
		for _, id := range defaultSkills {
			if _, ok := e.Skills.Get(id); ok {
				state.activated = append(state.activated, id)
			}
		}
	}
	e.runs[runID] = state
}

func (e *Engine) getRunSkillState(runID string) *runSkillState {
	e.runMu.Lock()
	defer e.runMu.Unlock()
	if e.runs == nil {
		return nil
	}
	return e.runs[runID]
}

func (e *Engine) composeSystem(base string, runID string) string {
	if e.Skills == nil {
		return base
	}
	st := e.getRunSkillState(runID)
	var activated []string
	if st != nil {
		activated = append([]string(nil), st.activated...)
		if st.baseSystem != "" {
			base = st.baseSystem
		}
	}
	return skill.ComposeSystem(base, e.Skills, activated)
}

func (e *Engine) appendSessionAuthHint(system, conversationID string) string {
	if !identity.ConversationHasSessionAuth(e.Identities, conversationID) {
		return system
	}
	if strings.Contains(system, identity.SessionAuthReadyHint) {
		return system
	}
	if strings.TrimSpace(system) == "" {
		return identity.SessionAuthReadyHint
	}
	return system + "\n\n" + identity.SessionAuthReadyHint
}

func (e *Engine) enabledToolMap() map[string]bool {
	enabled := make(map[string]bool)
	if e.Tools == nil {
		return enabled
	}
	for _, spec := range e.Tools.Specs() {
		enabled[spec.Name] = true
	}
	return enabled
}

func (e *Engine) specsForRun(runID string) []llm.ToolSpec {
	if e.Tools == nil {
		return nil
	}
	all := e.Tools.Specs()
	if e.Skills == nil || len(e.Skills.List()) == 0 {
		return all
	}

	st := e.getRunSkillState(runID)
	enabled := e.enabledToolMap()
	var visibleNames []string
	if st == nil || !st.defaultNonEmpty {
		visibleNames = skill.VisibleTools(e.Skills, nil, enabled)
	} else if len(st.activated) == 0 {
		visibleNames = nil
	} else {
		visibleNames = skill.VisibleTools(e.Skills, st.activated, enabled)
	}

	// Skills declare guidance tools; connectors added at runtime must still reach the model.
	visibleNames = unionEnabledToolNames(visibleNames, enabled)

	byName := make(map[string]llm.ToolSpec, len(all))
	for _, spec := range all {
		byName[spec.Name] = spec
	}
	out := make([]llm.ToolSpec, 0, len(visibleNames)+1)
	for _, name := range visibleNames {
		if spec, ok := byName[name]; ok {
			out = append(out, spec)
		}
	}
	out = append(out, skill.ActivateToolSpec())
	return out
}

func unionEnabledToolNames(skillNames []string, enabled map[string]bool) []string {
	seen := make(map[string]struct{}, len(skillNames)+len(enabled))
	for _, name := range skillNames {
		seen[name] = struct{}{}
	}
	for name, on := range enabled {
		if on {
			seen[name] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (e *Engine) handleActivateSkill(runID string, args map[string]any) (map[string]any, bool) {
	if e.Skills == nil {
		return map[string]any{"error": "skills catalog not configured"}, true
	}

	ids := parseActivateIDs(args)
	if len(ids) == 0 {
		return map[string]any{"error": "id or ids required"}, true
	}

	e.runMu.Lock()
	defer e.runMu.Unlock()
	e.ensureRuns()
	st := e.runs[runID]
	if st == nil {
		st = &runSkillState{}
		e.runs[runID] = st
	}

	var unknown []string
	var pkgs []skill.Package
	for _, id := range ids {
		pkg, ok := e.Skills.Get(id)
		if !ok {
			unknown = append(unknown, id)
			continue
		}
		pkgs = append(pkgs, pkg)
	}
	if len(unknown) > 0 {
		return map[string]any{
			"error":   fmt.Sprintf("unknown skill id: %v", unknown),
			"unknown": unknown,
		}, true
	}

	seen := make(map[string]struct{}, len(st.activated))
	for _, id := range st.activated {
		seen[id] = struct{}{}
	}
	enabled := make(map[string]bool)
	if e.Tools != nil {
		for _, spec := range e.Tools.Specs() {
			enabled[spec.Name] = true
		}
	}

	var added []string
	for _, pkg := range pkgs {
		if _, dup := seen[pkg.ID]; dup {
			continue
		}
		seen[pkg.ID] = struct{}{}
		st.activated = append(st.activated, pkg.ID)
		for _, toolName := range pkg.Tools {
			if enabled[toolName] {
				added = append(added, toolName)
			}
		}
	}

	available := skill.VisibleTools(e.Skills, st.activated, enabled)

	return map[string]any{
		"activated":       append([]string(nil), st.activated...),
		"added_tools":     added,
		"available_tools": available,
	}, false
}

func parseActivateIDs(args map[string]any) []string {
	var ids []string
	if args == nil {
		return ids
	}
	if id, ok := args["id"].(string); ok && id != "" {
		ids = append(ids, id)
	}
	switch v := args["ids"].(type) {
	case []string:
		ids = append(ids, v...)
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				ids = append(ids, s)
			}
		}
	}
	return ids
}
