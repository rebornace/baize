package run

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
	"golang.org/x/sync/errgroup"
)

const DefaultParallelToolLimit = 8

// errRunComplete signals runLoop that the run was already finalized (e.g.
// workflow finished) and should return success without another LLM turn.
var errRunComplete = errors.New("run complete")

func (e *Engine) parallelToolLimit() int {
	if e.ParallelToolLimit > 0 {
		return e.ParallelToolLimit
	}
	return DefaultParallelToolLimit
}

// toolCallsNeedSerial reports whether this tool_call batch must run serially
// (activate_skill, approval-gated, login-gated, or identity-aware turns where
// a tool may return login_required and later same-turn calls must be stubbed).
func (e *Engine) toolCallsNeedSerial(ctx context.Context, calls []llm.ToolCall) bool {
	if e.Identities != nil {
		// Keep serial login-pause semantics (stub later calls without invoking).
		return true
	}
	for _, tc := range calls {
		if tc.Name == skill.ActivateToolName {
			return true
		}
		resolved := tc.Name
		if e.Tools != nil {
			if canon, err := e.Tools.ResolveName(tc.Name); err == nil {
				resolved = canon
			}
			if e.Tools.RequiresApproval(resolved) {
				return true
			}
			if info, ok := e.Tools.Get(resolved); ok && info.RequireLogin {
				return true
			}
		}
		if e.blockedByLogin(ctx, resolved) {
			return true
		}
	}
	return false
}

type parallelToolOutcome struct {
	content map[string]any
	isError bool
	err     error
	done    bool
}

// runToolCallsParallel invokes safe read-ish tools concurrently, then appends
// results in the original tool_call order. On NeedsLoginPause it cancels peers
// and stubs later calls.
func (e *Engine) runToolCallsParallel(ctx context.Context, runID string, messages *[]llm.Message, calls []llm.ToolCall) (loginPaused bool, pauseTool string, err error) {
	convID := identity.ConversationIDFrom(ctx)
	n := len(calls)
	out := make([]parallelToolOutcome, n)

	gctx, cancel := context.WithCancel(ctx)
	defer cancel()

	g, gctx := errgroup.WithContext(gctx)
	g.SetLimit(e.parallelToolLimit())

	var pauseMu sync.Mutex
	pauseAt := -1

	for i, tc := range calls {
		i, tc := i, tc
		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return nil // cancelled peer; stubbed on refill
			}
			content, isError, tcErr := e.invokeToolPersist(gctx, runID, tc.ID, tc.Name, tc.Arguments, false, false)
			out[i] = parallelToolOutcome{content: content, isError: isError, err: tcErr, done: true}
			if tcErr != nil && (errors.Is(tcErr, ErrHITLRejected) || errors.Is(tcErr, ErrWaitingHuman)) {
				cancel()
				return tcErr
			}
			if tool.NeedsLoginPause(content) {
				pauseMu.Lock()
				if pauseAt < 0 || i < pauseAt {
					pauseAt = i
				}
				pauseMu.Unlock()
				cancel()
			}
			return nil
		})
	}
	if waitErr := g.Wait(); waitErr != nil && (errors.Is(waitErr, ErrHITLRejected) || errors.Is(waitErr, ErrWaitingHuman)) {
		return false, "", waitErr
	}

	for i, tc := range calls {
		if err := ctx.Err(); err != nil {
			return false, "", err
		}
		if e.isCancelled(runID) {
			return false, "", context.Canceled
		}

		if pauseAt >= 0 && i > pauseAt {
			stub := tool.LoginRequiredContentMsg("已暂停：请先完成浏览器登录后再继续")
			e.persistToolResult(runID, tc.ID, tc.Name, stub, true)
			msg := e.toolResultMessage(tc.ID, stub)
			msg = e.applyToolOffload(ctx, runID, convID, msg, stub, true)
			*messages = append(*messages, msg)
			continue
		}

		res := out[i]
		if !res.done {
			stub := tool.LoginRequiredContentMsg("已暂停：请先完成浏览器登录后再继续")
			e.persistToolResult(runID, tc.ID, tc.Name, stub, true)
			msg := e.toolResultMessage(tc.ID, stub)
			msg = e.applyToolOffload(ctx, runID, convID, msg, stub, true)
			*messages = append(*messages, msg)
			continue
		}
		if res.err != nil && (errors.Is(res.err, ErrHITLRejected) || errors.Is(res.err, ErrWaitingHuman)) {
			return false, "", res.err
		}
		e.persistToolResult(runID, tc.ID, tc.Name, res.content, res.isError)
		msg := e.toolResultMessage(tc.ID, res.content)
		msg = e.applyToolOffload(ctx, runID, convID, msg, res.content, res.isError)
		*messages = append(*messages, msg)
		if tool.NeedsLoginPause(res.content) {
			loginPaused = true
			pauseTool = tc.Name
		}
	}
	return loginPaused, pauseTool, nil
}

// runToolCallsSerial preserves the historical one-at-a-time tool loop (needed
// for activate_skill, HITL, and login-blocked tools).
func (e *Engine) runToolCallsSerial(ctx context.Context, runID string, messages *[]llm.Message, calls []llm.ToolCall) (loginPaused bool, pauseTool string, err error) {
	convID := identity.ConversationIDFrom(ctx)
	for _, tc := range calls {
		if err := ctx.Err(); err != nil {
			return false, "", err
		}
		if e.isCancelled(runID) {
			return false, "", context.Canceled
		}

		if loginPaused {
			stub := tool.LoginRequiredContentMsg("已暂停：请先完成浏览器登录后再继续")
			e.persistToolResult(runID, tc.ID, tc.Name, stub, true)
			msg := e.toolResultMessage(tc.ID, stub)
			msg = e.applyToolOffload(ctx, runID, convID, msg, stub, true)
			*messages = append(*messages, msg)
			continue
		}

		if tc.Name == skill.ActivateToolName {
			content, isError := e.handleActivateSkill(runID, tc.Arguments)
			if !isError && len(*messages) > 0 && (*messages)[0].Role == llm.RoleSystem {
				(*messages)[0].Content = e.composeSystem("", runID)
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
			msg := llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: tc.ID,
				Content:    string(raw),
			}
			msg = e.applyToolOffload(ctx, runID, convID, msg, content, isError)
			*messages = append(*messages, msg)
			if !isError {
				if werr := e.maybeRunWorkflow(ctx, runID); werr != errNoWorkflow {
					if werr != nil {
						return false, "", werr
					}
					return false, "", errRunComplete
				}
			}
			continue
		}

		content, isError, tcErr := e.invokeTool(ctx, runID, tc.ID, tc.Name, tc.Arguments, false)
		if tcErr != nil && (errors.Is(tcErr, ErrHITLRejected) || errors.Is(tcErr, ErrWaitingHuman)) {
			return false, "", tcErr
		}
		msg := e.toolResultMessage(tc.ID, content)
		msg = e.applyToolOffload(ctx, runID, convID, msg, content, isError)
		*messages = append(*messages, msg)
		if tool.NeedsLoginPause(content) {
			loginPaused = true
			pauseTool = tc.Name
		}
	}
	return loginPaused, pauseTool, nil
}

func (e *Engine) runToolCalls(ctx context.Context, runID string, messages *[]llm.Message, calls []llm.ToolCall) (loginPaused bool, pauseTool string, err error) {
	if len(calls) <= 1 || e.parallelToolLimit() <= 1 || e.toolCallsNeedSerial(ctx, calls) {
		return e.runToolCallsSerial(ctx, runID, messages, calls)
	}
	return e.runToolCallsParallel(ctx, runID, messages, calls)
}
