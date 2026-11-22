package api

import (
	"context"
	"encoding/json"
	"fmt"
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
	writeJSON(w, http.StatusOK, map[string]any{"pending": out, "count": len(out)})
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
// over SSE. It still polls the store (2s) rather than adding a pub/sub layer,
// but removes the 15s client polling lag and keeps one connection per UI.
func (s *Server) handleStreamPendingApprovals(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unsupported", "response writer does not support streaming")
		return
	}
	items, err := s.pendingApprovals(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeSnapshot := func(items []pendingApprovalJSON) string {
		b, _ := json.Marshal(map[string]any{"pending": items, "count": len(items)})
		fmt.Fprintf(w, "event: pending\ndata: %s\n\n", b)
		flusher.Flush()
		return string(b)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "retry: 3000\n\n")
	last := writeSnapshot(items)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			items, err := s.pendingApprovals(r.Context())
			if err != nil {
				continue
			}
			b, _ := json.Marshal(map[string]any{"pending": items, "count": len(items)})
			if string(b) == last {
				continue
			}
			last = writeSnapshot(items)
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
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
