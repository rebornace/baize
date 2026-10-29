package run

import (
	"context"
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
	"github.com/rebornace/baize/internal/toolindex"
	"github.com/rebornace/baize/internal/workspace"
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
	// EventDecideToolNarrow (DP-2a) records the system routing and the
	// narrowed tool set actually sent to the main model.
	EventDecideToolNarrow = "decide.tool_narrow"
	// EventDecideToolPruned (DP-3, redirected) records one bulky tool result
	// replaced by a short placeholder inside the run so it stops growing the
	// per-turn prompt. The raw result remains in the tool.result event.
	EventDecideToolPruned = "decide.tool_pruned"
	// EventDecideDegraded is the unified observability event for decision-layer
	// fail-open. Every decision point (DP-1/2a/3/4) emits this once when its
	// verdict comes back degraded (or the raw implementation errors), so a
	// single subscription can monitor layer health across all kinds instead of
	// stitching per-event `degraded` flags. The point's normal fail-open
	// behavior is unchanged; this event records only.
	EventDecideDegraded = "decide.degraded"
	// EventTokenEstimate is an observability-only event comparing the
	// token estimator's prompt projection for the payload actually sent with
	// the provider-reported prompt_tokens. It changes no behavior (compaction
	// triggering is untouched); it exists to quantify estimator bias before
	// deciding whether the tool-schema accounting should be corrected.
	EventTokenEstimate = "tokens.estimate"

	DefaultToolTimeout = 60 * time.Second
)

// ErrHITLRejected reports that a human rejected the pending tool approval.
var ErrHITLRejected = errors.New("hitl rejected")

// ErrWaitingHuman reports that the run paused for HITL/login and released the
// worker slot. The run remains waiting_human until ContinueFromHITL (typically
// via a KindHitlResume job) continues it. Callers must not treat this as failure.
var ErrWaitingHuman = errors.New("waiting human")

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
	// ParallelToolLimit caps concurrent safe tool invokes in one LLM turn
	// (default 8). Values <=1 force serial execution.
	ParallelToolLimit int
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
	// Skills is optional. When non-nil and non-empty, runLoop injects
	// activated skill instructions and exposes activate_skill. Skill tool
	// lists are a floor, not an allowlist; enabled connector tools stay
	// candidates for DP-2a routing.
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
	// Workspace optionally persists bulky tool/paste bodies under offload/.
	// nil disables offload writes (fail-open: model keeps the full result).
	Workspace *workspace.Service
	// Settings optionally supplies hot-reloadable engine knobs. nil = use the
	// struct fields above (YAML defaults); non-nil overrides per-field.
	Settings KnobReader
	// ToolEmbedder optionally dense-embeds tool docs + queries for DP-2a
	// Tool-RAG (ToolBench-style). nil ⇒ lexical BM25/sparse fallback only.
	// Mutate only via SetToolEmbedder so the catalog cache is invalidated.
	ToolEmbedder toolindex.Embedder
	// Now overrides the clock for tool argument prechecks (tests). nil uses time.Now.
	Now func() time.Time

	toolIdxCache toolIndexCache
	embedMu      sync.Mutex

	runMu sync.Mutex
	runs  map[string]*runSkillState

	cancelMu sync.Mutex
	cancels  map[string]context.CancelFunc

	// stickyNarrow retains the last DP-2a sent tool set per conversation so
	// small ranking jitter does not reshuffle schemas (prefix-cache friendly).
	stickyMu     sync.Mutex
	stickyNarrow map[string]stickyNarrowState

	// connectorGroups optionally expands DP-2a allowed sources to group siblings.
	connectorGroups *ConnectorGroupIndex

	// DispatchJob enqueues child KindRun jobs (bootstrap wires Server.Dispatch).
	DispatchJob DispatchJobFunc
	// OnChildTerminal wakes the parent with a receipt (bootstrap wires KindChildReceipt).
	OnChildTerminal ChildTerminalFunc
	// childScopeByRun holds in-process jail for depth-1 children.
	childScopeByRun *sync.Map
	// childReceiptOnce dedupes parent wake-ups per child run id.
	childReceiptOnce *sync.Map
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
	// Locale selects localized skill bodies (e.g. "en", "zh-CN"). Empty uses
	// the default SKILL.md body.
	Locale string
}

