package channel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

// validPNG builds a small in-memory PNG so attach.processImage can decode it.
func validPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

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

// stubResolver returns a BuildDeps-style ResolveModel for channel tests:
// visionID is the model an image turn is pinned to ("" = no vision model).
// Non-image turns resolve to "" (Switch primary).
func stubResolver(visionID string) func(llm.TaskSignals) (string, bool, bool) {
	return func(sig llm.TaskSignals) (string, bool, bool) {
		if sig.HasImages {
			if visionID == "" {
				return "", false, true
			}
			return visionID, true, true
		}
		return "", true, true
	}
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

// TestHandleInboundUnsupportedFileStillReplies guards the "WeChat sends a
// non-parsable file type and gets no reply" regression: an unsupported MIME
// (e.g. .zip -> application/octet-stream) must NOT abort CreateRun. The run is
// still created (so the agent replies) and the filename is recorded as a note.
func TestHandleInboundUnsupportedFileStillReplies(t *testing.T) {
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
		Text:   "这个文件帮我看下",
		Extras: map[string]string{"account": "acc-1"},
		Files: []InboundFile{{
			Name: "archive.zip",
			MIME: "application/octet-stream",
			Data: []byte{0x50, 0x4B, 0x03, 0x04, 0, 0, 0, 0}, // ZIP magic
		}},
	})
	if err != nil {
		t.Fatalf("HandleInbound must not fail for unsupported file: %v", err)
	}
	if runs.createCount() != 1 {
		t.Fatalf("CreateRun calls = %d, want 1 (unsupported file must still create a run)", runs.createCount())
	}
	in := runs.lastCreate()
	if !strings.Contains(in.Input, "archive.zip") {
		t.Fatalf("display input should name the file: %q", in.Input)
	}
	if len(gotParts) == 0 {
		t.Fatal("expected user parts noting the unsupported file")
	}
	joined := ""
	for _, p := range gotParts {
		joined += p.Text
	}
	if !strings.Contains(joined, "archive.zip") || !strings.Contains(joined, "暂不支持解析") {
		t.Fatalf("user parts should note unsupported file: %q", joined)
	}
}

// TestHandleInboundMixedSupportedAndUnsupportedFiles verifies a supported
// file is extracted while an unsupported sibling is name-noted (neither drops
// the message).
func TestHandleInboundMixedSupportedAndUnsupportedFiles(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{}}
	rt, _ := newTestRuntime(t, runs)
	ch := &fakeChannel{name: "fake"}
	err := rt.HandleInbound(context.Background(), ch, Inbound{
		PeerID: "peer-1",
		Text:   "两个文件",
		Extras: map[string]string{"account": "acc-1"},
		Files: []InboundFile{
			{Name: "note.txt", MIME: "text/plain", Data: []byte("readable text")},
			{Name: "data.bin", MIME: "application/octet-stream", Data: []byte{0, 1, 2, 3}},
		},
	})
	if err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}
	if runs.createCount() != 1 {
		t.Fatalf("CreateRun calls = %d, want 1", runs.createCount())
	}
	in := runs.lastCreate()
	for _, want := range []string{"note.txt", "data.bin"} {
		if !strings.Contains(in.Input, want) {
			t.Fatalf("display input missing %q: %q", want, in.Input)
		}
	}
}

// fakeMediaStore records saved images/files and returns a fixed URL.
type fakeMediaStore struct {
	images []string
	files  []string
}

func (f *fakeMediaStore) SaveInboundImage(_ context.Context, convID, filename, mime string, data []byte) (string, string, error) {
	f.images = append(f.images, filename)
	return "/v0/channels/media/" + convID + "/obj_" + filename, "obj_" + filename, nil
}

func (f *fakeMediaStore) SaveInboundFile(_ context.Context, convID, filename, mime string, data []byte) (string, string, error) {
	f.files = append(f.files, filename)
	return "/v0/channels/media/" + convID + "/dl_" + filename, "dl_" + filename, nil
}

