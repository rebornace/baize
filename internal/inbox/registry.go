package inbox

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"sync"
)

// Registry holds inbound webhook channels for lookup at request time.
type Registry struct {
	mu       sync.RWMutex
	channels map[string]Channel
}

// NewRegistry returns an empty channel registry.
func NewRegistry() *Registry {
	return &Registry{
		channels: make(map[string]Channel),
	}
}

// Replace swaps the full channel set.
func (r *Registry) Replace(channels []Channel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := make(map[string]Channel, len(channels))
	for _, c := range channels {
		if c.ID == "" {
			continue
		}
		next[c.ID] = c
	}
	r.channels = next
}

// Get returns a channel when it is enabled and has a non-empty secret.
func (r *Registry) Get(id string) (Channel, bool) {
	c, ok := r.GetAny(id)
	if !ok || !c.Enabled || strings.TrimSpace(c.Secret) == "" {
		return Channel{}, false
	}
	return c, true
}

// GetAny returns a channel by id regardless of enabled/secret state.
func (r *Registry) GetAny(id string) (Channel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.channels[id]
	if !ok {
		return Channel{}, false
	}
	return c, true
}

// SecretHint returns the last four characters of a secret for display.
func SecretHint(secret string) string {
	if len(secret) <= 4 {
		return secret
	}
	return secret[len(secret)-4:]
}

// GenerateSecret returns a new 32-byte random secret encoded as Base64URL.
func GenerateSecret() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
