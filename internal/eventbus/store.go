package eventbus

import "github.com/rebornace/baize/internal/store"

type notifyingStore struct {
	store.Store
	hub *Hub
}

// Notify wraps st so successful AppendEvent / terminal UpdateRun fan out via hub.
func Notify(st store.Store, hub *Hub) store.Store {
	return &notifyingStore{Store: st, hub: hub}
}

func (n *notifyingStore) AppendEvent(runID string, ev store.Event) error {
	if err := n.Store.AppendEvent(runID, ev); err != nil {
		return err
	}
	evs, err := n.Store.ListEvents(runID)
	if err != nil || len(evs) == 0 {
		return nil
	}
	idx := len(evs) - 1
	n.hub.Publish(runID, IndexedEvent{Index: idx, Event: evs[idx]})
	return nil
}

func (n *notifyingStore) UpdateRun(id string, status store.Status, output, errMsg string) error {
	if err := n.Store.UpdateRun(id, status, output, errMsg); err != nil {
		return err
	}
	if status == store.StatusSucceeded || status == store.StatusFailed || status == store.StatusCancelled {
		n.hub.PublishEnd(id, status)
	}
	return nil
}
