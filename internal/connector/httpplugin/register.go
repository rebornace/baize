package httpplugin

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/plugincallback"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// ErrToolConflict is returned when registering would overwrite another connector's tools.
var ErrToolConflict = errors.New("tool_conflict")

// RegisterWithOpts health-checks the sidecar, lists tools, and registers them into the Registry.
func RegisterWithOpts(st store.Store, reg *tool.Registry, opts RegisterOpts) (store.Connector, []tool.Info, error) {
	client := NewClient(opts.BaseURL)
	if err := client.Healthz(context.Background()); err != nil {
		return store.Connector{}, nil, err
	}
	tools, err := client.ListTools(context.Background())
	if err != nil {
		return store.Connector{}, nil, err
	}

	names := make([]string, 0, len(tools))
	filtered := make([]ToolDesc, 0, len(tools))
	for _, td := range tools {
		name := strings.TrimSpace(td.Name)
		if name == "" {
			continue
		}
		td.Name = name
		names = append(names, name)
		filtered = append(filtered, td)
	}
	if len(filtered) == 0 {
		return store.Connector{}, nil, ErrInvalidPlugin
	}
	if reg.WouldConflict(opts.ID, names) {
		return store.Connector{}, nil, ErrToolConflict
	}
	reg.UnregisterConnector(opts.ID)

	approval := map[string]bool{}
	for _, n := range opts.RequireApproval {
		approval[n] = true
	}
	login := map[string]bool{}
	for _, n := range opts.RequireLogin {
		login[n] = true
	}
	// Persist only names that still exist after this registration (drop disappeared).
	requireLogin := make([]string, 0, len(names))
	for _, n := range names {
		if login[n] {
			requireLogin = append(requireLogin, n)
		}
	}
	sort.Strings(requireLogin)

	for _, td := range filtered {
		td := td
		name := td.Name
		schema := td.InputSchema
		if len(schema) == 0 {
			schema = map[string]any{"type": "object"}
		}
		needApproval := approval[name]
		if !approval[name] && identity.MatchToolName(opts.Capture.ToolNameGlob, name) {
			needApproval = false
		}
		needLogin := login[name]
		reg.RegisterMeta(tool.Meta{
			Spec: llm.ToolSpec{
				Name:        name,
				Description: td.Description,
				InputSchema: schema,
			},
			ConnectorID:  opts.ID,
			OperationID:  name,
			Method:       "",
			Path:         "",
			RequireLogin: needLogin,
		}, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
			conv := identity.ConversationIDFrom(ctx)
			var overlay map[string]string
			if conv == "" {
				overlay = opts.Headers
				if opts.AuthMode == "passthrough" {
					if h := identity.PassthroughHeadersFrom(ctx); len(h) > 0 {
						overlay = h
					} else {
						overlay = nil
					}
				}
			}
			var usedID string
			resOK := false
			if opts.Identities != nil && opts.Resolver != nil {
				force := identity.ForceIdentityIDFrom(ctx)
				var defaultHeaders map[string]string
				if conv == "" {
					defaultHeaders = opts.Headers
					if opts.AuthMode == "passthrough" {
						if h := identity.PassthroughHeadersFrom(ctx); len(h) > 0 {
							defaultHeaders = h
						} else {
							defaultHeaders = nil
						}
					}
				}
				in := authresolve.ResolveInput{
					Identities:      opts.Identities.List(conv),
					SecuritySchemes: nil,
					DefaultHeaders:  defaultHeaders,
					ForceIdentityID: force,
				}
				res := opts.Resolver.Resolve(ctx, in)
				if res.OK {
					overlay = res.Headers
					usedID = res.IdentityID
					resOK = true
				} else if conv != "" {
					overlay = nil
				}
			}
			if conv != "" && reg.RequiresLogin(name) && (!resOK || len(overlay) == 0) {
				return tool.LoginRequiredContent(), true, nil
			}
		out, invErr := client.Invoke(ctx, name, args, InvokeMeta{
			RunID:            identity.RunIDFrom(ctx),
			AgentID:          identity.AgentIDFrom(ctx),
			Headers:          overlay,
			CallbackEventURL: buildCallbackEventURL(opts, identity.RunIDFrom(ctx)),
		})
			if invErr != nil {
				return nil, true, invErr
			}
			if usedID != "" && opts.Identities != nil {
				_ = opts.Identities.Touch(conv, usedID)
			}
			if conv != "" && !out.IsError && opts.Identities != nil && identity.MatchToolName(opts.Capture.ToolNameGlob, name) {
				if h, label, sub, claims, ok := identity.ExtractCredential(opts.Capture, out.Content); ok {
					_, _ = opts.Identities.Upsert(conv, identity.Identity{
						Label:             label,
						Scheme:            opts.Capture.DefaultScheme,
						Subject:           sub,
						CredentialHeaders: h,
						Source:            identity.SourceLoginCapture,
						ClaimsSummary:     claims,
						IsDefault:         true,
					})
				}
			}
			return out.Content, out.IsError, nil
		}, needApproval)
	}

	auth := opts.Auth
	if strings.TrimSpace(auth.Capture.ToolNameGlob) == "" && strings.TrimSpace(opts.Capture.ToolNameGlob) != "" {
		auth.Capture = store.CaptureAuth{
			ToolNameGlob:   opts.Capture.ToolNameGlob,
			TokenJSONPaths: opts.Capture.TokenJSONPaths,
			LabelJSONPaths: opts.Capture.LabelJSONPaths,
			HeaderTemplate: opts.Capture.HeaderTemplate,
			DefaultScheme:  opts.Capture.DefaultScheme,
		}
	}
	c := store.Connector{
		ID:              opts.ID,
		Type:            "http",
		Spec:            "",
		BaseURL:         opts.BaseURL,
		RequireApproval: opts.RequireApproval,
		RequireLogin:    requireLogin,
		Auth:            auth,
	}
	st.UpsertConnector(c)
	return c, filterInfos(reg, opts.ID), nil
}

func filterInfos(reg *tool.Registry, connectorID string) []tool.Info {
	all := reg.List()
	out := make([]tool.Info, 0, len(all))
	for _, info := range all {
		if info.ConnectorID == connectorID {
			out = append(out, info)
		}
	}
	return out
}

// buildCallbackEventURL returns the callback_urls.event value for the given
// runID, or "" when callback injection is not possible. Injection requires
// all of: non-empty runID, non-nil Signer, non-empty Secret, non-empty
// PublicBase. PublicBase is normalized by stripping trailing slashes. A
// Signer/Issue error also yields "" (fail-open: invoke proceeds without
// callback_urls rather than erroring the tool call).
func buildCallbackEventURL(opts RegisterOpts, runID string) string {
	return SignCallbackEventURL(opts.CallbackSigner, opts.CallbackSecret, opts.CallbackPublicBase, opts.CallbackTTL, runID)
}

// SignCallbackEventURL issues a short-lived token and formats the callback
// URL advertised to sidecars. It is the exported counterpart of
// buildCallbackEventURL, used by callers (e.g. connector.Apply's plugin
// invoker) that do not carry a full RegisterOpts. Returns "" when any piece
// is missing or signing fails, so callers can fail open.
func SignCallbackEventURL(signer CallbackSigner, secret []byte, publicBase string, ttl time.Duration, runID string) string {
	return plugincallback.EventURL(plugincallback.Signer(signer), secret, publicBase, ttl, runID)
}
