package settingscrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
)

const Prefix = "bz1:"

var (
	ErrNoKey      = errors.New("BAIZE_SETTINGS_KEY is not set")
	ErrCiphertext = errors.New("encrypted secret requires BAIZE_SETTINGS_KEY")
	ErrCorrupt    = errors.New("corrupt sealed secret")
)

var base64KeyPattern = regexp.MustCompile(`^[A-Za-z0-9+/]+=*$`)

// Key is a 32-byte AES key. nil means "no key configured".
type Key []byte

func KeyFromEnv() (Key, error) {
	raw := os.Getenv("BAIZE_SETTINGS_KEY")
	if raw == "" {
		return nil, nil
	}
	material := keyMaterial(raw)
	sum := sha256.Sum256(material)
	return Key(sum[:]), nil
}

func keyMaterial(raw string) []byte {
	if base64KeyPattern.MatchString(raw) {
		decoded, err := base64.StdEncoding.DecodeString(raw)
		if err == nil && len(decoded) >= 16 {
			return decoded
		}
	}
	return []byte(raw)
}

func IsSealed(s string) bool {
	return strings.HasPrefix(s, Prefix)
}

func Seal(key Key, plaintext string) (string, error) {
	if len(key) == 0 {
		return "", ErrNoKey
	}
	if plaintext == "" || IsSealed(plaintext) {
		return plaintext, nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	payload := make([]byte, len(nonce)+len(sealed))
	copy(payload, nonce)
	copy(payload[len(nonce):], sealed)
	return Prefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

func Open(key Key, value string) (string, error) {
	if !IsSealed(value) {
		return value, nil
	}
	if len(key) == 0 {
		return "", ErrCiphertext
	}
	payload, err := base64.RawURLEncoding.DecodeString(value[len(Prefix):])
	if err != nil {
		return "", ErrCorrupt
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(payload) < gcm.NonceSize() {
		return "", ErrCorrupt
	}
	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrCorrupt
	}
	return string(plain), nil
}
