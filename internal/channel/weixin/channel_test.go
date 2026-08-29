package weixin

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

type fakeRuns struct {
	mu      sync.Mutex
	active  map[string]bool
	creates []store.CreateRunInput
}

func (f *fakeRuns) HasActiveRun(conversationID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active[conversationID], nil
}

func (f *fakeRuns) WaitingHumanRun(conversationID string) (*store.Run, error) {
	return nil, nil
}

func (f *fakeRuns) CreateRun(in store.CreateRunInput) (*store.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates = append(f.creates, in)
	return &store.Run{
		ID:             fmt.Sprintf("run-%d", len(f.creates)),
		AgentID:        in.AgentID,
		Input:          in.Input,
		ConversationID: in.ConversationID,
		Status:         store.StatusQueued,
	}, nil
}

func (f *fakeRuns) createCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.creates)
}

func newTestRuntime(t *testing.T, runs *fakeRuns) (*channel.Runtime, *conversation.MemoryStore) {
	t.Helper()
	meta := conversation.NewMemoryStore()
	rt := &channel.Runtime{
		Runs:           runs,
		Meta:           meta,
		Messages:       meta,
		Assignee:       "alice",
		DefaultAgentID: "agent-1",
	}
	return rt, meta
}

func waitUntil(t *testing.T, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timeout waiting for condition")
}

func TestSaveLoadCreds(t *testing.T) {
	dir := t.TempDir()
	if err := SaveCreds(dir, "bot@im.bot", "tok-abc"); err != nil {
		t.Fatalf("SaveCreds: %v", err)
	}
	accountID, token, err := LoadCreds(dir)
	if err != nil {
		t.Fatalf("LoadCreds: %v", err)
	}
	if accountID != "bot@im.bot" || token != "tok-abc" {
		t.Fatalf("got account=%q token=%q", accountID, token)
	}
}

func TestLoadCredsMissing(t *testing.T) {
	_, _, err := LoadCreds(t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing creds")
	}
}

func TestChannelName(t *testing.T) {
	ch := New(NewFake(), nil, "acc", "tok")
	if ch.Name() != "weixin" {
		t.Fatalf("Name=%q", ch.Name())
	}
}

func TestChannelPollTwoPeersEnsureMeta(t *testing.T) {
	fake := NewFake()
	fake.Updates = []Update{
		{PeerID: "peer-a", Text: "hi-a", ContextToken: "ctx-a"},
		{PeerID: "peer-b", Text: "hi-b", ContextToken: "ctx-b"},
	}
	runs := &fakeRuns{active: map[string]bool{}}
	rt, meta := newTestRuntime(t, runs)
	ch := New(fake, rt, "acc-1", "tok-1")
	ch.emptyPollWait = 5 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := ch.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
		defer stopCancel()
		_ = ch.Stop(stopCtx)
	}()

	waitUntil(t, 2*time.Second, func() bool { return runs.createCount() >= 2 })

	for _, peer := range []string{"peer-a", "peer-b"} {
		id := "weixin:acc-1:" + peer
		m, err := meta.GetMeta(id)
		if err != nil {
			t.Fatalf("GetMeta %s: %v", id, err)
		}
		if m.Source != "weixin" || m.ChannelPeer != peer || m.OwnerID != "alice" {
			t.Fatalf("meta %+v", m)
		}
	}
	if runs.createCount() != 2 {
		t.Fatalf("CreateRun=%d want 2", runs.createCount())
	}
}

func TestChannelBusySendsFixedText(t *testing.T) {
	fake := NewFake()
	fake.Updates = []Update{
		{PeerID: "peer-1", Text: "again", ContextToken: "ctx-1"},
	}
	runs := &fakeRuns{active: map[string]bool{"weixin:acc-1:peer-1": true}}
	rt, _ := newTestRuntime(t, runs)
	ch := New(fake, rt, "acc-1", "tok-1")
	ch.emptyPollWait = 5 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := ch.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
		defer stopCancel()
		_ = ch.Stop(stopCtx)
	}()

	waitUntil(t, 2*time.Second, func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return len(fake.Sent) >= 1
	})

	if runs.createCount() != 0 {
		t.Fatalf("CreateRun=%d want 0", runs.createCount())
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.Sent) != 1 {
		t.Fatalf("Sent=%+v", fake.Sent)
	}
	if fake.Sent[0].Text != channel.BusyReply || fake.Sent[0].ToUserID != "peer-1" {
		t.Fatalf("Sent[0]=%+v", fake.Sent[0])
	}
	if fake.Sent[0].ContextToken != "ctx-1" {
		t.Fatalf("context_token=%q", fake.Sent[0].ContextToken)
	}
}

