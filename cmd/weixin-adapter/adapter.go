package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
)

// Adapter holds the weixin-adapter process dependencies and runtime state.
type Adapter struct {
	ilink weixinlink.ILink // iLink client (real or fake in tests)

	baizeInboundURL string
	secret          string
	credsDir        string

	emptyWait  time.Duration // backoff between empty poll batches (tests override)
	httpClient *http.Client  // client used for signed forwards to baize

	// requestShutdown, when set, asks the process to exit after handling
	// /admin/shutdown (main wires it to a clean process exit; tests leave it
	// nil so the endpoint just acks).
	requestShutdown func()

	mu      sync.Mutex
	account string
	token   string
	polling bool
	// pollToken is the credential the running poll loop was started with. The
	// loop captures its token once at launch; if a re-scan replaces a.token
	// while polling, startPolling must detect the change and restart the loop,
	// otherwise the stale token makes GetUpdates fail forever (silently).
	pollToken string
	cancel    func()        // stops the current poll loop
	pollDone  chan struct{} // closed when the current poll loop exits (nil when idle)
}

func (a *Adapter) handleHealthz(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	polling := a.polling
	hasCreds := a.token != "" && a.account != ""
	a.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":          "ok",
		"polling":         polling,
		"has_credentials": hasCreds,
	})
}

// routes returns the adapter's HTTP mux. Every /admin endpoint is mounted
// behind adminGuard (HMAC verification before any state change or body
// trust); /outbound verifies the same HMAC itself and sends to iLink.
// /healthz is always present.
func (a *Adapter) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleHealthz)
	mux.HandleFunc("POST /admin/login/start", a.adminGuard(a.handleAdminLoginStart))
	mux.HandleFunc("GET /admin/login/status", a.adminGuard(a.handleAdminLoginStatus))
	mux.HandleFunc("POST /admin/logout", a.adminGuard(a.handleAdminLogout))
	mux.HandleFunc("GET /admin/status", a.adminGuard(a.handleAdminStatus))
	mux.HandleFunc("POST /admin/start", a.adminGuard(a.handleAdminStart))
	mux.HandleFunc("POST /admin/stop", a.adminGuard(a.handleAdminStop))
	mux.HandleFunc("POST /admin/shutdown", a.adminGuard(a.handleAdminShutdown))
	mux.HandleFunc("POST /outbound", a.handleOutbound)
	return mux
}

// setCredentials stores the iLink account/token.
func (a *Adapter) setCredentials(account, token string) {
	a.mu.Lock()
	a.account = account
	a.token = token
	a.mu.Unlock()
}

// stopPolling cancels the running poll loop if any and waits for the goroutine
// to exit. It is a safe no-op when nothing is polling. The WaitGroup is
// awaited OUTSIDE the lock (the poll loop takes a.mu via currentAccount),
// so cancelling under the lock and waiting afterward cannot deadlock.
// stopPolling cancels the running poll loop if any and waits for the goroutine
// to exit. It is a safe no-op when nothing is polling. The loop is joined via
// its done channel OUTSIDE the lock (the poll loop takes a.mu via
// currentAccount/currentToken), so cancelling under the lock and waiting
// afterward cannot deadlock.
func (a *Adapter) stopPolling() {
	a.mu.Lock()
	cancel := a.cancel
	done := a.pollDone
	a.cancel = nil
	a.pollDone = nil
	a.polling = false
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}
