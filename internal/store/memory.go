package store

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Memory is an in-memory Store implementation.
type Memory struct {
	mu              sync.RWMutex
	agents          map[string]Agent
	connectors      map[string]Connector
	tools           map[string]Tool
	runs            map[string]*Run
	leaseUntil      map[string]time.Time
	events          map[string][]Event
	hitl            map[string]*HITLPayload
	settings        map[string][]byte
	inboxDeliveries map[string]map[string]InboxDelivery
	inboxThreads    map[string]map[string]string
	webhookOutbox   map[string]WebhookOutboxEntry
	mcpExportIDs    map[string]MCPExportIdentity
	mcpExportKeys   map[string]MCPExportKey
	modelProfiles   map[string]ModelProfile
}

// NewMemory creates an empty in-memory Store.
func NewMemory() *Memory {
	return &Memory{
		agents:          map[string]Agent{},
		connectors:      map[string]Connector{},
		tools:           map[string]Tool{},
		runs:            map[string]*Run{},
		leaseUntil:      map[string]time.Time{},
		events:          map[string][]Event{},
		hitl:            map[string]*HITLPayload{},
		settings:        map[string][]byte{},
		inboxDeliveries: map[string]map[string]InboxDelivery{},
		inboxThreads:    map[string]map[string]string{},
		webhookOutbox:   map[string]WebhookOutboxEntry{},
		mcpExportIDs:    map[string]MCPExportIdentity{},
		mcpExportKeys:   map[string]MCPExportKey{},
		modelProfiles:   map[string]ModelProfile{},
	}
}

// New is an alias for NewMemory.
func New() *Memory {
	return NewMemory()
}

func (s *Memory) UpsertAgent(a Agent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.Skills != nil {
		a.Skills = append([]string(nil), a.Skills...)
	}
	s.agents[a.ID] = a
}

func (s *Memory) ListAgents() []Agent {
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

func (s *Memory) GetAgent(id string) (Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.agents[id]
	if !ok {
		return Agent{}, fmt.Errorf("agent not found")
	}
	return cloneAgent(a), nil
}

func (s *Memory) UpsertConnector(c Connector) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connectors[c.ID] = c
}

func (s *Memory) GetConnector(id string) (Connector, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.connectors[id]
	if !ok {
		return Connector{}, fmt.Errorf("connector not found")
	}
	return c, nil
}

func (s *Memory) DeleteConnector(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.connectors[id]; !ok {
		return fmt.Errorf("connector not found")
	}
	delete(s.connectors, id)
	for name, t := range s.tools {
		if t.ConnectorID == id {
			delete(s.tools, name)
		}
	}
	return nil
}

func (s *Memory) ListConnectors() []Connector {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.connectors))
	for id := range s.connectors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Connector, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.connectors[id])
	}
	return out
}

func (s *Memory) UpsertTool(t Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[t.Name] = t
}

func (s *Memory) GetTool(name string) (Tool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tools[name]
	if !ok {
		return Tool{}, fmt.Errorf("tool not found")
	}
	return t, nil
}

func (s *Memory) ListTools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.tools))
	for n := range s.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Tool, 0, len(names))
	for _, n := range names {
		out = append(out, s.tools[n])
	}
	return out
}

