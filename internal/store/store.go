package store

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

type Agent struct {
	ID     string `json:"id"`
	System string `json:"system"`
}

type Connector struct {
	ID      string `json:"id"`
	Type    string `json:"type"` // openapi
	Spec    string `json:"spec"`
	BaseURL string `json:"base_url"`
}

type Event struct {
	Type      string         `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Data      map[string]any `json:"data,omitempty"`
}

type Run struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	Input     string    `json:"input"`
	Status    Status    `json:"status"`
	Output    string    `json:"output,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	mu         sync.RWMutex
	agents     map[string]Agent
	connectors map[string]Connector
	runs       map[string]*Run
	events     map[string][]Event
}

func New() *Store {
	return &Store{
		agents:     map[string]Agent{},
		connectors: map[string]Connector{},
		runs:       map[string]*Run{},
		events:     map[string][]Event{},
	}
}

func (s *Store) UpsertAgent(a Agent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[a.ID] = a
}

func (s *Store) GetAgent(id string) (Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.agents[id]
	if !ok {
		return Agent{}, fmt.Errorf("agent not found")
	}
	return a, nil
}

func (s *Store) UpsertConnector(c Connector) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connectors[c.ID] = c
}

func (s *Store) GetConnector(id string) (Connector, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.connectors[id]
	if !ok {
		return Connector{}, fmt.Errorf("connector not found")
	}
	return c, nil
}

func (s *Store) CreateRun(agentID, input string) (*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := "run_" + uuid.NewString()
	r := &Run{ID: id, AgentID: agentID, Input: input, Status: StatusRunning, CreatedAt: time.Now().UTC()}
	s.runs[id] = r
	s.events[id] = nil
	return r, nil
}

func (s *Store) GetRun(id string) (*Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.runs[id]
	if !ok {
		return nil, fmt.Errorf("run not found")
	}
	cp := *r
	return &cp, nil
}

func (s *Store) UpdateRun(id string, status Status, output, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return fmt.Errorf("run not found")
	}
	r.Status = status
	r.Output = output
	r.Error = errMsg
	return nil
}

func (s *Store) AppendEvent(runID string, ev Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[runID]; !ok {
		return fmt.Errorf("run not found")
	}
	ev.Timestamp = time.Now().UTC()
	s.events[runID] = append(s.events[runID], ev)
	return nil
}

func (s *Store) ListEvents(runID string) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.runs[runID]; !ok {
		return nil, fmt.Errorf("run not found")
	}
	evs := s.events[runID]
	cp := make([]Event, len(evs))
	copy(cp, evs)
	return cp, nil
}
