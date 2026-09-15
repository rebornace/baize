package run

import (
	"context"
	"strings"

	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

// recordTerminalMessage persists the final assistant / system_note message for
// a run to the conversation store. No-op when Messages is nil or the run has no
// conversation_id. waiting_human is intentionally not recorded (the resume
// path will write the terminal message when it reaches succeeded/failed).
func (e *Engine) recordTerminalMessage(runID string) {
	if e.Messages == nil {
		return
	}
	runRec, err := e.Store.GetRun(runID)
	if err != nil || runRec == nil || runRec.ConversationID == "" {
		return
	}
	if !e.userTurnStillLinked(runRec) {
		// User turn was rolled back; do not resurrect assistant rows for superseded runs.
		return
	}
	switch runRec.Status {
	case store.StatusSucceeded:
		if strings.TrimSpace(runRec.Output) == "" {
			return
		}
		thinking, redacted := e.terminalThinkingFromEvents(runID)
		_, _ = e.Messages.Append(runRec.ConversationID, conversation.Message{
			Role:             conversation.RoleAssistant,
			Content:          runRec.Output,
			Thinking:         thinking,
			ThinkingRedacted: redacted,
			RunID:            runID,
		})
		e.deliverOutbound(runID, runRec.ConversationID, runRec.Output)
	case store.StatusFailed:
		note := strings.TrimSpace(runRec.Error)
		if note == "" {
			note = "运行失败"
		} else {
			note = "运行失败：" + note
		}
		_, _ = e.Messages.Append(runRec.ConversationID, conversation.Message{
			Role:    conversation.RoleSystemNote,
			Content: note,
			RunID:   runID,
		})
	case store.StatusCancelled:
		_, _ = e.Messages.Append(runRec.ConversationID, conversation.Message{
			Role:    conversation.RoleSystemNote,
			Content: "已取消",
			RunID:   runID,
		})
	case store.StatusRejected:
		_, _ = e.Messages.Append(runRec.ConversationID, conversation.Message{
			Role:    conversation.RoleSystemNote,
			Content: HITLRejectedNote,
			RunID:   runID,
		})
	}
}

// deliverOutbound mirrors succeeded assistant text to a channel peer when the
// conversation meta is weixin-backed. Failures are logged inside Deliver; they
// must not change the run's succeeded status.
func (e *Engine) deliverOutbound(runID, conversationID, text string) {
	if e.Outbound == nil || e.Meta == nil || conversationID == "" {
		return
	}
	meta, err := e.Meta.GetMeta(conversationID)
	if err != nil {
		return
	}
	var extras map[string]string
	if e.OutboundExtras != nil {
		extras = e.OutboundExtras(conversationID)
	}
	extras = channel.WithOutboundMeta(extras, channel.OutboundKindAssistant, runID)
	channel.DeliverAssistantReply(context.Background(), e.Outbound, meta, text, nil, extras)
}

// deliverHITLNotify pushes an approval prompt to the weixin peer when a run
// enters waiting_human. Failures are logged only.
func (e *Engine) deliverHITLNotify(runID string, payload *store.HITLPayload) {
	if e.Outbound == nil || e.Meta == nil || runID == "" {
		return
	}
	runRec, err := e.Store.GetRun(runID)
	if err != nil || runRec == nil || runRec.ConversationID == "" {
		return
	}
	meta, err := e.Meta.GetMeta(runRec.ConversationID)
	if err != nil {
		return
	}
	var extras map[string]string
	if e.OutboundExtras != nil {
		extras = e.OutboundExtras(runRec.ConversationID)
	}
	extras = channel.WithOutboundMeta(extras, channel.OutboundKindNotify, runID)
	channel.DeliverUserText(context.Background(), e.Outbound, meta, channel.FormatHITLNotify(payload), extras)
}

func (e *Engine) userTurnStillLinked(runRec *store.Run) bool {
	msgs := e.Messages.List(runRec.ConversationID)
	for _, m := range msgs {
		if m.RunID == runRec.ID && m.Role == conversation.RoleUser {
			return true
		}
	}
	// Runs created before user rows carried run_id: accept when the latest user
	// message still matches this run's input.
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == conversation.RoleUser {
			return msgs[i].Content == runRec.Input
		}
	}
	return false
}

// terminalThinkingFromEvents reads thinking fields from the final llm.message
// event for this run. Used when persisting the conversation assistant row.
func (e *Engine) terminalThinkingFromEvents(runID string) (thinking string, redacted bool) {
	evs, err := e.Store.ListEvents(runID)
	if err != nil {
		return "", false
	}
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type != EventLLMMessage {
			continue
		}
		thinking = asString(evs[i].Data["thinking"])
		redacted = asBool(evs[i].Data["thinking_redacted"])
		return thinking, redacted
	}
	return "", false
}