func TestChannelMediaFillsInboundFiles(t *testing.T) {
	img := tinyPNG(t)
	fake := NewFake()
	fake.MediaBytes = img
	fake.Updates = []Update{{
		PeerID:       "peer-1",
		Text:         "看图",
		ContextToken: "ctx-m",
		Media: []MediaRef{{
			URL:      "http://cdn.example/img",
			FileName: "pic.png",
			MIME:     "image/png",
		}},
	}}

	runs := &fakeRuns{active: map[string]bool{}}
	rt, _ := newTestRuntime(t, runs)
	ch := New(fake, rt, "acc-1", "tok-1")
	ch.emptyPollWait = 5 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := ch.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
		defer stopCancel()
		_ = ch.Stop(stopCtx)
	}()

	waitUntil(t, 2*time.Second, func() bool { return runs.createCount() >= 1 })

	runs.mu.Lock()
	in := runs.creates[0]
	runs.mu.Unlock()
	if !strings.Contains(in.Input, "pic.png") {
		t.Fatalf("CreateRun input=%q want attachment pic.png (proves DownloadMedia bytes reached Inbound.Files)", in.Input)
	}
}

func TestChannelIgnoresGroupPeers(t *testing.T) {
	fake := NewFake()
	fake.Updates = []Update{
		{PeerID: "room@chatroom", Text: "群消息", ContextToken: "ctx-g"},
		{PeerID: "peer-dm", Text: "私聊", ContextToken: "ctx-d"},
	}
	runs := &fakeRuns{active: map[string]bool{}}
	rt, meta := newTestRuntime(t, runs)
	ch := New(fake, rt, "acc-1", "tok-1")
	ch.emptyPollWait = 5 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := ch.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
		defer stopCancel()
		_ = ch.Stop(stopCtx)
	}()

	waitUntil(t, 2*time.Second, func() bool { return runs.createCount() >= 1 })
	time.Sleep(50 * time.Millisecond)

	if runs.createCount() != 1 {
		t.Fatalf("CreateRun=%d want 1 (DM only)", runs.createCount())
	}
	if _, err := meta.GetMeta("weixin:acc-1:peer-dm"); err != nil {
		t.Fatalf("DM meta: %v", err)
	}
	if _, err := meta.GetMeta("weixin:acc-1:room@chatroom"); err == nil {
		t.Fatal("group peer should not get meta")
	}
}

func TestChannelSendTextUsesContextToken(t *testing.T) {
	fake := NewFake()
	ch := New(fake, nil, "acc", "tok")
	err := ch.SendText(context.Background(), "peer-1", "hello", map[string]string{
		"context_token": "ctx-out",
	})
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if len(fake.Sent) != 1 {
		t.Fatalf("Sent=%+v", fake.Sent)
	}
	if fake.Sent[0].ContextToken != "ctx-out" || fake.Sent[0].ToUserID != "peer-1" || fake.Sent[0].Text != "hello" {
		t.Fatalf("Sent[0]=%+v", fake.Sent[0])
	}
}

func TestChannelStopCancelsPoll(t *testing.T) {
	fake := NewFake()
	runs := &fakeRuns{active: map[string]bool{}}
	rt, _ := newTestRuntime(t, runs)
	ch := New(fake, rt, "acc", "tok")
	ch.emptyPollWait = 5 * time.Millisecond

	if err := ch.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := ch.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestRegisterChannelWeixin(t *testing.T) {
	names := channel.List()
	found := false
	for _, n := range names {
		if n == "weixin" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("weixin not registered; List=%v", names)
	}
}
