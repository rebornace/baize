package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

const (
	EventRunStarted   = "run.started"
	EventLLMToolCall  = "llm.tool_call"
	EventToolResult   = "tool.result"
	EventLLMMessage   = "llm.message"
	EventLLMError     = "llm.error"
	EventHITLWaiting  = "hitl.waiting"
	EventHITLResumed  = "hitl.resumed"
	EventHITLRejected = "hitl.rejected"
	EventRunCancelled = "run.cancelled"

	DefaultToolTimeout = 60 * time.Second
)

type Engine struct {
	Store    store.Store
	LLM      llm.Provider
	Tools    *tool.Registry
	Gate     *Gate
	MaxSteps int // default 16
	// ToolTimeout bounds a single Tools.Invoke (default 60s).
	ToolTimeout time.Duration
	// Messages optionally persists conversation history across runs.
	// nil = legacy behavior (no cross-run message persistence). When non-nil,
	// Execute injects a windowed history into the LLM prompt and the engine
	// records terminal assistant / system_note messages on succeeded / failed.
	Messages    conversation.Store
	MaxMessages int // conversation window size; config Load defaults <=0 to 40
	// Identities is optional. When non-nil, tools with RequireLogin are gated
	// before HITL / Invoke when the run has a conversation_id.
	Identities identity.Store
	// Skills is optional. When non-nil and non-empty, runLoop filters tool
	// specs by activated skills and exposes activate_skill.
	Skills *skill.Catalog

	runMu sync.Mutex
	runs  map[string]*runSkillState

	cancelMu sync.Mutex
	cancels  map[string]context.CancelFunc
}

func (e *Engine) Execute(ctx context.Context, runID string, ag agent.Def, input string) error {
	runRec, err := e.Store.GetRun(runID)
	if err != nil {
		return err
	}
	if runRec == nil {
		return fmt.Errorf("run not found: %s", runID)
	}
	ctx, cancel := context.WithCancel(ctx)
	e.registerCancel(runID, cancel)
	defer e.clearCancel(runID)

	ctx = e.injectAuthCtxFromRun(ctx, runRec)
	if err := e.ensureRunStarted(runID); err != nil {
		return err
	}
	e.beginRunSkills(runID, ag.Skills, ag.System)
	sys := e.composeSystem(ag.System, runID)
	sys = e.appendSessionAuthHint(sys, runRec.ConversationID)
	messages := e.buildMessages(sys, runRec.ConversationID, input)
	err = e.runLoop(ctx, runID, messages)
	if errors.Is(err, context.Canceled) {
		e.markCancelled(runID)
		return err
	}
	return err
}

// Cancel requests cooperative cancellation of an active run.
func (e *Engine) Cancel(runID string) error {
	runRec, err := e.Store.GetRun(runID)
	if err != nil || runRec == nil {
		return fmt.Errorf("run not found")
	}
	switch runRec.Status {
	case store.StatusQueued, store.StatusRunning, store.StatusWaitingHuman:
	default:
		return fmt.Errorf("run is not active")
	}

	e.cancelMu.Lock()
	cancel := e.cancels[runID]
	e.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
	// Mark cancelled immediately so UI / HasActiveRun unblock even if the
	// worker is blocked outside a cancellable call.
	e.markCancelled(runID)
	return nil
}

func (e *Engine) registerCancel(runID string, cancel context.CancelFunc) {
	e.cancelMu.Lock()
	defer e.cancelMu.Unlock()
	if e.cancels == nil {
		e.cancels = make(map[string]context.CancelFunc)
	}
	if prev, ok := e.cancels[runID]; ok {
		prev()
	}
	e.cancels[runID] = cancel
}

func (e *Engine) clearCancel(runID string) {
	e.cancelMu.Lock()
	defer e.cancelMu.Unlock()
	delete(e.cancels, runID)
}

func (e *Engine) markCancelled(runID string) {
	runRec, err := e.Store.GetRun(runID)
	if err != nil || runRec == nil {
		return
	}
	switch runRec.Status {
	case store.StatusSucceeded, store.StatusFailed, store.StatusCancelled:
		return
	}
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventRunCancelled,
		Data: map[string]any{"reason": "cancelled"},
	})
	_ = e.Store.SetHITL(runID, nil)
	_ = e.Store.UpdateRun(runID, store.StatusCancelled, "", "cancelled")
	e.recordTerminalMessage(runID)
}

