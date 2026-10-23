package run

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/middleware"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
)

const (
	SpawnGroupRunName  = "spawn_group_run"
	EventRunSpawned    = "run.spawned"
	EventRunReceipt    = "run.receipt"
	maxConcurrentChild = 2
)

// GroupRunReceipt is the structured child→parent handoff (L2).
type GroupRunReceipt struct {
	Status     string         `json:"status"`
	Summary    string         `json:"summary"`
	Facts      map[string]any `json:"facts,omitempty"`
	ChildRunID string         `json:"child_run_id"`
	GroupID    string         `json:"group_id,omitempty"`
	OpenHITL   bool           `json:"open_hitl,omitempty"`
	Error      string         `json:"error,omitempty"`
}

// DispatchJobFunc enqueues a worker job (set by bootstrap).
type DispatchJobFunc func(job middleware.Job)

// ChildTerminalFunc notifies the parent when a scoped child reaches a terminal status.
type ChildTerminalFunc func(parentRunID string, receipt GroupRunReceipt)

// SpawnTools registers the group-scoped sub-run builtin.
func (e *Engine) SpawnTools() []contextToolReg {
	return []contextToolReg{
		{Spec: spawnGroupRunSpec(), Invoker: e.spawnGroupRun},
	}
}

func spawnGroupRunSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name: SpawnGroupRunName,
		Description: "在指定连接器组内启动 depth=1 作用域子 Run，等待结构化回执。" +
			"Spawn a depth-1 group-scoped child run and wait for a structured receipt. " +
			"Requires settings scoped_subrun_enabled=true.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"group_id": map[string]any{"type": "string", "description": "Connector group id"},
				"goal":     map[string]any{"type": "string", "description": "One-line child goal"},
				"entity_keys": map[string]any{
					"type": "object", "additionalProperties": map[string]any{"type": "string"},
					"description": "Stable entity keys (order id, customer id, …)",
				},
				"mode": map[string]any{
					"type": "string", "enum": []any{store.SpawnModeReadonly, store.SpawnModeReadwrite},
					"description": "readonly rejects approval/login tools",
				},
				"success_criteria": map[string]any{"type": "string"},
				"budget": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"max_steps": map[string]any{"type": "integer"},
					},
				},
			},
			"required": []string{"group_id", "goal"},
		},
	}
}

func (e *Engine) effectiveScopedSubrunEnabled() bool {
	if e.Settings == nil {
		return false
	}
	return e.Settings.Knobs().ScopedSubrunEnabled
}

