package hostedlogin

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
)

const exportConvPrefix = "mcp-export-id:"

// Invoker is the subset of tool.Registry needed to call a login tool.
type Invoker interface {
	Invoke(ctx context.Context, name string, args map[string]any) (map[string]any, bool, error)
}

// SubmitDeps wires persistence and runtime identity refresh for hosted login.
type SubmitDeps struct {
	Sessions   Sessions
	Store      store.Store
	Registry   Invoker
	Identities identity.Store
	Audit      Auditor
	Waiter     Waiter
}

// Submit maps form fields to credentials (via login tool or direct headers),
// persists them into the session target, and deletes the session on success.
func Submit(ctx context.Context, deps SubmitDeps, sessionID, csrf string, form map[string]string) error {
	if deps.Sessions == nil || deps.Store == nil {
		return fmt.Errorf("hostedlogin: incomplete deps")
	}
	sess, ok := deps.Sessions.Get(sessionID)
	if !ok {
		return fmt.Errorf("hostedlogin: session not found or expired")
	}
	if strings.TrimSpace(csrf) == "" || csrf != sess.CSRF {
		return fmt.Errorf("hostedlogin: invalid csrf")
	}
	convID := strings.TrimSpace(sess.ConversationID)
	exportIdentityID := strings.TrimSpace(sess.IdentityID)
	if convID == "" && exportIdentityID == "" {
		return fmt.Errorf("hostedlogin: session missing identity target")
	}

	headers, label, scheme, err := collectCredential(ctx, deps, sess, form)
	if err != nil {
		audit(deps.Audit, AuditEvent{
			Kind: AuditSubmitFail, SessionID: sessionID,
			ConnectorID: sess.ConnectorID, ConversationID: convID,
			ExportKeyID: sess.ExportKeyID, Err: err.Error(),
		})
		return err
	}

	if err := persistCredential(deps, sess, convID, exportIdentityID, headers, label, scheme); err != nil {
		audit(deps.Audit, AuditEvent{
			Kind: AuditSubmitFail, SessionID: sessionID,
			ConnectorID: sess.ConnectorID, ConversationID: convID,
			ExportKeyID: sess.ExportKeyID, Err: err.Error(),
		})
		return err
	}
	deps.Sessions.Delete(sessionID)
	if deps.Waiter != nil {
		deps.Waiter.NotifyOK(sessionID)
	}
	audit(deps.Audit, AuditEvent{
		Kind: AuditSubmitOK, SessionID: sessionID,
		ConnectorID: sess.ConnectorID, ConversationID: convID,
		ExportKeyID: sess.ExportKeyID,
	})
	return nil
}

func collectCredential(ctx context.Context, deps SubmitDeps, sess Session, form map[string]string) (headers map[string]string, label, scheme string, err error) {
	mode := strings.TrimSpace(sess.Mode)
	switch mode {
	case ModeHeaderPairs:
		h := headersFromPairs(sess, form)
		if len(h) == 0 {
			return nil, "", "", fmt.Errorf("hostedlogin: no header values provided")
		}
		return h, "", "header", nil
	case ModeBasic:
		if strings.TrimSpace(sess.LoginTool) == "" {
			user, pass := basicCredentials(sess, form)
			if user == "" && pass == "" {
				return nil, "", "", fmt.Errorf("hostedlogin: username and password required")
			}
			token := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
			return map[string]string{"Authorization": "Basic " + token}, user, "basic", nil
		}
		// Fall through to login-tool invoke with mapped args.
	case ModeTokenPaste, ModePasswordForm, "":
		// invoke path below
	default:
		return nil, "", "", fmt.Errorf("hostedlogin: unsupported mode %q", mode)
	}

	if deps.Registry == nil {
		return nil, "", "", fmt.Errorf("hostedlogin: incomplete deps")
	}
	if strings.TrimSpace(sess.LoginTool) == "" {
		return nil, "", "", fmt.Errorf("hostedlogin: session missing login tool")
	}
	args := mapFormToArgs(sess, form)
	content, isError, invErr := deps.Registry.Invoke(ctx, sess.LoginTool, args)
	if invErr != nil {
		return nil, "", "", fmt.Errorf("hostedlogin: login failed: %w", invErr)
	}
	if isError {
		msg := "login failed"
		if content != nil {
			if m, ok := content["message"].(string); ok && strings.TrimSpace(m) != "" {
				msg = m
			} else if m, ok := content["error"].(string); ok && strings.TrimSpace(m) != "" {
				msg = m
			}
		}
		return nil, "", "", fmt.Errorf("hostedlogin: %s", msg)
	}
	conn, err := deps.Store.GetConnector(sess.ConnectorID)
	if err != nil {
		return nil, "", "", fmt.Errorf("hostedlogin: connector: %w", err)
	}
	captureCfg := captureDefaults(captureAuthToConfig(conn.Auth.Capture))
	h, lab, _, _, ok := identity.ExtractCredential(captureCfg, content)
	if !ok || len(h) == 0 {
		return nil, "", "", fmt.Errorf("hostedlogin: login succeeded but no credential could be parsed")
	}
	sch := strings.TrimSpace(captureCfg.DefaultScheme)
	if sch == "" {
		sch = "bearer"
	}
	return h, lab, sch, nil
}