func (e *Engine) toolTimeout() time.Duration {
	if e.ToolTimeout > 0 {
		return e.ToolTimeout
	}
	return DefaultToolTimeout
}

// buildMessages assembles the LLM prompt: system + windowed conversation
// history + current user input. When the most recent history entry is already a
// user message with the same content as input (the API appends the user message
// before calling Execute), the current input is not appended again to avoid a
// duplicate turn.
func (e *Engine) buildMessages(system, conversationID, input string) []llm.Message {
	messages := []llm.Message{{Role: llm.RoleSystem, Content: system}}
	if e.Messages != nil && conversationID != "" {
		for _, m := range e.Messages.ListWindow(conversationID, e.MaxMessages) {
			switch m.Role {
			case conversation.RoleUser:
				messages = append(messages, llm.Message{Role: llm.RoleUser, Content: m.Content})
			case conversation.RoleAssistant, conversation.RoleSystemNote:
				messages = append(messages, llm.Message{Role: llm.RoleAssistant, Content: m.Content})
			}
		}
	}
	if len(messages) > 0 {
		last := messages[len(messages)-1]
		if last.Role == llm.RoleUser && last.Content == input {
			return messages
		}
	}
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: input})
	return messages
}

// ContinueFromHITL applies a human decision to a waiting_human run.
// Same-process: prefers Gate.Resume so a blocked Execute continues.
// After restart (no waiter): reads HITLPayload, applies approve/reject, and continues ReAct.
func (e *Engine) ContinueFromHITL(ctx context.Context, runID string, d Decision) error {
	ctx = e.injectAuthCtx(ctx, runID)
	if e.Gate != nil {
		if err := e.Gate.Resume(runID, d); err == nil {
			return nil
		}
	}

	run, err := e.Store.GetRun(runID)
	if err != nil {
		return err
	}
	if run.Status != store.StatusWaitingHuman {
		return fmt.Errorf("run not waiting_human")
	}
	payload, err := e.Store.GetHITL(runID)
	if err != nil {
		return err
	}
	if payload == nil {
		return fmt.Errorf("no hitl payload")
	}

	if !d.Approve {
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventHITLRejected,
			Data: map[string]any{"decision": "reject", "comment": d.Comment},
		})
		_ = e.Store.SetHITL(runID, nil)
		_ = e.Store.UpdateRun(runID, store.StatusFailed, "", "hitl rejected")
		e.recordTerminalMessage(runID)
		return fmt.Errorf("hitl rejected")
	}

	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventHITLResumed,
		Data: map[string]any{"decision": "approve", "comment": d.Comment},
	})
	_ = e.Store.UpdateRun(runID, store.StatusRunning, "", "")
	_ = e.Store.SetHITL(runID, nil)

	toolCallID := lastToolCallID(e.Store, runID)
	toolCtx, toolCancel := context.WithTimeout(ctx, e.toolTimeout())
	content, isError, invErr := e.Tools.Invoke(toolCtx, payload.ToolName, payload.Arguments)
	toolCancel()
	if invErr != nil {
		isError = true
		if content == nil {
			msg := invErr.Error()
			if errors.Is(invErr, context.DeadlineExceeded) {
				msg = fmt.Sprintf("tool timed out after %s", e.toolTimeout())
			}
			content = map[string]any{"error": msg}
		}
	}
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventToolResult,
		Data: map[string]any{
			"tool_call_id": toolCallID,
			"name":         payload.ToolName,
			"content":      identity.RedactSensitive(content),
			"is_error":     isError,
		},
	})

	ag, err := e.Store.GetAgent(run.AgentID)
	if err != nil {
		return err
	}
	if e.getRunSkillState(runID) == nil {
		e.beginRunSkills(runID, append([]string(nil), ag.Skills...), ag.System)
	}
	evs, err := e.Store.ListEvents(runID)
	if err != nil {
		return err
	}
	sys := e.composeSystem(ag.System, runID)
	sys = e.appendSessionAuthHint(sys, run.ConversationID)
	messages := e.buildResumeMessages(sys, run.ConversationID, run.Input, evs)
	return e.runLoop(ctx, runID, messages)
}

