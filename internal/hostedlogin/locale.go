package hostedlogin

import (
	"net/http"
	"strings"
)

// NormalizeLocale maps lang / Accept-Language tags to zh-CN or en.
// Empty or unknown values default to zh-CN (product default).
func NormalizeLocale(locale string) string {
	l := strings.ToLower(strings.TrimSpace(locale))
	l = strings.ReplaceAll(l, "_", "-")
	switch {
	case l == "en" || strings.HasPrefix(l, "en-"):
		return "en"
	case l == "" || l == "zh" || l == "zh-cn" || l == "zh-hans" || strings.HasPrefix(l, "zh-"):
		return "zh-CN"
	default:
		return "zh-CN"
	}
}

// LocaleFromRequest prefers ?lang=, then Accept-Language, else zh-CN.
func LocaleFromRequest(r *http.Request) string {
	if r == nil {
		return "zh-CN"
	}
	if q := strings.TrimSpace(r.URL.Query().Get("lang")); q != "" {
		return NormalizeLocale(q)
	}
	if formLang := strings.TrimSpace(r.FormValue("lang")); formLang != "" {
		return NormalizeLocale(formLang)
	}
	accept := strings.TrimSpace(r.Header.Get("Accept-Language"))
	if accept == "" {
		return "zh-CN"
	}
	// Take the first tag (ignore q-values for simplicity).
	first := accept
	if i := strings.IndexByte(accept, ','); i >= 0 {
		first = accept[:i]
	}
	if i := strings.IndexByte(first, ';'); i >= 0 {
		first = first[:i]
	}
	return NormalizeLocale(first)
}

func isEnglish(locale string) bool {
	return NormalizeLocale(locale) == "en"
}
