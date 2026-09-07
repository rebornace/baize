package weixinlink

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAESRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef") // 16 bytes
	for _, plain := range [][]byte{
		[]byte("a"),
		[]byte("exactly 16 bytes"), // len 16 -> full block of padding
		[]byte("hello weixin image bytes"),
		bytes.Repeat([]byte{0xAB}, 100),
	} {
		ct := encryptAES128ECB(key, plain)
		if len(ct)%16 != 0 {
			t.Fatalf("ciphertext not block-aligned: %d", len(ct))
		}
		if bytes.Equal(ct, plain) {
			t.Fatal("ciphertext equals plaintext")
		}
		got, err := decryptAES128ECB(key, ct)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if !bytes.Equal(got, plain) {
			t.Fatalf("round-trip mismatch: got %q want %q", got, plain)
		}
	}
}

func TestDecryptRejectsBadPadding(t *testing.T) {
	key := []byte("0123456789abcdef")
	bad := bytes.Repeat([]byte{0xFF}, 32) // not valid PKCS7
	if _, err := decryptAES128ECB(key, bad); err == nil {
		t.Fatal("expected padding error")
	}
}

func TestResolveAESKey(t *testing.T) {
	raw := []byte("0123456789abcdef")
	hexKey := hex.EncodeToString(raw)                           // image_item.aeskey
	b64Raw := base64.StdEncoding.EncodeToString(raw)            // format A
	b64Hex := base64.StdEncoding.EncodeToString([]byte(hexKey)) // format B
	for _, in := range []string{hexKey, b64Raw, b64Hex} {
		got, err := resolveAESKey(in)
		if err != nil {
			t.Fatalf("resolveAESKey(%q): %v", in, err)
		}
		if !bytes.Equal(got, raw) {
			t.Fatalf("resolveAESKey(%q) = %x", in, got)
		}
	}
	if _, err := resolveAESKey(""); err == nil {
		t.Fatal("empty key should error")
	}
	if _, err := resolveAESKey("not-a-key"); err == nil {
		t.Fatal("garbage key should error")
	}
}

func TestDownloadMediaDecrypted(t *testing.T) {
	key := []byte("0123456789abcdef")
	plain := []byte("PNG-PLAINTEXT-BYTES")
	ciphertext := encryptAES128ECB(key, plain)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(ciphertext)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, srv.Client())
	ref := MediaRef{URL: srv.URL + "/f", AESKey: base64.StdEncoding.EncodeToString(key)}
	data, dec, err := c.DownloadMediaDecrypted(context.Background(), "tok", ref)
	if err != nil {
		t.Fatalf("DownloadMediaDecrypted: %v", err)
	}
	if !dec || !bytes.Equal(data, plain) {
		t.Fatalf("dec=%v data=%q", dec, data)
	}

	// No key -> raw bytes, decrypted=false.
	raw, dec2, err := c.DownloadMediaDecrypted(context.Background(), "tok", MediaRef{URL: srv.URL + "/f"})
	if err != nil || dec2 || !bytes.Equal(raw, ciphertext) {
		t.Fatalf("no-key: dec=%v err=%v", dec2, err)
	}

	// Key present but body not encrypted -> nil, decrypted=false (degrade).
	plainSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-encrypted-plain"))
	}))
	t.Cleanup(plainSrv.Close)
	c2 := NewClient(plainSrv.URL, plainSrv.Client())
	data3, dec3, err := c2.DownloadMediaDecrypted(context.Background(), "tok",
		MediaRef{URL: plainSrv.URL + "/f", AESKey: base64.StdEncoding.EncodeToString(key)})
	if err != nil {
		t.Fatalf("undecryptable should not error: %v", err)
	}
	if dec3 || data3 != nil {
		t.Fatalf("undecryptable: dec=%v data=%v", dec3, data3)
	}
}