// buildResumeMessages assembles the LLM prompt for a cold HITL resume: the
// system prompt + windowed conversation history + current user input (via
// buildMessages, which dedups a trailing user input already persisted by the
// API), followed by the assistant tool-call / tool-result turns already
// recorded for this run. This keeps cross-restart HITL from dropping prior
// conversation context while avoiding duplicate system or user-input messages.
func (e *Engine) buildResumeMessages(system, conversationID, input string, evs []store.Event) []llm.Message {
	messages := e.buildMessages(system, conversationID, input)
	messages = append(messages, eventsAfterInput(evs)...)
	return messages
}

// eventsAfterInput converts run events into LLM messages, skipping the leading
// system and user-input messages (which are provided separately by
// buildMessages). It returns only the assistant tool-call / tool-result / final
// assistant message turns recorded for this run.
func eventsAfterInput(evs []store.Event) []llm.Message {
	var out []llm.Message
	var pending []llm.ToolCall
	flushPending := func() {
		if len(pending) == 0 {
			return
		}
		out = append(out, llm.Message{Role: llm.RoleAssistant, ToolCalls: pending})
		pending = nil
	}
	for _, ev := range evs {
		switch ev.Type {
		case EventLLMToolCall:
			pending = append(pending, llm.ToolCall{
				ID:        asString(ev.Data["id"]),
				Name:      asString(ev.Data["name"]),
				Arguments: asMap(ev.Data["arguments"]),
			})
		case EventToolResult:
			flushPending()
			raw, _ := json.Marshal(ev.Data["content"])
			out = append(out, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: asString(ev.Data["tool_call_id"]),
				Content:    string(raw),
			})
		case EventLLMMessage:
			flushPending()
			out = append(out, llm.Message{
				Role:    llm.RoleAssistant,
				Content: asString(ev.Data["content"]),
			})
		}
	}
	flushPending()
	return out
}

func (e *Engine) isCancelled(runID string) bool {
	runRec, err := e.Store.GetRun(runID)
	return err == nil && runRec != nil && runRec.Status == store.StatusCancelled
}

