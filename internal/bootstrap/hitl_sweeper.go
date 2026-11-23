package bootstrap

import (
	"context"
	"log"
	"time"

	"github.com/rebornace/baize/internal/run"
)

// hitlSweepInterval bounds how long an overdue approval can hold a
// conversation/schedule after its TTL passes.
const hitlSweepInterval = 30 * time.Second

func runHITLSweeper(ctx context.Context, eng *run.Engine) {
	if eng == nil {
		return
	}
	ticker := time.NewTicker(hitlSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			settled, err := eng.SweepExpiredHITL(now)
			if err != nil {
				log.Printf("hitl sweep: %v", err)
				continue
			}
			if settled > 0 {
				log.Printf("hitl sweep: expired %d waiting run(s)", settled)
			}
		}
	}
}
