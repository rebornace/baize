package api_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/channelmedia"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

type captureUserLLM struct {
	vision       bool
	mu           sync.Mutex
	lastUserText string
	lastSystem   string
	sawImagePart bool
	chatCalls    int
}

func (c *captureUserLLM) SupportsVision() bool { return c.vision }

func (c *captureUserLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	_ = ctx
	_ = tools
	c.mu.Lock()
	defer c.mu.Unlock()
	c.chatCalls++
	if len(messages) > 0 && messages[0].Role == llm.RoleSystem {
		c.lastSystem = messages[0].Content
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != llm.RoleUser {
			continue
		}
		if len(messages[i].Parts) > 0 {
			for _, p := range messages[i].Parts {
				switch p.Type {
				case "text":
					c.lastUserText = p.Text
				case "image":
					c.sawImagePart = true
				}
			}
		} else {
			c.lastUserText = messages[i].Content
		}
		break
	}
	return llm.Message{Role: llm.RoleAssistant, Content: "done"}, nil
}

func (c *captureUserLLM) snapshot() (string, bool, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastUserText, c.sawImagePart, c.chatCalls
}

func (c *captureUserLLM) systemSnapshot() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastSystem
}

func attachmentsServer(t *testing.T, vision bool) (*api.Server, store.Store, *captureUserLLM, http.Handler, *skill.Catalog) {
	t.Helper()
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	skillDir := filepath.Join(builtin, "data-analytics")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("---\nname: data-analytics\ndescription: analytics skill\ntools:\n  - list_tickets\n---\n\nuse list_tickets for analytics\n")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	cat, err := skill.LoadCatalog([]string{builtin}, filepath.Join(root, "user"))
	if err != nil {
		t.Fatal(err)
	}
	st := store.NewMemory()
	reg := tool.NewRegistry()
	reg.Register("list_tickets", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})
	llmMock := &captureUserLLM{vision: vision}
	eng := &run.Engine{Store: st, LLM: llmMock, Tools: reg, Gate: run.NewGate(), Skills: cat}
	srv := api.NewServer(st, reg, eng)
	srv.LLM = llmMock
	srv.SkillCatalog = cat
	srv.Messages = conversation.NewMemoryStore()
	// Persist uploaded attachments exactly like production (channelmedia over
	// a blob store) so the user bubble carries renderable media references.
	blobs, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatalf("open memory blob: %v", err)
	}
	media := channelmedia.New(blobs)
	srv.ChannelMedia = media
	srv.ChatMedia = media
	return srv, st, llmMock, srv.Handler(), cat
}