func (e *Engine) spawnGroupRun(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
	runID := identity.RunIDFrom(ctx)
	callID := identity.ToolCallIDFrom(ctx)

	if !e.effectiveScopedSubrunEnabled() {
		return map[string]any{"error": "scoped_subrun_enabled is false; enable in settings"}, true, nil
	}
	parent, err := e.Store.GetRun(runID)
	if err != nil || parent == nil {
		return map[string]any{"error": "parent run not found"}, true, nil
	}
	if parent.Depth > 0 {
		return map[string]any{"error": "child runs cannot spawn further children (depth=1)"}, true, nil
	}

	groupID, _ := args["group_id"].(string)
	groupID = strings.TrimSpace(groupID)
	goal, _ := args["goal"].(string)
	goal = strings.TrimSpace(goal)
	if groupID == "" || goal == "" {
		return map[string]any{"error": "group_id and goal are required"}, true, nil
	}
	mode := store.SpawnModeReadonly
	if m, ok := args["mode"].(string); ok && strings.TrimSpace(m) != "" {
		mode = strings.TrimSpace(m)
	}
	if mode != store.SpawnModeReadonly && mode != store.SpawnModeReadwrite {
		return map[string]any{"error": "mode must be readonly or readwrite"}, true, nil
	}
	members := e.groupMembers(groupID)
	if len(members) == 0 {
		return map[string]any{"error": "unknown or empty connector group: " + groupID}, true, nil
	}

	children, _ := e.Store.ListChildRuns(runID)
	active := 0
	for _, c := range children {
		switch c.Status {
		case store.StatusSucceeded, store.StatusFailed, store.StatusCancelled, store.StatusRejected:
		default:
			active++
		}
	}
	if active >= maxConcurrentChild {
		return map[string]any{"error": fmt.Sprintf("at most %d concurrent child runs", maxConcurrentChild)}, true, nil
	}

	entityKeys := map[string]string{}
	if raw, ok := args["entity_keys"].(map[string]any); ok {
		for k, v := range raw {
			entityKeys[k] = fmt.Sprint(v)
		}
	}
	criteria, _ := args["success_criteria"].(string)
	maxSteps := 8
	if budget, ok := args["budget"].(map[string]any); ok {
		switch v := budget["max_steps"].(type) {
		case float64:
			if int(v) > 0 {
				maxSteps = int(v)
			}
		case int:
			if v > 0 {
				maxSteps = v
			}
		}
	}
	if maxSteps > e.effectiveMaxSteps() {
		maxSteps = e.effectiveMaxSteps()
	}

	input := buildChildInput(goal, criteria, entityKeys, groupID, mode)
	child, err := e.Store.CreateRun(store.CreateRunInput{
		AgentID:            parent.AgentID,
		Input:              input,
		ConversationID:     parent.ConversationID,
		IdentityID:         parent.IdentityID,
		ModelProfileID:     parent.ModelProfileID,
		ThinkingLevel:      parent.ThinkingLevel,
		PassthroughHeaders: parent.PassthroughHeaders,
		WebhookConfig:      parent.WebhookConfig,
		ParentRunID:        parent.ID,
		Depth:              1,
		GroupID:            groupID,
		SpawnMode:          mode,
	})
	if err != nil {
		return map[string]any{"error": err.Error()}, true, nil
	}
	e.setChildScope(child.ID, childScope{
		GroupID:     groupID,
		Mode:        mode,
		Sources:     members,
		MaxSteps:    maxSteps,
		ParentRunID: parent.ID,
	})

	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventRunSpawned,
		Data: map[string]any{
			"child_run_id": child.ID,
			"group_id":     groupID,
			"mode":         mode,
			"goal":         goal,
		},
	})

	if e.DispatchJob != nil {
		e.DispatchJob(middleware.Job{
			RunID:   child.ID,
			Kind:    middleware.KindRun,
			AgentID: child.AgentID,
			Input:   child.Input,
		})
	}

	_, waitErr := e.awaitHITLDecision(ctx, runID, &store.HITLPayload{
		Prompt:     "等待组作用域子 Run 回执 / waiting for scoped child receipt",
		ToolName:   SpawnGroupRunName,
		Arguments:  args,
		Kind:       store.HITLKindWaitingChild,
		ChildRunID: child.ID,
		ToolCallID: callID,
	})
	if waitErr != nil {
		return nil, false, waitErr
	}
	return nil, false, ErrWaitingHuman
}

func buildChildInput(goal, criteria string, keys map[string]string, groupID, mode string) string {
	var b strings.Builder
	b.WriteString("[scoped-subrun group=")
	b.WriteString(groupID)
	b.WriteString(" mode=")
	b.WriteString(mode)
	b.WriteString("]\nGoal: ")
	b.WriteString(goal)
	if criteria != "" {
		b.WriteString("\nSuccess criteria: ")
		b.WriteString(criteria)
	}
	if len(keys) > 0 {
		b.WriteString("\nEntity keys:")
		for k, v := range keys {
			b.WriteString("\n- ")
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(v)
		}
	}
	b.WriteString("\nComplete the goal using only tools in this connector group. Return a concise factual answer.")
	return b.String()
}

func (e *Engine) groupMembers(groupID string) []string {
	idx := e.connectorGroups
	if idx == nil || groupID == "" {
		return nil
	}
	return append([]string(nil), idx.byGroup[groupID]...)
}

type childScope struct {
	GroupID     string
	Mode        string
	Sources     []string
	MaxSteps    int
	ParentRunID string
}

func (e *Engine) childScopes() *sync.Map {
	// lazy via engine field — use dedicated map on Engine
	if e.childScopeByRun == nil {
		e.childScopeByRun = &sync.Map{}
	}
	return e.childScopeByRun
}

func (e *Engine) setChildScope(runID string, sc childScope) {
	e.childScopes().Store(runID, sc)
}

func (e *Engine) getChildScope(runID string) (childScope, bool) {
	if e.childScopeByRun == nil {
		return childScope{}, false
	}
	v, ok := e.childScopeByRun.Load(runID)
	if !ok {
		return childScope{}, false
	}
	sc, _ := v.(childScope)
	return sc, true
}

