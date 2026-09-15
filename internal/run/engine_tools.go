package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// buildResumeMessages assembles the LLM prompt for a cold HITL resume: the
// system prompt + windowed conversation history + current user input (via
// buildMessages, which dedups a trailing user input already persisted by the
// API), followed by the assistant tool-call / tool-result turns already
// recorded for this run. This keeps cross-restart HITL from dropping prior
// conversation context while avoiding duplicate system or user-input messages.
func (e *Engine) buildResumeMessages(system, conversationID, input string, evs []store.Event) []llm.Message {
	messages := e.buildMessages(system, conversationID, input, nil)
	messages = append(messages, e.eventsAfterInput(evs, conversationID)...)
	return messages
}

// toolResultMessage builds the LLM tool message for one tool invocation. When
// the invoker attached image results, the message is multimodal (a text part
// with the JSON content followed by image parts); otherwise it is plain text.
func (e *Engine) toolResultMessage(tcID string, content map[string]any) llm.Message {
	cleaned, results := tool.ExtractImageParts(content)
	raw, _ := json.Marshal(cleaned)
	if len(results) == 0 {
		return llm.Message{Role: llm.RoleTool, ToolCallID: tcID, Content: string(raw)}
	}
	parts := make([]llm.ContentPart, 0, len(results)+1)
	parts = append(parts, llm.ContentPart{Type: "text", Text: string(raw)})
	for _, r := range results {
		parts = append(parts, r.Part)
	}
	return llm.Message{Role: llm.RoleTool, ToolCallID: tcID, Parts: parts}
}

// persistToolResult appends the tool.result event. Image bytes are stripped
// (never persisted to events); lightweight image_refs pointers are recorded so
// a cold resume can rebuild the image via ImagePartResolver.
func (e *Engine) persistToolResult(runID, callID, name string, content map[string]any, isError bool) {
	cleaned, results := tool.ExtractImageParts(content)
	data := map[string]any{
		"tool_call_id": callID,
		"name":         name,
		"content":      identity.RedactSensitive(cleaned),
		"is_error":     isError,
	}
	if len(results) > 0 {
		refs := make([]map[string]any, 0, len(results))
		for _, r := range results {
			refs = append(refs, map[string]any{"workspace_path": r.Path})
		}
		data["image_refs"] = refs
	}
	_ = e.Store.AppendEvent(runID, store.Event{Type: EventToolResult, Data: data})
}

// imageRefsFromEvent extracts workspace paths from a persisted image_refs
// value, tolerating both in-memory ([]map[string]any) and JSON-round-tripped
// ([]any of map[string]any) shapes.
func imageRefsFromEvent(data map[string]any) []string {
	raw, ok := data["image_refs"]
	if !ok {
		return nil
	}
	var paths []string
	switch refs := raw.(type) {
	case []map[string]any:
		for _, ref := range refs {
			if p, ok := ref["workspace_path"].(string); ok && p != "" {
				paths = append(paths, p)
			}
		}
	case []any:
		for _, item := range refs {
			if m, ok := item.(map[string]any); ok {
				if p, ok := m["workspace_path"].(string); ok && p != "" {
					paths = append(paths, p)
				}
			}
		}
	}
	return paths
}

// toolResultFromEvent rebuilds a tool message from a persisted event,
// re-attaching image parts via the resolver when image_refs are present.
func (e *Engine) toolResultFromEvent(ev store.Event, convID string) llm.Message {
	tcID := asString(ev.Data["tool_call_id"])
	raw, _ := json.Marshal(ev.Data["content"])
	msg := llm.Message{Role: llm.RoleTool, ToolCallID: tcID, Content: string(raw)}
	refs := imageRefsFromEvent(ev.Data)
	var parts []llm.ContentPart
	for _, wsPath := range refs {
		if e.ImagePartResolver == nil {
			break
		}
		if part, ok := e.ImagePartResolver(convID, wsPath); ok {
			parts = append(parts, part)
		}
	}
	if len(parts) > 0 {
		msg.Parts = append([]llm.ContentPart{{Type: "text", Text: string(raw)}}, parts...)
		msg.Content = ""
	} else if len(refs) > 0 {
		msg.Content = string(raw) + "\n(image available at " + strings.Join(refs, ", ") + " — call read_image to view)"
	}
	return msg
}

