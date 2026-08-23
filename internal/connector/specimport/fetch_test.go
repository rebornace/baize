package specimport_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/connector/specimport"
)

func TestFetchSpecFromURLDirectJSON(t *testing.T) {
	specimport.SetAllowPrivateFetchHosts(true)
	defer specimport.SetAllowPrivateFetchHosts(false)
	spec := readFixture(t, "openapi3-min.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(spec))
	}))
	defer srv.Close()

	body, err := specimport.FetchSpecFromURL(srv.URL + "/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	if specimport.DetectFormat(body) != specimport.FormatOpenAPI3 {
		t.Fatalf("detect=%q want openapi3", specimport.DetectFormat(body))
	}
}

func TestFetchSpecFromURLSwaggerUIHTML(t *testing.T) {
	specimport.SetAllowPrivateFetchHosts(true)
	defer specimport.SetAllowPrivateFetchHosts(false)
	spec := readFixture(t, "openapi3-min.json")
	var specSrv *httptest.Server
	specSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(spec))
	}))
	defer specSrv.Close()

	html := `<!DOCTYPE html><html><body><script>
SwaggerUIBundle({ url: "` + specSrv.URL + "/v1/openapi.json" + `" });
</script></body></html>`
	uiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".json") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(spec))
			return
		}
		_, _ = w.Write([]byte(html))
	}))
	defer uiSrv.Close()

	body, err := specimport.FetchSpecFromURL(uiSrv.URL + "/swagger-ui/")
	if err != nil {
		t.Fatal(err)
	}
	if specimport.DetectFormat(body) != specimport.FormatOpenAPI3 {
		t.Fatalf("detect=%q want openapi3", specimport.DetectFormat(body))
	}
}

func TestFetchSpecFromURLBlocksPrivateHost(t *testing.T) {
	_, err := specimport.FetchSpecFromURL("http://127.0.0.1/swagger.json")
	if err == nil {
		t.Fatal("expected error for loopback")
	}
	if !errors.Is(err, specimport.ErrFetchBlocked) {
		t.Fatalf("err=%v want ErrFetchBlocked", err)
	}
}

func TestFetchSpecFromURLRejectsNonHTTP(t *testing.T) {
	_, err := specimport.FetchSpecFromURL("file:///etc/passwd")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, specimport.ErrInvalidSpecURL) {
		t.Fatalf("err=%v want ErrInvalidSpecURL", err)
	}
}
