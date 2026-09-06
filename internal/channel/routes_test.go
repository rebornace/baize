package channel

import (
	"net/http"
	"testing"
)

type fakeRegistrar struct {
	patterns []string
}

func (f *fakeRegistrar) RegisterRoute(pattern string, h http.Handler) {
	f.patterns = append(f.patterns, pattern)
}

func TestRouteRegistrarInterface(t *testing.T) {
	var _ RouteRegistrar = (*fakeRegistrar)(nil)
	fr := &fakeRegistrar{}
	fr.RegisterRoute("POST /v0/channels/x/inbound", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	if len(fr.patterns) != 1 || fr.patterns[0] != "POST /v0/channels/x/inbound" {
		t.Fatalf("unexpected patterns: %v", fr.patterns)
	}
}

func TestBuildDepsCarriesRoutes(t *testing.T) {
	fr := &fakeRegistrar{}
	deps := BuildDeps{Routes: fr}
	if deps.Routes == nil {
		t.Fatal("Routes not carried")
	}
}