func putAgent(t *testing.T, h http.Handler, id string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/v0/agents/"+id,
		jsonBody(t, map[string]any{"system": "helper", "skills": []string{"data-analytics"}}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("put agent status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func postRun(t *testing.T, h http.Handler, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v0/runs", jsonBody(t, body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func decodeError(t *testing.T, body []byte) (code, message string) {
	t.Helper()
	var wrap struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		t.Fatalf("decode error body: %v body=%s", err, body)
	}
	return wrap.Error.Code, wrap.Error.Message
}

func tinyPNGBase64(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	var buf strings.Builder
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString([]byte(buf.String()))
}

func TestPostRunUnknownSkillReturns400(t *testing.T) {
	_, st, _, h, _ := attachmentsServer(t, false)
	putAgent(t, h, "a1")

	rr := postRun(t, h, map[string]any{
		"agent_id":        "a1",
		"input":           "hi",
		"conversation_id": "c1",
		"skills":          []string{"nope"},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	code, _ := decodeError(t, rr.Body.Bytes())
	if code != "unknown_skill" {
		t.Fatalf("code=%q want unknown_skill", code)
	}
	if busy, _ := st.HasActiveRun("c1"); busy {
		t.Fatal("expected no run created for unknown skill")
	}
}

func TestPostRunImageWithoutVisionReturns400(t *testing.T) {
	_, st, _, h, _ := attachmentsServer(t, false)
	putAgent(t, h, "a1")

	rr := postRun(t, h, map[string]any{
		"agent_id":        "a1",
		"input":           "look at this",
		"conversation_id": "c1",
		"attachments": []map[string]any{
			{
				"filename":       "dot.png",
				"media_type":     "image/png",
				"content_base64": tinyPNGBase64(t),
			},
		},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	code, _ := decodeError(t, rr.Body.Bytes())
	if code != "vision_unsupported" {
		t.Fatalf("code=%q want vision_unsupported", code)
	}
	if busy, _ := st.HasActiveRun("c1"); busy {
		t.Fatal("expected no run created when vision unsupported")
	}
}

// seedVisionProfile inserts a vision-capable model profile and returns its
// generated id. Image turns are restricted to vision-capable profiles by the
// router regardless of tier.
func seedVisionProfile(t *testing.T, st store.Store, name string) string {
	t.Helper()
	p, err := st.UpsertModelProfile(store.ModelProfile{
		Name:           name,
		Provider:       "openai_compatible",
		BaseURL:        "http://example.test/v1",
		Model:          "vision-model",
		APIKey:         "sk-test",
		SupportsVision: true,
		AutoTier:       store.AutoTierStandard,
	})
	if err != nil {
		t.Fatalf("seed vision profile %q: %v", name, err)
	}
	return p.ID
}

// postRunID extracts run_id from a successful POST /runs response.
func postRunID(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var resp struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode run response: %v body=%s", err, rr.Body.String())
	}
	return resp.RunID
}

// TestPostRunImageAutoRoutesToVisionProfile: the default provider is text-only
// but a vision profile exists. An image turn must NOT be rejected; the run is
// created and pinned to the vision profile (smart routing), with the image
// delivered as a multimodal part.
func TestPostRunImageAutoRoutesToVisionProfile(t *testing.T) {
	_, st, llmMock, h, _ := attachmentsServer(t, false)
	putAgent(t, h, "a1")
	vid := seedVisionProfile(t, st, "视觉模型")

	rr := postRun(t, h, map[string]any{
		"agent_id":        "a1",
		"input":           "look at this",
		"conversation_id": "c1",
		"attachments": []map[string]any{
			{
				"filename":       "dot.png",
				"media_type":     "image/png",
				"content_base64": tinyPNGBase64(t),
			},
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	// The run must be pinned to the vision profile.
	runID := postRunID(t, rr)
	runRec, err := st.GetRun(runID)
	if err != nil || runRec == nil {
		t.Fatalf("get run: %v", err)
	}
	if got := runRec.ModelProfileID; got != vid {
		t.Fatalf("run ModelProfileID = %q, want %q (auto-routed)", got, vid)
	}
	// Wait for the async run to execute so the LLM actually receives the part.
	pollRunStatus(t, h, runID, store.StatusSucceeded)
	if _, sawImage, _ := llmMock.snapshot(); !sawImage {
		t.Fatal("expected the image part to be delivered to the model")
	}
}

// seedTextProfile inserts a text-only model profile and returns its id.
func seedTextProfile(t *testing.T, st store.Store, name string) string {
	t.Helper()
	p, err := st.UpsertModelProfile(store.ModelProfile{
		Name:           name,
		Provider:       "openai_compatible",
		BaseURL:        "http://example.test/v1",
		Model:          "text-model",
		APIKey:         "sk-test",
		SupportsVision: false,
	})
	if err != nil {
		t.Fatalf("seed text profile %q: %v", name, err)
	}
	return p.ID
}

// TestPostRunImageManualTextModelRejected: a manual text-only model selection
// on an image turn is NEVER silently rerouted to a vision model — even when a
// vision profile exists. It is rejected with vision_unsupported so the user
// explicitly chooses Auto or a vision model.
func TestPostRunImageManualTextModelRejected(t *testing.T) {
	_, st, llmMock, h, _ := attachmentsServer(t, false)
	putAgent(t, h, "a1")
	textID := seedTextProfile(t, st, "纯文本模型")
	_ = seedVisionProfile(t, st, "视觉模型") // a vision model exists but must NOT be auto-used

	rr := postRun(t, h, map[string]any{
		"agent_id":         "a1",
		"input":            "look",
		"conversation_id":  "c1",
		"model_profile_id": textID,
		"attachments": []map[string]any{
			{
				"filename":       "dot.png",
				"media_type":     "image/png",
				"content_base64": tinyPNGBase64(t),
			},
		},
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want 400", rr.Code, rr.Body.String())
	}
	code, _ := decodeError(t, rr.Body.Bytes())
	if code != "vision_unsupported" {
		t.Fatalf("code=%q want vision_unsupported", code)
	}
	if busy, _ := st.HasActiveRun("c1"); busy {
		t.Fatal("expected no run created for manual text-only model + image")
	}
	if _, sawImage, calls := llmMock.snapshot(); sawImage || calls != 0 {
		t.Fatalf("LLM must not be called on rejected turn: sawImage=%v calls=%d", sawImage, calls)
	}
}

// TestPostRunImageAutoExplicitTokenRoutes: sending model_profile_id="auto"
// behaves like the default auto mode (routes to the vision model).
func TestPostRunImageAutoExplicitTokenRoutes(t *testing.T) {
	_, st, llmMock, h, _ := attachmentsServer(t, false)
	putAgent(t, h, "a1")
	vid := seedVisionProfile(t, st, "视觉模型")

	rr := postRun(t, h, map[string]any{
		"agent_id":         "a1",
		"input":            "look",
		"conversation_id":  "c1",
		"model_profile_id": "auto",
		"attachments": []map[string]any{
			{
				"filename":       "dot.png",
				"media_type":     "image/png",
				"content_base64": tinyPNGBase64(t),
			},
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	runID := postRunID(t, rr)
	runRec, err := st.GetRun(runID)
	if err != nil || runRec == nil {
		t.Fatalf("get run: %v", err)
	}
	if got := runRec.ModelProfileID; got != vid {
		t.Fatalf("run ModelProfileID = %q, want %q (auto routed)", got, vid)
	}
	pollRunStatus(t, h, runID, store.StatusSucceeded)
	if _, sawImage, _ := llmMock.snapshot(); !sawImage {
		t.Fatal("expected the image part to be delivered to the model")
	}
}

// TestPostRunImageExplicitVisionProfile: the user explicitly selects a
// vision-capable profile; that id wins over auto-routing.
func TestPostRunImageExplicitVisionProfile(t *testing.T) {
	_, st, _, h, _ := attachmentsServer(t, false)
	putAgent(t, h, "a1")
	pick := seedVisionProfile(t, st, "我选的视觉")
	_ = seedVisionProfile(t, st, "另一个视觉")

	rr := postRun(t, h, map[string]any{
		"agent_id":         "a1",
		"input":            "look",
		"conversation_id":  "c1",
		"model_profile_id": pick,
		"attachments": []map[string]any{
			{
				"filename":       "dot.png",
				"media_type":     "image/png",
				"content_base64": tinyPNGBase64(t),
			},
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	runRec, err := st.GetRun(postRunID(t, rr))
	if err != nil || runRec == nil {
		t.Fatalf("get run: %v", err)
	}
	if got := runRec.ModelProfileID; got != pick {
		t.Fatalf("run ModelProfileID = %q, want %q (explicit choice)", got, pick)
	}
}

// TestPostRunTextFileNoVisionModelNoRouting: a text-only attachment with no
// vision model present must NOT be rejected (no images => no vision gate) and
// must not pin any model.
func TestPostRunTextFileNoVisionModelNoRouting(t *testing.T) {
	_, st, _, h, _ := attachmentsServer(t, false)
	putAgent(t, h, "a1")

	rr := postRun(t, h, map[string]any{
		"agent_id":        "a1",
		"input":           "read this",
		"conversation_id": "c1",
		"attachments": []map[string]any{
			{
				"filename":       "note.txt",
				"media_type":     "text/plain",
				"content_base64": base64.StdEncoding.EncodeToString([]byte("hello text")),
			},
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	runRec, err := st.GetRun(postRunID(t, rr))
	if err != nil || runRec == nil {
		t.Fatalf("get run: %v", err)
	}
	if got := runRec.ModelProfileID; got != "" {
		t.Fatalf("text file run ModelProfileID = %q, want empty (no routing)", got)
	}
}

func TestPostRunMarkdownAttachmentInjected(t *testing.T) {
	_, st, llmMock, h, _ := attachmentsServer(t, false)
	putAgent(t, h, "a1")

	md := "# Title\nhello attachment body"
	rr := postRun(t, h, map[string]any{
		"agent_id":        "a1",
		"input":           "summarize the attachment",
		"conversation_id": "c1",
		"attachments": []map[string]any{
			{
				"filename":       "notes.md",
				"media_type":     "text/markdown",
				"content_base64": base64.StdEncoding.EncodeToString([]byte(md)),
			},
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	runID, _ := created["run_id"].(string)
	if runID == "" {
		t.Fatalf("created=%v", created)
	}
	pollRunStatus(t, h, runID, store.StatusSucceeded)

	userText, sawImage, _ := llmMock.snapshot()
	if sawImage {
		t.Fatal("did not expect image part for text-only attachment")
	}
	if !strings.Contains(userText, "【附件: notes.md】") {
		t.Fatalf("LLM user text missing attachment header: %q", userText)
	}
	if !strings.Contains(userText, "hello attachment body") {
		t.Fatalf("LLM user text missing attachment content: %q", userText)
	}
	if !strings.Contains(userText, "summarize the attachment") {
		t.Fatalf("LLM user text missing original input: %q", userText)
	}

	getMsgs := httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/messages", nil)
	mr := httptest.NewRecorder()
	h.ServeHTTP(mr, getMsgs)
	if mr.Code != http.StatusOK {
		t.Fatalf("messages status=%d", mr.Code)
	}
	var msgs []conversation.Message
	if err := json.NewDecoder(mr.Body).Decode(&msgs); err != nil {
		t.Fatal(err)
	}
	var userBubble string
	for _, m := range msgs {
		if m.Role == conversation.RoleUser {
			userBubble = m.Content
		}
	}
	// The bubble shows the typed text once plus a renderable file card marker;
	// it must not repeat the filename as a "（附件：…）" note or leak extracted
	// text / an internal media URL into the plain text portion.
	if !strings.Contains(userBubble, "summarize the attachment") {
		t.Fatalf("persisted user bubble missing typed text: %q", userBubble)
	}
	if !strings.Contains(userBubble, "[file:notes.md](/v0/channels/media/") {
		t.Fatalf("persisted user bubble missing renderable file marker: %q", userBubble)
	}
	if strings.Contains(userBubble, "（附件：") {
		t.Fatalf("persisted user bubble must not repeat a （附件：…） note: %q", userBubble)
	}
	if strings.Contains(userBubble, "hello attachment body") {
		t.Fatalf("persisted user bubble must not contain extracted attachment text: %q", userBubble)
	}

	// The run record (model-facing input) must stay clean: no media marker.
	runRec, err := st.GetRun(runID)
	if err != nil || runRec == nil {
		t.Fatalf("get run: %v", err)
	}
	if strings.Contains(runRec.Input, "/v0/channels/media/") || strings.Contains(runRec.Input, "（附件：") {
		t.Fatalf("run.Input must be clean model-facing text, got %q", runRec.Input)
	}
	if runRec.Input != "summarize the attachment" {
		t.Fatalf("run.Input = %q, want clean typed text", runRec.Input)
	}
}

// TestPostRunImageBubbleRendersInlineMarker: a web-uploaded image persists a
// thumbnail and the user bubble carries an inline-image media marker (not a
// bare filename note), while the run input stays clean.
func TestPostRunImageBubbleRendersInlineMarker(t *testing.T) {
	_, st, llmMock, h, _ := attachmentsServer(t, true)
	putAgent(t, h, "a1")
	seedVisionProfile(t, st, "视觉模型")

	rr := postRun(t, h, map[string]any{
		"agent_id":        "a1",
		"input":           "看这张图",
		"conversation_id": "c1",
		"attachments": []map[string]any{
			{
				"filename":       "pic.png",
				"media_type":     "image/png",
				"content_base64": tinyPNGBase64(t),
			},
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	runID := postRunID(t, rr)
	pollRunStatus(t, h, runID, store.StatusSucceeded)
	if _, sawImage, _ := llmMock.snapshot(); !sawImage {
		t.Fatal("expected the image part to be delivered to the model")
	}

	getMsgs := httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/messages", nil)
	mr := httptest.NewRecorder()
	h.ServeHTTP(mr, getMsgs)
	if mr.Code != http.StatusOK {
		t.Fatalf("messages status=%d", mr.Code)
	}
	var msgs []conversation.Message
	if err := json.NewDecoder(mr.Body).Decode(&msgs); err != nil {
		t.Fatal(err)
	}
	var bubble string
	for _, m := range msgs {
		if m.Role == conversation.RoleUser {
			bubble = m.Content
		}
	}
	if !strings.Contains(bubble, "![图片](/v0/channels/media/") {
		t.Fatalf("image bubble missing inline marker: %q", bubble)
	}
	if strings.Contains(bubble, "（附件：") {
		t.Fatalf("image bubble must not repeat （附件：…）: %q", bubble)
	}

	// The stored object must be downloadable through the ACL media route.
	_, images := splitBubbleMedia(bubble)
	if len(images) != 1 {
		t.Fatalf("want 1 image url, got %v (%q)", images, bubble)
	}
	mediaReq := httptest.NewRequest(http.MethodGet, images[0], nil)
	mediaRR := httptest.NewRecorder()
	h.ServeHTTP(mediaRR, mediaReq)
	if mediaRR.Code != http.StatusOK {
		t.Fatalf("media GET %s status=%d", images[0], mediaRR.Code)
	}
}

// splitBubbleMedia is a tiny test helper mirroring the frontend's marker
// grammar, returning image and file media URLs found in a bubble.
func splitBubbleMedia(bubble string) (files, images []string) {
	re := regexp.MustCompile(`/v0/channels/media/[^)\s]+`)
	for _, line := range strings.Split(bubble, "\n") {
		u := re.FindString(line)
		if u == "" {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "![") {
			images = append(images, u)
		} else {
			files = append(files, u)
		}
	}
	return files, images
}

func TestPostRunImageWithVisionSendsImagePart(t *testing.T) {
	_, st, llmMock, h, _ := attachmentsServer(t, true)
	putAgent(t, h, "a1")
	seedVisionProfile(t, st, "视觉模型")

	rr := postRun(t, h, map[string]any{
		"agent_id":        "a1",
		"input":           "describe the image",
		"conversation_id": "c1",
		"attachments": []map[string]any{
			{
				"filename":       "dot.png",
				"media_type":     "image/png",
				"content_base64": tinyPNGBase64(t),
			},
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	runID, _ := created["run_id"].(string)
	pollRunStatus(t, h, runID, store.StatusSucceeded)
	_, sawImage, _ := llmMock.snapshot()
	if !sawImage {
		t.Fatal("expected an image part forwarded to the LLM when vision supported")
	}
}

func TestUIConfigReportsSupportsVision(t *testing.T) {
	for _, vision := range []bool{false, true} {
		vision := vision
		t.Run("vision="+map[bool]string{false: "false", true: "true"}[vision], func(t *testing.T) {
			_, st, _, h, _ := attachmentsServer(t, vision)
			if vision {
				seedVisionProfile(t, st, "视觉模型")
			}
			req := httptest.NewRequest(http.MethodGet, "/v0/ui-config", nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			var cfg struct {
				SupportsVision bool `json:"supports_vision"`
			}
			if err := json.NewDecoder(rr.Body).Decode(&cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.SupportsVision != vision {
				t.Fatalf("supports_vision=%v want %v", cfg.SupportsVision, vision)
			}
		})
	}
}

func TestPostRunSkillMentionOverridesAgentDefaults(t *testing.T) {
	_, _, llmMock, h, _ := attachmentsServer(t, false)
	putAgent(t, h, "a1")

	rr := postRun(t, h, map[string]any{
		"agent_id":        "a1",
		"input":           "@data-analytics build a dashboard",
		"conversation_id": "c1",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	runID, _ := created["run_id"].(string)
	pollRunStatus(t, h, runID, store.StatusSucceeded)

	userText, _, _ := llmMock.snapshot()
	if strings.Contains(userText, "@data-analytics") {
		t.Fatalf("skill mention must be stripped from user text: %q", userText)
	}
	if !strings.Contains(userText, "build a dashboard") {
		t.Fatalf("user text missing cleaned input: %q", userText)
	}
}

// TestPostRunEmptySkillsClearsAgentDefaults asserts that an explicit
// "skills": [] in the request body deactivates the agent's default skills for
// this run. The observation point is the composed system prompt: when no skill
// is activated, skill.ComposeSystem omits the per-skill "## Skill: <id>"
// guidance section. The agent (a1) is configured with default skill
// "data-analytics"; sending skills: [] must therefore produce a system prompt
// that does NOT contain "## Skill: data-analytics".
func TestPostRunEmptySkillsClearsAgentDefaults(t *testing.T) {
	_, _, llmMock, h, _ := attachmentsServer(t, false)
	putAgent(t, h, "a1")

	rr := postRun(t, h, map[string]any{
		"agent_id":        "a1",
		"input":           "plain question",
		"conversation_id": "c1",
		"skills":          []string{},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	runID, _ := created["run_id"].(string)
	pollRunStatus(t, h, runID, store.StatusSucceeded)

	sys := llmMock.systemSnapshot()
	if strings.Contains(sys, "## Skill: data-analytics") {
		t.Fatalf("explicit skills:[] must deactivate agent default skill; system prompt still contains it: %q", sys)
	}
	if strings.Contains(sys, "use list_tickets for analytics") {
		t.Fatalf("explicit skills:[] must drop the default skill guidance body from the system prompt: %q", sys)
	}
}

// TestPostRunSkillMentionActivatesSkill asserts that an @id (or /id) mention in
// the input actually activates the referenced skill for the run, not just
// strips the marker from the user text. The observation point is the composed
// system prompt: an activated skill produces a "## Skill: <id>" section
// carrying the skill's guidance body. The agent (a1) is configured with
// default skill "data-analytics", but the input carries no body.skills, so
// activation here is driven solely by the @data-analytics mention.
func TestPostRunSkillMentionActivatesSkill(t *testing.T) {
	_, _, llmMock, h, _ := attachmentsServer(t, false)
	putAgent(t, h, "a1")

	rr := postRun(t, h, map[string]any{
		"agent_id":        "a1",
		"input":           "@data-analytics build a dashboard",
		"conversation_id": "c1",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	runID, _ := created["run_id"].(string)
	pollRunStatus(t, h, runID, store.StatusSucceeded)

	sys := llmMock.systemSnapshot()
	if !strings.Contains(sys, "## Skill: data-analytics") {
		t.Fatalf("mention must activate the skill; system prompt missing skill section: %q", sys)
	}
	if !strings.Contains(sys, "use list_tickets for analytics") {
		t.Fatalf("mention must activate the skill; system prompt missing skill guidance body: %q", sys)
	}
}