// TestHandleInboundPersistsImageForInlineDisplay guards the "WeChat image
// shows as media.bin / no inline image" regression: an inbound image is
// persisted via the MediaStore and the stored user bubble carries a renderable
// image reference (while run.Input stays free of the URL).
func TestHandleInboundPersistsImageForInlineDisplay(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{}}
	rt, meta := newTestRuntime(t, runs)
	media := &fakeMediaStore{}
	rt.Media = media
	rt.ResolveModel = func(llm.TaskSignals) (string, bool, bool) {
		return "", true, true // primary model is vision-capable
	}

	// Valid PNG magic + IHDR-ish bytes (attach.processImage decodes real
	// images; use a small real PNG built by image/png).
	png := validPNG(t)
	ch := &fakeChannel{name: "fake"}
	err := rt.HandleInbound(context.Background(), ch, Inbound{
		PeerID: "peer-1",
		Text:   "看这张图",
		Extras: map[string]string{"account": "acc-1"},
		Files: []InboundFile{{
			Name: "photo.png",
			MIME: "image/png",
			Data: png,
		}},
	})
	if err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}
	if len(media.images) != 1 {
		t.Fatalf("expected 1 image persisted, got %d", len(media.images))
	}
	convID := "weixin:acc-1:peer-1"
	msgs := meta.List(convID)
	if len(msgs) == 0 {
		t.Fatal("no user message persisted")
	}
	last := msgs[len(msgs)-1]
	if !strings.Contains(last.Content, "/v0/channels/media/") || !strings.Contains(last.Content, "![图片]") {
		t.Fatalf("user bubble missing image reference: %q", last.Content)
	}
	// run.Input must not carry the URL (LLM gets bytes via parts).
	in := runs.lastCreate()
	if strings.Contains(in.Input, "/v0/channels/media/") {
		t.Fatalf("run.Input must not contain media URL: %q", in.Input)
	}
}

// TestHandleInboundPersistsFileForDownload guards issue #2: a non-image file
// (docx) is persisted and referenced as a downloadable link in the user bubble,
// even though its contents are only name-noted for the model.
func TestHandleInboundPersistsFileForDownload(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{}}
	rt, meta := newTestRuntime(t, runs)
	media := &fakeMediaStore{}
	rt.Media = media

	docx := []byte{0x50, 0x4B, 0x03, 0x04, 0, 0, 0, 0} // OOXML/zip magic
	ch := &fakeChannel{name: "fake"}
	err := rt.HandleInbound(context.Background(), ch, Inbound{
		PeerID: "peer-1",
		Text:   "行程文件",
		Extras: map[string]string{"account": "acc-1"},
		Files: []InboundFile{{
			Name: "黄山三日行程.docx",
			MIME: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			Data: docx,
		}},
	})
	if err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}
	if len(media.files) != 1 {
		t.Fatalf("expected 1 file persisted for download, got %d", len(media.files))
	}
	if media.files[0] != "黄山三日行程.docx" {
		t.Fatalf("persisted file name = %q", media.files[0])
	}
	convID := "weixin:acc-1:peer-1"
	msgs := meta.List(convID)
	last := msgs[len(msgs)-1]
	if !strings.Contains(last.Content, "[file:黄山三日行程.docx](") {
		t.Fatalf("user bubble missing downloadable file link: %q", last.Content)
	}
}

// hasImagePart reports whether parts carry a multimodal image part.
func hasImagePart(parts []llm.ContentPart) bool {
	for _, p := range parts {
		if p.Type == "image" {
			return true
		}
	}
	return false
}

// TestHandleInboundRoutesImageToVisionModel: the default model is text-only
// (SupportsVision=false) but a vision profile exists. An inbound image must be
// encoded as a multimodal part AND the run pinned to the vision profile so the
// Switch resolves a vision-capable model for that run.
func TestHandleInboundRoutesImageToVisionModel(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{}}
	rt, _ := newTestRuntime(t, runs)
	rt.ResolveModel = stubResolver("mp_vision")
	var gotParts []llm.ContentPart
	rt.AfterCreateRun = func(_ context.Context, _ *store.Run, parts []llm.ContentPart) error {
		gotParts = parts
		return nil
	}
	ch := &fakeChannel{name: "fake"}
	err := rt.HandleInbound(context.Background(), ch, Inbound{
		PeerID: "peer-1",
		Text:   "看这张图",
		Extras: map[string]string{"account": "acc-1"},
		Files: []InboundFile{{
			Name: "photo.png",
			MIME: "image/png",
			Data: validPNG(t),
		}},
	})
	if err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}
	in := runs.lastCreate()
	if in.ModelProfileID != "mp_vision" {
		t.Fatalf("ModelProfileID = %q, want mp_vision (image routed to vision model)", in.ModelProfileID)
	}
	if !hasImagePart(gotParts) {
		t.Fatalf("expected an image part for the vision model, got %+v", gotParts)
	}
}

