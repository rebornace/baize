package channel

import "net/http"

// RouteRegistrar is implemented by the HTTP server (api.Server). Channels
// that expose their own HTTP endpoints (e.g. the webhook channel's inbound
// webhook) register them during Bootstrap instead of editing core routing.
// It depends only on net/http so the channel package never imports api.
type RouteRegistrar interface {
	RegisterRoute(pattern string, h http.Handler)
}
