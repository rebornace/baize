package api

import (
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
	out := []pendingApprovalJSON{}
	if s.Store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"pending": out, "count": 0})
		return
	}
	runs, err := s.Store.ListWaitingHumanRuns(pendingApprovalsLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	titles := s.conversationTitleMap()
	gate := s.gateTokens().Enabled()
	principal := s.principalFrom(r.Context())
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
	writeJSON(w, http.StatusOK, map[string]any{"pending": out, "count": len(out)})
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
