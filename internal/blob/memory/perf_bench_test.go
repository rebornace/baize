package memory_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
)

func BenchmarkPerfBlobPutGet64KiB(b *testing.B) {
	s, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		b.Fatal(err)
	}
	payload := bytes.Repeat([]byte("x"), 64*1024)
	ctx := context.Background()
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("k-%d", i)
		if err := s.Put(ctx, key, payload, "application/octet-stream"); err != nil {
			b.Fatal(err)
		}
		if _, err := s.Get(ctx, key); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPerfBlobPutGet256KiB(b *testing.B) {
	s, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		b.Fatal(err)
	}
	payload := bytes.Repeat([]byte("x"), 256*1024)
	ctx := context.Background()
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("k-%d", i)
		if err := s.Put(ctx, key, payload, "application/octet-stream"); err != nil {
			b.Fatal(err)
		}
		if _, err := s.Get(ctx, key); err != nil {
			b.Fatal(err)
		}
	}
}
