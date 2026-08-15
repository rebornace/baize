package run

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
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
)

type Engine struct {
	Store    store.Store
	LLM      llm.Provider
	Tools    *tool.Registry
	Gate     *Gate
	MaxSteps int // default 16
	// Messages optionally persists conversation history across runs.
	// nil = legacy behavior (no cross-run message persistence). When non-nil,
	// Execute injects a windowed history into the LLM prompt and the engine
	// records terminal assistant / system_note messages on succeeded / failed.
	Messages    conversation.Store
	MaxMessages int // conversation window size; config Load defaults <=0 to 40
	// Identities is optional. When non-nil, tools with RequireLogin are gated
	// before HITL / Invoke when the run has a conversation_id.
	Identities identity.Store
}

func (e *Engine) Execute(ctx context.Context, runID string, ag agent.Def, input string) error {
	runRec, err := e.Store.GetRun(runID)
	if err != nil {
		return err
	}
	if runRec == nil {
		return fmt.Errorf("run not found: %s", runID)
	}
	ctx = e.injectAuthCtxFromRun(ctx, runRec)
	if err := e.ensureRunStarted(runID); err != nil {
		return err
	}
	messages := e.buildMessages(ag.System, runRec.ConversationID, input)
	return e.runLoop(ctx, runID, messages)
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
	content, isError, invErr := e.Tools.Invoke(ctx, payload.ToolName, payload.Arguments)
	if invErr != nil {
		isError = true
		if content == nil {
			content = map[string]any{"error": invErr.Error()}
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
	evs, err := e.Store.ListEvents(runID)
	if err != nil {
		return err
	}
	messages := e.buildResumeMessages(ag.System, run.ConversationID, run.Input, evs)
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

func (e *Engine) runLoop(ctx context.Context, runID string, messages []llm.Message) error {
	maxSteps := e.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 16
	}
	specs := e.Tools.Specs()

	for step := 0; step < maxSteps; step++ {
		msg, err := e.LLM.Chat(ctx, messages, specs)
		if err != nil {
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
				_ = e.Store.AppendEvent(runID, store.Event{
					Type: EventLLMToolCall,
					Data: map[string]any{
						"id":        tc.ID,
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				})

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
				}

				content, isError, invErr := e.Tools.Invoke(ctx, tc.Name, tc.Arguments)
				if invErr != nil {
					isError = true
					if content == nil {
						content = map[string]any{"error": invErr.Error()}
					}
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
		return ctx.Err()
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
	}
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
