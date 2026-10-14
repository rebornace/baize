package hostedlogin

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/store"
)

const DefaultSessionTTL = 15 * time.Minute

// Session is a short-lived hosted-login ticket.
// Exactly one target must be set: IdentityID (MCP export identity) or
// ConversationID (Chat conversation identity store).
type Session struct {
	ID               string
	CSRF             string
	MethodID         string
	ExportKeyID      string
	ConnectorID      string
	LoginTool        string
	Mode             string
	IdentityID       string
	ConversationID   string
	// WorkspaceID is the Web UI workspace for ConversationID targets.
	// Persist must use identity.Key(ConversationID, WorkspaceID) so Accounts
	// and the run engine read the same pool (non-default workspaces).
	WorkspaceID      string
	Fields           []store.HostedLoginField
	OAuth            *store.HostedLoginOAuth
	AvailableMethods []store.HostedLoginMethod
	PublicOrigin     string
	ExpiresAt        time.Time
	// PendingTool / PendingArgs optionally describe an MCP tools/call to replay
	// after successful login (set by export auth-expired path).
	PendingTool string
	PendingArgs map[string]any
}

// Sessions issues and looks up hosted-login sessions.
type Sessions interface {
	Issue(seed Session) (id, csrf string, err error)
	Get(id string) (Session, bool)
	Replace(id string, sess Session) bool
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

func (s *memorySessions) Replace(id string, sess Session) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.purgeExpiredLocked(now)
	old, ok := s.byID[id]
	if !ok || !old.ExpiresAt.After(now) {
		return false
	}
	sess.ID = old.ID
	sess.CSRF = old.CSRF
	sess.ExpiresAt = old.ExpiresAt
	s.byID[id] = sess
	return true
}

func (s *memorySessions) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, id)
}

// ResolveMethod finds a hosted-login method by id within cfg.Methods.
func ResolveMethod(cfg store.HostedLoginConfig, methodID string) (store.HostedLoginMethod, error) {
	id := strings.TrimSpace(methodID)
	if id == "" {
		return store.HostedLoginMethod{}, fmt.Errorf("hostedlogin: method_id required")
	}
	for _, m := range cfg.Methods {
		if m.ID == id {
			return m, nil
		}
	}
	return store.HostedLoginMethod{}, fmt.Errorf("hostedlogin: unknown method_id %q", id)
}

// MaterializeSession copies method recipe fields onto seed and records MethodID.
func MaterializeSession(cfg store.HostedLoginConfig, methodID string, seed Session) (Session, error) {
	method, err := ResolveMethod(cfg, methodID)
	if err != nil {
		return Session{}, err
	}
	if err := ValidateMethod(method); err != nil {
		return Session{}, err
	}
	out := seed
	out.MethodID = method.ID
	out.Mode = method.Mode
	out.LoginTool = method.LoginTool
	if method.Fields != nil {
		out.Fields = append([]store.HostedLoginField(nil), method.Fields...)
	} else {
		out.Fields = nil
	}
	out.OAuth = CloneOAuth(method.OAuth)
	if len(cfg.Methods) > 0 {
		out.AvailableMethods = append([]store.HostedLoginMethod(nil), cfg.Methods...)
	}
	return out, nil
}

// SwitchMethodDeps updates the active method on an existing session.
type SwitchMethodDeps struct {
	Sessions Sessions
	Store    store.Store
}

// SwitchMethod changes the hosted session to another configured login method.
func SwitchMethod(deps SwitchMethodDeps, sessionID, csrf, methodID string) error {
	if deps.Sessions == nil || deps.Store == nil {
		return fmt.Errorf("hostedlogin: incomplete deps")
	}
	sess, ok := deps.Sessions.Get(sessionID)
	if !ok {
		return fmt.Errorf("hostedlogin: session not found or expired")
	}
	if strings.TrimSpace(csrf) == "" || csrf != sess.CSRF {
		return fmt.Errorf("hostedlogin: invalid csrf")
	}
	conn, err := deps.Store.GetConnector(sess.ConnectorID)
	if err != nil {
		return fmt.Errorf("hostedlogin: connector: %w", err)
	}
	updated, err := MaterializeSession(conn.Auth.HostedLogin, methodID, sess)
	if err != nil {
		return err
	}
	if !deps.Sessions.Replace(sessionID, updated) {
		return fmt.Errorf("hostedlogin: session not found or expired")
	}
	return nil
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
