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
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/memory"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/skillparse"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

const (
	EventInboxReceived    = "inbox.received"
	EventInboxResumed     = "inbox.resumed"
	EventRunStarted       = "run.started"
	EventLLMToolCall      = "llm.tool_call"
	EventToolResult       = "tool.result"
	EventLLMThinkingDelta = "llm.thinking.delta"
	EventLLMContentDelta  = "llm.content.delta"
	EventLLMThinking      = "llm.thinking"
	EventLLMMessage       = "llm.message"
	EventLLMUsage         = "llm.usage"
	EventLLMError         = "llm.error"
	EventHITLWaiting      = "hitl.waiting"
	EventHITLResumed      = "hitl.resumed"
	EventHITLRejected     = "hitl.rejected"
	EventRunCancelled     = "run.cancelled"
	// EventContextCompacted records a rolling-summary compaction before a run.
	EventContextCompacted = "context.compacted"
	EventWorkflowPrefix   = "workflow."
	// EventModelRouted (DP-0) records the desired capability tier and the
	// resolved profile id when a run is created. Observability only.
	EventModelRouted = "model.routed"
	// EventDecideToolShadow (DP-2a) records the tool candidates the decision
	// layer would keep. Shadow mode writes this without changing the tools
	// actually sent to the model.
	EventDecideToolShadow = "decide.tool_shadow"
	// EventDecideToolPruned (DP-3, redirected) records one bulky tool result
	// replaced by a short placeholder inside the run so it stops growing the
	// per-turn prompt. The raw result remains in the tool.result event.
	EventDecideToolPruned = "decide.tool_pruned"

	DefaultToolTimeout = 60 * time.Second
)

// ErrHITLRejected reports that a human rejected the pending tool approval.
var ErrHITLRejected = errors.New("hitl rejected")

// HITLRejectedNote is the neutral, user-facing system note recorded when a
// human declines an approval. It deliberately avoids "失败"/error wording: a
// rejection is an intentional decision, not a run error.
const HITLRejectedNote = "已按你的选择停止本次操作。"

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
	// Compactor optionally folds older history into a rolling summary before a
	// run when the prompt approaches the model's context limit. nil = disabled
	// (hard sliding window only).
	Compactor *Compactor
	// Identities is optional. When non-nil, tools with RequireLogin are gated
	// before HITL / Invoke when the run has a conversation_id.
	Identities identity.Store
	// Skills is optional. When non-nil and non-empty, runLoop filters tool
	// specs by activated skills and exposes activate_skill.
	Skills *skill.Catalog
	// Meta is optional. When set with Outbound, succeeded assistant replies
	// for weixin conversations are delivered to the channel peer.
	Meta conversation.MetaStore
	// Memory is optional account-scoped fact store (P6). nil disables memory.
	Memory memory.Store
	// Decider optionally consults the decision layer before memory extraction
	// (DP-1). nil disables the layer (legacy behavior); always nil-guard call
	// sites because most test-built Engines do not set it.
	Decider decide.Ask
	// Outbound is optional channel used for UI→peer sync after a succeeded run.
	Outbound channel.Channel
	// OutboundExtras optionally supplies per-conversation extras (e.g. context_token).
	OutboundExtras func(conversationID string) map[string]string
	// ImagePartResolver rebuilds image parts for persisted image_refs on cold
	// resume. nil (or ok=false) degrades that tool result to a text note.
	ImagePartResolver func(conversationID, workspacePath string) (llm.ContentPart, bool)
	// Settings optionally supplies hot-reloadable engine knobs. nil = use the
	// struct fields above (YAML defaults); non-nil overrides per-field.
	Settings KnobReader

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
	if e.Compactor != nil && runRec.ConversationID != "" {
		changed, cerr := e.Compactor.MaybeCompact(ctx, runRec.ConversationID, e.specsForRun(runID), runRec.ModelProfileID)
		if cerr != nil {
			// Compaction is best-effort: record an error event and continue
			// with the hard window. Never block the reply.
			_ = e.Store.AppendEvent(runID, store.Event{
				Type: EventLLMError,
				Data: map[string]any{"error": "context compaction skipped: " + cerr.Error()},
			})
		} else if changed {
			_ = e.Store.AppendEvent(runID, store.Event{
				Type: EventContextCompacted,
				Data: map[string]any{"note": "older history folded into rolling summary"},
			})
		}
	}
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
	case store.StatusSucceeded, store.StatusFailed, store.StatusCancelled, store.StatusRejected:
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
	if e.Settings != nil {
		if d := e.Settings.Knobs().ToolTimeout; d > 0 {
			return d
		}
	}
	if e.ToolTimeout > 0 {
		return e.ToolTimeout
	}
	return DefaultToolTimeout
}

