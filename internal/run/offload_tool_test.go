package run

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/workspace"
)

func newMemWorkspace(t *testing.T) *workspace.Service {
	t.Helper()
	st, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return workspace.New(st)
}

// bulkyToolPayload returns a tool result whose estimated tokens exceed the
// default offload threshold (500).
func bulkyToolPayload() map[string]any {
	return map[string]any{"data": strings.Repeat("z", 2500)}
}

func TestToolResultOffloadsWhenBulky(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	reg := tool.NewRegistry()
	reg.RegisterSpec(llm.ToolSpec{Name: "bulky_tool"}, func(_ context.Context, _ map[string]any) (map[string]any, bool, error) {
		return bulkyToolPayload(), false, nil
	})

	var modelTool string
	turn := 0
	llmStub := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		if turn == 0 {
			turn++
			return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
				{ID: "c1", Name: "bulky_tool", Arguments: map[string]any{}},
			}}
		}
		for _, m := range msgs {
			if m.Role == llm.RoleTool {
				modelTool = m.Content
				if len(m.Parts) > 0 {
					modelTool = toolMessageText(m)
				}
			}
		}
		return llm.Message{Role: llm.RoleAssistant, Content: "done"}
	}}

	ws := newMemWorkspace(t)
	eng := &Engine{Store: st, LLM: llmStub, Tools: reg, Gate: NewGate(), Workspace: ws}

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", ConversationID: "conv1", Input: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "go"); err != nil {
		t.Fatal(err)
	}

	if !IsOffloadPreview(modelTool) {
		t.Fatalf("model tool content not offload preview: %q", modelTool)
	}
	if !strings.Contains(modelTool, "path: offload/") {
		t.Fatalf("preview missing offload path: %q", modelTool)
	}

	evs, _ := st.ListEvents(r.ID)
	var toolResult *store.Event
	var offloaded *store.Event
	for i := range evs {
		switch evs[i].Type {
		case EventToolResult:
			toolResult = &evs[i]
		case EventContextOffloaded:
			offloaded = &evs[i]
		}
	}
	if toolResult == nil {
		t.Fatal("missing tool.result event")
	}
	content, _ := toolResult.Data["content"].(map[string]any)
	data, _ := content["data"].(string)
	if data != strings.Repeat("z", 2500) {
		t.Fatalf("tool.result must keep full body, got len=%d", len(data))
	}
	if offloaded == nil {
		t.Fatal("missing context.offloaded event")
	}
	if offloaded.Data["source"] != "tool" {
		t.Fatalf("source=%v want tool", offloaded.Data["source"])
	}
	path, _ := offloaded.Data["path"].(string)
	if !strings.HasPrefix(path, "offload/") {
		t.Fatalf("path=%q", path)
	}
}

func TestToolResultOffloadFailOpen(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	reg := tool.NewRegistry()
	payload := bulkyToolPayload()
	reg.RegisterSpec(llm.ToolSpec{Name: "bulky_tool"}, func(_ context.Context, _ map[string]any) (map[string]any, bool, error) {
		return payload, false, nil
	})

	wantRaw, _ := json.Marshal(payload)
	var modelTool string
	turn := 0
	llmStub := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		if turn == 0 {
			turn++
			return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
				{ID: "c1", Name: "bulky_tool", Arguments: map[string]any{}},
			}}
		}
		for _, m := range msgs {
			if m.Role == llm.RoleTool {
				modelTool = m.Content
			}
		}
		return llm.Message{Role: llm.RoleAssistant, Content: "done"}
	}}

	// Workspace nil → fail-open: model still sees full JSON.
	eng := &Engine{Store: st, LLM: llmStub, Tools: reg, Gate: NewGate()}

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", ConversationID: "conv1", Input: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "go"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRun(r.ID)
	if err != nil || got.Status != store.StatusSucceeded {
		t.Fatalf("status=%v err=%v want succeeded", got.Status, err)
	}
	if modelTool != string(wantRaw) {
		t.Fatalf("fail-open model content=%q want full JSON", modelTool)
	}
	evs, _ := st.ListEvents(r.ID)
	for _, ev := range evs {
		if ev.Type == EventContextOffloaded {
			t.Fatalf("no offload event expected on fail-open; ev=%+v", ev)
		}
	}
}

func TestToolResultBelowThresholdUnchanged(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	reg := tool.NewRegistry()
	small := map[string]any{"ok": true}
	reg.RegisterSpec(llm.ToolSpec{Name: "tiny_tool"}, func(_ context.Context, _ map[string]any) (map[string]any, bool, error) {
		return small, false, nil
	})
	wantRaw, _ := json.Marshal(small)

	var modelTool string
	turn := 0
	llmStub := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		if turn == 0 {
			turn++
			return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
				{ID: "c1", Name: "tiny_tool", Arguments: map[string]any{}},
			}}
		}
		for _, m := range msgs {
			if m.Role == llm.RoleTool {
				modelTool = m.Content
			}
		}
		return llm.Message{Role: llm.RoleAssistant, Content: "done"}
	}}

	ws := newMemWorkspace(t)
	eng := &Engine{Store: st, LLM: llmStub, Tools: reg, Gate: NewGate(), Workspace: ws}

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", ConversationID: "conv1", Input: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "go"); err != nil {
		t.Fatal(err)
	}
	if modelTool != string(wantRaw) {
		t.Fatalf("small result must be unchanged, got %q", modelTool)
	}
	if IsOffloadPreview(modelTool) {
		t.Fatal("small result must not be offload preview")
	}
	evs, _ := st.ListEvents(r.ID)
	for _, ev := range evs {
		if ev.Type == EventContextOffloaded {
			t.Fatalf("no context.offloaded for small result; ev=%+v", ev)
		}
	}
}
