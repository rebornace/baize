package store

import (
	"fmt"
	"sort"
	"time"
)

func (s *Memory) GetRunCost(runID string) (RunCost, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.runCosts[runID]
	return c, ok, nil
}

func (s *Memory) PutRunCost(c RunCost) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runCosts == nil {
		s.runCosts = map[string]RunCost{}
	}
	c.UpdatedAt = time.Now().UTC()
	s.runCosts[c.RunID] = c
	return nil
}

func (s *Memory) ListRunIDsMissingCostsSince(since time.Time, limit int) ([]string, error) {
	return s.listRunIDsMissingCosts(since, time.Now().UTC().Add(100*365*24*time.Hour), limit)
}

func (s *Memory) ListRunIDsMissingCostsBetween(from, to time.Time, limit int) ([]string, error) {
	return s.listRunIDsMissingCosts(from, to, limit)
}

func (s *Memory) listRunIDsMissingCosts(from, to time.Time, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 200
	}
	from, to = from.UTC(), to.UTC()
	s.mu.RLock()
	defer s.mu.RUnlock()
	type candidate struct {
		id string
		at time.Time
	}
	var items []candidate
	for id, r := range s.runs {
		if !terminalRunStatus(r.Status) || r.CreatedAt.Before(from) || r.CreatedAt.After(to) {
			continue
		}
		if _, ok := s.runCosts[id]; ok {
			continue
		}
		items = append(items, candidate{id: id, at: r.CreatedAt})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].at.Before(items[j].at) })
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.id)
	}
	return out, nil
}

func (s *Memory) SumRunCostsByScope(agentID, conversationID string, from, to time.Time) (float64, error) {
	from = from.UTC()
	to = to.UTC()
	s.mu.RLock()
	defer s.mu.RUnlock()
	var total float64
	for id, cost := range s.runCosts {
		r := s.runs[id]
		if r == nil {
			continue
		}
		if agentID != "" && r.AgentID != agentID {
			continue
		}
		if conversationID != "" && r.ConversationID != conversationID {
			continue
		}
		if r.CreatedAt.Before(from) || r.CreatedAt.After(to) {
			continue
		}
		total += cost.CostUSD
	}
	return total, nil
}

func (s *Memory) SumRunCostsGrouped(by string, from, to time.Time) (map[string]float64, error) {
	from = from.UTC()
	to = to.UTC()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]float64{}
	for id, cost := range s.runCosts {
		r := s.runs[id]
		if r == nil || r.CreatedAt.Before(from) || r.CreatedAt.After(to) {
			continue
		}
		var key string
		switch by {
		case "model":
			key = r.ModelProfileID
		case "agent":
			key = r.AgentID
		default:
			return nil, fmt.Errorf("unknown cost dimension %q", by)
		}
		out[key] += cost.CostUSD
	}
	return out, nil
}

func (s *SQLStore) GetRunCost(runID string) (RunCost, bool, error) {
	var (
		c       RunCost
		priced  int
		updated string
	)
	err := s.queryRow(`SELECT run_id, day, cost_usd, priced, updated_at FROM run_costs WHERE run_id = ?`, runID).
		Scan(&c.RunID, &c.Day, &c.CostUSD, &priced, &updated)
	if err != nil {
		return RunCost{}, false, nil
	}
	c.Priced = priced != 0
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return c, true, nil
}

func (s *SQLStore) PutRunCost(c RunCost) error {
	priced := 0
	if c.Priced {
		priced = 1
	}
	now := time.Now().UTC()
	_, err := s.exec(`INSERT INTO run_costs (run_id, day, cost_usd, priced, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (run_id) DO UPDATE SET
		  day = excluded.day, cost_usd = excluded.cost_usd,
		  priced = excluded.priced, updated_at = excluded.updated_at`,
		c.RunID, c.Day, c.CostUSD, priced, now.Format(time.RFC3339Nano))
	return err
}

