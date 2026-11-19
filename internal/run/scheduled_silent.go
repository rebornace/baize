package run

import (
	"strings"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

const (
	// EventRunSilent records a scheduled run that chose to stay quiet.
	EventRunSilent = "run.silent"
	// SilentMarker is the exact reply that suppresses a scheduled run.
	SilentMarker = "NO_UPDATE"
	// SilentNote is the WebUI system note for a silent scheduled run.
	SilentNote = "定时检查：无新增。"

	scheduledHintZH = "[系统] 本轮由定时日程触发。先核对日程指令与已有结果；如果没有新的、重要的变化，只回复 NO_UPDATE（不要加标点或其它文字），系统会静默收口。如果有新变化，给出简短汇报。"
	scheduledHintEN = "[system] This turn was triggered by a schedule. Check the instruction and existing results; if there is nothing new or important, reply with only NO_UPDATE (no punctuation or other text) and the system will finish silently. If something changed, give a short briefing."
)

func isSilentMarker(content string) bool {
	s := strings.TrimSpace(content)
	s = strings.Trim(s, "。.!！?？")
	return strings.EqualFold(strings.TrimSpace(s), SilentMarker)
}

func (e *Engine) appendScheduleHint(system string, runRec *store.Run, locale string) string {
	if runRec == nil || runRec.Origin != store.RunOriginScheduled {
		return system
	}
	hint := scheduledHintZH
	if !isChineseLocale(locale) {
		hint = scheduledHintEN
	}
	if strings.TrimSpace(system) == "" {
		return hint
	}
	return system + "\n\n" + hint
}

// finishSilentRun settles a scheduled run that replied NO_UPDATE: no assistant
// message, no outbound delivery, no memory extraction.
func (e *Engine) finishSilentRun(runID string) error {
	if err := e.Store.AppendEvent(runID, store.Event{
		Type: EventRunSilent,
		Data: map[string]any{"origin": store.RunOriginScheduled},
	}); err != nil {
		return err
	}
	if err := e.Store.UpdateRun(runID, store.StatusSucceeded, "", ""); err != nil {
		return err
	}
	// Handles child-run receipts and is a no-op for the empty assistant output.
	e.recordTerminalMessage(runID)
	runRec, err := e.Store.GetRun(runID)
	if err != nil || runRec == nil || e.Messages == nil || runRec.ConversationID == "" {
		return nil
	}
	if !e.userTurnStillLinked(runRec) {
		return nil
	}
	_, _ = e.Messages.Append(runRec.ConversationID, conversation.Message{
		Role:    conversation.RoleSystemNote,
		Content: SilentNote,
		RunID:   runID,
	})
	return nil
}