// TestHandleInboundTextFileUsesDefaultModel: a docx/text attachment carries no
// image part, so the run must NOT be pinned to a vision model — it stays on the
// default model even when a vision profile is available.
func TestHandleInboundTextFileUsesDefaultModel(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{}}
	rt, _ := newTestRuntime(t, runs)
	rt.ResolveModel = stubResolver("mp_vision")
	var gotParts []llm.ContentPart
	rt.AfterCreateRun = func(_ context.Context, _ *store.Run, parts []llm.ContentPart) error {
		gotParts = parts
		return nil
	}
	ch := &fakeChannel{name: "fake"}
	err := rt.HandleInbound(context.Background(), ch, Inbound{
		PeerID: "peer-1",
		Text:   "看下这个文档",
		Extras: map[string]string{"account": "acc-1"},
		Files: []InboundFile{{
			Name: "note.txt",
			MIME: "text/plain",
			Data: []byte("just some text, no images here"),
		}},
	})
	if err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}
	in := runs.lastCreate()
	if in.ModelProfileID != "" {
		t.Fatalf("ModelProfileID = %q, want empty (text-only file stays on default model)", in.ModelProfileID)
	}
	if hasImagePart(gotParts) {
		t.Fatalf("text file must not produce an image part: %+v", gotParts)
	}
}

// TestHandleInboundNoVisionModelDegradesImage: the default model is text-only
// and no vision profile exists. The image must be degraded to a text note (no
// image part) and the run left on the default model, so the message still gets
// a reply instead of erroring.
func TestHandleInboundNoVisionModelDegradesImage(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{}}
	rt, _ := newTestRuntime(t, runs)
	rt.ResolveModel = stubResolver("") // no vision model
	var gotParts []llm.ContentPart
	rt.AfterCreateRun = func(_ context.Context, _ *store.Run, parts []llm.ContentPart) error {
		gotParts = parts
		return nil
	}
	ch := &fakeChannel{name: "fake"}
	err := rt.HandleInbound(context.Background(), ch, Inbound{
		PeerID: "peer-1",
		Text:   "看这张图",
		Extras: map[string]string{"account": "acc-1"},
		Files: []InboundFile{{
			Name: "photo.png",
			MIME: "image/png",
			Data: validPNG(t),
		}},
	})
	if err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}
	in := runs.lastCreate()
	if in.ModelProfileID != "" {
		t.Fatalf("ModelProfileID = %q, want empty (no vision model configured)", in.ModelProfileID)
	}
	if hasImagePart(gotParts) {
		t.Fatalf("image part must be dropped without a vision model: %+v", gotParts)
	}
}

// TestHandleInboundDefaultVisionModelNotOverridden: when the primary model is
// already vision-capable, images are sent as multimodal parts but the run is
// NOT pinned to a separate vision profile (the resolver reports vision
// reachable with an empty id, so the Switch uses its primary model).
func TestHandleInboundDefaultVisionModelNotOverridden(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{}}
	rt, _ := newTestRuntime(t, runs)
	rt.ResolveModel = func(llm.TaskSignals) (string, bool, bool) {
		return "", true, true // primary model handles images; no pin
	}
	var gotParts []llm.ContentPart
	rt.AfterCreateRun = func(_ context.Context, _ *store.Run, parts []llm.ContentPart) error {
		gotParts = parts
		return nil
	}
	ch := &fakeChannel{name: "fake"}
	err := rt.HandleInbound(context.Background(), ch, Inbound{
		PeerID: "peer-1",
		Text:   "看这张图",
		Extras: map[string]string{"account": "acc-1"},
		Files: []InboundFile{{
			Name: "photo.png",
			MIME: "image/png",
			Data: validPNG(t),
		}},
	})
	if err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}
	in := runs.lastCreate()
	if in.ModelProfileID != "" {
		t.Fatalf("ModelProfileID = %q, want empty (default model already supports vision)", in.ModelProfileID)
	}
	if !hasImagePart(gotParts) {
		t.Fatalf("expected an image part for the vision-capable default: %+v", gotParts)
	}
}

