// Package memory is an in-process blob driver, primarily for tests and future
// in-process deployments.
package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/rebornace/baize/internal/blob"
)

func init() {
	blob.RegisterDriver("memory", func(_ context.Context, _ blob.Options) (blob.Store, error) {
		return &store{m: map[string][]byte{}}, nil
	})
}

type store struct {
	mu sync.RWMutex
	m  map[string][]byte
}

var _ blob.Store = (*store)(nil)

func (s *store) Put(_ context.Context, key string, data []byte, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	s.m[key] = cp
	return nil
}

func (s *store) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.m[key]
	if !ok {
		return nil, fmt.Errorf("get %s: %w", key, blob.ErrNotFound)
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out, nil
}

func (s *store) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

func (s *store) List(_ context.Context, prefix string) ([]blob.ListEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]blob.ListEntry, 0)
	for k, v := range s.m {
		if strings.HasPrefix(k, prefix) {
			out = append(out, blob.ListEntry{Key: k, Size: int64(len(v))})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}