func (s *Memory) ListToolsByConnector(id string) []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.tools))
	for n, t := range s.tools {
		if t.ConnectorID == id {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	out := make([]Tool, 0, len(names))
	for _, n := range names {
		out = append(out, s.tools[n])
	}
	return out
}

func (s *Memory) DeleteTool(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tools[name]; !ok {
		return fmt.Errorf("tool not found")
	}
	delete(s.tools, name)
	return nil
}

func (s *Memory) ReplaceConnectorTools(connectorID string, tools []Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, t := range s.tools {
		if t.ConnectorID == connectorID {
			delete(s.tools, name)
		}
	}
	for _, t := range tools {
		if t.ConnectorID == "" {
			t.ConnectorID = connectorID
		}
		s.tools[t.Name] = t
	}
}

func (s *Memory) CreateRun(in CreateRunInput) (*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := "run_" + uuid.NewString()
	r := &Run{
		ID:                 id,
		AgentID:            in.AgentID,
		Input:              in.Input,
		Status:             StatusRunning,
		CreatedAt:          time.Now().UTC(),
		ConversationID:     in.ConversationID,
		IdentityID:         in.IdentityID,
		ModelProfileID:     in.ModelProfileID,
		PassthroughHeaders: cloneHeaders(in.PassthroughHeaders),
		WebhookConfig:      cloneWebhookConfig(in.WebhookConfig),
	}
	s.runs[id] = r
	s.events[id] = nil
	return r, nil
}

func (s *Memory) GetRun(id string) (*Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.runs[id]
	if !ok {
		return nil, fmt.Errorf("run not found")
	}
	cp := *r
	cp.PassthroughHeaders = cloneHeaders(r.PassthroughHeaders)
	cp.WebhookConfig = cloneWebhookConfig(r.WebhookConfig)
	if until, held := s.leaseUntil[id]; held {
		lease := until
		cp.LeaseUntil = &lease
	}
	return &cp, nil
}

func (s *Memory) SetPassthroughHeaders(id string, headers map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return fmt.Errorf("run not found")
	}
	r.PassthroughHeaders = cloneHeaders(headers)
	return nil
}

// cloneHeaders returns a shallow copy of h so callers cannot mutate stored
// state through the input map. Returns nil for nil input.
func cloneHeaders(h map[string]string) map[string]string {
	if h == nil {
		return nil
	}
	cp := make(map[string]string, len(h))
	for k, v := range h {
		cp[k] = v
	}
	return cp
}

func cloneWebhookConfig(w *WebhookConfig) *WebhookConfig {
	if w == nil {
		return nil
	}
	cp := &WebhookConfig{URL: w.URL}
	if w.Headers != nil {
		cp.Headers = make(map[string]string, len(w.Headers))
		for k, v := range w.Headers {
			cp.Headers[k] = v
		}
	}
	return cp
}

func (s *Memory) GetSetting(key string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, ok := s.settings[key]
	if !ok {
		return nil, false, nil
	}
	cp := make([]byte, len(raw))
	copy(cp, raw)
	return cp, true, nil
}

func (s *Memory) UpsertSetting(key string, jsonRaw []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(jsonRaw))
	copy(cp, jsonRaw)
	s.settings[key] = cp
	return nil
}

func (s *Memory) UpdateRun(id string, status Status, output, errMsg string) error {
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

func (s *Memory) LeaseRun(id string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return false, fmt.Errorf("run not found")
	}
	if r.Status != StatusQueued && r.Status != StatusRunning {
		return false, nil
	}
	now := time.Now().UTC()
	if until, held := s.leaseUntil[id]; held && until.After(now) {
		return false, nil
	}
	s.leaseUntil[id] = now.Add(ttl)
	return true, nil
}

func (s *Memory) HeartbeatRun(id string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[id]; !ok {
		return false, nil
	}
	s.leaseUntil[id] = time.Now().UTC().Add(ttl)
	return true, nil
}

func (s *Memory) ClearRunLease(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.leaseUntil, id)
	return nil
}

