package skill

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/dbutil"
	"github.com/rebornace/baize/internal/store"
)

const skillUsageSchema = `
CREATE TABLE IF NOT EXISTS skill_usage (
  skill_id TEXT PRIMARY KEY,
  activate_count INTEGER NOT NULL DEFAULT 0,
  last_activated_at TEXT,
  installed_at TEXT NOT NULL
);
`

// SQLUsageStore persists skill usage on a shared SQL database.
type SQLUsageStore struct {
	db      *sql.DB
	dialect store.SQLDialect
}

var _ UsageStore = (*SQLUsageStore)(nil)

// OpenUsage opens a UsageStore for driver ("sqlite"/"postgres") on db.
func OpenUsage(driver string, db *sql.DB) (UsageStore, error) {
	if db == nil {
		return nil, fmt.Errorf("db required")
	}
	dialect := store.DialectSQLite
	if strings.EqualFold(driver, "postgres") || strings.EqualFold(driver, "postgresql") {
		dialect = store.DialectPostgres
	}
	if _, err := db.Exec(skillUsageSchema); err != nil {
		return nil, fmt.Errorf("migrate skill_usage: %w", err)
	}
	return &SQLUsageStore{db: db, dialect: dialect}, nil
}

func (s *SQLUsageStore) q(query string) string {
	if s.dialect == store.DialectPostgres {
		return dbutil.RebindPostgres(query)
	}
	return query
}

func (s *SQLUsageStore) RecordActivate(skillID string, at time.Time) error {
	if skillID == "" {
		return fmt.Errorf("skill id required")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}
	ts := at.Format(time.RFC3339Nano)
	_, err := s.db.Exec(s.q(`
INSERT INTO skill_usage (skill_id, activate_count, last_activated_at, installed_at)
VALUES (?, 1, ?, ?)
ON CONFLICT(skill_id) DO UPDATE SET
  activate_count = skill_usage.activate_count + 1,
  last_activated_at = excluded.last_activated_at
`), skillID, ts, ts)
	if err != nil {
		return err
	}
	return nil
}

func (s *SQLUsageStore) EnsureInstalled(skillID string, at time.Time) error {
	if skillID == "" {
		return fmt.Errorf("skill id required")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}
	ts := at.Format(time.RFC3339Nano)
	_, err := s.db.Exec(s.q(`
INSERT INTO skill_usage (skill_id, activate_count, last_activated_at, installed_at)
VALUES (?, 0, NULL, ?)
ON CONFLICT(skill_id) DO NOTHING
`), skillID, ts)
	return err
}

func (s *SQLUsageStore) Get(skillID string) (Usage, bool) {
	row := s.db.QueryRow(s.q(`
SELECT skill_id, activate_count, last_activated_at, installed_at
FROM skill_usage WHERE skill_id = ?`), skillID)
	u, err := scanUsage(row)
	if err != nil {
		return Usage{}, false
	}
	return u, true
}

func (s *SQLUsageStore) List() []Usage {
	rows, err := s.db.Query(s.q(`
SELECT skill_id, activate_count, last_activated_at, installed_at FROM skill_usage`))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Usage
	for rows.Next() {
		u, err := scanUsage(rows)
		if err != nil {
			return out
		}
		out = append(out, u)
	}
	return out
}

func scanUsage(row interface {
	Scan(dest ...any) error
}) (Usage, error) {
	var (
		u       Usage
		last    sql.NullString
		install string
	)
	if err := row.Scan(&u.SkillID, &u.ActivateCount, &last, &install); err != nil {
		return Usage{}, err
	}
	if last.Valid && last.String != "" {
		if t, err := time.Parse(time.RFC3339Nano, last.String); err == nil {
			u.LastActivatedAt = t.UTC()
		} else if t, err := time.Parse(time.RFC3339, last.String); err == nil {
			u.LastActivatedAt = t.UTC()
		}
	}
	if t, err := time.Parse(time.RFC3339Nano, install); err == nil {
		u.InstalledAt = t.UTC()
	} else if t, err := time.Parse(time.RFC3339, install); err == nil {
		u.InstalledAt = t.UTC()
	}
	return u, nil
}
