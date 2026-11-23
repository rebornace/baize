package channel

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

const mergeConvID = "weixin:acc-1:peer-1"

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func sendText(t *testing.T, rt *Runtime, ch Channel, text string) {
	t.Helper()
	if err := rt.HandleInbound(context.Background(), ch, Inbound{
		PeerID: "peer-1",
		Text:   text,
		Extras: map[string]string{"account": "acc-1"},
	}); err != nil {
		t.Fatalf("HandleInbound(%q): %v", text, err)
	}
}

func countSent(ch *fakeChannel, text string) int {
	n := 0
	for _, sent := range ch.texts() {
		if sent.text == text {
			n++
		}
	}
	return n
}

func TestBusyTextMergesIntoOneFollowUpRun(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{mergeConvID: true}}
	rt, _ := newTestRuntime(t, runs)
	tick := make(chan time.Time, 1)
	rt.drainTick = tick
	ch := &fakeChannel{name: "fake"}
	var starts atomic.Int32
	rt.AfterCreateRun = func(context.Context, *store.Run, []llm.ContentPart) error {
		starts.Add(1)
		return nil
	}

	sendText(t, rt, ch, "第一条")
	sendText(t, rt, ch, "第二条")

	if got := countSent(ch, QueuedReply); got != 1 {
		t.Fatalf("queued acks=%d want 1 (sent=%+v)", got, ch.texts())
	}
	if got := countSent(ch, BusyReply); got != 0 {
		t.Fatalf("busy replies=%d want 0", got)
	}
	if runs.createCount() != 0 || starts.Load() != 0 {
		t.Fatalf("must not run while busy: creates=%d starts=%d", runs.createCount(), starts.Load())
	}

	runs.setActive(mergeConvID, false)
	tick <- time.Now()
	waitFor(t, "merged run", func() bool { return runs.createCount() == 1 })
	if got := runs.lastCreate().Input; got != "第一条\n\n第二条" {
		t.Fatalf("merged input=%q", got)
	}
	waitFor(t, "engine start", func() bool { return starts.Load() == 1 })
}

func TestSweepWaitsWhileConversationIsBusy(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{mergeConvID: true}}
	rt, _ := newTestRuntime(t, runs)
	tick := make(chan time.Time, 1)
	rt.drainTick = tick

	sendText(t, rt, &fakeChannel{name: "fake"}, "排队")
	tick <- time.Now()
	time.Sleep(50 * time.Millisecond)
	if runs.createCount() != 0 {
		t.Fatalf("created a run while still busy: %d", runs.createCount())
	}

	runs.setActive(mergeConvID, false)
	tick <- time.Now()
	waitFor(t, "run after idle", func() bool { return runs.createCount() == 1 })
	if got := runs.lastCreate().Input; got != "排队" {
		t.Fatalf("input=%q", got)
	}
}

func TestIdleWithPendingQueuesNewText(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{}}
	rt, _ := newTestRuntime(t, runs)
	ch := &fakeChannel{name: "fake"}
	// Pretend a sweeper is running with a claimed-looking pending batch so a
	// new idle message must append instead of starting its own run.
	rt.queueMu.Lock()
	rt.pending = map[string]*pendingInbound{mergeConvID: {
		peerID: "peer-1",
		ch:     ch,
		extras: map[string]string{"account": "acc-1"},
		text:   "先到",
	}}
	rt.sweeping = true
	rt.queueMu.Unlock()

	sendText(t, rt, ch, "后到")

	if runs.createCount() != 0 {
		t.Fatalf("must append to pending, created %d runs", runs.createCount())
	}
	if got := countSent(ch, QueuedReply); got != 0 {
		t.Fatalf("appended text must be quiet, got %d acks", got)
	}
	rt.queueMu.Lock()
	got := rt.pending[mergeConvID].text
	rt.queueMu.Unlock()
	if got != "先到\n\n后到" {
		t.Fatalf("pending text=%q", got)
	}
}