func (e *Engine) runLoop(ctx context.Context, runID string, messages []llm.Message) error {
	maxSteps := e.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 16
	}

	for step := 0; step < maxSteps; step++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if e.isCancelled(runID) {
			return context.Canceled
		}
		specs := e.specsForRun(runID)
		msg, err := e.LLM.Chat(ctx, messages, specs)
		if err != nil {
			if ctx.Err() != nil || e.isCancelled(runID) {
				return context.Canceled
			}
			_ = e.Store.AppendEvent(runID, store.Event{
				Type: EventLLMError,
				Data: map[string]any{"error": err.Error()},
			})
			_ = e.Store.UpdateRun(runID, store.StatusFailed, "", err.Error())
			e.recordTerminalMessage(runID)
			return err
		}

		if len(msg.ToolCalls) > 0 {
			messages = append(messages, msg)
			for _, tc := range msg.ToolCalls {
				if err := ctx.Err(); err != nil {
					return err
				}
				if e.isCancelled(runID) {
					return context.Canceled
				}
				_ = e.Store.AppendEvent(runID, store.Event{
					Type: EventLLMToolCall,
					Data: map[string]any{
						"id":        tc.ID,
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				})

				if tc.Name == skill.ActivateToolName {
					content, isError := e.handleActivateSkill(runID, tc.Arguments)
					if !isError && len(messages) > 0 && messages[0].Role == llm.RoleSystem {
						messages[0].Content = e.composeSystem("", runID)
					}
					_ = e.Store.AppendEvent(runID, store.Event{
						Type: EventToolResult,
						Data: map[string]any{
							"tool_call_id": tc.ID,
							"name":         tc.Name,
							"content":      identity.RedactSensitive(content),
							"is_error":     isError,
						},
					})
					raw, _ := json.Marshal(content)
					messages = append(messages, llm.Message{
						Role:       llm.RoleTool,
						ToolCallID: tc.ID,
						Content:    string(raw),
					})
					continue
				}

				if e.blockedByLogin(ctx, tc.Name) {
					content, isError := tool.LoginRequiredContent(), true
					_ = e.Store.AppendEvent(runID, store.Event{
						Type: EventToolResult,
						Data: map[string]any{
							"tool_call_id": tc.ID,
							"name":         tc.Name,
							"content":      identity.RedactSensitive(content),
							"is_error":     isError,
						},
					})
					raw, _ := json.Marshal(content)
					messages = append(messages, llm.Message{
						Role:       llm.RoleTool,
						ToolCallID: tc.ID,
						Content:    string(raw),
					})
					continue
				}

				if e.Tools.RequiresApproval(tc.Name) {
					if err := e.awaitHITL(ctx, runID, tc); err != nil {
						return err
					}
					if e.isCancelled(runID) {
						return context.Canceled
					}
				}

				invokeCtx := identity.WithToolCallID(ctx, tc.ID)
				toolCtx, toolCancel := context.WithTimeout(invokeCtx, e.toolTimeout())
				content, isError, invErr := e.Tools.Invoke(toolCtx, tc.Name, tc.Arguments)
				toolCancel()
				if invErr != nil {
					isError = true
					if content == nil {
						msg := invErr.Error()
						if errors.Is(invErr, context.DeadlineExceeded) {
							msg = fmt.Sprintf("tool timed out after %s", e.toolTimeout())
						}
						content = map[string]any{"error": msg}
					}
				}
				if ctx.Err() != nil || e.isCancelled(runID) {
					return context.Canceled
				}

				_ = e.Store.AppendEvent(runID, store.Event{
					Type: EventToolResult,
					Data: map[string]any{
						"tool_call_id": tc.ID,
						"name":         tc.Name,
						"content":      identity.RedactSensitive(content),
						"is_error":     isError,
					},
				})

				raw, _ := json.Marshal(content)
				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					ToolCallID: tc.ID,
					Content:    string(raw),
				})
			}
			continue
		}

		if e.isCancelled(runID) {
			return context.Canceled
		}
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventLLMMessage,
			Data: map[string]any{"content": msg.Content},
		})
		if err := e.Store.UpdateRun(runID, store.StatusSucceeded, msg.Content, ""); err != nil {
			return err
		}
		e.recordTerminalMessage(runID)
		return nil
	}

	if e.isCancelled(runID) {
		return context.Canceled
	}
	errMsg := fmt.Sprintf("max steps exceeded (%d)", maxSteps)
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventLLMError,
		Data: map[string]any{"error": errMsg},
	})
	_ = e.Store.UpdateRun(runID, store.StatusFailed, "", errMsg)
	e.recordTerminalMessage(runID)
	return fmt.Errorf("%s", errMsg)
}

func (e *Engine) blockedByLogin(ctx context.Context, name string) bool {
	if e.Identities == nil || !e.Tools.RequiresLogin(name) {
		return false
	}
	conv := identity.ConversationIDFrom(ctx)
	if conv == "" {
		return false
	}
	res := authresolve.OpenAPISecurityResolver{}.Resolve(ctx, authresolve.ResolveInput{
		Identities:      e.Identities.List(conv),
		SecuritySchemes: e.Tools.SecuritySchemes(name),
		ForceIdentityID: identity.ForceIdentityIDFrom(ctx),
	})
	return !res.OK || len(res.Headers) == 0
}

func (e *Engine) awaitHITL(ctx context.Context, runID string, tc llm.ToolCall) error {
	if e.Gate == nil {
		return fmt.Errorf("approval required but gate is nil")
	}
	payload := &store.HITLPayload{
		Prompt:    fmt.Sprintf("Approve tool %s?", tc.Name),
		ToolName:  tc.Name,
		Arguments: tc.Arguments,
	}

	// Arm the waiter before advertising waiting_human so resume cannot miss it.
	ch, err := e.Gate.BeginWait(runID)
	if err != nil {
		return err
	}
	defer e.Gate.EndWait(runID)

	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventHITLWaiting,
		Data: map[string]any{
			"prompt":    payload.Prompt,
			"tool_name": payload.ToolName,
			"arguments": payload.Arguments,
		},
	})
	_ = e.Store.UpdateRun(runID, store.StatusWaitingHuman, "", "")
	_ = e.Store.SetHITL(runID, payload)

	var d Decision
	select {
	case d = <-ch:
	case <-ctx.Done():
		_ = e.Store.SetHITL(runID, nil)
		return context.Canceled
	}
	if !d.Approve {
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventHITLRejected,
			Data: map[string]any{"decision": "reject", "comment": d.Comment},
		})
		_ = e.Store.SetHITL(runID, nil)
		_ = e.Store.UpdateRun(runID, store.StatusFailed, "", "hitl rejected")
		e.recordTerminalMessage(runID)
		return fmt.Errorf("hitl rejected")
	}
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventHITLResumed,
		Data: map[string]any{"decision": "approve", "comment": d.Comment},
	})
	_ = e.Store.UpdateRun(runID, store.StatusRunning, "", "")
	_ = e.Store.SetHITL(runID, nil)
	return nil
}

