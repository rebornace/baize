package procfs

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rebornace/baize/internal/store"
)

const MaxFollowOnRuns = 5

var ErrNoEvents = errors.New("no events in timeline")

type TimelineEntry struct {
	RunID     string
	Type      string
	Timestamp time.Time
	Summary   string
	ToolName  string
}

type Timeline struct {
	SourceRunID    string
	ConversationID string
	Entries        []TimelineEntry
}

func BuildTimeline(st store.Store, sourceRunID string) (Timeline, error) {
	source, err := st.GetRun(sourceRunID)
	if err != nil {
		return Timeline{}, err
	}

	runIDs := []string{source.ID}
	if source.ConversationID != "" {
		runs, err := st.ListRunsByConversationSince(source.ConversationID, source.CreatedAt, MaxFollowOnRuns)
		if err != nil {
			return Timeline{}, err
		}
		runIDs = make([]string, 0, len(runs))
		for _, r := range runs {
			runIDs = append(runIDs, r.ID)
		}
	}

	var entries []TimelineEntry
	for _, runID := range runIDs {
		evs, err := st.ListEvents(runID)
		if err != nil {
			return Timeline{}, err
		}
		for _, ev := range evs {
			entries = append(entries, entryFromEvent(runID, ev))
		}
	}
	if len(entries) == 0 {
		return Timeline{}, ErrNoEvents
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	return Timeline{
		SourceRunID:    source.ID,
		ConversationID: source.ConversationID,
		Entries:        entries,
	}, nil
}

func entryFromEvent(runID string, ev store.Event) TimelineEntry {
	e := TimelineEntry{
		RunID:     runID,
		Type:      ev.Type,
		Timestamp: ev.Timestamp,
	}
	switch ev.Type {
	case "llm.tool_call", "tool.result":
		e.ToolName = toolNameFromData(ev.Data)
		if e.ToolName != "" {
			e.Summary = e.ToolName
		}
	case "hitl.waiting":
		e.ToolName = toolNameFromData(ev.Data)
		e.Summary = "HITL waiting"
		if e.ToolName != "" {
			e.Summary = fmt.Sprintf("HITL waiting: %s", e.ToolName)
		}
	case "hitl.resumed":
		e.Summary = hitlSummary("HITL resumed", ev.Data)
	case "hitl.rejected":
		e.Summary = hitlSummary("HITL rejected", ev.Data)
	case EventHumanSkillDraftOpened:
		src := dataString(ev.Data, "source")
		if src == "" {
			e.Summary = "opened skill draft"
		} else {
			e.Summary = fmt.Sprintf("opened skill draft (%s)", src)
		}
	case EventHumanSkillDraftPolished:
		e.Summary = "polished skill draft"
	case EventHumanSkillDraftInstalled:
		id := dataString(ev.Data, "skill_id")
		if id == "" {
			e.Summary = "installed skill draft"
		} else {
			e.Summary = fmt.Sprintf("installed skill %s", id)
		}
	default:
		if e.Summary == "" {
			e.Summary = ev.Type
		}
	}
	return e
}

func hitlSummary(prefix string, data map[string]any) string {
	dec := dataString(data, "decision")
	comment := truncateRunes(dataString(data, "comment"), maxHITLCommentRunes)
	parts := []string{prefix}
	if dec != "" {
		parts = append(parts, dec)
	}
	if comment != "" {
		parts = append(parts, comment)
	}
	return strings.Join(parts, ": ")
}

func dataString(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	v, ok := data[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func truncateRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
}

func toolNameFromData(data map[string]any) string {
	if data == nil {
		return ""
	}
	for _, key := range []string{"name", "tool", "tool_name"} {
		v, ok := data[key]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if ok && s != "" {
			return s
		}
	}
	return ""
}
