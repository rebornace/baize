package api

import (
	"context"

	"github.com/rebornace/baize/internal/schedule"
	"github.com/rebornace/baize/internal/store"
)

// ScheduleRunOpener starts scheduled runs via Server.startRun.
type ScheduleRunOpener struct {
	Server *Server
}

// NewScheduleRunOpener returns a schedule.RunOpener backed by the API server.
func NewScheduleRunOpener(s *Server) schedule.RunOpener {
	return ScheduleRunOpener{Server: s}
}

// Open copies agent, identity, and model fields from the conversation's latest run.
func (o ScheduleRunOpener) Open(conversationID, instruction, ownerID string) (string, error) {
	_ = ownerID
	if o.Server == nil || o.Server.Store == nil {
		return "", store.ErrNoPriorConversationRun
	}
	prev, err := o.Server.Store.LatestConversationRun(conversationID)
	if err != nil {
		return "", err
	}
	runRec, err := o.Server.startRun(context.Background(), startRunInput{
		AgentID:        prev.AgentID,
		Input:          instruction,
		ConversationID: conversationID,
		IdentityID:     prev.IdentityID,
		ModelProfileID: prev.ModelProfileID,
		ThinkingLevel:  prev.ThinkingLevel,
	})
	if err != nil {
		return "", err
	}
	return runRec.ID, nil
}
