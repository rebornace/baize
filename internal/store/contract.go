package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// ContractHash returns a stable 16-hex fingerprint of a tool's wire contract
// (name, method, path, operation_id, source, canonical input_schema).
func ContractHash(t Tool) string {
	h := sha256.New()
	writeField := func(s string) {
		_, _ = h.Write([]byte(s))
		_, _ = h.Write([]byte{0})
	}
	writeField(t.Name)
	writeField(t.Method)
	writeField(t.Path)
	writeField(t.OperationID)
	writeField(t.Source)
	schemaBytes := []byte("null")
	if t.InputSchema != nil {
		if b, err := json.Marshal(t.InputSchema); err == nil {
			var v any
			if err := json.Unmarshal(b, &v); err == nil {
				if cb, err := json.Marshal(v); err == nil {
					schemaBytes = cb
				} else {
					schemaBytes = b
				}
			} else {
				schemaBytes = b
			}
		}
	}
	_, _ = h.Write(schemaBytes)
	sum := hex.EncodeToString(h.Sum(nil))
	if len(sum) > 16 {
		return sum[:16]
	}
	return sum
}