func (e *Engine) injectAuthCtx(ctx context.Context, runID string) context.Context {
	runRec, err := e.Store.GetRun(runID)
	if err != nil || runRec == nil {
		return ctx
	}
	return e.injectAuthCtxFromRun(ctx, runRec)
}

func (e *Engine) injectAuthCtxFromRun(ctx context.Context, runRec *store.Run) context.Context {
	ctx = identity.WithConversationID(ctx, runRec.ConversationID)
	if runRec.IdentityID != "" {
		ctx = identity.WithForceIdentityID(ctx, runRec.IdentityID)
	}
	if len(runRec.PassthroughHeaders) > 0 {
		ctx = identity.WithPassthroughHeaders(ctx, runRec.PassthroughHeaders)
	}
	ctx = identity.WithRunID(ctx, runRec.ID)
	ctx = identity.WithAgentID(ctx, runRec.AgentID)
	return ctx
}

// recordTerminalMessage persists the final assistant / system_note message for
// a run to the conversation store. No-op when Messages is nil or the run has no
// conversation_id. waiting_human is intentionally not recorded (the resume
// path will write the terminal message when it reaches succeeded/failed).
func (e *Engine) recordTerminalMessage(runID string) {
	if e.Messages == nil {
		return
	}
	runRec, err := e.Store.GetRun(runID)
	if err != nil || runRec == nil || runRec.ConversationID == "" {
		return
	}
	if !e.userTurnStillLinked(runRec) {
		// User turn was rolled back; do not resurrect assistant rows for superseded runs.
		return
	}
	switch runRec.Status {
	case store.StatusSucceeded:
		if strings.TrimSpace(runRec.Output) == "" {
			return
		}
		_, _ = e.Messages.Append(runRec.ConversationID, conversation.Message{
			Role:    conversation.RoleAssistant,
			Content: runRec.Output,
			RunID:   runID,
		})
	case store.StatusFailed:
		note := strings.TrimSpace(runRec.Error)
		if note == "" {
			note = "运行失败"
		} else {
			note = "运行失败：" + note
		}
		_, _ = e.Messages.Append(runRec.ConversationID, conversation.Message{
			Role:    conversation.RoleSystemNote,
			Content: note,
			RunID:   runID,
		})
	case store.StatusCancelled:
		_, _ = e.Messages.Append(runRec.ConversationID, conversation.Message{
			Role:    conversation.RoleSystemNote,
			Content: "已取消",
			RunID:   runID,
		})
	}
}

func (e *Engine) userTurnStillLinked(runRec *store.Run) bool {
	msgs := e.Messages.List(runRec.ConversationID)
	for _, m := range msgs {
		if m.RunID == runRec.ID && m.Role == conversation.RoleUser {
			return true
		}
	}
	// Runs created before user rows carried run_id: accept when the latest user
	// message still matches this run's input.
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == conversation.RoleUser {
			return msgs[i].Content == runRec.Input
		}
	}
	return false
}

func (e *Engine) ensureRunStarted(runID string) error {
	evs, err := e.Store.ListEvents(runID)
	if err != nil {
		return err
	}
	for _, ev := range evs {
		if ev.Type == EventRunStarted {
			return nil
		}
	}
	return e.Store.AppendEvent(runID, store.Event{Type: EventRunStarted})
}

func lastToolCallID(st store.Store, runID string) string {
	evs, err := st.ListEvents(runID)
	if err != nil {
		return ""
	}
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type == EventLLMToolCall {
			if id, ok := evs[i].Data["id"].(string); ok {
				return id
			}
		}
	}
	return ""
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}
