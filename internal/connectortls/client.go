package connectortls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/rebornace/baize/internal/store"
)

var errCertKeyPair = errors.New("connectortls: cert_file and key_file must both be set or both empty")

// Empty reports whether TLS is unconfigured (nil or all path fields empty).
func Empty(t *store.ConnectorTLS) bool {
	if t == nil {
		return true
	}
	return t.CertFile == "" && t.KeyFile == "" && t.CAFile == ""
}

// Validate checks TLS path configuration. Nil or all-empty is valid; cert and key must be paired.
func Validate(t *store.ConnectorTLS) error {
	if Empty(t) {
		return nil
	}
	hasCert := t.CertFile != ""
	hasKey := t.KeyFile != ""
	if hasCert != hasKey {
		return errCertKeyPair
	}
	return nil
}

// HTTPClient builds an http.Client with optional client mTLS and custom CA roots.
// Unconfigured TLS returns a client with only the timeout set.
func HTTPClient(timeout time.Duration, t *store.ConnectorTLS) (*http.Client, error) {
	if err := Validate(t); err != nil {
		return nil, err
	}
	if Empty(t) {
		return &http.Client{Timeout: timeout}, nil
	}

	rootPool, err := x509.SystemCertPool()
	if err != nil || rootPool == nil {
		rootPool = x509.NewCertPool()
	}
	if t.CAFile != "" {
		pem, err := os.ReadFile(t.CAFile)
		if err != nil {
			return nil, fmt.Errorf("connectortls: read ca_file: %w", err)
		}
		if !rootPool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("connectortls: no certificates found in ca_file")
		}
	}

	tlsCfg := &tls.Config{
		RootCAs:            rootPool,
		InsecureSkipVerify: false,
		MinVersion:         tls.VersionTLS12,
	}
	if t.CertFile != "" && t.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(t.CertFile, t.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("connectortls: load client cert/key: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	// Clone DefaultTransport so HTTP_PROXY/HTTPS_PROXY/NO_PROXY behave like http.DefaultClient.
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("connectortls: unexpected DefaultTransport type %T", http.DefaultTransport)
	}
	tr := base.Clone()
	tr.TLSClientConfig = tlsCfg

	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
	}, nil
}
