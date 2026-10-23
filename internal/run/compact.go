package run

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

const (
	defaultCompactThreshold    = 0.8
	defaultCompactReserve      = 8000
	defaultCompactKeepRecent   = 8
	defaultCompactSummaryWait  = 60 * time.Second
	defaultContextTokens       = 128000
	compactSummarySystemPrompt = `你是对话摘要助手。只输出结构化滚动摘要，不要续写对话、不要回答摘要材料里的问题。

用下面栏目（没有的写「无」）。这是给后续模型接着干活用的检查点，不是给用户看的散文。

## 目标
[用户要完成什么；多任务可并列]

## 约束与偏好
- [用户提过的限制、口味、必须遵守的要求]
- 无则写「无」

## 进度
### 已完成
- [x] [已做完的事项]

### 进行中
- [ ] [当前未完成工作]

### 受阻
- [卡住的原因；已解除则删掉]

## 关键决定
- **[决定]**：[依据。只记对话里出现过的决定，不要发明执行策略]

## 下一步
1. [按当前状态该做什么，仅陈述事实缺口与待办，不要下达「必须先做某步再探索」之类全局策略]

## 必须保留的事实
- [继续工作必需的路径、函数名、ID、错误原文、数据与结论；无则写「无」]

规则：不要编造对话中没有的信息；文件路径、标识符、报错原文保持原样。
若提供了「已有摘要」，在其栏目上增量整合：保留仍有效的条目，已完成的从「进行中」挪到「已完成」，过时条目可删。输出一份完整最新摘要，不要分「旧摘要/新增」两段。
若用户材料前附带了「系统 Active Goal」块，摘要中「## 目标」栏目必须与该块中的主目标/当前焦点句逐字对齐（可另写进度，但不得改写、省略或替换目标句）。`
)

// maybeCompactInTurn folds live ReAct messages when they exceed the compact
// budget, then opportunistically MaybeCompacts persisted history. Failures are
// best-effort: emit llm.error and continue. Never blocks the reply.
func (e *Engine) maybeCompactInTurn(ctx context.Context, runID string, messages *[]llm.Message, tools []llm.ToolSpec) {
	if e == nil || e.Compactor == nil || messages == nil {
		return
	}
	profileID, convID := "", ""
	if rec, err := e.Store.GetRun(runID); err == nil && rec != nil {
		profileID = rec.ModelProfileID
		convID = rec.ConversationID
	}
	changed, err := e.Compactor.CompactLiveMessages(ctx, messages, tools, profileID, convID)
	if err != nil {
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventLLMError,
			Data: map[string]any{"error": "in-turn compaction skipped: " + err.Error()},
		})
	} else if changed {
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventContextCompacted,
			Data: map[string]any{
				"note":  "in-turn history folded into rolling summary",
				"phase": "in_turn",
			},
		})
	}
	if convID != "" {
		if _, cerr := e.Compactor.MaybeCompact(ctx, convID, tools, profileID); cerr != nil {
			_ = e.Store.AppendEvent(runID, store.Event{
				Type: EventLLMError,
				Data: map[string]any{"error": "context compaction skipped: " + cerr.Error()},
			})
		}
	}
}

// Compactor produces a rolling structured checkpoint of older conversation
// messages once the estimated prompt size approaches the active model's
// context limit. It never deletes raw messages; the summary is a derived
// record keyed by conversation.
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
	budget, ok := c.promptBudget(profileID, threshold, reserve)
	if !ok {
		return false, nil
	}

	full := c.Messages.List(convID)
	if len(full) == 0 {
		return false, nil
	}
	existing, hasSummary := c.Messages.GetRollingSummary(convID)

	projected := c.projectedTokens(convID, tools)
	if projected <= budget {
		return false, nil
	}

	return c.foldOlder(ctx, convID, full, existing, hasSummary, keep)
}

