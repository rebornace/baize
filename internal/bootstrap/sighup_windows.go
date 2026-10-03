//go:build windows

package bootstrap

import "context"

// watchConfigReloadSignal is a no-op on Windows (no SIGHUP). Use POST /v0/settings/reload.
func watchConfigReloadSignal(ctx context.Context, reload func() error) {
	_ = ctx
	_ = reload
}
