package openapi_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/connectortls"
	"github.com/rebornace/baize/internal/store"
)

func TestInvokerInvokeOverMTLS(t *testing.T) {
	srv, mtlsClient := startRequireClientCertServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/ping" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	t.Cleanup(srv.Close)

	inv := &openapi.Invoker{
		BaseURL:    srv.URL,
		HTTPClient: mtlsClient,
		Tools: []openapi.ToolRoute{{
			Name:   "ping",
			Method: http.MethodGet,
			Path:   "/v1/ping",
		}},
	}
	res, err := inv.Invoke(context.Background(), "ping", nil)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("is_error content=%+v", res.Content)
	}
	if res.Content["ok"] != true {
		t.Fatalf("content=%+v", res.Content)
	}
}

func startRequireClientCertServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *http.Client) {
	t.Helper()
	dir := t.TempDir()
	caCert, caKey := mustGenerateCA(t)
	caPath := writePEM(t, dir, "ca.pem", caCert)

	serverCert, serverKey := mustGenerateCert(t, caCert, caKey, false, "server", net.ParseIP("127.0.0.1"))
	clientCert, clientKey := mustGenerateCert(t, caCert, caKey, true, "client", nil)

	clientCertPath := writePEM(t, dir, "client.crt", clientCert)
	clientKeyPath := writeKey(t, dir, "client.key", clientKey)
	serverCertPath := writePEM(t, dir, "server.crt", serverCert)
	serverKeyPath := writeKey(t, dir, "server.key", serverKey)

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCert)

	srv := httptest.NewUnstartedServer(handler)
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{mustLoadPair(t, serverCertPath, serverKeyPath)},
		ClientAuth:   tls.RequireAnyClientCert,
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS12,
	}
	srv.StartTLS()

	cl, err := connectortls.HTTPClient(5*time.Second, &store.ConnectorTLS{
		CertFile: clientCertPath,
		KeyFile:  clientKeyPath,
		CAFile:   caPath,
	})
	if err != nil {
		t.Fatalf("HTTPClient: %v", err)
	}
	return srv, cl
}

func mustGenerateCA(t *testing.T) ([]byte, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), key
}

func mustGenerateCert(t *testing.T, caPEM []byte, caKey *rsa.PrivateKey, clientAuth bool, cn string, ipSAN net.IP) ([]byte, *rsa.PrivateKey) {
	t.Helper()
	block, _ := pem.Decode(caPEM)
	if block == nil {
		t.Fatal("decode CA")
	}
	ca, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	extKU := []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	if clientAuth {
		extKU = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  extKU,
	}
	if ipSAN != nil {
		tmpl.IPAddresses = []net.IP{ipSAN}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), key
}

func writePEM(t *testing.T, dir, name string, pemBytes []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeKey(t *testing.T, dir, name string, key *rsa.PrivateKey) string {
	t.Helper()
	der := x509.MarshalPKCS1PrivateKey(key)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	return writePEM(t, dir, name, pemBytes)
}

func mustLoadPair(t *testing.T, certPath, keyPath string) tls.Certificate {
	t.Helper()
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}
