package llm

import (
	"context"
	"errors"
	"testing"
)

// choiceProvider implements both Chooser and StreamChooser, recording the
// tool_choice it was asked to enforce.
type choiceProvider struct {
	gotChoice ToolChoice
}

func (c *choiceProvider) Chat(_ context.Context, _ []Message, _ []ToolSpec) (Message, error) {
	return Message{Content: "plain"}, nil
}
func (c *choiceProvider) SupportsVision() bool { return false }
func (c *choiceProvider) ChatWithChoice(_ context.Context, _ []Message, _ []ToolSpec, choice ToolChoice) (Message, error) {
	c.gotChoice = choice
	return Message{Content: "chosen"}, nil
}
func (c *choiceProvider) ChatStreamWithChoice(
	_ context.Context, _ []Message, _ []ToolSpec, choice ToolChoice, _, _ func(string),
) (Message, error) {
	c.gotChoice = choice
	return Message{Content: "chosen-stream"}, nil
}

func newChoiceSwitch(p Provider) *Switch {
	src := &fakeProfileSource{
		list: []ModelProfileView{{ID: "mp_def", Model: "model-def"}},
	}
	sw := NewSwitch(src)
	sw.build = func(_ ModelProfileView) Provider { return p }
	return sw
}

func TestSwitchChatWithChoiceForwards(t *testing.T) {
	cp := &choiceProvider{}
	sw := newChoiceSwitch(cp)
	msg, err := sw.ChatWithChoice(context.Background(), nil, nil, ToolChoice{Type: ToolChoiceRequired})
	if err != nil {
		t.Fatalf("chat with choice: %v", err)
	}
	if msg.Content != "chosen" || cp.gotChoice.Type != ToolChoiceRequired {
		t.Fatalf("choice not forwarded: %+v %+v", msg, cp.gotChoice)
	}
}

func TestSwitchChatStreamWithChoiceForwards(t *testing.T) {
	cp := &choiceProvider{}
	sw := newChoiceSwitch(cp)
	msg, err := sw.ChatStreamWithChoice(context.Background(), nil, nil,
		ToolChoice{Type: ToolChoiceRequired}, nil, nil)
	if err != nil {
		t.Fatalf("stream with choice: %v", err)
	}
	if msg.Content != "chosen-stream" {
		t.Fatalf("got %q", msg.Content)
	}
}

// When the backing provider lacks Chooser, the constrained call must error
// rather than silently run unconstrained.
func TestSwitchChatWithChoiceUnsupportedErrors(t *testing.T) {
	sw := newChoiceSwitch(&recordingProvider{model: "x"})
	if _, err := sw.ChatWithChoice(context.Background(), nil, nil, ToolChoice{Type: ToolChoiceRequired}); !errors.Is(err, errChoiceUnsupported) {
		t.Fatalf("want choice-unsupported error, got %v", err)
	}
	if _, err := sw.ChatStreamWithChoice(context.Background(), nil, nil,
		ToolChoice{Type: ToolChoiceRequired}, nil, nil); !errors.Is(err, errChoiceUnsupported) {
		t.Fatalf("want choice-unsupported error, got %v", err)
	}
}
