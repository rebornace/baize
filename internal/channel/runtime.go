package channel

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/attach"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

// BusyReply is sent when the conversation already has an active run.
const BusyReply = "请稍候，上一轮还在处理"

// Validation errors for HandleInbound.
var (
	ErrNoAssignee = errors.New("channel: assignee is required")
	ErrNoPeer     = errors.New("channel: peer id is required")
	ErrNoAccount  = errors.New("channel: account is required in extras")
	ErrNoAgent    = errors.New("channel: default agent id is required")
)

// RunStore is the subset of store.Store needed for inbound runs.
type RunStore interface {
	CreateRun(in store.CreateRunInput) (*store.Run, error)
	HasActiveRun(conversationID string) (bool, error)
}

// Runtime turns normalized Channel inbound messages into conversation meta + runs.
type Runtime struct {
	Runs           RunStore
	Meta           conversation.MetaStore
	Messages       conversation.Store // optional; nil skips message append
	Assignee       string
	DefaultAgentID string
	// SupportsVision controls whether image attachments become multimodal parts.
	// When false, images are only named in the display/LLM text (design §5).
	SupportsVision bool
	// AfterCreateRun is an optional hook to start the engine after CreateRun.
	AfterCreateRun func(ctx context.Context, run *store.Run, userParts []llm.ContentPart) error

	tokenMu sync.Mutex
	tokens  map[string]string // conversation_id -> context_token
}

// HandleInbound maps a peer message to a conversation, replies busy if needed,
// otherwise EnsureMeta + CreateRun (and optional engine hook).
func (r *Runtime) HandleInbound(ctx context.Context, ch Channel, in Inbound) error {
	if r == nil {
		return errors.New("channel: nil runtime")
	}
	if ch == nil {
		return errors.New("channel: nil channel")
	}
	if r.Runs == nil {
		return errors.New("channel: run store is required")
	}
	if r.Meta == nil {
		return errors.New("channel: meta store is required")
	}
	assignee := strings.TrimSpace(r.Assignee)
	if assignee == "" {
		return ErrNoAssignee
	}
	peerID := strings.TrimSpace(in.PeerID)
	if peerID == "" {
		return ErrNoPeer
	}
	account := ""
	if in.Extras != nil {
		account = strings.TrimSpace(in.Extras["account"])
	}
	if account == "" {
		return ErrNoAccount
	}

	convID := "weixin:" + account + ":" + peerID
	if err := r.Meta.EnsureMeta(conversation.Meta{
		ID:          convID,
		OwnerID:     assignee,
		Source:      "weixin",
		ChannelPeer: peerID,
		UpdatedAt:   time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("channel: ensure meta: %w", err)
	}
	r.rememberContextToken(convID, in.Extras)

	busy, err := r.Runs.HasActiveRun(convID)
	if err != nil {
		return fmt.Errorf("channel: has active run: %w", err)
	}
	if busy {
		extras := copyExtras(in.Extras)
		if err := ch.SendText(ctx, peerID, BusyReply, extras); err != nil {
			return fmt.Errorf("channel: send busy reply: %w", err)
		}
		return nil
	}

	agentID := strings.TrimSpace(r.DefaultAgentID)
	if agentID == "" {
		return ErrNoAgent
	}

	displayText, userParts, err := buildInboundContent(in.Text, in.Files, r.SupportsVision)
	if err != nil {
		return err
	}

	runRec, err := r.Runs.CreateRun(store.CreateRunInput{
		AgentID:        agentID,
		Input:          displayText,
		ConversationID: convID,
	})
	if err != nil {
		return fmt.Errorf("channel: create run: %w", err)
	}

	if r.Messages != nil {
		_, _ = r.Messages.Append(convID, conversation.Message{
			Role:    conversation.RoleUser,
			Content: displayText,
			RunID:   runRec.ID,
		})
	}

	if r.AfterCreateRun != nil {
		if err := r.AfterCreateRun(ctx, runRec, userParts); err != nil {
			return fmt.Errorf("channel: after create run: %w", err)
		}
	}
	return nil
}

func copyExtras(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (r *Runtime) rememberContextToken(conversationID string, extras map[string]string) {
	if r == nil || extras == nil {
		return
	}
	tok := strings.TrimSpace(extras["context_token"])
	if tok == "" {
		return
	}
	r.tokenMu.Lock()
	defer r.tokenMu.Unlock()
	if r.tokens == nil {
		r.tokens = make(map[string]string)
	}
	r.tokens[conversationID] = tok
}

// OutboundExtras returns cached channel extras (e.g. context_token) for a conversation.
func (r *Runtime) OutboundExtras(conversationID string) map[string]string {
	if r == nil {
		return nil
	}
	r.tokenMu.Lock()
	defer r.tokenMu.Unlock()
	tok := r.tokens[conversationID]
	if tok == "" {
		return nil
	}
	return map[string]string{"context_token": tok}
}

// buildInboundContent mirrors internal/api handlePostRun attachment assembly:
// display text for persistence / CreateRun.Input, and optional multimodal parts
// for the engine hook.
func buildInboundContent(text string, files []InboundFile, supportsVision bool) (display string, parts []llm.ContentPart, err error) {
	text = strings.TrimSpace(text)
	if len(files) == 0 {
		return text, nil, nil
	}

	atts := make([]attach.AttachmentIn, 0, len(files))
	for _, f := range files {
		atts = append(atts, attach.AttachmentIn{
			Filename:   f.Name,
			MediaType:  f.MIME,
			ContentB64: base64.StdEncoding.EncodeToString(f.Data),
		})
	}
	textExts, imageExts, err := attach.Process(atts, attach.DefaultOptions())
	if err != nil {
		return "", nil, fmt.Errorf("channel: process attachments: %w", err)
	}

	llmText := text
	if len(textExts) > 0 || len(imageExts) > 0 {
		var b strings.Builder
		b.WriteString(text)
		for _, t := range textExts {
			b.WriteString("\n\n【附件: ")
			b.WriteString(t.Filename)
			b.WriteString("】\n")
			b.WriteString(t.Text)
		}
		if !supportsVision {
			for _, img := range imageExts {
				b.WriteString("\n\n【附件: ")
				b.WriteString(img.Filename)
				b.WriteString("】\n")
				b.WriteString("（图片，当前模型不支持视觉）")
			}
		}
		llmText = b.String()
		parts = append(parts, llm.ContentPart{Type: "text", Text: llmText})
		if supportsVision {
			for _, img := range imageExts {
				parts = append(parts, llm.ContentPart{
					Type:       "image",
					ImageMIME:  img.ImageMIME,
					ImageBytes: img.ImageBytes,
				})
			}
		}
	}

	display = text
	if n := len(textExts) + len(imageExts); n > 0 {
		names := make([]string, 0, n)
		for _, e := range textExts {
			names = append(names, e.Filename)
		}
		for _, e := range imageExts {
			names = append(names, e.Filename)
		}
		display = strings.TrimSpace(text) + "（附件：" + strings.Join(names, ", ") + "）"
	}
	return display, parts, nil
}
