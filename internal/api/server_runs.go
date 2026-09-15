package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/artifact"
	"github.com/rebornace/baize/internal/attach"
	"github.com/rebornace/baize/internal/authcred"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
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
