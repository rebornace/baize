package run

import (
	"strings"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

const droppedToolPlaceholder = "[dropped from model-facing context; raw result remains in run events]"

func (e *Engine) contextProjectionEnabled() bool {
	return e != nil && e.Settings != nil && e.Settings.Knobs().ContextProjectionEnabled
}

func (e *Engine) armContextProjection(runID string, rec *store.Run) {
	if rec == nil || rec.ConversationID == "" || !e.contextProjectionEnabled() {
		return
	}
	if e.Compactor == nil || !e.Compactor.exceedsCompactBudget(rec.ConversationID, e.specsForRun(runID), rec.ModelProfileID) {
		return
	}
	e.setContextArmed(runID, true)
}

func (e *Engine) setContextArmed(runID string, armed bool) {
	e.runMu.Lock()
	defer e.runMu.Unlock()
	if st := e.runs[runID]; st != nil {
		st.contextArmed = armed
	}
}

func (e *Engine) contextArmed(runID string) bool {
	e.runMu.Lock()
	defer e.runMu.Unlock()
	if st := e.runs[runID]; st != nil {
		return st.contextArmed
	}
	return false
}

func filterContextTools(specs []llm.ToolSpec, armed bool) []llm.ToolSpec {
	if armed {
		return specs
	}
	out := make([]llm.ToolSpec, 0, len(specs))
	for _, s := range specs {
		if contextToolNames[s.Name] {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (e *Engine) appendContextProjection(messages []llm.Message, conversationID string) []llm.Message {
	if !e.contextProjectionEnabled() || e.Messages == nil || conversationID == "" {
		return messages
	}
	p, ok := e.Messages.GetContextProjection(conversationID)
	if !ok {
		return messages
	}
	var b strings.Builder
	if len(p.Pins) > 0 {
		b.WriteString("必须保留的要点（不要向用户提及这是投影）：\n")
		for _, pin := range p.Pins {
			pin = strings.TrimSpace(pin)
			if pin == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(pin)
			b.WriteByte('\n')
		}
	}
	if s := strings.TrimSpace(p.Summary); s != "" {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("模型侧上下文笔记（不要向用户提及这是投影）：\n\n")
		b.WriteString(s)
	}
	if b.Len() == 0 {
		return messages
	}
	return append(messages, llm.Message{Role: llm.RoleSystem, Content: b.String()})
}

func (e *Engine) applyDroppedToolStubs(messages []llm.Message, conversationID string) {
	if e.Messages == nil || conversationID == "" {
		return
	}
	p, ok := e.Messages.GetContextProjection(conversationID)
	if !ok || len(p.Dropped) == 0 {
		return
	}
	drop := make(map[string]struct{}, len(p.Dropped))
	for _, id := range p.Dropped {
		if id != "" {
			drop[id] = struct{}{}
		}
	}
	for i, m := range messages {
		if m.Role != llm.RoleTool || m.ToolCallID == "" {
			continue
		}
		if _, hit := drop[m.ToolCallID]; !hit {
			continue
		}
		messages[i] = llm.Message{
			Role:       llm.RoleTool,
			ToolCallID: m.ToolCallID,
			Content:    droppedToolPlaceholder,
		}
	}
}

func (e *Engine) loadProjection(convID string) conversation.ContextProjection {
	if e.Messages == nil || convID == "" {
		return conversation.ContextProjection{ConversationID: convID}
	}
	if p, ok := e.Messages.GetContextProjection(convID); ok {
		return p
	}
	return conversation.ContextProjection{ConversationID: convID}
}

func (e *Engine) saveProjection(p conversation.ContextProjection) error {
	if e.Messages == nil {
		return nil
	}
	p.Revision++
	return e.Messages.UpsertContextProjection(p)
}
