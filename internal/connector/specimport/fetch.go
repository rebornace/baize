package specimport

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const maxFetchSize = 4 << 20

var (
	// ErrInvalidSpecURL is returned when spec_url is not a valid http(s) URL.
	ErrInvalidSpecURL = errors.New("invalid_spec_url")
	// ErrFetchBlocked is returned when fetching link-local / unspecified targets is blocked.
	ErrFetchBlocked = errors.New("spec_fetch_blocked")
	// ErrFetchFailed is returned when HTTP fetch fails.
	ErrFetchFailed = errors.New("spec_fetch_failed")
)

// SetAllowPrivateFetchHosts is retained for older tests; loopback and private
// hosts are always allowed so intranet OpenAPI sources (e.g. localhost) work.
func SetAllowPrivateFetchHosts(_ bool) {}

var (
	reSwaggerConfigURL = regexp.MustCompile(`configUrl\s*:\s*["']([^"']+)["']`)
	reSwaggerUrlsArray = regexp.MustCompile(`urls\s*:\s*\[\s*\{[^}]*?url\s*:\s*["']([^"']+)["']`)
	reSwaggerURL       = regexp.MustCompile(`url\s*:\s*["']([^"']+)["']`)
)

var defaultFetchClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if _, err := validateHTTPURL(req.URL.String()); err != nil {
			return err
		}
		return nil
	},
}

// FetchSpecFromURL downloads an OpenAPI/Swagger/Postman document from an http(s) URL.
// Loopback and private (intranet) hosts are allowed; link-local / unspecified IPs are not.
// HTML Swagger UI pages are resolved to their embedded spec URL when possible.
func FetchSpecFromURL(rawURL string) ([]byte, error) {
	u, err := validateHTTPURL(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, err
	}
	content, err := fetchURL(u)
	if err != nil {
		return nil, err
	}
	if looksLikeHTML(content) {
		content, err = resolveHTMLSwaggerPage(string(content), u)
		if err != nil {
			return nil, err
		}
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("%w: empty response", ErrInvalidSpec)
	}
	if len(content) > maxFetchSize {
		return nil, fmt.Errorf("fetched spec exceeds 4 MiB limit")
	}
	return content, nil
}

func fetchURL(u *url.URL) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetchFailed, err)
	}
	// Prefer JSON only: drf-yasg content-negotiates YAML when application/yaml is
	// accepted, and YAML→openapi2 decode is fragile even after JSON re-encode.
	req.Header.Set("Accept", "application/openapi+json, application/json;q=0.9, */*;q=0.1")
	req.Header.Set("User-Agent", "baize-spec-fetch/1.0")

	res, err := defaultFetchClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetchFailed, err)
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTP %d", ErrFetchFailed, res.StatusCode)
	}

	limited := io.LimitReader(res.Body, maxFetchSize+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetchFailed, err)
	}
	if len(body) > maxFetchSize {
		return nil, fmt.Errorf("fetched spec exceeds 4 MiB limit")
	}
	return body, nil
}

func validateHTTPURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("%w: empty url", ErrInvalidSpecURL)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSpecURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%w: only http and https are allowed", ErrInvalidSpecURL)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("%w: missing host", ErrInvalidSpecURL)
	}
	if u.User != nil {
		return nil, fmt.Errorf("%w: userinfo in url is not allowed", ErrInvalidSpecURL)
	}
	host := strings.ToLower(u.Hostname())
	if ip := net.ParseIP(host); ip != nil {
		if isDangerousFetchIP(ip) {
			return nil, fmt.Errorf("%w: link-local or unspecified address is not allowed", ErrFetchBlocked)
		}
	}
	return u, nil
}

// isDangerousFetchIP blocks cloud-metadata-style literals while allowing
// loopback and RFC1918 private addresses used by intranet APIs.
func isDangerousFetchIP(ip net.IP) bool {
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 0 {
		return true
	}
	return false
}

func looksLikeHTML(content []byte) bool {
	trim := strings.TrimSpace(string(content))
	if len(trim) == 0 {
		return false
	}
	lower := strings.ToLower(trim)
	return strings.HasPrefix(lower, "<!doctype html") ||
		strings.HasPrefix(lower, "<html") ||
		(strings.Contains(lower, "<html") && strings.Contains(lower, "</html>"))
}

func extractSwaggerUISpecURL(html string, pageURL *url.URL) (string, error) {
	if m := reSwaggerConfigURL.FindStringSubmatch(html); len(m) > 1 {
		return resolveSpecURL(pageURL, m[1])
	}
	if m := reSwaggerUrlsArray.FindStringSubmatch(html); len(m) > 1 {
		return resolveSpecURL(pageURL, m[1])
	}
	if m := reSwaggerURL.FindStringSubmatch(html); len(m) > 1 {
		return resolveSpecURL(pageURL, m[1])
	}
	return "", fmt.Errorf("could not find OpenAPI/Swagger spec URL in HTML; paste the direct .json/.yaml link instead")
}

func resolveSpecURL(base *url.URL, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty spec url in page")
	}
	refURL, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	resolved := base.ResolveReference(refURL)
	return resolved.String(), nil
}
