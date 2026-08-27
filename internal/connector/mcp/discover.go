package mcp

import (
	"context"

	"github.com/rebornace/baize/internal/store"
)

func DiscoverToolsHTTP(ctx context.Context, endpoint string, headers map[string]string, connectorID string) ([]store.Tool, error) {
	session, err := ConnectHTTP(ctx, endpoint, headers)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return DiscoverTools(ctx, session, connectorID)
}
