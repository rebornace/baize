package eval

import (
	"time"
	"unicode/utf8"

	"github.com/rebornace/baize/internal/store"
)

const inputPreviewMaxRunes = 120

// Query is the EV1a filter for candidate runs in a created_at window.
type Query struct {
	From          time.Time
	To            time.Time
	Status        store.Status
	MinDurationMs int64
	EventType     string
	Limit         int
}

// Candidate is a run summary returned by QueryCandidates.
type Candidate struct {
	ID           string
	AgentID      string
	Status       store.Status
	CreatedAt    time.Time
	EndedAt      *time.Time
	DurationMs   *int64
	InputPreview string
}

func previewInput(s string) string {
	if utf8.RuneCountInString(s) <= inputPreviewMaxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:inputPreviewMaxRunes]) + "…"
}

func runHasEventType(evs []store.Event, eventType string) bool {
	for _, ev := range evs {
		if ev.Type == eventType {
			return true
		}
	}
	return false
}

func runTiming(createdAt time.Time, evs []store.Event) (endedAt time.Time, durationMs int64, ok bool) {
	if len(evs) == 0 {
		return time.Time{}, 0, false
	}
	last := evs[0].Timestamp
	for _, ev := range evs[1:] {
		if ev.Timestamp.After(last) {
			last = ev.Timestamp
		}
	}
	d := last.Sub(createdAt)
	if d < 0 {
		d = 0
	}
	return last, d.Milliseconds(), true
}

// QueryCandidates lists runs in the time window with optional status, duration, and event-type filters.
func QueryCandidates(st store.Store, q Query) ([]Candidate, error) {
	limit := normalizeQueryLimit(q.Limit)
	runs, err := st.ListRunsCreatedBetween(q.From, q.To, q.Status, storeMaxFetch())
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, limit)
	for _, r := range runs {
		evs, err := st.ListEvents(r.ID)
		if err != nil {
			return nil, err
		}
		if q.EventType != "" && !runHasEventType(evs, q.EventType) {
			continue
		}
		ended, durMs, hasTiming := runTiming(r.CreatedAt, evs)
		if q.MinDurationMs > 0 {
			if !hasTiming || durMs < q.MinDurationMs {
				continue
			}
		}
		cand := Candidate{
			ID:           r.ID,
			AgentID:      r.AgentID,
			Status:       r.Status,
			CreatedAt:    r.CreatedAt,
			InputPreview: previewInput(r.Input),
		}
		if hasTiming {
			end := ended.UTC()
			cand.EndedAt = &end
			ms := durMs
			cand.DurationMs = &ms
		}
		out = append(out, cand)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func normalizeQueryLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func storeMaxFetch() int {
	return 500
}
