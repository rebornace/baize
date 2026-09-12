package weixinlink

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	pathGetUploadURL = "/ilink/bot/getuploadurl"
	mediaTypeImage   = 1 // getuploadurl media_type: image
	mediaTypeFile    = 3 // getuploadurl media_type: file
	itemTypeImage    = 2 // sendmessage item type: image
	itemTypeFile     = 4 // sendmessage item type: file
)

type uploadURLResponse struct {
	UploadParam   string `json:"upload_param"`
	UploadFullURL string `json:"upload_full_url"`
	Ret           int    `json:"ret"`
	ErrCode       int    `json:"errcode"`
	ErrMsg        string `json:"errmsg"`
}

// SendImage encrypts data, uploads it to the iLink CDN, and sends an image
// message item to toUserID. contextToken is the inbound message's token.
func (c *Client) SendImage(ctx context.Context, token, toUserID, filename, mime string, data []byte, contextToken string) error {
	dlParam, aesKeyB64, cipherLen, err := c.uploadMedia(ctx, token, toUserID, mediaTypeImage, data)
	if err != nil {
		return fmt.Errorf("weixin upload image: %w", err)
	}
	return c.sendItems(ctx, token, toUserID, contextToken, "", []wireItem{{
		Type: itemTypeImage,
		ImageItem: &wireMediaItem{
			Media:   &wireCDNMedia{EncryptQueryParam: dlParam, AESKey: aesKeyB64, EncryptType: 1},
			MidSize: cipherLen,
		},
	}})
}

// SendFile encrypts data, uploads it, and sends a file attachment item.
func (c *Client) SendFile(ctx context.Context, token, toUserID, filename, mime string, data []byte, contextToken string) error {
	dlParam, aesKeyB64, _, err := c.uploadMedia(ctx, token, toUserID, mediaTypeFile, data)
	if err != nil {
		return fmt.Errorf("weixin upload file: %w", err)
	}
	name := strings.TrimSpace(filename)
	if name == "" {
		name = "file.bin"
	}
	return c.sendItems(ctx, token, toUserID, contextToken, "", []wireItem{{
		Type: itemTypeFile,
		FileItem: &wireMediaItem{
			Media:    &wireCDNMedia{EncryptQueryParam: dlParam, AESKey: aesKeyB64, EncryptType: 1},
			FileName: name,
			Len:      strconv.Itoa(len(data)),
		},
	}})
}

// uploadMedia performs the three-step iLink outbound media flow: generate an
// AES key + encrypt (AES-128-ECB/PKCS7), request a CDN upload URL, PUT the
// ciphertext, and return the CDN download handle (x-encrypted-param), the
// base64 AES key for the sendmessage item, and the ciphertext length.
func (c *Client) uploadMedia(ctx context.Context, token, toUserID string, mediaType int, data []byte) (string, string, int64, error) {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return "", "", 0, fmt.Errorf("gen aes key: %w", err)
	}
	filekeyRaw := make([]byte, 16)
	if _, err := rand.Read(filekeyRaw); err != nil {
		return "", "", 0, fmt.Errorf("gen filekey: %w", err)
	}
	filekeyHex := hex.EncodeToString(filekeyRaw)
	ciphertext := encryptAES128ECB(key, data)
	sum := md5.Sum(data)

	reqBody := map[string]any{
		"filekey":       filekeyHex,
		"media_type":    mediaType,
		"to_user_id":    toUserID,
		"rawsize":       len(data),
		"rawfilemd5":    hex.EncodeToString(sum[:]),
		"filesize":      len(ciphertext),
		"no_need_thumb": true,
		"aeskey":        hex.EncodeToString(key),
		"base_info":     map[string]string{"channel_version": channelVersion},
	}
	preq, err := c.newAuthJSONRequest(ctx, http.MethodPost, c.BaseURL+pathGetUploadURL, token, reqBody)
	if err != nil {
		return "", "", 0, err
	}
	var up uploadURLResponse
	if err := c.doJSON(preq, &up); err != nil {
		return "", "", 0, fmt.Errorf("getuploadurl: %w", err)
	}
	if up.Ret != 0 || up.ErrCode != 0 {
		return "", "", 0, fmt.Errorf("getuploadurl: ret=%d errcode=%d errmsg=%s", up.Ret, up.ErrCode, up.ErrMsg)
	}
	uploadURL := strings.TrimSpace(up.UploadFullURL)
	if uploadURL == "" {
		uploadURL = strings.TrimRight(c.CDNBaseURL, "/") +
			"/upload?encrypted_query_param=" + url.QueryEscape(up.UploadParam) +
			"&filekey=" + url.QueryEscape(filekeyHex)
	}

	putReq, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(ciphertext))
	if err != nil {
		return "", "", 0, err
	}
	putReq.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.HTTP.Do(putReq)
	if err != nil {
		return "", "", 0, fmt.Errorf("cdn upload: %w", err)
	}
	dlParam := resp.Header.Get("x-encrypted-param")
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", 0, fmt.Errorf("cdn upload: http %s", resp.Status)
	}
	if dlParam == "" {
		return "", "", 0, fmt.Errorf("cdn upload: missing x-encrypted-param response header")
	}
	return dlParam, base64.StdEncoding.EncodeToString(key), int64(len(ciphertext)), nil
}
