package config_test

import (
	"testing"

	"github.com/rebornace/baize/internal/config"
)

func TestLoadConnectorGroups(t *testing.T) {
	path := writeConfig(t, `
store:
  driver: memory
connector_groups:
  - id: finance
    connectors: [expense, invoice]
  - id: commerce
    connectors: [mall, order]
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ConnectorGroups) != 2 {
		t.Fatalf("groups=%d want 2", len(cfg.ConnectorGroups))
	}
	if cfg.ConnectorGroups[0].ID != "finance" || len(cfg.ConnectorGroups[0].Connectors) != 2 {
		t.Fatalf("finance=%+v", cfg.ConnectorGroups[0])
	}
	if cfg.ConnectorGroups[1].ID != "commerce" {
		t.Fatalf("commerce=%+v", cfg.ConnectorGroups[1])
	}
}

func TestLoadConnectorGroupsOmitted(t *testing.T) {
	path := writeConfig(t, "store:\n  driver: memory\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ConnectorGroups) != 0 {
		t.Fatalf("want empty, got %+v", cfg.ConnectorGroups)
	}
}
