package specimport

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	reScriptSrc        = regexp.MustCompile(`<script[^>]+src=["']([^"']+)["']`)
	reInitSwaggerURL   = regexp.MustCompile(`"swaggerUrl"\s*:\s*"([^"\\]*(?:\\.[^"\\]*)*)"`)
	reInitSwaggerURL2  = regexp.MustCompile(`swaggerUrl\s*:\s*["']([^"']+)["']`)
)

func resolveHTMLSwaggerPage(html string, pageURL *url.URL) ([]byte, error) {
	for _, scriptURL := range collectSwaggerInitScriptURLs(html, pageURL) {
		doc, err := fetchSpecFromInitScript(scriptURL)
		if err == nil {
			return doc, nil
		}
	}
	specURL, inlineErr := extractSwaggerUISpecURL(html, pageURL)
	if inlineErr == nil {
		u2, err := validatePublicHTTPURL(specURL)
		if err != nil {
			return nil, err
		}
		body, err := fetchURL(u2)
		if err != nil {
			return nil, err
		}
		if looksLikeHTML(body) {
			return nil, fmt.Errorf("%w: resolved spec URL still returns HTML", ErrInvalidSpec)
		}
		return body, nil
	}
	return nil, fmt.Errorf("%w: %v", ErrInvalidSpec, inlineErr)
}

func collectSwaggerInitScriptURLs(html string, pageURL *url.URL) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(raw string) {
		resolved, err := resolveSpecURL(pageURL, raw)
		if err != nil {
			return
		}
		if seen[resolved] {
			return
		}
		seen[resolved] = true
		out = append(out, resolved)
	}
	for _, m := range reScriptSrc.FindAllStringSubmatch(html, -1) {
		if len(m) < 2 {
			continue
		}
		src := m[1]
		if strings.Contains(src, "swagger-ui-init.js") {
			add(src)
		}
	}
	base := pageURLWithTrailingSlash(pageURL)
	for _, name := range []string{"swagger-ui-init.js", "api/swagger-ui-init.js"} {
		add(base.ResolveReference(mustParseRef(name)).String())
	}
	return out
}

func pageURLWithTrailingSlash(u *url.URL) *url.URL {
	clone := *u
	if !strings.HasSuffix(clone.Path, "/") {
		clone.Path += "/"
	}
	return &clone
}

func mustParseRef(ref string) *url.URL {
	u, err := url.Parse(ref)
	if err != nil {
		panic(err)
	}
	return u
}

func fetchSpecFromInitScript(scriptURL string) ([]byte, error) {
	u, err := validatePublicHTTPURL(scriptURL)
	if err != nil {
		return nil, err
	}
	body, err := fetchURL(u)
	if err != nil {
		return nil, err
	}
	if doc, err := extractEmbeddedSwaggerDoc(body); err == nil {
		return doc, nil
	}
	if swaggerURL, err := extractSwaggerURLFromInitJS(string(body)); err == nil {
		u2, err := validatePublicHTTPURL(swaggerURL)
		if err != nil {
			return nil, err
		}
		return fetchURL(u2)
	}
	return nil, fmt.Errorf("init script does not contain swaggerDoc or swaggerUrl")
}

func extractSwaggerURLFromInitJS(js string) (string, error) {
	if m := reInitSwaggerURL.FindStringSubmatch(js); len(m) > 1 {
		return unescapeJSString(m[1]), nil
	}
	if m := reInitSwaggerURL2.FindStringSubmatch(js); len(m) > 1 {
		return m[1], nil
	}
	return "", fmt.Errorf("swaggerUrl not found")
}

func unescapeJSString(s string) string {
	s = strings.ReplaceAll(s, "\\\"", "\"")
	s = strings.ReplaceAll(s, "\\/", "/")
	return s
}

func extractEmbeddedSwaggerDoc(content []byte) ([]byte, error) {
	s := string(content)
	for _, marker := range []string{"\"swaggerDoc\"", "swaggerDoc"} {
		searchFrom := 0
		for {
			pos := strings.Index(s[searchFrom:], marker)
			if pos < 0 {
				break
			}
			pos += searchFrom
			rest := strings.TrimSpace(s[pos+len(marker):])
			if strings.HasPrefix(rest, ":") {
				rest = strings.TrimSpace(rest[1:])
			}
			if len(rest) == 0 || rest[0] != '{' {
				searchFrom = pos + len(marker)
				continue
			}
			jsonObj, ok := extractBalancedJSONObject(rest)
			if !ok {
				searchFrom = pos + len(marker)
				continue
			}
			if DetectFormat(jsonObj) != "" {
				return jsonObj, nil
			}
			searchFrom = pos + len(marker)
		}
	}
	return nil, fmt.Errorf("embedded swaggerDoc not found or not OpenAPI/Swagger")
}

func extractBalancedJSONObject(s string) ([]byte, bool) {
	if len(s) == 0 || s[0] != '{' {
		return nil, false
	}
	depth := 0
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return []byte(s[:i+1]), true
			}
		}
	}
	return nil, false
}
