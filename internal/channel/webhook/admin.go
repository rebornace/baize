package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/webhooksig"
)

// adminClient talks to the out-of-process adapter's management plane
// (/admin/*). The HTTP implementation lives below; tests inject fakes. All
// methods return an error when the adapter is unreachable.
type adminClient interface {
	// Status returns hasCredentials and polling from the adapter /admin/status.
	Status(ctx context.Context) (hasCreds bool, polling bool, err error)
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	LoginStart(ctx context.Context) (ticket, qrURL string, err error)
	// LoginPoll polls QR login; status is "pending"|"success"|"expired".
	LoginPoll(ctx context.Context, ticket string) (status string, err error)
	Logout(ctx context.Context) error
}

// httpAdminClient signs every management request with the shared HMAC secret
// (outbound side: baize -> adapter admin guard). GETs carry an empty body,
// which is signed so the adapter's guard verifies the same bytes.
type httpAdminClient struct {
	baseURL string
	secret  string
	hc      *http.Client
}

func newHTTPAdminClient(baseURL, secret string) *httpAdminClient {
	return &httpAdminClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		secret:  secret,
		hc:      &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *httpAdminClient) do(ctx context.Context, method, path string, query url.Values, out any) error {
	full := c.baseURL + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	// Admin requests currently carry no JSON body; sign the empty body so the
	// adapter's adminGuard (which reads the body) verifies the same bytes.
	var body []byte
	req, err := http.NewRequestWithContext(ctx, method, full, bytes.NewReader(body))
	if err != nil {
		return err
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set(HeaderProtocol, ProtocolVersion)
	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set(HeaderSignature, webhooksig.Sign(c.secret, ts, body))
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("adapter %s: http %d: %s", path, resp.StatusCode, string(data))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("adapter %s: decode: %w", path, err)
		}
	}
	return nil
}

func (c *httpAdminClient) Status(ctx context.Context) (bool, bool, error) {
	var st struct {
		HasCredentials bool `json:"has_credentials"`
		Polling        bool `json:"polling"`
	}
	if err := c.do(ctx, http.MethodGet, "/admin/status", nil, &st); err != nil {
		return false, false, err
	}
	return st.HasCredentials, st.Polling, nil
}

func (c *httpAdminClient) Start(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/admin/start", nil, nil)
}

func (c *httpAdminClient) Stop(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/admin/stop", nil, nil)
}

func (c *httpAdminClient) Logout(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/admin/logout", nil, nil)
}

func (c *httpAdminClient) LoginStart(ctx context.Context) (string, string, error) {
	var out struct {
		Ticket string `json:"ticket"`
		QRURL  string `json:"qr_url"`
	}
	if err := c.do(ctx, http.MethodPost, "/admin/login/start", nil, &out); err != nil {
		return "", "", err
	}
	return out.Ticket, out.QRURL, nil
}

func (c *httpAdminClient) LoginPoll(ctx context.Context, ticket string) (string, error) {
	var out struct {
		Status string `json:"status"`
	}
	q := url.Values{}
	q.Set("ticket", ticket)
	if err := c.do(ctx, http.MethodGet, "/admin/login/status", q, &out); err != nil {
		return "", err
	}
	return out.Status, nil
}

var _ adminClient = (*httpAdminClient)(nil)

// LoginStart asks the out-of-process adapter to begin a QR login flow. It is
// part of channel.ManagedChannel; the generic management plane proxies to it.
// Returns an explicit error when no adapter admin plane is configured.
func (c *Channel) LoginStart(ctx context.Context) (channel.LoginTicket, error) {
	if c.admin == nil {
		return channel.LoginTicket{}, fmt.Errorf("webhook %s: no adapter admin plane configured", c.cfg.Name)
	}
	ticket, qr, err := c.admin.LoginStart(ctx)
	if err != nil {
		return channel.LoginTicket{}, err
	}
	return channel.LoginTicket{Ticket: ticket, QRURL: qr}, nil
}

// LoginPoll polls a QR login ticket started via LoginStart. The returned
// status is "pending"|"success"|"expired" (adapter-defined).
func (c *Channel) LoginPoll(ctx context.Context, ticket string) (string, error) {
	if c.admin == nil {
		return "", fmt.Errorf("webhook %s: no adapter admin plane configured", c.cfg.Name)
	}
	return c.admin.LoginPoll(ctx, ticket)
}

// Logout clears the adapter-side credentials. It is best-effort: when no
// adapter admin plane is configured there is nothing to clear and it succeeds.
func (c *Channel) Logout(ctx context.Context) error {
	if c.admin != nil {
		if err := c.admin.Logout(ctx); err != nil {
			return err
		}
	}
	return nil
}

var _ channel.ManagedChannel = (*Channel)(nil)
