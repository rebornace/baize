package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/llm"
)

// TestOpenAIMultimodalContentArray verifies that a Message with image Parts is
// encoded as a content array containing a text part and an image_url part whose
// url is a base64 data URI.
func TestOpenAIMultimodalContentArray(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = b
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	p := llm.NewOpenAI(srv.URL, "k", "m")
	p.VisionSupported = true
	img := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	_, err := p.Chat(context.Background(), []llm.Message{
		{
			Role: llm.RoleUser,
			Parts: []llm.ContentPart{
				{Type: "text", Text: "看图"},
				{Type: "image", ImageMIME: "image/png", ImageBytes: img},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	var req struct {
		Messages []struct {
			Role    string        `json:"role"`
			Content []interface{} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(capturedBody, &req); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, capturedBody)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "user" {
		t.Fatalf("role: %s", req.Messages[0].Role)
	}
	if len(req.Messages[0].Content) != 2 {
		t.Fatalf("expected 2 content parts, got %d: %+v", len(req.Messages[0].Content), req.Messages[0].Content)
	}
	first, _ := req.Messages[0].Content[0].(map[string]any)
	if first["type"] != "text" || first["text"] != "看图" {
		t.Fatalf("text part: %+v", first)
	}
	second, _ := req.Messages[0].Content[1].(map[string]any)
	if second["type"] != "image_url" {
		t.Fatalf("image part type: %+v", second)
	}
	iu, _ := second["image_url"].(map[string]any)
	url, _ := iu["url"].(string)
	const wantPrefix = "data:image/png;base64,"
	if !strings.HasPrefix(url, wantPrefix) {
		t.Fatalf("url prefix: %s", url)
	}
	// The data URI suffix must decode back to the original bytes.
	if !strings.HasSuffix(url, "iVBORw0KGgo=") {
		t.Fatalf("url suffix: %s", url)
	}
}

// TestOpenAIDataURLPassthrough verifies that a ContentPart with DataURL set is
// forwarded verbatim instead of being re-encoded from ImageBytes.
func TestOpenAIDataURLPassthrough(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = b
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	p := llm.NewOpenAI(srv.URL, "k", "m")
	const dataURL = "data:image/jpeg;base64,/9j/4AAQ"
	_, err := p.Chat(context.Background(), []llm.Message{
		{
			Role: llm.RoleUser,
			Parts: []llm.ContentPart{
				{Type: "image", DataURL: dataURL},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if !strings.Contains(string(capturedBody), `"url":"`+dataURL+`"`) {
		t.Fatalf("body missing data url: %s", capturedBody)
	}
}

// TestOpenAIPlainTextContentIsString verifies backward compatibility: a
// message without Parts is encoded with content as a plain string, not an array.
func TestOpenAIPlainTextContentIsString(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = b
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	p := llm.NewOpenAI(srv.URL, "k", "m")
	_, err := p.Chat(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "hello"},
	}, nil)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if !strings.Contains(string(capturedBody), `"content":"hello"`) {
		t.Fatalf("body missing string content: %s", capturedBody)
	}
	if strings.Contains(string(capturedBody), `"content":[`) {
		t.Fatalf("content should not be array: %s", capturedBody)
	}
}

// TestOpenAISupportsVisionFlag verifies the SupportsVision() method reflects
// the struct field.
func TestOpenAISupportsVisionFlag(t *testing.T) {
	p := llm.NewOpenAI("http://x", "k", "m")
	if p.SupportsVision() {
		t.Fatalf("default should be false")
	}
	p.VisionSupported = true
	if !p.SupportsVision() {
		t.Fatalf("set true should be true")
	}
}

// TestMockSupportsVisionConfigurable verifies the Mock provider reports the
// configured vision support flag.
func TestMockSupportsVisionConfigurable(t *testing.T) {
	m := llm.NewMock()
	if m.SupportsVision() {
		t.Fatalf("default mock should not support vision")
	}
	m.VisionSupported = true
	if !m.SupportsVision() {
		t.Fatalf("mock should be configurable to support vision")
	}
}
