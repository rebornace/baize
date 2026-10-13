package api

import (
	"net"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/hostedlogin"
)

func (s *Server) handleHostedLoginGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.HostedSessions == nil {
		http.Error(w, "hosted login unavailable", http.StatusServiceUnavailable)
		return
	}
	if !s.hostedLoginLimiter().AllowHosted(clientIP(r), id) {
		s.hostedAudit().Record(hostedlogin.AuditEvent{
			Kind: hostedlogin.AuditRateLimited, SessionID: id, ClientIP: clientIP(r),
		})
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}
	sess, ok := s.HostedSessions.Get(id)
	if !ok {
		http.Error(w, "session not found or expired", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(hostedlogin.RenderPage(sess, "")))
}

func (s *Server) handleHostedLoginPost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.HostedSessions == nil || s.Store == nil {
		http.Error(w, "hosted login unavailable", http.StatusServiceUnavailable)
		return
	}
	ip := clientIP(r)
	if !s.hostedLoginLimiter().AllowHosted(ip, id) {
		s.hostedAudit().Record(hostedlogin.AuditEvent{
			Kind: hostedlogin.AuditRateLimited, SessionID: id, ClientIP: ip,
		})
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	csrf := r.FormValue("csrf")
	form := map[string]string{}
	for k := range r.PostForm {
		if k == "csrf" {
			continue
		}
		form[k] = r.PostForm.Get(k)
	}
	err := hostedlogin.Submit(r.Context(), hostedlogin.SubmitDeps{
		Sessions:   s.HostedSessions,
		Store:      s.Store,
		Registry:   s.Registry,
		Identities: s.Identities,
		Audit:      s.hostedAudit(),
	}, id, csrf, form)
	if err != nil {
		sess, ok := s.HostedSessions.Get(id)
		if !ok {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusBadRequest)
		msg := err.Error()
		if i := strings.Index(msg, "hostedlogin: "); i >= 0 {
			msg = strings.TrimSpace(msg[i+len("hostedlogin: "):])
		}
		_, _ = w.Write([]byte(hostedlogin.RenderPage(sess, msg)))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(hostedlogin.RenderSuccessPage()))
}

func clientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		if i := strings.Index(xff, ","); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return xff
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}
