package store

import (
	"sort"
	"time"
)

func cloneEvalCase(c EvalCase) EvalCase {
	out := c
	if c.EventsJSON != nil {
		out.EventsJSON = append([]byte(nil), c.EventsJSON...)
	}
	if c.RequiredReads != nil {
		out.RequiredReads = append([]string(nil), c.RequiredReads...)
	}
	if c.Consequential != nil {
		out.Consequential = append([]string(nil), c.Consequential...)
	}
	if c.ExpectConseqAny != nil {
		out.ExpectConseqAny = append([]string(nil), c.ExpectConseqAny...)
	}
	if c.Chain != nil {
		out.Chain = append([]EvalChainLink(nil), c.Chain...)
	}
	if c.LastScoreCodes != nil {
		out.LastScoreCodes = append([]string(nil), c.LastScoreCodes...)
	}
	if c.LastScorePass != nil {
		v := *c.LastScorePass
		out.LastScorePass = &v
	}
	if c.LastScoredAt != nil {
		t := *c.LastScoredAt
		out.LastScoredAt = &t
	}
	return out
}

func (s *Memory) ListRunsCreatedBetween(from, to time.Time, status Status, limit int) ([]*Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit = normalizeEvalListLimit(limit)
	from = from.UTC()
	to = to.UTC()
	var out []*Run
	for _, r := range s.runs {
		created := r.CreatedAt.UTC()
		if created.Before(from) || created.After(to) {
			continue
		}
		if status != "" && r.Status != status {
			continue
		}
		cp := *r
		cp.PassthroughHeaders = cloneHeaders(r.PassthroughHeaders)
		cp.WebhookConfig = cloneWebhookConfig(r.WebhookConfig)
		if until, held := s.leaseUntil[r.ID]; held {
			lease := until
			cp.LeaseUntil = &lease
		}
		out = append(out, &cp)
	}
	sortRunsByCreatedAtDesc(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func sortRunsByCreatedAtDesc(runs []*Run) {
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].CreatedAt.After(runs[j].CreatedAt)
	})
}

func (s *Memory) UpsertEvalCase(c EvalCase) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.evalCases == nil {
		s.evalCases = map[string]EvalCase{}
	}
	s.evalCases[c.ID] = cloneEvalCase(c)
	return nil
}

func (s *Memory) GetEvalCase(id string) (EvalCase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.evalCases[id]
	if !ok {
		return EvalCase{}, ErrEvalCaseNotFound
	}
	return cloneEvalCase(c), nil
}

func (s *Memory) ListEvalCases(limit int) ([]EvalCase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit = normalizeEvalListLimit(limit)
	ids := make([]string, 0, len(s.evalCases))
	for id := range s.evalCases {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return s.evalCases[ids[i]].CreatedAt.After(s.evalCases[ids[j]].CreatedAt)
	})
	if len(ids) > limit {
		ids = ids[:limit]
	}
	out := make([]EvalCase, 0, len(ids))
	for _, id := range ids {
		out = append(out, cloneEvalCase(s.evalCases[id]))
	}
	return out, nil
}

func (s *Memory) DeleteEvalCase(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.evalCases[id]; !ok {
		return ErrEvalCaseNotFound
	}
	delete(s.evalCases, id)
	return nil
}

func (s *Memory) FindEvalCaseBySourceRun(runID string) (EvalCase, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.evalCases {
		if c.SourceRunID == runID {
			return cloneEvalCase(c), true, nil
		}
	}
	return EvalCase{}, false, nil
}
