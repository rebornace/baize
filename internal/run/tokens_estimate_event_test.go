package run

import (
	"testing"

	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

func findTokenEstimate(evs []store.Event) *store.Event {
	for i := range evs {
		if evs[i].Type == EventTokenEstimate {
			return &evs[i]
		}
	}
	return nil
}

func TestEmitTokenEstimateRecordsBias(t *testing.T) {
	st := store.NewMemory()
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st}

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "你好世界"}, // 4 CJK -> 4 + 4 overhead = 8
	}
	tools := []llm.ToolSpec{
		{
			Name:        "do_thing",
			Description: "run it",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "integer"},
				},
			},
		},
	}
	usage := llm.Usage{PromptTokens: 100, CompletionTokens: 5, TotalTokens: 105}
	eng.emitTokenEstimate(r.ID, 1, messages, tools, usage)

	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	ev := findTokenEstimate(evs)
	if ev == nil {
		t.Fatalf("missing %s event", EventTokenEstimate)
	}

	wantEst := EstimateMessagesTokens(messages) + EstimateToolsTokens(tools)
	if got := asInt(ev.Data, "estimated"); got != wantEst {
		t.Fatalf("estimated=%d want %d", got, wantEst)
	}
	// Tool projection must include the InputSchema (not just name+description).
	if asInt(ev.Data, "estimated_tools") <= EstimateTextTokens("do_thing")+
		EstimateTextTokens("run it")+perMessageTokens {
		t.Fatalf("estimated_tools should include InputSchema; data=%v", ev.Data)
	}
	if got := asInt(ev.Data, "real"); got != 100 {
		t.Fatalf("real=%d want 100", got)
	}
	if ratio, _ := ev.Data["ratio"].(float64); ratio >= 1 {
		t.Fatalf("ratio=%v want <1 for this arbitrary real value", ratio)
	}
}

func TestEmitTokenEstimateSkippedWithoutRealUsage(t *testing.T) {
	st := store.NewMemory()
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st}

	eng.emitTokenEstimate(r.ID, 1,
		[]llm.Message{{Role: llm.RoleUser, Content: "hi"}},
		nil,
		llm.Usage{TotalTokens: 0},
	)

	evs, _ := st.ListEvents(r.ID)
	if findTokenEstimate(evs) != nil {
		t.Fatalf("must not emit %s without real prompt usage", EventTokenEstimate)
	}
}
