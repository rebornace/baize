package channel

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
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
	WaitingHumanRun(conversationID string) (*store.Run, error)
}

// Runtime turns normalized Channel inbound messages into conversation meta + runs.
type Runtime struct {
	Runs           RunStore
	Meta           conversation.MetaStore
	Messages       conversation.Store // optional; nil skips message append
	Assignee       string
	DefaultAgentID string
	// ResolveModel performs task-aware Auto routing for an inbound turn. Read
	// the doc on BuildDeps.ResolveModel. nil means no profile source is wired
	// (tests / non-LLM paths): images then degrade to text notes.
	ResolveModel func(sig llm.TaskSignals) (profileID string, visionOK, hasModels bool)
	// AfterCreateRun is an optional hook to start the engine after CreateRun.
	AfterCreateRun func(ctx context.Context, run *store.Run, userParts []llm.ContentPart) error
	// ResumeHITL continues a waiting_human run (approve/reject). Optional;
	// when nil, waiting_human inbound only gets HITLHelpReply.
	ResumeHITL func(ctx context.Context, runID string, approve bool, comment string) error
	// Source is the meta.Source / conv-id prefix for this channel (e.g. "weixin").
	// Empty defaults to "weixin" to preserve historical conversation ids.
	Source string

	// Media optionally persists inbound channel images so they render inline in
	// the web UI. nil = images are only sent to a vision-capable model and
	// named in text (no inline display).
	Media MediaStore

	// routeMu guards Assignee/DefaultAgentID hot updates (SetRouting) against
	// concurrent reads in HandleInbound. Construction-time writes before the
	// Runtime is published to the HTTP server are unsynchronized by design.
	routeMu sync.RWMutex

	tokenMu sync.Mutex
	tokens  map[string]string // conversation_id -> context_token
	accts   map[string]string // conversation_id -> account
}

// SetRouting hot-updates the assignee and default agent id. Empty/blank
// values are ignored so a partial update cannot blank a routing field.
// Safe to call concurrently with HandleInbound.
func (r *Runtime) SetRouting(assignee, agentID string) {
	r.routeMu.Lock()
	defer r.routeMu.Unlock()
	if id := strings.TrimSpace(assignee); id != "" {
		r.Assignee = id
	}
	if id := strings.TrimSpace(agentID); id != "" {
		r.DefaultAgentID = id
	}
}

