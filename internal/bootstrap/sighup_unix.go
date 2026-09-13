//go:build unix

package bootstrap

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// watchConfigReloadSignal listens for SIGHUP and reloads layered YAML into
// runtimecfg without swapping Store. Cancel ctx to stop.
func watchConfigReloadSignal(ctx context.Context, reload func() error) {
	if reload == nil {
		return
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	go func() {
		defer signal.Stop(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ch:
				if err := reload(); err != nil {
					log.Printf("SIGHUP config reload failed: %v", err)
					continue
				}
				log.Printf("SIGHUP: layered config reloaded")
			}
		}
	}()
}
