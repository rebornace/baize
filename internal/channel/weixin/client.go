package weixin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	pathGetBotQRCode    = "/ilink/bot/get_bot_qrcode"
	pathGetQRCodeStatus = "/ilink/bot/get_qrcode_status"
	pathGetUpdates      = "/ilink/bot/getupdates"
	pathSendMessage     = "/ilink/bot/sendmessage"

	botTypeClawBot        = "3"
	channelVersion        = "1.0.0"
	authTypeILinkBotToken = "ilink_bot_token"
	defaultLongPoll       = 35 * time.Second

	itemTypeText       = 1
	messageTypeBot     = 2
	messageStateFinish = 2
)

// Client is the real HTTP iLink client.
type Client struct {
	BaseURL         string
	CDNBaseURL      string
	HTTP            *http.Client
	LongPollTimeout time.Duration
}

// NewClient builds a Client. Empty baseURL uses DefaultBaseURL; nil httpClient uses http.DefaultClient.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		BaseURL:         strings.TrimRight(baseURL, "/"),
		CDNBaseURL:      DefaultCDNBaseURL,
		HTTP:            httpClient,
		LongPollTimeout: defaultLongPoll,
	}
}

var _ ILink = (*Client)(nil)

type qrResponse struct {
	QRCode           string `json:"qrcode"`
	QRCodeImgContent string `json:"qrcode_img_content"`
}

type qrStatusResponse struct {
	Status      string `json:"status"`
	BotToken    string `json:"bot_token"`
	ILinkBotID  string `json:"ilink_bot_id"`
	ILinkUserID string `json:"ilink_user_id"`
	BaseURL     string `json:"baseurl"`
}

type baseInfo struct {
	ChannelVersion string `json:"channel_version"`
}

type getUpdatesRequest struct {
	GetUpdatesBuf string   `json:"get_updates_buf"`
	BaseInfo      baseInfo `json:"base_info"`
}

type getUpdatesResponse struct {
	Ret               int           `json:"ret"`
	ErrCode           int           `json:"errcode"`
	ErrMsg            string        `json:"errmsg"`
	Msgs              []wireMessage `json:"msgs"`
	GetUpdatesBuf     string        `json:"get_updates_buf"`
	LongPollTimeoutMS int           `json:"longpolling_timeout_ms"`
}

type wireMessage struct {
	FromUserID   string     `json:"from_user_id"`
	ToUserID     string     `json:"to_user_id"`
	ContextToken string     `json:"context_token"`
	ItemList     []wireItem `json:"item_list"`
}

type wireItem struct {
	Type      int            `json:"type"`
	TextItem  *wireTextItem  `json:"text_item,omitempty"`
	ImageItem *wireMediaItem `json:"image_item,omitempty"`
	FileItem  *wireMediaItem `json:"file_item,omitempty"`
	VoiceItem *wireMediaItem `json:"voice_item,omitempty"`
	VideoItem *wireMediaItem `json:"video_item,omitempty"`
}

type wireTextItem struct {
	Text string `json:"text"`
}

type wireMediaItem struct {
	Media    *wireCDNMedia `json:"media,omitempty"`
	FileName string        `json:"file_name,omitempty"`
	AESKey   string        `json:"aeskey,omitempty"`
}

type wireCDNMedia struct {
	EncryptQueryParam string `json:"encrypt_query_param"`
	AESKey            string `json:"aes_key"`
	EncryptType       int    `json:"encrypt_type"`
}

type sendMessageRequest struct {
	Msg      wireOutboundMessage `json:"msg"`
	BaseInfo baseInfo            `json:"base_info"`
}

type wireOutboundMessage struct {
	FromUserID   string     `json:"from_user_id"`
	ToUserID     string     `json:"to_user_id"`
	ClientID     string     `json:"client_id"`
	MessageType  int        `json:"message_type"`
	MessageState int        `json:"message_state"`
	ContextToken string     `json:"context_token"`
	ItemList     []wireItem `json:"item_list"`
}

