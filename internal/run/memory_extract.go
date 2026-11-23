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

	// memoryExtractMaxPerRun caps how many new auto entries one run may add.
	memoryExtractMaxPerRun = 3

	memoryExtractSystem = `你是记忆抽取器。根据用户输入与助手回复，提取值得跨会话长期保留的短事实。
只输出 JSON 数组，不要其它文字。格式：[{"key":"","text":"..."}]
最多 5 条；每条 text 不超过 500 字；key 可空。无值得记录的事实时输出 []。`
)

type memoryExtractItem struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// maybeExtractMemory runs a short follow-up Chat after a succeeded run and
// upserts SourceAuto entries; non-empty keys that already exist are skipped
// with memory.extract_skipped and Upsert is not called. Failures emit
// memory.extract_skipped and never change the run status. Info logs must not
// include full memory text.
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

	// Explicit memory writes in this run already carry the user's intent; a
	// follow-up auto extraction tends to duplicate them under new keys.
	// Skip before the decider/LLM so the extraction call is not paid for.
	if e.runHasExplicitMemoryWrite(runID) {
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventMemoryExtractSkipped,
			Data: map[string]any{"reason": "explicit_in_run"},
		})
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
	existing := e.existingMemoryIndex(owner)
	written := 0
	for i, it := range items {
		text := strings.TrimSpace(it.Text)
		if text == "" {
			continue
		}
		norm := normalizeMemoryText(text)
		if norm != "" && existing.exact[norm] {
			_ = e.Store.AppendEvent(runID, store.Event{
				Type: EventMemoryExtractSkipped,
				Data: map[string]any{"reason": "duplicate_text"},
			})
			continue
		}
		candidate := newNearDupText(text)
		if nearDuplicateAny(candidate, existing.near) {
			_ = e.Store.AppendEvent(runID, store.Event{
				Type: EventMemoryExtractSkipped,
				Data: map[string]any{"reason": "near_duplicate"},
			})
			continue
		}
		key := strings.TrimSpace(it.Key)
		if key != "" {
			_, exists, lerr := e.Memory.GetByOwnerKey(owner, key)
			if lerr != nil {
				_ = e.Store.AppendEvent(runID, store.Event{
					Type: EventMemoryExtractSkipped,
					Data: map[string]any{"reason": "lookup_error", "key": key},
				})
				continue
			}
			if exists {
				_ = e.Store.AppendEvent(runID, store.Event{
					Type: EventMemoryExtractSkipped,
					Data: map[string]any{"reason": "key_exists", "key": key},
				})
				continue
			}
		}
		if written >= memoryExtractMaxPerRun {
			_ = e.Store.AppendEvent(runID, store.Event{
				Type: EventMemoryExtractSkipped,
				Data: map[string]any{"reason": "cap_reached", "count": len(items) - i},
			})
			break
		}
		_, _ = e.Memory.Upsert(memory.Entry{
			OwnerID: owner,
			Key:     key,
			Text:    text,
			Source:  memory.SourceAuto,
		})
		if norm != "" {
			existing.exact[norm] = true
		}
		existing.near = append(existing.near, candidate)
		written++
	}
}

// runHasExplicitMemoryWrite reports whether this run already wrote or removed
// memory through the explicit tools. A read failure fails open (extract).
func (e *Engine) runHasExplicitMemoryWrite(runID string) bool {
	if e.Store == nil {
		return false
	}
	events, err := e.Store.ListEvents(runID)
	if err != nil {
		return false
	}
	for _, ev := range events {
		if ev.Type != "tool.result" {
			continue
		}
		name, _ := ev.Data["name"].(string)
		if name != memory.RememberName && name != memory.ForgetName {
			continue
		}
		if isErr, _ := ev.Data["is_error"].(bool); !isErr {
			return true
		}
	}
	return false
}

// normalizeMemoryText collapses case and whitespace for duplicate detection.
func normalizeMemoryText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

// memoryMatchIndex holds the existing entries used by the extractor's
// duplicate checks: exact normalized text plus precomputed near-dup features.
type memoryMatchIndex struct {
	exact map[string]bool
	near  []nearDupText
}

// existingMemoryIndex loads the owner's entries (bounded pagination) and
// precomputes both exact and near-duplicate features. A read failure yields
// whatever was collected so far.
func (e *Engine) existingMemoryIndex(owner string) memoryMatchIndex {
	idx := memoryMatchIndex{exact: map[string]bool{}}
	const page = 200
	for offset := 0; offset < 5000; offset += page {
		items, err := e.Memory.List(owner, page, offset)
		if err != nil {
			break
		}
		for _, it := range items {
			if n := normalizeMemoryText(it.Text); n != "" {
				idx.exact[n] = true
			}
			idx.near = append(idx.near, newNearDupText(it.Text))
		}
		if len(items) < page {
			break
		}
	}
	return idx
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
