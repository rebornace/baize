package run

import (
	"regexp"
	"strings"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

const (
	// EventUnverifiedWrite records a write-completion claim made without any
	// tool call in the run.
	EventUnverifiedWrite = "run.unverified_write"
	// UnverifiedWriteNote is the user-facing system note for that case.
	UnverifiedWriteNote = "本轮没有调用任何工具，但回复声称写操作已完成。请核实结果。"
)

var claimSentenceSplit = regexp.MustCompile(`[。！？!?;；\n]+|\.\s+`)

var writeClaimNegation = regexp.MustCompile(
	`未|没有|没能|无法|不能|失败|未成功|并未|是否|(?i:\bnot\b|\bfailed\b|\bcannot\b|\bcould not\b|\bwasn't\b|\bweren't\b|\bdidn't\b)`,
)

const writeVerbAlternation = `创建|新建|复制|更新|修改|删除|分配|提交|发布|保存|写入|导入|上传|发送|设置|记住|清空|关闭|开启|启用|停用|重命名|移动`

var writeClaimPatterns = []*regexp.Regexp{
	// 已 / 已经把 / 已将 … 写动词
	regexp.MustCompile(`已(?:经)?(?:把|将)?[^。！？!?;；\n]{0,24}(?:` + writeVerbAlternation + `)`),
	// 复制完成 / 创建成功
	regexp.MustCompile(`(?:` + writeVerbAlternation + `)完成`),
	// 已经 / 已为你 / 已帮你 … 写动词
	regexp.MustCompile(`(?:已经|已为你|已帮你)[^。！？!?;；\n]{0,24}(?:` + writeVerbAlternation + `)`),
	// I have created / I've updated
	regexp.MustCompile(`(?i)\b(?:I|we)(?:'ve| have)?\s+(?:created|updated|deleted|copied|assigned|submitted|published|saved|uploaded|sent|renamed|moved|enabled|disabled|imported|written)\b`),
	// Created successfully
	regexp.MustCompile(`(?i)\b(?:created|updated|deleted|copied|assigned|submitted|published|saved|uploaded|sent|renamed|moved|enabled|disabled|imported|written)\s+successfully\b`),
}

// unverifiedWriteClaims returns short claim fragments from the final assistant
// output. It is deliberately conservative: sentence-level negation/questions
// are skipped, and only explicit write verbs are considered.
func unverifiedWriteClaims(output string) []string {
	if strings.TrimSpace(output) == "" {
		return nil
	}
	var claims []string
	seen := map[string]bool{}
	for _, raw := range claimSentenceSplit.Split(output, -1) {
		sentence := strings.TrimSpace(raw)
		if sentence == "" || writeClaimNegation.MatchString(sentence) {
			continue
		}
		for _, re := range writeClaimPatterns {
			loc := re.FindStringIndex(sentence)
			if loc == nil {
				continue
			}
			claim := truncateClaimRunes(strings.TrimSpace(sentence[loc[0]:loc[1]]), 120)
			if claim == "" || seen[claim] {
				break
			}
			seen[claim] = true
			claims = append(claims, claim)
			break
		}
		if len(claims) >= 5 {
			break
		}
	}
	return claims
}

func truncateClaimRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

// noteUnverifiedWrite appends the audit event and system note when a succeeded
// run claims a completed write without any tool call. It is idempotent.
func (e *Engine) noteUnverifiedWrite(runID string, runRec *store.Run) {
	if e == nil || e.Store == nil || e.Messages == nil || runRec == nil || runRec.ConversationID == "" {
		return
	}
	evs, err := e.Store.ListEvents(runID)
	if err != nil {
		return
	}
	toolCalls := 0
	for _, ev := range evs {
		switch ev.Type {
		case EventLLMToolCall:
			toolCalls++
		case EventUnverifiedWrite:
			return
		}
	}
	if toolCalls > 0 {
		return
	}
	claims := unverifiedWriteClaims(runRec.Output)
	if len(claims) == 0 {
		return
	}
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventUnverifiedWrite,
		Data: map[string]any{"reason": "no_tool_call", "claims": claims},
	})
	_, _ = e.Messages.Append(runRec.ConversationID, conversation.Message{
		Role:    conversation.RoleSystemNote,
		Content: UnverifiedWriteNote,
		RunID:   runID,
	})
}
