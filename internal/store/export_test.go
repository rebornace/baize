package store

import "time"

// PutModelProfilePlainForTest inserts a profile with a plaintext API key,
// bypassing seal/open. Only available in test binaries (export_test.go).
func (s *Memory) PutModelProfilePlainForTest(p ModelProfile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == "" {
		p.ID = "mp_migrate_test"
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = p.CreatedAt
	}
	s.modelProfiles[p.ID] = p
}
