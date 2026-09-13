package bootstrap

import (
	"context"
	"log"
	"time"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
)

// runtimeRefreshInterval is how often the holder re-reads the KV so changes
// made on another replica propagate. Local PATCH swaps the snapshot immediately
// (no wait); this only covers cross-replica convergence.
const runtimeRefreshInterval = 20 * time.Second

// buildRuntimeHolder constructs the hot-reload settings holder from the config
// baseline (knobs from cfg, credentials already resolved by the caller) and
// applies any persisted KV overrides. Corrupt/unreadable KV is logged and
// ignored (falls back to YAML); it never blocks startup.
func buildRuntimeHolder(cfg config.Config, st store.Store, operatorToken, adminToken string, operators []controlplane.Operator) *runtimecfg.Holder {
	base := runtimecfg.Snapshot{
		Knobs: runtimecfg.Knobs{
			MaxMessages:          cfg.Conversation.MaxMessages,
			MaxSteps:             cfg.Run.MaxSteps,
			ToolTimeout:          time.Duration(cfg.Run.ToolTimeoutSec) * time.Second,
			CompactionEnabled:    cfg.CompactEnabled(),
			CompactThreshold:     cfg.Conversation.CompactThreshold,
			CompactReserveTokens: cfg.Conversation.CompactReserveOutput,
			CompactKeepRecent:    cfg.Conversation.CompactRecentMessages,
			// Not exposed in config; mirrors run.defaultCompactSummaryWait. Kept
			// non-zero so GET reports the real effective default (60s).
			CompactSummaryTimeout: 60 * time.Second,
		},
		Creds: runtimecfg.Credentials{
			OperatorToken: operatorToken,
			AdminToken:    adminToken,
			Operators:     operators,
		},
	}
	h := runtimecfg.New(base)
	if err := h.Load(context.Background(), st); err != nil {
		log.Printf("runtimecfg: load persisted overrides failed (using YAML baseline): %v", err)
	}
	return h
}
