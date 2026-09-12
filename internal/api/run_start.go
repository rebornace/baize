package api

import (
	"context"
	"strings"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/middleware"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

type startRunInput struct {
	AgentID, Input, ConversationID, IdentityID string
	Skills                                     []string
	Webhook                                    *store.WebhookConfig
	Passthrough                                map[string]string
	UserParts                                  []llm.ContentPart
	PreEvents                                  []store.Event
	ModelProfileID                             string
	// BubbleContent is the exact text persisted to the conversation message
	// for display. It may carry UI-only attachment reference lines
	// (![图片](…)/[file:…](…)) that must not reach the model or a mirrored
	// channel peer. When empty, Input is persisted.
	BubbleContent string
}

func (s *Server) startRun(ctx context.Context, in startRunInput) (*store.Run, error) {
	ag, err := s.Store.GetAgent(in.AgentID)
	if err != nil {
		return nil, err
	}

	conv := strings.TrimSpace(in.ConversationID)
	if err := s.prepareConversationMeta(ctx, conv); err != nil {
		return nil, err
	}

	createIn := store.CreateRunInput{
		AgentID:            in.AgentID,
		Input:              in.Input,
		ConversationID:     in.ConversationID,
		IdentityID:         in.IdentityID,
		PassthroughHeaders: in.Passthrough,
		WebhookConfig:      in.Webhook,
		ModelProfileID:     in.ModelProfileID,
	}

	runRec, err := s.Store.CreateRun(createIn)
	if err != nil {
		return nil, err
	}

	// The persisted conversation bubble may carry UI-only attachment reference
	// lines; the run record, dispatched job and mirrored channel peer all use
	// the clean model-facing Input.
	bubble := in.BubbleContent
	if bubble == "" {
		bubble = in.Input
	}
	if s.Messages != nil && conv != "" {
		_, _ = s.Messages.Append(conv, conversation.Message{
			Role:    conversation.RoleUser,
			Content: bubble,
			RunID:   runRec.ID,
		})
		s.deliverUserOutbound(ctx, runRec.ID, conv, in.Input)
	}

	for _, ev := range in.PreEvents {
		_ = s.Store.AppendEvent(runRec.ID, ev)
	}

	def := agent.Def{ID: ag.ID, System: ag.System, Skills: append([]string(nil), ag.Skills...)}
	runOpts := run.RunOptions{Skills: in.Skills, UserParts: in.UserParts}
	_ = s.Store.AppendEvent(runRec.ID, store.Event{Type: run.EventRunStarted})
	s.Dispatch(ctx, middleware.Job{
		RunID:     runRec.ID,
		Kind:      middleware.KindRun,
		AgentID:   def.ID,
		Input:     in.Input,
		Skills:    runOpts.Skills,
		UserParts: PartsToMiddleware(runOpts.UserParts),
	})

	return s.Store.GetRun(runRec.ID)
}

// deliverUserOutbound mirrors a UI/API user turn to the channel peer when the
// conversation is channel-backed. Outbound may be a concrete channel (single
// channel deployment) or a *channel.Router; channel.DeliverUserText resolves
// the child channel by meta.Source. No-op for ui-only conversations.
func (s *Server) deliverUserOutbound(ctx context.Context, runID, convID, text string) {
	if s == nil || s.Outbound == nil || strings.TrimSpace(text) == "" || strings.TrimSpace(convID) == "" {
		return
	}
	ms := s.metaStore()
	if ms == nil {
		return
	}
	meta, err := ms.GetMeta(convID)
	if err != nil {
		return
	}
	var extras map[string]string
	if s.OutboundExtras != nil {
		extras = s.OutboundExtras(convID)
	}
	extras = channel.WithOutboundMeta(extras, channel.OutboundKindOperator, runID)
	channel.DeliverUserText(ctx, s.Outbound, meta, text, extras)
}
