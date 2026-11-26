package conversation

import (
	"time"

	"github.com/rebornace/baize/internal/controlplane"
)

// Meta is ownership metadata for a conversation.
type Meta struct {
	ID          string
	OwnerID     string
	Source      string
	Title       string
	UpdatedAt   time.Time
	ChannelPeer string
	WorkspaceID string
	// ArchivedAt is zero for active conversations. Archived conversations stay
	// readable but are hidden from the default sidebar list.
	ArchivedAt time.Time
	// Tags are operator-defined labels used for filtering; max 8.
	Tags []string
}

// MetaFilter selects conversation meta rows. Empty OwnerID means no owner filter.
// Empty WorkspaceID means no workspace filter.
type MetaFilter struct {
	OwnerID     string
	WorkspaceID string
}

// MetaStore persists conversation ownership metadata.
type MetaStore interface {
	EnsureMeta(m Meta) error
	// GetMeta returns the meta row, or ErrMetaNotFound if missing.
	// Other errors (scan/parse/DB) must not be collapsed into not-found.
	GetMeta(id string) (Meta, error)
	ListMeta(filter MetaFilter) ([]Meta, error)
	// DeleteMeta removes the meta row for id. Deleting a missing id is not an
	// error (idempotent); a DB/scan failure is returned.
	DeleteMeta(id string) error
	// UpdateMetaWorkspace moves a conversation to another workspace. A missing
	// meta row returns ErrMetaNotFound.
	UpdateMetaWorkspace(id, workspaceID string) error
	// UpdateMetaTitle sets a user-facing title. Empty clears it so the list
	// falls back to the first user message. A missing row returns ErrMetaNotFound.
	UpdateMetaTitle(id, title string) error
	// UpdateMetaArchived archives or unarchives a conversation. A missing row
	// returns ErrMetaNotFound.
	UpdateMetaArchived(id string, archived bool) error
	// UpdateMetaTags replaces the conversation's tags. A missing row returns
	// ErrMetaNotFound.
	UpdateMetaTags(id string, tags []string) error
}

// CanAccess reports whether principal may read/write the conversation meta.
// Admins always pass; operators only when OwnerID matches.
func CanAccess(p controlplane.Principal, m Meta) bool {
	if p.Role == controlplane.RoleAdmin || p.Role == controlplane.RoleViewer {
		return true
	}
	if p.Role != controlplane.RoleOperator {
		return false
	}
	return m.OwnerID != "" && m.OwnerID == p.OperatorID
}
