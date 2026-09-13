package run

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
)

const (
	defaultCompactThreshold    = 0.8
	defaultCompactReserve      = 8000
	defaultCompactKeepRecent   = 8
	defaultCompactSummaryWait  = 60 * time.Second
	defaultContextTokens       = 128000
	compactSummarySystemPrompt = `你是对话摘要助手。请把给定的多轮对话压缩成简洁的中文摘要，保留：用户的目标与偏好、已做出的关键决定、待办与未决事项、关键事实（文件名、ID、数据、结论）。不要编造对话中没有的信息。
若提供了「已有摘要」，请在其基础上增量整合新对话，输出一份完整、自洽的最新摘要（不要罗列「新增/旧摘要」的边界）。`
)

// Compactor produces a rolling summary of older conversation messages once the
// estimated prompt size approaches the active model's context limit. It never
// deletes raw messages; the summary is a derived record keyed by conversation.
type Compactor struct {
	Messages conversation.Store
	// LLM generates summaries. Pass the llm.Switch: summaries are invoked on a
	// bare context (no per-run profile id) so the Switch resolves the DEFAULT
	// profile, independent of the model chosen for this run.
	LLM      llm.Provider
	Profiles llm.ProfileSource

	Threshold      float64       // fraction of context that may be used before folding
	ReserveTokens  int           // headroom reserved for answer + tools + current turn
	KeepRecent     int           // number of newest messages always kept verbatim
	SummaryTimeout time.Duration // cap on the summarization call
	// Settings optionally supplies hot-reloadable knobs (compaction switch +
	// thresholds). nil = use the struct fields (YAML defaults). Read inside
	// MaybeCompact rather than mutating shared fields, to avoid races across
	// concurrent runs.
	Settings KnobReader
}

// effectiveCompaction resolves the hot-reloadable compaction switch and
// thresholds, layering snapshot overrides over struct fields over code
// defaults. enabled=false means compaction is turned off for this run.
func (c *Compactor) effectiveCompaction() (enabled bool, threshold float64, reserve, keep int) {
	threshold, reserve, keep = c.Threshold, c.ReserveTokens, c.KeepRecent
	enabled = true
	if c.Settings != nil {
		k := c.Settings.Knobs()
		if !k.CompactionEnabled {
			enabled = false
		}
		if k.CompactThreshold > 0 {
			threshold = k.CompactThreshold
		}
		if k.CompactReserveTokens > 0 {
			reserve = k.CompactReserveTokens
		}
		if k.CompactKeepRecent > 0 {
			keep = k.CompactKeepRecent
		}
	}
	if threshold <= 0 {
		threshold = defaultCompactThreshold
	}
	if reserve <= 0 {
		reserve = defaultCompactReserve
	}
	if keep <= 0 {
		keep = defaultCompactKeepRecent
	}
	return enabled, threshold, reserve, keep
}

// effectiveSummaryTimeout resolves the cap on the summarization LLM call:
// hot-reloadable knob > struct field > code default.
func (c *Compactor) effectiveSummaryTimeout() time.Duration {
	if c.Settings != nil {
		if d := c.Settings.Knobs().CompactSummaryTimeout; d > 0 {
			return d
		}
	}
	if c.SummaryTimeout > 0 {
		return c.SummaryTimeout
	}
	return defaultCompactSummaryWait
}

// MaybeCompact folds older messages into a rolling summary when the projected
// prompt (tools + existing summary + full history) exceeds the budget derived
// from the run's model context limit. It returns changed=true when a new
// summary was persisted. Failures are returned to the caller; the engine logs
// and continues with the hard window (compaction never blocks a reply).
func (c *Compactor) MaybeCompact(ctx context.Context, convID string, tools []llm.ToolSpec, profileID string) (bool, error) {
	if c == nil || c.Messages == nil || c.LLM == nil || c.Profiles == nil || convID == "" {
		return false, nil
	}
	enabled, threshold, reserve, keep := c.effectiveCompaction()
	if !enabled {
		return false, nil // compaction switched off at runtime
	}
	summaryTimeout := c.effectiveSummaryTimeout()

	view, err := c.resolveView(profileID)
	if err != nil || view.ID == "" {
		return false, nil // no usable profile -> compaction disabled (mock/demo path)
	}
	if view.ContextTokens <= 0 {
		view.ContextTokens = defaultContextTokens // normalize unconfigured profile
	}
	budget := int(float64(view.ContextTokens)*threshold) - reserve
	if budget < 1000 {
		budget = 1000
	}

	full := c.Messages.List(convID)
	if len(full) == 0 {
		return false, nil
	}
	existing, hasSummary := c.Messages.GetRollingSummary(convID)

	projected := EstimateToolsTokens(tools) + EstimateTextTokens(existing.Summary) + EstimateMessagesTokens(toLLMMessages(full))
	if projected <= budget {
		return false, nil
	}

	keepStart := len(full) - keep
	if keepStart <= 0 {
		return false, nil // everything is "recent"; nothing foldable
	}
	covered := 0
	if hasSummary {
		covered = existing.CoversThroughOrder + 1
	}
	if keepStart <= covered {
		return false, nil // no new messages beyond the cursor to fold
	}
	newFold := full[covered:keepStart]

	newSummary, err := c.summarize(ctx, summaryTimeout, existing.Summary, newFold)
	if err != nil {
		return false, err
	}

	rec := conversation.RollingSummary{
		ConversationID:         convID,
		Summary:                newSummary,
		CoversThroughMessageID: full[keepStart-1].ID,
		CoversThroughOrder:     keepStart - 1,
	}
	if err := c.Messages.UpsertRollingSummary(rec); err != nil {
		return false, err
	}
	return true, nil
}

func (c *Compactor) resolveView(profileID string) (llm.ModelProfileView, error) {
	if profileID != "" {
		if v, err := c.Profiles.ModelProfileByID(profileID); err == nil && v.ID != "" {
			return v, nil
		}
	}
	list, err := c.Profiles.ListProfiles()
	if err != nil {
		return llm.ModelProfileView{}, err
	}
	return llm.PrimaryModelProfile(list)
}

func (c *Compactor) summarize(ctx context.Context, timeout time.Duration, prior string, fold []conversation.Message) (string, error) {
	msgs := []llm.Message{{Role: llm.RoleSystem, Content: compactSummarySystemPrompt}}
	var b strings.Builder
	if prior != "" {
		b.WriteString("已有摘要：\n")
		b.WriteString(prior)
		b.WriteString("\n\n请在已有摘要基础上，整合以下新对话，输出完整最新摘要。\n\n新对话：\n")
	} else {
		b.WriteString("请把以下多轮对话压缩成滚动摘要：\n\n")
	}
	b.WriteString(renderTranscript(fold))
	msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: b.String()})

	// Bare context: no per-run profile id => Switch uses the DEFAULT model.
	sumCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := c.LLM.Chat(sumCtx, msgs, nil)
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}
	return strings.TrimSpace(out.Content), nil
}

func renderTranscript(msgs []conversation.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case conversation.RoleUser:
			b.WriteString("[用户] ")
		case conversation.RoleAssistant:
			b.WriteString("[助手] ")
		default:
			continue // skip tool / system_note noise
		}
		b.WriteString(m.Content)
		b.WriteString("\n\n")
	}
	return b.String()
}

// toLLMMessages converts persisted messages for token estimation (content only).
func toLLMMessages(msgs []conversation.Message) []llm.Message {
	out := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, llm.Message{Role: llm.Role(m.Role), Content: m.Content})
	}
	return out
}
