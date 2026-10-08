package run

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/skill"
)

type runSkillState struct {
	activated       []string
	defaultNonEmpty bool
	baseSystem      string
	// exclusive when true: specsForRun only exposes tools listed on activated
	// skills (plus activate_skill). When false (floor), all enabled tools stay
	// visible and skill tools: is only a DP-2a floor.
	exclusive bool
	// locale selects localized skill bodies (e.g. "en", "zh-CN"). Empty = default.
	locale string

	// workflow pipeline mode: set once when an activated skill carries a
	// workflow.yaml; workflowResults is the template data tree (input +
	// "<step_id>.result" nodes).
	workflowStarted bool
	workflowSkill   string
	workflowResults map[string]any
	contextArmed    bool
}

func (e *Engine) ensureRuns() {
	if e.runs == nil {
		e.runs = make(map[string]*runSkillState)
	}
}

func (e *Engine) beginRunSkills(runID string, defaultSkills []string, baseSystem string, input ...map[string]any) {
	e.beginRunSkillsOpts(runID, defaultSkills, baseSystem, false, "", input...)
}

func (e *Engine) beginRunSkillsOpts(runID string, defaultSkills []string, baseSystem string, exclusive bool, locale string, input ...map[string]any) {
	e.runMu.Lock()
	defer e.runMu.Unlock()
	e.ensureRuns()

	state := &runSkillState{
		defaultNonEmpty: len(defaultSkills) > 0,
		baseSystem:      baseSystem,
		exclusive:       exclusive,
		locale:          locale,
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
	st := e.getRunSkillState(runID)
	locale := ""
	if st != nil {
		locale = st.locale
		if st.baseSystem != "" {
			base = st.baseSystem
		}
	}
	if e.Skills == nil {
		return appendClockHint(base, locale)
	}
	var activated []string
	if st != nil {
		activated = append([]string(nil), st.activated...)
	}
	return appendClockHint(skill.ComposeSystem(base, e.Skills, activated, locale), locale)
}

// appendClockHint gives the model an absolute "now" so it can interpret
// timestamps in tool results (expiry, ranges, "today") instead of claiming it
// cannot judge time from the tool schema alone.
func appendClockHint(system, locale string) string {
	if strings.Contains(system, "当前时间：") || strings.Contains(system, "Current time:") {
		return system
	}
	now := time.Now()
	if isChineseLocale(locale) {
		if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
			now = now.In(loc)
		}
	}
	stamp := now.Format(time.RFC3339)
	var line string
	if isChineseLocale(locale) {
		line = "当前时间：" + stamp + "。解读工具返回或用户提到的日期/时间时，请以此为「现在」，结合结果里的时间字段判断，不要只看工具定义就声称无法判断时间。"
	} else {
		line = "Current time: " + stamp + ". Use this as \"now\" when interpreting dates/times in tool results or user requests; read timestamp fields in results—do not claim you cannot determine time from the tool schema alone."
	}
	if strings.TrimSpace(system) == "" {
		return line
	}
	return system + "\n\n" + line
}

func isChineseLocale(locale string) bool {
	l := strings.ToLower(strings.TrimSpace(locale))
	return l == "" || strings.HasPrefix(l, "zh")
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
		return e.withContextTools(all, all, runID)
	}

	enabled := e.enabledToolMap()
	st := e.getRunSkillState(runID)
	exclusive := st != nil && st.exclusive

	var visibleNames []string
	if exclusive {
		// Allowlist: only tools declared on currently activated skills.
		// Empty activated must stay empty (do not use VisibleTools' empty→all
		// floor fallback — that is for the default non-exclusive path).
		var activated []string
		if st != nil {
			activated = st.activated
		}
		if len(activated) > 0 {
			visibleNames = skill.VisibleTools(e.Skills, activated, enabled)
		}
	} else {
		// Floor mode (default): Skills inject workflow guidance via
		// composeSystem; their `tools:` list is a floor for DP-2a
		// (preserveSkillDeclaredTools), not an allowlist. OpenAPI connectors
		// registered at runtime must remain routing candidates or a default
		// skill such as data-analytics would hide every undeclared operation.
		visibleNames = unionEnabledToolNames(nil, enabled)
	}

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
	return e.withContextTools(out, all, runID)
}

func (e *Engine) withContextTools(specs, all []llm.ToolSpec, runID string) []llm.ToolSpec {
	specs = filterContextTools(specs, false)
	if !e.contextArmed(runID) {
		return specs
	}
	have := make(map[string]bool, len(specs))
	for _, s := range specs {
		have[s.Name] = true
	}
	for _, spec := range all {
		if contextToolNames[spec.Name] && !have[spec.Name] {
			specs = append(specs, spec)
			have[spec.Name] = true
		}
	}
	return specs
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
