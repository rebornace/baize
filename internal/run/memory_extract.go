package run

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/memory"
	"github.com/rebornace/baize/internal/store"
)

const (
	EventMemoryExtractSkipped = "memory.extract_skipped"

	memoryExtractSystem = `你是记忆抽取器。根据用户输入与助手回复，提取值得跨会话长期保留的短事实。
只输出 JSON 数组，不要其它文字。格式：[{"key":"","text":"..."}]
最多 5 条；每条 text 不超过 500 字；key 可空。无值得记录的事实时输出 []。`
)

type memoryExtractItem struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// maybeExtractMemory runs a short follow-up Chat after a succeeded run and
// upserts SourceAuto entries. Failures emit memory.extract_skipped and never
// change the run status. Info logs must not include full memory text.
func (e *Engine) maybeExtractMemory(ctx context.Context, runID, owner, input, output string) {
	if !e.effectiveMemoryAutoExtract() || e.Memory == nil || e.LLM == nil {
		return
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return
	}
	input = strings.TrimSpace(input)
	output = strings.TrimSpace(output)
	if input == "" && output == "" {
		return
	}

	// DP-1: ask the decision layer whether this turn is worth a memory
	// extraction before paying for the generative call. OnFail is Yes: if the
	// layer is down/degraded, extract as today (fail open, never lose a memory).
	if e.effectiveDecideMemory() {
		ans, derr := e.Decider.Ask(ctx, decide.Question{
			Kind:    decide.KindMemoryExtract,
			Context: buildExtractProbe(input, output),
			OnFail:  decide.VerdictYes,
			TraceID: runID,
		})
		// Unified observability: when the layer fails open, record it once.
		// The extraction below still proceeds (OnFail=Yes).
		if degradedFromAsk(ans, derr) {
			e.emitDecideDegraded(runID, decide.KindMemoryExtract, map[string]any{"source": ans.Source})
		}
		// A returned error is treated as fail-open (extract as today). The
		// production *decide.Chain never errors, so this only happens with a
		// raw implementation; safety wins over a saved call.
		if derr == nil && ans.Verdict == decide.VerdictNo && !ans.Degraded {
			_ = e.Store.AppendEvent(runID, store.Event{
				Type: EventMemoryExtractSkipped,
				Data: map[string]any{
					"reason":       "decider_no",
					"source":       ans.Source,
					"saved_tokens": estimateExtractionInputTokens(input, output),
				},
			})
			return
		}
	}

	user := "用户输入：\n" + input + "\n\n助手回复：\n" + output
	msg, err := e.LLM.Chat(ctx, []llm.Message{
		{Role: llm.RoleSystem, Content: memoryExtractSystem},
		{Role: llm.RoleUser, Content: user},
	}, nil)
	if err != nil {
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventMemoryExtractSkipped,
			Data: map[string]any{"error": truncateErr(err.Error())},
		})
		return
	}

	items, perr := parseMemoryExtractJSON(msg.Content)
	if perr != nil {
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventMemoryExtractSkipped,
			Data: map[string]any{"error": truncateErr(perr.Error())},
		})
		return
	}
	for _, it := range items {
		text := strings.TrimSpace(it.Text)
		if text == "" {
			continue
		}
		_, _ = e.Memory.Upsert(memory.Entry{
			OwnerID: owner,
			Key:     strings.TrimSpace(it.Key),
			Text:    text,
			Source:  memory.SourceAuto,
		})
	}
}

func parseMemoryExtractJSON(content string) ([]memoryExtractItem, error) {
	s := strings.TrimSpace(content)
	if s == "" {
		return nil, errExtractEmpty
	}
	// Strip optional markdown fences.
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSpace(s)
		if strings.HasPrefix(strings.ToLower(s), "json") {
			s = strings.TrimSpace(s[4:])
		}
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = strings.TrimSpace(s[:i])
		}
	}
	// Prefer outermost array slice if surrounding prose slipped in.
	if i := strings.Index(s, "["); i >= 0 {
		if j := strings.LastIndex(s, "]"); j > i {
			s = s[i : j+1]
		}
	}
	var items []memoryExtractItem
	if err := json.Unmarshal([]byte(s), &items); err != nil {
		return nil, err
	}
	if len(items) > 5 {
		items = items[:5]
	}
	return items, nil
}

var errExtractEmpty = errors.New("empty extract response")

func truncateErr(s string) string {
	const max = 200
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
