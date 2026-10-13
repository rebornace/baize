package run

import (
	"strings"
	"unicode/utf8"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

const EventGoalSeeded = "goal.seeded"

func (e *Engine) activeGoalEnabled() bool {
	return e != nil && e.Settings != nil && e.Settings.Knobs().ActiveGoalEnabled
}

func (e *Engine) appendActiveGoal(messages []llm.Message, conversationID string) []llm.Message {
	if !e.activeGoalEnabled() || e.Messages == nil || conversationID == "" {
		return messages
	}
	stack, ok := e.Messages.GetGoalStack(conversationID)
	if !ok || len(stack.Frames) == 0 {
		return messages
	}
	if block := goalBlock(stack); block != "" {
		messages = append(messages, llm.Message{Role: llm.RoleSystem, Content: block})
	}
	return messages
}

func goalBlock(stack conversation.GoalStack) string {
	root, hasRoot := conversation.Root(stack)
	if !hasRoot {
		return ""
	}
	tip, hasTip := conversation.Tip(stack)
	if !hasTip {
		if !conversation.HasInterruptedFrame(stack) {
			return ""
		}
		text := strings.TrimSpace(root.Text)
		if text == "" {
			return ""
		}
		return "【主目标】" + text + "（已暂停，可继续）"
	}
	if len(stack.Frames) == 1 {
		var b strings.Builder
		b.WriteString("【当前目标】")
		b.WriteString(strings.TrimSpace(tip.Text))
		writeGoalDetails(&b, tip.Details)
		writeGoalCriteria(&b, tip.Criteria)
		return b.String()
	}
	if root.ID == tip.ID {
		if interrupted, ok := conversation.InterruptedTip(stack); ok {
			var b strings.Builder
			b.WriteString("【主目标】")
			b.WriteString(strings.TrimSpace(root.Text))
			writeGoalDetails(&b, root.Details)
			b.WriteByte('\n')
			b.WriteString("【已暂停焦点】")
			b.WriteString(strings.TrimSpace(interrupted.Text))
			writeGoalDetails(&b, interrupted.Details)
			return b.String()
		}
	}
	var b strings.Builder
	b.WriteString("【主目标】")
	b.WriteString(strings.TrimSpace(root.Text))
	writeGoalDetails(&b, root.Details)
	b.WriteByte('\n')
	b.WriteString("【当前焦点】")
	b.WriteString(strings.TrimSpace(tip.Text))
	writeGoalDetails(&b, tip.Details)
	writeGoalCriteria(&b, tip.Criteria)
	return b.String()
}

func writeGoalDetails(b *strings.Builder, details string) {
	details = strings.TrimSpace(details)
	if details == "" {
		return
	}
	b.WriteByte('\n')
	b.WriteString("【详情】")
	b.WriteString(details)
}

func writeGoalCriteria(b *strings.Builder, criteria []string) {
	var lines []string
	for _, c := range criteria {
		c = strings.TrimSpace(c)
		if c != "" {
			lines = append(lines, c)
		}
	}
	if len(lines) == 0 {
		return
	}
	b.WriteByte('\n')
	b.WriteString("【验收】")
	for _, c := range lines {
		b.WriteByte('\n')
		b.WriteString("- ")
		b.WriteString(c)
	}
}

func (e *Engine) maybeSeedGoal(runID, conversationID, input string) {
	if !e.activeGoalEnabled() || e.Messages == nil || conversationID == "" {
		return
	}
	if stack, ok := e.Messages.GetGoalStack(conversationID); ok && len(stack.Frames) > 0 {
		return
	}
	raw := truncateGoalSeedText(input, e.activeGoalMaxTextRunes()+conversation.DefaultMaxGoalDetailsRunes)
	if raw == "" {
		return
	}
	text, details := conversation.SplitGoalTitleAndDetails(raw)
	text = truncateGoalSeedText(text, e.activeGoalMaxTextRunes())
	if text == "" {
		return
	}
	if utf8.RuneCountInString(details) > conversation.DefaultMaxGoalDetailsRunes {
		details = string([]rune(details)[:conversation.DefaultMaxGoalDetailsRunes])
	}
	stack, err := conversation.SetRootDetailed(conversation.GoalStack{}, conversationID, text, details, nil, conversation.GoalSourceHarness)
	if err != nil {
		return
	}
	if err := e.Messages.UpsertGoalStack(stack); err != nil {
		return
	}
	if e.Store != nil && runID != "" {
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventGoalSeeded,
			Data: map[string]any{
				"conversation_id": conversationID,
				"revision":        stack.Revision,
			},
		})
	}
}

func (e *Engine) appendActiveGoalHint(system string) string {
	if !e.activeGoalEnabled() {
		return system
	}
	const marker = "goal_set"
	if strings.Contains(system, marker) && strings.Contains(system, "【详情】") {
		return system
	}
	hint := "任务目标：用 goal_set / goal_update 维护短标题（一句话）；用户目标含糊时先记下再随对话补全。" +
		"较长背景与约束写入 details（注入为【详情】），可验收条目写入 criteria。" +
		"勿把整段用户原文当作目标标题。"
	if strings.TrimSpace(system) == "" {
		return hint
	}
	return system + "\n\n" + hint
}

func (e *Engine) activeGoalMaxTextRunes() int {
	maxRunes := conversation.DefaultMaxGoalTextRunes
	if e != nil && e.Settings != nil {
		if k := e.Settings.Knobs().ActiveGoalMaxTextRunes; k > 0 {
			maxRunes = k
		}
	}
	if maxRunes > conversation.DefaultMaxGoalTextRunes {
		maxRunes = conversation.DefaultMaxGoalTextRunes
	}
	return maxRunes
}

func truncateGoalSeedText(input string, maxRunes int) string {
	text := strings.TrimSpace(input)
	if text == "" || maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(text) <= maxRunes {
		return text
	}
	return string([]rune(text)[:maxRunes])
}
