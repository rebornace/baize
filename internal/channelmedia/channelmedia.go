// Package channelmedia persists images that arrive over messaging channels
// (WeChat, etc.) to the blob store and serves them back to the web UI with the
// owning conversation's access control. It lets an inbound WeChat image render
// inline in the chat instead of showing only a bare "（附件：…）" note.
//
// Storage layout (blob keys):
//
//	channel-media/<safe-conv-segment>/<uuid><ext>
//
// The conversation id is reduced to a filesystem-safe segment (":" and other
// separators replaced with "_") because the file blob driver maps keys 1:1 to
// paths and ":" is illegal in Windows filenames; the browser-facing URL keeps
// the real conversation id (used for the conversation ACL) and only the object
// name is used to locate the blob.
package channelmedia

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/google/uuid"

	"github.com/rebornace/baize/internal/blob"
)

// blobPrefix namespaces channel media inside the shared blob store (artifacts
// use "artifacts/", the file workspace uses "workspaces/").
const blobPrefix = "channel-media"

// MaxImageBytes caps a single served image (inbound images are thumbnail-capped
// upstream by internal/attach, so this is a defense-in-depth bound).
const MaxImageBytes = 8 << 20 // 8 MiB

// Store persists and serves inbound channel images on a blob.Store.
type Store struct {
	blobs blob.Store
}

// New builds a Store over the given blob store.
func New(blobs blob.Store) *Store {
	return &Store{blobs: blobs}
}

// SaveInboundImage stores data for convID and returns the browser-relative GET
// URL (served with the conversation ACL). It satisfies channel.MediaStore.
func (s *Store) SaveInboundImage(ctx context.Context, convID, filename, mime string, data []byte) (string, string, error) {
	if s == nil || s.blobs == nil {
		return "", "", errors.New("channelmedia: no blob store configured")
	}
	seg := safeConvSegment(convID)
	if seg == "" {
		return "", "", errors.New("channelmedia: empty conversation id")
	}
	obj := uuid.NewString() + extFor(filename, mime)
	key := path.Join(blobPrefix, seg, obj)
	if err := s.blobs.Put(ctx, key, data, mime); err != nil {
		return "", "", fmt.Errorf("channelmedia: put %s: %w", key, err)
	}
	// The URL carries the real conversation id (ACL) + the opaque object name.
	url := "/v0/channels/media/" + convID + "/" + obj
	return url, obj, nil
}

// OpenImage reads a previously stored image for convID/object, returning the
// bytes, detected MIME, and found=false (nil error) when the object does not
// exist. It satisfies api.ChannelMediaOpener. object must be a bare object name
// (no path separators); the blob key is rebuilt from the sanitized
// conversation segment so a crafted object cannot escape the namespace.
func (s *Store) OpenImage(ctx context.Context, convID, object string) ([]byte, string, bool, error) {
	if s == nil || s.blobs == nil {
		return nil, "", false, errors.New("channelmedia: no blob store configured")
	}
	object = strings.TrimSpace(object)
	if object == "" || object == "." || object == ".." ||
		strings.ContainsAny(object, "/\\") || strings.Contains(object, "..") {
		return nil, "", false, errors.New("channelmedia: invalid object name")
	}
	key := path.Join(blobPrefix, safeConvSegment(convID), object)
	b, err := s.blobs.Get(ctx, key)
	if err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			return nil, "", false, nil
		}
		return nil, "", false, fmt.Errorf("channelmedia: get %s: %w", key, err)
	}
	if len(b) > MaxImageBytes {
		return nil, "", false, fmt.Errorf("channelmedia: image too large: %d > %d", len(b), MaxImageBytes)
	}
	ct := http.DetectContentType(b)
	if !strings.HasPrefix(ct, "image/") {
		return nil, "", false, fmt.Errorf("channelmedia: object %s is not an image (detected %s)", object, ct)
	}
	return b, ct, true, nil
}

// safeConvSegment reduces a conversation id (e.g. "weixin:acc:peer") to a
// filesystem-safe path segment. Every rune that is not alphanumeric, '-', or
// '_' becomes '_', so Windows-illegal characters (':', '\') cannot break the
// file blob driver's path mapping.
func safeConvSegment(convID string) string {
	convID = strings.TrimSpace(convID)
	var b strings.Builder
	for _, r := range convID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// extFor picks a safe image extension from the filename, falling back to the
// MIME type, then ".jpg".
func extFor(filename, mime string) string {
	lower := strings.ToLower(strings.TrimSpace(filename))
	for _, e := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp"} {
		if strings.HasSuffix(lower, e) {
			return e
		}
	}
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	}
	return ".jpg"
}
