package decide

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/llm"
)

// capturingProvider records the prompt and returns a fixed reply.
type capturingProvider struct {
	reply  llm.Message
	err    error
	prompt string
}

func (f *capturingProvider) Chat(_ context.Context, msgs []llm.Message, _ []llm.ToolSpec) (llm.Message, error) {
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(m.Content)
	}
	f.prompt = b.String()
	return f.reply, f.err
}

func (f *capturingProvider) SupportsVision() bool { return false }

func TestRemoteMultiParsesChosenNames(t *testing.T) {
	p := &capturingProvider{reply: llm.Message{Content: `["search_orders", "get_order"]`}}
	r := NewRemoteMulti(p)
	if !r.Enabled() {
		t.Fatal("remote multi with provider must be enabled")
	}
	ans, err := r.Ask(context.Background(), Question{
		Kind:    KindToolCandidates,
		Options: []string{"search_orders", "get_order", "delete_order"},
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if ans.Source != SourceRemote {
		t.Fatalf("source=%v want remote", ans.Source)
	}
	if len(ans.Values) != 2 || ans.Values[0] != "search_orders" || ans.Values[1] != "get_order" {
		t.Fatalf("values=%v want [search_orders get_order]", ans.Values)
	}
}

// Only names present in Options may come back; unknown names are dropped.
func TestRemoteMultiFiltersUnknownNames(t *testing.T) {
	p := &capturingProvider{reply: llm.Message{Content: `["search_orders", "hallucinated"]`}}
	r := NewRemoteMulti(p)
	ans, err := r.Ask(context.Background(), Question{
		Kind:    KindToolCandidates,
		Options: []string{"search_orders", "get_order"},
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(ans.Values) != 1 || ans.Values[0] != "search_orders" {
		t.Fatalf("values=%v want only [search_orders]", ans.Values)
	}
}

// The prompt must list the options by name.
func TestRemoteMultiPromptListsOptions(t *testing.T) {
	p := &capturingProvider{reply: llm.Message{Content: `[]`}}
	r := NewRemoteMulti(p)
	_, _ = r.Ask(context.Background(), Question{
		Kind:    KindToolCandidates,
		Options: []string{"alpha", "beta"},
	})
	if !strings.Contains(p.prompt, "alpha") || !strings.Contains(p.prompt, "beta") {
		t.Fatalf("prompt must list options: %q", p.prompt)
	}
}

func TestRemoteMultiBadJSONIsUnavailable(t *testing.T) {
	p := &capturingProvider{reply: llm.Message{Content: "我觉得这些都不错"}}
	r := NewRemoteMulti(p)
	_, err := r.Ask(context.Background(), Question{
		Kind: KindToolCandidates, Options: []string{"alpha"},
	})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err=%v want ErrUnavailable", err)
	}
}

func TestRemoteMultiChatErrorIsUnavailable(t *testing.T) {
	p := &capturingProvider{err: errors.New("network down")}
	r := NewRemoteMulti(p)
	_, err := r.Ask(context.Background(), Question{
		Kind: KindToolCandidates, Options: []string{"alpha"},
	})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err=%v want ErrUnavailable", err)
	}
}

func TestRemoteMultiNilProviderDisabled(t *testing.T) {
	r := NewRemoteMulti(nil)
	if r.Enabled() {
		t.Fatal("nil provider must be disabled")
	}
}

// Called without options it abstains rather than guessing.
func TestRemoteMultiNoOptionsUnavailable(t *testing.T) {
	p := &capturingProvider{reply: llm.Message{Content: `[]`}}
	r := NewRemoteMulti(p)
	_, err := r.Ask(context.Background(), Question{Kind: KindToolCandidates})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err=%v want ErrUnavailable", err)
	}
}
