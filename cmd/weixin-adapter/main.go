// Command weixin-adapter is the out-of-process WeChat (iLink) adapter for
// baize. It logs into iLink via QR (driven by baize's admin proxy), long-polls
// inbound messages and signs them to baize's webhook, and serves /outbound for
// baize to send text/images/files back to WeChat.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
)

func main() {
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
	client := weixinlink.NewClient(cfg.ILinkBaseURL, nil)
	a := &Adapter{
		ilink:           client,
		baizeInboundURL: cfg.BaizeInboundURL,
		secret:          cfg.Secret,
		credsDir:        cfg.CredsDir,
	}
	// /admin/shutdown (HMAC-guarded, from baize) exits the process cleanly so
	// baize can terminate an adapter it adopted but did not spawn (orphan).
	shutdownCh := make(chan struct{})
	a.requestShutdown = func() { close(shutdownCh) }
	// Load any persisted credentials at startup (baize may call /admin/start).
	if acct, tok, lerr := weixinlink.LoadCreds(cfg.CredsDir); lerr == nil {
		a.setCredentials(acct, tok)
	}

	srv := &http.Server{Addr: cfg.Addr, Handler: a.routes()}
	go func() {
		log.Printf("weixin-adapter listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case <-stop:
	case <-shutdownCh:
		log.Printf("weixin-adapter shutting down via /admin/shutdown")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a.stopPolling()
	_ = srv.Shutdown(ctx)
}
