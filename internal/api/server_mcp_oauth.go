package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/rebornace/baize/internal/connector/mcpoauth"
	"github.com/rebornace/baize/internal/settingscrypto"
	"github.com/rebornace/baize/internal/store"
)

func (s *Server) oauthSessions() *mcpoauth.SessionStore {
	if s.OAuthSessions == nil {
		s.OAuthSessions = mcpoauth.NewSessionStore()
	}
	return s.OAuthSessions
}

func mcpOAuthRedirectURI(publicBase, connectorID string) string {
	return strings.TrimRight(publicBase, "/") + "/v0/connectors/" + connectorID + "/mcp/oauth/callback"
}

func (s *Server) handleMCPOAuthStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	publicBase := strings.TrimSpace(s.CallbackPublicBase)
	if publicBase == "" {
		writeError(w, http.StatusBadRequest, "public_base_required", "configure runtime.public_base_url for MCP OAuth callbacks")
		return
	}

	c, err := s.Store.GetConnector(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "connector_not_found", "connector not found")
		return
	}
	if c.Type != "mcp" || strings.TrimSpace(c.MCP.URL) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "MCP OAuth requires an HTTP MCP connector with url")
		return
	}

	ep, err := mcpoauth.Discover(r.Context(), http.DefaultClient, c.MCP.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "oauth_discover_failed", err.Error())
		return
	}

	redirectURI := mcpOAuthRedirectURI(publicBase, id)
	oauth := ensureMCPOAuth(c.MCP.OAuth)
	oauth.AuthorizationEndpoint = ep.AuthorizationEndpoint
	oauth.TokenEndpoint = ep.TokenEndpoint
	oauth.RegistrationEndpoint = ep.RegistrationEndpoint
	oauth.ResourceMetadataURL = ep.ResourceMetadataURL

	clientID := strings.TrimSpace(oauth.ClientID)
	if clientID == "" {
		if strings.TrimSpace(ep.RegistrationEndpoint) == "" {
			writeError(w, http.StatusBadRequest, "oauth_client_required", "provide mcp.oauth.client_id or use an authorization server that supports DCR")
			return
		}
		reg, err := mcpoauth.RegisterClient(r.Context(), http.DefaultClient, ep.RegistrationEndpoint, redirectURI)
		if err != nil {
			writeError(w, http.StatusBadRequest, "oauth_client_required", "DCR failed; provide mcp.oauth.client_id: "+err.Error())
			return
		}
		clientID = reg.ClientID
		oauth.ClientID = clientID
		if reg.ClientSecret != "" {
			key, keyErr := settingscrypto.KeyFromEnv()
			if writeIfSettingsKeyRequired(w, keyErr) {
				return
			}
			if len(key) == 0 {
				writeError(w, http.StatusBadRequest, "settings_key_required", settingsKeyRequiredMsg)
				return
			}
			sealed, sealErr := settingscrypto.Seal(key, reg.ClientSecret)
			if writeIfSettingsKeyRequired(w, sealErr) {
				return
			}
			if sealErr != nil {
				writeError(w, http.StatusInternalServerError, "internal", sealErr.Error())
				return
			}
			oauth.ClientSecretSealed = sealed
		}
	}

	verifier, challenge, err := mcpoauth.GeneratePKCE()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	state, err := mcpoauth.GenerateState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	s.oauthSessions().Put(state, mcpoauth.Pending{
		ConnectorID: id,
		Verifier:    verifier,
		RedirectURI: redirectURI,
	})

	c.MCP.OAuth = oauth
	s.Store.UpsertConnector(c)

	authURL, err := buildAuthorizationURL(ep.AuthorizationEndpoint, clientID, redirectURI, challenge, state, c.MCP.URL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"authorization_url": authURL})
}

func buildAuthorizationURL(authorizationEndpoint, clientID, redirectURI, challenge, state, resource string) (string, error) {
	u, err := url.Parse(authorizationEndpoint)
	if err != nil {
		return "", fmt.Errorf("invalid authorization_endpoint: %w", err)
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	if resource != "" {
		q.Set("resource", resource)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func ensureMCPOAuth(o *store.MCPOAuthConfig) *store.MCPOAuthConfig {
	if o == nil {
		return &store.MCPOAuthConfig{}
	}
	cp := *o
	return &cp
}

func (s *Server) handleMCPOAuthCallback(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "code and state are required")
		return
	}

	pending, err := s.oauthSessions().Take(state)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_state", "invalid or expired OAuth state")
		return
	}
	if pending.ConnectorID != id {
		writeError(w, http.StatusBadRequest, "invalid_state", "OAuth state does not match connector")
		return
	}

	c, err := s.Store.GetConnector(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "connector_not_found", "connector not found")
		return
	}
	if c.MCP.OAuth == nil || strings.TrimSpace(c.MCP.OAuth.TokenEndpoint) == "" || strings.TrimSpace(c.MCP.OAuth.ClientID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "connector is missing OAuth client/token endpoint")
		return
	}

	clientSecret := ""
	if sealed := strings.TrimSpace(c.MCP.OAuth.ClientSecretSealed); sealed != "" {
		key, keyErr := settingscrypto.KeyFromEnv()
		if writeIfSettingsKeyRequired(w, keyErr) {
			return
		}
		plain, openErr := settingscrypto.Open(key, sealed)
		if writeIfSettingsKeyRequired(w, openErr) {
			return
		}
		if openErr != nil {
			writeError(w, http.StatusInternalServerError, "internal", openErr.Error())
			return
		}
		clientSecret = plain
	}

	bundle, err := mcpoauth.ExchangeCode(
		r.Context(),
		http.DefaultClient,
		c.MCP.OAuth.TokenEndpoint,
		c.MCP.OAuth.ClientID,
		clientSecret,
		code,
		pending.RedirectURI,
		pending.Verifier,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, "oauth_exchange_failed", err.Error())
		return
	}

	key, keyErr := settingscrypto.KeyFromEnv()
	if writeIfSettingsKeyRequired(w, keyErr) {
		return
	}
	if len(key) == 0 {
		writeError(w, http.StatusBadRequest, "settings_key_required", settingsKeyRequiredMsg)
		return
	}
	sealed, err := mcpoauth.SealBundle(key, bundle)
	if writeIfSettingsKeyRequired(w, err) {
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	oauth := ensureMCPOAuth(c.MCP.OAuth)
	oauth.Status = "authorized"
	oauth.TokenBundleSealed = sealed
	c.MCP.OAuth = oauth
	s.Store.UpsertConnector(c)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>MCP OAuth</title></head><body><p>Authorization complete. You can close this window.</p></body></html>`))
}

func (s *Server) handleMCPOAuthDisconnect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.Store.GetConnector(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "connector_not_found", "connector not found")
		return
	}
	if c.Type != "mcp" {
		writeError(w, http.StatusBadRequest, "invalid_request", "not an MCP connector")
		return
	}
	oauth := ensureMCPOAuth(c.MCP.OAuth)
	oauth.Status = ""
	oauth.TokenBundleSealed = ""
	c.MCP.OAuth = oauth
	s.Store.UpsertConnector(c)

	writeJSON(w, http.StatusOK, map[string]any{
		"id":     c.ID,
		"status": "",
		"mcp":    redactMCPForAPI(c.MCP),
	})
}

func (s *Server) handleMCPOAuthStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.Store.GetConnector(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "connector_not_found", "connector not found")
		return
	}
	status, clientID := "", ""
	if c.MCP.OAuth != nil {
		status = c.MCP.OAuth.Status
		clientID = c.MCP.OAuth.ClientID
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    status,
		"client_id": clientID,
	})
}
