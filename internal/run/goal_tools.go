package run

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

const (
	GoalSetName      = "goal_set"
	GoalPushName     = "goal_push"
	GoalCompleteName = "goal_complete"
	GoalUpdateName   = "goal_update"

	EventGoalSet     = "goal.set"
	EventGoalPushed  = "goal.pushed"
	EventGoalPopped  = "goal.popped"
	EventGoalUpdated = "goal.updated"
)

var goalToolNames = map[string]bool{
	GoalSetName:      true,
	GoalPushName:     true,
	GoalCompleteName: true,
	GoalUpdateName:   true,
}

// GoalTools registers agent-facing goal stack editors. They mutate GoalStack
// only (never conversation.Messages) and do not require HITL.
func (e *Engine) GoalTools() []contextToolReg {
	return []contextToolReg{
		{Spec: goalSetSpec(), Invoker: e.goalSet},
		{Spec: goalPushSpec(), Invoker: e.goalPush},
		{Spec: goalCompleteSpec(), Invoker: e.goalComplete},
		{Spec: goalUpdateSpec(), Invoker: e.goalUpdate},
	}
}

func goalSetSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name: GoalSetName,
		Description: "设定或更换本会话主目标（重置为单帧）。text 用一句话短标题；较长背景/约束放 details；可验收条目放 criteria。" +
			"用户目标含糊时可先设简短目标，后续用 goal_update 补全。" +
			"Set/replace the main goal: short title in text, longer context in details.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":     map[string]any{"type": "string", "description": "Short goal title (one sentence)"},
				"details":  map[string]any{"type": "string", "description": "Longer context / constraints (optional)"},
				"criteria": goalCriteriaSchema(),
			},
			"required": []string{"text"},
		},
	}
}

func goalPushSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name: GoalPushName,
		Description: "压入临时焦点（先做别的），做完后用 goal_complete 回到下层。text 保持短；长说明放 details。" +
			"Push a temporary focus; short text, optional details.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":     map[string]any{"type": "string", "description": "Short focus title"},
				"details":  map[string]any{"type": "string", "description": "Longer context (optional)"},
				"criteria": goalCriteriaSchema(),
			},
			"required": []string{"text"},
		},
	}
}

func goalCompleteSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name: GoalCompleteName,
		Description: "完成当前焦点并回到下层目标（pop）。深度为 1 时清空栈。" +
			"Complete the current focus and pop back to the parent goal.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

func goalUpdateSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name: GoalUpdateName,
		Description: "随对话澄清补全当前目标：改短标题、补 details、或改 criteria；不改变栈深度。" +
			"Refine the active tip as the task becomes clearer.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":     map[string]any{"type": "string", "description": "Short title (omit to keep)"},
				"details":  map[string]any{"type": "string", "description": "Details body (omit to keep; empty clears)"},
				"criteria": goalCriteriaSchema(),
			},
		},
	}
}

func goalCriteriaSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"items":       map[string]any{"type": "string"},
		"description": "Optional acceptance criteria (max 8)",
	}
}

func normalizeGoalTitleDetails(text, details string, maxText int) (string, string, error) {
	text = strings.TrimSpace(text)
	details = strings.TrimSpace(details)
	if details == "" && utf8.RuneCountInString(text) > conversation.SoftMaxGoalTitleRunes {
		text, details = conversation.SplitGoalTitleAndDetails(text)
	}
	if text == "" {
		return "", "", fmt.Errorf("text is required")
	}
	if utf8.RuneCountInString(text) > maxText {
		return "", "", fmt.Errorf("text exceeds %d runes", maxText)
	}
	if utf8.RuneCountInString(details) > conversation.DefaultMaxGoalDetailsRunes {
		return "", "", fmt.Errorf("details exceed %d runes", conversation.DefaultMaxGoalDetailsRunes)
	}
	return text, details, nil
}

func (e *Engine) goalSet(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
	if !e.activeGoalEnabled() {
		return failGoal("active goal is disabled")
	}
	conv, errPayload, ok := convFromToolCtx(ctx)
	if !ok {
		return errPayload, true, nil
	}
	text, ok := strArg(args, "text")
	if !ok {
		return failGoal("text is required")
	}
	details, _ := strArg(args, "details")
	text, details, err := normalizeGoalTitleDetails(text, details, e.activeGoalMaxTextRunes())
	if err != nil {
		return failGoal("%v", err)
	}
	criteria, err := optionalCriteriaArg(args)
	if err != nil {
		return failGoal("%v", err)
	}
	if e.Messages == nil {
		return failGoal("message store not configured")
	}
	stack, err := conversation.SetRootDetailed(conversation.GoalStack{}, conv, text, details, criteria, conversation.GoalSourceAgent)
	if err != nil {
		return failGoal("%v", err)
	}
	if err := e.Messages.UpsertGoalStack(stack); err != nil {
		return failGoal("%v", err)
	}
	e.appendGoalToolEvent(ctx, EventGoalSet, stack)
	return goalToolOK(stack), false, nil
}