func (c *Client) GetQR(ctx context.Context) (ticket, qrURL string, err error) {
	u := c.BaseURL + pathGetBotQRCode + "?bot_type=" + url.QueryEscape(botTypeClawBot)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", "", fmt.Errorf("weixin get qr: build request: %w", err)
	}
	var resp qrResponse
	if err := c.doJSON(req, &resp); err != nil {
		return "", "", fmt.Errorf("weixin get qr: %w", err)
	}
	if resp.QRCode == "" {
		return "", "", fmt.Errorf("weixin get qr: empty qrcode")
	}
	return resp.QRCode, resp.QRCodeImgContent, nil
}

func (c *Client) PollLogin(ctx context.Context, ticket string) (status, accountID, token string, err error) {
	u := c.BaseURL + pathGetQRCodeStatus + "?qrcode=" + url.QueryEscape(ticket)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", "", "", fmt.Errorf("weixin poll login: build request: %w", err)
	}
	req.Header.Set("iLink-App-ClientVersion", "1")

	var resp qrStatusResponse
	if err := c.doJSON(req, &resp); err != nil {
		return "", "", "", fmt.Errorf("weixin poll login: %w", err)
	}

	switch resp.Status {
	case "wait", "scaned":
		return LoginStatusPending, "", "", nil
	case "expired":
		return LoginStatusExpired, "", "", nil
	case "confirmed":
		accountID = resp.ILinkBotID
		if accountID == "" {
			accountID = resp.ILinkUserID
		}
		if resp.BotToken == "" {
			return "", "", "", fmt.Errorf("weixin poll login: confirmed without bot_token")
		}
		if resp.BaseURL != "" {
			c.BaseURL = strings.TrimRight(resp.BaseURL, "/")
		}
		return LoginStatusSuccess, accountID, resp.BotToken, nil
	default:
		return "", "", "", fmt.Errorf("weixin poll login: unknown status %q", resp.Status)
	}
}

func (c *Client) GetUpdates(ctx context.Context, token, cursor string) ([]Update, string, error) {
	body := getUpdatesRequest{
		GetUpdatesBuf: cursor,
		BaseInfo:      baseInfo{ChannelVersion: channelVersion},
	}
	req, err := c.newAuthJSONRequest(ctx, http.MethodPost, c.BaseURL+pathGetUpdates, token, body)
	if err != nil {
		return nil, "", fmt.Errorf("weixin getupdates: %w", err)
	}

	timeout := c.LongPollTimeout
	if timeout <= 0 {
		timeout = defaultLongPoll
	}
	httpClient := c.HTTP
	if httpClient.Timeout == 0 || httpClient.Timeout < timeout {
		clone := *httpClient
		clone.Timeout = timeout + 5*time.Second
		httpClient = &clone
	}

	var resp getUpdatesResponse
	if err := c.doJSONWithClient(httpClient, req, &resp); err != nil {
		return nil, "", fmt.Errorf("weixin getupdates: %w", err)
	}
	if resp.Ret != 0 || resp.ErrCode != 0 {
		return nil, "", fmt.Errorf("weixin getupdates: ret=%d errcode=%d errmsg=%s", resp.Ret, resp.ErrCode, resp.ErrMsg)
	}
	if resp.LongPollTimeoutMS > 0 {
		c.LongPollTimeout = time.Duration(resp.LongPollTimeoutMS) * time.Millisecond
	}

	updates := make([]Update, 0, len(resp.Msgs))
	for _, m := range resp.Msgs {
		u := Update{
			PeerID:       m.FromUserID,
			ContextToken: m.ContextToken,
		}
		for _, item := range m.ItemList {
			switch item.Type {
			case itemTypeText:
				if item.TextItem != nil {
					if u.Text != "" {
						u.Text += "\n"
					}
					u.Text += item.TextItem.Text
				}
			default:
				if ref := mediaRefFromItem(item); ref != nil {
					u.Media = append(u.Media, *ref)
				}
			}
		}
		updates = append(updates, u)
	}
	return updates, resp.GetUpdatesBuf, nil
}

