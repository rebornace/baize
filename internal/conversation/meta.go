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
}

// MetaFilter selects conversation meta rows. Empty OwnerID means no owner filter.
type MetaFilter struct {
	OwnerID string
}

// MetaStore persists conversation ownership metadata.
type MetaStore interface {
	EnsureMeta(m Meta) error
	GetMeta(id string) (Meta, bool)
	ListMeta(filter MetaFilter) ([]Meta, error)
}

// CanAccess reports whether principal may read/write the conversation meta.
// Admins always pass; operators only when OwnerID matches.
func CanAccess(p controlplane.Principal, m Meta) bool {
	if p.Role == controlplane.RoleAdmin {
		return true
	}
	if p.Role != controlplane.RoleOperator {
		return false
	}
	return m.OwnerID != "" && m.OwnerID == p.OperatorID
}
