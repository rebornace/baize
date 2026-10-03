package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/rebornace/baize/internal/blob"
	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/connector/specstore"
)

// loadOpenAPIRoutes loads tool routes from a blob key or a filesystem path.
// Legacy absolute paths under connectors/.../openapi.normalized.json are
// rejected with a clear error (no dual-read of the old on-disk layout).
func loadOpenAPIRoutes(ctx context.Context, blobs blob.Store, spec string) ([]openapi.ToolRoute, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("%w: empty spec", openapi.ErrInvalidSpec)
	}
	if specstore.IsBlobSpecKey(spec) {
		if blobs == nil {
			return nil, fmt.Errorf("%w: blob store is required for spec key %q", openapi.ErrInvalidSpec, spec)
		}
		data, err := blobs.Get(ctx, spec)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", openapi.ErrInvalidSpec, err)
		}
		return openapi.LoadToolsFromBytes(data)
	}
	if isLegacyConnectorFSPath(spec) {
		return nil, fmt.Errorf("%w: filesystem spec path %q is no longer supported; re-import the connector via the API", openapi.ErrInvalidSpec, spec)
	}
	routes, err := openapi.LoadTools(spec)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", openapi.ErrInvalidSpec, err)
	}
	return routes, nil
}

func isLegacyConnectorFSPath(spec string) bool {
	return specstore.IsLegacyConnectorFSPath(spec)
}
