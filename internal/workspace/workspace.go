package workspace

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/rebornace/baize/internal/blob"
	"github.com/rebornace/baize/internal/llm"
)

const (
	rootPrefix    = "workspaces"
	uploadsDir    = "uploads"
	MaxWriteBytes = 256 << 10 // 256 KiB
	MaxReadBytes  = 64 << 10  // 64 KiB returned to the model
	MaxImageBytes = 10 << 20  // 10 MiB images
)

// TruncatedMarker is appended to read_file content cut at MaxReadBytes.
const TruncatedMarker = "…[truncated]"

// ErrNotFound is returned when a workspace file does not exist.
var ErrNotFound = errors.New("workspace: file not found")

// Entry is one logical directory entry returned by ListFiles.
type Entry struct {
	Name string `json:"name"`
	Type string `json:"type"` // "file" | "dir"
	Size int64  `json:"size,omitempty"`
}

// Service manages per-conversation workspaces on top of a blob.Store.
type Service struct {
	blobs    blob.Store
	visionOK func() bool
}

// Option configures a Service.
type Option func(*Service)

// WithVision supplies a vision-capability probe (returns true if the active
// model accepts image parts). When nil, read_image assumes vision is off.
func WithVision(f func() bool) Option {
	return func(s *Service) { s.visionOK = f }
}

// New builds a Service over the given blob store.
func New(blobs blob.Store, opts ...Option) *Service {
	s := &Service{blobs: blobs, visionOK: func() bool { return false }}
	for _, o := range opts {
		o(s)
	}
	return s
}

// VisionEnabled reports whether the active model supports images.
func (s *Service) VisionEnabled() bool { return s.visionOK != nil && s.visionOK() }

// safeConvID validates a server-supplied conversation ID before it is embedded
// in a blob key (defense-in-depth, carry-over M-4). Even though callers should
// only pass trusted IDs, this guarantees path.Join(rootPrefix, convID, rel) can
// never escape the "workspaces/" prefix: the ID must be non-empty and must not
// contain separators, traversal segments, or dot-only names.
func safeConvID(convID string) error {
	if strings.TrimSpace(convID) == "" {
		return errors.New("workspace requires a conversation context")
	}
	// Normalize backslashes the same way safeRelPath does so a Windows-style
	// separator cannot be used to smuggle a path component.
	normalized := strings.ReplaceAll(convID, "\\", "/")
	if strings.ContainsAny(normalized, "/") ||
		strings.Contains(convID, "\\") ||
		convID == "." || convID == ".." ||
		strings.Contains(convID, "..") {
		return fmt.Errorf("invalid conversation ID %q: must not contain path separators or traversal", convID)
	}
	return nil
}

func requireConv(convID string) error {
	return safeConvID(convID)
}

// key maps a logical relative path to a blob key for the conversation.
func (s *Service) key(convID, rel string) string {
	return path.Join(rootPrefix, convID, rel)
}

// SaveUpload persists an extracted text upload under uploads/ and returns its
// logical path (e.g. "uploads/report.pdf").
func (s *Service) SaveUpload(ctx context.Context, convID, filename, text string) (string, error) {
	if err := requireConv(convID); err != nil {
		return "", err
	}
	logical := path.Join(uploadsDir, sanitizeName(filename))
	if err := s.blobs.Put(ctx, s.key(convID, logical), []byte(text), "text/plain; charset=utf-8"); err != nil {
		return "", err
	}
	return logical, nil
}

// SaveUploadBytes persists an image upload (raw bytes) under uploads/ and
// returns its logical path.
func (s *Service) SaveUploadBytes(ctx context.Context, convID, filename string, data []byte, mime string) (string, error) {
	if err := requireConv(convID); err != nil {
		return "", err
	}
	if mime == "" {
		mime = http.DetectContentType(data)
	}
	logical := path.Join(uploadsDir, sanitizeName(filename))
	if err := s.blobs.Put(ctx, s.key(convID, logical), data, mime); err != nil {
		return "", err
	}
	return logical, nil
}

// WriteFile stores content at a logical path (whole-file overwrite).
func (s *Service) WriteFile(ctx context.Context, convID, relPath, content string) error {
	if err := requireConv(convID); err != nil {
		return err
	}
	rel, err := safeRelPath(relPath)
	if err != nil {
		return err
	}
	if len(content) > MaxWriteBytes {
		return fmt.Errorf("file too large: %d bytes > %d limit", len(content), MaxWriteBytes)
	}
	return s.blobs.Put(ctx, s.key(convID, rel), []byte(content), "text/plain; charset=utf-8")
}

