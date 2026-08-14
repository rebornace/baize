package httpplugin

import (
	"context"
	"errors"
	"strings"

	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
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

	for _, td := range filtered {
		td := td
		name := td.Name
		schema := td.InputSchema
		if len(schema) == 0 {
			schema = map[string]any{"type": "object"}
		}
		needApproval := approval[name]
		reg.RegisterMeta(tool.Meta{
			Spec: llm.ToolSpec{
				Name:        name,
				Description: td.Description,
				InputSchema: schema,
			},
			ConnectorID: opts.ID,
			OperationID: name,
			Method:      "",
			Path:        "",
		}, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
			overlay := opts.Headers
			if opts.AuthMode == "passthrough" {
				if h := identity.PassthroughHeadersFrom(ctx); len(h) > 0 {
					overlay = h
				} else {
					overlay = nil
				}
			}
			var usedID string
			if opts.Identities != nil && opts.Resolver != nil {
				conv := identity.ConversationIDFrom(ctx)
				force := identity.ForceIdentityIDFrom(ctx)
				defaultHeaders := opts.Headers
				if opts.AuthMode == "passthrough" {
					if h := identity.PassthroughHeadersFrom(ctx); len(h) > 0 {
						defaultHeaders = h
					} else {
						defaultHeaders = nil
					}
				}
				in := authresolve.ResolveInput{
					Identities:      opts.Identities.List(conv),
					SecuritySchemes: []string{},
					DefaultHeaders:  defaultHeaders,
					ForceIdentityID: force,
				}
				res := opts.Resolver.Resolve(ctx, in)
				if res.OK {
					overlay = res.Headers
					usedID = res.IdentityID
				}
			}
			out, invErr := client.Invoke(ctx, name, args, InvokeMeta{
				RunID:   identity.RunIDFrom(ctx),
				AgentID: identity.AgentIDFrom(ctx),
				Headers: overlay,
			})
			if invErr != nil {
				return nil, true, invErr
			}
			if usedID != "" && opts.Identities != nil {
				_ = opts.Identities.Touch(identity.ConversationIDFrom(ctx), usedID)
			}
			return out.Content, out.IsError, nil
		}, needApproval)
	}

	c := store.Connector{
		ID:              opts.ID,
		Type:            "http",
		Spec:            "",
		BaseURL:         opts.BaseURL,
		RequireApproval: opts.RequireApproval,
		Auth:            opts.Auth,
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