func TestBusyWaitingHumanDoesNotMerge(t *testing.T) {
	waiting := &store.Run{ID: "run-hitl", ConversationID: mergeConvID, Status: store.StatusWaitingHuman}
	runs := &fakeRuns{
		active:  map[string]bool{mergeConvID: true},
		waiting: map[string]*store.Run{mergeConvID: waiting},
	}
	rt, _ := newTestRuntime(t, runs)
	var resumed atomic.Bool
	rt.ResumeHITL = func(context.Context, string, bool, string) error {
		resumed.Store(true)
		return nil
	}
	ch := &fakeChannel{name: "fake"}

	sendText(t, rt, ch, "再加一条")
	if got := countSent(ch, HITLHelpReply); got != 1 {
		t.Fatalf("HITL help=%d want 1 (sent=%+v)", got, ch.texts())
	}
	if rt.hasQueued(mergeConvID) {
		t.Fatal("text must not be queued while waiting for approval")
	}
	sendText(t, rt, ch, "同意")
	if !resumed.Load() {
		t.Fatal("approval must still resume the waiting run")
	}
	if rt.hasQueued(mergeConvID) {
		t.Fatal("approval must not enqueue a merged batch")
	}
}

func TestQueueAppendPreservesOrderAndTruncates(t *testing.T) {
	p := &pendingInbound{}
	p.appendText("  第一条  ")
	p.appendText("第二条")
	if p.text != "第一条\n\n第二条" {
		t.Fatalf("text=%q", p.text)
	}

	big := &pendingInbound{}
	big.appendText(strings.Repeat("甲", maxQueuedRunes+50))
	if !big.truncated {
		t.Fatal("expected truncation")
	}
	if got := utf8.RuneCountInString(big.text); got > maxQueuedRunes+utf8.RuneCountInString(queueTruncatedNote) {
		t.Fatalf("truncated text too long: %d runes", got)
	}
	big.appendText("新增内容")
	if strings.Contains(big.text, "新增内容") {
		t.Fatalf("appends after truncation must be ignored: %q", big.text)
	}
}

type failingMeta struct {
	conversation.MetaStore
}

func (f failingMeta) EnsureMeta(conversation.Meta) error { return errors.New("meta down") }

func TestSweepErrorRestoresBatchAndRetires(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{mergeConvID: true}}
	rt, meta := newTestRuntime(t, runs)
	tick := make(chan time.Time, 1)
	rt.drainTick = tick

	sendText(t, rt, &fakeChannel{name: "fake"}, "待处理")
	// Fail only the drain attempt: the initial queue above must succeed.
	rt.Meta = failingMeta{MetaStore: meta}
	runs.setActive(mergeConvID, false)
	tick <- time.Now()

	waitFor(t, "restored batch", func() bool {
		rt.queueMu.Lock()
		defer rt.queueMu.Unlock()
		p := rt.pending[mergeConvID]
		return !rt.sweeping && p != nil && strings.Contains(p.text, "待处理")
	})
	if runs.createCount() != 0 {
		t.Fatalf("failed start must not create a run, got %d", runs.createCount())
	}
}

type ackFailChannel struct {
	*fakeChannel
}

func (c ackFailChannel) SendText(context.Context, string, string, map[string]string) error {
	return errors.New("ack send down")
}

func newStoreRuntime(t *testing.T) (*Runtime, *store.Memory) {
	t.Helper()
	st := store.NewMemory()
	msgs := conversation.NewMemoryStore()
	rt := &Runtime{
		Runs:           st,
		Meta:           msgs,
		Messages:       msgs,
		Assignee:       "alice",
		DefaultAgentID: "agent-1",
		Source:         "weixin",
		drainTick:      neverDrainTick,
	}
	st.UpsertAgent(store.Agent{ID: "agent-1"})
	return rt, st
}

