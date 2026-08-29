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
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/workflow"
)

const (
	EventInboxReceived  = "inbox.received"
	EventInboxResumed   = "inbox.resumed"
	EventRunStarted     = "run.started"
	EventLLMToolCall    = "llm.tool_call"
	EventToolResult     = "tool.result"
	EventLLMMessage     = "llm.message"
	EventLLMError       = "llm.error"
	EventHITLWaiting    = "hitl.waiting"
	EventHITLResumed    = "hitl.resumed"
	EventHITLRejected   = "hitl.rejected"
	EventRunCancelled   = "run.cancelled"
	EventWorkflowPrefix = "workflow."

	DefaultToolTimeout = 60 * time.Second
)

// ErrHITLRejected reports that a human rejected the pending tool approval.
var ErrHITLRejected = errors.New("hitl rejected")

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
	// Meta is optional. When set with Outbound, succeeded assistant replies
	// for weixin conversations are delivered to the channel peer.
	Meta conversation.MetaStore
	// Outbound is optional channel used for UI→peer sync after a succeeded run.
	Outbound channel.Channel
	// OutboundExtras optionally supplies per-conversation extras (e.g. context_token).
	OutboundExtras func(conversationID string) map[string]string

	runMu sync.Mutex
	runs  map[string]*runSkillState

	cancelMu sync.Mutex
	cancels  map[string]context.CancelFunc
}

// RunOptions carries per-run overrides for ExecuteWithOpts.
type RunOptions struct {
	// Skills overrides the agent's default skills for this run. nil = use
	// agent.Def.Skills; a non-nil slice (including an empty slice) replaces
	// the default set for this run (empty = explicit clear, no skill active
	// beyond activate_skill).
	Skills []string
	// UserParts is an optional multimodal payload for the current user turn.
	// When non-empty, the engine sends the user message as Parts (text +
	// image parts) instead of a plain Content string, and replaces the
	// trailing persisted user message (which carries only the display text)
	// with this multimodal version so the LLM sees exactly one user turn.
	UserParts []llm.ContentPart
}

func (e *Engine) Execute(ctx context.Context, runID string, ag agent.Def, input string) error {
	return e.ExecuteWithOpts(ctx, runID, ag, input, RunOptions{})
}

// ExecuteWithOpts is the per-run entry point. See RunOptions for the semantics
// of each override. It is safe to call via Execute (zero opts) for callers that
// do not need per-run skills or multimodal user content.
func (e *Engine) ExecuteWithOpts(ctx context.Context, runID string, ag agent.Def, input string, opts RunOptions) error {
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
	skills := opts.Skills
	if skills == nil {
		skills = ag.Skills
	}
	e.beginRunSkills(runID, skills, ag.System, beginRunInput(input))
	sys := e.composeSystem(ag.System, runID)
	sys = e.appendSessionAuthHint(sys, runRec.ConversationID)
	messages := e.buildMessages(sys, runRec.ConversationID, input, opts.UserParts)
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
//
// When userParts is non-empty, the trailing persisted user message (which carries
// only the display text, without attachment content or image bytes) is replaced
// with a multimodal user message built from userParts. This keeps exactly one
// user turn in the prompt while injecting attachment text and image parts that
// are never persisted to SQLite.
func (e *Engine) buildMessages(system, conversationID, input string, userParts []llm.ContentPart) []llm.Message {
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
	if len(userParts) > 0 {
		if len(messages) > 0 && messages[len(messages)-1].Role == llm.RoleUser {
			messages[len(messages)-1] = llm.Message{Role: llm.RoleUser, Parts: userParts}
		} else {
			messages = append(messages, llm.Message{Role: llm.RoleUser, Parts: userParts})
		}
		return messages
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

	evs0, err := e.Store.ListEvents(runID)
	if err != nil {
		return err
	}
	// Cold resume into an in-flight workflow run: reject BEFORE invoking the
	// pending step — the persisted tree/step state cannot be rebuilt, and
	// firing side effects from a broken pipeline is worse than stopping.
	if workflowInterrupted(evs0) {
		errMsg := "workflow run interrupted by restart; please re-run"
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventLLMError,
			Data: map[string]any{"error": errMsg},
		})
		_ = e.Store.UpdateRun(runID, store.StatusFailed, "", errMsg)
		e.recordTerminalMessage(runID)
		return fmt.Errorf("%s", errMsg)
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
		e.beginRunSkills(runID, append([]string(nil), ag.Skills...), ag.System, beginRunInput(run.Input))
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
	messages := e.buildMessages(system, conversationID, input, nil)
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
					if !isError {
						if werr := e.maybeRunWorkflow(ctx, runID); werr != errNoWorkflow {
							return werr
						}
					}
					continue
				}

				content, _, tcErr := e.invokeTool(ctx, runID, tc.ID, tc.Name, tc.Arguments, false)
				if tcErr != nil && errors.Is(tcErr, ErrHITLRejected) {
					return tcErr
				}
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

// awaitHITL is the ReAct-path wrapper: shapes the payload from the LLM tool
// call and delegates to the payload-level kernel.
func (e *Engine) awaitHITL(ctx context.Context, runID string, tc llm.ToolCall) error {
	return e.awaitHITLPayload(ctx, runID,
		fmt.Sprintf("Approve tool %s?", tc.Name), tc.Name, tc.Arguments)
}

var errNoWorkflow = errors.New("run has no workflow")

func beginRunInput(input string) map[string]any {
	return map[string]any{"text": input}
}

// maybeRunWorkflow switches a run into pipeline mode when the just-activated
// skill carries a workflow.yaml, executes it to completion, and finalizes the
// run. It returns errNoWorkflow when no workflow applies.
func (e *Engine) maybeRunWorkflow(ctx context.Context, runID string) error {
	st := e.getRunSkillState(runID)
	if st == nil || st.workflowStarted || e.Skills == nil {
		return errNoWorkflow
	}
	var wf *workflow.Workflow
	for _, id := range st.activated {
		if pkg, ok := e.Skills.Get(id); ok && pkg.Workflow != nil {
			wf = pkg.Workflow
			break
		}
	}
	if wf == nil {
		return errNoWorkflow
	}

	e.runMu.Lock()
	st.workflowStarted = true
	st.workflowSkill = wf.Name
	e.runMu.Unlock()

	stepApproved := make(map[string]bool, len(wf.Steps))
	for _, s := range wf.Steps {
		stepApproved[s.ID] = s.Approve
	}

	werr := wf.Run(ctx, st.workflowResults, workflow.ExecHooks{
		Emit: func(typ string, data map[string]any) error {
			return e.Store.AppendEvent(runID, store.Event{Type: typ, Data: data})
		},
		Gate: func(gctx context.Context, p store.HITLPayload) (bool, error) {
			if err := e.awaitHITLPayload(gctx, runID, p.Prompt, p.ToolName, p.Arguments); err != nil {
				if errors.Is(err, ErrHITLRejected) {
					return false, nil
				}
				return false, err
			}
			return true, nil
		},
		Invoke: func(ictx context.Context, toolName string, stepID string, args map[string]any) (map[string]any, bool, error) {
			callID := fmt.Sprintf("wf-%s-%s", st.workflowSkill, stepID)
			return e.invokeTool(ictx, runID, callID, toolName, args, stepApproved[stepID])
		},
	})
	if werr != nil {
		return e.finalizeFailedRun(runID, werr)
	}

	content := "workflow completed"
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventLLMMessage,
		Data: map[string]any{"content": content},
	})
	if err := e.Store.UpdateRun(runID, store.StatusSucceeded, content, ""); err != nil {
		return err
	}
	e.recordTerminalMessage(runID)
	return nil
}

