package conversation

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rebornace/baize/internal/store"
)

const sqliteMessagesSchema = `
CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL,
  role TEXT NOT NULL,
  content TEXT NOT NULL,
  thinking TEXT,
  thinking_redacted INTEGER NOT NULL DEFAULT 0,
  run_id TEXT,
  created_at TEXT NOT NULL,
  pinned_skills TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX IF NOT EXISTS idx_messages_conv_created ON messages(conversation_id, created_at);
CREATE TABLE IF NOT EXISTS conversation_meta (
  id TEXT PRIMARY KEY,
  owner_id TEXT NOT NULL,
  source TEXT NOT NULL,
  title TEXT,
  channel_peer TEXT,
  workspace_id TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS conversation_summaries (
  conversation_id TEXT PRIMARY KEY,
  summary TEXT NOT NULL,
  covers_through_message_id TEXT NOT NULL,
  covers_through_order INTEGER NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS conversation_projections (
  conversation_id TEXT PRIMARY KEY,
  pins TEXT NOT NULL DEFAULT '[]',
  summary TEXT NOT NULL DEFAULT '',
  dropped TEXT NOT NULL DEFAULT '[]',
  revision INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL
);
`

// SQLiteStore persists conversation messages in SQLite.
//
// Callers must configure the shared *sql.DB for serialized access before OpenSQLite:
// SetMaxOpenConns(1), SetMaxIdleConns(1), and PRAGMA busy_timeout (see store.OpenSQLite).
// Without these settings, concurrent writers may hit "database is locked".
type SQLiteStore struct {
	db      *sql.DB
	dialect store.SQLDialect
}

var _ Store = (*SQLiteStore)(nil)

// OpenSQLite creates a SQLiteStore on db, ensuring the messages schema exists.
//
// db must already use MaxOpenConns(1) and busy_timeout, or be the same *sql.DB opened
// by store.OpenSQLite. OpenSQLite does not apply connection-pool or pragma settings.
func OpenSQLite(db *sql.DB) (*SQLiteStore, error) {
	return OpenSQL(db, store.DialectSQLite)
}

