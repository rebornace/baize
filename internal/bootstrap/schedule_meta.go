package bootstrap

import (
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/schedule"
)

type scheduleMetaAdapter struct {
	conversation.MetaStore
}

func (a scheduleMetaAdapter) GetMeta(conversationID string) (string, error) {
	m, err := a.MetaStore.GetMeta(conversationID)
	if err != nil {
		return "", err
	}
	return m.OwnerID, nil
}

func scheduleMetaReader(meta conversation.MetaStore) schedule.MetaReader {
	if meta == nil {
		return nil
	}
	return scheduleMetaAdapter{MetaStore: meta}
}
