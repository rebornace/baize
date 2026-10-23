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
	if hash, rev, cid := e.toolContractFields(name); hash != "" || cid != "" {
		if hash != "" {
			data["contract_hash"] = hash
		}
		if rev > 0 {
			data["catalog_revision"] = rev
		}
		if cid != "" {
			data["connector_id"] = cid
		}
	} else if cid := e.connectorIDForTool(name); cid != "" {
		data["connector_id"] = cid
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
		Identities:      e.Identities.List(identity.KeyFrom(ctx)),
		SecuritySchemes: e.Tools.SecuritySchemes(name),
		ForceIdentityID: identity.ForceIdentityIDFrom(ctx),
	})
	return !res.OK || len(res.Headers) == 0
}

// awaitHITL is the ReAct-path wrapper: shapes the payload from the LLM tool
// call and delegates to the payload-level kernel.
func (e *Engine) awaitHITL(ctx context.Context, runID string, tc llm.ToolCall) (Decision, error) {
	return e.awaitHITLDecision(ctx, runID, &store.HITLPayload{
		Prompt:    fmt.Sprintf("Approve tool %s?", tc.Name),
		ToolName:  tc.Name,
		Arguments: tc.Arguments,
		Kind:      store.HITLKindApproval,
	})
}

// invokeTool performs one full tool interaction for a run: pre-call gate
// (login / approval) and events, then the timeout-bounded Invoke with the same
// event/data shapes as before. A rejection returns ErrHITLRejected and the run
// has already been finalized as rejected (with a humanized note); the caller
// must stop instead of appending further results. Transient failures inside
// one interaction keep the last-known return shape so the value/error contract
// stays uniform.
func (e *Engine) invokeTool(ctx context.Context, runID, callID, name string, args map[string]any, skipApproval bool) (map[string]any, bool, error) {
	return e.invokeToolPersist(ctx, runID, callID, name, args, skipApproval, true)
}

