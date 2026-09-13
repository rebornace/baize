package webhook

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/channel"
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

func TestBootstrapInjectsDynamicPort(t *testing.T) {
	creds := t.TempDir()
	c, err := openFromConfig("weixin", map[string]string{
		"secret":            "s",
		"outbound_url":      "http://127.0.0.1:8090/outbound",
		"admin_url":         "http://127.0.0.1:8090",
		"assignee":          "a",
		"adapter_autostart": "true",
		"adapter_command":   "weixin-adapter",
		"adapter_args":      "-creds=" + creds,
		"adapter_creds_dir": creds,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := c.Bootstrap(channel.BuildDeps{}); err != nil {
		t.Fatal(err)
	}
	if c.sup == nil || c.sup.portFile == "" {
		t.Fatal("expected dynamic portFile on supervisor")
	}
	joined := strings.Join(c.sup.args, " ")
	if !strings.Contains(joined, "-addr=127.0.0.1:0") || !strings.Contains(joined, "-port-file=") {
		t.Fatalf("args=%v", c.sup.args)
	}
}

func TestBootstrapKeepsFixedAddr(t *testing.T) {
	creds := t.TempDir()
	c, err := openFromConfig("weixin", map[string]string{
		"secret":            "s",
		"outbound_url":      "http://127.0.0.1:8090/outbound",
		"admin_url":         "http://127.0.0.1:8090",
		"assignee":          "a",
		"adapter_autostart": "true",
		"adapter_command":   "weixin-adapter",
		"adapter_args":      "-addr=127.0.0.1:8090,-creds=" + creds,
		"adapter_creds_dir": creds,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := c.Bootstrap(channel.BuildDeps{}); err != nil {
		t.Fatal(err)
	}
	if c.sup.portFile != "" {
		t.Fatal("fixed addr must not set portFile")
	}
	joined := strings.Join(c.sup.args, " ")
	if !strings.Contains(joined, "-addr=127.0.0.1:8090") {
		t.Fatalf("args=%v", c.sup.args)
	}
	if strings.Contains(joined, "-addr=127.0.0.1:0") {
		t.Fatalf("dynamic addr injected: %v", c.sup.args)
	}
}

const fakeDynamicAdapterSource = `package main
import (
  "net"
  "net/http"
  "os"
)
func main() {
  addr := os.Getenv("FAKE_ADDR")
  if addr == "" { addr = "127.0.0.1:0" }
  ln, err := net.Listen("tcp", addr)
  if err != nil { panic(err) }
  if pf := os.Getenv("PORT_FILE"); pf != "" {
    if err := os.WriteFile(pf, []byte(ln.Addr().String()+"\n"), 0644); err != nil { panic(err) }
  }
  mux := http.NewServeMux()
  mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
  _ = http.Serve(ln, mux)
}
`

func buildFakeDynamicAdapter(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte(fakeDynamicAdapterSource), 0o600); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "fakedyn"+exeSuffix())
	build := exec.Command("go", "build", "-o", exe, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return exe
}

func TestSupervisorDynamicPortFile(t *testing.T) {
	exe := buildFakeDynamicAdapter(t)
	portFile := filepath.Join(t.TempDir(), "listen.port")
	sup := &supervisor{
		command:    exe,
		healthzURL: "http://127.0.0.1:1/healthz",
		portFile:   portFile,
		env:        []string{"FAKE_ADDR=127.0.0.1:0", "PORT_FILE=" + portFile},
		timeout:    10 * time.Second,
	}
	sup.onListen = func(addr string) {
		if _, _, err := net.SplitHostPort(addr); err != nil {
			t.Errorf("bad addr %q: %v", addr, err)
		}
		sup.healthzURL = "http://" + addr + "/healthz"
	}
	ctx := context.Background()
	if err := sup.start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = sup.stop(ctx) }()
	resp, err := http.Get(sup.healthzURL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}
