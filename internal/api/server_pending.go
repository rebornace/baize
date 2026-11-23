package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

// pendingApprovalsLimit bounds the cross-conversation list; oldest first.
const pendingApprovalsLimit = 100

// defaultPendingStreamLimit bounds concurrent SSE subscribers per process.
const defaultPendingStreamLimit = 100

// pendingStreamWriteTimeout drops a client that cannot accept a frame.
const pendingStreamWriteTimeout = 5 * time.Second

// pendingStreamFallback is a low-frequency safety net when the event bus is
// present. Without a Hub (tests or embedded servers) fall back to the old 2s
// poll so updates are still observed.
const (
	pendingStreamFallback      = 30 * time.Second
	pendingStreamFallbackNoHub = 2 * time.Second
)

func (s *Server) acquirePendingStream() bool {
	limit := s.PendingStreamLimit
	if limit <= 0 {
		limit = defaultPendingStreamLimit
	}
	s.pendingStreamOnce.Do(func() {
		s.pendingStreamSlots = make(chan struct{}, limit)
	})
	select {
	case s.pendingStreamSlots <- struct{}{}:
		s.pendingStreamLive.Add(1)
		return true
	default:
		s.pendingStreamDrop.Add(1)
		return false
	}
}

func (s *Server) releasePendingStream() {
	select {
	case <-s.pendingStreamSlots:
	default:
		return
	}
	s.pendingStreamLive.Add(-1)
}

func (s *Server) pendingStreamStats() (live, dropped, slow int64) {
	return s.pendingStreamLive.Load(), s.pendingStreamDrop.Load(), s.pendingStreamSlow.Load()
}

