package channel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

type fakeChannel struct {
	mu        sync.Mutex
	name      string
	sentTexts []sentText
}

type sentText struct {
	peerID string
	text   string
	extras map[string]string
}

func (f *fakeChannel) Name() string { return f.name }

func (f *fakeChannel) Start(context.Context) error { return nil }

func (f *fakeChannel) Stop(context.Context) error { return nil }

func (f *fakeChannel) SendText(_ context.Context, peerID, text string, extras map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := map[string]string{}
	for k, v := range extras {
		cp[k] = v
	}
	f.sentTexts = append(f.sentTexts, sentText{peerID: peerID, text: text, extras: cp})
	return nil
}

func (f *fakeChannel) SendMedia(context.Context, string, string, string, []byte, map[string]string) error {
	return nil
}

func (f *fakeChannel) texts() []sentText {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]sentText, len(f.sentTexts))
	copy(out, f.sentTexts)
	return out
}

type fakeRuns struct {
	mu      sync.Mutex
	active  map[string]bool
	waiting map[string]*store.Run
	creates []store.CreateRunInput
	runs    []*store.Run
}

func (f *fakeRuns) HasActiveRun(conversationID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active[conversationID], nil
}

func (f *fakeRuns) WaitingHumanRun(conversationID string) (*store.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r := f.waiting[conversationID]; r != nil {
		cp := *r
		return &cp, nil
	}
	return nil, nil
}

func (f *fakeRuns) CreateRun(in store.CreateRunInput) (*store.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates = append(f.creates, in)
	r := &store.Run{
		ID:             fmt.Sprintf("run-%d", len(f.creates)),
		AgentID:        in.AgentID,
		Input:          in.Input,
		ConversationID: in.ConversationID,
		IdentityID:     in.IdentityID,
		Status:         store.StatusQueued,
	}
	f.runs = append(f.runs, r)
	return r, nil
}

func (f *fakeRuns) createCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.creates)
}

func (f *fakeRuns) lastCreate() store.CreateRunInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.creates[len(f.creates)-1]
}

func newTestRuntime(t *testing.T, runs *fakeRuns) (*Runtime, *conversation.MemoryStore) {
	t.Helper()
	meta := conversation.NewMemoryStore()
	rt := &Runtime{
		Runs:           runs,
		Meta:           meta,
		Messages:       meta,
		Assignee:       "alice",
		DefaultAgentID: "agent-1",
	}
	return rt, meta
}

func TestHandleInboundCreatesMetaAndRun(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{}}
	rt, meta := newTestRuntime(t, runs)
	var startCalls int
	rt.AfterCreateRun = func(ctx context.Context, run *store.Run, userParts []llm.ContentPart) error {
		startCalls++
		if run == nil || run.ID == "" {
			t.Fatalf("AfterCreateRun got nil/empty run")
		}
		return nil
	}
	ch := &fakeChannel{name: "fake"}

	err := rt.HandleInbound(context.Background(), ch, Inbound{
		PeerID: "peer-1",
		Text:   "你好",
		Extras: map[string]string{"account": "acc-1"},
	})
	if err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}

	wantID := "weixin:acc-1:peer-1"
	m, err := meta.GetMeta(wantID)
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if m.OwnerID != "alice" || m.Source != "weixin" || m.ChannelPeer != "peer-1" {
		t.Fatalf("meta = %+v, want owner=alice source=weixin peer=peer-1", m)
	}
	if runs.createCount() != 1 {
		t.Fatalf("CreateRun calls = %d, want 1", runs.createCount())
	}
	in := runs.lastCreate()
	if in.AgentID != "agent-1" || in.ConversationID != wantID || in.Input != "你好" {
		t.Fatalf("CreateRun input = %+v", in)
	}
	if startCalls != 1 {
		t.Fatalf("AfterCreateRun calls = %d, want 1", startCalls)
	}
	msgs := meta.List(wantID)
	if len(msgs) != 1 || msgs[0].Role != conversation.RoleUser || msgs[0].Content != "你好" {
		t.Fatalf("messages = %+v", msgs)
	}
	if len(ch.texts()) != 0 {
		t.Fatalf("unexpected SendText: %+v", ch.texts())
	}
}

func TestHandleInboundBusySendsFixedText(t *testing.T) {
	convID := "weixin:acc-1:peer-1"
	runs := &fakeRuns{active: map[string]bool{convID: true}}
	rt, _ := newTestRuntime(t, runs)
	var startCalls int
	rt.AfterCreateRun = func(context.Context, *store.Run, []llm.ContentPart) error {
		startCalls++
		return nil
	}
	ch := &fakeChannel{name: "fake"}

	err := rt.HandleInbound(context.Background(), ch, Inbound{
		PeerID: "peer-1",
		Text:   "又一条",
		Extras: map[string]string{"account": "acc-1"},
	})
	if err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}
	if runs.createCount() != 0 {
		t.Fatalf("CreateRun called %d times, want 0", runs.createCount())
	}
	if startCalls != 0 {
		t.Fatalf("AfterCreateRun called %d times, want 0", startCalls)
	}
	sent := ch.texts()
	if len(sent) != 1 {
		t.Fatalf("SendText calls = %d, want 1", len(sent))
	}
	if sent[0].peerID != "peer-1" || sent[0].text != BusyReply {
		t.Fatalf("SendText = %+v, want peer-1 / %q", sent[0], BusyReply)
	}
	if BusyReply != "请稍候，上一轮还在处理" {
		t.Fatalf("BusyReply drifted: %q", BusyReply)
	}
}

