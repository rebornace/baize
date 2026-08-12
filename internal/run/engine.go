package run

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

const (
	EventRunStarted  = "run.started"
	EventLLMToolCall = "llm.tool_call"
	EventToolResult  = "tool.result"
	EventLLMMessage  = "llm.message"
	EventLLMError    = "llm.error"
)

type Engine struct {
	Store    *store.Store
	LLM      llm.Provider
	Tools    *tool.Registry
	MaxSteps int // default 8
}

func (e *Engine) Execute(ctx context.Context, runID string, ag agent.Def, input string) error {
	maxSteps := e.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 8
	}

	if err := e.ensureRunStarted(runID); err != nil {
		return err
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: ag.System},
		{Role: llm.RoleUser, Content: input},
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
						"content":      content,
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
