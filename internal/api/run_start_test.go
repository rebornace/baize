package api

import (
	"strconv"
	"strings"
	"testing"
)

func TestValidateInputArgs(t *testing.T) {
	if err := validateInputArgs(nil); err != nil {
		t.Fatalf("nil: %v", err)
	}
	if err := validateInputArgs(map[string]any{"project_id": "p-42"}); err != nil {
		t.Fatalf("valid: %v", err)
	}

	many := map[string]any{}
	for i := 0; i <= maxInputArgsKeys; i++ {
		many["k"+strconv.Itoa(i)] = i
	}
	if err := validateInputArgs(many); err == nil || !strings.Contains(err.Error(), "too many keys") {
		t.Fatalf("many keys: %v", err)
	}

	if err := validateInputArgs(map[string]any{"blob": strings.Repeat("x", maxInputArgsBytes+1)}); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversize: %v", err)
	}
}
