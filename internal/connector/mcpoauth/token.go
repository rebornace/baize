package mcpoauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/settingscrypto"
)

const defaultAccessTokenSkew = 60 * time.Second

// SealBundle JSON-encodes and seals a token bundle with settingscrypto.
func SealBundle(key settingscrypto.Key, b TokenBundle) (string, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return "", fmt.Errorf("marshal token bundle: %w", err)
	}
	return settingscrypto.Seal(key, string(raw))
}

// OpenBundle opens a sealed token bundle.
func OpenBundle(key settingscrypto.Key, sealed string) (TokenBundle, error) {
	plain, err := settingscrypto.Open(key, sealed)
	if err != nil {
		return TokenBundle{}, err
	}
	var b TokenBundle
	if err := json.Unmarshal([]byte(plain), &b); err != nil {
		return TokenBundle{}, fmt.Errorf("unmarshal token bundle: %w", err)
	}
	return b, nil
}

// MergeHeaders copies static headers and sets Authorization from the OAuth access token.
func MergeHeaders(static map[string]string, accessToken string) map[string]string {
	out := make(map[string]string, len(static)+1)
	for k, v := range static {
		out[k] = v
	}
	if accessToken != "" {
		out["Authorization"] = "Bearer " + accessToken
	}
	return out
}

// ExchangeCode trades an authorization code (with PKCE verifier) for tokens.
func ExchangeCode(
	ctx context.Context,
	client *http.Client,
	tokenURL, clientID, clientSecret, code, redirectURI, verifier string,
) (TokenBundle, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	return requestToken(ctx, client, tokenURL, clientID, clientSecret, form)
}

// Refresh obtains a new token bundle using a refresh token.
func Refresh(
	ctx context.Context,
	client *http.Client,
	tokenURL, clientID, clientSecret, refreshToken string,
) (TokenBundle, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	return requestToken(ctx, client, tokenURL, clientID, clientSecret, form)
}

// EnsureAccessToken returns a usable bundle, refreshing when within skew of expiry.
func EnsureAccessToken(
	ctx context.Context,
	client *http.Client,
	tokenURL, clientID, clientSecret string,
	bundle TokenBundle,
	skew time.Duration,
) (TokenBundle, error) {
	if skew == 0 {
		skew = defaultAccessTokenSkew
	}
	if !bundleNeedsRefresh(bundle, skew) {
		return bundle, nil
	}
	if bundle.RefreshToken == "" {
		if tokenExpired(bundle) {
			return TokenBundle{}, ErrNeedsReauth
		}
		return bundle, nil
	}
	return Refresh(ctx, client, tokenURL, clientID, clientSecret, bundle.RefreshToken)
}

func bundleNeedsRefresh(b TokenBundle, skew time.Duration) bool {
	if b.ExpiresAt.IsZero() {
		return false
	}
	return time.Until(b.ExpiresAt) <= skew
}

func tokenExpired(b TokenBundle) bool {
	if b.ExpiresAt.IsZero() {
		return false
	}
	return !time.Now().Before(b.ExpiresAt)
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}

func requestToken(
	ctx context.Context,
	client *http.Client,
	tokenURL, clientID, clientSecret string,
	form url.Values,
) (TokenBundle, error) {
	if client == nil {
		client = http.DefaultClient
	}
	form.Set("client_id", clientID)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return TokenBundle{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return TokenBundle{}, fmt.Errorf("token endpoint request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TokenBundle{}, fmt.Errorf("token endpoint read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyStr := strings.TrimSpace(string(body))
		if isOAuthInvalidGrant(resp.StatusCode, bodyStr) {
			return TokenBundle{}, fmt.Errorf("%w: %s", ErrNeedsReauth, bodyStr)
		}
		return TokenBundle{}, fmt.Errorf("token endpoint: status %d: %s", resp.StatusCode, bodyStr)
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return TokenBundle{}, fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return TokenBundle{}, fmt.Errorf("token response missing access_token")
	}
	b := TokenBundle{
		AccessToken: tr.AccessToken,
		TokenType:   tr.TokenType,
		Scope:       tr.Scope,
	}
	if tr.RefreshToken != "" {
		b.RefreshToken = tr.RefreshToken
	} else if rt := form.Get("refresh_token"); rt != "" {
		b.RefreshToken = rt
	}
	if tr.ExpiresIn > 0 {
		b.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return b, nil
}

// isOAuthInvalidGrant reports whether a token-endpoint error body is a clear
// invalid_grant that requires interactive re-authorization.
func isOAuthInvalidGrant(status int, body string) bool {
	if status != http.StatusBadRequest && status != http.StatusUnauthorized {
		return false
	}
	var er struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(body), &er) == nil && er.Error == "invalid_grant" {
		return true
	}
	return strings.Contains(body, `"error":"invalid_grant"`) ||
		strings.Contains(body, `"error": "invalid_grant"`)
}
