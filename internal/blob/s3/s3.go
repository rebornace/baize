// Package s3 is the S3-compatible blob driver (AWS S3 / MinIO / Aliyun OSS /
// Tencent COS) backed by github.com/minio/minio-go/v7 (Apache-2.0 client SDK).
package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rebornace/baize/internal/blob"
)

func init() {
	blob.RegisterDriver("s3", open)
}

type store struct {
	client *minio.Client
	bucket string
	prefix string // 无首尾 "/"
}

var _ blob.Store = (*store)(nil)

func open(ctx context.Context, opts blob.Options) (blob.Store, error) {
	c := opts.S3
	if c.Endpoint == "" {
		return nil, fmt.Errorf("blob s3 driver: storage.s3.endpoint is required")
	}
	if c.Bucket == "" {
		return nil, fmt.Errorf("blob s3 driver: storage.s3.bucket is required")
	}
	if c.AccessKey == "" || c.SecretKey == "" {
		return nil, fmt.Errorf("blob s3 driver: access/secret key are required (set storage.s3.access_key_env/secret_key_env)")
	}
	mopts := &minio.Options{
		Creds:  credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""),
		Secure: c.UseSSL,
		Region: c.Region,
	}
	if c.PathStyle {
		mopts.BucketLookup = minio.BucketLookupPath
	}
	cl, err := minio.New(c.Endpoint, mopts)
	if err != nil {
		return nil, fmt.Errorf("s3 client: %w", err)
	}

	exists, err := cl.BucketExists(ctx, c.Bucket)
	if err != nil {
		return nil, fmt.Errorf("s3 bucket %q check: %w", c.Bucket, err)
	}
	if !exists {
		if !c.AutoCreate {
			return nil, fmt.Errorf("s3 bucket %q does not exist at %q (set storage.s3.auto_create_bucket: true to create it)", c.Bucket, c.Endpoint)
		}
		if err := cl.MakeBucket(ctx, c.Bucket, minio.MakeBucketOptions{Region: c.Region}); err != nil {
			return nil, fmt.Errorf("create s3 bucket %q: %w", c.Bucket, err)
		}
	}
	return &store{client: cl, bucket: c.Bucket, prefix: strings.Trim(c.Prefix, "/")}, nil
}

func (s *store) object(key string) string {
	if s.prefix == "" {
		return key
	}
	return s.prefix + "/" + key
}

func (s *store) Put(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, s.object(key),
		bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}

func (s *store) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, s.object(key), minio.GetObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("get %s: %w", key, blob.ErrNotFound)
		}
		return nil, fmt.Errorf("s3 get %s: %w", key, err)
	}
	defer obj.Close()
	b, err := io.ReadAll(obj) // GetObject 的 NoSuchKey 常在 Read 时暴露
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("get %s: %w", key, blob.ErrNotFound)
		}
		return nil, fmt.Errorf("s3 read %s: %w", key, err)
	}
	return b, nil
}

func (s *store) Delete(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.bucket, s.object(key), minio.RemoveObjectOptions{})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("s3 delete %s: %w", key, err)
	}
	return nil
}

func (s *store) List(ctx context.Context, prefix string) ([]blob.ListEntry, error) {
	fullPrefix := s.object(prefix)
	out := make([]blob.ListEntry, 0)
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    fullPrefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("s3 list %s: %w", prefix, obj.Err)
		}
		key := obj.Key
		if s.prefix != "" {
			key = strings.TrimPrefix(key, s.prefix+"/")
		}
		out = append(out, blob.ListEntry{Key: key, Size: obj.Size})
	}
	return out, nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	resp := minio.ToErrorResponse(err)
	return resp.Code == "NoSuchKey" || resp.Code == "NoSuchBucket" || resp.StatusCode == http.StatusNotFound
}
