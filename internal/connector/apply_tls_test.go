package connector_test

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestApplyRejectsUnpairedTLSPaths(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()

	_, _, _, err := connector.Apply(connector.ApplyInput{
		Store:    st,
		Registry: reg,
		ID:       "tls-test",
		Type:     "http",
		BaseURL:  "http://127.0.0.1:9",
		TLS:      store.ConnectorTLS{CertFile: "/tmp/client.crt"},
	})
	if err == nil {
		t.Fatal("want error for cert without key")
	}
	if !strings.Contains(err.Error(), "cert_file and key_file") {
		t.Fatalf("unexpected error: %v", err)
	}
}
