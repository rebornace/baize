package decide

import (
	"context"
	"errors"
	"testing"

	"github.com/rebornace/baize/internal/llm"
)

// fakeProvider is a minimal llm.Provider returning a fixed reply.
type fakeProvider struct {
	reply llm.Message
	err   error
}

func (f *fakeProvider) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSpec) (llm.Message, error) {
	return f.reply, f.err
}

func (f *fakeProvider) SupportsVision() bool { return false }

func TestRemoteYes(t *testing.T) {
	p := &fakeProvider{reply: llm.Message{Content: "yes"}}
	r := NewRemote(p)
	if !r.Enabled() {
		t.Fatal("remote with provider must be enabled")
	}
	ans, err := r.Ask(context.Background(), Question{Kind: KindMemoryExtract, Context: "ctx"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if ans.Verdict != VerdictYes || ans.Source != SourceRemote {
		t.Fatalf("ans=%+v want yes/remote", ans)
	}
}

func TestRemoteParsesNoFromProse(t *testing.T) {
	p := &fakeProvider{reply: llm.Message{Content: "答案：NO。"}}
	r := NewRemote(p)
	ans, err := r.Ask(context.Background(), Question{Kind: KindMemoryExtract})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if ans.Verdict != VerdictNo {
		t.Fatalf("verdict=%v want no", ans.Verdict)
	}
}

func TestRemoteGarbageIsUnavailable(t *testing.T) {
	p := &fakeProvider{reply: llm.Message{Content: "我不太确定，也许吧"}}
	r := NewRemote(p)
	_, err := r.Ask(context.Background(), Question{Kind: KindMemoryExtract})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err=%v want ErrUnavailable", err)
	}
}

func TestRemoteChatErrorIsUnavailable(t *testing.T) {
	p := &fakeProvider{err: errors.New("network down")}
	r := NewRemote(p)
	_, err := r.Ask(context.Background(), Question{Kind: KindMemoryExtract})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err=%v want ErrUnavailable", err)
	}
}

func TestRemoteNilProviderDisabled(t *testing.T) {
	r := NewRemote(nil)
	if r.Enabled() {
		t.Fatal("remote with nil provider must be disabled")
	}
}
