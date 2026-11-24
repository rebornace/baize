package store

import (
	"errors"
	"sort"
	"time"
)

const attachmentListDefaultLimit = 100
const attachmentListMaxLimit = 500

func normalizeAttachmentLimit(limit int) int {
	if limit <= 0 {
		return attachmentListDefaultLimit
	}
	if limit > attachmentListMaxLimit {
		return attachmentListMaxLimit
	}
	return limit
}

func (s *Memory) AddConversationAttachment(a ConversationAttachment) error {
	if a.ConversationID == "" || a.Object == "" {
		return errors.New("attachment: conversation and object are required")
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attachments == nil {
		s.attachments = map[string]map[string]ConversationAttachment{}
	}
	if s.attachments[a.ConversationID] == nil {
		s.attachments[a.ConversationID] = map[string]ConversationAttachment{}
	}
	s.attachments[a.ConversationID][a.Object] = a
	return nil
}

func (s *Memory) GetConversationAttachment(conversationID, object string) (ConversationAttachment, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.attachments[conversationID][object]
	return a, ok, nil
}

func (s *Memory) ListConversationAttachments(conversationID string, limit int) ([]ConversationAttachment, error) {
	limit = normalizeAttachmentLimit(limit)
	s.mu.RLock()
	out := make([]ConversationAttachment, 0, len(s.attachments[conversationID]))
	for _, a := range s.attachments[conversationID] {
		out = append(out, a)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].Object > out[j].Object
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Memory) DeleteConversationAttachment(conversationID, object string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if byObject := s.attachments[conversationID]; byObject != nil {
		delete(byObject, object)
		if len(byObject) == 0 {
			delete(s.attachments, conversationID)
		}
	}
	return nil
}

func (s *Memory) ListConversationAttachmentsBefore(before time.Time, limit int) ([]ConversationAttachment, error) {
	limit = normalizeAttachmentLimit(limit)
	s.mu.RLock()
	out := make([]ConversationAttachment, 0)
	for _, byObject := range s.attachments {
		for _, a := range byObject {
			if a.CreatedAt.Before(before) {
				out = append(out, a)
			}
		}
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *SQLStore) AddConversationAttachment(a ConversationAttachment) error {
	if a.ConversationID == "" || a.Object == "" {
		return errors.New("attachment: conversation and object are required")
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	_, err := s.exec(`INSERT INTO conversation_attachments
		(conversation_id, object, filename, media_type, size_bytes, workspace_path, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (conversation_id, object) DO UPDATE SET
		  filename = excluded.filename, media_type = excluded.media_type,
		  size_bytes = excluded.size_bytes, workspace_path = excluded.workspace_path,
		  created_at = excluded.created_at`,
		a.ConversationID, a.Object, a.Filename, a.MediaType, a.SizeBytes, a.WorkspacePath,
		a.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *SQLStore) GetConversationAttachment(conversationID, object string) (ConversationAttachment, bool, error) {
	var (
		a         ConversationAttachment
		createdAt string
	)
	err := s.queryRow(`SELECT conversation_id, object, filename, media_type, size_bytes, workspace_path, created_at
		FROM conversation_attachments WHERE conversation_id = ? AND object = ?`, conversationID, object).
		Scan(&a.ConversationID, &a.Object, &a.Filename, &a.MediaType, &a.SizeBytes, &a.WorkspacePath, &createdAt)
	if err != nil {
		return ConversationAttachment{}, false, nil
	}
	a.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return a, true, nil
}

func (s *SQLStore) ListConversationAttachments(conversationID string, limit int) ([]ConversationAttachment, error) {
	limit = normalizeAttachmentLimit(limit)
	rows, err := s.query(`SELECT conversation_id, object, filename, media_type, size_bytes, workspace_path, created_at
		FROM conversation_attachments WHERE conversation_id = ?
		ORDER BY created_at DESC, object DESC LIMIT ?`, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ConversationAttachment, 0)
	for rows.Next() {
		a, err := scanConversationAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *SQLStore) DeleteConversationAttachment(conversationID, object string) error {
	_, err := s.exec(`DELETE FROM conversation_attachments WHERE conversation_id = ? AND object = ?`,
		conversationID, object)
	return err
}

func (s *SQLStore) ListConversationAttachmentsBefore(before time.Time, limit int) ([]ConversationAttachment, error) {
	limit = normalizeAttachmentLimit(limit)
	rows, err := s.query(`SELECT conversation_id, object, filename, media_type, size_bytes, workspace_path, created_at
		FROM conversation_attachments WHERE created_at < ?
		ORDER BY created_at ASC, object ASC LIMIT ?`,
		before.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ConversationAttachment, 0)
	for rows.Next() {
		a, err := scanConversationAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanConversationAttachment(row rowScanner) (ConversationAttachment, error) {
	var (
		a         ConversationAttachment
		createdAt string
	)
	if err := row.Scan(&a.ConversationID, &a.Object, &a.Filename, &a.MediaType,
		&a.SizeBytes, &a.WorkspacePath, &createdAt); err != nil {
		return ConversationAttachment{}, err
	}
	a.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return a, nil
}