func TestBusyQueuePersistsAndDeletesAfterRun(t *testing.T) {
	rt, st := newStoreRuntime(t)
	tick := make(chan time.Time, 1)
	rt.drainTick = tick
	ch := &fakeChannel{name: "fake"}
	active, err := st.CreateRun(store.CreateRunInput{
		AgentID: "agent-1", Input: "busy", ConversationID: mergeConvID,
	})
	if err != nil {
		t.Fatal(err)
	}
	var starts atomic.Int32
	rt.AfterCreateRun = func(context.Context, *store.Run, []llm.ContentPart) error {
		starts.Add(1)
		return nil
	}

	sendText(t, rt, ch, "第一条")
	sendText(t, rt, ch, "第二条")

	rows, err := st.ListChannelPending("weixin")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%+v", rows)
	}
	row := rows[0]
	if row.ConversationID != mergeConvID || row.BatchID == "" || row.Text != "第一条\n\n第二条" {
		t.Fatalf("row=%+v", row)
	}
	if row.Account != "acc-1" || row.PeerID != "peer-1" {
		t.Fatalf("routing=%+v", row)
	}

	if err := st.UpdateRun(active.ID, store.StatusSucceeded, "", ""); err != nil {
		t.Fatal(err)
	}
	tick <- time.Now()
	waitFor(t, "merged run", func() bool { return starts.Load() == 1 })

	runs, err := st.ListRunsByConversationSince(mergeConvID, time.Time{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	merged := ""
	for _, r := range runs {
		if r.ID != active.ID {
			merged = r.Input
		}
	}
	if merged != "第一条\n\n第二条" {
		t.Fatalf("merged run input=%q", merged)
	}
	waitFor(t, "pending deleted", func() bool {
		rows, err := st.ListChannelPending("weixin")
		return err == nil && len(rows) == 0
	})
}

func TestRestorePendingDrainsAfterRestart(t *testing.T) {
	rt, st := newStoreRuntime(t)
	seed := store.ChannelPending{
		ConversationID: mergeConvID,
		BatchID:        "b1",
		Source:         "weixin",
		Account:        "acc-1",
		PeerID:         "peer-1",
		Text:           "重启前的消息",
		UpdatedAt:      time.Now().UTC(),
	}
	if err := st.UpsertChannelPending(seed); err != nil {
		t.Fatal(err)
	}
	var got atomic.Value
	rt.AfterCreateRun = func(_ context.Context, runRec *store.Run, _ []llm.ContentPart) error {
		got.Store(runRec.Input)
		return nil
	}
	rt.drainTick = make(chan time.Time) // restored idle batch runs without a tick
	rt.RestorePending(&fakeChannel{name: "fake"})

	waitFor(t, "restored run", func() bool { return got.Load() != nil })
	if got.Load().(string) != "重启前的消息" {
		t.Fatalf("input=%q", got.Load())
	}
	waitFor(t, "pending deleted", func() bool {
		rows, err := st.ListChannelPending("weixin")
		return err == nil && len(rows) == 0
	})
}

func TestRestorePendingMergesRowsInOrder(t *testing.T) {
	rt, st := newStoreRuntime(t)
	if _, err := st.CreateRun(store.CreateRunInput{
		AgentID: "agent-1", Input: "busy", ConversationID: mergeConvID,
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, row := range []store.ChannelPending{
		{
			ConversationID: mergeConvID, BatchID: "b1", Source: "weixin",
			Account: "acc-1", PeerID: "peer-1", Text: "先", UpdatedAt: now.Add(-2 * time.Second),
		},
		{
			ConversationID: mergeConvID, BatchID: "b2", Source: "weixin",
			Account: "acc-1", PeerID: "peer-1", Text: "后", UpdatedAt: now.Add(-time.Second),
		},
	} {
		if err := st.UpsertChannelPending(row); err != nil {
			t.Fatal(err)
		}
	}
	rt.drainTick = make(chan time.Time) // active run parks the sweeper
	rt.RestorePending(&fakeChannel{name: "fake"})

	rt.queueMu.Lock()
	p := rt.pending[mergeConvID]
	rt.queueMu.Unlock()
	if p == nil || p.batchID != "b1" || p.text != "先\n\n后" {
		t.Fatalf("pending=%+v", p)
	}
	rows, err := st.ListChannelPending("weixin")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].BatchID != "b1" || rows[0].Text != "先\n\n后" {
		t.Fatalf("rows=%+v", rows)
	}
}

type failingPendingRuns struct {
	*fakeRuns
}

func (f failingPendingRuns) UpsertChannelPending(store.ChannelPending) error {
	return errors.New("db down")
}

func (f failingPendingRuns) ListChannelPending(string) ([]store.ChannelPending, error) {
	return nil, errors.New("db down")
}

func (f failingPendingRuns) DeleteChannelPending(string, string) error {
	return errors.New("db down")
}

func TestQueuePersistFailureKeepsMemoryBatch(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{mergeConvID: true}}
	rt, _ := newTestRuntime(t, runs)
	rt.Runs = failingPendingRuns{fakeRuns: runs}
	sendText(t, rt, &fakeChannel{name: "fake"}, "别丢")
	if !rt.hasQueued(mergeConvID) {
		t.Fatal("in-memory batch must survive a failed write")
	}
}

type recordingMedia struct {
	mu    sync.Mutex
	files []InboundFile
}

func (m *recordingMedia) SaveInboundImage(context.Context, string, string, string, []byte) (string, string, error) {
	return "/img", "img", nil
}

func (m *recordingMedia) SaveInboundFile(_ context.Context, _, name, mime string, data []byte) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files = append(m.files, InboundFile{
		Name: name,
		MIME: mime,
		Data: append([]byte(nil), data...),
	})
	return "/file", "file", nil
}

