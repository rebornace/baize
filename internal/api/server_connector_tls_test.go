package api_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func writeTestCAPEM(t *testing.T) string {
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
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	p := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(p, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// 第二次 PUT 省略 tls 时必须保留已持久化的 TLS 路径（与 auth.capture 同策略）。
func TestPutConnectorOmitsTLSPreservesExisting(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	caPath := writeTestCAPEM(t)
	spec := writeLoginGetMeAPISpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	h := srv.Handler()

	putConnectorJSON(t, h, "app1", map[string]any{
		"type":     "openapi",
		"spec":     spec,
		"base_url": upstream.URL,
	})

	conn, err := st.GetConnector("app1")
	if err != nil {
		t.Fatalf("get connector: %v", err)
	}
	conn.TLS = store.ConnectorTLS{CAFile: caPath}
	st.UpsertConnector(conn)

	body := map[string]any{
		"type":     "openapi",
		"spec":     spec,
		"base_url": upstream.URL,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"tls"`) {
		t.Fatalf("test body must omit tls key, got %s", raw)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/v0/connectors/app1", strings.NewReader(string(raw))))
	if rr.Code != http.StatusOK {
		t.Fatalf("second PUT status=%d body=%s", rr.Code, rr.Body.String())
	}

	got, err := st.GetConnector("app1")
	if err != nil {
		t.Fatalf("get after second PUT: %v", err)
	}
	if got.TLS.CAFile != caPath {
		t.Fatalf("TLS.CAFile=%q want %q", got.TLS.CAFile, caPath)
	}

	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, httptest.NewRequest(http.MethodGet, "/v0/connectors/app1", nil))
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	var getPayload struct {
		TLS store.ConnectorTLS `json:"tls"`
	}
	if err := json.Unmarshal(getRR.Body.Bytes(), &getPayload); err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(getPayload.TLS.CAFile) != filepath.Clean(caPath) {
		t.Fatalf("GET tls.ca_file=%q want %q", getPayload.TLS.CAFile, caPath)
	}
}

func TestListConnectorsIncludesTLSWhenConfigured(t *testing.T) {
	caPath := writeTestCAPEM(t)
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID:      "c1",
		Type:    "openapi",
		BaseURL: "http://example.com",
		TLS:     store.ConnectorTLS{CAFile: caPath},
	})
	h := apiNewServer(st, tool.NewRegistry()).Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v0/connectors", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("LIST status=%d body=%s", rr.Code, rr.Body.String())
	}
	var listPayload struct {
		Connectors []struct {
			TLS store.ConnectorTLS `json:"tls"`
		} `json:"connectors"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listPayload); err != nil {
		t.Fatal(err)
	}
	if len(listPayload.Connectors) != 1 {
		t.Fatalf("connectors=%d want 1", len(listPayload.Connectors))
	}
	if filepath.Clean(listPayload.Connectors[0].TLS.CAFile) != filepath.Clean(caPath) {
		t.Fatalf("LIST tls.ca_file=%q want %q", listPayload.Connectors[0].TLS.CAFile, caPath)
	}
}