func (s *SQLiteStore) Append(conversationID string, msg Message) (Message, error) {
	msg.ID = "msg_" + uuid.NewString()
	msg.ConversationID = conversationID
	msg.CreatedAt = time.Now().UTC()

	var runID any
	if msg.RunID != "" {
		runID = msg.RunID
	}

	_, err := s.exec(
		`INSERT INTO messages (id, conversation_id, role, content, thinking, thinking_redacted, run_id, created_at, pinned_skills) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.ConversationID, msg.Role, msg.Content, nullIfEmpty(msg.Thinking), msg.ThinkingRedacted, runID, msg.CreatedAt.Format(time.RFC3339Nano), encodeJSONList(msg.PinnedSkills),
	)
	if err != nil {
		return Message{}, err
	}
	return msg, nil
}

func (s *SQLiteStore) List(conversationID string) []Message {
	rows, err := s.query(
		`SELECT id, conversation_id, role, content, thinking, thinking_redacted, run_id, created_at, pinned_skills
		 FROM messages WHERE conversation_id = ? ORDER BY created_at ASC`,
		conversationID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	return out
}

func (s *SQLiteStore) ListWindow(conversationID string, n int) []Message {
	all := s.List(conversationID)
	if n <= 0 || n >= len(all) {
		return all
	}
	return all[len(all)-n:]
}

func (s *SQLiteStore) ListSummaries() []Summary {
	rows, err := s.query(
		`SELECT conversation_id, MAX(created_at) FROM messages GROUP BY conversation_id`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		var maxCreated string
		if err := rows.Scan(&id, &maxCreated); err != nil {
			return nil
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil
	}

	out := make([]Summary, 0, len(ids))
	for _, id := range ids {
		msgs := s.List(id)
		if len(msgs) == 0 {
			continue
		}
		out = append(out, Summarize(id, msgs))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (s *SQLiteStore) Clear(conversationID string) {
	_, _ = s.exec(`DELETE FROM messages WHERE conversation_id = ?`, conversationID)
	s.ClearRollingSummary(conversationID)
	s.ClearContextProjection(conversationID)
}

func (s *SQLiteStore) TruncateFrom(conversationID, messageID string) (int, error) {
	var createdAt string
	err := s.queryRow(
		`SELECT created_at FROM messages WHERE id = ? AND conversation_id = ?`,
		messageID, conversationID,
	).Scan(&createdAt)
	if err == sql.ErrNoRows {
		return 0, ErrMessageNotFound
	}
	if err != nil {
		return 0, err
	}
	res, err := s.exec(
		`DELETE FROM messages WHERE conversation_id = ? AND created_at >= ?`,
		conversationID, createdAt,
	)
	if err != nil {
		return 0, err
	}
	s.ClearRollingSummary(conversationID)
	s.ClearContextProjection(conversationID)
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *SQLiteStore) Fork(srcConversationID, throughMessageID string) (string, int, error) {
	var createdAt string
	err := s.queryRow(
		`SELECT created_at FROM messages WHERE id = ? AND conversation_id = ?`,
		throughMessageID, srcConversationID,
	).Scan(&createdAt)
	if err == sql.ErrNoRows {
		return "", 0, ErrMessageNotFound
	}
	if err != nil {
		return "", 0, err
	}
	rows, err := s.query(
		`SELECT id, conversation_id, role, content, thinking, thinking_redacted, run_id, created_at, pinned_skills
		 FROM messages WHERE conversation_id = ? AND created_at <= ? ORDER BY created_at ASC`,
		srcConversationID, createdAt,
	)
	if err != nil {
		return "", 0, err
	}
	defer rows.Close()

	var toCopy []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return "", 0, err
		}
		toCopy = append(toCopy, m)
	}
	if err := rows.Err(); err != nil {
		return "", 0, err
	}
	if len(toCopy) == 0 {
		return "", 0, ErrMessageNotFound
	}

	newID := "conv_" + uuid.NewString()
	for _, m := range toCopy {
		newMsg := m
		newMsg.ID = "msg_" + uuid.NewString()
		newMsg.ConversationID = newID
		var runID any
		if newMsg.RunID != "" {
			runID = newMsg.RunID
		}
		_, err := s.exec(
			`INSERT INTO messages (id, conversation_id, role, content, thinking, thinking_redacted, run_id, created_at, pinned_skills) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			newMsg.ID, newMsg.ConversationID, newMsg.Role, newMsg.Content, nullIfEmpty(newMsg.Thinking), newMsg.ThinkingRedacted, runID, newMsg.CreatedAt.Format(time.RFC3339Nano), encodeJSONList(newMsg.PinnedSkills),
		)
		if err != nil {
			return "", 0, err
		}
	}
	return newID, len(toCopy), nil
}

func (s *SQLiteStore) SetRunID(messageID, runID string) error {
	res, err := s.exec(`UPDATE messages SET run_id = ? WHERE id = ?`, runID, messageID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrMessageNotFound
	}
	return nil
}

var _ MetaStore = (*SQLiteStore)(nil)

