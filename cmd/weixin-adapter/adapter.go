package main

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
)

// Adapter holds the weixin-adapter process dependencies and runtime state.
type Adapter struct {
	ilink weixinlink.ILink // iLink client (real or fake in tests)

	baizeInboundURL string
	secret          string
	credsDir        string

	mu      sync.Mutex
	account string
	token   string
	polling bool
	cancel  func() // stops the poll loop
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

// routes returns the adapter's HTTP mux. Endpoints are added in later tasks;
// /healthz is always present.
func (a *Adapter) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleHealthz)
	return mux
}

// setCredentials stores the iLink account/token. Task 6 extends this with
// persistence/refresh behavior; the skeleton only needs thread-safe storage.
func (a *Adapter) setCredentials(account, token string) {
	a.mu.Lock()
	a.account = account
	a.token = token
	a.mu.Unlock()
}

// stopPolling cancels the running poll loop if any. Task 6 owns the full
// polling lifecycle; the skeleton needs a safe no-op when nothing is polling.
func (a *Adapter) stopPolling() {
	a.mu.Lock()
	cancel := a.cancel
	a.cancel = nil
	a.polling = false
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
