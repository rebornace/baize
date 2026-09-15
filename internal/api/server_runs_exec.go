package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/authcred"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/middleware"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

// runExecute dispatches to a RunWithOptions runner when available so per-run
// skills and multimodal user parts are honored; otherwise it falls back to the
// plain Execute path. Test fakes that only implement Runner still work.
func (s *Server) runExecute(ctx context.Context, runID string, def agent.Def, input string, opts run.RunOptions) error {
	if ex, ok := s.Runner.(RunWithOptions); ok {
		return ex.ExecuteWithOpts(ctx, runID, def, input, opts)
	}
	return s.Runner.Execute(ctx, runID, def, input)
}

// ExecuteJob runs a queued run under a worker lease with idempotency gating.
// It implements middleware.Executor: queue workers and the local fallback
// goroutine both funnel through it. Terminal or HITL-waiting runs are
// ack-skipped (resume is a separate entry point); a live lease held by
// another worker is also ack-skipped.
func (s *Server) ExecuteJob(ctx context.Context, job middleware.Job) error {
	if job.RunID == "" {
		return nil
	}
	cur, err := s.Store.GetRun(job.RunID)
	if err != nil || cur == nil {
		return err
	}
	switch cur.Status {
	case store.StatusSucceeded, store.StatusFailed, store.StatusCancelled, store.StatusRejected, store.StatusWaitingHuman:
		return nil
	}

	ttl := s.LeaseTTL
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	acquired, err := s.Store.LeaseRun(job.RunID, ttl)
	if err != nil {
		return err
	}
	if !acquired {
		return nil // another worker holds the lease
	}
	defer func() { _ = s.Store.ClearRunLease(job.RunID) }()

	hbCtx, stopHB := context.WithCancel(ctx)
	defer stopHB()
	go s.leaseHeartbeat(hbCtx, job.RunID, ttl)

	def, input, opts, err := s.resolveJob(job, cur)
	if err != nil {
		s.finalizeRunError(job.RunID, cur.ConversationID, err)
		return nil
	}
	if err := s.runExecute(ctx, job.RunID, def, input, opts); err != nil {
		// A human declining an approval is an intentional terminal outcome
		// (run settled as "rejected"), not a job failure to log/retry.
		if errors.Is(err, run.ErrHITLRejected) {
			return nil
		}
		s.finalizeRunError(job.RunID, cur.ConversationID, err)
		return err
	}
	return nil
}

// leaseHeartbeat renews the worker lease roughly every ttl/3 until ctx ends.
func (s *Server) leaseHeartbeat(ctx context.Context, runID string, ttl time.Duration) {
	t := time.NewTicker(ttl / 3)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = s.Store.HeartbeatRun(runID, ttl)
		}
	}
}

// resolveJob rebuilds the agent def, input, and run options for a queued job,
// falling back to the persisted run row when the job omits a field.
func (s *Server) resolveJob(job middleware.Job, cur *store.Run) (agent.Def, string, run.RunOptions, error) {
	agentID := strings.TrimSpace(job.AgentID)
	if agentID == "" {
		agentID = cur.AgentID
	}
	ag, err := s.Store.GetAgent(agentID)
	if err != nil {
		return agent.Def{}, "", run.RunOptions{}, err
	}
	def := agent.Def{ID: ag.ID, System: ag.System, Skills: append([]string(nil), ag.Skills...)}
	input := job.Input
	if strings.TrimSpace(input) == "" {
		input = cur.Input
	}
	opts := run.RunOptions{Skills: job.Skills, UserParts: partsFromMiddleware(job.UserParts)}
	return def, input, opts, nil
}

// finalizeRunError mirrors the legacy background-goroutine failure handling:
// re-read the run, and only when it is still queued/running mark it failed,
// append an LLM error event, and leave a system note in the conversation.
func (s *Server) finalizeRunError(runID, convID string, runErr error) {
	cur, _ := s.Store.GetRun(runID)
	if cur == nil {
		return
	}
	if cur.Status != store.StatusRunning && cur.Status != store.StatusQueued {
		return
	}
	_ = s.Store.UpdateRun(runID, store.StatusFailed, "", runErr.Error())
	_ = s.Store.AppendEvent(runID, store.Event{
		Type: run.EventLLMError,
		Data: map[string]any{"error": runErr.Error()},
	})
	if s.Messages != nil && convID != "" {
		note := strings.TrimSpace(runErr.Error())
		if note == "" {
			note = "运行失败"
		} else {
			note = "运行失败：" + note
		}
		_, _ = s.Messages.Append(convID, conversation.Message{
			Role:    conversation.RoleSystemNote,
			Content: note,
			RunID:   runID,
		})
	}
}

// Dispatch enqueues a job when a Queue is configured, falling back to a local
// goroutine (legacy behavior) on nil queue or enqueue failure.
func (s *Server) Dispatch(ctx context.Context, job middleware.Job) {
	if job.EnqueuedAt.IsZero() {
		job.EnqueuedAt = time.Now().UTC()
	}
	if s.Queue != nil {
		if err := s.Queue.Enqueue(ctx, job); err == nil {
			return
		}
	}
	go func() { _ = s.ExecuteJob(context.Background(), job) }()
}

type executeRunOpts struct {
	skipUserAppend bool
	rebindUserID   string
	// runOpts carries per-run skill overrides and multimodal user parts. The
	// regenerate path leaves this zero-valued (reuse agent defaults, no parts)
	// since attachments are not re-sent on regenerate.
	runOpts run.RunOptions
}

// createAndExecuteRun creates a run, appends the user message, and starts Execute in background.
func (s *Server) createAndExecuteRun(r *http.Request, agentID, input, convID, identityID string, opts executeRunOpts) (*store.Run, error) {
	ag, err := s.Store.GetAgent(agentID)
	if err != nil {
		return nil, fmt.Errorf("unknown agent")
	}
	if err := s.prepareConversationMeta(r.Context(), convID); err != nil {
		return nil, err
	}
	createIn := store.CreateRunInput{
		AgentID:        agentID,
		Input:          input,
		ConversationID: convID,
		IdentityID:     strings.TrimSpace(identityID),
	}
	if authcred.NormalizeMode(s.AuthMode) == authcred.ModePassthrough {
		createIn.PassthroughHeaders = authcred.PickHeaders(r.Header, s.AuthWhitelist)
	}
	runRec, err := s.Store.CreateRun(createIn)
	if err != nil {
		return nil, err
	}
	if opts.skipUserAppend && opts.rebindUserID != "" {
		if s.Messages == nil {
			return nil, fmt.Errorf("message store not configured")
		}
		if err := s.Messages.SetRunID(opts.rebindUserID, runRec.ID); err != nil {
			return nil, err
		}
	} else if s.Messages != nil && convID != "" {
		_, _ = s.Messages.Append(convID, conversation.Message{
			Role:    conversation.RoleUser,
			Content: input,
			RunID:   runRec.ID,
		})
	}
	def := agent.Def{ID: ag.ID, System: ag.System, Skills: append([]string(nil), ag.Skills...)}
	_ = s.Store.AppendEvent(runRec.ID, store.Event{Type: run.EventRunStarted})
	s.Dispatch(r.Context(), middleware.Job{
		RunID:     runRec.ID,
		Kind:      middleware.KindRun,
		AgentID:   def.ID,
		Input:     input,
		Skills:    opts.runOpts.Skills,
		UserParts: PartsToMiddleware(opts.runOpts.UserParts),
	})
	return runRec, nil
}
