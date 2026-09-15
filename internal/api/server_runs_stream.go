package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/store"
)

// ssePollInterval is the SSE fallback poll period when Hub nudges are missing
// (e.g. cross-replica AppendEvent). Tests may shorten it via t.Cleanup restore.
var ssePollInterval = 3 * time.Second

func (s *Server) handleRunStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	runRec, err := s.Store.GetRun(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	if !s.requireConversationAccess(w, r, runRec.ConversationID) {
		return
	}

	after := parseStreamAfter(r)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)

	evs, err := s.Store.ListEvents(id)
	if err != nil {
		return
	}
	lastSent := after
	for i, ev := range evs {
		if i <= after {
			continue
		}
		if err := writeSSEEvent(w, rc, i, ev); err != nil {
			return
		}
		lastSent = i
	}

	terminal := runRec.Status == store.StatusSucceeded || runRec.Status == store.StatusFailed || runRec.Status == store.StatusRejected
	if terminal {
		_ = writeSSEEnded(w, rc, runRec.Status)
		return
	}
	if s.Hub == nil {
		// Replay-only: cannot subscribe for live increments.
		return
	}

	sub := s.Hub.Subscribe(id)
	defer sub.Cancel()

	// Fan-out before Subscribe is dropped; drain buffer then re-read store.
	var drainErr error
	lastSent, drainErr = drainSubEvents(w, rc, sub, lastSent)
	if drainErr != nil {
		return
	}
	if catchUpTerminal, status, err := catchUpRunStream(w, rc, s.Store, id, &lastSent); err != nil {
		return
	} else if catchUpTerminal {
		_ = writeSSEEnded(w, rc, status)
		return
	}
	lastSent, drainErr = drainSubEvents(w, rc, sub, lastSent)
	if drainErr != nil {
		return
	}

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	poll := time.NewTicker(ssePollInterval)
	defer poll.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return
			}
			_ = rc.Flush()
		case <-poll.C:
			if catchUpTerminal, status, err := catchUpRunStream(w, rc, s.Store, id, &lastSent); err != nil {
				return
			} else if catchUpTerminal {
				_ = writeSSEEnded(w, rc, status)
				return
			}
		case ev, ok := <-sub.Events:
			if !ok {
				return
			}
			if ev.Event.Type == "external.nudge" {
				if catchUpTerminal, status, err := catchUpRunStream(w, rc, s.Store, id, &lastSent); err != nil {
					return
				} else if catchUpTerminal {
					_ = writeSSEEnded(w, rc, status)
					return
				}
				continue
			}
			if ev.Index <= lastSent {
				continue
			}
			if err := writeSSEEvent(w, rc, ev.Index, ev.Event); err != nil {
				return
			}
			lastSent = ev.Index
		case stt, ok := <-sub.Ended:
			if !ok {
				return
			}
			// Events and Ended may both be ready; drain events first so the
			// final AppendEvent is not lost when select picks Ended. Then
			// catch up from store: channel may only hold external.nudge
			// placeholders while real events live in the store.
			lastSent, drainErr = drainSubEvents(w, rc, sub, lastSent)
			if drainErr != nil {
				return
			}
			if _, _, err := catchUpRunStream(w, rc, s.Store, id, &lastSent); err != nil {
				return
			}
			_ = writeSSEEnded(w, rc, stt)
			return
		}
	}
}

// drainSubEvents non-blocking writes any buffered subscription events with index > lastSent.
// external.nudge placeholders are skipped (no write / no lastSent advance); callers
// catch up from the store via catchUpRunStream.
func drainSubEvents(w http.ResponseWriter, rc *http.ResponseController, sub *eventbus.Subscription, lastSent int) (int, error) {
	for {
		select {
		case ev, ok := <-sub.Events:
			if !ok {
				return lastSent, nil
			}
			if ev.Event.Type == "external.nudge" {
				continue
			}
			if ev.Index <= lastSent {
				continue
			}
			if err := writeSSEEvent(w, rc, ev.Index, ev.Event); err != nil {
				return lastSent, err
			}
			lastSent = ev.Index
		default:
			return lastSent, nil
		}
	}
}

// catchUpRunStream re-reads the store after Subscribe to recover events/status
// published in the ListEvents→Subscribe window (no subscriber yet).
func catchUpRunStream(w http.ResponseWriter, rc *http.ResponseController, st store.Store, id string, lastSent *int) (terminal bool, status store.Status, err error) {
	runRec, err := st.GetRun(id)
	if err != nil {
		return false, "", err
	}
	evs, err := st.ListEvents(id)
	if err != nil {
		return false, "", err
	}
	for i, ev := range evs {
		if i <= *lastSent {
			continue
		}
		if err := writeSSEEvent(w, rc, i, ev); err != nil {
			return false, "", err
		}
		*lastSent = i
	}
	if runRec.Status == store.StatusSucceeded || runRec.Status == store.StatusFailed || runRec.Status == store.StatusRejected {
		return true, runRec.Status, nil
	}
	return false, "", nil
}

func parseStreamAfter(r *http.Request) int {
	after := -1
	if q := r.URL.Query().Get("after"); q != "" {
		if n, err := strconv.Atoi(q); err == nil {
			after = n
		} else {
			after = -1
		}
	}
	if id := r.Header.Get("Last-Event-ID"); id != "" {
		if n, err := strconv.Atoi(id); err == nil {
			after = n
		} else {
			after = -1
		}
	}
	return after
}

func writeSSEEvent(w http.ResponseWriter, rc *http.ResponseController, index int, ev store.Event) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", index, payload); err != nil {
		return err
	}
	return rc.Flush()
}

func writeSSEEnded(w http.ResponseWriter, rc *http.ResponseController, status store.Status) error {
	payload, err := json.Marshal(map[string]string{"status": string(status)})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: run.ended\ndata: %s\n\n", payload); err != nil {
		return err
	}
	return rc.Flush()
}