func TestHandleInboundWaitingHumanApprove(t *testing.T) {
	convID := "weixin:acc-1:peer-1"
	waiting := &store.Run{ID: "run-hitl", ConversationID: convID, Status: store.StatusWaitingHuman}
	runs := &fakeRuns{
		active:  map[string]bool{convID: true},
		waiting: map[string]*store.Run{convID: waiting},
	}
	rt, _ := newTestRuntime(t, runs)
	var resumed struct {
		runID   string
		approve bool
	}
	rt.ResumeHITL = func(_ context.Context, runID string, approve bool, _ string) error {
		resumed.runID = runID
		resumed.approve = approve
		return nil
	}
	ch := &fakeChannel{name: "fake"}
	if err := rt.HandleInbound(context.Background(), ch, Inbound{
		PeerID: "peer-1",
		Text:   "批准",
		Extras: map[string]string{"account": "acc-1", "context_token": "tok"},
	}); err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}
	if resumed.runID != "run-hitl" || !resumed.approve {
		t.Fatalf("ResumeHITL = %+v", resumed)
	}
	if runs.createCount() != 0 {
		t.Fatal("CreateRun should not run during HITL resume")
	}
	sent := ch.texts()
	if len(sent) != 1 || !strings.Contains(sent[0].text, "已批准") {
		t.Fatalf("SendText = %+v", sent)
	}
}

func TestHandleInboundWaitingHumanHelp(t *testing.T) {
	convID := "weixin:acc-1:peer-1"
	runs := &fakeRuns{
		active:  map[string]bool{convID: true},
		waiting: map[string]*store.Run{convID: {ID: "run-hitl", Status: store.StatusWaitingHuman}},
	}
	rt, _ := newTestRuntime(t, runs)
	ch := &fakeChannel{name: "fake"}
	if err := rt.HandleInbound(context.Background(), ch, Inbound{
		PeerID: "peer-1",
		Text:   "随便说说",
		Extras: map[string]string{"account": "acc-1"},
	}); err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}
	sent := ch.texts()
	if len(sent) != 1 || sent[0].text != HITLHelpReply {
		t.Fatalf("SendText = %+v, want %q", sent, HITLHelpReply)
	}
}

func TestHandleInboundRequiresAssigneeAccountPeer(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{}}
	rt, _ := newTestRuntime(t, runs)
	ch := &fakeChannel{name: "fake"}
	ctx := context.Background()

	rt.Assignee = ""
	if err := rt.HandleInbound(ctx, ch, Inbound{PeerID: "p", Extras: map[string]string{"account": "a"}}); !errors.Is(err, ErrNoAssignee) {
		t.Fatalf("want ErrNoAssignee, got %v", err)
	}
	rt.Assignee = "alice"
	if err := rt.HandleInbound(ctx, ch, Inbound{PeerID: "", Extras: map[string]string{"account": "a"}}); !errors.Is(err, ErrNoPeer) {
		t.Fatalf("want ErrNoPeer, got %v", err)
	}
	if err := rt.HandleInbound(ctx, ch, Inbound{PeerID: "p", Extras: nil}); !errors.Is(err, ErrNoAccount) {
		t.Fatalf("want ErrNoAccount, got %v", err)
	}
	if runs.createCount() != 0 {
		t.Fatalf("CreateRun should not be called on validation errors")
	}
}

func TestHandleInboundAttachmentUserParts(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{}}
	rt, _ := newTestRuntime(t, runs)
	var gotParts []llm.ContentPart
	rt.AfterCreateRun = func(_ context.Context, _ *store.Run, parts []llm.ContentPart) error {
		gotParts = parts
		return nil
	}
	ch := &fakeChannel{name: "fake"}
	err := rt.HandleInbound(context.Background(), ch, Inbound{
		PeerID: "peer-1",
		Text:   "看附件",
		Extras: map[string]string{"account": "acc-1"},
		Files: []InboundFile{{
			Name: "note.txt",
			MIME: "text/plain",
			Data: []byte("hello from file"),
		}},
	})
	if err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}
	in := runs.lastCreate()
	if !strings.Contains(in.Input, "（附件：note.txt）") {
		t.Fatalf("display input = %q", in.Input)
	}
	if len(gotParts) == 0 {
		t.Fatal("expected user parts for text attachment")
	}
	joined := ""
	for _, p := range gotParts {
		joined += p.Text
	}
	if !strings.Contains(joined, "hello from file") || !strings.Contains(joined, "【附件: note.txt】") {
		t.Fatalf("user parts text = %q", joined)
	}
}

func TestRegisterOpenList(t *testing.T) {
	resetRegistryForTest()
	t.Cleanup(resetRegistryForTest)

	RegisterChannel("fake", func(cfg Config) (Channel, error) {
		return &fakeChannel{name: "fake"}, nil
	})
	names := List()
	if len(names) != 1 || names[0] != "fake" {
		t.Fatalf("List = %v", names)
	}
	ch, err := Open("fake", Config{"k": "v"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if ch.Name() != "fake" {
		t.Fatalf("Name = %q", ch.Name())
	}
	if _, err := Open("missing", nil); err == nil {
		t.Fatal("expected error for unknown channel")
	}
}

func TestHandleInboundMetaUpdatedAt(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{}}
	rt, meta := newTestRuntime(t, runs)
	ch := &fakeChannel{name: "fake"}
	before := time.Now().UTC().Add(-time.Second)
	if err := rt.HandleInbound(context.Background(), ch, Inbound{
		PeerID: "p",
		Text:   "hi",
		Extras: map[string]string{"account": "a"},
	}); err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}
	m, err := meta.GetMeta("weixin:a:p")
	if err != nil {
		t.Fatal(err)
	}
	if m.UpdatedAt.Before(before) {
		t.Fatalf("UpdatedAt not set: %v", m.UpdatedAt)
	}
}
