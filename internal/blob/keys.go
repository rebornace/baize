package blob

import (
	"fmt"
	"strings"
)

const (
	PrefixConnectors    = "connectors/"
	PrefixSkillsUser    = "skills/user/"
	PrefixSkillsManaged = "skills/managed/"
)

// ConnectorNormalizedKey is the blob key for normalized OpenAPI stored in Connector.Spec.
func ConnectorNormalizedKey(id string) string {
	return PrefixConnectors + id + "/openapi.normalized.json"
}

// ConnectorImportedKey is the blob key for the raw imported connector payload.
func ConnectorImportedKey(id string) string {
	return PrefixConnectors + id + "/imported.bin"
}

// SkillObjectKey builds a blob key for a user or managed skill object.
// source must be "user" or "managed"; rel is a path relative to the skill package root (e.g. SKILL.md).
func SkillObjectKey(source, id, rel string) string {
	var prefix string
	switch source {
	case "user":
		prefix = PrefixSkillsUser
	case "managed":
		prefix = PrefixSkillsManaged
	default:
		panic(fmt.Sprintf("blob: unknown skill source %q (want user or managed)", source))
	}
	rel = strings.TrimPrefix(rel, "/")
	return prefix + id + "/" + rel
}
