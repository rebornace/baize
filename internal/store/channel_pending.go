package store

import (
	"fmt"
	"sort"
	"time"
)

func channelPendingKey(conversationID, batchID string) string {
	return conversationID + "\x00" + batchID
}

// UpsertChannelPending stores or updates one busy-merge batch.
func (s *Memory) UpsertChannelPending(p ChannelPending) error {
	if p.ConversationID == "" || p.BatchID == "" {
		return fmt.Errorf("channel pending: conversation_id and batch_id are required")
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.channelPending == nil {
		s.channelPending = map[string]ChannelPending{}
	}
	s.channelPending[channelPendingKey(p.ConversationID, p.BatchID)] = p
	return nil
}

// ListChannelPending returns batches for source (all sources when empty),
// oldest first.
func (s *Memory) ListChannelPending(source string) ([]ChannelPending, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ChannelPending, 0, len(s.channelPending))
	for _, p := range s.channelPending {
		if source != "" && p.Source != source {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].UpdatedAt.Before(out[j].UpdatedAt)
		}
		if out[i].ConversationID != out[j].ConversationID {
			return out[i].ConversationID < out[j].ConversationID
		}
		return out[i].BatchID < out[j].BatchID
	})
	return out, nil
}

// DeleteChannelPending removes one batch (idempotent).
func (s *Memory) DeleteChannelPending(conversationID, batchID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.channelPending, channelPendingKey(conversationID, batchID))
	return nil
}
