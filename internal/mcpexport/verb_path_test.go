package mcpexport_test

import (
	"testing"

	"github.com/rebornace/baize/internal/mcpexport"
)

func TestVerbInPathGET(t *testing.T) {
	if !mcpexport.SuggestForceDenyExport("GET", "/orders/approve") {
		t.Fatal("approve")
	}
	if !mcpexport.SuggestForceDenyExport("GET", "/export/run") {
		t.Fatal("export/run")
	}
	if mcpexport.SuggestForceDenyExport("GET", "/orders/{id}") {
		t.Fatal("plain get")
	}
	if mcpexport.SuggestForceDenyExport("POST", "/orders/approve") {
		t.Fatal("POST not this heuristic")
	}
}
