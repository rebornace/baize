package weixinlink

import (
	"bytes"
	"context"
	"crypto/aes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestSendImageThreeStepUpload(t *testing.T) {
	var (
		mu        sync.Mutex
		gotUpload map[string]any
		cdnBody   []byte
		sendMsg   map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ilink/bot/getuploadurl":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotUpload)
			// CDN upload URL points back at this server under /cdn.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"upload_param":    "up-param",
				"upload_full_url": "",
			})
		case strings.HasPrefix(r.URL.Path, "/cdn/upload"):
			b, _ := io.ReadAll(r.Body)
			cdnBody = b
			// CDN returns the download handle in a response header.
			w.Header().Set("x-encrypted-param", "dl-param-XYZ")
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/ilink/bot/sendmessage":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &sendMsg)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	// NewClient sets CDNBaseURL to the default; override for the test.
	c := NewClient(srv.URL, srv.Client())
	c.CDNBaseURL = srv.URL + "/cdn"

	img := []byte("FAKE-PNG-BYTES")
	if err := c.SendImage(context.Background(), "tok", "peer@im.wechat", "pic.png", "image/png", img, "ctx-tok"); err != nil {
		t.Fatalf("SendImage: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	// Step 1: getuploadurl fields.
	if gotUpload["media_type"].(float64) != 1 {
		t.Fatalf("media_type=%v want 1", gotUpload["media_type"])
	}
	if gotUpload["to_user_id"] != "peer@im.wechat" {
		t.Fatalf("to_user_id=%v", gotUpload["to_user_id"])
	}
	if gotUpload["rawsize"].(float64) != float64(len(img)) {
		t.Fatalf("rawsize=%v", gotUpload["rawsize"])
	}
	if gotUpload["rawfilemd5"] == "" || gotUpload["aeskey"] == "" {
		t.Fatalf("missing md5/aeskey: %v", gotUpload)
	}
	if gotUpload["filesize"].(float64) != float64(len(cdnBody)) {
		t.Fatalf("filesize=%v cdn=%d", gotUpload["filesize"], len(cdnBody))
	}
	// CDN got ciphertext that decrypts back to the image with the advertised
	// aeskey (hex).
	keyBytes, err := hex.DecodeString(gotUpload["aeskey"].(string))
	if err != nil || len(keyBytes) != 16 {
		t.Fatalf("aeskey not 16-byte hex: %v", gotUpload["aeskey"])
	}
	block, _ := aes.NewCipher(keyBytes)
	if len(cdnBody)%16 != 0 {
		t.Fatalf("cdn body not block aligned: %d", len(cdnBody))
	}
	pt := make([]byte, len(cdnBody))
	for i := 0; i < len(cdnBody); i += 16 {
		block.Decrypt(pt[i:i+16], cdnBody[i:i+16])
	}
	if !bytes.Contains(pt, img) {
		t.Fatalf("decrypted CDN body does not contain image: %q", pt)
	}
	// Step 3: sendmessage image_item references x-encrypted-param + base64 key.
	msg := sendMsg["msg"].(map[string]any)
	if msg["context_token"] != "ctx-tok" || msg["to_user_id"] != "peer@im.wechat" {
		t.Fatalf("send msg envelope=%v", msg)
	}
	items := msg["item_list"].([]any)
	item := items[0].(map[string]any)
	if item["type"].(float64) != 2 {
		t.Fatalf("item type=%v want 2 (image)", item["type"])
	}
	imgItem := item["image_item"].(map[string]any)
	media := imgItem["media"].(map[string]any)
	if media["encrypt_query_param"] != "dl-param-XYZ" {
		t.Fatalf("encrypt_query_param=%v", media["encrypt_query_param"])
	}
	if media["aes_key"] != base64.StdEncoding.EncodeToString(keyBytes) {
		t.Fatalf("aes_key=%v", media["aes_key"])
	}
}

func TestSendFileItemTypeFour(t *testing.T) {
	var sendMsg map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ilink/bot/getuploadurl":
			_ = json.NewEncoder(w).Encode(map[string]any{"upload_param": "up"})
		case strings.HasPrefix(r.URL.Path, "/cdn/upload"):
			w.Header().Set("x-encrypted-param", "dl-file")
		case r.URL.Path == "/ilink/bot/sendmessage":
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &sendMsg)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, srv.Client())
	c.CDNBaseURL = srv.URL + "/cdn"

	if err := c.SendFile(context.Background(), "tok", "peer@im.wechat", "report.pdf", "application/pdf", []byte("PDFDATA"), ""); err != nil {
		t.Fatalf("SendFile: %v", err)
	}
	item := sendMsg["msg"].(map[string]any)["item_list"].([]any)[0].(map[string]any)
	if item["type"].(float64) != 4 {
		t.Fatalf("file item type=%v want 4", item["type"])
	}
	fileItem := item["file_item"].(map[string]any)
	if fileItem["file_name"] != "report.pdf" {
		t.Fatalf("file_name=%v", fileItem["file_name"])
	}
	if fileItem["media"].(map[string]any)["encrypt_query_param"] != "dl-file" {
		t.Fatalf("file media param missing")
	}
}
