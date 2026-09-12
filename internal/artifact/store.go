package artifact

import (
	"context"
	"errors"
)

// ErrNotFound is returned by Store.Get when no artifact exists for the id
// (no metadata row, or the underlying blob object is missing).
var ErrNotFound = errors.New("artifact: not found")

// Store persists analysis-page HTML artifacts. Bytes live in a blob.Store;
// metadata (id -> run_id) lives in the SQL backend.
type Store interface {
	PutHTML(ctx context.Context, runID string, html string) (id string, err error)
	Get(ctx context.Context, id string) (html string, runID string, err error)
}
