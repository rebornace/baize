package authcred

import "net/http"

func PickHeaders(h http.Header, whitelist []string) map[string]string {
	if whitelist == nil {
		whitelist = []string{"Authorization"}
	}
	if len(whitelist) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string)
	for _, name := range whitelist {
		val := h.Get(name)
		if val == "" {
			continue
		}
		out[name] = val
	}
	return out
}