func TestRegisterOpenList(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

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

func TestHandleInboundUsesRuntimeSource(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{}}
	meta := conversation.NewMemoryStore()
	rt := &Runtime{
		Runs: runs, Meta: meta, Messages: meta,
		Assignee:       "alice",
		DefaultAgentID: "agent-1",
		Source:         "feishu", // 非 weixin 前缀
	}
	in := Inbound{PeerID: "p1", Text: "hi", Extras: map[string]string{"account": "acc"}}
	if err := rt.HandleInbound(context.Background(), &fakeChannel{name: "feishu"}, in); err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}
	m, err := meta.GetMeta("feishu:acc:p1")
	if err != nil {
		t.Fatalf("expected conv id feishu:acc:p1, err=%v", err)
	}
	if m.Source != "feishu" {
		t.Fatalf("meta.Source=%q want feishu", m.Source)
	}
}

func TestConvIDHelpers(t *testing.T) {
	id := ConvID("weixin", "acc", "peer")
	if id != "weixin:acc:peer" {
		t.Fatalf("ConvID=%q", id)
	}
	if src := SourceFromConvID(id); src != "weixin" {
		t.Fatalf("SourceFromConvID=%q", src)
	}
	if src := SourceFromConvID("ui:abc"); src != "ui" {
		t.Fatalf("ui prefix=%q", src)
	}
}

func TestOutboundExtrasIncludesAccountAndContextToken(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{}}
	rt, _ := newTestRuntime(t, runs)
	convID := "weixin:acct-1:peer-1"
	rt.rememberContextToken(convID, map[string]string{
		"context_token": "tok-abc",
		"account":       "acct-1",
	})
	ex := rt.OutboundExtras(convID)
	if ex == nil {
		t.Fatal("OutboundExtras returned nil")
	}
	if ex["context_token"] != "tok-abc" {
		t.Fatalf("context_token = %q, want tok-abc", ex["context_token"])
	}
	if ex["account"] != "acct-1" {
		t.Fatalf("account = %q, want acct-1", ex["account"])
	}
	// Only token, no account -> still returns map with context_token.
	rt.rememberContextToken("weixin:acct-2:peer-2", map[string]string{"context_token": "t2"})
	if ex2 := rt.OutboundExtras("weixin:acct-2:peer-2"); ex2["context_token"] != "t2" || ex2["account"] != "" {
		t.Fatalf("ex2 = %+v", ex2)
	}
	// Unknown conversation -> nil.
	if got := rt.OutboundExtras("weixin:nope:x"); got != nil {
		t.Fatalf("unknown conv = %+v, want nil", got)
	}
}

func TestSetRoutingHotUpdatesAndIgnoresBlank(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{}}
	rt, _ := newTestRuntime(t, runs)

	// Hot update applies both fields.
	rt.SetRouting("op:alice", "ag-2")
	assignee, agentID := rt.routing()
	if assignee != "op:alice" || agentID != "ag-2" {
		t.Fatalf("routing() = %q, %q; want op:alice, ag-2", assignee, agentID)
	}

	// Blank/empty values are ignored: a partial update cannot blank a field.
	rt.SetRouting("  ", "")
	assignee, agentID = rt.routing()
	if assignee != "op:alice" || agentID != "ag-2" {
		t.Fatalf("routing() after blank = %q, %q; want unchanged op:alice, ag-2", assignee, agentID)
	}

	// A non-blank partial update changes only the supplied field.
	rt.SetRouting("op:bob", "   ")
	assignee, agentID = rt.routing()
	if assignee != "op:bob" || agentID != "ag-2" {
		t.Fatalf("routing() after partial = %q, %q; want op:bob, ag-2", assignee, agentID)
	}
}

func TestAccountFromConvID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"weixin:acct-1:peer-1", "acct-1"},
		{"webhook:bot9:snowflake", "bot9"},
		{"weixin:acct-1:peer:with:colon", "acct-1"},
		{"ui:abc", ""},
		{"no-colon", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := AccountFromConvID(c.in); got != c.want {
			t.Errorf("AccountFromConvID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
