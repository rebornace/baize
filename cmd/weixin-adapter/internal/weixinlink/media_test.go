package weixinlink

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
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

	// Non-empty but unparseable key: must degrade to nil, NOT forward the raw
	// ciphertext (which would otherwise be mis-sent as model content).
	bad, dec4, err := c.DownloadMediaDecrypted(context.Background(), "tok",
		MediaRef{URL: srv.URL + "/f", AESKey: "not-a-valid-key"})
	if err != nil {
		t.Fatalf("malformed key should not error: %v", err)
	}
	if dec4 || bad != nil {
		t.Fatalf("malformed key: dec=%v data=%v (want nil degradation)", dec4, bad)
	}
}

// PNG magic bytes (http.DetectContentType reports image/png).
var pngMagic = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0}

func TestNormalizeMediaImageSniffsMIMEAndNamesExtension(t *testing.T) {
	// iLink image items carry no file_name; the decoded bytes determine the
	// type and a filename with the correct extension is synthesized (the old
	// code defaulted these to "media.bin").
	m := NormalizeMedia(&MediaRef{Kind: "image"}, pngMagic)
	if m.MIME != "image/png" {
		t.Fatalf("image MIME = %q, want image/png", m.MIME)
	}
	if m.FileName == "" || m.FileName == "media.bin" {
		t.Fatalf("image FileName = %q, want synthesized non-media.bin name", m.FileName)
	}
	if !strings.HasSuffix(m.FileName, ".png") {
		t.Fatalf("image FileName = %q, want .png extension", m.FileName)
	}

	// JPEG magic bytes -> .jpg.
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 8)...)
	mj := NormalizeMedia(&MediaRef{Kind: "image"}, jpeg)
	if mj.MIME != "image/jpeg" {
		t.Fatalf("jpeg MIME = %q, want image/jpeg", mj.MIME)
	}
	if !strings.HasSuffix(mj.FileName, ".jpg") {
		t.Fatalf("jpeg FileName = %q, want .jpg", mj.FileName)
	}

	// An explicitly provided filename is preserved.
	named := NormalizeMedia(&MediaRef{Kind: "image", FileName: "photo.jpg"}, pngMagic)
	if named.FileName != "photo.jpg" {
		t.Fatalf("explicit filename overwritten: %q", named.FileName)
	}
}

func TestNormalizeMediaFileKeepsExtensionMIME(t *testing.T) {
	// A .pdf file must keep its extension-derived MIME (old code hardcoded
	// application/octet-stream for every file_item).
	f := NormalizeMedia(&MediaRef{Kind: "file", FileName: "report.pdf", MIME: "application/pdf"}, []byte("%PDF-1.4"))
	if f.MIME != "application/pdf" {
		t.Fatalf("pdf MIME = %q, want application/pdf", f.MIME)
	}
	if f.FileName != "report.pdf" {
		t.Fatalf("pdf FileName = %q", f.FileName)
	}

	// An unknown binary with no usable extension stays octet-stream and gets a
	// non-empty synthesized name.
	bin := NormalizeMedia(&MediaRef{Kind: "file", FileName: "blob.xyz"}, []byte{0x00, 0x01, 0x02, 0x03})
	if bin.MIME != "application/octet-stream" {
		t.Fatalf("binary MIME = %q, want application/octet-stream", bin.MIME)
	}
}

func TestMediaRefFromFileItemTypesByExtension(t *testing.T) {
	item := wireItem{
		FileItem: &wireMediaItem{
			Media:    &wireCDNMedia{EncryptQueryParam: "qp", AESKey: "k"},
			FileName: "deck.docx",
		},
	}
	ref := mediaRefFromItem(item)
	if ref == nil {
		t.Fatal("expected ref")
	}
	if ref.Kind != "file" {
		t.Fatalf("Kind = %q, want file", ref.Kind)
	}
	if ref.MIME != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Fatalf("file MIME = %q, want docx MIME", ref.MIME)
	}

	img := mediaRefFromItem(wireItem{
		ImageItem: &wireMediaItem{Media: &wireCDNMedia{EncryptQueryParam: "qp"}},
	})
	if img.Kind != "image" {
		t.Fatalf("image Kind = %q, want image", img.Kind)
	}
	// Image MIME is left for byte-sniffing after download (not hardcoded).
	if img.MIME != "" {
		t.Fatalf("image MIME before download = %q, want empty (sniff later)", img.MIME)
	}
}