func (s *Memory) ListRunsForReconcile(limit int) ([]*Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().UTC()
	grace := now.Add(-reconcileGrace)
	var out []*Run
	for _, r := range s.runs {
		if r.Status != StatusQueued && r.Status != StatusRunning {
			continue
		}
		until, held := s.leaseUntil[r.ID]
		expired := held && !until.After(now)
		neverLeasedOld := !held && r.CreatedAt.Before(grace)
		if expired || neverLeasedOld {
			cp := *r
			cp.PassthroughHeaders = cloneHeaders(r.PassthroughHeaders)
			cp.WebhookConfig = cloneWebhookConfig(r.WebhookConfig)
			if held {
				lease := until
				cp.LeaseUntil = &lease
			}
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Memory) AppendEvent(runID string, ev Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[runID]; !ok {
		return fmt.Errorf("run not found")
	}
	ev.Timestamp = time.Now().UTC()
	s.events[runID] = append(s.events[runID], ev)
	return nil
}

func (s *Memory) ListEvents(runID string) ([]Event, error) {
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

func (s *Memory) SetHITL(runID string, payload *HITLPayload) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[runID]; !ok {
		return fmt.Errorf("run not found")
	}
	if payload == nil {
		delete(s.hitl, runID)
		return nil
	}
	cp := *payload
	if payload.Arguments != nil {
		cp.Arguments = make(map[string]any, len(payload.Arguments))
		for k, v := range payload.Arguments {
			cp.Arguments[k] = v
		}
	}
	s.hitl[runID] = &cp
	return nil
}

func (s *Memory) GetHITL(runID string) (*HITLPayload, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.runs[runID]; !ok {
		return nil, fmt.Errorf("run not found")
	}
	p, ok := s.hitl[runID]
	if !ok || p == nil {
		return nil, nil
	}
	cp := *p
	if p.Arguments != nil {
		cp.Arguments = make(map[string]any, len(p.Arguments))
		for k, v := range p.Arguments {
			cp.Arguments[k] = v
		}
	}
	return &cp, nil
}

func (s *Memory) HasActiveRun(conversationID string) (bool, error) {
	if conversationID == "" {
		return false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.runs {
		if r.ConversationID != conversationID {
			continue
		}
		switch r.Status {
		case StatusQueued, StatusRunning, StatusWaitingHuman:
			return true, nil
		}
	}
	return false, nil
}

func (s *Memory) WaitingHumanRun(conversationID string) (*Run, error) {
	if conversationID == "" {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var best *Run
	for _, r := range s.runs {
		if r.ConversationID != conversationID || r.Status != StatusWaitingHuman {
			continue
		}
		if best == nil || r.CreatedAt.After(best.CreatedAt) {
			cp := *r
			best = &cp
		}
	}
	return best, nil
}

func (s *Memory) GetInboxDelivery(channelID, idempotencyKey string) (InboxDelivery, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byKey, ok := s.inboxDeliveries[channelID]
	if !ok {
		return InboxDelivery{}, false, nil
	}
	d, ok := byKey[idempotencyKey]
	if !ok {
		return InboxDelivery{}, false, nil
	}
	// Rows older than InboxDeliveryTTL are treated as misses so the same key
	// may be claimed again after the window.
	if !InboxDeliveryFresh(d, time.Now()) {
		return InboxDelivery{}, false, nil
	}
	return d, true, nil
}

func (s *Memory) PutInboxDelivery(d InboxDelivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	byKey, ok := s.inboxDeliveries[d.ChannelID]
	if !ok {
		byKey = map[string]InboxDelivery{}
		s.inboxDeliveries[d.ChannelID] = byKey
	}
	if existing, exists := byKey[d.IdempotencyKey]; exists {
		// Allow overwrite of expired rows; reject fresh duplicates.
		if InboxDeliveryFresh(existing, time.Now()) {
			return ErrInboxDeliveryExists
		}
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now().UTC()
	}
	byKey[d.IdempotencyKey] = d
	return nil
}

func (s *Memory) UpdateInboxDelivery(channelID, idempotencyKey, runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	byKey, ok := s.inboxDeliveries[channelID]
	if !ok {
		return fmt.Errorf("inbox delivery not found")
	}
	d, ok := byKey[idempotencyKey]
	if !ok || !InboxDeliveryFresh(d, time.Now()) {
		return fmt.Errorf("inbox delivery not found")
	}
	d.RunID = runID
	byKey[idempotencyKey] = d
	return nil
}

func (s *Memory) GetInboxThread(channelID, externalID string) (string, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byExt, ok := s.inboxThreads[channelID]
	if !ok {
		return "", false, nil
	}
	conv, ok := byExt[externalID]
	if !ok {
		return "", false, nil
	}
	return conv, true, nil
}

func (s *Memory) PutInboxThread(channelID, externalID, conversationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	byExt, ok := s.inboxThreads[channelID]
	if !ok {
		byExt = map[string]string{}
		s.inboxThreads[channelID] = byExt
	}
	byExt[externalID] = conversationID
	return nil
}
