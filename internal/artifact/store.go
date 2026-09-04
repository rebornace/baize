package artifact

import "context"

// Store persists analysis-page HTML artifacts. Bytes live in a blob.Store;
// metadata (id -> run_id) lives in the SQL backend.
type Store interface {
	PutHTML(ctx context.Context, runID string, html string) (id string, err error)
	Get(ctx context.Context, id string) (html string, runID string, err error)
}
