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
	"strings"
	"sync"
	"testing"

	"github.com/rebornace/baize/internal/api"
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

func TestPostRunMarkdownAttachmentInjected(t *testing.T) {
	_, _, llmMock, h, _ := attachmentsServer(t, false)
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
	if !strings.Contains(userBubble, "notes.md") {
		t.Fatalf("persisted user bubble missing filename: %q", userBubble)
	}
	if strings.Contains(userBubble, "hello attachment body") {
		t.Fatalf("persisted user bubble must not contain extracted attachment text: %q", userBubble)
	}
}

func TestPostRunImageWithVisionSendsImagePart(t *testing.T) {
	_, _, llmMock, h, _ := attachmentsServer(t, true)
	putAgent(t, h, "a1")

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
			_, _, _, h, _ := attachmentsServer(t, vision)
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
