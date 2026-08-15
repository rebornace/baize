package connector

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rebornace/baize/internal/authcred"
	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/connector/httpplugin"
	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// ApplyInput configures connector registration shared by bootstrap and PUT.
type ApplyInput struct {
	Store                   store.Store
	Registry                *tool.Registry
	Identities              identity.Store
	ID, Type, Spec, BaseURL string
	RequireApproval         []string
	RequireApprovalMutating bool
	RequireLogin            *[]string // nil=从 Registry 保留同名；非 nil=整表（空切片=全公开）
	Auth                    store.ConnectorAuth
}

// Apply resolves auth, registers tools, and persists the connector.
func Apply(in ApplyInput) (store.Connector, []tool.Info, error) {
	authCfg := authcred.Config{
		Mode:        in.Auth.Mode,
		Static:      authcred.Static{Headers: in.Auth.Static.Headers},
		Passthrough: authcred.PassThru{Headers: in.Auth.Passthrough.Headers},
		VaultRef:    authcred.VaultRef{Headers: in.Auth.VaultRef.Headers},
	}
	headers, err := authcred.ResolveDefaults(authCfg)
	if err != nil {
		return store.Connector{}, nil, fmt.Errorf("resolve connector auth: %w", err)
	}

	var requireLogin []string
	if in.RequireLogin == nil {
		for _, info := range in.Registry.List() {
			if info.ConnectorID == in.ID && info.RequireLogin {
				requireLogin = append(requireLogin, info.Name)
			}
		}
		sort.Strings(requireLogin)
	} else {
		requireLogin = append([]string(nil), (*in.RequireLogin)...)
		sort.Strings(requireLogin)
	}

	typ := strings.TrimSpace(in.Type)
	if typ == "" {
		typ = "openapi"
	}

	authMode := authcred.NormalizeMode(in.Auth.Mode)
	var c store.Connector
	var infos []tool.Info

	switch typ {
	case "http":
		if strings.TrimSpace(in.BaseURL) == "" {
			return store.Connector{}, nil, fmt.Errorf("connector.base_url is required")
		}
		auth := in.Auth
		auth.Capture = store.CaptureAuth{}
		c, infos, err = httpplugin.RegisterWithOpts(in.Store, in.Registry, httpplugin.RegisterOpts{
			ID:              in.ID,
			BaseURL:         in.BaseURL,
			RequireApproval: in.RequireApproval,
			RequireLogin:    requireLogin,
			Headers:         headers,
			AuthMode:        authMode,
			Auth:            auth,
			Identities:      in.Identities,
			Resolver:        authresolve.OpenAPISecurityResolver{},
		})
	case "openapi":
		if strings.TrimSpace(in.Spec) == "" {
			return store.Connector{}, nil, fmt.Errorf("connector.spec is required")
		}
		capture := CaptureDefaults(identity.CaptureConfig{
			ToolNameGlob:   in.Auth.Capture.ToolNameGlob,
			TokenJSONPaths: in.Auth.Capture.TokenJSONPaths,
			LabelJSONPaths: in.Auth.Capture.LabelJSONPaths,
			HeaderTemplate: in.Auth.Capture.HeaderTemplate,
			DefaultScheme:  in.Auth.Capture.DefaultScheme,
		})
		if capture.DefaultScheme == "" {
			if routes, loadErr := openapi.LoadTools(in.Spec); loadErr == nil {
				capture.DefaultScheme = UniqueSecurityScheme(routes)
			}
		}
		auth := in.Auth
		auth.Capture = store.CaptureAuth{
			ToolNameGlob:   capture.ToolNameGlob,
			TokenJSONPaths: capture.TokenJSONPaths,
			LabelJSONPaths: capture.LabelJSONPaths,
			HeaderTemplate: capture.HeaderTemplate,
			DefaultScheme:  capture.DefaultScheme,
		}
		c, infos, err = openapi.RegisterWithOpts(in.Store, in.Registry, openapi.RegisterOpts{
			ID:                      in.ID,
			Type:                    typ,
			SpecPath:                in.Spec,
			BaseURL:                 in.BaseURL,
			RequireApproval:         in.RequireApproval,
			RequireApprovalMutating: in.RequireApprovalMutating,
			RequireLogin:            requireLogin,
			Headers:                 headers,
			AuthMode:                authMode,
			Auth:                    auth,
			Identities:              in.Identities,
			Resolver:                authresolve.OpenAPISecurityResolver{},
			Capture:                 capture,
		})
	default:
		return store.Connector{}, nil, fmt.Errorf("unsupported connector type: %s", typ)
	}
	if err != nil {
		return store.Connector{}, nil, err
	}

	// Ensure RequireLogin is persisted if RegisterWithOpts left it empty.
	if len(c.RequireLogin) == 0 && len(requireLogin) > 0 {
		c.RequireLogin = append([]string(nil), requireLogin...)
		sort.Strings(c.RequireLogin)
		in.Store.UpsertConnector(c)
	}
	return c, infos, nil
}
