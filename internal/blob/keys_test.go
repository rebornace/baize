package blob_test

import (
	"testing"

	"github.com/rebornace/baize/internal/blob"
)

func TestConnectorNormalizedKey(t *testing.T) {
	if blob.ConnectorNormalizedKey("c1") != "connectors/c1/openapi.normalized.json" {
		t.Fatal()
	}
}

func TestConnectorImportedKey(t *testing.T) {
	if blob.ConnectorImportedKey("c1") != "connectors/c1/imported.bin" {
		t.Fatal()
	}
}

func TestSkillObjectKey(t *testing.T) {
	if blob.SkillObjectKey("user", "s1", "SKILL.md") != "skills/user/s1/SKILL.md" {
		t.Fatal()
	}
	if blob.SkillObjectKey("managed", "m1", "workflow.yaml") != "skills/managed/m1/workflow.yaml" {
		t.Fatal()
	}
}
