package run

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/middleware"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/workflow"
)

const (
	SpawnGroupRunName  = "spawn_group_run"
	EventRunSpawned    = "run.spawned"
	EventRunReceipt    = "run.receipt"
	EventRunAutoWake   = "run.auto_wake"
	EventLaseDecision  = "lase.decision"
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
	// UserParts rehydrates the parent's original user turn (text attachments
	// and images) when continuing the LLM loop. Never part of the wire receipt.
	UserParts []llm.ContentPart `json:"-"`
}

const (
	childReceiptMarker   = "RECEIPT_JSON:"
	childReceiptMaxBytes = 8192
	childFactMaxBytes    = 500
	childFactsMax        = 20
)

// parseChildReceipt extracts the optional structured facts line from a child
// answer. A missing or malformed marker is not an error: the parent still gets
// the plain answer.
func parseChildReceipt(output string) (string, map[string]any) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "", nil
	}
	lines := strings.Split(trimmed, "\n")
	markerAt := -1
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, childReceiptMarker) {
			continue
		}
		if len(line) > childReceiptMaxBytes {
			break
		}
		markerAt = i
		break
	}
	if markerAt < 0 {
		return trimmed, nil
	}
	raw := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[markerAt]), childReceiptMarker))
	var parsed struct {
		Summary string         `json:"summary"`
		Facts   map[string]any `json:"facts"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return trimmed, nil
	}
	summary := strings.TrimSpace(parsed.Summary)
	if summary == "" {
		summary = strings.TrimSpace(strings.Join(lines[:markerAt], "\n"))
	}
	if summary == "" {
		summary = trimmed
	}
	return summary, sanitizeChildFacts(parsed.Facts)
}

func sanitizeChildFacts(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if len(out) >= childFactsMax {
			break
		}
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		switch t := v.(type) {
		case string:
			out[key] = truncateText(t, childFactMaxBytes)
		case bool:
			out[key] = t
		case float64:
			out[key] = t
		default:
			raw, err := json.Marshal(v)
			if err != nil || len(raw) > childFactMaxBytes {
				continue
			}
			out[key] = string(raw)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
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
				"async": map[string]any{
					"type":        "boolean",
					"description": "return immediately and let the child run in the background",
				},
				"success_criteria": map[string]any{"type": "string"},
				"lase": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"confidence": map[string]any{
							"type":        "number",
							"description": "Leader self-confidence 0-1 for handling the task directly",
						},
						"evidence_conflict": map[string]any{
							"type":        "boolean",
							"description": "True when sources disagree or evidence is missing",
						},
						"reason": map[string]any{"type": "string"},
					},
					"description": "Required when run.lase_gate is enabled",
				},
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

// evaluateLaseGate records an advisory confidence/evidence decision. It is
// deliberately model-declared: a confident, non-conflicting task is handled by
// the leader; uncertain or conflicting evidence may spawn one child. The gate
// never approves writes and is off unless run.lase_gate is enabled.
func (e *Engine) evaluateLaseGate(runID string, args map[string]any, groupID, goal string) (bool, map[string]any, error) {
	raw, ok := args["lase"].(map[string]any)
	if !ok {
		return false, nil, fmt.Errorf("lase_gate is enabled: pass lase.confidence, lase.evidence_conflict, lase.reason")
	}
	confidence := 0.0
	switch n := raw["confidence"].(type) {
	case float64:
		confidence = n
	case int:
		confidence = float64(n)
	default:
		return false, nil, fmt.Errorf("lase.confidence must be a number between 0 and 1")
	}
	if confidence < 0 || confidence > 1 {
		return false, nil, fmt.Errorf("lase.confidence must be between 0 and 1")
	}
	conflict, _ := raw["evidence_conflict"].(bool)
	reason, _ := raw["reason"].(string)
	reason = truncateText(strings.TrimSpace(reason), 200)
	threshold := e.LaseConfidenceThreshold
	if threshold <= 0 {
		threshold = 0.8
	}
	skip := !conflict && confidence >= threshold
	action := "spawn"
	if skip {
		action = "skip"
	}
	if e.Store != nil && runID != "" {
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventLaseDecision,
			Data: map[string]any{
				"action":            action,
				"group_id":          groupID,
				"goal":              goal,
				"confidence":        confidence,
				"evidence_conflict": conflict,
				"threshold":         threshold,
				"reason":            reason,
			},
		})
	}
	if skip {
		return true, map[string]any{
			"allow":    false,
			"skipped":  true,
			"reason":   "leader confidence is high and evidence is not conflicting; handle it directly",
			"evidence": reason,
		}, nil
	}
	return false, nil, nil
}

func (e *Engine) spawnGroupRun(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
	runID := identity.RunIDFrom(ctx)
	callID := identity.ToolCallIDFrom(ctx)

	if _, ok := workflow.StepStateFromContext(ctx); ok {
		return map[string]any{"error": "spawn_group_run is not available inside a workflow step; split it into a normal step"}, true, nil
	}

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
	if e.LaseGateEnabled {
		skip, payload, err := e.evaluateLaseGate(runID, args, groupID, goal)
		if err != nil {
			return map[string]any{"error": err.Error()}, true, nil
		}
		if skip {
			return payload, false, nil
		}
	}
	async, _ := args["async"].(bool)
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
			"async":        async,
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

	if async {
		return map[string]any{
			"status":       "started",
			"child_run_id": child.ID,
			"group_id":     groupID,
			"mode":         mode,
			"async":        true,
		}, false, nil
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
	b.WriteString("\nEnd with one line: RECEIPT_JSON: {\"summary\":\"one-line result\",\"facts\":{\"key\":\"value\"}}")
	b.WriteString("\nfacts must contain stable IDs, totals, or error text the parent needs. Never include secrets.")
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
	summary, facts := parseChildReceipt(child.Output)
	goal, async := e.childSpawnInfo(child.ParentRunID, child.ID)
	receipt := GroupRunReceipt{
		Status:     string(child.Status),
		Summary:    summary,
		Facts:      facts,
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
			"facts":        receipt.Facts,
			"group_id":     receipt.GroupID,
			"error":        receipt.Error,
			"async":        async,
		},
	})
	if async {
		e.appendChildReceiptNote(child, goal, receipt)
		e.maybeAutoWakeAsyncChild(child, goal, receipt)
	}
	// Only wake a parent that is actually waiting on this child. Async parents
	// keep running (or may already be terminal); never start a resume for them.
	if p, _ := e.Store.GetHITL(child.ParentRunID); p != nil &&
		p.Kind == store.HITLKindWaitingChild && p.ChildRunID == child.ID {
		if e.OnChildTerminal != nil {
			e.OnChildTerminal(child.ParentRunID, receipt)
		}
	}
}

// childSpawnInfo returns the spawn goal and whether the child was async.
func (e *Engine) childSpawnInfo(parentID, childID string) (string, bool) {
	evs, err := e.Store.ListEvents(parentID)
	if err != nil {
		return "", false
	}
	for _, ev := range evs {
		if ev.Type != EventRunSpawned {
			continue
		}
		if cid, _ := ev.Data["child_run_id"].(string); cid != childID {
			continue
		}
		goal, _ := ev.Data["goal"].(string)
		async, _ := ev.Data["async"].(bool)
		return goal, async
	}
	return "", false
}

// appendChildReceiptNote makes an async child's terminal state visible in the
// conversation even if the parent run already ended.
func (e *Engine) appendChildReceiptNote(child *store.Run, goal string, receipt GroupRunReceipt) {
	if child == nil || e.Messages == nil || child.ParentRunID == "" {
		return
	}
	parent, err := e.Store.GetRun(child.ParentRunID)
	if err != nil || parent == nil || strings.TrimSpace(parent.ConversationID) == "" {
		return
	}
	title := "[子代理完成]"
	switch child.Status {
	case store.StatusFailed:
		title = "[子代理失败]"
	case store.StatusCancelled:
		title = "[子代理已取消]"
	case store.StatusRejected:
		title = "[子代理已拒绝]"
	}
	lines := []string{title}
	if strings.TrimSpace(goal) != "" {
		lines = append(lines, "目标："+strings.TrimSpace(goal))
	}
	if strings.TrimSpace(receipt.Summary) != "" {
		lines = append(lines, "摘要："+strings.TrimSpace(receipt.Summary))
	}
	i := 0
	for k, v := range receipt.Facts {
		if i >= 5 {
			break
		}
		lines = append(lines, "- "+k+": "+shortFact(v))
		i++
	}
	if receipt.Error != "" {
		lines = append(lines, "错误："+receipt.Error)
	}
	_, _ = e.Messages.Append(parent.ConversationID, conversation.Message{
		Role: conversation.RoleSystemNote, Content: strings.Join(lines, "\n"), RunID: parent.ID,
	})
}

func shortFact(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	text := strings.Trim(string(raw), "\"")
	if len(text) > 160 {
		text = text[:160] + "…"
	}
	return text
}

func (e *Engine) effectiveSubagentAsyncWake() bool {
	if e.Settings == nil {
		return false
	}
	return e.Settings.Knobs().SubagentAsyncWakeEnabled
}

// maybeAutoWakeAsyncChild starts one follow-up parent run from an async child's
// receipt when the setting is on and the conversation is completely idle. It
// never interrupts a running or waiting parent, and marks both sides so one
// auto-wake cannot chain into a loop.
func (e *Engine) maybeAutoWakeAsyncChild(child *store.Run, goal string, receipt GroupRunReceipt) {
	if child == nil || !e.effectiveSubagentAsyncWake() || e.DispatchJob == nil {
		return
	}
	if receipt.Status != string(store.StatusSucceeded) {
		return
	}
	parent, err := e.Store.GetRun(child.ParentRunID)
	if err != nil || parent == nil || parent.Depth > 0 || parent.ConversationID == "" {
		return
	}
	switch parent.Status {
	case store.StatusSucceeded, store.StatusFailed, store.StatusCancelled, store.StatusRejected:
	default:
		return
	}
	if e.hasAutoWakeEvent(parent.ID) {
		return
	}
	if active, _ := e.Store.HasActiveRun(parent.ConversationID); active {
		return
	}

	input := autoWakeInput(goal, receipt)
	next, err := e.Store.CreateRun(store.CreateRunInput{
		AgentID:            parent.AgentID,
		Input:              input,
		ConversationID:     parent.ConversationID,
		IdentityID:         parent.IdentityID,
		ModelProfileID:     parent.ModelProfileID,
		ThinkingLevel:      parent.ThinkingLevel,
		PassthroughHeaders: parent.PassthroughHeaders,
		WebhookConfig:      parent.WebhookConfig,
		Origin:             store.RunOriginManual,
	})
	if err != nil {
		return
	}
	_ = e.Store.AppendEvent(parent.ID, store.Event{
		Type: EventRunAutoWake,
		Data: map[string]any{"child_run_id": child.ID, "new_run_id": next.ID},
	})
	_ = e.Store.AppendEvent(next.ID, store.Event{
		Type: EventRunAutoWake,
		Data: map[string]any{"source_child_run_id": child.ID, "source_parent_run_id": parent.ID},
	})
	if e.Messages != nil {
		_, _ = e.Messages.Append(parent.ConversationID, conversation.Message{
			Role:    conversation.RoleSystemNote,
			Content: autoWakeNote(goal, receipt),
			RunID:   next.ID,
		})
	}
	e.DispatchJob(middleware.Job{
		RunID:   next.ID,
		Kind:    middleware.KindRun,
		AgentID: next.AgentID,
		Input:   next.Input,
	})
}

func (e *Engine) hasAutoWakeEvent(runID string) bool {
	evs, err := e.Store.ListEvents(runID)
	if err != nil {
		return false
	}
	for _, ev := range evs {
		if ev.Type == EventRunAutoWake {
			return true
		}
	}
	return false
}

func autoWakeInput(goal string, receipt GroupRunReceipt) string {
	var b strings.Builder
	b.WriteString("[子代理回执]")
	if goal != "" {
		b.WriteString(" 目标：" + goal + "。")
	}
	b.WriteString(" 状态：" + receipt.Status + "。")
	if receipt.Summary != "" {
		b.WriteString(" 摘要：" + receipt.Summary + "。")
	}
	if len(receipt.Facts) > 0 {
		b.WriteString(" 事实：")
		i := 0
		for k, v := range receipt.Facts {
			if i >= 6 {
				break
			}
			if i > 0 {
				b.WriteString("；")
			}
			b.WriteString(k + "=" + shortFact(v))
			i++
		}
		b.WriteString("。")
	}
	if receipt.Error != "" {
		b.WriteString(" 错误：" + receipt.Error + "。")
	}
	b.WriteString("请根据回执继续处理；如果没有需要继续的动作，简要告知用户。")
	return b.String()
}

func autoWakeNote(goal string, receipt GroupRunReceipt) string {
	summary := strings.TrimSpace(receipt.Summary)
	if summary == "" {
		summary = "子代理已完成"
	}
	if goal != "" {
		return "[子代理完成，自动继续] " + goal + "：" + summary
	}
	return "[子代理完成，自动继续] " + summary
}

// notifyParentChildWaiting tells the parent that a child needs a human. The
// parent stays waiting_child; the event only drives the parent card.
func (e *Engine) notifyParentChildWaiting(runID string, payload *store.HITLPayload) {
	if payload == nil || payload.Kind == store.HITLKindWaitingChild {
		return
	}
	rec, err := e.Store.GetRun(runID)
	if err != nil || rec == nil || rec.Depth != 1 || rec.ParentRunID == "" {
		return
	}
	if e.parentChildWaitOpen(rec.ParentRunID, runID, payload.Kind, payload.ToolName) {
		return
	}
	_ = e.Store.AppendEvent(rec.ParentRunID, store.Event{
		Type: "run.child_waiting",
		Data: map[string]any{
			"child_run_id": runID,
			"kind":         payload.Kind,
			"tool_name":    payload.ToolName,
			"prompt":       payload.Prompt,
			"connector_id": payload.ConnectorID,
		},
	})
}

// notifyParentChildResumed clears the parent card's waiting state after the
// child actually returns to running.
func (e *Engine) notifyParentChildResumed(runID string) {
	rec, err := e.Store.GetRun(runID)
	if err != nil || rec == nil || rec.Depth != 1 || rec.ParentRunID == "" {
		return
	}
	_ = e.Store.AppendEvent(rec.ParentRunID, store.Event{
		Type: "run.child_resumed",
		Data: map[string]any{"child_run_id": runID},
	})
}

// parentChildWaitOpen reports whether the parent already carries this exact
// open child wait, so repeated persistence does not spam the event stream.
func (e *Engine) parentChildWaitOpen(parentID, childID, kind, toolName string) bool {
	evs, err := e.Store.ListEvents(parentID)
	if err != nil {
		return false
	}
	open := map[string]string{}
	for _, ev := range evs {
		cid, _ := ev.Data["child_run_id"].(string)
		if cid == "" {
			continue
		}
		switch ev.Type {
		case "run.child_waiting":
			k, _ := ev.Data["kind"].(string)
			t, _ := ev.Data["tool_name"].(string)
			open[cid] = k + "\x00" + t
		case "run.child_resumed", EventRunReceipt:
			delete(open, cid)
		}
	}
	return open[childID] == kind+"\x00"+toolName
}

// ContinueFromChildReceipt injects the spawn tool result and resumes the parent ReAct loop.
func (e *Engine) ContinueFromChildReceipt(ctx context.Context, parentRunID string, receipt GroupRunReceipt) error {
	runRec, err := e.Store.GetRun(parentRunID)
	if err != nil {
		return err
	}
	if runRec.Status != store.StatusWaitingHuman {
		return ErrHITLAlreadySettled
	}
	payload, err := e.Store.GetHITL(parentRunID)
	if err != nil || payload == nil {
		return fmt.Errorf("no hitl payload")
	}
	if payload.Kind != store.HITLKindWaitingChild {
		return fmt.Errorf("hitl kind is not waiting_child")
	}
	claimed, cerr := e.Store.BeginHITLResume(parentRunID)
	if cerr != nil {
		return cerr
	}
	if !claimed {
		return ErrHITLAlreadySettled
	}
	defer e.releaseHITLResumeOnError(parentRunID)
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
		e.beginRunSkills(parentRunID, append([]string(nil), ag.Skills...), ag.System, beginRunInput(runRec.Input, nil))
	}
	sys := e.composeSystem(ag.System, parentRunID)
	sys = e.appendSessionAuthHint(sys, runRec.ConversationID)
	sys = e.appendActiveGoalHint(sys)
	evs, err := e.Store.ListEvents(parentRunID)
	if err != nil {
		return err
	}
	messages := e.buildResumeMessagesWithParts(parentRunID, sys, runRec.ConversationID, runRec.Input, evs, receipt.UserParts)
	return e.runLoop(ctx, parentRunID, messages)
}
