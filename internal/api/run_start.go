package api

import (
	"context"
	"strings"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
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
	}

	runRec, err := s.Store.CreateRun(createIn)
	if err != nil {
		return nil, err
	}

	if s.Messages != nil && conv != "" {
		_, _ = s.Messages.Append(conv, conversation.Message{
			Role:    conversation.RoleUser,
			Content: in.Input,
			RunID:   runRec.ID,
		})
		s.deliverWeixinUserOutbound(ctx, conv, in.Input)
	}

	for _, ev := range in.PreEvents {
		_ = s.Store.AppendEvent(runRec.ID, ev)
	}

	def := agent.Def{ID: ag.ID, System: ag.System, Skills: append([]string(nil), ag.Skills...)}
	runOpts := run.RunOptions{Skills: in.Skills, UserParts: in.UserParts}
	_ = s.Store.AppendEvent(runRec.ID, store.Event{Type: run.EventRunStarted})
	go func(runID, input string, def agent.Def, opts run.RunOptions) {
		err := s.runExecute(context.Background(), runID, def, input, opts)
		if err == nil {
			return
		}
		cur, getErr := s.Store.GetRun(runID)
		if getErr != nil || cur == nil {
			return
		}
		switch cur.Status {
		case store.StatusRunning, store.StatusQueued:
			_ = s.Store.UpdateRun(runID, store.StatusFailed, "", err.Error())
			_ = s.Store.AppendEvent(runID, store.Event{
				Type: run.EventLLMError,
				Data: map[string]any{"error": err.Error()},
			})
			if s.Messages != nil && cur.ConversationID != "" {
				note := strings.TrimSpace(err.Error())
				if note == "" {
					note = "运行失败"
				} else {
					note = "运行失败：" + note
				}
				_, _ = s.Messages.Append(cur.ConversationID, conversation.Message{
					Role:    conversation.RoleSystemNote,
					Content: note,
					RunID:   runID,
				})
			}
		}
	}(runRec.ID, in.Input, def, runOpts)

	return s.Store.GetRun(runRec.ID)
}

// deliverWeixinUserOutbound mirrors a UI/API user turn to the weixin peer when
// the conversation is channel-backed. No-op for ui-only conversations.
func (s *Server) deliverWeixinUserOutbound(ctx context.Context, convID, text string) {
	if s == nil || s.WeixinChannel == nil || strings.TrimSpace(text) == "" || strings.TrimSpace(convID) == "" {
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
	if s.WeixinRuntime != nil {
		extras = s.WeixinRuntime.OutboundExtras(convID)
	}
	channel.DeliverUserText(ctx, s.WeixinChannel, meta, text, extras)
}