// ReadFile returns UTF-8 text content, truncated at MaxReadBytes.
func (s *Service) ReadFile(ctx context.Context, convID, relPath string) (string, error) {
	if err := requireConv(convID); err != nil {
		return "", err
	}
	rel, err := safeRelPath(relPath)
	if err != nil {
		return "", err
	}
	b, err := s.blobs.Get(ctx, s.key(convID, rel))
	if err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			return "", fmt.Errorf("%s: %w", rel, ErrNotFound)
		}
		return "", err
	}
	if ct := http.DetectContentType(b); strings.HasPrefix(ct, "image/") {
		return "", fmt.Errorf("%s is a binary image; use read_image to view it", rel)
	}
	if len(b) > MaxReadBytes {
		// Cut on the byte budget, then back up to a valid rune boundary so we
		// never emit a partial multi-byte tail.
		cut := MaxReadBytes
		for cut > 0 && !utf8.RuneStart(b[cut]) {
			cut--
		}
		return string(b[:cut]) + TruncatedMarker, nil
	}
	return string(b), nil
}

// DeleteFile removes a file; deleting a missing file is a no-op success.
func (s *Service) DeleteFile(ctx context.Context, convID, relPath string) error {
	if err := requireConv(convID); err != nil {
		return err
	}
	rel, err := safeRelPath(relPath)
	if err != nil {
		return err
	}
	return s.blobs.Delete(ctx, s.key(convID, rel))
}

// ListFiles aggregates the flat blob key space into one directory level under
// dir (empty = workspace root). Entries are files or immediate subdirectories.
func (s *Service) ListFiles(ctx context.Context, convID, dir string) ([]Entry, error) {
	if err := requireConv(convID); err != nil {
		return nil, err
	}
	cleanDir := ""
	if strings.TrimSpace(dir) != "" {
		d, err := safeRelPath(dir)
		if err != nil {
			return nil, err
		}
		cleanDir = d
	}
	listPrefix := s.key(convID, cleanDir)
	if cleanDir != "" {
		listPrefix += "/"
	} else {
		listPrefix = path.Join(rootPrefix, convID) + "/"
	}
	objs, err := s.blobs.List(ctx, listPrefix)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []Entry
	for _, o := range objs {
		logical := strings.TrimPrefix(o.Key, listPrefix)
		if logical == "" {
			continue
		}
		first := logical
		isDir := false
		if idx := strings.IndexByte(logical, '/'); idx >= 0 {
			first = logical[:idx]
			isDir = true
		}
		if first == "" || seen[first] {
			continue
		}
		seen[first] = true
		if isDir {
			out = append(out, Entry{Name: first, Type: "dir"})
		} else {
			out = append(out, Entry{Name: first, Type: "file", Size: o.Size})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type == "dir" // dirs first
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// ReadImage returns an image content part and detected MIME for a logical path.
func (s *Service) ReadImage(ctx context.Context, convID, relPath string) (llm.ContentPart, string, error) {
	if err := requireConv(convID); err != nil {
		return llm.ContentPart{}, "", err
	}
	if !s.VisionEnabled() {
		return llm.ContentPart{}, "", errors.New("current model does not support vision; switch to a vision-capable model to view images")
	}
	rel, err := safeRelPath(relPath)
	if err != nil {
		return llm.ContentPart{}, "", err
	}
	b, err := s.blobs.Get(ctx, s.key(convID, rel))
	if err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			return llm.ContentPart{}, "", fmt.Errorf("%s: %w", rel, ErrNotFound)
		}
		return llm.ContentPart{}, "", err
	}
	if len(b) > MaxImageBytes {
		return llm.ContentPart{}, "", fmt.Errorf("image too large: %d bytes > %d limit", len(b), MaxImageBytes)
	}
	ct := http.DetectContentType(b)
	if !strings.HasPrefix(ct, "image/") {
		return llm.ContentPart{}, "", fmt.Errorf("%s is not an image (detected %s); use read_file for text", rel, ct)
	}
	return llm.ContentPart{Type: "image", ImageMIME: ct, ImageBytes: b}, ct, nil
}

// ResolveImagePart rebuilds an image part from a persisted image_refs pointer
// (used by the engine on cold resume). ok=false (no error) when missing.
func (s *Service) ResolveImagePart(ctx context.Context, convID, relPath string) (llm.ContentPart, bool) {
	part, _, err := s.ReadImage(ctx, convID, relPath)
	if err != nil {
		return llm.ContentPart{}, false
	}
	return part, true
}
