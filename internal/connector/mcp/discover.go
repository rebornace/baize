package mcp

import (
	"context"
	"net/http"

	"github.com/rebornace/baize/internal/store"
)

func DiscoverToolsHTTP(ctx context.Context, endpoint string, headers map[string]string, connectorID string, hc *http.Client) ([]store.Tool, error) {
	session, err := ConnectHTTP(ctx, endpoint, headers, hc)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return DiscoverTools(ctx, session, connectorID)
}