func (e *Engine) goalPush(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
	if !e.activeGoalEnabled() {
		return failGoal("active goal is disabled")
	}
	conv, errPayload, ok := convFromToolCtx(ctx)
	if !ok {
		return errPayload, true, nil
	}
	text, ok := strArg(args, "text")
	if !ok {
		return failGoal("text is required")
	}
	details, _ := strArg(args, "details")
	text, details, err := normalizeGoalTitleDetails(text, details, e.activeGoalMaxTextRunes())
	if err != nil {
		return failGoal("%v", err)
	}
	criteria, err := optionalCriteriaArg(args)
	if err != nil {
		return failGoal("%v", err)
	}
	if e.Messages == nil {
		return failGoal("message store not configured")
	}
	stack, ok := e.loadGoalStack(conv)
	if !ok || len(stack.Frames) == 0 {
		return failGoal("no main goal yet; call goal_set first")
	}
	stack, err = conversation.PushDetailed(stack, text, details, criteria, conversation.GoalSourceAgent, e.activeGoalMaxDepth())
	if err != nil {
		return failGoal("%v", err)
	}
	if err := e.Messages.UpsertGoalStack(stack); err != nil {
		return failGoal("%v", err)
	}
	e.appendGoalToolEvent(ctx, EventGoalPushed, stack)
	return goalToolOK(stack), false, nil
}

func (e *Engine) goalComplete(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
	if !e.activeGoalEnabled() {
		return failGoal("active goal is disabled")
	}
	conv, errPayload, ok := convFromToolCtx(ctx)
	if !ok {
		return errPayload, true, nil
	}
	if e.Messages == nil {
		return failGoal("message store not configured")
	}
	stack, ok := e.loadGoalStack(conv)
	if !ok || len(stack.Frames) == 0 {
		return failGoal("goal stack is empty")
	}
	stack, err := conversation.Pop(stack)
	if err != nil {
		return failGoal("%v", err)
	}
	if err := e.Messages.UpsertGoalStack(stack); err != nil {
		return failGoal("%v", err)
	}
	e.appendGoalToolEvent(ctx, EventGoalPopped, stack)
	return goalToolOK(stack), false, nil
}

func (e *Engine) goalUpdate(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
	if !e.activeGoalEnabled() {
		return failGoal("active goal is disabled")
	}
	conv, errPayload, ok := convFromToolCtx(ctx)
	if !ok {
		return errPayload, true, nil
	}
	text := ""
	if t, has := strArg(args, "text"); has {
		text = t
	}
	var detailsPtr *string
	if _, has := args["details"]; has {
		d, _ := strArg(args, "details")
		detailsPtr = &d
	}
	var criteria []string
	criteriaSet := false
	if _, has := args["criteria"]; has {
		c, err := optionalCriteriaArg(args)
		if err != nil {
			return failGoal("%v", err)
		}
		if c == nil {
			c = []string{}
		}
		criteria = c
		criteriaSet = true
	}
	if text == "" && !criteriaSet && detailsPtr == nil {
		return failGoal("text, details, or criteria is required")
	}
	if text != "" {
		provided := ""
		if detailsPtr != nil {
			provided = *detailsPtr
		}
		t, d, err := normalizeGoalTitleDetails(text, provided, e.activeGoalMaxTextRunes())
		if err != nil {
			return failGoal("%v", err)
		}
		text = t
		if detailsPtr != nil || d != "" {
			detailsPtr = &d
		}
	} else if detailsPtr != nil {
		if utf8.RuneCountInString(*detailsPtr) > conversation.DefaultMaxGoalDetailsRunes {
			return failGoal("details exceed %d runes", conversation.DefaultMaxGoalDetailsRunes)
		}
	}
	if e.Messages == nil {
		return failGoal("message store not configured")
	}
	stack, ok := e.loadGoalStack(conv)
	if !ok || len(stack.Frames) == 0 {
		return failGoal("goal stack is empty")
	}
	var patchCriteria []string
	if criteriaSet {
		patchCriteria = criteria
	}
	stack, err := conversation.PatchTipDetailed(stack, text, patchCriteria, detailsPtr)
	if err != nil {
		return failGoal("%v", err)
	}
	if err := e.Messages.UpsertGoalStack(stack); err != nil {
		return failGoal("%v", err)
	}
	e.appendGoalToolEvent(ctx, EventGoalUpdated, stack)
	return goalToolOK(stack), false, nil
}

