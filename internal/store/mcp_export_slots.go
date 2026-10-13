package store

import (
	"encoding/json"
	"strings"
)

// NormalizeMCPExportKey ensures IdentitySlots is populated from legacy IdentityID
// when empty, and keeps IdentityID aligned with the default (empty connector) slot.
func NormalizeMCPExportKey(k *MCPExportKey) {
	if k == nil {
		return
	}
	if len(k.IdentitySlots) == 0 {
		if id := strings.TrimSpace(k.IdentityID); id != "" {
			k.IdentitySlots = []MCPExportIdentitySlot{{IdentityID: id}}
		}
		return
	}
	k.IdentitySlots = cloneIdentitySlots(k.IdentitySlots)
	if def := DefaultSlotIdentityID(*k); def != "" {
		k.IdentityID = def
	}
}

// DefaultSlotIdentityID returns the identity bound to the default slot
// (empty connector_id), falling back to IdentityID.
func DefaultSlotIdentityID(k MCPExportKey) string {
	for _, slot := range k.IdentitySlots {
		if strings.TrimSpace(slot.ConnectorID) == "" && strings.TrimSpace(slot.IdentityID) != "" {
			return strings.TrimSpace(slot.IdentityID)
		}
	}
	return strings.TrimSpace(k.IdentityID)
}

// ResolveExportIdentityID selects the identity for a connector/role.
// Match order: exact connector+role → connector with empty role → default slot.
func ResolveExportIdentityID(k MCPExportKey, connectorID, role string) (string, bool) {
	NormalizeMCPExportKey(&k)
	connectorID = strings.TrimSpace(connectorID)
	role = strings.TrimSpace(role)

	if connectorID != "" {
		if role != "" {
			for _, slot := range k.IdentitySlots {
				if strings.TrimSpace(slot.ConnectorID) == connectorID &&
					strings.TrimSpace(slot.Role) == role &&
					strings.TrimSpace(slot.IdentityID) != "" {
					return strings.TrimSpace(slot.IdentityID), true
				}
			}
		}
		for _, slot := range k.IdentitySlots {
			if strings.TrimSpace(slot.ConnectorID) == connectorID &&
				strings.TrimSpace(slot.Role) == "" &&
				strings.TrimSpace(slot.IdentityID) != "" {
				return strings.TrimSpace(slot.IdentityID), true
			}
		}
	}
	if id := DefaultSlotIdentityID(k); id != "" {
		return id, true
	}
	return "", false
}

func cloneIdentitySlots(slots []MCPExportIdentitySlot) []MCPExportIdentitySlot {
	if slots == nil {
		return nil
	}
	out := make([]MCPExportIdentitySlot, len(slots))
	copy(out, slots)
	return out
}

func encodeIdentitySlotsJSON(slots []MCPExportIdentitySlot) ([]byte, error) {
	if len(slots) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(slots)
}

func decodeIdentitySlotsJSON(raw []byte) ([]MCPExportIdentitySlot, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var out []MCPExportIdentitySlot
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