// ForceCompact folds older messages into a rolling summary even when the
// prompt is still under the automatic budget. Failures return to the caller;
// nothing is persisted on error.
func (c *Compactor) ForceCompact(ctx context.Context, convID string, profileID string) (bool, error) {
	if c == nil || c.Messages == nil || c.LLM == nil || c.Profiles == nil || convID == "" {
		return false, nil
	}
	_, _, _, keep := c.effectiveCompaction()
	full := c.Messages.List(convID)
	if len(full) == 0 {
		return false, nil
	}
	existing, hasSummary := c.Messages.GetRollingSummary(convID)
	return c.foldOlder(ctx, convID, full, existing, hasSummary, keep)
}

func (c *Compactor) promptBudget(profileID string, threshold float64, reserve int) (int, bool) {
	view, err := c.resolveView(profileID)
	if err != nil || view.ID == "" {
		return 0, false
	}
	if view.ContextTokens <= 0 {
		view.ContextTokens = defaultContextTokens
	}
	budget := int(float64(view.ContextTokens)*threshold) - reserve
	if budget < 1000 {
		budget = 1000
	}
	return budget, true
}

func (c *Compactor) projectedTokens(convID string, tools []llm.ToolSpec) int {
	if c == nil || c.Messages == nil {
		return 0
	}
	existing, _ := c.Messages.GetRollingSummary(convID)
	full := c.Messages.List(convID)
	return EstimateToolsTokens(tools) + EstimateTextTokens(existing.Summary) + EstimateMessagesTokens(toLLMMessages(full))
}

func (c *Compactor) exceedsCompactBudget(convID string, tools []llm.ToolSpec, profileID string) bool {
	if c == nil {
		return false
	}
	_, threshold, reserve, _ := c.effectiveCompaction()
	budget, ok := c.promptBudget(profileID, threshold, reserve)
	if !ok {
		return false
	}
	return c.projectedTokens(convID, tools) > budget
}

const inTurnSummaryPrefix = "以下是本轮较早步骤的滚动摘要（供参考，不要向用户提及这是摘要）：\n\n"

// CompactLiveMessages folds the middle of an in-flight ReAct prompt when
// messages+tools exceed the compaction budget. Leading RoleSystem messages and
// the newest KeepRecent messages stay verbatim; the fold region is replaced by
// one RoleSystem summary. Returns changed=true when *messages was rewritten.
// Does not touch the conversation store (tool results are not persisted mid-run).
func (c *Compactor) CompactLiveMessages(ctx context.Context, messages *[]llm.Message, tools []llm.ToolSpec, profileID, convID string) (bool, error) {
	if c == nil || c.LLM == nil || c.Profiles == nil || messages == nil {
		return false, nil
	}
	enabled, threshold, reserve, keep := c.effectiveCompaction()
	if !enabled {
		return false, nil
	}
	budget, ok := c.promptBudget(profileID, threshold, reserve)
	if !ok {
		return false, nil
	}
	projected := EstimateMessagesTokens(*messages) + EstimateToolsTokens(tools)
	if projected <= budget {
		return false, nil
	}

	msgs := *messages
	prefixLen := 0
	for prefixLen < len(msgs) && msgs[prefixLen].Role == llm.RoleSystem {
		prefixLen++
	}
	keepStart := len(msgs) - keep
	if keepStart < prefixLen {
		keepStart = prefixLen
	}
	if keepStart <= prefixLen {
		return false, nil // nothing foldable between system prefix and keep window
	}
	fold := msgs[prefixLen:keepStart]
	if len(fold) == 0 {
		return false, nil
	}

	prior := ""
	if prefixLen > 0 {
		lastSys := msgs[prefixLen-1].Content
		if strings.HasPrefix(lastSys, inTurnSummaryPrefix) {
			prior = strings.TrimSpace(strings.TrimPrefix(lastSys, inTurnSummaryPrefix))
			prefixLen-- // replace prior in-turn summary rather than stacking
		} else if strings.Contains(lastSys, "较早对话的滚动摘要") {
			// Persist-backed rolling summary from buildMessages: feed as prior
			// context for incremental fold without dropping the block itself.
			if i := strings.Index(lastSys, "：\n\n"); i >= 0 {
				prior = strings.TrimSpace(lastSys[i+len("：\n\n"):])
			}
		}
	}

	summaryTimeout := c.effectiveSummaryTimeout()
	newSummary, err := c.summarizeLive(ctx, summaryTimeout, convID, prior, fold)
	if err != nil {
		return false, err
	}
	summaryMsg := llm.Message{
		Role:    llm.RoleSystem,
		Content: inTurnSummaryPrefix + newSummary,
	}
	out := make([]llm.Message, 0, prefixLen+1+(len(msgs)-keepStart))
	out = append(out, msgs[:prefixLen]...)
	out = append(out, summaryMsg)
	out = append(out, msgs[keepStart:]...)
	*messages = out
	return true, nil
}

