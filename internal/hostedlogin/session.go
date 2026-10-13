package hostedlogin

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/store"
)

const DefaultSessionTTL = 15 * time.Minute

// Session is a short-lived hosted-login ticket.
// Exactly one target must be set: IdentityID (MCP export identity) or
// ConversationID (Chat conversation identity store).
type Session struct {
	ID             string
	CSRF           string
	ExportKeyID    string
	ConnectorID    string
	LoginTool      string
	Mode           string
	IdentityID     string
	ConversationID string
	Fields         []store.HostedLoginField
	PublicOrigin   string
	ExpiresAt      time.Time
}

// Sessions issues and looks up hosted-login sessions.
type Sessions interface {
	Issue(seed Session) (id, csrf string, err error)
	Get(id string) (Session, bool)
	Delete(id string)
}

type memorySessions struct {
	ttl  time.Duration
	mu   sync.Mutex
	byID map[string]Session
	now  func() time.Time
}

// NewMemorySessions returns an in-process session store with the given TTL.
// Non-positive ttl defaults to DefaultSessionTTL (15m).
func NewMemorySessions(ttl time.Duration) Sessions {
	if ttl <= 0 {
		ttl = DefaultSessionTTL
	}
	return &memorySessions{
		ttl:  ttl,
		byID: make(map[string]Session),
		now:  time.Now,
	}
}

func (s *memorySessions) Issue(seed Session) (id, csrf string, err error) {
	id, err = randomHex(16)
	if err != nil {
		return "", "", err
	}
	csrf, err = randomHex(16)
	if err != nil {
		return "", "", err
	}
	now := s.now()
	sess := seed
	sess.ID = id
	sess.CSRF = csrf
	sess.ExpiresAt = now.Add(s.ttl)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked(now)
	s.byID[id] = sess
	return id, csrf, nil
}

func (s *memorySessions) Get(id string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.purgeExpiredLocked(now)
	sess, ok := s.byID[id]
	if !ok {
		return Session{}, false
	}
	if !sess.ExpiresAt.After(now) {
		delete(s.byID, id)
		return Session{}, false
	}
	return sess, true
}

func (s *memorySessions) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, id)
}

func (s *memorySessions) purgeExpiredLocked(now time.Time) {
	for id, sess := range s.byID {
		if !sess.ExpiresAt.After(now) {
			delete(s.byID, id)
		}
	}
}

func randomHex(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("hostedlogin: random: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
