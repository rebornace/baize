package applog_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/applog"
)

func TestInitJSONEmitsComponent(t *testing.T) {
	var buf bytes.Buffer
	applog.ResetRingForTest()
	applog.InitWithWriter("json", &buf)
	applog.L().Info("hello", slog.String("component", "test"))
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["component"] != "test" || m["msg"] != "hello" {
		t.Fatalf("got %#v", m)
	}
}

func TestRecentCapturesLines(t *testing.T) {
	var buf bytes.Buffer
	applog.ResetRingForTest()
	applog.InitWithWriter("text", &buf)
	applog.L().Info("ring-one", slog.String("component", "test"))
	applog.L().Info("ring-two", slog.String("component", "test"))
	got := applog.Recent(10)
	if len(got) < 2 {
		t.Fatalf("want >=2 lines, got %v", got)
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "ring-one") || !strings.Contains(joined, "ring-two") {
		t.Fatalf("missing messages: %v", got)
	}
	last := applog.Recent(1)
	if len(last) != 1 || !strings.Contains(last[0], "ring-two") {
		t.Fatalf("Recent(1)=%v", last)
	}
}
