package store

import (
	"fmt"
	"sort"
)

func (s *SQLStore) UpsertAgent(a Agent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.Skills != nil {
		a.Skills = append([]string(nil), a.Skills...)
	}
	s.agents[a.ID] = a
}

func (s *SQLStore) ListAgents() []Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.agents))
	for id := range s.agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Agent, 0, len(ids))
	for _, id := range ids {
		out = append(out, cloneAgent(s.agents[id]))
	}
	return out
}

func (s *SQLStore) GetAgent(id string) (Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.agents[id]
	if !ok {
		return Agent{}, fmt.Errorf("agent not found")
	}
	return cloneAgent(a), nil
}
