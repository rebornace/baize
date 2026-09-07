package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
	"github.com/rebornace/baize/internal/webhooksig"
)

const (
	adminBodyLimit = 1 << 16
	adminSkew      = 300 * time.Second
)

var errLoginRequired = errors.New("login required")

// adminGuard verifies the baize->adapter HMAC signature BEFORE dispatching an
// admin endpoint or trusting any body content. GET requests carry an empty
// body (baize signs the empty body); POST bodies are read here for
// verification. All /admin endpoints are mounted behind this guard and a
// failed verification always yields 401 without touching adapter state.
func (a *Adapter) adminGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, adminBodyLimit))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
			return
		}
		if err := webhooksig.Verify(a.secret, r.Header.Get("X-Baize-Channel-Timestamp"), body,
			r.Header.Get("X-Baize-Channel-Signature"), time.Now(), adminSkew); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
			return
		}
		next(w, r)
	}
}

func (a *Adapter) handleAdminLoginStart(w http.ResponseWriter, r *http.Request) {
	ticket, qrURL, err := a.ilink.GetQR(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ticket": ticket, "qr_url": qrURL})
}

func (a *Adapter) handleAdminLoginStatus(w http.ResponseWriter, r *http.Request) {
	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if ticket == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ticket required"})
		return
	}
	status, accountID, token, err := a.ilink.PollLogin(r.Context(), ticket)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if status == weixinlink.LoginStatusSuccess {
		// Persist FIRST so a SaveCreds failure leaves no half-applied
		// in-memory state; only then commit to memory and start polling.
		// If polling fails, creds are already on disk and baize can recover
		// by calling /admin/start after a restart.
		if err := weixinlink.SaveCreds(a.credsDir, accountID, token); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save creds: " + err.Error()})
			return
		}
		a.setCredentials(accountID, token)
		if err := a.startPolling(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "start polling: " + err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (a *Adapter) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	// Stop polling and clear in-memory state regardless of disk state: the
	// running process is logged out either way. A ClearCreds failure (e.g.
	// Windows file lock) still returns 500 so the caller knows the on-disk
	// credential may remain and needs manual intervention; swallowing it
	// would let LoadCreds silently re-login after a restart.
	a.stopPolling()
	a.mu.Lock()
	a.account = ""
	a.token = ""
	a.mu.Unlock()
	if err := weixinlink.ClearCreds(a.credsDir); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "clear creds: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (a *Adapter) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	st := map[string]any{
		"has_credentials": a.token != "" && a.account != "",
		"polling":         a.polling,
		"account_id":      a.account,
	}
	a.mu.Unlock()
	writeJSON(w, http.StatusOK, st)
}

func (a *Adapter) handleAdminStart(w http.ResponseWriter, r *http.Request) {
	if err := a.startPolling(); err != nil {
		if errors.Is(err, errLoginRequired) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "login required"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (a *Adapter) handleAdminStop(w http.ResponseWriter, r *http.Request) {
	a.stopPolling()
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// handleAdminShutdown stops polling, acks, then asks the process to exit. The
// exit runs in a goroutine AFTER the response is flushed so baize receives the
// 200 before the process goes away. Used by baize to terminate an adopted
// orphan (which baize did not spawn and therefore cannot kill by PID).
func (a *Adapter) handleAdminShutdown(w http.ResponseWriter, r *http.Request) {
	a.stopPolling()
	writeJSON(w, http.StatusOK, map[string]string{"status": "shutting_down"})
	if a.requestShutdown != nil {
		go func() {
			time.Sleep(200 * time.Millisecond)
			a.requestShutdown()
		}()
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