func writePendingSSE(w http.ResponseWriter, rc *http.ResponseController, payload string) error {
	if err := rc.SetWriteDeadline(time.Now().Add(pendingStreamWriteTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	if _, err := io.WriteString(w, payload); err != nil {
		return err
	}
	return rc.Flush()
}

type pendingApprovalJSON struct {
	RunID             string         `json:"run_id"`
	ConversationID    string         `json:"conversation_id"`
	ConversationTitle string         `json:"conversation_title,omitempty"`
	Kind              string         `json:"kind"`
	ToolName          string         `json:"tool_name,omitempty"`
	Prompt            string         `json:"prompt,omitempty"`
	Arguments         map[string]any `json:"arguments,omitempty"`
	WaitingSince      string         `json:"waiting_since,omitempty"`
	WorkflowSkill     string         `json:"workflow_skill,omitempty"`
	WorkflowStep      string         `json:"workflow_step,omitempty"`
	Origin            string         `json:"origin,omitempty"`
}

// handleListPendingApprovals lists waiting_human runs across conversations so
// the WebUI can show one operator entry instead of requiring the chat to be
// open. Read-only: approval decisions still go through the existing run/HITL
// endpoints.
func (s *Server) handleListPendingApprovals(w http.ResponseWriter, r *http.Request) {
	out, err := s.pendingApprovals(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	live, dropped, slow := s.pendingStreamStats()
	writeJSON(w, http.StatusOK, map[string]any{
		"pending": out, "count": len(out),
		"stream_active": live, "stream_dropped": dropped, "stream_slow": slow,
	})
}

// pendingApprovals builds the cross-conversation list and applies the
// operator ACL. Used by both the REST snapshot and the SSE stream.
func (s *Server) pendingApprovals(ctx context.Context) ([]pendingApprovalJSON, error) {
	out := []pendingApprovalJSON{}
	if s.Store == nil {
		return out, nil
	}
	runs, err := s.Store.ListWaitingHumanRuns(pendingApprovalsLimit)
	if err != nil {
		return nil, err
	}
	titles := s.conversationTitleMap()
	gate := s.gateTokens().Enabled()
	principal := s.principalFrom(ctx)
	for _, runRec := range runs {
		if runRec == nil {
			continue
		}
		if gate && !s.canSeePendingConversation(principal, runRec.ConversationID) {
			continue
		}
		item := pendingApprovalJSON{
			RunID:             runRec.ID,
			ConversationID:    runRec.ConversationID,
			ConversationTitle: titles[runRec.ConversationID],
			Kind:              store.HITLKindApproval,
			Origin:            runRec.Origin,
		}
		if payload, err := s.Store.GetHITL(runRec.ID); err == nil && payload != nil {
			if kind := strings.TrimSpace(payload.Kind); kind != "" {
				item.Kind = kind
			}
			item.ToolName = payload.ToolName
			item.Prompt = payload.Prompt
			item.Arguments = payload.Arguments
			if payload.Workflow != nil {
				item.WorkflowSkill = payload.Workflow.Skill
				item.WorkflowStep = payload.Workflow.StepID
			}
		}
		if since := run.HITLWaitingSince(s.Store, runRec); !since.IsZero() {
			item.WaitingSince = since.UTC().Format(time.RFC3339)
		}
		out = append(out, item)
	}
	return out, nil
}

// handleStreamPendingApprovals pushes the same snapshot as the REST endpoint
// over SSE. HITL changes wake subscribers through the in-process event bus;
// a low-frequency re-read covers missed nudges or cross-replica writes.
func (s *Server) handleStreamPendingApprovals(w http.ResponseWriter, r *http.Request) {
	if !s.acquirePendingStream() {
		w.Header().Set("Retry-After", "5")
		writeError(w, http.StatusTooManyRequests, "too_many_streams", "too many pending-approval streams")
		return
	}
	defer s.releasePendingStream()

	if _, ok := w.(http.Flusher); !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unsupported", "response writer does not support streaming")
		return
	}

	var pendingCh <-chan struct{}
	if s.Hub != nil {
		sub := s.Hub.SubscribePending()
		defer sub.Cancel()
		pendingCh = sub.C
	}

	items, err := s.pendingApprovals(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	rc := http.NewResponseController(w)
	writeSnapshot := func(items []pendingApprovalJSON) (string, error) {
		b, _ := json.Marshal(map[string]any{"pending": items, "count": len(items)})
		return string(b), writePendingSSE(w, rc, "event: pending\ndata: "+string(b)+"\n\n")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if err := writePendingSSE(w, rc, "retry: 3000\n\n"); err != nil {
		s.pendingStreamSlow.Add(1)
		return
	}
	last, err := writeSnapshot(items)
	if err != nil {
		s.pendingStreamSlow.Add(1)
		return
	}

	fallback := pendingStreamFallback
	if s.Hub == nil {
		fallback = pendingStreamFallbackNoHub
	}
	ticker := time.NewTicker(fallback)
	defer ticker.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	refresh := func() bool {
		items, err := s.pendingApprovals(r.Context())
		if err != nil {
			return true
		}
		b, _ := json.Marshal(map[string]any{"pending": items, "count": len(items)})
		if string(b) == last {
			return true
		}
		last, err = writeSnapshot(items)
		if err != nil {
			s.pendingStreamSlow.Add(1)
			return false
		}
		return true
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-pendingCh:
			if !refresh() {
				return
			}
		case <-ticker.C:
			if !refresh() {
				return
			}
		case <-heartbeat.C:
			if err := writePendingSSE(w, rc, ": ping\n\n"); err != nil {
				s.pendingStreamSlow.Add(1)
				return
			}
		}
	}
}

// conversationTitleMap mirrors the titles shown in the sidebar. Failures are
// soft: the list falls back to the conversation id in the UI.
func (s *Server) conversationTitleMap() map[string]string {
	out := map[string]string{}
	if s.Messages == nil {
		return out
	}
	for _, sum := range s.Messages.ListSummaries() {
		out[sum.ID] = sum.Title
	}
	return out
}

// canSeePendingConversation reuses the conversation ACL. A missing meta row
// denies operators (nothing to prove ownership with) and passes admins.
func (s *Server) canSeePendingConversation(p controlplane.Principal, convID string) bool {
	ms := s.metaStore()
	var meta conversation.Meta
	if ms != nil {
		if m, err := ms.GetMeta(convID); err == nil {
			meta = m
		}
	}
	return conversation.CanAccess(p, meta)
}
