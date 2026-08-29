package conversation

import (
	"database/sql"
	"fmt"
	"sort"
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
  run_id TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_conv_created ON messages(conversation_id, created_at);
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
		`INSERT INTO messages (id, conversation_id, role, content, run_id, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.ConversationID, msg.Role, msg.Content, runID, msg.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return Message{}, err
	}
	return msg, nil
}

func (s *SQLiteStore) List(conversationID string) []Message {
	rows, err := s.query(
		`SELECT id, conversation_id, role, content, run_id, created_at
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
		`SELECT id, conversation_id, role, content, run_id, created_at
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
			`INSERT INTO messages (id, conversation_id, role, content, run_id, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			newMsg.ID, newMsg.ConversationID, newMsg.Role, newMsg.Content, runID, newMsg.CreatedAt.Format(time.RFC3339Nano),
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

func scanMessage(scanner interface {
	Scan(dest ...any) error
}) (Message, error) {
	var m Message
	var createdAt string
	var runID sql.NullString
	if err := scanner.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &runID, &createdAt); err != nil {
		return Message{}, err
	}
	if runID.Valid {
		m.RunID = runID.String
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