func (c *Client) SendMessage(ctx context.Context, token string, msg OutboundMessage) error {
	clientID := msg.ClientID
	if clientID == "" {
		clientID = fmt.Sprintf("baize:%d-%d", time.Now().UnixMilli(), rand.Intn(1<<16))
	}
	body := sendMessageRequest{
		Msg: wireOutboundMessage{
			FromUserID:   "",
			ToUserID:     msg.ToUserID,
			ClientID:     clientID,
			MessageType:  messageTypeBot,
			MessageState: messageStateFinish,
			ContextToken: msg.ContextToken,
			ItemList: []wireItem{{
				Type:     itemTypeText,
				TextItem: &wireTextItem{Text: msg.Text},
			}},
		},
		BaseInfo: baseInfo{ChannelVersion: channelVersion},
	}
	req, err := c.newAuthJSONRequest(ctx, http.MethodPost, c.BaseURL+pathSendMessage, token, body)
	if err != nil {
		return fmt.Errorf("weixin sendmessage: %w", err)
	}
	if err := c.doJSON(req, nil); err != nil {
		return fmt.Errorf("weixin sendmessage: %w", err)
	}
	return nil
}

// DownloadMedia GETs raw media bytes. AES CDN decryption is left as TODO when key/format is unclear.
func (c *Client) DownloadMedia(ctx context.Context, _ string, media MediaRef) ([]byte, error) {
	downloadURL := media.URL
	if downloadURL == "" && media.EncryptQueryParam != "" {
		downloadURL = strings.TrimRight(c.CDNBaseURL, "/") + "/download?encrypted_query_param=" + url.QueryEscape(media.EncryptQueryParam)
	}
	if downloadURL == "" {
		return nil, fmt.Errorf("weixin download media: empty url and encrypt_query_param")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("weixin download media: build request: %w", err)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weixin download media: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("weixin download media: http %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("weixin download media: read body: %w", err)
	}
	// TODO: decrypt AES-ECB when aes_key / encrypt_type encoding is confirmed for this media.
	_ = media.AESKey
	return data, nil
}

func mediaRefFromItem(item wireItem) *MediaRef {
	var media *wireCDNMedia
	var fileName string
	var aesOverride string
	switch {
	case item.ImageItem != nil:
		media = item.ImageItem.Media
		fileName = item.ImageItem.FileName
		aesOverride = item.ImageItem.AESKey
	case item.FileItem != nil:
		media = item.FileItem.Media
		fileName = item.FileItem.FileName
	case item.VoiceItem != nil:
		media = item.VoiceItem.Media
		fileName = item.VoiceItem.FileName
	case item.VideoItem != nil:
		media = item.VideoItem.Media
		fileName = item.VideoItem.FileName
	}
	if media == nil {
		return nil
	}
	aesKey := media.AESKey
	if aesOverride != "" {
		aesKey = aesOverride
	}
	return &MediaRef{
		EncryptQueryParam: media.EncryptQueryParam,
		AESKey:            aesKey,
		FileName:          fileName,
	}
}

func (c *Client) newAuthJSONRequest(ctx context.Context, method, rawURL, token string, body any) (*http.Request, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("AuthorizationType", authTypeILinkBotToken)
	req.Header.Set("X-WECHAT-UIN", randomWechatUIN())
	return req, nil
}

func (c *Client) doJSON(req *http.Request, out any) error {
	return c.doJSONWithClient(c.HTTP, req, out)
}

func (c *Client) doJSONWithClient(httpClient *http.Client, req *http.Request, out any) error {
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http %s: %s", resp.Status, truncate(string(data), 256))
	}
	if out == nil || len(data) == 0 || string(data) == "{}" {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	return nil
}

func randomWechatUIN() string {
	v := rand.Uint32()
	return base64.StdEncoding.EncodeToString([]byte(strconv.FormatUint(uint64(v), 10)))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
