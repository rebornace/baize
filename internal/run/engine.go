package run

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rebornace/baize/internal/agent"
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
	MaxSteps int // default 8
}

func (e *Engine) Execute(ctx context.Context, runID string, ag agent.Def, input string) error {
	ctx = e.injectAuthCtx(ctx, runID)
	if err := e.ensureRunStarted(runID); err != nil {
		return err
	}
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: ag.System},
		{Role: llm.RoleUser, Content: input},
	}
	return e.runLoop(ctx, runID, messages)
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
	messages := messagesFromEvents(ag.System, run.Input, evs)
	return e.runLoop(ctx, runID, messages)
}

func (e *Engine) runLoop(ctx context.Context, runID string, messages []llm.Message) error {
	maxSteps := e.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 8
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
		return nil
	}

	errMsg := fmt.Sprintf("max steps exceeded (%d)", maxSteps)
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventLLMError,
		Data: map[string]any{"error": errMsg},
	})
	_ = e.Store.UpdateRun(runID, store.StatusFailed, "", errMsg)
	return fmt.Errorf("%s", errMsg)
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
	ctx = identity.WithConversationID(ctx, runRec.ConversationID)
	if runRec.IdentityID != "" {
		ctx = identity.WithForceIdentityID(ctx, runRec.IdentityID)
	}
	return ctx
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

func messagesFromEvents(system, input string, evs []store.Event) []llm.Message {
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: system},
		{Role: llm.RoleUser, Content: input},
	}
	var pending []llm.ToolCall
	flushPending := func() {
		if len(pending) == 0 {
			return
		}
		msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, ToolCalls: pending})
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
			msgs = append(msgs, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: asString(ev.Data["tool_call_id"]),
				Content:    string(raw),
			})
		case EventLLMMessage:
			flushPending()
			msgs = append(msgs, llm.Message{
				Role:    llm.RoleAssistant,
				Content: asString(ev.Data["content"]),
			})
		}
	}
	flushPending()
	return msgs
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}
