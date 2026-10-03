package mcpoauth

import (
	"errors"
	"sync"
	"time"
)

const defaultSessionTTL = 10 * time.Minute

// ErrSessionNotFound is returned when Take is called with an unknown or already consumed state.
var ErrSessionNotFound = errors.New("mcp oauth: session not found")

// ErrSessionExpired is returned when Take is called after the session expiry.
var ErrSessionExpired = errors.New("mcp oauth: session expired")

// Pending holds in-flight OAuth authorization data keyed by state.
type Pending struct {
	ConnectorID string
	Verifier    string
	RedirectURI string
	Expires     time.Time
}

// SessionStore keeps pending OAuth sessions in memory (single-process runtime).
type SessionStore struct {
	ttl time.Duration
	now func() time.Time
	m   sync.Map
}

// NewSessionStore returns a store with the default session TTL.
func NewSessionStore() *SessionStore {
	return &SessionStore{
		ttl: defaultSessionTTL,
		now: time.Now,
	}
}

// Put stores a pending session. If Expires is zero, it is set to now + TTL.
func (s *SessionStore) Put(state string, p Pending) {
	if p.Expires.IsZero() {
		p.Expires = s.now().Add(s.ttl)
	}
	s.m.Store(state, p)
}

// Take removes and returns the pending session for state. Each state may be taken only once.
func (s *SessionStore) Take(state string) (Pending, error) {
	v, ok := s.m.LoadAndDelete(state)
	if !ok {
		return Pending{}, ErrSessionNotFound
	}
	p := v.(Pending)
	if !s.now().Before(p.Expires) {
		return Pending{}, ErrSessionExpired
	}
	return p, nil
}
