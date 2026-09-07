package weixinlink

import (
	"context"
	"crypto/aes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// iLink media uses AES-128-ECB + PKCS7 padding for both inbound CDN downloads
// (decrypt) and outbound CDN uploads (encrypt).

// MediaDownloader downloads and (when keyed) decrypts inbound CDN media.
type MediaDownloader interface {
	// DownloadMediaDecrypted returns plaintext and decrypted=true when the
	// media carried a usable AES key and decrypted cleanly; raw bytes and
	// decrypted=false when no key is present; nil and decrypted=false when a
	// key was present but decryption failed (caller degrades to a filename
	// placeholder rather than forwarding ciphertext garbage).
	DownloadMediaDecrypted(ctx context.Context, token string, m MediaRef) (data []byte, decrypted bool, err error)
}

func encryptAES128ECB(key, plaintext []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic("weixinlink: invalid aes key: " + err.Error())
	}
	padded := pkcs7Pad(plaintext, block.BlockSize())
	out := make([]byte, len(padded))
	bs := block.BlockSize()
	for i := 0; i < len(padded); i += bs {
		block.Encrypt(out[i:i+bs], padded[i:i+bs])
	}
	return out
}

func decryptAES128ECB(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	bs := block.BlockSize()
	if len(ciphertext) == 0 || len(ciphertext)%bs != 0 {
		return nil, fmt.Errorf("weixinlink: ciphertext not a multiple of block size %d", bs)
	}
	out := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += bs {
		block.Decrypt(out[i:i+bs], ciphertext[i:i+bs])
	}
	return pkcs7Unpad(out, bs)
}

func pkcs7Pad(in []byte, blockSize int) []byte {
	pad := blockSize - len(in)%blockSize
	out := make([]byte, len(in)+pad)
	copy(out, in)
	for i := len(in); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out
}

func pkcs7Unpad(in []byte, blockSize int) ([]byte, error) {
	if len(in) == 0 || len(in)%blockSize != 0 {
		return nil, fmt.Errorf("weixinlink: invalid padded length")
	}
	pad := int(in[len(in)-1])
	if pad < 1 || pad > blockSize {
		return nil, fmt.Errorf("weixinlink: invalid PKCS7 padding %d", pad)
	}
	for i := len(in) - pad; i < len(in); i++ {
		if int(in[i]) != pad {
			return nil, fmt.Errorf("weixinlink: inconsistent PKCS7 padding")
		}
	}
	return in[:len(in)-pad], nil
}

// resolveAESKey parses the inbound media aes key in one of:
//   - image_item.aeskey: 32-char hex of the 16-byte key
//   - media.aes_key format A: base64(raw 16 bytes)
//   - media.aes_key format B: base64(32-char hex string)
func resolveAESKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("weixinlink: empty aes key")
	}
	if b, err := hex.DecodeString(s); err == nil && len(b) == 16 {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		if len(b) == 16 {
			return b, nil
		}
		if h := strings.TrimSpace(string(b)); len(h) == 32 {
			if kb, err := hex.DecodeString(h); err == nil && len(kb) == 16 {
				return kb, nil
			}
		}
	}
	return nil, fmt.Errorf("weixinlink: unrecognized aes key format")
}

// DownloadMediaDecrypted downloads CDN bytes and decrypts them when keyed.
func (c *Client) DownloadMediaDecrypted(ctx context.Context, _ string, m MediaRef) ([]byte, bool, error) {
	raw, err := c.DownloadMedia(ctx, "", m) // CDN needs no bearer token
	if err != nil {
		return nil, false, err
	}
	// No key at all: the media is plaintext (or an unkeyed CDN), forward as-is.
	if strings.TrimSpace(m.AESKey) == "" {
		return raw, false, nil
	}
	key, kerr := resolveAESKey(m.AESKey)
	if kerr != nil {
		// A key was advertised but is unparseable: degrade (do NOT forward
		// ciphertext garbage as if it were content). Caller skips the bytes.
		return nil, false, nil
	}
	pt, derr := decryptAES128ECB(key, raw)
	if derr != nil {
		return nil, false, nil // key present but not decryptable: degrade
	}
	return pt, true, nil
}

var _ MediaDownloader = (*Client)(nil)
