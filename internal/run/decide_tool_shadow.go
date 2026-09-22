package run

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

// buildToolProbe assembles a cheap description of the candidate tools: name
// plus the first line of the description only. It never includes InputSchema;
// the probe must stay far cheaper than the prefill it might one day save.
func buildToolProbe(specs []llm.ToolSpec) string {
	var b strings.Builder
	for _, s := range specs {
		firstLine := strings.TrimSpace(s.Description)
		if i := strings.IndexByte(firstLine, '\n'); i >= 0 {
			firstLine = strings.TrimSpace(firstLine[:i])
		}
		b.WriteString("- ")
		b.WriteString(s.Name)
		b.WriteString(": ")
		b.WriteString(firstLine)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// toolNames returns the candidate tool names in spec order.
func toolNames(specs []llm.ToolSpec) []string {
	names := make([]string, 0, len(specs))
	for _, s := range specs {
		names = append(names, s.Name)
	}
	return names
}

// recordToolShadow consults the decision layer for which tools to keep and
// writes a decide.tool_shadow event. In shadow mode this never filters the
// specs the caller sends; it only records the layer's pick so recall can be
// measured before enforce mode is ever enabled.
//
// OnFail is VerdictYes: if every implementation abstains, the answer carries
// no narrowed list, and the caller keeps sending the full set (today's
// behavior).
func (e *Engine) recordToolShadow(ctx context.Context, runID string, turn int, specs []llm.ToolSpec, messages []llm.Message) {
	// The decision model must see what the user actually wants; without it
	// there is no basis to drop tools and it returns everything. The request
	// is capped (cheap probe) and followed by the name+description candidates.
	userRequest := ""
	if runRec, err := e.Store.GetRun(runID); err == nil && runRec != nil {
		userRequest = strings.TrimSpace(runRec.Input)
		if r := []rune(userRequest); len(r) > decideProbeMaxRunes {
			userRequest = string(r[:decideProbeMaxRunes])
		}
	}

	// Trajectory-aware narrowing: after the first ReAct step the decision
	// model must also see the compact trail of what has already been tried and
	// what it returned, so the candidate set can follow the plan as it unfolds
	// (e.g. product already listed, now fetch its SKU stock). Without this a
	// turn-0 pick can never cover tools the model discovers mid-execution.
	trajectory := buildCompactTrajectory(messages, decideTrajectoryMaxRunes)

	// Deterministic keyword prefilter first: narrow the catalog to a short,
	// high-recall candidate list so the cheap decision model only scans a
	// handful of tools. The query fed to keyword matching includes both the
	// original request and the executed trail, so tools surfaced by prior steps
	// (not named in the original request) still enter the candidate list.
	// Shadow-only, so the full specs the main model receives are untouched;
	// the prefilter set itself is recorded so its recall can be measured
	// independently of the model's final pick.
	matchQuery := userRequest
	if trajectory != "" {
		matchQuery = userRequest + "\n" + trajectory
	}
	preSpecs, noMatch := prefilterTools(matchQuery, specs, e.effectiveDecidePreTopK())
	preNames := toolNames(preSpecs)

	decisionContext := "用户这一轮请求：\n" + userRequest + "\n"
	if trajectory != "" {
		decisionContext += "\n到此为止的执行轨迹（精简）：\n" + trajectory + "\n"
	}
	decisionContext += "\n候选工具（名称: 说明首行）：\n" + buildToolProbe(preSpecs)
	ans := decide.Answer{}
	if !noMatch {
		ans, _ = e.Decider.Ask(ctx, decide.Question{
			Kind:    decide.KindToolCandidates,
			Context: decisionContext,
			Options: preNames,
			OnFail:  decide.VerdictYes,
			TraceID: runID,
		})
	}

	kept := validCandidateNames(ans.Values, specs)
	data := map[string]any{
		"turn":            turn,
		"total":           len(specs),
		"prefilter":       preNames,
		"prefilter_count": len(preNames),
		"prefilter_empty": noMatch,
		"kept":            kept,
		"kept_count":      len(kept),
		"source":          ans.Source,
		"degraded":        ans.Degraded,
	}
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventDecideToolShadow,
		Data: data,
	})
}

// decideTrajectoryMaxRunes caps the compact execution trail appended to the
// decision probe. The trail is a cheap hint about where the plan is, not a
// replay of full tool payloads (those can be large), so each call and result
// is truncated hard.
const decideTrajectoryMaxRunes = 800

const decideTrajectoryItemRunes = 120

// buildCompactTrajectory renders the already-executed portion of the ReAct
// loop as a short, ordered list:
//
//	-> tool_a(args hint)
//	<- short result
//
// Only assistant tool calls and the corresponding tool messages are emitted;
// plain prose is skipped. The whole trail is capped to keep the probe cheap.
// On the first step there is no trail and it returns "".
func buildCompactTrajectory(messages []llm.Message, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	// Pre-pass: index every tool result. Results always appear AFTER the
	// assistant tool-call message, so a single forward pass would emit the
	// call before its result exists.
	resultByID := make(map[string]string)
	for _, m := range messages {
		if m.Role == llm.RoleTool && m.ToolCallID != "" {
			r := []rune(strings.TrimSpace(m.Content))
			if len(r) > decideTrajectoryItemRunes {
				r = r[:decideTrajectoryItemRunes]
			}
			resultByID[m.ToolCallID] = string(r)
		}
	}

	var lines []string
	for _, m := range messages {
		if m.Role != llm.RoleAssistant {
			continue
		}
		for _, tc := range m.ToolCalls {
			lines = append(lines, "-> "+tc.Name+"("+compactArgs(tc.Arguments)+")")
			if res, ok := resultByID[tc.ID]; ok {
				lines = append(lines, "<- "+res)
			}
		}
	}

	var b strings.Builder
	for _, ln := range lines {
		if b.Len() > 0 && b.Len()+len(ln)+1 > maxRunes {
			break
		}
		b.WriteString(ln)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// compactArgs renders a tool call's arguments as a short key=value hint so the
// decision model can see WHAT was queried (e.g. the SKU id) without carrying
// the full payload.
func compactArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := strings.TrimSpace(fmt.Sprintf("%v", args[k]))
		if r := []rune(v); len(r) > 40 {
			v = string(r[:40])
		}
		parts = append(parts, k+"="+v)
	}
	s := strings.Join(parts, ", ")
	if r := []rune(s); len(r) > decideTrajectoryItemRunes {
		s = string(r[:decideTrajectoryItemRunes])
	}
	return s
}

// validCandidateNames restricts the layer's pick to names that actually exist
// and removes duplicates, preserving spec order, so a hallucinated tool name
// can never pollute the recorded set.
func validCandidateNames(picked []string, specs []llm.ToolSpec) []string {
	allowed := make(map[string]bool, len(specs))
	for _, s := range specs {
		allowed[s.Name] = true
	}
	seen := make(map[string]bool, len(picked))
	kept := make([]string, 0, len(picked))
	for _, name := range picked {
		if !allowed[name] || seen[name] {
			continue
		}
		seen[name] = true
		kept = append(kept, name)
	}
	return kept
}
