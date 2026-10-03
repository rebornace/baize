package webhook

import (
	"context"
	"fmt"
	"time"

	"github.com/rebornace/baize/internal/channel"
)

// Process-level control for autostart (baize-owned) adapter instances. These
// drive the OS process lifecycle, distinct from the polling start/stop tied to
// the enabled setting. Independently deployed adapters (no supervisor) are not
// owned by baize: process calls degrade to polling reconcile / no-op.

var _ channel.ProcessController = (*Channel)(nil)

// setManualStop records (under settingsMu, which also guards Status) whether
// the operator intentionally stopped the adapter process.
func (c *Channel) setManualStop(stopped bool) {
	c.settingsMu.Lock()
	c.manualStop = stopped
	c.settingsMu.Unlock()
}

// StartProcess launches (or adopts) the adapter process and resumes polling
// when the channel is enabled.
func (c *Channel) StartProcess(ctx context.Context) error {
	c.procMu.Lock()
	defer c.procMu.Unlock()
	if c.sup == nil {
		// Adapter deployed independently: baize cannot launch its process,
		// just resume polling when the channel is enabled.
		c.setManualStop(false)
		c.reconcileEnabled(c.GetSettings().Enabled)
		return nil
	}
	if err := c.sup.start(ctx); err != nil {
		return fmt.Errorf("webhook %s: start adapter: %w", c.cfg.Name, err)
	}
	c.setManualStop(false)
	c.reconcileEnabled(c.GetSettings().Enabled)
	return nil
}

// StopProcess stops polling and terminates the baize-owned adapter process.
// An adopted orphan (spawned by a previous baize run, no PID handle here) is
// asked to exit via its HMAC /admin/shutdown endpoint.
func (c *Channel) StopProcess(ctx context.Context) error {
	c.procMu.Lock()
	defer c.procMu.Unlock()
	if c.admin != nil {
		_ = c.admin.Stop(ctx) // stop polling first (best-effort)
	}
	if c.sup == nil {
		c.setManualStop(true)
		return nil
	}
	if c.sup.ownsProcess() {
		err := c.sup.terminate(ctx)
		c.sup.waitUntilDown(ctx, 8*time.Second)
		c.setManualStop(true)
		return err
	}
	// Adopted orphan: baize has no handle; ask the process to exit itself.
	if c.admin != nil && c.sup.isAdopted() {
		if err := c.admin.Shutdown(ctx); err != nil {
			return fmt.Errorf("webhook %s: shutdown adopted adapter: %w", c.cfg.Name, err)
		}
		c.sup.waitUntilDown(ctx, 8*time.Second)
	}
	c.sup.resetForRespawn()
	c.setManualStop(true)
	return nil
}

// RestartProcess terminates the running adapter process (owned kill, or
// /admin/shutdown for an adopted orphan), waits for the port to clear, then
// launches a fresh process and resumes polling when enabled. Used to recover a
// wedged adapter without restarting baize.
func (c *Channel) RestartProcess(ctx context.Context) error {
	c.procMu.Lock()
	defer c.procMu.Unlock()
	if c.sup == nil {
		// Independent adapter: baize cannot restart its process; just
		// reconcile polling.
		c.setManualStop(false)
		c.reconcileEnabled(c.GetSettings().Enabled)
		return nil
	}
	if c.admin != nil {
		_ = c.admin.Stop(ctx) // stop polling on the old process first
	}
	switch {
	case c.sup.ownsProcess():
		_ = c.sup.terminate(ctx)
	case c.admin != nil && c.sup.isAdopted():
		_ = c.admin.Shutdown(ctx)
	}
	c.sup.waitUntilDown(ctx, 10*time.Second)
	c.sup.resetForRespawn()
	// Settle briefly so the listen port is released before re-spawn.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(400 * time.Millisecond):
	}
	if err := c.sup.start(ctx); err != nil {
		return fmt.Errorf("webhook %s: restart adapter: %w", c.cfg.Name, err)
	}
	c.setManualStop(false)
	c.reconcileEnabled(c.GetSettings().Enabled)
	return nil
}
