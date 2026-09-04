// Package file is the zero-dependency local-disk blob driver. It materializes
// keys as files under a root directory, mirroring the on-disk layout used before
// blob abstraction (so upgrades need no migration).
package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rebornace/baize/internal/blob"
)

func init() {
	blob.RegisterDriver("file", func(_ context.Context, opts blob.Options) (blob.Store, error) {
		if opts.File.RootDir == "" {
			return nil, fmt.Errorf("blob file driver: file.root_dir is required")
		}
		return &store{root: opts.File.RootDir}, nil
	})
}

type store struct {
	root string
}

var _ blob.Store = (*store)(nil)

func (s *store) path(key string) string {
	return filepath.Join(s.root, filepath.FromSlash(key))
}

func (s *store) Put(_ context.Context, key string, data []byte, _ string) error {
	p := s.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", key, err)
	}
	return nil
}

func (s *store) Get(_ context.Context, key string) ([]byte, error) {
	b, err := os.ReadFile(s.path(key))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("get %s: %w", key, blob.ErrNotFound)
		}
		return nil, fmt.Errorf("read %s: %w", key, err)
	}
	return b, nil
}

func (s *store) Delete(_ context.Context, key string) error {
	err := os.Remove(s.path(key))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", key, err)
	}
	return nil
}
