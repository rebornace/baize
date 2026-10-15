package applog_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/rebornace/baize/internal/applog"
)

func TestInitJSONEmitsComponent(t *testing.T) {
	var buf bytes.Buffer
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
