package webhook

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExplicitFixedAddr(t *testing.T) {
	if !explicitFixedAddr([]string{"-addr=127.0.0.1:8090"}) {
		t.Fatal("expected fixed")
	}
	if explicitFixedAddr([]string{"-addr=127.0.0.1:0"}) {
		t.Fatal(":0 is not fixed")
	}
	if explicitFixedAddr([]string{"-creds=./data"}) {
		t.Fatal("no addr => not fixed (dynamic default)")
	}
}

func TestInjectDynamicPortArgs(t *testing.T) {
	out := injectDynamicPortArgs([]string{"-addr=127.0.0.1:8090", "-creds=./d"}, "./d/listen.port")
	joined := strings.Join(out, " ")
	if strings.Contains(joined, "8090") {
		t.Fatalf("old fixed addr remained: %v", out)
	}
	var hasAddr0, hasPortFile bool
	for _, a := range out {
		if a == "-addr=127.0.0.1:0" {
			hasAddr0 = true
		}
		if a == "-port-file=./d/listen.port" {
			hasPortFile = true
		}
		if a == "-creds=./d" {
			// ok
		}
	}
	if !hasAddr0 || !hasPortFile {
		t.Fatalf("missing dynamic flags: %v", out)
	}
	if len(out) != 3 {
		t.Fatalf("args=%v", out)
	}
}

func TestWaitPortFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "listen.port")
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = os.WriteFile(path, []byte("127.0.0.1:5555\n"), 0o644)
	}()
	addr, err := waitPortFile(context.Background(), path, 2*time.Second)
	if err != nil || addr != "127.0.0.1:5555" {
		t.Fatalf("addr=%q err=%v", addr, err)
	}
}

func TestHTTPBaseFromListenAddr(t *testing.T) {
	if httpBaseFromListenAddr("127.0.0.1:9") != "http://127.0.0.1:9" {
		t.Fatal()
	}
}
