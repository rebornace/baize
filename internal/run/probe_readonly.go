package run

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"golang.org/x/sync/errgroup"
)

const (
	ProbeReadonlyName   = "probe_readonly"
	EventProbeReadonly  = "probe.readonly"
)

// probeDeniedNames are builtins that must never run inside a readonly probe
// (mutations, activation, proposals, or nested probes).
var probeDeniedNames = map[string]bool{
	ProbeReadonlyName:      true,
	skill.ActivateToolName: true,
	skill.ProposeName:      true,
	skill.UnarchiveName:    true,
	"propose_plugin":       true,
	"schedule_create":      true,
	ContextPinName:         true,
	ContextDropName:        true,
	ContextRewriteName:     true,
	ContextCompactName:     true,
	GoalSetName:      true,
	GoalPushName:     true,
	GoalCompleteName: true,
	GoalUpdateName:   true,
}

// ProbeTools registers the readonly fan-out probe builtin.
func (e *Engine) ProbeTools() []contextToolReg {
	return []contextToolReg{
		{Spec: probeReadonlySpec(), Invoker: e.probeReadonly},
	}
}

func probeReadonlySpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name: ProbeReadonlyName,
		Description: "在同一 Run 内并行调用多个只读工具并合并结果（共享 run_id / binding，不是子代理）。" +
			"Fan out read-only tool calls in this run; results merge into the main context.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"calls": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":      map[string]any{"type": "string"},
							"arguments": map[string]any{"type": "object"},
						},
						"required": []string{"name"},
					},
					"description": "Read-only tool calls to run in parallel (max = parallel_tool_limit)",
				},
			},
			"required": []string{"calls"},
		},
	}
}

type probeCall struct {
	Name string
	Args map[string]any
}

func (e *Engine) probeReadonly(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
	calls, err := parseProbeCalls(args)
	if err != nil {
		return map[string]any{"error": err.Error()}, true, nil
	}
	limit := e.parallelToolLimit()
	if len(calls) == 0 {
		return map[string]any{"error": "calls must be a non-empty array"}, true, nil
	}
	if len(calls) > limit {
		return map[string]any{"error": fmt.Sprintf("at most %d calls allowed", limit)}, true, nil
	}

	runID := identity.RunIDFrom(ctx)
	allowed := e.probeAllowedNames(runID)
	for i, c := range calls {
		if err := e.validateProbeCall(c, allowed); err != nil {
			return map[string]any{"error": fmt.Sprintf("calls[%d]: %v", i, err)}, true, nil
		}
	}

	type outcome struct {
		name    string
		content map[string]any
		isError bool
	}
	out := make([]outcome, len(calls))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(limit)
	var mu sync.Mutex
	names := make([]string, 0, len(calls))

	for i, c := range calls {
		i, c := i, c
		names = append(names, c.Name)
		g.Go(func() error {
			content, isErr, invErr := e.Tools.Invoke(gctx, c.Name, c.Args)
			if invErr != nil {
				content = map[string]any{"error": invErr.Error()}
				isErr = true
			}
			if content == nil {
				content = map[string]any{}
			}
			mu.Lock()
			out[i] = outcome{name: c.Name, content: content, isError: isErr}
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()

	results := make([]map[string]any, 0, len(out))
	for _, o := range out {
		results = append(results, map[string]any{
			"name":     o.name,
			"content":  identity.RedactSensitive(o.content),
			"is_error": o.isError,
		})
	}

	if runID != "" && e.Store != nil {
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventProbeReadonly,
			Data: map[string]any{
				"names": names,
				"count": len(names),
			},
		})
	}
	return map[string]any{"ok": true, "results": results}, false, nil
}

func parseProbeCalls(args map[string]any) ([]probeCall, error) {
	raw, ok := args["calls"]
	if !ok || raw == nil {
		return nil, fmt.Errorf("calls required")
	}
	arr, ok := raw.([]any)
	if !ok {
		// JSON decoders sometimes produce []map[string]any via typed helpers.
		if typed, ok := raw.([]map[string]any); ok {
			out := make([]probeCall, 0, len(typed))
			for _, m := range typed {
				name, _ := m["name"].(string)
				name = strings.TrimSpace(name)
				carg, _ := m["arguments"].(map[string]any)
				if carg == nil {
					carg = map[string]any{}
				}
				out = append(out, probeCall{Name: name, Args: carg})
			}
			return out, nil
		}
		return nil, fmt.Errorf("calls must be an array")
	}
	out := make([]probeCall, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("each call must be an object")
		}
		name, _ := m["name"].(string)
		name = strings.TrimSpace(name)
		carg, _ := m["arguments"].(map[string]any)
		if carg == nil {
			carg = map[string]any{}
		}
		out = append(out, probeCall{Name: name, Args: carg})
	}
	return out, nil
}

func (e *Engine) probeAllowedNames(runID string) map[string]bool {
	allowed := map[string]bool{}
	if runID != "" {
		for _, spec := range e.specsForRun(runID) {
			allowed[spec.Name] = true
		}
	}
	if len(allowed) == 0 && e.Tools != nil {
		for _, spec := range e.Tools.Specs() {
			allowed[spec.Name] = true
		}
	}
	return allowed
}

func (e *Engine) validateProbeCall(c probeCall, allowed map[string]bool) error {
	if c.Name == "" {
		return fmt.Errorf("name required")
	}
	if probeDeniedNames[c.Name] {
		return fmt.Errorf("tool %q is not allowed in probe_readonly", c.Name)
	}
	if strings.HasPrefix(c.Name, "propose_") {
		return fmt.Errorf("tool %q is not allowed in probe_readonly", c.Name)
	}
	if e.Tools == nil {
		return fmt.Errorf("tool registry unavailable")
	}
	resolved := c.Name
	if canon, err := e.Tools.ResolveName(c.Name); err == nil {
		resolved = canon
	} else {
		return fmt.Errorf("unknown tool %q", c.Name)
	}
	if probeDeniedNames[resolved] {
		return fmt.Errorf("tool %q is not allowed in probe_readonly", resolved)
	}
	if !allowed[resolved] && !allowed[c.Name] {
		return fmt.Errorf("tool %q is not in the current run binding", resolved)
	}
	if e.Tools.RequiresApproval(resolved) {
		return fmt.Errorf("tool %q requires approval and cannot be probed", resolved)
	}
	if info, ok := e.Tools.Get(resolved); ok && info.RequireLogin {
		return fmt.Errorf("tool %q requires login and cannot be probed", resolved)
	}
	return nil
}