// eventsAfterInput converts run events into LLM messages, skipping the leading
// system and user-input messages (which are provided separately by
// buildMessages). It returns only the assistant tool-call / tool-result / final
// assistant message turns recorded for this run.
func (e *Engine) eventsAfterInput(evs []store.Event, convID string) []llm.Message {
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
			out = append(out, e.toolResultFromEvent(ev, convID))
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

// awaitHITL is the ReAct-path wrapper: shapes the payload from the LLM tool
// call and delegates to the payload-level kernel.
func (e *Engine) awaitHITL(ctx context.Context, runID string, tc llm.ToolCall) error {
	return e.awaitHITLPayload(ctx, runID,
		fmt.Sprintf("Approve tool %s?", tc.Name), tc.Name, tc.Arguments)
}

// invokeTool performs one full tool interaction for a run: pre-call gate
// (login / approval) and events, then the timeout-bounded Invoke with the same
// event/data shapes as before. A rejection returns ErrHITLRejected and the run
// has already been finalized as rejected (with a humanized note); the caller
// must stop instead of appending further results. Transient failures inside
// one interaction keep the last-known return shape so the value/error contract
// stays uniform.
func (e *Engine) invokeTool(ctx context.Context, runID, callID, name string, args map[string]any, skipApproval bool) (map[string]any, bool, error) {
	isError := false
	content := map[string]any{}

	rejected, rerr := func() (bool, error) {
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventLLMToolCall,
			Data: map[string]any{"id": callID, "name": name, "arguments": redactToolArgs(args)},
		})

		if e.blockedByLogin(ctx, name) {
			content = tool.LoginRequiredContent()
			isError = true
			return false, nil
		}

		if !skipApproval && e.Tools.RequiresApproval(name) {
			if err := e.awaitHITL(ctx, runID, llm.ToolCall{ID: callID, Name: name, Arguments: args}); err != nil {
				if !errors.Is(err, ErrHITLRejected) {
					if ctx.Err() != nil || e.isCancelled(runID) {
						return false, context.Canceled
					}
					return false, err
				}
				return true, nil
			}
			if e.isCancelled(runID) {
				return false, context.Canceled
			}
		}

		invokeCtx := identity.WithToolCallID(ctx, callID)
		toolCtx, cancel := context.WithTimeout(invokeCtx, e.toolTimeout())
		c, toolIsErr, ierr := e.Tools.Invoke(toolCtx, name, args)
		cancel()
		isError = toolIsErr
		if ierr != nil {
			isError = true
			if c == nil {
				msg := ierr.Error()
				if errors.Is(ierr, context.DeadlineExceeded) {
					msg = fmt.Sprintf("tool timed out after %s", e.toolTimeout())
				}
				c = map[string]any{"error": msg}
			}
		}
		content = c
		return false, nil
	}()
	if rejected {
		return nil, true, ErrHITLRejected
	}
	if rerr != nil && (ctx.Err() != nil || e.isCancelled(runID)) {
		return nil, false, rerr
	}
	if ctx.Err() != nil || e.isCancelled(runID) {
		return nil, false, context.Canceled
	}

	e.persistToolResult(runID, callID, name, content, isError)
	return content, isError, nil
}

func (e *Engine) awaitHITLPayload(ctx context.Context, runID, prompt, toolName string, args map[string]any) error {
	if e.Gate == nil {
		return fmt.Errorf("approval required but gate is nil")
	}
	payload := &store.HITLPayload{
		Prompt:    prompt,
		ToolName:  toolName,
		Arguments: args,
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
	e.deliverHITLNotify(runID, payload)

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
		e.finalizeRejectedRun(runID)
		return ErrHITLRejected
	}
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventHITLResumed,
		Data: map[string]any{"decision": "approve", "comment": d.Comment},
	})
	_ = e.Store.UpdateRun(runID, store.StatusRunning, "", "")
	_ = e.Store.SetHITL(runID, nil)
	return nil
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

var sensitiveToolArgKey = regexp.MustCompile(`(?i)(password|passwd|secret|token|api_key)`)

// redactToolArgs returns a copy of args with sensitive keys masked for events.
func redactToolArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		if sensitiveToolArgKey.MatchString(k) {
			out[k] = "***"
		} else {
			out[k] = v
		}
	}
	return out
}