func (m *recordingMedia) snapshot() []InboundFile {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]InboundFile, len(m.files))
	copy(out, m.files)
	return out
}

func TestBusyAttachmentQueuesAndPersists(t *testing.T) {
	rt, st := newStoreRuntime(t)
	ch := &fakeChannel{name: "fake"}
	if _, err := st.CreateRun(store.CreateRunInput{
		AgentID: "agent-1", Input: "busy", ConversationID: mergeConvID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.HandleInbound(context.Background(), ch, Inbound{
		PeerID: "peer-1",
		Text:   "看图",
		Files:  []InboundFile{{Name: "a.txt", MIME: "text/plain", Data: []byte("hello")}},
		Extras: map[string]string{"account": "acc-1"},
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListChannelPending("weixin")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0].Attachments) != 1 ||
		rows[0].Attachments[0].Name != "a.txt" ||
		string(rows[0].Attachments[0].Data) != "hello" {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestRestorePendingDeliversAttachments(t *testing.T) {
	rt, st := newStoreRuntime(t)
	ch := &fakeChannel{name: "fake"}
	if err := st.UpsertChannelPending(store.ChannelPending{
		ConversationID: mergeConvID,
		BatchID:        "b1",
		Source:         "weixin",
		Account:        "acc-1",
		PeerID:         "peer-1",
		Text:           "看图",
		Attachments: []store.PendingAttachment{{
			Name: "a.txt", MIME: "text/plain", Data: []byte("hello"),
		}},
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	media := &recordingMedia{}
	rt.Media = media
	rt.drainTick = make(chan time.Time)
	var input atomic.Value
	rt.AfterCreateRun = func(_ context.Context, runRec *store.Run, _ []llm.ContentPart) error {
		input.Store(runRec.Input)
		return nil
	}
	rt.RestorePending(ch)

	waitFor(t, "restored run", func() bool { return input.Load() != nil })
	if got := input.Load().(string); !strings.Contains(got, "看图") {
		t.Fatalf("input=%q", got)
	}
	waitFor(t, "restored attachment", func() bool { return len(media.snapshot()) == 1 })
	file := media.snapshot()[0]
	if file.Name != "a.txt" || file.MIME != "text/plain" || string(file.Data) != "hello" {
		t.Fatalf("file=%+v", file)
	}
	waitFor(t, "pending deleted", func() bool {
		rows, err := st.ListChannelPending("weixin")
		return err == nil && len(rows) == 0
	})
}

func TestRestorePendingWithoutStoreIsNoop(t *testing.T) {
	rt, _ := newTestRuntime(t, &fakeRuns{active: map[string]bool{}})
	rt.RestorePending(&fakeChannel{name: "fake"})
}

func TestQueueAckFailureKeepsBatch(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{mergeConvID: true}}
	rt, _ := newTestRuntime(t, runs)
	tick := make(chan time.Time, 1)
	rt.drainTick = tick
	ch := ackFailChannel{fakeChannel: &fakeChannel{name: "fake"}}

	if err := rt.HandleInbound(context.Background(), ch, Inbound{
		PeerID: "peer-1",
		Text:   "别丢",
		Extras: map[string]string{"account": "acc-1"},
	}); err != nil {
		t.Fatalf("ack failure must not fail the handler: %v", err)
	}
	if !rt.hasQueued(mergeConvID) {
		t.Fatal("message must stay queued when the ack fails")
	}

	runs.setActive(mergeConvID, false)
	tick <- time.Now()
	waitFor(t, "queued run", func() bool { return runs.createCount() == 1 })
	if got := runs.lastCreate().Input; got != "别丢" {
		t.Fatalf("input=%q", got)
	}
}
