package run

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

const (
	offloadPreviewRunes   = 500
	EventContextOffloaded = "context.offloaded"
	// PasteOffloadThreshold is the rune count at which paste/attachment text
	// is written to the workspace and replaced with a model-facing stub (AC-04).
	PasteOffloadThreshold = 64 << 10
)

// OffloadFileWriter persists large paste/tool bodies under offload/.
// *workspace.Service satisfies this; nil writers fail open.
type OffloadFileWriter interface {
	WriteOffloadFile(ctx context.Context, convID, content string) (string, error)
}

const (
	offloadHeaderOK    = "[已卸出到工作区文件，可用 read_file 分段读取]"
	offloadHeaderError = "[已卸出到工作区文件（错误结果），可用 read_file 分段读取]"
)

// FormatOffloadMessage builds the unified model-facing stub (spec §2.3).
// fullBody is redacted before taking the preview prefix. isError adds an error note.
func FormatOffloadMessage(path, fullBody string, isError bool) string {
	header := offloadHeaderOK
	if isError {
		header = offloadHeaderError
	}
	preview := truncateRunes(redactOffloadBody(fullBody), offloadPreviewRunes)
	return header + "\npath: " + path + "\npreview: " + preview
}

// IsOffloadPreview reports whether text is already an offload stub (skip DP-3).
func IsOffloadPreview(text string) bool {
	return strings.Contains(text, "[已卸出到工作区文件") &&
		strings.Contains(text, "\npath: offload/")
}

// MaybeOffloadPaste writes text to the conversation workspace when it reaches
// PasteOffloadThreshold runes and returns the model-facing stub. Fail-open:
// disabled / missing writer or conv / already a stub / write error → original.
// enabled defaults to true at call sites that have no Settings (Engine default).
func MaybeOffloadPaste(ctx context.Context, ws OffloadFileWriter, convID, text string, enabled bool) string {
	if !enabled || ws == nil || convID == "" || text == "" {
		return text
	}
	if IsOffloadPreview(text) {
		return text
	}
	if utf8.RuneCountInString(text) < PasteOffloadThreshold {
		return text
	}
	body := redactOffloadBody(text)
	path, err := ws.WriteOffloadFile(ctx, convID, body)
	if err != nil {
		return text
	}
	return FormatOffloadMessage(path, body, false)
}

func redactOffloadBody(fullBody string) string {
	var parsed any
	if err := json.Unmarshal([]byte(fullBody), &parsed); err == nil {
		redacted := identity.RedactSensitive(parsed)
		if b, err := json.Marshal(redacted); err == nil {
			return string(b)
		}
	}
	if s, ok := identity.RedactSensitive(fullBody).(string); ok {
		return s
	}
	return fullBody
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	n := 0
	for i := range s {
		if n == maxRunes {
			return s[:i]
		}
		n++
	}
	return s
}

// effectiveContextOffload reports whether tool/paste offload is live.
// Settings==nil defaults on (spec default); otherwise the knob decides.
func (e *Engine) effectiveContextOffload() bool {
	if e.Settings == nil {
		return true
	}
	return e.Settings.Knobs().ContextOffloadEnabled
}

// effectiveOffloadToolThreshold is the estimated-token size above which a tool
// result is written to the workspace. Mirrors the DP-3 prune threshold.
func (e *Engine) effectiveOffloadToolThreshold() int {
	return e.effectiveDecidePruneThreshold()
}

// applyToolOffload rewrites a model-facing tool message to an offload stub when
// the result is bulky and the workspace write succeeds. Fail-open: returns msg
// unchanged on any skip/failure. Does not mutate persisted tool.result events.
func (e *Engine) applyToolOffload(ctx context.Context, runID, convID string, msg llm.Message, content map[string]any, isError bool) llm.Message {
	if e == nil || !e.effectiveContextOffload() || e.Workspace == nil || convID == "" {
		return msg
	}
	sizeBefore := EstimateMessagesTokens([]llm.Message{msg})
	if sizeBefore <= e.effectiveOffloadToolThreshold() {
		return msg
	}
	cleaned, _ := tool.ExtractImageParts(content)
	redacted := identity.RedactSensitive(cleaned)
	body, err := json.Marshal(redacted)
	if err != nil {
		return msg
	}
	path, werr := e.Workspace.WriteOffloadFile(ctx, convID, string(body))
	if werr != nil {
		return msg
	}
	stub := FormatOffloadMessage(path, string(body), isError)
	msg = replaceToolMessageText(msg, stub)
	sizeAfter := EstimateMessagesTokens([]llm.Message{msg})
	saved := sizeBefore - sizeAfter
	if saved < 0 {
		saved = 0
	}
	if e.Store != nil {
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventContextOffloaded,
			Data: map[string]any{
				"source":       "tool",
				"path":         path,
				"tool_call_id": msg.ToolCallID,
				"saved_tokens": saved,
				"is_error":     isError,
			},
		})
	}
	return msg
}

// replaceToolMessageText swaps the textual body of a tool message while
// preserving Role, ToolCallID, and any image parts.
func replaceToolMessageText(msg llm.Message, text string) llm.Message {
	if len(msg.Parts) == 0 {
		msg.Content = text
		return msg
	}
	parts := make([]llm.ContentPart, len(msg.Parts))
	copy(parts, msg.Parts)
	replaced := false
	for i := range parts {
		if parts[i].Type == "text" {
			parts[i].Text = text
			replaced = true
			break
		}
	}
	if !replaced {
		parts = append([]llm.ContentPart{{Type: "text", Text: text}}, parts...)
	}
	msg.Parts = parts
	msg.Content = ""
	return msg
}
