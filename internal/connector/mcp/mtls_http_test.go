package mcp_test

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

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mcpbridge "github.com/rebornace/baize/internal/connector/mcp"
	"github.com/rebornace/baize/internal/connectortls"
	"github.com/rebornace/baize/internal/store"
)

func TestConnectHTTPOverMTLS(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "mtls-mock", Version: "v0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "echo",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args httpEchoArgs) (*mcp.CallToolResult, httpEchoOutput, error) {
		return nil, httpEchoOutput(args), nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)

	httpServer, mtlsClient := startRequireClientCertServer(t, handler)
	t.Cleanup(httpServer.Close)

	ctx := context.Background()
	session, err := mcpbridge.ConnectHTTP(ctx, httpServer.URL, nil, mtlsClient)
	if err != nil {
		t.Fatalf("ConnectHTTP over mTLS: %v", err)
	}
	defer session.Close()

	tools, err := mcpbridge.DiscoverTools(ctx, session, "mtls-mcp")
	if err != nil {
		t.Fatalf("DiscoverTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools=%+v", tools)
	}

	// Without client cert, handshake must fail.
	if _, err := mcpbridge.ConnectHTTP(ctx, httpServer.URL, nil, nil); err == nil {
		t.Fatal("expected ConnectHTTP without client cert to fail")
	}
}

func TestDiscoverToolsHTTPOverMTLS(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "mtls-discover", Version: "v0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "echo",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args httpEchoArgs) (*mcp.CallToolResult, httpEchoOutput, error) {
		return nil, httpEchoOutput(args), nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	httpServer, mtlsClient := startRequireClientCertServer(t, handler)
	t.Cleanup(httpServer.Close)

	tools, err := mcpbridge.DiscoverToolsHTTP(context.Background(), httpServer.URL, nil, "mtls-mcp", mtlsClient)
	if err != nil {
		t.Fatalf("DiscoverToolsHTTP: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools=%+v", tools)
	}
}

func startRequireClientCertServer(t *testing.T, handler http.Handler) (*httptest.Server, *http.Client) {
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