// SetToolEmbedder hot-swaps the DP-2a dense embedder and clears the tool
// index cache. Pass nil to fall back to lexical retrieval.
func (e *Engine) SetToolEmbedder(emb toolindex.Embedder) {
	if e == nil {
		return
	}
	e.embedMu.Lock()
	defer e.embedMu.Unlock()
	e.ToolEmbedder = emb
	e.toolIdxCache.mu.Lock()
	e.toolIdxCache.fp = ""
	e.toolIdxCache.index = nil
	e.toolIdxCache.mu.Unlock()
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
	exclusive := strings.EqualFold(strings.TrimSpace(ag.ToolBinding), "exclusive")
	e.beginRunSkillsOpts(runID, skills, ag.System, exclusive, opts.Locale, beginRunInput(input))
	e.armContextProjection(runID, runRec)
	e.maybeSeedGoal(runID, runRec.ConversationID, input)
	sys := e.composeSystem(ag.System, runID)
	sys = e.appendSessionAuthHint(sys, runRec.ConversationID)
	sys = e.appendActiveGoalHint(sys)
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

	// Cancel parent → cancel active children first (kill tree).
	if runRec.Depth == 0 {
		if kids, lerr := e.Store.ListChildRuns(runID); lerr == nil {
			for _, c := range kids {
				switch c.Status {
				case store.StatusSucceeded, store.StatusFailed, store.StatusCancelled, store.StatusRejected:
					continue
				}
				_ = e.Cancel(c.ID)
			}
		}
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

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
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
		messages = e.appendActiveGoal(messages, conversationID)
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
		messages = e.appendContextProjection(messages, conversationID)
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
	ctx = applyDecisionPrincipal(e.injectAuthCtx(ctx, runID), d)
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

	if payload.Kind == store.HITLKindWaitingChild {
		// Parent is waiting on a child receipt; operators resume the child run,
		// or cancel the child (which wakes the parent). Do not approve/reject here.
		return fmt.Errorf("parent is waiting_child; resume or cancel child %s", payload.ChildRunID)
	}

	isLoginWait := payload.Kind == store.HITLKindLogin

	if !d.Approve {
		if isLoginWait {
			comment := strings.TrimSpace(d.Comment)
			if comment == store.HITLCommentSkipStep {
				e.skipToolForRun(runID, payload.ToolName)
				_ = e.Store.AppendEvent(runID, store.Event{
					Type: EventHITLResumed,
					Data: map[string]any{
						"decision": "skip_step", "connector_id": payload.ConnectorID,
						"tool_name": payload.ToolName, "kind": store.HITLKindLogin,
					},
				})
				_ = e.Store.UpdateRun(runID, store.StatusRunning, "", "")
				_ = e.Store.SetHITL(runID, nil)
				return e.continueRunAfterLoginWait(ctx, run, runID, payload.ConnectorID, payload.ToolName, loginWaitSkipStep)
			}
			if comment == store.HITLCommentSkipLogin {
				e.skipConnectorForRun(runID, payload.ConnectorID)
				_ = e.Store.AppendEvent(runID, store.Event{
					Type: EventHITLResumed,
					Data: map[string]any{
						"decision": "skip_login", "connector_id": payload.ConnectorID,
						"tool_name": payload.ToolName, "kind": store.HITLKindLogin,
					},
				})
				_ = e.Store.UpdateRun(runID, store.StatusRunning, "", "")
				_ = e.Store.SetHITL(runID, nil)
				return e.continueRunAfterLoginWait(ctx, run, runID, payload.ConnectorID, payload.ToolName, loginWaitSkipSystem)
			}
		}
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventHITLRejected,
			Data: map[string]any{"decision": "reject", "comment": d.Comment},
		})
		e.finalizeRejectedRun(runID)
		return nil
	}

	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventHITLResumed,
		Data: map[string]any{"decision": "approve", "comment": d.Comment, "kind": payload.Kind},
	})
	_ = e.Store.UpdateRun(runID, store.StatusRunning, "", "")
	_ = e.Store.SetHITL(runID, nil)

	if isLoginWait {
		return e.continueRunAfterLoginWait(ctx, run, runID, payload.ConnectorID, payload.ToolName, loginWaitResumed)
	}

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
	sys = e.appendActiveGoalHint(sys)
	messages := e.buildResumeMessages(sys, run.ConversationID, run.Input, evs)
	return e.runLoop(ctx, runID, messages)
}

