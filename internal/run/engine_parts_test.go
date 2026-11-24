package run

import (
	"testing"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

func TestBuildResumeMessagesWithParts(t *testing.T) {
	st := store.NewMemory()
	msgs := conversation.NewMemoryStore()
	e := &Engine{Store: st, Messages: msgs}
	runRec, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "look", ConversationID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := msgs.Append("c1", conversation.Message{
		Role: conversation.RoleUser, RunID: runRec.ID, Content: "look",
	}); err != nil {
		t.Fatal(err)
	}
	parts := []llm.ContentPart{
		{Type: "text", Text: "look"},
		{Type: "image", ImageMIME: "image/png", ImageBytes: []byte{1, 2, 3}},
	}
	out := e.buildResumeMessagesWithParts(runRec.ID, "sys", "c1", "look", nil, parts)
	for _, m := range out {
		if m.Role != llm.RoleUser || len(m.Parts) != len(parts) {
			continue
		}
		if m.Parts[1].Type == "image" && len(m.Parts[1].ImageBytes) == 3 {
			return
		}
	}
	t.Fatalf("no user message carried the image parts: %+v", out)
}