func persistCredential(
	deps SubmitDeps,
	sess Session,
	convID, exportIdentityID string,
	headers map[string]string,
	label, scheme string,
) error {
	if convID != "" {
		if deps.Identities == nil {
			return fmt.Errorf("hostedlogin: identities store unavailable")
		}
		sch := scheme
		if sch == "" {
			sch = "bearer"
		}
		if _, err := deps.Identities.Upsert(identity.Scope(convID), identity.Identity{
			Label:             label,
			Scheme:            sch,
			Subject:           label,
			CredentialHeaders: headers,
			Source:            identity.SourceLoginCapture,
			IsDefault:         true,
		}); err != nil {
			return fmt.Errorf("hostedlogin: persist conversation identity: %w", err)
		}
		return nil
	}

	exportID, err := deps.Store.GetMCPExportIdentity(exportIdentityID)
	if err != nil {
		return fmt.Errorf("hostedlogin: identity: %w", err)
	}
	exportID.Headers = headers
	if scheme != "" {
		exportID.Scheme = scheme
	} else if exportID.Scheme == "" {
		exportID.Scheme = "Bearer"
	}
	if label != "" && strings.TrimSpace(exportID.Name) == "" {
		exportID.Name = label
	}
	if err := deps.Store.UpsertMCPExportIdentity(exportID); err != nil {
		return fmt.Errorf("hostedlogin: persist identity: %w", err)
	}
	if deps.Identities != nil {
		if _, err := deps.Identities.Upsert(exportConvPrefix+exportID.ID, exportToRuntimeIdentity(exportID)); err != nil {
			return fmt.Errorf("hostedlogin: refresh runtime identity: %w", err)
		}
	}
	_ = sess
	return nil
}

func headersFromPairs(sess Session, form map[string]string) map[string]string {
	fields := sess.Fields
	if len(fields) == 0 {
		fields = DefaultHeaderPairFields()
	}
	out := map[string]string{}
	for _, f := range fields {
		header := strings.TrimSpace(f.Arg)
		if header == "" {
			header = strings.TrimSpace(f.Name)
		}
		key := strings.TrimSpace(f.Name)
		if key == "" {
			key = header
		}
		if header == "" {
			continue
		}
		val := strings.TrimSpace(form[key])
		if val == "" {
			continue
		}
		out[header] = val
	}
	return out
}

func basicCredentials(sess Session, form map[string]string) (user, pass string) {
	fields := effectiveBasicFields(sess.Fields)
	args := mapFormToArgs(Session{Fields: fields, Mode: ModeBasic}, form)
	if v, ok := args["username"].(string); ok {
		user = v
	}
	if v, ok := args["password"].(string); ok {
		pass = v
	}
	// Also accept common aliases from field names.
	if user == "" {
		user = form["username"]
		if user == "" {
			user = form["user"]
		}
	}
	if pass == "" {
		pass = form["password"]
		if pass == "" {
			pass = form["pass"]
		}
	}
	return user, pass
}

func mapFormToArgs(sess Session, form map[string]string) map[string]any {
	args := map[string]any{}
	fields := sess.Fields
	if len(fields) == 0 {
		switch sess.Mode {
		case ModeTokenPaste:
			fields = DefaultTokenFields()
		case ModeBasic:
			fields = DefaultBasicFields()
		case ModeHeaderPairs:
			fields = DefaultHeaderPairFields()
		}
	}
	for _, f := range fields {
		key := strings.TrimSpace(f.Name)
		if key == "" {
			key = strings.TrimSpace(f.Arg)
		}
		arg := strings.TrimSpace(f.Arg)
		if arg == "" {
			arg = key
		}
		if key == "" || arg == "" {
			continue
		}
		args[arg] = form[key]
	}
	return args
}

func captureAuthToConfig(c store.CaptureAuth) identity.CaptureConfig {
	return identity.CaptureConfig{
		ToolNameGlob:   c.ToolNameGlob,
		TokenJSONPaths: append([]string(nil), c.TokenJSONPaths...),
		LabelJSONPaths: append([]string(nil), c.LabelJSONPaths...),
		HeaderTemplate: c.HeaderTemplate,
		DefaultScheme:  c.DefaultScheme,
	}
}

func captureDefaults(c identity.CaptureConfig) identity.CaptureConfig {
	if strings.EqualFold(strings.TrimSpace(c.ToolNameGlob), "__none__") {
		return c
	}
	if strings.TrimSpace(c.ToolNameGlob) == "" {
		c.ToolNameGlob = "*login*"
	}
	if len(c.TokenJSONPaths) == 0 {
		c.TokenJSONPaths = []string{
			"accessToken", "data.accessToken",
			"token", "data.token",
			"access_token", "data.access_token",
			"tokenValue", "data.tokenValue",
		}
	}
	if len(c.LabelJSONPaths) == 0 {
		c.LabelJSONPaths = []string{"email", "data.email", "data.user.email", "username", "data.username"}
	}
	if strings.TrimSpace(c.HeaderTemplate) == "" {
		c.HeaderTemplate = "Bearer {{token}}"
	}
	return c
}

func exportToRuntimeIdentity(export store.MCPExportIdentity) identity.Identity {
	scheme := strings.ToLower(strings.TrimSpace(export.Scheme))
	if scheme == "" {
		scheme = "bearer"
	}
	return identity.Identity{
		ID:                export.ID,
		Label:             export.Name,
		Scheme:            scheme,
		CredentialHeaders: export.Headers,
		Source:            identity.SourceManual,
		Subject:           export.ID,
	}
}
