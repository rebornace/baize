package s3_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/s3"
)

// fakeS3 is a minimal path-style S3 endpoint sufficient for minio-go
// PutObject/GetObject/RemoveObject/BucketExists/MakeBucket.
type fakeS3 struct {
	mu         sync.Mutex
	objects    map[string][]byte // object name (no bucket) -> bytes
	putCT      map[string]string
	madeBucket map[string]bool
}

func newFakeS3() *fakeS3 {
	return &fakeS3{objects: map[string][]byte{}, putCT: map[string]string{}, madeBucket: map[string]bool{}}
}

func (f *fakeS3) handler(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	bucket := parts[0]
	object := ""
	if len(parts) > 1 {
		object = parts[1]
	}
	switch r.Method {
	case http.MethodHead: // BucketExists
		if bucket == "baize" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	case http.MethodPut:
		if object == "" { // MakeBucket
			f.mu.Lock()
			f.madeBucket[bucket] = true
			f.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			return
		}
		b, _ := io.ReadAll(r.Body)
		// http (非 https) 下 minio-go 用 SigV4 流式分帧上传：
		// Content-Encoding: aws-chunked，帧格式
		// "<hex>;chunk-signature=<sig>\r\n<data>\r\n" 重复 + 末块 "0;...\r\n\r\n"。
		if strings.Contains(r.Header.Get("Content-Encoding"), "aws-chunked") {
			decoded, derr := decodeAWSChunked(bytes.NewReader(b))
			if derr != nil {
				http.Error(w, derr.Error(), http.StatusBadRequest)
				return
			}
			b = decoded
		}
		f.mu.Lock()
		f.objects[object] = b
		f.putCT[object] = r.Header.Get("Content-Type")
		f.mu.Unlock()
		w.Header().Set("ETag", `"etag"`)
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		// minio-go 在签名前先发 GET /bucket?location 探测区域（path-style）。
		// 真实 S3：桶存在返回 LocationConstraint XML；不存在返回 404。
		if object == "" && r.URL.Query().Has("location") {
			if bucket == "baize" {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></LocationConstraint>`)
				return
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `<?xml version="1.0"?><Error><Code>NoSuchBucket</Code><Message>missing</Message><BucketName>%s</BucketName></Error>`, bucket)
			return
		}
		if object == "" && r.URL.Query().Get("list-type") == "2" {
			pfx := r.URL.Query().Get("prefix")
			f.mu.Lock()
			type kv struct {
				key  string
				size int64
			}
			var matched []kv
			for k, v := range f.objects {
				if strings.HasPrefix(k, pfx) {
					matched = append(matched, kv{k, int64(len(v))})
				}
			}
			f.mu.Unlock()
			sort.Slice(matched, func(i, j int) bool { return matched[i].key < matched[j].key })
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
			for _, m := range matched {
				fmt.Fprintf(w, `<Contents><Key>%s</Key><Size>%d</Size></Contents>`,
					xmlEscape(m.key), m.size)
			}
			_, _ = io.WriteString(w, `</ListBucketResult>`)
			return
		}
		f.mu.Lock()
		b, ok := f.objects[object]
		f.mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `<?xml version="1.0"?><Error><Code>NoSuchKey</Code><Message>missing</Message><Key>%s</Key></Error>`, object)
			return
		}
		w.Header().Set("Last-Modified", "Mon, 2 Sep 2024 00:00:00 GMT")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	case http.MethodDelete:
		f.mu.Lock()
		delete(f.objects, object)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

func openAgainst(t *testing.T, srv *httptest.Server, bucket string, autoCreate bool) (blob.Store, error) {
	t.Helper()
	endpoint := strings.TrimPrefix(srv.URL, "http://")
	return blob.Open(context.Background(), "s3", blob.Options{S3: blob.S3Options{
		Endpoint:   endpoint,
		Bucket:     bucket,
		Prefix:     "baize",
		AccessKey:  "ak",
		SecretKey:  "sk",
		UseSSL:     false,
		PathStyle:  true,
		AutoCreate: autoCreate,
	}})
}

func TestS3PutGetRoundTrip(t *testing.T) {
	fake := newFakeS3()
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer srv.Close()

	s, err := openAgainst(t, srv, "baize", false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "artifacts/art_1.html", []byte("<html>ok</html>"), "text/html; charset=utf-8"); err != nil {
		t.Fatal(err)
	}
	// 对象名含 prefix，Content-Type 透传。
	fake.mu.Lock()
	ct := fake.putCT["baize/artifacts/art_1.html"]
	_, present := fake.objects["baize/artifacts/art_1.html"]
	fake.mu.Unlock()
	if !present {
		t.Fatalf("object not stored under prefix; keys=%v", mapKeys(fake))
	}
	if ct != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type=%q", ct)
	}
	got, err := s.Get(ctx, "artifacts/art_1.html")
	if err != nil || string(got) != "<html>ok</html>" {
		t.Fatalf("get=%q err=%v", got, err)
	}
}

func TestS3GetMissingMapsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(newFakeS3().handler))
	defer srv.Close()
	s, err := openAgainst(t, srv, "baize", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Get(context.Background(), "artifacts/nope.html")
	if !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestS3DeleteIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(newFakeS3().handler))
	defer srv.Close()
	s, err := openAgainst(t, srv, "baize", false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "k", []byte("v"), ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "k"); err != nil { // 幂等
		t.Fatalf("delete missing should be nil, got %v", err)
	}
}

func TestS3MissingBucketNoAutoCreateFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(newFakeS3().handler))
	defer srv.Close()
	if _, err := openAgainst(t, srv, "other-bucket", false); err == nil {
		t.Fatalf("want error when bucket missing and auto_create=false")
	}
}

func TestS3MissingBucketAutoCreate(t *testing.T) {
	fake := newFakeS3()
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer srv.Close()
	// fake 对未知 bucket HEAD 404，但 PUT /bucket 仍 200（MakeBucket）。
	s, err := openAgainst(t, srv, "fresh-bucket", true)
	if err != nil {
		t.Fatalf("auto-create should succeed, got %v", err)
	}
	fake.mu.Lock()
	created := fake.madeBucket["fresh-bucket"]
	fake.mu.Unlock()
	if !created {
		t.Fatalf("MakeBucket was not called for fresh-bucket")
	}
	if err := s.Put(context.Background(), "k", []byte("v"), ""); err != nil {
		t.Fatal(err)
	}
}

func TestS3RequiresCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(newFakeS3().handler))
	defer srv.Close()
	endpoint := strings.TrimPrefix(srv.URL, "http://")
	if _, err := blob.Open(context.Background(), "s3", blob.Options{S3: blob.S3Options{
		Endpoint: endpoint, Bucket: "baize", UseSSL: false, PathStyle: true,
	}}); err == nil {
		t.Fatalf("want error when credentials missing")
	}
}

func TestS3ListByPrefix(t *testing.T) {
	fake := newFakeS3()
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer srv.Close()
	s, err := openAgainst(t, srv, "baize", false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_ = s.Put(ctx, "workspaces/c1/a.txt", []byte("ab"), "")
	_ = s.Put(ctx, "workspaces/c1/n/b.txt", []byte("cde"), "")
	_ = s.Put(ctx, "workspaces/c2/z.txt", []byte("z"), "")
	got, err := s.List(ctx, "workspaces/c1/")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %v", got)
	}
	// key 应已剥离驱动 prefix（"baize/"），与 Put 用的 key 空间一致。
	want := map[string]int64{"workspaces/c1/a.txt": 2, "workspaces/c1/n/b.txt": 3}
	for _, e := range got {
		if want[e.Key] != e.Size {
			t.Fatalf("unexpected entry %+v (want map %v)", e, want)
		}
	}
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func mapKeys(f *fakeS3) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []string{}
	for k := range f.objects {
		out = append(out, k)
	}
	return out
}

// decodeAWSChunked decodes an aws-chunked SigV4 streaming body as produced by
// minio-go over plain HTTP, validating the framing rather than blindly
// stripping it (so the assertion would fail if the client stopped chunking).
// Layout: "<hexLen>;chunk-signature=<sig>\r\n<data>\r\n" ... "0;...\r\n\r\n".
func decodeAWSChunked(r io.Reader) ([]byte, error) {
	br := bufio.NewReader(r)
	var out bytes.Buffer
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("aws-chunked: read chunk header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		semi := strings.Index(line, ";")
		sizeField := line
		if semi >= 0 {
			sizeField = line[:semi]
		}
		size, err := strconv.ParseInt(sizeField, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("aws-chunked: bad chunk size %q: %w", sizeField, err)
		}
		if size == 0 { // terminal chunk; expect trailing \r\n.
			return out.Bytes(), nil
		}
		chunk := make([]byte, size)
		if _, err := io.ReadFull(br, chunk); err != nil {
			return nil, fmt.Errorf("aws-chunked: read chunk data: %w", err)
		}
		out.Write(chunk)
		crlf := make([]byte, 2)
		if _, err := io.ReadFull(br, crlf); err != nil || crlf[0] != '\r' || crlf[1] != '\n' {
			return nil, fmt.Errorf("aws-chunked: missing CRLF after chunk")
		}
	}
}
