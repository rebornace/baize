package llm_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/llm"
)

func TestMockCreateTicket(t *testing.T) {
	p := llm.NewMock()
	msg, err := p.Chat(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "创建一个紧急工单：VPN 挂了"},
	}, []llm.ToolSpec{{Name: "create_ticket"}, {Name: "list_tickets"}})
	if err != nil || len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Name != "create_ticket" {
		t.Fatalf("%+v %v", msg, err)
	}
}

func TestMockSummarizeAfterTool(t *testing.T) {
	p := llm.NewMock()
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "创建工单"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "1", Name: "create_ticket"}}},
		{Role: llm.RoleTool, ToolCallID: "1", Content: `{"id":"t1"}`},
	}
	msg, err := p.Chat(context.Background(), msgs, nil)
	if err != nil || msg.Content == "" || len(msg.ToolCalls) != 0 {
		t.Fatalf("%+v %v", msg, err)
	}
}
