package blob

import (
	"fmt"
	"strings"
)

const (
	PrefixConnectors     = "connectors/"
	PrefixSkillsUser     = "skills/user/"
	PrefixSkillsManaged  = "skills/managed/"
	PrefixSkillsArchived = "skills/archived/"
)

// ConnectorNormalizedKey is the blob key for normalized OpenAPI stored in Connector.Spec.
func ConnectorNormalizedKey(id string) string {
	return PrefixConnectors + id + "/openapi.normalized.json"
}

// ConnectorImportedKey is the blob key for the raw imported connector payload.
func ConnectorImportedKey(id string) string {
	return PrefixConnectors + id + "/imported.bin"
}

// SkillObjectKey builds a blob key for a user, managed, or archived skill object.
// source must be "user", "managed", or "archived"; rel is relative to the skill package root.
func SkillObjectKey(source, id, rel string) string {
	var prefix string
	switch source {
	case "user":
		prefix = PrefixSkillsUser
	case "managed":
		prefix = PrefixSkillsManaged
	case "archived":
		prefix = PrefixSkillsArchived
	default:
		panic(fmt.Sprintf("blob: unknown skill source %q (want user, managed, or archived)", source))
	}
	rel = strings.TrimPrefix(rel, "/")
	return prefix + id + "/" + rel
}
