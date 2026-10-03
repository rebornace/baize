package conversation

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	DefaultWorkspaceID   = "default"
	DefaultWorkspaceName = "Default"
	maxWorkspaceNameLen  = 40
)

// Workspace groups Web UI conversations that share business-system logins.
type Workspace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// WorkspaceStore persists chat workspaces.
type WorkspaceStore interface {
	EnsureDefaultWorkspace() error
	ListWorkspaces() ([]Workspace, error)
	GetWorkspace(id string) (Workspace, error)
	CreateWorkspace(name string) (Workspace, error)
}

// NormalizeWorkspaceID returns DefaultWorkspaceID when id is empty.
func NormalizeWorkspaceID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return DefaultWorkspaceID
	}
	return id
}

func validateWorkspaceName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("workspace name is required")
	}
	if utf8.RuneCountInString(name) > maxWorkspaceNameLen {
		return "", fmt.Errorf("workspace name is too long")
	}
	return name, nil
}

func newWorkspaceID() string {
	return "ws_" + uuid.NewString()
}

func defaultWorkspace(now time.Time) Workspace {
	return Workspace{ID: DefaultWorkspaceID, Name: DefaultWorkspaceName, CreatedAt: now}
}

var _ WorkspaceStore = (*MemoryStore)(nil)

func (s *MemoryStore) EnsureDefaultWorkspace() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.workspaces == nil {
		s.workspaces = map[string]Workspace{}
	}
	if _, ok := s.workspaces[DefaultWorkspaceID]; !ok {
		s.workspaces[DefaultWorkspaceID] = defaultWorkspace(time.Now().UTC())
	}
	return nil
}

func (s *MemoryStore) ListWorkspaces() ([]Workspace, error) {
	if err := s.EnsureDefaultWorkspace(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Workspace, 0, len(s.workspaces))
	for _, w := range s.workspaces {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID == DefaultWorkspaceID {
			return true
		}
		if out[j].ID == DefaultWorkspaceID {
			return false
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *MemoryStore) GetWorkspace(id string) (Workspace, error) {
	id = NormalizeWorkspaceID(id)
	if err := s.EnsureDefaultWorkspace(); err != nil {
		return Workspace{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.workspaces[id]
	if !ok {
		return Workspace{}, fmt.Errorf("workspace not found")
	}
	return w, nil
}

func (s *MemoryStore) CreateWorkspace(name string) (Workspace, error) {
	name, err := validateWorkspaceName(name)
	if err != nil {
		return Workspace{}, err
	}
	if err := s.EnsureDefaultWorkspace(); err != nil {
		return Workspace{}, err
	}
	w := Workspace{ID: newWorkspaceID(), Name: name, CreatedAt: time.Now().UTC()}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspaces[w.ID] = w
	return w, nil
}