func (e *Engine) finalizeFailedRun(runID string, cause error) error {
	errMsg := cause.Error()
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventLLMError,
		Data: map[string]any{"error": errMsg},
	})
	_ = e.Store.UpdateRun(runID, store.StatusFailed, "", errMsg)
	e.recordTerminalMessage(runID)
	return cause
}

// workflowInterrupted reports whether the event stream shows a workflow run
// that started but never reached a terminal event (llm.message completion or
// llm.error failure) — i.e. a cold resume would land mid-pipeline.
func workflowInterrupted(evs []store.Event) bool {
	started := false
	for _, ev := range evs {
		switch ev.Type {
		case EventWorkflowPrefix + "started":
			started = true
		case "workflow.completed", EventLLMMessage, EventLLMError:
			started = false
		}
	}
	return started
}

// invokeTool performs one full tool interaction for a run: pre-call gate
// (login / approval) and events, then the timeout-bounded Invoke with the same
// event/data shapes as before. A rejection returns ErrHITLRejected and the run
// has already been finalized as failed; the caller must stop instead of
// appending further results. Transient failures inside one interaction keep the
// last-known return shape so the value/error contract stays uniform.
func (e *Engine) invokeTool(ctx context.Context, runID, callID, name string, args map[string]any, skipApproval bool) (map[string]any, bool, error) {
	isError := false
	content := map[string]any{}

	rejected, rerr := func() (bool, error) {
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventLLMToolCall,
			Data: map[string]any{"id": callID, "name": name, "arguments": args},
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
		c, _, ierr := e.Tools.Invoke(toolCtx, name, args)
		cancel()
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

	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventToolResult,
		Data: map[string]any{
			"tool_call_id": callID,
			"name":         name,
			"content":      identity.RedactSensitive(content),
			"is_error":     isError,
		},
	})
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
		e.deliverOutbound(runRec.ConversationID, runRec.Output)
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

// deliverOutbound mirrors succeeded assistant text to a channel peer when the
// conversation meta is weixin-backed. Failures are logged inside Deliver; they
// must not change the run's succeeded status.
func (e *Engine) deliverOutbound(conversationID, text string) {
	if e.Outbound == nil || e.Meta == nil || conversationID == "" {
		return
	}
	meta, err := e.Meta.GetMeta(conversationID)
	if err != nil {
		return
	}
	var extras map[string]string
	if e.OutboundExtras != nil {
		extras = e.OutboundExtras(conversationID)
	}
	channel.DeliverAssistantReply(context.Background(), e.Outbound, meta, text, nil, extras)
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
