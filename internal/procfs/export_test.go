package procfs

import "github.com/rebornace/baize/internal/store"

// EntryFromEventForTest exposes entryFromEvent to external tests.
func EntryFromEventForTest(runID string, ev store.Event) TimelineEntry {
	return entryFromEvent(runID, ev)
}