func (s *SQLiteStore) EnsureMeta(m Meta) error {
	if m.ID == "" {
		return fmt.Errorf("meta id required")
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = time.Now().UTC()
	}
	if m.Source == "ui" && strings.TrimSpace(m.WorkspaceID) == "" {
		m.WorkspaceID = DefaultWorkspaceID
	}
	_, err := s.exec(
		`INSERT INTO conversation_meta (id, owner_id, source, title, channel_peer, workspace_id, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO NOTHING`,
		m.ID, m.OwnerID, m.Source, nullIfEmpty(m.Title), nullIfEmpty(m.ChannelPeer),
		m.WorkspaceID, m.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteStore) GetMeta(id string) (Meta, error) {
	row := s.queryRow(
		`SELECT id, owner_id, source, title, channel_peer, workspace_id, updated_at
		 FROM conversation_meta WHERE id = ?`, id,
	)
	m, err := scanMeta(row)
	if err == sql.ErrNoRows {
		return Meta{}, ErrMetaNotFound
	}
	if err != nil {
		return Meta{}, err
	}
	return m, nil
}

// DeleteMeta removes the meta row. Deleting a missing id is a no-op (0 rows).
func (s *SQLiteStore) DeleteMeta(id string) error {
	_, err := s.exec(`DELETE FROM conversation_meta WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) ListMeta(filter MetaFilter) ([]Meta, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if filter.OwnerID != "" && filter.WorkspaceID != "" {
		rows, err = s.query(
			`SELECT id, owner_id, source, title, channel_peer, workspace_id, updated_at
			 FROM conversation_meta WHERE owner_id = ? AND workspace_id = ? ORDER BY updated_at DESC`,
			filter.OwnerID, filter.WorkspaceID,
		)
	} else if filter.OwnerID != "" {
		rows, err = s.query(
			`SELECT id, owner_id, source, title, channel_peer, workspace_id, updated_at
			 FROM conversation_meta WHERE owner_id = ? ORDER BY updated_at DESC`,
			filter.OwnerID,
		)
	} else if filter.WorkspaceID != "" {
		rows, err = s.query(
			`SELECT id, owner_id, source, title, channel_peer, workspace_id, updated_at
			 FROM conversation_meta WHERE workspace_id = ? ORDER BY updated_at DESC`,
			filter.WorkspaceID,
		)
	} else {
		rows, err = s.query(
			`SELECT id, owner_id, source, title, channel_peer, workspace_id, updated_at
			 FROM conversation_meta ORDER BY updated_at DESC`,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Meta
	for rows.Next() {
		m, err := scanMeta(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func scanMeta(scanner interface {
	Scan(dest ...any) error
}) (Meta, error) {
	var m Meta
	var title, peer, workspaceID sql.NullString
	var updatedAt string
	if err := scanner.Scan(&m.ID, &m.OwnerID, &m.Source, &title, &peer, &workspaceID, &updatedAt); err != nil {
		return Meta{}, err
	}
	if title.Valid {
		m.Title = title.String
	}
	if peer.Valid {
		m.ChannelPeer = peer.String
	}
	if workspaceID.Valid {
		m.WorkspaceID = workspaceID.String
	}
	ts, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		ts, err = time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return Meta{}, fmt.Errorf("parse updated_at: %w", err)
		}
	}
	m.UpdatedAt = ts
	return m, nil
}

func scanMessage(scanner interface {
	Scan(dest ...any) error
}) (Message, error) {
	var m Message
	var createdAt string
	var thinking, runID, pinnedSkills sql.NullString
	var redacted bool
	if err := scanner.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &thinking, &redacted, &runID, &createdAt, &pinnedSkills); err != nil {
		return Message{}, err
	}
	if thinking.Valid {
		m.Thinking = thinking.String
	}
	m.ThinkingRedacted = redacted
	if runID.Valid {
		m.RunID = runID.String
	}
	if pinnedSkills.Valid {
		m.PinnedSkills = decodeJSONList(pinnedSkills.String)
	}
	ts, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		ts, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return Message{}, fmt.Errorf("parse created_at: %w", err)
		}
	}
	m.CreatedAt = ts
	return m, nil
}

func (s *SQLiteStore) GetRollingSummary(conversationID string) (RollingSummary, bool) {
	var rs RollingSummary
	var updated string
	err := s.queryRow(
		`SELECT conversation_id, summary, covers_through_message_id, covers_through_order, updated_at
		 FROM conversation_summaries WHERE conversation_id = ?`, conversationID,
	).Scan(&rs.ConversationID, &rs.Summary, &rs.CoversThroughMessageID, &rs.CoversThroughOrder, &updated)
	if err != nil {
		return RollingSummary{}, false
	}
	if t, perr := time.Parse(time.RFC3339Nano, updated); perr == nil {
		rs.UpdatedAt = t
	}
	return rs, true
}

func (s *SQLiteStore) UpsertRollingSummary(sum RollingSummary) error {
	if sum.ConversationID == "" {
		return fmt.Errorf("conversation id required")
	}
	if sum.UpdatedAt.IsZero() {
		sum.UpdatedAt = time.Now().UTC()
	}
	_, err := s.exec(
		`INSERT INTO conversation_summaries
		   (conversation_id, summary, covers_through_message_id, covers_through_order, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(conversation_id) DO UPDATE SET
		   summary = excluded.summary,
		   covers_through_message_id = excluded.covers_through_message_id,
		   covers_through_order = excluded.covers_through_order,
		   updated_at = excluded.updated_at`,
		sum.ConversationID, sum.Summary, sum.CoversThroughMessageID, sum.CoversThroughOrder,
		sum.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteStore) ClearRollingSummary(conversationID string) {
	_, _ = s.exec(`DELETE FROM conversation_summaries WHERE conversation_id = ?`, conversationID)
}

func (s *SQLiteStore) GetContextProjection(conversationID string) (ContextProjection, bool) {
	var p ContextProjection
	var pins, dropped, updated string
	err := s.queryRow(
		`SELECT conversation_id, pins, summary, dropped, revision, updated_at
		 FROM conversation_projections WHERE conversation_id = ?`, conversationID,
	).Scan(&p.ConversationID, &pins, &p.Summary, &dropped, &p.Revision, &updated)
	if err != nil {
		return ContextProjection{}, false
	}
	p.Pins = decodeJSONList(pins)
	p.Dropped = decodeJSONList(dropped)
	if t, perr := time.Parse(time.RFC3339Nano, updated); perr == nil {
		p.UpdatedAt = t
	}
	return p, true
}

func (s *SQLiteStore) UpsertContextProjection(p ContextProjection) error {
	if err := validateProjection(p); err != nil {
		return err
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = time.Now().UTC()
	}
	_, err := s.exec(
		`INSERT INTO conversation_projections
		   (conversation_id, pins, summary, dropped, revision, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(conversation_id) DO UPDATE SET
		   pins = excluded.pins,
		   summary = excluded.summary,
		   dropped = excluded.dropped,
		   revision = excluded.revision,
		   updated_at = excluded.updated_at`,
		p.ConversationID, encodeJSONList(p.Pins), p.Summary, encodeJSONList(p.Dropped),
		p.Revision, p.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteStore) ClearContextProjection(conversationID string) {
	_, _ = s.exec(`DELETE FROM conversation_projections WHERE conversation_id = ?`, conversationID)
}

var _ WorkspaceStore = (*SQLiteStore)(nil)

func (s *SQLiteStore) EnsureDefaultWorkspace() error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.exec(
		`INSERT INTO chat_workspaces (id, name, created_at) VALUES (?, ?, ?)
		 ON CONFLICT(id) DO NOTHING`,
		DefaultWorkspaceID, DefaultWorkspaceName, now,
	)
	return err
}

func (s *SQLiteStore) ListWorkspaces() ([]Workspace, error) {
	if err := s.EnsureDefaultWorkspace(); err != nil {
		return nil, err
	}
	rows, err := s.query(`SELECT id, name, created_at FROM chat_workspaces ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Workspace
	for rows.Next() {
		w, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID == DefaultWorkspaceID {
			return true
		}
		if out[j].ID == DefaultWorkspaceID {
			return false
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, rows.Err()
}

func (s *SQLiteStore) GetWorkspace(id string) (Workspace, error) {
	id = NormalizeWorkspaceID(id)
	if err := s.EnsureDefaultWorkspace(); err != nil {
		return Workspace{}, err
	}
	row := s.queryRow(`SELECT id, name, created_at FROM chat_workspaces WHERE id = ?`, id)
	w, err := scanWorkspace(row)
	if err == sql.ErrNoRows {
		return Workspace{}, fmt.Errorf("workspace not found")
	}
	return w, err
}

func (s *SQLiteStore) CreateWorkspace(name string) (Workspace, error) {
	name, err := validateWorkspaceName(name)
	if err != nil {
		return Workspace{}, err
	}
	if err := s.EnsureDefaultWorkspace(); err != nil {
		return Workspace{}, err
	}
	w := Workspace{ID: newWorkspaceID(), Name: name, CreatedAt: time.Now().UTC()}
	if _, err := s.exec(
		`INSERT INTO chat_workspaces (id, name, created_at) VALUES (?, ?, ?)`,
		w.ID, w.Name, w.CreatedAt.Format(time.RFC3339Nano),
	); err != nil {
		return Workspace{}, err
	}
	return w, nil
}

func scanWorkspace(scanner interface {
	Scan(dest ...any) error
}) (Workspace, error) {
	var w Workspace
	var created string
	if err := scanner.Scan(&w.ID, &w.Name, &created); err != nil {
		return Workspace{}, err
	}
	ts, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		ts, err = time.Parse(time.RFC3339, created)
		if err != nil {
			return Workspace{}, fmt.Errorf("parse workspace created_at: %w", err)
		}
	}
	w.CreatedAt = ts
	return w, nil
}
