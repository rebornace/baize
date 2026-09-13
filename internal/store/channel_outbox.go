package store

import (
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
)

var errChannelOutboxSQLNotImplemented = errors.New("channel outbox: sql store not implemented")

func normalizeChannelOutboxEntry(e *ChannelOutboxEntry, now time.Time) {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.MaxAttempts <= 0 {
		e.MaxAttempts = ChannelOutboxMaxAttempts
	}
	if e.Status == "" {
		e.Status = ChannelOutboxPending
	}
	if len(e.BlobKeysJSON) == 0 {
		e.BlobKeysJSON = []byte(`[]`)
	}
	if e.NextRetryAt.IsZero() {
		e.NextRetryAt = now
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	e.UpdatedAt = now
}

func channelOutboxActive(status ChannelOutboxStatus) bool {
	return status == ChannelOutboxPending || status == ChannelOutboxDelivered
}

func (s *Memory) PutChannelOutboxIfAbsent(entry ChannelOutboxEntry) (bool, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.channelOutbox == nil {
		s.channelOutbox = map[string]ChannelOutboxEntry{}
	}
	now := time.Now().UTC()
	normalizeChannelOutboxEntry(&entry, now)
	for _, existing := range s.channelOutbox {
		if existing.DeliveryKey == entry.DeliveryKey && channelOutboxActive(existing.Status) {
			return false, existing.ID, nil
		}
	}
	s.channelOutbox[entry.ID] = entry
	return true, entry.ID, nil
}

func (s *Memory) ListChannelOutboxDue(now time.Time, limit int) ([]ChannelOutboxEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 20
	}
	var due []ChannelOutboxEntry
	for _, e := range s.channelOutbox {
		if e.Status != ChannelOutboxPending {
			continue
		}
		if e.NextRetryAt.After(now) {
			continue
		}
		due = append(due, e)
	}
	sort.Slice(due, func(i, j int) bool {
		if due[i].NextRetryAt.Equal(due[j].NextRetryAt) {
			return due[i].CreatedAt.Before(due[j].CreatedAt)
		}
		return due[i].NextRetryAt.Before(due[j].NextRetryAt)
	})
	if len(due) > limit {
		due = due[:limit]
	}
	return due, nil
}

func (s *Memory) ListChannelOutbox(channel string, statuses []ChannelOutboxStatus, limit int) ([]ChannelOutboxEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	allowed := map[ChannelOutboxStatus]bool{}
	for _, st := range statuses {
		allowed[st] = true
	}
	var out []ChannelOutboxEntry
	for _, e := range s.channelOutbox {
		if e.Channel != channel {
			continue
		}
		if len(allowed) > 0 && !allowed[e.Status] {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Memory) GetChannelOutbox(id string) (ChannelOutboxEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.channelOutbox[id]
	if !ok {
		return ChannelOutboxEntry{}, ErrChannelOutboxNotFound
	}
	return e, nil
}

func (s *Memory) UpdateChannelOutbox(entry ChannelOutboxEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.channelOutbox[entry.ID]; !ok {
		return ErrChannelOutboxNotFound
	}
	entry.UpdatedAt = time.Now().UTC()
	s.channelOutbox[entry.ID] = entry
	return nil
}

func (s *Memory) ResetChannelOutboxRetry(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.channelOutbox[id]
	if !ok {
		return ErrChannelOutboxNotFound
	}
	now := time.Now().UTC()
	e.Status = ChannelOutboxPending
	e.Attempt = 0
	e.LastError = ""
	e.NextRetryAt = now
	e.UpdatedAt = now
	s.channelOutbox[id] = e
	return nil
}

func (s *SQLStore) PutChannelOutboxIfAbsent(entry ChannelOutboxEntry) (bool, string, error) {
	return false, "", errChannelOutboxSQLNotImplemented
}

func (s *SQLStore) ListChannelOutboxDue(now time.Time, limit int) ([]ChannelOutboxEntry, error) {
	return nil, errChannelOutboxSQLNotImplemented
}

func (s *SQLStore) ListChannelOutbox(channel string, statuses []ChannelOutboxStatus, limit int) ([]ChannelOutboxEntry, error) {
	return nil, errChannelOutboxSQLNotImplemented
}

func (s *SQLStore) GetChannelOutbox(id string) (ChannelOutboxEntry, error) {
	return ChannelOutboxEntry{}, errChannelOutboxSQLNotImplemented
}

func (s *SQLStore) UpdateChannelOutbox(entry ChannelOutboxEntry) error {
	return errChannelOutboxSQLNotImplemented
}

func (s *SQLStore) ResetChannelOutboxRetry(id string) error {
	return errChannelOutboxSQLNotImplemented
}
