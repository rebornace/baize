// Package blob defines a swappable object-storage abstraction for binary
// content (report artifacts, and later connector specs / skills / attachments).
// Drivers register themselves via RegisterDriver, mirroring the middleware
// driver pattern; "file" is the zero-dependency default.
package blob

import (
	"context"
	"errors"
)

// ErrNotFound is returned by Get when an object does not exist. Drivers map
// their underlying "not found" error to this sentinel (wrap with fmt.Errorf
// "...: %w", blob.ErrNotFound). Delete treats a missing object as a no-op
// success and does not return ErrNotFound.
var ErrNotFound = errors.New("blob: object not found")

// Store is a key/value binary object store. Keys are driver-relative and use "/"
// separators (e.g. "artifacts/art_xxx.html"). There is no directory notion;
// drivers create parent paths as needed.
type Store interface {
	// Put writes data under key. contentType sets the S3 Content-Type header;
	// the file driver may ignore it.
	Put(ctx context.Context, key string, data []byte, contentType string) error
	// Get returns the object bytes. A missing object returns an error wrapping
	// ErrNotFound.
	Get(ctx context.Context, key string) ([]byte, error)
	// Delete removes the object. Deleting a missing object is a no-op success.
	Delete(ctx context.Context, key string) error
}

// Options configures a blob driver. Only the section for the selected driver is
// read.
type Options struct {
	File FileOptions
	S3   S3Options
}

// FileOptions configures the local-disk driver.
type FileOptions struct {
	// RootDir is the directory under which keys are materialized. Required for
	// the file driver.
	RootDir string
}

// S3Options configures the S3-compatible driver (AWS S3 / MinIO / OSS / COS).
type S3Options struct {
	Endpoint   string // e.g. s3.amazonaws.com, play.min.io, oss-cn-hangzhou.aliyuncs.com
	Region     string
	Bucket     string
	Prefix     string // prepended to every key for multi-tenant/env isolation
	AccessKey  string // resolved from the environment by bootstrap
	SecretKey  string
	UseSSL     bool
	PathStyle  bool // true for MinIO/self-hosted; false for AWS
	AutoCreate bool // MakeBucket on startup if missing
}
