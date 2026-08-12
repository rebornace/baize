package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY, agent_id TEXT, input TEXT, status TEXT,
  output TEXT, error TEXT, created_at TEXT, hitl_json TEXT
);
CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT, type TEXT, timestamp TEXT, data_json TEXT
);
`

// SQLite is a file-backed Store implementation.
type SQLite struct {
	db         *sql.DB
	mu         sync.RWMutex
	agents     map[string]Agent
	connectors map[string]Connector
}

// OpenSQLite opens (or creates) a SQLite database at path.
func OpenSQLite(path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(sqliteSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return &SQLite{
		db:         db,
		agents:     map[string]Agent{},
		connectors: map[string]Connector{},
	}, nil
}

// Close closes the underlying database.
func (s *SQLite) Close() error {
	return s.db.Close()
}

func (s *SQLite) UpsertAgent(a Agent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[a.ID] = a
}

func (s *SQLite) GetAgent(id string) (Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.agents[id]
	if !ok {
		return Agent{}, fmt.Errorf("agent not found")
	}
	return a, nil
}

func (s *SQLite) UpsertConnector(c Connector) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connectors[c.ID] = c
}

func (s *SQLite) GetConnector(id string) (Connector, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.connectors[id]
	if !ok {
		return Connector{}, fmt.Errorf("connector not found")
	}
	return c, nil
}

func (s *SQLite) CreateRun(agentID, input string) (*Run, error) {
	id := "run_" + uuid.NewString()
	now := time.Now().UTC()
	r := &Run{
		ID:        id,
		AgentID:   agentID,
		Input:     input,
		Status:    StatusRunning,
		CreatedAt: now,
	}
	_, err := s.db.Exec(
		`INSERT INTO runs (id, agent_id, input, status, output, error, created_at, hitl_json)
		 VALUES (?, ?, ?, ?, '', '', ?, NULL)`,
		r.ID, r.AgentID, r.Input, string(r.Status), r.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *SQLite) GetRun(id string) (*Run, error) {
	var r Run
	var status, createdAt string
	err := s.db.QueryRow(
		`SELECT id, agent_id, input, status, output, error, created_at FROM runs WHERE id = ?`,
		id,
	).Scan(&r.ID, &r.AgentID, &r.Input, &status, &r.Output, &r.Error, &createdAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("run not found")
	}
	if err != nil {
		return nil, err
	}
	r.Status = Status(status)
	ts, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		ts, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
	}
	r.CreatedAt = ts
	return &r, nil
}

func (s *SQLite) UpdateRun(id string, status Status, output, errMsg string) error {
	res, err := s.db.Exec(
		`UPDATE runs SET status = ?, output = ?, error = ? WHERE id = ?`,
		string(status), output, errMsg, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("run not found")
	}
	return nil
}

func (s *SQLite) AppendEvent(runID string, ev Event) error {
	if _, err := s.GetRun(runID); err != nil {
		return err
	}
	ev.Timestamp = time.Now().UTC()
	dataJSON, err := json.Marshal(ev.Data)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO events (run_id, type, timestamp, data_json) VALUES (?, ?, ?, ?)`,
		runID, ev.Type, ev.Timestamp.Format(time.RFC3339Nano), string(dataJSON),
	)
	return err
}

func (s *SQLite) ListEvents(runID string) ([]Event, error) {
	if _, err := s.GetRun(runID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT type, timestamp, data_json FROM events WHERE run_id = ? ORDER BY id ASC`,
		runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var ev Event
		var ts, dataJSON string
		if err := rows.Scan(&ev.Type, &ts, &dataJSON); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, ts)
			if err != nil {
				return nil, fmt.Errorf("parse event timestamp: %w", err)
			}
		}
		ev.Timestamp = parsed
		if dataJSON != "" && dataJSON != "null" {
			if err := json.Unmarshal([]byte(dataJSON), &ev.Data); err != nil {
				return nil, err
			}
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Event{}
	}
	return out, nil
}

func (s *SQLite) SetHITL(runID string, payload *HITLPayload) error {
	if _, err := s.GetRun(runID); err != nil {
		return err
	}
	var hitlSQL sql.NullString
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		hitlSQL = sql.NullString{String: string(b), Valid: true}
	}
	_, err := s.db.Exec(`UPDATE runs SET hitl_json = ? WHERE id = ?`, hitlSQL, runID)
	return err
}

func (s *SQLite) GetHITL(runID string) (*HITLPayload, error) {
	var hitlSQL sql.NullString
	err := s.db.QueryRow(`SELECT hitl_json FROM runs WHERE id = ?`, runID).Scan(&hitlSQL)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("run not found")
	}
	if err != nil {
		return nil, err
	}
	if !hitlSQL.Valid || hitlSQL.String == "" || hitlSQL.String == "null" {
		return nil, nil
	}
	var p HITLPayload
	if err := json.Unmarshal([]byte(hitlSQL.String), &p); err != nil {
		return nil, err
	}
	return &p, nil
}