func (e *Engine) loadGoalStack(convID string) (conversation.GoalStack, bool) {
	if e.Messages == nil || convID == "" {
		return conversation.GoalStack{}, false
	}
	return e.Messages.GetGoalStack(convID)
}

func (e *Engine) appendGoalToolEvent(ctx context.Context, typ string, stack conversation.GoalStack) {
	if e.Store == nil {
		return
	}
	runID := identity.RunIDFrom(ctx)
	if runID == "" {
		return
	}
	data := map[string]any{
		"conversation_id": stack.ConversationID,
		"revision":        stack.Revision,
		"source":          string(conversation.GoalSourceAgent),
		"depth":           len(stack.Frames),
	}
	_ = e.Store.AppendEvent(runID, store.Event{Type: typ, Data: data})
}

func goalToolOK(stack conversation.GoalStack) map[string]any {
	out := map[string]any{
		"ok":       true,
		"revision": stack.Revision,
		"depth":    len(stack.Frames),
	}
	if root, ok := conversation.Root(stack); ok {
		out["root"] = root.Text
		if d := strings.TrimSpace(root.Details); d != "" {
			out["root_details"] = d
		}
	}
	if tip, ok := conversation.Tip(stack); ok {
		out["tip"] = tip.Text
		if d := strings.TrimSpace(tip.Details); d != "" {
			out["tip_details"] = d
		}
	}
	return out
}

func optionalCriteriaArg(args map[string]any) ([]string, error) {
	raw, ok := args["criteria"]
	if !ok || raw == nil {
		return nil, nil
	}
	list, ok := stringSliceArg(args, "criteria")
	if !ok {
		// Explicit empty array is valid (clear criteria).
		switch raw.(type) {
		case []any, []string:
			return []string{}, nil
		default:
			return nil, fmt.Errorf("criteria must be an array of strings")
		}
	}
	return list, nil
}

func failGoal(format string, args ...any) (map[string]any, bool, error) {
	return map[string]any{"error": fmt.Sprintf(format, args...)}, true, nil
}

func filterGoalTools(specs []llm.ToolSpec, enabled bool) []llm.ToolSpec {
	if enabled {
		return specs
	}
	out := make([]llm.ToolSpec, 0, len(specs))
	for _, s := range specs {
		if goalToolNames[s.Name] {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (e *Engine) withGoalTools(specs, all []llm.ToolSpec) []llm.ToolSpec {
	enabled := e.activeGoalEnabled()
	specs = filterGoalTools(specs, false)
	if !enabled {
		return specs
	}
	have := make(map[string]bool, len(specs))
	for _, s := range specs {
		have[s.Name] = true
	}
	for _, spec := range all {
		if goalToolNames[spec.Name] && !have[spec.Name] {
			specs = append(specs, spec)
			have[spec.Name] = true
		}
	}
	return specs
}

func (e *Engine) activeGoalMaxDepth() int {
	maxDepth := 8
	if e != nil && e.Settings != nil {
		if k := e.Settings.Knobs().ActiveGoalMaxDepth; k > 0 {
			maxDepth = k
		}
	}
	return maxDepth
}

// isGoalSystemContent reports whether content is an Active Goal inject block.
func isGoalSystemContent(content string) bool {
	return strings.HasPrefix(content, "【当前目标】") || strings.HasPrefix(content, "【主目标】")
}

// refreshActiveGoalMessage replaces or inserts the Goal system block so mid-run
// tool writes are visible on the next Chat turn. Returns a new slice.
func (e *Engine) refreshActiveGoalMessage(messages []llm.Message, conversationID string) []llm.Message {
	block := ""
	if e.activeGoalEnabled() && e.Messages != nil && conversationID != "" {
		if stack, ok := e.Messages.GetGoalStack(conversationID); ok {
			block = goalBlock(stack)
		}
	}
	out := make([]llm.Message, 0, len(messages)+1)
	inserted := false
	for _, m := range messages {
		if m.Role == llm.RoleSystem && isGoalSystemContent(m.Content) {
			if block != "" && !inserted {
				out = append(out, llm.Message{Role: llm.RoleSystem, Content: block})
				inserted = true
			}
			continue
		}
		out = append(out, m)
	}
	if block == "" || inserted {
		return out
	}
	insertAt := 0
	for insertAt < len(out) && out[insertAt].Role == llm.RoleSystem {
		c := out[insertAt].Content
		if strings.HasPrefix(c, "以下是较早对话的滚动摘要") ||
			strings.HasPrefix(c, "必须保留的要点") ||
			strings.Contains(c, "投影笔记") {
			break
		}
		insertAt++
	}
	out = append(out, llm.Message{})
	copy(out[insertAt+1:], out[insertAt:])
	out[insertAt] = llm.Message{Role: llm.RoleSystem, Content: block}
	return out
}