func (s *SQLStore) ListRunIDsMissingCostsSince(since time.Time, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.query(`SELECT r.id FROM runs r
		LEFT JOIN run_costs c ON c.run_id = r.id
		WHERE r.status IN ('succeeded','failed','cancelled','rejected')
		  AND r.created_at >= ? AND c.run_id IS NULL
		ORDER BY r.created_at ASC LIMIT ?`,
		formatSQLiteTime(since), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *SQLStore) ListRunIDsMissingCostsBetween(from, to time.Time, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.query(`SELECT r.id FROM runs r
		LEFT JOIN run_costs c ON c.run_id = r.id
		WHERE r.status IN ('succeeded','failed','cancelled','rejected')
		  AND r.created_at >= ? AND r.created_at <= ? AND c.run_id IS NULL
		ORDER BY r.created_at ASC LIMIT ?`,
		formatSQLiteTime(from), formatSQLiteTime(to), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *SQLStore) SumRunCostsByScope(agentID, conversationID string, from, to time.Time) (float64, error) {
	q := `SELECT COALESCE(SUM(rc.cost_usd), 0) FROM run_costs rc
		JOIN runs r ON r.id = rc.run_id
		WHERE r.created_at >= ? AND r.created_at <= ?`
	args := []any{formatSQLiteTime(from), formatSQLiteTime(to)}
	if agentID != "" {
		q += ` AND r.agent_id = ?`
		args = append(args, agentID)
	}
	if conversationID != "" {
		q += ` AND r.conversation_id = ?`
		args = append(args, conversationID)
	}
	var total float64
	if err := s.queryRow(q, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (s *SQLStore) SumRunCostsGrouped(by string, from, to time.Time) (map[string]float64, error) {
	var col string
	switch by {
	case "model":
		col = "r.model_profile_id"
	case "agent":
		col = "r.agent_id"
	default:
		return nil, fmt.Errorf("unknown cost dimension %q", by)
	}
	rows, err := s.query(`SELECT COALESCE(`+col+`, ''), COALESCE(SUM(rc.cost_usd), 0)
		FROM run_costs rc JOIN runs r ON r.id = rc.run_id
		WHERE r.created_at >= ? AND r.created_at <= ?
		GROUP BY 1`,
		formatSQLiteTime(from), formatSQLiteTime(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var key string
		var cost float64
		if err := rows.Scan(&key, &cost); err != nil {
			return nil, err
		}
		out[key] = cost
	}
	return out, rows.Err()
}

func (s *Memory) ListCostDays(fromDay, toDay string) ([]CostDay, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]CostDay, 0, len(s.costDays))
	for day, d := range s.costDays {
		if day >= fromDay && day <= toDay {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	return out, nil
}

func (s *Memory) PutCostDay(d CostDay) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.costDays == nil {
		s.costDays = map[string]CostDay{}
	}
	d.UpdatedAt = time.Now().UTC()
	s.costDays[d.Day] = d
	return nil
}

func (s *SQLStore) ListCostDays(fromDay, toDay string) ([]CostDay, error) {
	rows, err := s.query(
		`SELECT day, cost_usd, runs, finalized, updated_at FROM cost_days
		 WHERE day >= ? AND day <= ? ORDER BY day`, fromDay, toDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CostDay
	for rows.Next() {
		var (
			d         CostDay
			finalized int
			updated   string
		)
		if err := rows.Scan(&d.Day, &d.CostUSD, &d.Runs, &finalized, &updated); err != nil {
			return nil, err
		}
		d.Finalized = finalized != 0
		d.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *SQLStore) PutCostDay(d CostDay) error {
	finalized := 0
	if d.Finalized {
		finalized = 1
	}
	now := time.Now().UTC()
	_, err := s.exec(`INSERT INTO cost_days (day, cost_usd, runs, finalized, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (day) DO UPDATE SET
		  cost_usd = excluded.cost_usd, runs = excluded.runs,
		  finalized = excluded.finalized, updated_at = excluded.updated_at`,
		d.Day, d.CostUSD, d.Runs, finalized, now.Format(time.RFC3339Nano))
	return err
}

func (s *Memory) ClearCostCache() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runCosts = nil
	s.costDays = nil
	return nil
}

func (s *SQLStore) ClearCostCache() error {
	if _, err := s.exec(`DELETE FROM run_costs`); err != nil {
		return err
	}
	_, err := s.exec(`DELETE FROM cost_days`)
	return err
}
