package connector

import (
	"context"
	"net/http"

	"github.com/rebornace/baize/internal/connector/mcpoauth"
	"github.com/rebornace/baize/internal/settingscrypto"
	"github.com/rebornace/baize/internal/store"
)

// resolveMCPHTTPOAuthHeaders starts from staticHeaders and, when oauth (or the
// connector's sealed bundle in store) is present, opens it, ensures a usable
// access token (refreshing when needed), and merges Authorization (OAuth wins).
//
// oauthOverride, when non-nil, is used instead of loading from store (Apply
// discover path). Refreshed bundles are written back onto override and, when
// store+connectorID are set, best-effort UpsertConnector.
//
// On EnsureAccessToken failure it best-effort sets status=needs_reauth and
// returns a tool-error content map with code oauth_reauth_required.
func resolveMCPHTTPOAuthHeaders(
	ctx context.Context,
	st store.Store,
	connectorID string,
	key settingscrypto.Key,
	staticHeaders map[string]string,
	oauthOverride *store.MCPOAuthConfig,
) (headers map[string]string, reauth map[string]any, err error) {
	headers = copyStringMap(staticHeaders)

	var oauth *store.MCPOAuthConfig
	var conn store.Connector
	if oauthOverride != nil {
		oauth = oauthOverride
		if st != nil && connectorID != "" {
			if c, getErr := st.GetConnector(connectorID); getErr == nil {
				conn = c
			} else {
				conn = store.Connector{ID: connectorID, Type: "mcp"}
			}
		}
	} else {
		var ok bool
		oauth, conn, ok = loadMCPOAuth(st, connectorID)
		if !ok {
			return headers, nil, nil
		}
	}
	if oauth == nil || oauth.TokenBundleSealed == "" {
		return headers, nil, nil
	}
	if len(key) == 0 {
		return headers, nil, nil
	}

	bundle, err := mcpoauth.OpenBundle(key, oauth.TokenBundleSealed)
	if err != nil {
		return nil, nil, err
	}

	clientSecret := ""
	if oauth.ClientSecretSealed != "" {
		plain, openErr := settingscrypto.Open(key, oauth.ClientSecretSealed)
		if openErr != nil {
			return nil, nil, openErr
		}
		clientSecret = plain
	}

	before := bundle
	bundle, err = mcpoauth.EnsureAccessToken(
		ctx, http.DefaultClient, oauth.TokenEndpoint, oauth.ClientID, clientSecret, bundle, 0,
	)
	if err != nil {
		oauth.Status = "needs_reauth"
		if st != nil && connectorID != "" {
			conn.ID = connectorID
			if conn.Type == "" {
				conn.Type = "mcp"
			}
			conn.MCP.OAuth = oauth
			// Best-effort: only write when connector already exists.
			if _, getErr := st.GetConnector(connectorID); getErr == nil {
				st.UpsertConnector(conn)
			}
		}
		return nil, map[string]any{
			"error": "MCP OAuth 需重新授权",
			"code":  "oauth_reauth_required",
		}, nil
	}

	if bundleChanged(before, bundle) {
		sealed, sealErr := mcpoauth.SealBundle(key, bundle)
		if sealErr == nil {
			oauth.TokenBundleSealed = sealed
			if oauth.Status == "" || oauth.Status == "needs_reauth" {
				oauth.Status = "authorized"
			}
			if st != nil && connectorID != "" {
				if existing, getErr := st.GetConnector(connectorID); getErr == nil {
					existing.MCP.OAuth = oauth
					st.UpsertConnector(existing)
				}
			}
		}
	}

	return mcpoauth.MergeHeaders(headers, bundle.AccessToken), nil, nil
}

func loadMCPOAuth(st store.Store, connectorID string) (*store.MCPOAuthConfig, store.Connector, bool) {
	if st == nil || connectorID == "" {
		return nil, store.Connector{}, false
	}
	c, err := st.GetConnector(connectorID)
	if err != nil {
		return nil, store.Connector{}, false
	}
	return c.MCP.OAuth, c, true
}

func bundleChanged(before, after mcpoauth.TokenBundle) bool {
	return before.AccessToken != after.AccessToken ||
		before.RefreshToken != after.RefreshToken ||
		!before.ExpiresAt.Equal(after.ExpiresAt) ||
		before.Scope != after.Scope ||
		before.TokenType != after.TokenType
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// ensureSettingsKey returns key, or KeyFromEnv when key is empty.
func ensureSettingsKey(key settingscrypto.Key) settingscrypto.Key {
	if len(key) > 0 {
		return key
	}
	envKey, _ := settingscrypto.KeyFromEnv()
	return envKey
}
