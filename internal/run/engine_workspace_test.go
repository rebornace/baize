package run

import (
	"context"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// visionStubLLM returns one read_image tool call then a final message, and
// captures every Chat call's messages.
type visionStubLLM struct {
	captured [][]llm.Message
	calls    int
}

func (s *visionStubLLM) SupportsVision() bool { return true }
func (s *visionStubLLM) Chat(_ context.Context, msgs []llm.Message, _ []llm.ToolSpec) (llm.Message, error) {
	s.captured = append(s.captured, msgs)
	s.calls++
	if s.calls == 1 {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
			ID: "call_1", Name: "read_image", Arguments: map[string]any{"path": "uploads/s.png"},
		}}}, nil
	}
	return llm.Message{Role: llm.RoleAssistant, Content: "seen it"}, nil
}

func newWorkspaceEngine(st store.Store, llm_ llm.Provider) (*Engine, *tool.Registry) {
	reg := tool.NewRegistry()
	eng := &Engine{Store: st, LLM: llm_, Tools: reg, Gate: NewGate()}
	return eng, reg
}

func TestToolImagePartEncodedIntoToolMessage(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	llmStub := &visionStubLLM{}
	eng, reg := newWorkspaceEngine(st, llmStub)

	img := llm.ContentPart{Type: "image", ImageMIME: "image/png", ImageBytes: []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A}}
	reg.RegisterSpecApproved(llm.ToolSpec{Name: "read_image"},
		func(_ context.Context, _ map[string]any) (map[string]any, bool, error) {
			return tool.WithImageParts(
				map[string]any{"path": "uploads/s.png", "note": "image attached"},
				tool.ImageResult{Path: "uploads/s.png", Part: img}), false, nil
		}, false)

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", ConversationID: "conv1", Input: "look"})
	if err != nil {
		t.Fatal(err)
	}
	ag := agent.Def{ID: "a", System: "sys"}
	if err := eng.Execute(context.Background(), r.ID, ag, "look"); err != nil {
		t.Fatal(err)
	}

	// 2nd Chat call: the tool message must carry an image part.
	if len(llmStub.captured) < 2 {
		t.Fatalf("expected >=2 Chat calls, got %d", llmStub.calls)
	}
	var foundImage bool
	for _, m := range llmStub.captured[1] {
		if m.Role == llm.RoleTool {
			for _, p := range m.Parts {
				if p.Type == "image" {
					foundImage = true
				}
			}
		}
	}
	if !foundImage {
		t.Fatalf("tool message did not carry an image part: %+v", llmStub.captured[1])
	}

	// Persisted ToolResult event: no reserved key leak; image_refs present.
	evs, _ := st.ListEvents(r.ID)
	sawRefs := false
	for _, ev := range evs {
		if ev.Type != EventToolResult {
			continue
		}
		content, _ := ev.Data["content"].(map[string]any)
		if _, leak := content["__baize_image_parts__"]; leak {
			t.Fatalf("image parts leaked into event content: %v", content)
		}
		refs, ok := ev.Data["image_refs"].([]map[string]any)
		if ok && len(refs) > 0 && refs[0]["workspace_path"] == "uploads/s.png" {
			sawRefs = true
		}
	}
	if !sawRefs {
		t.Fatalf("expected image_refs with workspace_path on tool.result event")
	}
}

func imageRefEvent() []store.Event {
	return []store.Event{{
		Type: EventToolResult,
		Data: map[string]any{
			"tool_call_id": "c1",
			"name":         "read_image",
			"content":      map[string]any{"path": "uploads/s.png"},
			"image_refs":   []map[string]any{{"workspace_path": "uploads/s.png"}},
		},
	}}
}

func TestColdResumeRebuildsImageFromResolver(t *testing.T) {
	eng, _ := newWorkspaceEngine(store.NewMemory(), &visionStubLLM{})
	eng.ImagePartResolver = func(_, _ string) (llm.ContentPart, bool) {
		return llm.ContentPart{Type: "image", ImageMIME: "image/png", ImageBytes: []byte("x")}, true
	}
	msgs := eng.eventsAfterInput(imageRefEvent(), "conv1")
	var gotImage bool
	for _, m := range msgs {
		if m.Role == llm.RoleTool {
			for _, p := range m.Parts {
				if p.Type == "image" {
					gotImage = true
				}
			}
		}
	}
	if !gotImage {
		t.Fatalf("cold resume did not rebuild image part: %+v", msgs)
	}
}

func TestColdResumeDegradesToTextWhenResolverFails(t *testing.T) {
	eng, _ := newWorkspaceEngine(store.NewMemory(), &visionStubLLM{})
	eng.ImagePartResolver = func(_, _ string) (llm.ContentPart, bool) { return llm.ContentPart{}, false }
	msgs := eng.eventsAfterInput(imageRefEvent(), "conv1")
	var toolMsg *llm.Message
	for i := range msgs {
		if msgs[i].Role == llm.RoleTool {
			toolMsg = &msgs[i]
		}
	}
	if toolMsg == nil {
		t.Fatal("no tool message")
	}
	if len(toolMsg.Parts) != 0 || !strings.Contains(toolMsg.Content, "read_image") {
		t.Fatalf("expected text degradation mentioning read_image, got %+v", toolMsg)
	}
}
