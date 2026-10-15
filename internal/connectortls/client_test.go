package connectortls_test

import (
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

	"github.com/rebornace/baize/internal/connectortls"
	"github.com/rebornace/baize/internal/store"
)

func TestValidatePairRequired(t *testing.T) {
	err := connectortls.Validate(&store.ConnectorTLS{CertFile: "a.crt"})
	if err == nil {
		t.Fatal("want error")
	}
	err = connectortls.Validate(&store.ConnectorTLS{KeyFile: "a.key"})
	if err == nil {
		t.Fatal("want error for key without cert")
	}
	if err := connectortls.Validate(&store.ConnectorTLS{CAFile: "ca.pem"}); err != nil {
		t.Fatalf("CA-only: %v", err)
	}
	if err := connectortls.Validate(nil); err != nil {
		t.Fatalf("nil: %v", err)
	}
}

func TestEmpty(t *testing.T) {
	if !connectortls.Empty(nil) {
		t.Fatal("nil should be empty")
	}
	if !connectortls.Empty(&store.ConnectorTLS{}) {
		t.Fatal("zero struct should be empty")
	}
	if connectortls.Empty(&store.ConnectorTLS{CAFile: "x"}) {
		t.Fatal("CA-only is not empty")
	}
}

func TestHTTPClientTransportMatchesDefaultBasics(t *testing.T) {
	dir := t.TempDir()
	caCert, _ := mustGenerateCA(t)
	caPath := writePEM(t, dir, "ca.pem", "CERTIFICATE", caCert)

	cl, err := connectortls.HTTPClient(time.Second, &store.ConnectorTLS{CAFile: caPath})
	if err != nil {
		t.Fatal(err)
	}
	tr, ok := cl.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type %T", cl.Transport)
	}
	def, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatal("DefaultTransport not *http.Transport")
	}
	if tr.Proxy == nil {
		t.Fatal("Proxy unset; want http.ProxyFromEnvironment via DefaultTransport clone")
	}
	if tr.DialContext == nil {
		t.Fatal("DialContext unset")
	}
	if tr.TLSHandshakeTimeout != def.TLSHandshakeTimeout {
		t.Fatalf("TLSHandshakeTimeout=%v want %v", tr.TLSHandshakeTimeout, def.TLSHandshakeTimeout)
	}
}

func TestHTTPClientWithClientCert(t *testing.T) {
	dir := t.TempDir()
	caCert, caKey := mustGenerateCA(t)
	caPath := writePEM(t, dir, "ca.pem", "CERTIFICATE", caCert)

	serverCert, serverKey := mustGenerateCert(t, caCert, caKey, false, "server", net.ParseIP("127.0.0.1"))
	clientCert, clientKey := mustGenerateCert(t, caCert, caKey, true, "client", nil)

	clientCertPath := writePEM(t, dir, "client.crt", "CERTIFICATE", clientCert)
	clientKeyPath := writeKey(t, dir, "client.key", clientKey)
	serverCertPath := writePEM(t, dir, "server.crt", "CERTIFICATE", serverCert)
	serverKeyPath := writeKey(t, dir, "server.key", serverKey)

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCert)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{mustLoadPair(t, serverCertPath, serverKeyPath)},
		ClientAuth:   tls.RequireAnyClientCert,
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS12,
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	cl, err := connectortls.HTTPClient(5*time.Second, &store.ConnectorTLS{
		CertFile: clientCertPath,
		KeyFile:  clientKeyPath,
		CAFile:   caPath,
	})
	if err != nil {
		t.Fatalf("HTTPClient: %v", err)
	}
	resp, err := cl.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET with client cert: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", resp.StatusCode)
	}

	plain, err := connectortls.HTTPClient(5*time.Second, &store.ConnectorTLS{CAFile: caPath})
	if err != nil {
		t.Fatalf("HTTPClient CA-only: %v", err)
	}
	resp2, err := plain.Get(srv.URL)
	if err == nil {
		resp2.Body.Close()
		t.Fatal("want error without client cert")
	}
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
	ku := x509.KeyUsageDigitalSignature
	extKU := []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	if clientAuth {
		extKU = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     ku,
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

func writePEM(t *testing.T, dir, name, typ string, pemBytes []byte) string {
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
	return writePEM(t, dir, name, "RSA PRIVATE KEY", pemBytes)
}

func mustLoadPair(t *testing.T, certPath, keyPath string) tls.Certificate {
	t.Helper()
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}
