package api

import (
	"context"
	"net/http"

	"github.com/rebornace/baize/internal/channel"
)

// RegisterRoute mounts an additional HTTP route on the core mux. Used by
// channels (via channel.RouteRegistrar) to expose their own endpoints without
// editing core routing. Must be called before the server starts serving.
func (s *Server) RegisterRoute(pattern string, h http.Handler) {
	s.mux.Handle(pattern, h)
}

// ChannelHandle is the api layer's per-channel runtime handle. The generic
// management plane (/v0/settings/channels/{name}/...) type-asserts h.Channel
// to channel.ManagedChannel; channels without that interface are not managed.
type ChannelHandle struct {
	Name     string
	Channel  channel.Channel
	Runtime  *channel.Runtime
	CredsDir string
	RunCtx   context.Context
}

// RegisterChannel registers a channel handle by name. It is safe for concurrent
// use; the map is mutex-protected. Nil handles and handles without a name are
// ignored. Only the short map insert happens under the lock — channel network
// operations never run while the lock is held.
func (s *Server) RegisterChannel(h *ChannelHandle) {
	if h == nil || h.Name == "" {
		return
	}
	s.channelsMu.Lock()
	if s.channels == nil {
		s.channels = map[string]*ChannelHandle{}
	}
	s.channels[h.Name] = h
	s.channelsMu.Unlock()
}

// Channel returns the handle for a registered channel name.
func (s *Server) Channel(name string) (*ChannelHandle, bool) {
	s.channelsMu.RLock()
	defer s.channelsMu.RUnlock()
	h, ok := s.channels[name]
	return h, ok
}
