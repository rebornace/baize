package llm

import (
	"encoding/json"
	"testing"
)

func TestToolChoiceMarshalJSON(t *testing.T) {
	cases := []struct {
		c    ToolChoice
		want string
	}{
		{ToolChoice{Type: ToolChoiceAuto}, `"auto"`},
		{ToolChoice{Type: ToolChoiceRequired}, `"required"`},
		{ToolChoice{Type: ToolChoiceNone}, `"none"`},
		{ToolChoice{Type: ToolChoiceNamed, Name: "foo"}, `{"type":"function","function":{"name":"foo"}}`},
		// A named choice without a name is invalid; fall back to the type string.
		{ToolChoice{Type: ToolChoiceNamed}, `"tool"`},
	}
	for _, tc := range cases {
		b, err := json.Marshal(tc.c)
		if err != nil {
			t.Fatalf("marshal %+v: %v", tc.c, err)
		}
		if string(b) != tc.want {
			t.Fatalf("got %s want %s", b, tc.want)
		}
	}
}