func (e *Engine) filterSpecsForChild(runID string, specs []llm.ToolSpec) []llm.ToolSpec {
	sc, ok := e.getChildScope(runID)
	if !ok {
		if e.Store == nil {
			return specs
		}
		// Rebuild from store for cold resume.
		rec, err := e.Store.GetRun(runID)
		if err != nil || rec == nil || rec.Depth == 0 {
			return specs
		}
		members := e.groupMembers(rec.GroupID)
		sc = childScope{GroupID: rec.GroupID, Mode: rec.SpawnMode, Sources: members, ParentRunID: rec.ParentRunID}
		e.setChildScope(runID, sc)
	}
	allowedSrc := map[string]bool{}
	for _, s := range sc.Sources {
		allowedSrc[s] = true
	}
	builtinOK := map[string]bool{
		// Keep a minimal builtin surface; no spawn / activate_skill / probes.
	}
	var out []llm.ToolSpec
	for _, s := range specs {
		if s.Name == SpawnGroupRunName || s.Name == ProbeReadonlyName || s.Name == skill.ActivateToolName {
			continue
		}
		if s.Source == "" {
			if builtinOK[s.Name] {
				out = append(out, s)
			}
			continue
		}
		if allowedSrc[s.Source] {
			out = append(out, s)
		}
	}
	return out
}

// NotifyChildTerminal builds a receipt and wakes the parent (if any).
func (e *Engine) NotifyChildTerminal(child *store.Run) {
	if child == nil || child.ParentRunID == "" {
		return
	}
	switch child.Status {
	case store.StatusSucceeded, store.StatusFailed, store.StatusCancelled, store.StatusRejected:
	default:
		// waiting_human and in-flight: parent stays blocked; resume the child.
		return
	}
	if e.childReceiptOnce == nil {
		e.childReceiptOnce = &sync.Map{}
	}
	if _, loaded := e.childReceiptOnce.LoadOrStore(child.ID, true); loaded {
		return
	}
	receipt := GroupRunReceipt{
		Status:     string(child.Status),
		Summary:    strings.TrimSpace(child.Output),
		ChildRunID: child.ID,
		GroupID:    child.GroupID,
		Error:      child.Error,
	}
	if receipt.Summary == "" && child.Error != "" {
		receipt.Summary = child.Error
	}
	_ = e.Store.AppendEvent(child.ParentRunID, store.Event{
		Type: EventRunReceipt,
		Data: map[string]any{
			"child_run_id": child.ID,
			"status":       receipt.Status,
			"summary":      receipt.Summary,
			"group_id":     receipt.GroupID,
			"error":        receipt.Error,
		},
	})
	if e.OnChildTerminal != nil {
		e.OnChildTerminal(child.ParentRunID, receipt)
	}
}

// ContinueFromChildReceipt injects the spawn tool result and resumes the parent ReAct loop.
func (e *Engine) ContinueFromChildReceipt(ctx context.Context, parentRunID string, receipt GroupRunReceipt) error {
	runRec, err := e.Store.GetRun(parentRunID)
	if err != nil {
		return err
	}
	if runRec.Status != store.StatusWaitingHuman {
		return fmt.Errorf("run not waiting_human")
	}
	payload, err := e.Store.GetHITL(parentRunID)
	if err != nil || payload == nil {
		return fmt.Errorf("no hitl payload")
	}
	if payload.Kind != store.HITLKindWaitingChild {
		return fmt.Errorf("hitl kind is not waiting_child")
	}
	callID := payload.ToolCallID
	if callID == "" {
		callID = lastToolCallID(e.Store, parentRunID)
	}
	raw, _ := json.Marshal(receipt)
	var content map[string]any
	_ = json.Unmarshal(raw, &content)
	if content == nil {
		content = map[string]any{"status": receipt.Status, "child_run_id": receipt.ChildRunID, "summary": receipt.Summary}
	}
	isErr := receipt.Status != string(store.StatusSucceeded)
	e.persistToolResult(parentRunID, callID, SpawnGroupRunName, content, isErr)
	_ = e.Store.UpdateRun(parentRunID, store.StatusRunning, "", "")
	_ = e.Store.SetHITL(parentRunID, nil)

	ag, err := e.Store.GetAgent(runRec.AgentID)
	if err != nil {
		return err
	}
	if e.getRunSkillState(parentRunID) == nil {
		e.beginRunSkills(parentRunID, append([]string(nil), ag.Skills...), ag.System, beginRunInput(runRec.Input))
	}
	sys := e.composeSystem(ag.System, parentRunID)
	sys = e.appendSessionAuthHint(sys, runRec.ConversationID)
	sys = e.appendActiveGoalHint(sys)
	evs, err := e.Store.ListEvents(parentRunID)
	if err != nil {
		return err
	}
	messages := e.buildResumeMessages(sys, runRec.ConversationID, runRec.Input, evs)
	return e.runLoop(ctx, parentRunID, messages)
}