type loginWaitContinueKind int

const (
	loginWaitResumed loginWaitContinueKind = iota
	loginWaitSkipStep
	loginWaitSkipSystem
)

func (e *Engine) continueRunAfterLoginWait(ctx context.Context, run *store.Run, runID, connectorID, toolName string, kind loginWaitContinueKind) error {
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
	sys = e.appendActiveGoalHint(sys)
	messages := e.buildResumeMessages(sys, run.ConversationID, run.Input, evs)
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: loginWaitContinueHint(kind, connectorID, toolName)})
	return e.runLoop(ctx, runID, messages)
}

func loginWaitContinueHint(kind loginWaitContinueKind, connectorID, toolName string) string {
	switch kind {
	case loginWaitSkipStep:
		tool := strings.TrimSpace(toolName)
		if tool == "" {
			tool = "刚才失败的工具"
		}
		return fmt.Sprintf(
			"[系统] 用户选择忽略当前这一步（工具 %q）的登录要求。请不要再调用该工具；可改用同一业务系统的其它接口、其它已登录系统，或换一种方式继续获取所需数据。",
			tool,
		)
	case loginWaitSkipSystem:
		cid := strings.TrimSpace(connectorID)
		if cid == "" {
			cid = "该业务系统"
		}
		return fmt.Sprintf(
			"[系统] 用户选择忽略业务系统 %q 的登录。请不要再调用该系统的工具；改用其它已登录系统或向用户说明无法完成依赖该系统的部分。",
			cid,
		)
	default:
		return "[系统] 用户已完成浏览器登录。请继续完成原任务，优先重试刚才因登录失败的工具。"
	}
}

func (e *Engine) isCancelled(runID string) bool {
	runRec, err := e.Store.GetRun(runID)
	return err == nil && runRec != nil && runRec.Status == store.StatusCancelled
}