// routing returns the current assignee and default agent id as a locked
// snapshot, safe to read while SetRouting updates them.
func (r *Runtime) routing() (assignee, agentID string) {
	r.routeMu.RLock()
	defer r.routeMu.RUnlock()
	return r.Assignee, r.DefaultAgentID
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
	// Locked snapshot: assignee/default agent may be hot-updated concurrently
	// by SetRouting, so never read the fields directly in this goroutine.
	assignee, defaultAgent := r.routing()
	assignee = strings.TrimSpace(assignee)
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

	src := strings.TrimSpace(r.Source)
	if src == "" {
		src = "weixin"
	}
	convID := ConvID(src, account, peerID)
	if err := r.Meta.EnsureMeta(conversation.Meta{
		ID:          convID,
		OwnerID:     assignee,
		Source:      src,
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
		return r.handleBusyInbound(ctx, ch, convID, peerID, in)
	}

	agentID := strings.TrimSpace(defaultAgent)
	if agentID == "" {
		return ErrNoAgent
	}

	// Task-aware smart model routing. Channel inbound has no manual picker, so
	// it is always Auto: the resolver classifies the turn (difficulty tier +
	// image need) and picks a model. We first classify on the raw payload to
	// learn whether a vision model is reachable — that decides whether images
	// are encoded as multimodal parts or degraded to text notes. After
	// extraction we resolve again against the parts actually delivered, so a
	// turn whose images all fail to parse is treated as text-only (it neither
	// pins a vision-only model nor needs vision). A nil resolver (tests / no
	// profile source) degrades images to text.
	hasImages := r.hasImage(in.Files)
	sig := llm.TaskSignals{
		Text:      in.Text,
		HasImages: hasImages,
		FileCount: r.countNonImageFiles(in.Files),
		HasCode:   llm.DetectCode(in.Text),
	}
	visionReachable := false
	if r.ResolveModel != nil {
		if _, ok, _ := r.ResolveModel(sig); ok {
			visionReachable = true
		}
	}
	deliverVision := hasImages && visionReachable

	displayText, userParts, images, err := buildInboundContent(in.Text, in.Files, deliverVision)
	if err != nil {
		return err
	}
	runModelProfileID := ""
	if r.ResolveModel != nil {
		// Re-resolve against what was actually delivered: when no image part
		// reaches the model (images degraded / failed to parse), treat the turn
		// as text-only so it is not pinned to a vision-only model.
		sig.HasImages = containsImagePart(userParts)
		if id, _, _ := r.ResolveModel(sig); id != "" {
			runModelProfileID = strings.TrimSpace(id)
		}
	}

	runRec, err := r.Runs.CreateRun(store.CreateRunInput{
		AgentID:        agentID,
		Input:          displayText,
		ConversationID: convID,
		ModelProfileID: runModelProfileID,
	})
	if err != nil {
		return fmt.Errorf("channel: create run: %w", err)
	}

	// Persist inbound attachments for the web UI and append references to the
	// stored user bubble: images render inline, other files (docx/pdf/zip/…
	// including types the model cannot parse) are kept for download.
	// Best-effort: a blob failure must not drop the run/reply (the model still
	// received extracted text / image bytes via userParts). References are NOT
	// added to run.Input (the LLM gets content via userParts, not URLs).
	bubbleContent := displayText
	if r.Messages != nil && r.Media != nil {
		for _, img := range images {
			u, _, serr := r.Media.SaveInboundImage(ctx, convID, img.Filename, img.ImageMIME, img.ImageBytes)
			if serr != nil {
				log.Printf("channel: persist inbound image %s: %v", img.Filename, serr)
				continue
			}
			bubbleContent += "\n![图片](" + u + ")"
		}
		// Persist the original bytes of every non-image attachment so the
		// operator can open/download it even when its contents were not (or
		// only partially) extracted for the model.
		for _, f := range in.Files {
			if strings.HasPrefix(strings.TrimSpace(f.MIME), "image/") {
				continue // images handled above (thumbnail-capped)
			}
			name := strings.TrimSpace(f.Name)
			if name == "" {
				name = "file"
			}
			u, _, serr := r.Media.SaveInboundFile(ctx, convID, name, f.MIME, f.Data)
			if serr != nil {
				log.Printf("channel: persist inbound file %s: %v", name, serr)
				continue
			}
			bubbleContent += "\n[file:" + name + "](" + u + ")"
		}
		_, _ = r.Messages.Append(convID, conversation.Message{
			Role:    conversation.RoleUser,
			Content: bubbleContent,
			RunID:   runRec.ID,
		})
	} else if r.Messages != nil {
		_, _ = r.Messages.Append(convID, conversation.Message{
			Role:    conversation.RoleUser,
			Content: bubbleContent,
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

func (r *Runtime) handleBusyInbound(ctx context.Context, ch Channel, convID, peerID string, in Inbound) error {
	extras := copyExtras(in.Extras)
	waiting, err := r.Runs.WaitingHumanRun(convID)
	if err != nil {
		return fmt.Errorf("channel: waiting human run: %w", err)
	}
	if waiting != nil {
		if approve, ok := ParseHITLDecision(in.Text); ok && r.ResumeHITL != nil {
			if err := r.ResumeHITL(ctx, waiting.ID, approve, strings.TrimSpace(in.Text)); err != nil {
				return fmt.Errorf("channel: resume hitl: %w", err)
			}
			ack := "已拒绝，运行将结束。"
			if approve {
				ack = "已批准，继续处理中…"
			}
			if err := ch.SendText(ctx, peerID, ack, extras); err != nil {
				return fmt.Errorf("channel: send hitl ack: %w", err)
			}
			return nil
		}
		if err := ch.SendText(ctx, peerID, HITLHelpReply, extras); err != nil {
			return fmt.Errorf("channel: send hitl help: %w", err)
		}
		return nil
	}
	if err := ch.SendText(ctx, peerID, BusyReply, extras); err != nil {
		return fmt.Errorf("channel: send busy reply: %w", err)
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
	r.tokenMu.Lock()
	defer r.tokenMu.Unlock()
	if r.tokens == nil {
		r.tokens = make(map[string]string)
	}
	if r.accts == nil {
		r.accts = make(map[string]string)
	}
	if tok := strings.TrimSpace(extras["context_token"]); tok != "" {
		r.tokens[conversationID] = tok
	}
	if acct := strings.TrimSpace(extras["account"]); acct != "" {
		r.accts[conversationID] = acct
	}
}

// OutboundExtras returns cached channel extras (e.g. context_token, account) for a conversation.
func (r *Runtime) OutboundExtras(conversationID string) map[string]string {
	if r == nil {
		return nil
	}
	r.tokenMu.Lock()
	defer r.tokenMu.Unlock()
	tok := r.tokens[conversationID]
	acct := r.accts[conversationID]
	if tok == "" && acct == "" {
		return nil
	}
	out := map[string]string{}
	if tok != "" {
		out["context_token"] = tok
	}
	if acct != "" {
		out["account"] = acct
	}
	return out
}

// hasImage reports whether the inbound files include an image attachment
// (MIME image/*). Routing to a vision model is driven by the actual payload —
// a docx/pdf that only extracts text never triggers it.
func (r *Runtime) hasImage(files []InboundFile) bool {
	for _, f := range files {
		if strings.HasPrefix(strings.TrimSpace(f.MIME), "image/") {
			return true
		}
	}
	return false
}

// countNonImageFiles returns how many attachments are not images (docx/pdf/
// zip/…). It feeds the task-difficulty classifier (many files -> harder).
func (r *Runtime) countNonImageFiles(files []InboundFile) int {
	n := 0
	for _, f := range files {
		if !strings.HasPrefix(strings.TrimSpace(f.MIME), "image/") {
			n++
		}
	}
	return n
}

// containsImagePart reports whether the assembled LLM payload actually carries
// an image part.
func containsImagePart(parts []llm.ContentPart) bool {
	for _, p := range parts {
		if p.Type == "image" {
			return true
		}
	}
	return false
}

// buildInboundContent mirrors internal/api handlePostRun attachment assembly:
// display text for persistence / CreateRun.Input, and optional multimodal parts
// for the engine hook.
//
// Unlike the interactive web upload (which surfaces an error to the operator
// and asks them to retry), an inbound channel message must never be silently
// dropped because a file is unsupported: a WeChat user sending a .zip or an
// oversized file would otherwise get no run and no reply. Unsupported /
// oversized / undecodable files are therefore recorded by name as a note for
// the agent (so it can acknowledge them), while supported files are extracted
// as usual.
func buildInboundContent(text string, files []InboundFile, supportsVision bool) (display string, parts []llm.ContentPart, images []attach.Extracted, err error) {
	text = strings.TrimSpace(text)
	if len(files) == 0 {
		return text, nil, nil, nil
	}

	opts := attach.DefaultOptions()
	processable := make([]attach.AttachmentIn, 0, len(files))
	skipped := make([]string, 0) // display names of files we could not parse
	for _, f := range files {
		dispName := strings.TrimSpace(f.Name)
		if dispName == "" {
			dispName = "file"
		}
		if int64(len(f.Data)) > int64(opts.MaxTotalBytes) {
			skipped = append(skipped, dispName)
			continue
		}
		if !attach.SupportsMediaType(f.MIME) {
			skipped = append(skipped, dispName)
			continue
		}
		processable = append(processable, attach.AttachmentIn{
			Filename:   f.Name,
			MediaType:  f.MIME,
			ContentB64: base64.StdEncoding.EncodeToString(f.Data),
		})
	}

	var textExts, imageExts []attach.Extracted
	if len(processable) > 0 {
		textExts, imageExts, err = attach.Process(processable, opts)
		if err != nil {
			// A supported type failed to parse (corrupt docx/pdf, bad image):
			// degrade to name-only notes rather than aborting the whole message.
			for _, a := range processable {
				skipped = append(skipped, a.Filename)
			}
			textExts, imageExts = nil, nil
		}
	}

	llmText := text
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
	for _, name := range skipped {
		b.WriteString("\n\n【附件: ")
		b.WriteString(name)
		b.WriteString("】\n（已收到该文件，但暂不支持解析其内容，无法读取其中的信息。）")
	}
	if len(textExts) > 0 || len(imageExts) > 0 || len(skipped) > 0 {
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

	names := make([]string, 0, len(textExts)+len(imageExts)+len(skipped))
	for _, e := range textExts {
		names = append(names, e.Filename)
	}
	for _, e := range imageExts {
		names = append(names, e.Filename)
	}
	names = append(names, skipped...)
	display = text
	if len(names) > 0 {
		display = strings.TrimSpace(text) + "（附件：" + strings.Join(names, ", ") + "）"
	}
	// imageExts are thumbnail-capped/re-encoded images; returned so the caller
	// can persist them for inline web display regardless of supportsVision
	// (displaying to an operator is independent of the model seeing the image).
	return display, parts, imageExts, nil
}
