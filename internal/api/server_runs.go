package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/artifact"
	"github.com/rebornace/baize/internal/attach"
	"github.com/rebornace/baize/internal/authcred"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/middleware"
	"github.com/rebornace/baize/internal/plugincallback"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/skillparse"
	"github.com/rebornace/baize/internal/store"
)

func (s *Server) handlePostRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AgentID        string                `json:"agent_id"`
		Input          string                `json:"input"`
		ConversationID string                `json:"conversation_id"`
		IdentityID     string                `json:"identity_id"`
		SessionToken   string                `json:"session_token"`
		WebhookURL     string                `json:"webhook_url"`
		WebhookHeaders map[string]string     `json:"webhook_headers"`
		Skills         []string              `json:"skills"`
		Attachments    []attach.AttachmentIn `json:"attachments"`
		ModelProfileID string                `json:"model_profile_id"`
		ThinkingLevel  string                `json:"thinking_level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	if strings.TrimSpace(body.AgentID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "agent_id is required")
		return
	}
	lvl := strings.ToLower(strings.TrimSpace(body.ThinkingLevel))
	if lvl != "" && lvl != "off" && lvl != "low" && lvl != "medium" && lvl != "high" {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid thinking_level")
		return
	}

	if _, err := s.Store.GetAgent(body.AgentID); err != nil {
		writeError(w, http.StatusNotFound, "agent_not_found", "unknown agent")
		return
	}

	// Omitting conversation_id keeps the machine path (default connector
	// headers apply; require_login is not enforced). Chat always sends an id.
	conv := strings.TrimSpace(body.ConversationID)
	identityID := strings.TrimSpace(body.IdentityID)
	runInput := body.Input
	if conv != "" && s.Identities != nil {
		cleaned, sessionID, err := identity.PrepareSessionAuth(s.Identities, conv, runInput, body.SessionToken)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		runInput = cleaned
		if identityID == "" && sessionID != "" {
			identityID = sessionID
		}
	}

	// Attachments: decode once (raw bytes preserved, upload order kept), then
	// extract model-bound text/images up-front so size/type/vision failures
	// reject the request before any run is created. The raw bytes are persisted
	// for UI preview/download; image bytes never enter SQLite.
	attachOpts := attach.DefaultOptions()
	decoded, err := attach.Decode(body.Attachments, attachOpts)
	if err != nil {
		s.writeAttachmentError(w, err)
		return
	}
	textExts, imageExts, err := attach.ProcessDecoded(decoded, attachOpts)
	if err != nil {
		s.writeAttachmentError(w, err)
		return
	}
	// Task-aware model routing. The per-run choice is either "auto" (smart
	// routing — the default and the only mode for channel inbound) or a
	// concrete profile id (manual). Auto classifies the turn's difficulty from
	// the actual content (text length, code, attachments, reasoning cues) and
	// picks a model in the matching capability tier, with neighbor fallback;
	// image turns are restricted to vision-capable models. A manual choice is
	// always honored exactly and NEVER silently rerouted, so a manual
	// text-only model on an image turn is rejected with guidance to pick Auto /
	// a vision model instead.
	profiles := s.routingProfiles()
	hasImages := len(imageExts) > 0
	sig := llm.TaskSignals{
		Text:      runInput,
		HasImages: hasImages,
		FileCount: len(textExts),
		HasCode:   llm.DetectCode(runInput),
	}
	sel, visionOK := llm.ResolveModel(llm.NormalizeProfileChoice(body.ModelProfileID), sig, profiles)
	if hasImages && !visionOK {
		writeError(w, http.StatusBadRequest, "vision_unsupported",
			"the selected model does not support vision; switch to 智能路由 (Auto) or a vision-capable model, or remove image attachments")
		return
	}
	modelProfileID := sel.ProfileID

	// Persist attachments to the per-conversation workspace (best-effort:
	// failures are logged and never block the turn). Saved logical paths are
	// listed so the model knows the files are available to read later. This
	// only applies to chat turns that carry a conversation id; the machine
	// path (empty conv) skips persistence entirely.
	var savedWorkspaceFiles []string
	if s.Workspace != nil && conv != "" {
		for _, t := range textExts {
			if p, perr := s.Workspace.SaveUpload(r.Context(), conv, t.Filename, t.Text); perr != nil {
				log.Printf("workspace: save text upload %q: %v", t.Filename, perr)
			} else {
				savedWorkspaceFiles = append(savedWorkspaceFiles, p)
			}
		}
		for _, im := range imageExts {
			if p, perr := s.Workspace.SaveUploadBytes(r.Context(), conv, im.Filename, im.ImageBytes, im.ImageMIME); perr != nil {
				log.Printf("workspace: save image upload %q: %v", im.Filename, perr)
			} else {
				savedWorkspaceFiles = append(savedWorkspaceFiles, p)
			}
		}
	}

	// Persist the uploads for UI preview/download (best-effort: a blob failure
	// never blocks the turn — the model still received extracted text / image
	// bytes via userParts). Images use the thumbnail-capped bytes; non-image
	// files keep their ORIGINAL bytes so they open/download unchanged even
	// though only their extracted text was sent to the model. Markers are built
	// in the original upload order.
	var bubbleMarkers []string
	if conv != "" && s.ChatMedia != nil && len(decoded) > 0 {
		imgIdx := 0 // imageExts are already in decoded order, images only
		for _, d := range decoded {
			if attach.IsImageMIME(d.MediaType) {
				if imgIdx >= len(imageExts) {
					continue
				}
				ex := imageExts[imgIdx]
				imgIdx++
				u, _, serr := s.ChatMedia.SaveInboundImage(r.Context(), conv, d.Filename, ex.ImageMIME, ex.ImageBytes)
				if serr != nil {
					log.Printf("chat media: save image upload %q: %v", d.Filename, serr)
					continue
				}
				bubbleMarkers = append(bubbleMarkers, "![图片]("+u+")")
				continue
			}
			u, _, serr := s.ChatMedia.SaveInboundFile(r.Context(), conv, d.Filename, d.MediaType, d.Data)
			if serr != nil {
				log.Printf("chat media: save file upload %q: %v", d.Filename, serr)
				continue
			}
			bubbleMarkers = append(bubbleMarkers, "[file:"+d.Filename+"]("+u+")")
		}
	}

	// Skill mentions (@id / /id) are stripped from the user text and merged
	// with body.skills. Omitting skills with no mentions keeps agent defaults;
	// any explicit input (non-nil body.skills or a mention) overrides for this
	// run, with an empty merged set meaning "no default skill active".
	cleanedInput, mentionIDs := skillparse.Parse(runInput)
	originalInput := strings.TrimSpace(runInput)
	mentionOnly := cleanedInput == "" && len(mentionIDs) > 0
	modelInput := cleanedInput
	if mentionOnly {
		modelInput = skillparse.MentionOnlyFallback
	}
	var runSkills []string
	if body.Skills != nil || len(mentionIDs) > 0 {
		merged := mergeSkillIDs(mentionIDs, body.Skills)
		if s.SkillCatalog != nil {
			var unknown []string
			for _, id := range merged {
				if _, ok := s.SkillCatalog.Get(id); !ok {
					unknown = append(unknown, id)
				}
			}
			if len(unknown) > 0 {
				writeError(w, http.StatusBadRequest, "unknown_skill",
					"unknown skill id: "+strings.Join(unknown, ", "))
				return
			}
		}
		runSkills = merged
	}

	// Build the LLM-bound user content (cleaned text + text-attachment blocks)
	// and the multimodal Parts payload when any attachments are present. The
	// persisted bubble stores only the visible text + filenames (no base64).
	llmText := modelInput
	var userParts []llm.ContentPart
	if len(textExts) > 0 || len(imageExts) > 0 {
		var b strings.Builder
		b.WriteString(modelInput)
		for _, t := range textExts {
			b.WriteString("\n\n【附件: ")
			b.WriteString(t.Filename)
			b.WriteString("】\n")
			b.WriteString(t.Text)
		}
		llmText = b.String()
		userParts = append(userParts, llm.ContentPart{Type: "text", Text: llmText})
		for _, img := range imageExts {
			userParts = append(userParts, llm.ContentPart{
				Type:       "image",
				ImageMIME:  img.ImageMIME,
				ImageBytes: img.ImageBytes,
			})
		}
	}
	// When attachments were persisted to the workspace, append the saved file
	// list to the LLM-bound content only (never to displayText, so the
	// persisted bubble shows just the user text + filenames).
	if len(savedWorkspaceFiles) > 0 {
		llmText = strings.TrimRight(llmText, "\n") +
			"\n\n[工作区] 以下文件已保存到本会话工作区，后续轮次可用 read_file（文本）或 read_image（图片）按需读取：" +
			strings.Join(savedWorkspaceFiles, ", ")
		if len(userParts) > 0 {
			userParts[0] = llm.ContentPart{Type: "text", Text: llmText}
		}
	}
	// run.Input / dispatched job / mirrored peer text carry only the clean
	// user text (attachment content is delivered to the model via userParts,
	// not via the input string). The persisted bubble appends renderable
	// attachment reference lines (inline images / download cards), so the UI
	// shows exactly what was sent with no redundant "（附件：…）" note.
	displayText := modelInput
	bubbleContent := cleanedInput
	if mentionOnly {
		bubbleContent = originalInput
	}
	if len(bubbleMarkers) > 0 {
		if bubbleContent != "" {
			bubbleContent += "\n"
		}
		bubbleContent += strings.Join(bubbleMarkers, "\n")
	}

	var passthrough map[string]string
	if authcred.NormalizeMode(s.AuthMode) == authcred.ModePassthrough {
		passthrough = authcred.PickHeaders(r.Header, s.AuthWhitelist)
	}
	var webhookCfg *store.WebhookConfig
	if u := strings.TrimSpace(body.WebhookURL); u != "" || len(body.WebhookHeaders) > 0 {
		webhookCfg = &store.WebhookConfig{
			URL:     u,
			Headers: body.WebhookHeaders,
		}
	}

	updated, err := s.startRun(r.Context(), startRunInput{
		AgentID:        body.AgentID,
		Input:          displayText,
		ConversationID: conv,
		IdentityID:     identityID,
		Skills:         runSkills,
		Webhook:        webhookCfg,
		Passthrough:    passthrough,
		UserParts:      userParts,
		ModelProfileID: modelProfileID,
		ThinkingLevel:  lvl,
		BubbleContent:  bubbleContent,
	})
	if err != nil {
		if err.Error() == "无权访问该会话" {
			writeError(w, http.StatusForbidden, "forbidden", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":          updated.ID,
		"status":          updated.Status,
		"conversation_id": conv,
	})
}

// runExecute dispatches to a RunWithOptions runner when available so per-run
// skills and multimodal user parts are honored; otherwise it falls back to the
// plain Execute path. Test fakes that only implement Runner still work.
func (s *Server) runExecute(ctx context.Context, runID string, def agent.Def, input string, opts run.RunOptions) error {
	if ex, ok := s.Runner.(RunWithOptions); ok {
		return ex.ExecuteWithOpts(ctx, runID, def, input, opts)
	}
	return s.Runner.Execute(ctx, runID, def, input)
}

// ExecuteJob runs a queued run under a worker lease with idempotency gating.
// It implements middleware.Executor: queue workers and the local fallback
// goroutine both funnel through it. Terminal or HITL-waiting runs are
// ack-skipped (resume is a separate entry point); a live lease held by
// another worker is also ack-skipped.
func (s *Server) ExecuteJob(ctx context.Context, job middleware.Job) error {
	if job.RunID == "" {
		return nil
	}
	cur, err := s.Store.GetRun(job.RunID)
	if err != nil || cur == nil {
		return err
	}
	switch cur.Status {
	case store.StatusSucceeded, store.StatusFailed, store.StatusCancelled, store.StatusRejected, store.StatusWaitingHuman:
		return nil
	}

	ttl := s.LeaseTTL
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	acquired, err := s.Store.LeaseRun(job.RunID, ttl)
	if err != nil {
		return err
	}
	if !acquired {
		return nil // another worker holds the lease
	}
	defer func() { _ = s.Store.ClearRunLease(job.RunID) }()

	hbCtx, stopHB := context.WithCancel(ctx)
	defer stopHB()
	go s.leaseHeartbeat(hbCtx, job.RunID, ttl)

	def, input, opts, err := s.resolveJob(job, cur)
	if err != nil {
		s.finalizeRunError(job.RunID, cur.ConversationID, err)
		return nil
	}
	if err := s.runExecute(ctx, job.RunID, def, input, opts); err != nil {
		// A human declining an approval is an intentional terminal outcome
		// (run settled as "rejected"), not a job failure to log/retry.
		if errors.Is(err, run.ErrHITLRejected) {
			return nil
		}
		s.finalizeRunError(job.RunID, cur.ConversationID, err)
		return err
	}
	return nil
}

// leaseHeartbeat renews the worker lease roughly every ttl/3 until ctx ends.
func (s *Server) leaseHeartbeat(ctx context.Context, runID string, ttl time.Duration) {
	t := time.NewTicker(ttl / 3)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = s.Store.HeartbeatRun(runID, ttl)
		}
	}
}

// resolveJob rebuilds the agent def, input, and run options for a queued job,
// falling back to the persisted run row when the job omits a field.
func (s *Server) resolveJob(job middleware.Job, cur *store.Run) (agent.Def, string, run.RunOptions, error) {
	agentID := strings.TrimSpace(job.AgentID)
	if agentID == "" {
		agentID = cur.AgentID
	}
	ag, err := s.Store.GetAgent(agentID)
	if err != nil {
		return agent.Def{}, "", run.RunOptions{}, err
	}
	def := agent.Def{ID: ag.ID, System: ag.System, Skills: append([]string(nil), ag.Skills...)}
	input := job.Input
	if strings.TrimSpace(input) == "" {
		input = cur.Input
	}
	opts := run.RunOptions{Skills: job.Skills, UserParts: partsFromMiddleware(job.UserParts)}
	return def, input, opts, nil
}

// finalizeRunError mirrors the legacy background-goroutine failure handling:
// re-read the run, and only when it is still queued/running mark it failed,
// append an LLM error event, and leave a system note in the conversation.
func (s *Server) finalizeRunError(runID, convID string, runErr error) {
	cur, _ := s.Store.GetRun(runID)
	if cur == nil {
		return
	}
	if cur.Status != store.StatusRunning && cur.Status != store.StatusQueued {
		return
	}
	_ = s.Store.UpdateRun(runID, store.StatusFailed, "", runErr.Error())
	_ = s.Store.AppendEvent(runID, store.Event{
		Type: run.EventLLMError,
		Data: map[string]any{"error": runErr.Error()},
	})
	if s.Messages != nil && convID != "" {
		note := strings.TrimSpace(runErr.Error())
		if note == "" {
			note = "运行失败"
		} else {
			note = "运行失败：" + note
		}
		_, _ = s.Messages.Append(convID, conversation.Message{
			Role:    conversation.RoleSystemNote,
			Content: note,
			RunID:   runID,
		})
	}
}

// Dispatch enqueues a job when a Queue is configured, falling back to a local
// goroutine (legacy behavior) on nil queue or enqueue failure.
func (s *Server) Dispatch(ctx context.Context, job middleware.Job) {
	if job.EnqueuedAt.IsZero() {
		job.EnqueuedAt = time.Now().UTC()
	}
	if s.Queue != nil {
		if err := s.Queue.Enqueue(ctx, job); err == nil {
			return
		}
	}
	go func() { _ = s.ExecuteJob(context.Background(), job) }()
}

// writeAttachmentError maps an attach.Process error to the spec's API error
// codes. Unsupported MIME, size/count limits, and empty-PDF each get a distinct
// code; everything else (e.g. bad base64) is reported as invalid_attachment.
func (s *Server) writeAttachmentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, attach.ErrUnsupported):
		writeError(w, http.StatusBadRequest, "unsupported_attachment", err.Error())
	case errors.Is(err, attach.ErrTooLarge):
		writeError(w, http.StatusBadRequest, "attachment_too_large", err.Error())
	case errors.Is(err, attach.ErrTooMany):
		writeError(w, http.StatusBadRequest, "too_many_attachments", err.Error())
	case errors.Is(err, attach.ErrEmptyPDFText):
		writeError(w, http.StatusBadRequest, "empty_pdf_text", err.Error())
	default:
		writeError(w, http.StatusBadRequest, "invalid_attachment", err.Error())
	}
}

// mergeSkillIDs concatenates mention IDs and body.skills, dropping empties and
// preserving first-seen order. It always returns a non-nil slice (empty when
// both inputs are empty) so callers can distinguish "explicit override with an
// empty set" from "omit / use agent defaults".
func mergeSkillIDs(mentionIDs, bodySkills []string) []string {
	seen := make(map[string]struct{}, len(mentionIDs)+len(bodySkills))
	out := make([]string, 0, len(mentionIDs)+len(bodySkills))
	for _, id := range mentionIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range bodySkills {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (s *Server) handlePostCancel(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing run id")
		return
	}
	runRec, err := s.Store.GetRun(id)
	if err != nil || runRec == nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	switch runRec.Status {
	case store.StatusQueued, store.StatusRunning, store.StatusWaitingHuman:
	default:
		writeError(w, http.StatusConflict, "not_active", "run is not active")
		return
	}
	canceller, ok := s.Runner.(RunCanceller)
	if !ok {
		writeError(w, http.StatusNotImplemented, "cancel_unavailable", "runner does not support cancel")
		return
	}
	if err := canceller.Cancel(id); err != nil {
		writeError(w, http.StatusConflict, "cancel_failed", err.Error())
		return
	}
	updated, err := s.Store.GetRun(id)
	if err != nil || updated == nil {
		writeJSON(w, http.StatusOK, map[string]string{"run_id": id, "status": string(store.StatusCancelled)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"run_id": updated.ID,
		"status": string(updated.Status),
	})
}

func (s *Server) handlePostResume(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	runRec, err := s.Store.GetRun(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	if !s.requireConversationAccess(w, r, runRec.ConversationID) {
		return
	}
	if runRec.Status != store.StatusWaitingHuman {
		writeError(w, http.StatusConflict, "not_waiting", "run is not waiting_human")
		return
	}

	var body struct {
		Decision string `json:"decision"`
		Comment  string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	approve := body.Decision == "approve"
	if body.Decision != "approve" && body.Decision != "reject" {
		writeError(w, http.StatusBadRequest, "invalid_request", "decision must be approve or reject")
		return
	}

	// In passthrough mode, refresh the run's passthrough headers from the
	// resume request before resuming execution. This must happen BEFORE
	// ContinueFromHITL so the engine's injectAuthCtxFromRun picks up the
	// fresh headers. An empty pick leaves the previously stored headers intact.
	if authcred.NormalizeMode(s.AuthMode) == authcred.ModePassthrough {
		if picked := authcred.PickHeaders(r.Header, s.AuthWhitelist); len(picked) > 0 {
			if err := s.Store.SetPassthroughHeaders(id, picked); err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
				return
			}
		}
	}

	_ = s.Runner.ContinueFromHITL(r.Context(), id, run.Decision{
		Approve: approve,
		Comment: body.Comment,
	})

	updated, err := s.Store.GetRun(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id": updated.ID,
		"status": updated.Status,
	})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	runRec, err := s.Store.GetRun(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	if !s.requireConversationAccess(w, r, runRec.ConversationID) {
		return
	}
	writeJSON(w, http.StatusOK, runRec)
}

// maxPluginCallbackPayloadBytes is the upper bound on the serialized payload
// field of a plugin callback body (spec §3.4: 64KiB). The whole body is
// bounded by http.MaxBytesReader; the payload itself is checked after decode.
const maxPluginCallbackPayloadBytes = 64 * 1024

// handlePluginCallback receives a sidecar plugin event for a specific Run.
// Auth is by HMAC token only (ACL: RoleNone); the control-plane gate never
// applies. Flow: verify token → run exists → rate limiter → decode → size
// check → append event → 204.
func (s *Server) handlePluginCallback(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	if runID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing run id")
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" || len(s.CallbackSecret) == 0 {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing callback token")
		return
	}
	if err := plugincallback.Verify(s.CallbackSecret, runID, token, time.Now()); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid callback token")
		return
	}
	if _, err := s.Store.GetRun(runID); err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	if s.CallbackGate != nil {
		if !s.CallbackGate(runID) {
			writeError(w, http.StatusTooManyRequests, "rate_limited", "callback budget exhausted for this run")
			return
		}
	} else if s.CallbackLimiter != nil && !s.CallbackLimiter.Allow(runID, time.Now()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "callback budget exhausted for this run")
		return
	}

	var body struct {
		Type    string         `json:"type"`
		Name    string         `json:"name"`
		Payload map[string]any `json:"payload"`
	}
	// Cap the whole request body at 1MiB so a hostile sidecar cannot stream
	// gigabytes into the decoder; the payload field is size-checked after.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing name")
		return
	}
	var payloadBytes int
	if body.Payload != nil {
		raw, err := json.Marshal(body.Payload)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "payload not serializable")
			return
		}
		payloadBytes = len(raw)
	}
	if payloadBytes > maxPluginCallbackPayloadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "payload exceeds 64KiB")
		return
	}

	// Event.Type is the fixed classification; the sidecar's body.type is the
	// event's own subtype, preserved as data.type for downstream consumers.
	data := map[string]any{
		"name":    body.Name,
		"payload": body.Payload,
	}
	if t := strings.TrimSpace(body.Type); t != "" {
		data["type"] = t
	}
	if err := s.Store.AppendEvent(runID, store.Event{
		Type: "plugin.callback",
		Data: data,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "append event failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetArtifact(w http.ResponseWriter, r *http.Request) {
	if s.Artifacts == nil {
		writeError(w, http.StatusNotFound, "artifact_not_found", "artifact not found")
		return
	}
	id := r.PathValue("id")
	html, runID, err := s.Artifacts.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, artifact.ErrNotFound) {
			writeError(w, http.StatusNotFound, "artifact_not_found", "artifact not found")
			return
		}
		log.Printf("artifact: get %s failed: %v", id, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "get artifact failed")
		return
	}
	if _, err := s.Store.GetRun(runID); err != nil {
		writeError(w, http.StatusNotFound, "artifact_not_found", "artifact not found")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

func (s *Server) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	runRec, err := s.Store.GetRun(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	if !s.requireConversationAccess(w, r, runRec.ConversationID) {
		return
	}
	evs, err := s.Store.ListEvents(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	if evs == nil {
		evs = []store.Event{}
	}
	writeJSON(w, http.StatusOK, evs)
}

// ssePollInterval is the SSE fallback poll period when Hub nudges are missing
// (e.g. cross-replica AppendEvent). Tests may shorten it via t.Cleanup restore.
var ssePollInterval = 3 * time.Second

func (s *Server) handleRunStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	runRec, err := s.Store.GetRun(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	if !s.requireConversationAccess(w, r, runRec.ConversationID) {
		return
	}

	after := parseStreamAfter(r)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)

	evs, err := s.Store.ListEvents(id)
	if err != nil {
		return
	}
	lastSent := after
	for i, ev := range evs {
		if i <= after {
			continue
		}
		if err := writeSSEEvent(w, rc, i, ev); err != nil {
			return
		}
		lastSent = i
	}

	terminal := runRec.Status == store.StatusSucceeded || runRec.Status == store.StatusFailed || runRec.Status == store.StatusRejected
	if terminal {
		_ = writeSSEEnded(w, rc, runRec.Status)
		return
	}
	if s.Hub == nil {
		// Replay-only: cannot subscribe for live increments.
		return
	}

	sub := s.Hub.Subscribe(id)
	defer sub.Cancel()

	// Fan-out before Subscribe is dropped; drain buffer then re-read store.
	var drainErr error
	lastSent, drainErr = drainSubEvents(w, rc, sub, lastSent)
	if drainErr != nil {
		return
	}
	if catchUpTerminal, status, err := catchUpRunStream(w, rc, s.Store, id, &lastSent); err != nil {
		return
	} else if catchUpTerminal {
		_ = writeSSEEnded(w, rc, status)
		return
	}
	lastSent, drainErr = drainSubEvents(w, rc, sub, lastSent)
	if drainErr != nil {
		return
	}

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	poll := time.NewTicker(ssePollInterval)
	defer poll.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return
			}
			_ = rc.Flush()
		case <-poll.C:
			if catchUpTerminal, status, err := catchUpRunStream(w, rc, s.Store, id, &lastSent); err != nil {
				return
			} else if catchUpTerminal {
				_ = writeSSEEnded(w, rc, status)
				return
			}
		case ev, ok := <-sub.Events:
			if !ok {
				return
			}
			if ev.Event.Type == "external.nudge" {
				if catchUpTerminal, status, err := catchUpRunStream(w, rc, s.Store, id, &lastSent); err != nil {
					return
				} else if catchUpTerminal {
					_ = writeSSEEnded(w, rc, status)
					return
				}
				continue
			}
			if ev.Index <= lastSent {
				continue
			}
			if err := writeSSEEvent(w, rc, ev.Index, ev.Event); err != nil {
				return
			}
			lastSent = ev.Index
		case stt, ok := <-sub.Ended:
			if !ok {
				return
			}
			// Events and Ended may both be ready; drain events first so the
			// final AppendEvent is not lost when select picks Ended. Then
			// catch up from store: channel may only hold external.nudge
			// placeholders while real events live in the store.
			lastSent, drainErr = drainSubEvents(w, rc, sub, lastSent)
			if drainErr != nil {
				return
			}
			if _, _, err := catchUpRunStream(w, rc, s.Store, id, &lastSent); err != nil {
				return
			}
			_ = writeSSEEnded(w, rc, stt)
			return
		}
	}
}

// drainSubEvents non-blocking writes any buffered subscription events with index > lastSent.
// external.nudge placeholders are skipped (no write / no lastSent advance); callers
// catch up from the store via catchUpRunStream.
func drainSubEvents(w http.ResponseWriter, rc *http.ResponseController, sub *eventbus.Subscription, lastSent int) (int, error) {
	for {
		select {
		case ev, ok := <-sub.Events:
			if !ok {
				return lastSent, nil
			}
			if ev.Event.Type == "external.nudge" {
				continue
			}
			if ev.Index <= lastSent {
				continue
			}
			if err := writeSSEEvent(w, rc, ev.Index, ev.Event); err != nil {
				return lastSent, err
			}
			lastSent = ev.Index
		default:
			return lastSent, nil
		}
	}
}

// catchUpRunStream re-reads the store after Subscribe to recover events/status
// published in the ListEvents→Subscribe window (no subscriber yet).
func catchUpRunStream(w http.ResponseWriter, rc *http.ResponseController, st store.Store, id string, lastSent *int) (terminal bool, status store.Status, err error) {
	runRec, err := st.GetRun(id)
	if err != nil {
		return false, "", err
	}
	evs, err := st.ListEvents(id)
	if err != nil {
		return false, "", err
	}
	for i, ev := range evs {
		if i <= *lastSent {
			continue
		}
		if err := writeSSEEvent(w, rc, i, ev); err != nil {
			return false, "", err
		}
		*lastSent = i
	}
	if runRec.Status == store.StatusSucceeded || runRec.Status == store.StatusFailed || runRec.Status == store.StatusRejected {
		return true, runRec.Status, nil
	}
	return false, "", nil
}

func parseStreamAfter(r *http.Request) int {
	after := -1
	if q := r.URL.Query().Get("after"); q != "" {
		if n, err := strconv.Atoi(q); err == nil {
			after = n
		} else {
			after = -1
		}
	}
	if id := r.Header.Get("Last-Event-ID"); id != "" {
		if n, err := strconv.Atoi(id); err == nil {
			after = n
		} else {
			after = -1
		}
	}
	return after
}

func writeSSEEvent(w http.ResponseWriter, rc *http.ResponseController, index int, ev store.Event) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", index, payload); err != nil {
		return err
	}
	return rc.Flush()
}

func writeSSEEnded(w http.ResponseWriter, rc *http.ResponseController, status store.Status) error {
	payload, err := json.Marshal(map[string]string{"status": string(status)})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: run.ended\ndata: %s\n\n", payload); err != nil {
		return err
	}
	return rc.Flush()
}

type executeRunOpts struct {
	skipUserAppend bool
	rebindUserID   string
	// runOpts carries per-run skill overrides and multimodal user parts. The
	// regenerate path leaves this zero-valued (reuse agent defaults, no parts)
	// since attachments are not re-sent on regenerate.
	runOpts run.RunOptions
}

// createAndExecuteRun creates a run, appends the user message, and starts Execute in background.
func (s *Server) createAndExecuteRun(r *http.Request, agentID, input, convID, identityID string, opts executeRunOpts) (*store.Run, error) {
	ag, err := s.Store.GetAgent(agentID)
	if err != nil {
		return nil, fmt.Errorf("unknown agent")
	}
	if err := s.prepareConversationMeta(r.Context(), convID); err != nil {
		return nil, err
	}
	createIn := store.CreateRunInput{
		AgentID:        agentID,
		Input:          input,
		ConversationID: convID,
		IdentityID:     strings.TrimSpace(identityID),
	}
	if authcred.NormalizeMode(s.AuthMode) == authcred.ModePassthrough {
		createIn.PassthroughHeaders = authcred.PickHeaders(r.Header, s.AuthWhitelist)
	}
	runRec, err := s.Store.CreateRun(createIn)
	if err != nil {
		return nil, err
	}
	if opts.skipUserAppend && opts.rebindUserID != "" {
		if s.Messages == nil {
			return nil, fmt.Errorf("message store not configured")
		}
		if err := s.Messages.SetRunID(opts.rebindUserID, runRec.ID); err != nil {
			return nil, err
		}
	} else if s.Messages != nil && convID != "" {
		_, _ = s.Messages.Append(convID, conversation.Message{
			Role:    conversation.RoleUser,
			Content: input,
			RunID:   runRec.ID,
		})
	}
	def := agent.Def{ID: ag.ID, System: ag.System, Skills: append([]string(nil), ag.Skills...)}
	_ = s.Store.AppendEvent(runRec.ID, store.Event{Type: run.EventRunStarted})
	s.Dispatch(r.Context(), middleware.Job{
		RunID:     runRec.ID,
		Kind:      middleware.KindRun,
		AgentID:   def.ID,
		Input:     input,
		Skills:    opts.runOpts.Skills,
		UserParts: PartsToMiddleware(opts.runOpts.UserParts),
	})
	return runRec, nil
}