func (e *Engine) runLoop(ctx context.Context, runID string, messages []llm.Message) error {
	// Read once at loop start so the step bound is stable for the whole run
	// even if an operator hot-patches knobs mid-run.
	maxSteps := e.effectiveMaxStepsForRun(runID)
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
		if rec, err := e.Store.GetRun(runID); err == nil && rec != nil {
			e.applyDroppedToolStubs(messages, rec.ConversationID)
			messages = e.refreshActiveGoalMessage(messages, rec.ConversationID)
		}
		turn := step
		specs := e.specsForRun(runID)
		// P1-⑥: mid-run live compaction after prune, before Chat. Folds
		// accumulated assistant/tool turns in *messages*; also opportunistically
		// MaybeCompact's the conversation store for the next run.
		e.maybeCompactInTurn(ctx, runID, &messages, specs)
		// DP-2a: consult the decision layer. In shadow this only records a
		// pick and returns specs unchanged; in enforce it returns the
		// deterministic prefilter set that actually narrows what the model
		// below is allowed to see. Tool execution is via the registry and is
		// unaffected. Only consulted when the tool count exceeds the
		// threshold; small sets are left untouched.
		if e.effectiveDecideTool() && len(specs) > e.effectiveDecideToolThreshold() {
			specs = e.narrowTools(ctx, runID, turn, specs, messages)
		}
		// R1: freeze the first main-model dependency closure before Chat.
		if turn == 0 {
			if err := e.persistRunBinding(runID, turn, specs, messages); err != nil {
				return err
			}
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
		// Upsert off the SSE read loop: one thinking/content delta per turn.
		// Pump must not block token reads — sync SQLite upserts were slower than
		// the model and made Flash feel capped vs other agents.
		pump := newStreamDeltaPump(e.Store, runID, turn)
		co := llm.NewCoalescer(0, pump.Think, pump.Content)
		var msg llm.Message
		var err error
		// DP-2b: when an enum constraint is requested (enforce mode), prefer
		// the streaming constrained call, falling back to a constrained
		// non-streaming call — never silently to an unconstrained one. A
		// provider lacking the optional Chooser capability fails open to the
		// ordinary call (constraint is an optimization, never a hard gate).
		if useChoice := e.effectiveDecideToolChoice(); useChoice {
			choice := llm.ToolChoice{Type: llm.ToolChoiceRequired}
			// Preferred: streaming constrained call.
			if sc, ok := e.LLM.(llm.StreamChooser); ok {
				msg, err = sc.ChatStreamWithChoice(chatCtx, messages, specs, choice, co.Think, co.Content)
			}
			// Streaming unavailable/failed -> still enforce via a non-streaming
			// constrained call.
			if err != nil {
				if c, ok := e.LLM.(llm.Chooser); ok {
					msg, err = c.ChatWithChoice(chatCtx, messages, specs, choice)
				}
			}
			// Provider cannot enforce at all -> fail open to the ordinary
			// (unconstrained) call rather than break the run.
			if err != nil {
				msg, err = e.LLM.Chat(chatCtx, messages, specs)
			}
		} else if st, ok := e.LLM.(llm.Streamer); ok {
			msg, err = st.ChatStream(chatCtx, messages, specs, co.Think, co.Content)
			if err != nil {
				msg, err = e.LLM.Chat(chatCtx, messages, specs)
			}
		} else {
			msg, err = e.LLM.Chat(chatCtx, messages, specs)
		}
		co.Flush()
		pump.Close()
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
				// Estimator-bias observation (no behavior change): messages
				// here is exactly the payload sent; msg is appended below.
				e.emitTokenEstimate(runID, turn, messages, specs, msg.Usage)
			}
			// Strip thinking so in-run history never re-feeds it to the model
			// (eventsAfterInput / buildMessages already omit it).
			msg.Thinking = ""
			msg.ThinkingRedacted = false
			messages = append(messages, msg)
			loginPaused, pauseTool, tcErr := e.runToolCalls(ctx, runID, &messages, msg.ToolCalls)
			if tcErr != nil {
				if errors.Is(tcErr, errRunComplete) {
					return nil
				}
				return tcErr
			}
			if loginPaused {
				connID := e.connectorIDForTool(pauseTool)
				waitErr := e.awaitLoginDecision(ctx, runID, pauseTool, connID)
				switch {
				case waitErr == nil,
					errors.Is(waitErr, ErrLoginResumedContinue),
					errors.Is(waitErr, ErrLoginSkippedContinue),
					errors.Is(waitErr, ErrLoginStepSkippedContinue):
					switch {
					case errors.Is(waitErr, ErrLoginStepSkippedContinue):
						messages = append(messages, llm.Message{
							Role:    llm.RoleUser,
							Content: loginWaitContinueHint(loginWaitSkipStep, connID, pauseTool),
						})
					case errors.Is(waitErr, ErrLoginSkippedContinue):
						messages = append(messages, llm.Message{
							Role:    llm.RoleUser,
							Content: loginWaitContinueHint(loginWaitSkipSystem, connID, pauseTool),
						})
					case errors.Is(waitErr, ErrLoginResumedContinue):
						messages = append(messages, llm.Message{
							Role:    llm.RoleUser,
							Content: loginWaitContinueHint(loginWaitResumed, connID, pauseTool),
						})
					}
					continue // next ReAct turn
				case errors.Is(waitErr, ErrHITLRejected):
					return waitErr
				default:
					return waitErr
				}
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
			// Estimator-bias observation (no behavior change).
			e.emitTokenEstimate(runID, turn, messages, specs, msg.Usage)
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
	// Budget grace: one tools-disabled concluding turn instead of failing
	// immediately (Hermes-style). Does not bump maxSteps / reopen ReAct.
	locale := ""
	if st := e.getRunSkillState(runID); st != nil {
		locale = st.locale
	}
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: budgetGraceHint(locale, maxSteps)})
	graceCtx := ctx
	if rec, err := e.Store.GetRun(runID); err == nil && rec != nil {
		if rec.ModelProfileID != "" {
			graceCtx = llm.WithModelProfileID(ctx, rec.ModelProfileID)
		}
		if rec.ThinkingLevel != "" {
			graceCtx = llm.WithThinkingLevel(graceCtx, rec.ThinkingLevel)
		}
	}
	graceMsg, graceErr := e.LLM.Chat(graceCtx, messages, nil)
	if graceErr == nil && e.isCancelled(runID) {
		return context.Canceled
	}
	if graceErr == nil && strings.TrimSpace(graceMsg.Content) != "" && len(graceMsg.ToolCalls) == 0 {
		msgData := map[string]any{"content": graceMsg.Content, "budget_grace": true}
		if graceMsg.Usage.TotalTokens > 0 {
			for k, v := range usageData(maxSteps, graceMsg.Usage) {
				msgData[k] = v
			}
		}
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventLLMMessage,
			Data: msgData,
		})
		if err := e.Store.UpdateRun(runID, store.StatusSucceeded, graceMsg.Content, ""); err != nil {
			return err
		}
		e.recordTerminalMessage(runID)
		e.triggerMemoryExtract(ctx, runID, graceMsg.Content)
		return nil
	}
	errMsg := fmt.Sprintf("max steps exceeded (%d)", maxSteps)
	if graceErr != nil {
		errMsg = errMsg + ": grace: " + graceErr.Error()
	}
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventLLMError,
		Data: map[string]any{"error": errMsg},
	})
	_ = e.Store.UpdateRun(runID, store.StatusFailed, "", errMsg)
	e.recordTerminalMessage(runID)
	return fmt.Errorf("%s", errMsg)
}

func budgetGraceHint(locale string, maxSteps int) string {
	if isChineseLocale(locale) {
		return fmt.Sprintf(
			"[系统] 本轮推理步数已用尽（上限 %d）。请不要再调用任何工具；仅根据对话与已有工具结果，用现有信息给出尽量完整的结论或说明缺口。",
			maxSteps,
		)
	}
	return fmt.Sprintf(
		"[system] Step budget exhausted (limit %d). Do not call any tools; conclude with the best answer from the conversation and tool results so far, or state what is still missing.",
		maxSteps,
	)
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
	if ws := e.workspaceIDFor(runRec.ConversationID); ws != "" {
		ctx = identity.WithWorkspaceID(ctx, ws)
	}
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

func (e *Engine) workspaceIDFor(conversationID string) string {
	if conversationID == "" || identity.IsolatedConversation(conversationID) {
		return ""
	}
	if e.Meta != nil {
		if m, err := e.Meta.GetMeta(conversationID); err == nil && strings.TrimSpace(m.WorkspaceID) != "" {
			return m.WorkspaceID
		}
	}
	return conversation.DefaultWorkspaceID
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
