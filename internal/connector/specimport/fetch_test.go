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
	specSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestFetchSpecFromURLAllowsLoopbackHost(t *testing.T) {
	// validateHTTPURL must accept localhost / loopback before the HTTP round-trip.
	// Dial may fail; we only assert the SSRF guard does not return ErrFetchBlocked.
	_, err := specimport.FetchSpecFromURL("http://127.0.0.1:9/swagger.json")
	if errors.Is(err, specimport.ErrFetchBlocked) {
		t.Fatalf("loopback must be allowed for intranet specs, got %v", err)
	}
	_, err = specimport.FetchSpecFromURL("http://localhost:9/swagger.json")
	if errors.Is(err, specimport.ErrFetchBlocked) {
		t.Fatalf("localhost must be allowed for intranet specs, got %v", err)
	}
}

func TestFetchSpecFromURLBlocksLinkLocalIP(t *testing.T) {
	_, err := specimport.FetchSpecFromURL("http://169.254.169.254/latest/meta-data/")
	if err == nil {
		t.Fatal("expected error for link-local metadata address")
	}
	if !errors.Is(err, specimport.ErrFetchBlocked) {
		t.Fatalf("err=%v want ErrFetchBlocked", err)
	}
}

func TestFetchSpecFromURLDrfYasgFormatOpenAPI(t *testing.T) {
	spec := readFixture(t, "swagger2-min.json")
	initJS := `"use strict";
var currentPath = window.location.protocol + "//" + window.location.host + window.location.pathname;
var defaultSpecUrl = currentPath + '?format=openapi';
var swaggerUiConfig = { url: defaultSpecUrl, dom_id: '#swagger-ui' };
`
	html := `<!DOCTYPE html><html><body>
<div id="swagger-ui"></div>
<script src="/static/drf-yasg/swagger-ui-init.js"></script>
</body></html>`

	mux := http.NewServeMux()
	mux.HandleFunc("/static/drf-yasg/swagger-ui-init.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(initJS))
	})
	mux.HandleFunc("/swagger/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") == "openapi" {
			w.Header().Set("Content-Type", "application/openapi+json")
			_, _ = w.Write([]byte(spec))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(html))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body, err := specimport.FetchSpecFromURL(srv.URL + "/swagger/")
	if err != nil {
		t.Fatal(err)
	}
	if specimport.DetectFormat(body) != specimport.FormatSwagger2 {
		t.Fatalf("detect=%q want swagger2", specimport.DetectFormat(body))
	}
}

func TestFetchSpecFromURLNestJSInitEmbedded(t *testing.T) {
	specimport.SetAllowPrivateFetchHosts(true)
	defer specimport.SetAllowPrivateFetchHosts(false)

	embedded := string(readFixture(t, "openapi3-min.json"))
	initJS := `window.onload = function() {
  let options = {
  "swaggerDoc": ` + embedded + `,
  "customOptions": {}
};
  SwaggerUIBundle({ spec: options.swaggerDoc });
}`
	initSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(initJS))
	}))
	defer initSrv.Close()

	html := `<!DOCTYPE html><html><body>
<script src="./swagger-ui-init.js"></script>
</body></html>`
	uiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "swagger-ui-init.js") {
			http.Redirect(w, r, initSrv.URL+"/swagger-ui-init.js", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(html))
	}))
	defer uiSrv.Close()

	body, err := specimport.FetchSpecFromURL(uiSrv.URL + "/api")
	if err != nil {
		t.Fatal(err)
	}
	if specimport.DetectFormat(body) != specimport.FormatOpenAPI3 {
		t.Fatalf("detect=%q want openapi3", specimport.DetectFormat(body))
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
