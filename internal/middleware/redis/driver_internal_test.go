package redis

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestDefaultConsumerNameIncludesPID(t *testing.T) {
	got := defaultConsumerName()
	wantSuffix := fmt.Sprintf("-%d", os.Getpid())
	if !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("consumer name %q want suffix %q", got, wantSuffix)
	}
	prefix := strings.TrimSuffix(got, wantSuffix)
	if prefix == "" {
		t.Fatal("consumer name missing hostname/baize prefix")
	}
}

func TestOpenAppliesHostnamePIDConsumerName(t *testing.T) {
	mr := miniredis.RunT(t)
	mw, err := Open(context.Background(), Config{
		Addr: mr.Addr(), Stream: "baize:runs-cname", ConsumerGroup: "baize-workers",
		EventsChannel: "baize:run-events-cname",
		// ConsumerName empty → Open fills hostname-pid
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mw.Close() })

	q, ok := mw.Queue.(*queue)
	if !ok {
		t.Fatal("queue type")
	}
	want := defaultConsumerName()
	if q.consumer != want {
		t.Fatalf("consumer=%q want %q", q.consumer, want)
	}
}

func TestOpenExplicitConsumerNameOverridesDefault(t *testing.T) {
	mr := miniredis.RunT(t)
	mw, err := Open(context.Background(), Config{
		Addr: mr.Addr(), Stream: "baize:runs-cname2", ConsumerGroup: "baize-workers",
		EventsChannel: "baize:run-events-cname2", ConsumerName: "explicit-consumer",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mw.Close() })
	q := mw.Queue.(*queue)
	if q.consumer != "explicit-consumer" {
		t.Fatalf("consumer=%q want explicit-consumer", q.consumer)
	}
}
