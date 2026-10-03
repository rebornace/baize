package run

import (
	"strings"

	"github.com/rebornace/baize/internal/memory"
)

func (e *Engine) effectiveMemoryEnabled() bool {
	if e.Settings == nil {
		return true
	}
	return e.Settings.Knobs().MemoryEnabled
}

func (e *Engine) effectiveMemoryAutoExtract() bool {
	if !e.effectiveMemoryEnabled() {
		return false
	}
	if e.Settings == nil {
		return true
	}
	return e.Settings.Knobs().MemoryAutoExtract
}

func (e *Engine) memoryOwner(conversationID string) string {
	if conversationID == "" || e.Meta == nil {
		return ""
	}
	m, err := e.Meta.GetMeta(conversationID)
	if err != nil || m.OwnerID == "" {
		return "local-dev"
	}
	return m.OwnerID
}

// memoryBlock builds the account-memory system message for the current turn.
// Empty string means skip injection.
func (e *Engine) memoryBlock(ownerID, query string) string {
	if !e.effectiveMemoryEnabled() || e.Memory == nil {
		return ""
	}
	ownerID = strings.TrimSpace(ownerID)
	query = strings.TrimSpace(query)
	if ownerID == "" || query == "" {
		return ""
	}
	entries, err := e.Memory.Search(ownerID, query, memory.DefaultTopK)
	if err != nil || len(entries) == 0 {
		return ""
	}

	const header = "以下是该账号的相关长期记忆（供回答参考；不要主动向用户逐条复述清单，除非用户问起）："
	n := len([]rune(header))
	var body strings.Builder
	for _, ent := range entries {
		text := strings.TrimSpace(ent.Text)
		if text == "" {
			continue
		}
		line := "\n- " + text
		lineN := len([]rune(line))
		if n+lineN > memory.DefaultMaxInjectChars {
			break
		}
		body.WriteString(line)
		n += lineN
	}
	if body.Len() == 0 {
		return ""
	}
	return header + body.String()
}
