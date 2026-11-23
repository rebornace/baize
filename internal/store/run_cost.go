package store

import (
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