// invokeToolPersist is invokeTool with optional deferred persistence (parallel
// tool batches persist in original tool_call order on refill).
func (e *Engine) invokeToolPersist(ctx context.Context, runID, callID, name string, args map[string]any, skipApproval, persist bool) (map[string]any, bool, error) {
	isError := false
	content := map[string]any{}

	// High-confidence name remap (delete→remove under the same Controller_
	// prefix) so a typo'd call still reaches HITL / Invoke. Ambiguous misses
	// stay on the requested name and surface suggestions in the tool error.
	resolved := name
	var resolvedFrom string
	if e.Tools != nil {
		if canon, err := e.Tools.ResolveName(name); err == nil {
			if canon != name {
				resolvedFrom = name
			}
			resolved = canon
		}
	}

	rejected, rerr := func() (bool, error) {
		callData := map[string]any{"id": callID, "name": resolved, "arguments": redactToolArgs(args)}
		if resolvedFrom != "" {
			callData["resolved_from"] = resolvedFrom
		}
		if hash, rev, cid := e.toolContractFields(resolved); hash != "" || cid != "" {
			if hash != "" {
				callData["contract_hash"] = hash
			}
			if rev > 0 {
				callData["catalog_revision"] = rev
			}
			if cid != "" {
				callData["connector_id"] = cid
			}
		}
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventLLMToolCall,
			Data: callData,
		})

		if sc, ok := e.getChildScope(runID); ok && sc.Mode == store.SpawnModeReadonly {
			if e.Tools != nil && e.Tools.RequiresApproval(resolved) {
				content = map[string]any{"error": "readonly scoped child cannot use approval-gated tools"}
				isError = true
				return false, nil
			}
			if info, ok := e.Tools.Get(resolved); ok && info.RequireLogin {
				content = map[string]any{"error": "readonly scoped child cannot use login-gated tools"}
				isError = true
				return false, nil
			}
		}

		if e.blockedByLogin(ctx, resolved) {
			content = tool.LoginRequiredContent()
			if _, _, cid := e.toolContractFields(resolved); cid != "" {
				content["connector_id"] = cid
			}
			isError = true
			return false, nil
		}

		if e.Tools != nil {
			if err, ok := e.Tools.PrecheckArgs(resolved, args, e.now()); ok && err != nil {
				content = map[string]any{"error": err.Error()}
				isError = true
				return false, nil
			}
		}

		var hitl Decision
		if !skipApproval && e.Tools.RequiresApproval(resolved) {
			var err error
			hitl, err = e.awaitHITL(ctx, runID, llm.ToolCall{ID: callID, Name: resolved, Arguments: args})
			if err != nil {
				if errors.Is(err, ErrWaitingHuman) {
					return false, ErrWaitingHuman
				}
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

		invokeCtx := identity.WithToolCallID(applyDecisionPrincipal(ctx, hitl), callID)
		toolCtx, cancel := context.WithTimeout(invokeCtx, e.toolTimeout())
		c, toolIsErr, ierr := e.Tools.Invoke(toolCtx, resolved, args)
		cancel()
		if errors.Is(ierr, ErrWaitingHuman) {
			return false, ErrWaitingHuman
		}
		isError = toolIsErr
		if ierr != nil {
			isError = true
			if c == nil {
				msg := ierr.Error()
				if errors.Is(ierr, context.DeadlineExceeded) {
					msg = fmt.Sprintf("tool timed out after %s", e.toolTimeout())
				}
				// If resolve failed earlier, attach near-name suggestions.
				if resolved == name && e.Tools != nil {
					if _, rerr := e.Tools.ResolveName(name); rerr != nil {
						msg = rerr.Error()
					}
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
	if errors.Is(rerr, ErrWaitingHuman) {
		return nil, false, ErrWaitingHuman
	}
	if rerr != nil && (ctx.Err() != nil || e.isCancelled(runID)) {
		return nil, false, rerr
	}
	if ctx.Err() != nil || e.isCancelled(runID) {
		return nil, false, context.Canceled
	}

	if isError {
		content = tool.NormalizeLoginRequired(content)
		if tool.IsLoginRequired(content) {
			if _, has := content["connector_id"]; !has {
				if _, _, cid := e.toolContractFields(resolved); cid != "" {
					content["connector_id"] = cid
				}
			}
		}
	}
	if persist {
		e.persistToolResult(runID, callID, resolved, content, isError)
	}
	return content, isError, nil
}

func (e *Engine) awaitHITLPayload(ctx context.Context, runID, prompt, toolName string, args map[string]any) error {
	// Workflow steps still use a blocking Gate wait: cold resume cannot rebuild
	// in-flight pipeline state (see workflowInterrupted). ReAct tool approvals
	// use non-blocking awaitHITLDecision instead.
	_, err := e.awaitHITLDecisionBlocking(ctx, runID, &store.HITLPayload{
		Prompt:    prompt,
		ToolName:  toolName,
		Arguments: args,
		Kind:      store.HITLKindApproval,
	})
	return err
}

// ErrLoginSkippedContinue means the user skipped login for a connector; ReAct should continue.
var ErrLoginSkippedContinue = errors.New("login skipped; continue")

// ErrLoginStepSkippedContinue means the user ignored only this login step; ReAct should continue
// without hiding the whole connector (sibling tools remain available).
var ErrLoginStepSkippedContinue = errors.New("login step skipped; continue")

// ErrLoginResumedContinue means the user finished browser login; ReAct should continue.
var ErrLoginResumedContinue = errors.New("login resumed; continue")

func (e *Engine) connectorIDForTool(name string) string {
	if e.Tools == nil {
		return ""
	}
	info, ok := e.Tools.Get(name)
	if !ok {
		return ""
	}
	return strings.TrimSpace(info.ConnectorID)
}

// awaitLoginDecision pauses the run until the user signs in, skips step/system, or aborts.
func (e *Engine) awaitLoginDecision(ctx context.Context, runID, toolName, connectorID string) error {
	_, err := e.awaitHITLDecision(ctx, runID, &store.HITLPayload{
		Prompt:      "需要登录后才能继续。请点击「浏览器登录」，或「忽略当前步骤」改用其它方式，或「忽略此系统」跳过该业务系统。",
		ToolName:    toolName,
		Kind:        store.HITLKindLogin,
		ConnectorID: connectorID,
	})
	return err
}

// awaitHITLDecision persists waiting_human + HITL payload and returns
// ErrWaitingHuman so the worker can release its slot. Resume is via
// ContinueFromHITL (cold path / KindHitlResume job).
func (e *Engine) awaitHITLDecision(ctx context.Context, runID string, payload *store.HITLPayload) (Decision, error) {
	if err := e.persistHITLWaiting(ctx, runID, payload); err != nil {
		return Decision{}, err
	}
	return Decision{}, ErrWaitingHuman
}

// awaitHITLDecisionBlocking is the legacy select-on-Gate path used by workflow
// step approvals (cold resume cannot rebuild pipeline state mid-flight).
func (e *Engine) awaitHITLDecisionBlocking(ctx context.Context, runID string, payload *store.HITLPayload) (Decision, error) {
	if e.Gate == nil {
		return Decision{}, fmt.Errorf("approval required but gate is nil")
	}
	// Register the waiter before advertising waiting_human so a concurrent
	// KindHitlResume cannot miss the channel and fall into the cold path.
	ch, err := e.Gate.BeginWait(runID)
	if err != nil {
		return Decision{}, err
	}
	defer e.Gate.EndWait(runID)

	if err := e.persistHITLWaiting(ctx, runID, payload); err != nil {
		return Decision{}, err
	}

	var d Decision
	select {
	case d = <-ch:
	case <-ctx.Done():
		_ = e.Store.SetHITL(runID, nil)
		return Decision{}, context.Canceled
	}
	if payload.Kind == store.HITLKindLogin {
		if d.Approve {
			_ = e.Store.AppendEvent(runID, store.Event{
				Type: EventHITLResumed,
				Data: map[string]any{"decision": "approve", "comment": d.Comment, "kind": store.HITLKindLogin},
			})
			_ = e.Store.UpdateRun(runID, store.StatusRunning, "", "")
			_ = e.Store.SetHITL(runID, nil)
		}
		return d, nil
	}
	if !d.Approve {
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventHITLRejected,
			Data: map[string]any{"decision": "reject", "comment": d.Comment},
		})
		e.finalizeRejectedRun(runID)
		return d, ErrHITLRejected
	}
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventHITLResumed,
		Data: map[string]any{"decision": "approve", "comment": d.Comment},
	})
	_ = e.Store.UpdateRun(runID, store.StatusRunning, "", "")
	_ = e.Store.SetHITL(runID, nil)
	return d, nil
}

func (e *Engine) persistHITLWaiting(ctx context.Context, runID string, payload *store.HITLPayload) error {
	if payload == nil {
		return fmt.Errorf("nil hitl payload")
	}
	if payload.Kind == "" {
		payload.Kind = store.HITLKindApproval
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.isCancelled(runID) {
		return context.Canceled
	}
	data := map[string]any{
		"prompt":    payload.Prompt,
		"tool_name": payload.ToolName,
		"arguments": payload.Arguments,
		"kind":      payload.Kind,
	}
	if payload.ConnectorID != "" {
		data["connector_id"] = payload.ConnectorID
	}
	_ = e.Store.AppendEvent(runID, store.Event{Type: EventHITLWaiting, Data: data})
	_ = e.Store.UpdateRun(runID, store.StatusWaitingHuman, "", "")
	_ = e.Store.SetHITL(runID, payload)
	e.deliverHITLNotify(runID, payload)
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