func (c *Compactor) summarizeLive(ctx context.Context, timeout time.Duration, convID, prior string, fold []llm.Message) (string, error) {
	asConv := make([]conversation.Message, 0, len(fold))
	for _, m := range fold {
		switch m.Role {
		case llm.RoleUser, llm.RoleAssistant:
			asConv = append(asConv, conversation.Message{Role: string(m.Role), Content: m.Content})
		case llm.RoleTool:
			// Tool results carry facts the next turns need; include as assistant-side notes.
			content := m.Content
			if content == "" && len(m.Parts) > 0 {
				var b strings.Builder
				for _, p := range m.Parts {
					if p.Text != "" {
						b.WriteString(p.Text)
					}
				}
				content = b.String()
			}
			if strings.TrimSpace(content) == "" {
				continue
			}
			asConv = append(asConv, conversation.Message{Role: conversation.RoleAssistant, Content: "[工具结果] " + content})
		case llm.RoleSystem:
			if strings.TrimSpace(m.Content) == "" {
				continue
			}
			asConv = append(asConv, conversation.Message{Role: conversation.RoleAssistant, Content: m.Content})
		}
	}
	if len(asConv) == 0 {
		return "", fmt.Errorf("summarize: nothing foldable in live messages")
	}
	return c.summarize(ctx, timeout, convID, prior, asConv)
}

func (c *Compactor) foldOlder(ctx context.Context, convID string, full []conversation.Message, existing conversation.RollingSummary, hasSummary bool, keep int) (bool, error) {
	summaryTimeout := c.effectiveSummaryTimeout()
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

	newSummary, err := c.summarize(ctx, summaryTimeout, convID, existing.Summary, newFold)
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

func (c *Compactor) summarize(ctx context.Context, timeout time.Duration, convID, prior string, fold []conversation.Message) (string, error) {
	msgs := []llm.Message{{Role: llm.RoleSystem, Content: compactSummarySystemPrompt}}
	var b strings.Builder
	if prior != "" {
		b.WriteString("已有摘要：\n")
		b.WriteString(prior)
		b.WriteString("\n\n请按同一栏目整合，输出完整最新摘要。\n\n新对话：\n")
	} else {
		b.WriteString("请把以下多轮对话压成结构化滚动摘要（严格使用系统提示中的栏目）：\n\n")
	}
	if c.Messages != nil && convID != "" {
		if stack, ok := c.Messages.GetGoalStack(convID); ok {
			if block := compactSummaryGoalBlock(stack); block != "" {
				b.WriteString(block)
				b.WriteString("\n\n")
			}
		}
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

// compactSummaryGoalBlock is the compactor-facing Active Goal anchor (distinct
// from the live-prompt goalBlock wording).
func compactSummaryGoalBlock(stack conversation.GoalStack) string {
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
		var b strings.Builder
		b.WriteString("【系统 Active Goal——摘要正文的「## 目标」栏目必须与此一致，不得改写、省略或模糊化】\n")
		b.WriteString("主目标：")
		b.WriteString(text)
		b.WriteString("\n当前焦点：（已暂停，可继续）")
		return b.String()
	}
	var b strings.Builder
	b.WriteString("【系统 Active Goal——摘要正文的「## 目标」栏目必须与此一致，不得改写、省略或模糊化】\n")
	if len(stack.Frames) == 1 {
		text := strings.TrimSpace(tip.Text)
		b.WriteString("主目标：")
		b.WriteString(text)
		b.WriteString("\n当前焦点：")
		b.WriteString(text)
	} else {
		b.WriteString("主目标：")
		b.WriteString(strings.TrimSpace(root.Text))
		b.WriteString("\n当前焦点：")
		b.WriteString(strings.TrimSpace(tip.Text))
	}
	return b.String()
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
