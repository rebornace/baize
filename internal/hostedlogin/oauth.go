package hostedlogin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/store"
)

// OAuthAuthorizeURL builds the IdP authorize redirect for a hosted session.
func OAuthAuthorizeURL(sess Session) (string, error) {
	if sess.OAuth == nil {
		return "", fmt.Errorf("hostedlogin: missing oauth config")
	}
	o := sess.OAuth
	redirect := oauthCallbackURL(sess)
	u, err := url.Parse(strings.TrimSpace(o.AuthorizeURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("hostedlogin: invalid authorize_url")
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", strings.TrimSpace(o.ClientID))
	q.Set("redirect_uri", redirect)
	q.Set("state", sess.CSRF)
	if scopes := strings.TrimSpace(strings.Join(o.Scopes, " ")); scopes != "" {
		q.Set("scope", scopes)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func oauthCallbackURL(sess Session) string {
	origin := strings.TrimRight(strings.TrimSpace(sess.PublicOrigin), "/")
	return fmt.Sprintf("%s/v0/auth/hosted/%s/oauth/callback", origin, sess.ID)
}

// CompleteOAuth exchanges an authorization code and persists the access token.
func CompleteOAuth(ctx context.Context, deps SubmitDeps, sessionID, state, code string) error {
	if deps.Sessions == nil || deps.Store == nil {
		return fmt.Errorf("hostedlogin: incomplete deps")
	}
	sess, ok := deps.Sessions.Get(sessionID)
	if !ok {
		return fmt.Errorf("hostedlogin: session not found or expired")
	}
	if strings.TrimSpace(state) == "" || state != sess.CSRF {
		return fmt.Errorf("hostedlogin: invalid oauth state")
	}
	if strings.TrimSpace(code) == "" {
		return fmt.Errorf("hostedlogin: missing authorization code")
	}
	if sess.OAuth == nil {
		return fmt.Errorf("hostedlogin: missing oauth config")
	}
	token, err := exchangeOAuthCode(ctx, sess, code)
	if err != nil {
		if deps.Waiter != nil {
			deps.Waiter.NotifyFail(sessionID, err)
		}
		audit(deps.Audit, AuditEvent{
			Kind: AuditSubmitFail, SessionID: sessionID,
			ConnectorID: sess.ConnectorID, ConversationID: sess.ConversationID,
			ExportKeyID: sess.ExportKeyID, Err: err.Error(),
		})
		return err
	}
	header := strings.TrimSpace(sess.OAuth.TokenHeader)
	if header == "" {
		header = "Authorization"
	}
	prefix := sess.OAuth.TokenPrefix
	if prefix == "" {
		prefix = "Bearer "
	}
	headers := map[string]string{header: prefix + token}
	if err := persistCredential(deps, sess, sess.ConversationID, sess.IdentityID, headers, sess.OAuth.ClientID, "bearer"); err != nil {
		if deps.Waiter != nil {
			deps.Waiter.NotifyFail(sessionID, err)
		}
		audit(deps.Audit, AuditEvent{
			Kind: AuditSubmitFail, SessionID: sessionID,
			ConnectorID: sess.ConnectorID, ConversationID: sess.ConversationID,
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
		ConnectorID: sess.ConnectorID, ConversationID: sess.ConversationID,
		ExportKeyID: sess.ExportKeyID,
	})
	return nil
}

func exchangeOAuthCode(ctx context.Context, sess Session, code string) (string, error) {
	o := sess.OAuth
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", oauthCallbackURL(sess))
	form.Set("client_id", strings.TrimSpace(o.ClientID))
	if secret := strings.TrimSpace(o.ClientSecret); secret != "" {
		form.Set("client_secret", secret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(o.TokenURL), strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("hostedlogin: oauth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("hostedlogin: oauth token exchange: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("hostedlogin: oauth token status %d", resp.StatusCode)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("hostedlogin: oauth token json: %w", err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", fmt.Errorf("hostedlogin: oauth response missing access_token")
	}
	return strings.TrimSpace(payload.AccessToken), nil
}

// CloneOAuth returns a shallow copy of oauth config for session seeding.
func CloneOAuth(o *store.HostedLoginOAuth) *store.HostedLoginOAuth {
	if o == nil {
		return nil
	}
	cp := *o
	if o.Scopes != nil {
		cp.Scopes = append([]string(nil), o.Scopes...)
	}
	return &cp
}
