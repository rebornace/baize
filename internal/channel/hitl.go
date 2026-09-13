package channel

import (
	"fmt"
	"strings"

	"github.com/rebornace/baize/internal/store"
)

// HITLHelpReply is sent when a waiting_human run exists but the inbound text
// is not a recognizable approve/reject decision.
const HITLHelpReply = "当前有待审批操作。请回复「批准」或「拒绝」，也可在 /ui 处理。"

// FormatHITLNotify builds the WeChat (or other channel) text for a pending approval.
func FormatHITLNotify(p *store.HITLPayload) string {
	if p == nil {
		return "需要人工审批才能继续。请回复「批准」或「拒绝」。"
	}
	tool := strings.TrimSpace(p.ToolName)
	prompt := strings.TrimSpace(p.Prompt)
	var b strings.Builder
	b.WriteString("需要人工审批才能继续：\n")
	if tool != "" {
		fmt.Fprintf(&b, "工具：%s\n", tool)
	}
	if prompt != "" {
		fmt.Fprintf(&b, "说明：%s\n", prompt)
	}
	b.WriteString("\n请回复「批准」或「拒绝」。也可在 /ui 操作。")
	return b.String()
}

// ParseHITLDecision reports whether text is an approve/reject command.
// ok=false means the text is not a HITL decision.
func ParseHITLDecision(text string) (approve bool, ok bool) {
	s := strings.TrimSpace(strings.ToLower(text))
	// Strip common punctuation / fullwidth variants.
	s = strings.Trim(s, "。.!！?？ \t")
	switch s {
	case "批准", "同意", "确认", "approve", "yes", "y", "ok":
		return true, true
	case "拒绝", "驳回", "不同意", "reject", "no", "n":
		return false, true
	default:
		return false, false
	}
}
