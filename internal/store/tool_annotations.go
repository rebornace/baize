package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

type toolAnnotationsJSON struct {
	ReadOnlyHint    *bool `json:"read_only_hint,omitempty"`
	DestructiveHint *bool `json:"destructive_hint,omitempty"`
}

func encodeToolAnnotations(t Tool) sql.NullString {
	if t.ReadOnlyHint == nil && t.DestructiveHint == nil {
		return sql.NullString{}
	}
	payload := toolAnnotationsJSON{
		ReadOnlyHint:    t.ReadOnlyHint,
		DestructiveHint: t.DestructiveHint,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(b), Valid: true}
}

func decodeToolAnnotations(raw sql.NullString, t *Tool) error {
	if !raw.Valid || raw.String == "" || raw.String == "null" {
		return nil
	}
	var payload toolAnnotationsJSON
	if err := json.Unmarshal([]byte(raw.String), &payload); err != nil {
		return fmt.Errorf("parse annotations_json: %w", err)
	}
	t.ReadOnlyHint = payload.ReadOnlyHint
	t.DestructiveHint = payload.DestructiveHint
	return nil
}
