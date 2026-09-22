package llm

import (
	"context"
	"encoding/json"
)

// ToolChoiceType selects the tool_choice discipline sent to the model
// (DP-2b). It maps 1:1 to the OpenAI chat/completions tool_choice field.
type ToolChoiceType string

const (
	// ToolChoiceAuto lets the model decide freely whether / which tool to
	// call. This is today's behavior; sending it adds no constraint.
	ToolChoiceAuto ToolChoiceType = "auto"
	// ToolChoiceRequired forces the model to call one of the supplied tools.
	ToolChoiceRequired ToolChoiceType = "required"
	// ToolChoiceNamed forces a call to one specific tool (ToolChoice.Name).
	ToolChoiceNamed ToolChoiceType = "tool"
	// ToolChoiceNone forbids tool calls.
	ToolChoiceNone ToolChoiceType = "none"
)

// ToolChoice is the DP-2b enum constraint. Name is used only when Type is
// ToolChoiceNamed.
type ToolChoice struct {
	Type ToolChoiceType
	Name string
}

// MarshalJSON renders the OpenAI wire shape: the simple disciplines are a bare
// string ("auto"/"required"/"none"), while a named choice is the object
// {"type":"function","function":{"name":...}}.
func (c ToolChoice) MarshalJSON() ([]byte, error) {
	if c.Type == ToolChoiceNamed && c.Name != "" {
		return json.Marshal(struct {
			Type     string `json:"type"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		}{
			Type: "function",
			Function: struct {
				Name string `json:"name"`
			}{Name: c.Name},
		})
	}
	return json.Marshal(c.Type)
}

// Chooser is an OPTIONAL Provider capability (DP-2b): it runs a chat call
// with a tool_choice enum constraint. Providers that do not implement it are
// unaffected and callers must fall back to Chat/ChatStream.
//
// The optional-interface shape (instead of a 4th Chat parameter) keeps the
// Provider/Streamer signatures — and every existing implementation and test
// fake — unchanged. See BA-ADR-005.
type Chooser interface {
	ChatWithChoice(ctx context.Context, messages []Message, tools []ToolSpec, choice ToolChoice) (Message, error)
}

// StreamChooser is the streaming counterpart of Chooser. Kept separate so a
// provider can support constrained non-streaming without streaming or vice
// versa.
type StreamChooser interface {
	ChatStreamWithChoice(
		ctx context.Context,
		messages []Message,
		tools []ToolSpec,
		choice ToolChoice,
		onThink, onContent func(cumulative string),
	) (Message, error)
}
