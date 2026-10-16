package schedule

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rebornace/baize/internal/dbutil"
)

const conversationSchedulesSchema = `
CREATE TABLE IF NOT EXISTS conversation_schedules (
  id TEXT PRIMARY KEY,
  owner_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  instruction TEXT NOT NULL,
  kind TEXT NOT NULL,
  timezone TEXT NOT NULL DEFAULT '',
  fire_at TEXT NOT NULL DEFAULT '',
  time_of_day TEXT NOT NULL DEFAULT '',
  weekday INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  next_run_at TEXT NOT NULL,
  running_run_id TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  revision INTEGER NOT NULL DEFAULT 0
);`

// MigrateConversationSchedules ensures the conversation_schedules table exists.
func MigrateConversationSchedules(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("schedule: db required")
	}
	if _, err := db.Exec(conversationSchedulesSchema); err != nil {
		return fmt.Errorf("schedule: migrate conversation_schedules: %w", err)
	}
	return nil
}

// SQLStore persists schedules in SQLite or Postgres via placeholder rebinding.
type SQLStore struct {
	db     *sql.DB
	rebind func(string) string
}

var _ Store = (*SQLStore)(nil)

// OpenSQLite creates a SQLStore on db and ensures schema exists.
func OpenSQLite(db *sql.DB) (*SQLStore, error) {
	if err := MigrateConversationSchedules(db); err != nil {
		return nil, err
	}
	return &SQLStore{db: db, rebind: func(q string) string { return q }}, nil
}

func (s *SQLStore) q(query string) string {
	if s.rebind != nil {
		return s.rebind(query)
	}
	return query
}

func (s *SQLStore) Insert(it Item) (Item, error) {
	if it.OwnerID == "" {
		return Item{}, fmt.Errorf("schedule: owner_id required")
	}
	status := it.Status
	if status == "" {
		status = StatusActive
	}
	if status == StatusActive {
		var n int
		err := s.db.QueryRow(s.q(
			`SELECT COUNT(*) FROM conversation_schedules WHERE owner_id = ? AND status = ?`,
		), it.OwnerID, StatusActive).Scan(&n)
		if err != nil {
			return Item{}, err
		}
		if n >= MaxActivePerOwner {
			return Item{}, ErrActiveCap
		}
	}
	out := it
	out.ID = "sched_" + uuid.NewString()
	out.Status = status
	out.Revision = 0
	_, err := s.db.Exec(s.q(`
INSERT INTO conversation_schedules (
  id, owner_id, conversation_id, instruction, kind, timezone, fire_at, time_of_day, weekday,
  status, next_run_at, running_run_id, last_error, revision
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		out.ID, out.OwnerID, out.ConversationID, out.Instruction, out.Kind, out.Timezone,
		formatScheduleTime(out.FireAt), out.TimeOfDay, out.Weekday,
		out.Status, formatScheduleTime(out.NextRunAt), out.RunningRunID, out.LastError, out.Revision,
	)
	if err != nil {
		return Item{}, err
	}
	return out, nil
}

func (s *SQLStore) ClaimDue(now time.Time) (Item, bool, error) {
	nowS := formatScheduleTime(now.UTC())
	row := s.db.QueryRow(s.q(`
UPDATE conversation_schedules
SET running_run_id = ?, revision = revision + 1
WHERE id = (
  SELECT id FROM conversation_schedules
  WHERE status = ? AND running_run_id = '' AND next_run_at <= ?
  ORDER BY next_run_at ASC, id ASC
  LIMIT 1
) AND running_run_id = ''
RETURNING id, owner_id, conversation_id, instruction, kind, timezone, fire_at, time_of_day, weekday,
  status, next_run_at, running_run_id, last_error, revision`),
		ClaimPending, StatusActive, nowS,
	)
	it, err := scanScheduleRow(row)
	if err == sql.ErrNoRows {
		return Item{}, false, nil
	}
	if err != nil {
		return Item{}, false, err
	}
	return it, true, nil
}

func (s *SQLStore) List(ownerID string) ([]Item, error) {
	rows, err := s.db.Query(s.q(`
SELECT id, owner_id, conversation_id, instruction, kind, timezone, fire_at, time_of_day, weekday,
  status, next_run_at, running_run_id, last_error, revision
FROM conversation_schedules WHERE owner_id = ? ORDER BY id`),
		ownerID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		it, err := scanScheduleRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *SQLStore) Pause(ownerID, id string) error {
	res, err := s.db.Exec(s.q(`
UPDATE conversation_schedules SET status = ?, revision = revision + 1
WHERE id = ? AND owner_id = ? AND status = ?`),
		StatusPaused, id, ownerID, StatusActive,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("schedule: pause failed")
	}
	return nil
}

func (s *SQLStore) Resume(ownerID, id string, next time.Time) error {
	res, err := s.db.Exec(s.q(`
UPDATE conversation_schedules SET status = ?, next_run_at = ?, revision = revision + 1
WHERE id = ? AND owner_id = ? AND status = ?`),
		StatusActive, formatScheduleTime(next.UTC()), id, ownerID, StatusPaused,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("schedule: resume failed")
	}
	return nil
}

func (s *SQLStore) Delete(ownerID, id string) error {
	res, err := s.db.Exec(s.q(`DELETE FROM conversation_schedules WHERE id = ? AND owner_id = ?`), id, ownerID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("schedule: not found")
	}
	return nil
}

func scanScheduleRow(row *sql.Row) (Item, error) {
	var (
		it      Item
		fireAt  string
		nextRun string
	)
	err := row.Scan(
		&it.ID, &it.OwnerID, &it.ConversationID, &it.Instruction, &it.Kind, &it.Timezone,
		&fireAt, &it.TimeOfDay, &it.Weekday, &it.Status, &nextRun, &it.RunningRunID, &it.LastError, &it.Revision,
	)
	if err != nil {
		return Item{}, err
	}
	it.FireAt, err = parseScheduleTime(fireAt)
	if err != nil {
		return Item{}, err
	}
	it.NextRunAt, err = parseScheduleTime(nextRun)
	if err != nil {
		return Item{}, err
	}
	return it, nil
}

func scanScheduleRows(rows *sql.Rows) (Item, error) {
	var (
		it      Item
		fireAt  string
		nextRun string
	)
	err := rows.Scan(
		&it.ID, &it.OwnerID, &it.ConversationID, &it.Instruction, &it.Kind, &it.Timezone,
		&fireAt, &it.TimeOfDay, &it.Weekday, &it.Status, &nextRun, &it.RunningRunID, &it.LastError, &it.Revision,
	)
	if err != nil {
		return Item{}, err
	}
	it.FireAt, err = parseScheduleTime(fireAt)
	if err != nil {
		return Item{}, err
	}
	it.NextRunAt, err = parseScheduleTime(nextRun)
	if err != nil {
		return Item{}, err
	}
	return it, nil
}

// openPostgresSQLStore is used from postgres.go.
func openPostgresSQLStore(db *sql.DB) (*SQLStore, error) {
	if err := MigrateConversationSchedules(db); err != nil {
		return nil, err
	}
	return &SQLStore{db: db, rebind: dbutil.RebindPostgres}, nil
}
