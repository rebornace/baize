package convlock

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDoSerializesSameKey(t *testing.T) {
	g := New()
	var active, maxActive int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = g.Do("conv", func() error {
				cur := atomic.AddInt32(&active, 1)
				for {
					old := atomic.LoadInt32(&maxActive)
					if cur <= old || atomic.CompareAndSwapInt32(&maxActive, old, cur) {
						break
					}
				}
				time.Sleep(2 * time.Millisecond)
				atomic.AddInt32(&active, -1)
				return nil
			})
		}()
	}
	wg.Wait()
	if maxActive != 1 {
		t.Fatalf("same key must be serialized, maxActive=%d", maxActive)
	}
	if g.size() != 0 {
		t.Fatalf("entries not reclaimed: %d", g.size())
	}
}

func TestDoAllowsDifferentKeys(t *testing.T) {
	g := New()
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan struct{}, 2)
	for _, key := range []string{"a", "b"} {
		go func(key string) {
			_ = g.Do(key, func() error {
				entered <- struct{}{}
				<-release
				return nil
			})
			done <- struct{}{}
		}(key)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("different keys must not block each other")
		}
	}
	close(release)
	for i := 0; i < 2; i++ {
		<-done
	}
}

func TestDoUnlocksOnError(t *testing.T) {
	g := New()
	want := errors.New("boom")
	if err := g.Do("conv", func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("err=%v want %v", err, want)
	}
	if err := g.Do("conv", func() error { return nil }); err != nil {
		t.Fatalf("guard stayed locked: %v", err)
	}
}

func TestDoEmptyKeyBypasses(t *testing.T) {
	g := New()
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() {
			_ = g.Do("", func() error {
				entered <- struct{}{}
				<-release
				return nil
			})
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("empty key must bypass the lock")
		}
	}
	close(release)
	if g.size() != 0 {
		t.Fatalf("empty key created entries: %d", g.size())
	}
}

func TestNilGuardRunsFn(t *testing.T) {
	var g *Guard
	ran := false
	if err := g.Do("conv", func() error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("nil guard must run fn: err=%v ran=%v", err, ran)
	}
}