// buildMessages assembles the LLM prompt: system + optional account-memory
// block + (when a rolling summary exists) a summary system message + verbatim
// conversation history + current user input. When a summary exists, the
// verbatim history starts AFTER the summary cursor (CoversThroughOrder): folded
// messages are delivered only via the summary and must never be repeated
// verbatim. Without a summary the hard sliding window (ListWindow) is used
// unchanged. When the most recent history entry is already a user message with
// the same content as input (the API appends the user message before calling
// Execute), the current input is not appended again to avoid a duplicate turn.
//
// When userParts is non-empty, the trailing persisted user message (which carries
// only the display text, without attachment content or image bytes) is replaced
// with a multimodal user message built from userParts. This keeps exactly one
// user turn in the prompt while injecting attachment text and image parts that
// are never persisted to SQLite.
func (e *Engine) buildMessages(system, conversationID, input string, userParts []llm.ContentPart) []llm.Message {
	messages := []llm.Message{{Role: llm.RoleSystem, Content: system}}
	if block := e.memoryBlock(e.memoryOwner(conversationID), input); block != "" {
		messages = append(messages, llm.Message{Role: llm.RoleSystem, Content: block})
	}
	if e.Messages != nil && conversationID != "" {
		hist := e.Messages.ListWindow(conversationID, e.effectiveMaxMessages())
		var sum conversation.RollingSummary
		hasSummary := false
		if s, ok := e.Messages.GetRollingSummary(conversationID); ok && strings.TrimSpace(s.Summary) != "" {
			sum = s
			hasSummary = true
			full := e.Messages.List(conversationID)
			start := sum.CoversThroughOrder + 1
			if start < 0 {
				start = 0
			}
			if start > len(full) {
				start = len(full)
			}
			hist = full[start:]
		}
		if hasSummary {
			messages = append(messages, llm.Message{
				Role:    llm.RoleSystem,
				Content: "以下是较早对话的滚动摘要（供参考，不要向用户提及这是摘要）：\n\n" + sum.Summary,
			})
		}
		for _, m := range hist {
			switch m.Role {
			case conversation.RoleUser:
				// Drop UI-only attachment reference lines (![图片]/[file:] →
				// /v0/channels/media/…) so internal media URLs never reach the
				// model as historical context; attachment bytes are delivered
				// via multimodal parts on their originating turn.
				messages = append(messages, llm.Message{Role: llm.RoleUser, Content: conversation.StripMediaRefs(m.Content)})
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
		if last.Role == llm.RoleUser {
			if last.Content == input {
				return messages
			}
			// Mention-only bubbles keep @id for display; model-facing input is
			// a fallback instruction. Replace this turn instead of appending a
			// second user message.
			if skillparse.IsMentionOnly(last.Content) {
				messages[len(messages)-1] = llm.Message{Role: llm.RoleUser, Content: input}
				return messages
			}
			if strings.TrimSpace(last.Content) == "" {
				messages[len(messages)-1] = llm.Message{Role: llm.RoleUser, Content: input}
				return messages
			}
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
		e.finalizeRejectedRun(runID)
		return nil
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
	e.persistToolResult(runID, toolCallID, payload.ToolName, content, isError)

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

func (e *Engine) isCancelled(runID string) bool {
	runRec, err := e.Store.GetRun(runID)
	return err == nil && runRec != nil && runRec.Status == store.StatusCancelled
}

func (e *Engine) runLoop(ctx context.Context, runID string, messages []llm.Message) error {
	// Read once at loop start so the step bound is stable for the whole run
	// even if an operator hot-patches knobs mid-run.
	maxSteps := e.effectiveMaxSteps()
	var turnThinking []string
	anyRedacted := false

	for step := 0; step < maxSteps; step++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if e.isCancelled(runID) {
			return context.Canceled
		}
		// DP-3 (redirected): before sending this turn, prune bulky tool
		// results accumulated earlier in the run so they stop inflating the
		// prompt. Fail open; on the first turn there is nothing to prune.
		if e.effectiveDecidePrune() {
			e.pruneToolResults(ctx, runID, messages)
		}
		turn := step
		specs := e.specsForRun(runID)
		// DP-2a shadow: record which tools the layer would keep without
		// changing the specs sent below. Only consulted when the tool count
		// exceeds the threshold; small sets are left untouched.
		if e.effectiveDecideTool() && len(specs) > e.effectiveDecideToolThreshold() {
			e.recordToolShadow(ctx, runID, turn, specs)
		}
		chatCtx := ctx
		if rec, err := e.Store.GetRun(runID); err == nil && rec != nil {
			if rec.ModelProfileID != "" {
				chatCtx = llm.WithModelProfileID(ctx, rec.ModelProfileID)
			}
			if rec.ThinkingLevel != "" {
				chatCtx = llm.WithThinkingLevel(chatCtx, rec.ThinkingLevel)
			}
		}
		co := llm.NewCoalescer(100*time.Millisecond, func(s string) {
			_ = e.Store.AppendEvent(runID, store.Event{
				Type: EventLLMThinkingDelta,
				Data: map[string]any{"turn": turn, "text": s},
			})
		}, func(s string) {
			_ = e.Store.AppendEvent(runID, store.Event{
				Type: EventLLMContentDelta,
				Data: map[string]any{"turn": turn, "text": s},
			})
		})
		var msg llm.Message
		var err error
		if st, ok := e.LLM.(llm.Streamer); ok {
			msg, err = st.ChatStream(chatCtx, messages, specs, co.Think, co.Content)
			if err != nil {
				msg, err = e.LLM.Chat(chatCtx, messages, specs)
			}
		} else {
			msg, err = e.LLM.Chat(chatCtx, messages, specs)
		}
		co.Flush()
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
		if strings.TrimSpace(msg.Thinking) != "" || msg.ThinkingRedacted {
			_ = e.Store.AppendEvent(runID, store.Event{
				Type: EventLLMThinking,
				Data: map[string]any{
					"turn":              turn,
					"text":              msg.Thinking,
					"thinking_redacted": msg.ThinkingRedacted,
				},
			})
		}
		if strings.TrimSpace(msg.Thinking) != "" {
			turnThinking = append(turnThinking, msg.Thinking)
		}
		if msg.ThinkingRedacted {
			anyRedacted = true
		}

		if len(msg.ToolCalls) > 0 {
			// Record the real cost of this call before any tool work. Only
			// reported usage is persisted; mock/unreporting providers leave a
			// zero value and are skipped rather than recording a false 0.
			if msg.Usage.TotalTokens > 0 {
				_ = e.Store.AppendEvent(runID, store.Event{
					Type: EventLLMUsage,
					Data: usageData(turn, msg.Usage),
				})
			}
			// Strip thinking so in-run history never re-feeds it to the model
			// (eventsAfterInput / buildMessages already omit it).
			msg.Thinking = ""
			msg.ThinkingRedacted = false
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
				messages = append(messages, e.toolResultMessage(tc.ID, content))
			}
			continue
		}

		if e.isCancelled(runID) {
			return context.Canceled
		}
		joined := llm.JoinThinking(turnThinking)
		msgData := map[string]any{"content": msg.Content}
		if joined != "" || anyRedacted {
			msgData["thinking"] = joined
			if anyRedacted && joined == "" {
				msgData["thinking_redacted"] = true
			}
		}
		if msg.Usage.TotalTokens > 0 {
			for k, v := range usageData(turn, msg.Usage) {
				msgData[k] = v
			}
		}
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventLLMMessage,
			Data: msgData,
		})
		if err := e.Store.UpdateRun(runID, store.StatusSucceeded, msg.Content, ""); err != nil {
			return err
		}
		e.recordTerminalMessage(runID)
		e.triggerMemoryExtract(ctx, runID, msg.Content)
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

func asString(v any) string {
	s, _ := v.(string)
	return s
}

// usageData serializes a turn's real usage for event persistence.
func usageData(turn int, u llm.Usage) map[string]any {
	return map[string]any{
		"turn":              turn,
		"prompt_tokens":     u.PromptTokens,
		"completion_tokens": u.CompletionTokens,
		"total_tokens":      u.TotalTokens,
		"cached_tokens":     u.CachedTokens,
	}
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}
