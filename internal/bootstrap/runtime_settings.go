package bootstrap

import (
	"context"
	"log"
	"os"
	"strings"
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

// runtimeBaseline builds the YAML/config baseline snapshot shared by startup
// and SIGHUP/reload so hot-reload cannot drop knobs that have no YAML field
// (e.g. ContextOffloadEnabled defaults on).
func runtimeBaseline(cfg config.Config, operatorToken, adminToken string, operators []controlplane.Operator, logFormat string) runtimecfg.Snapshot {
	return runtimecfg.Snapshot{
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
			// Memory knobs: no YAML yet; baseline always on (KV can override).
			MemoryEnabled:     true,
			MemoryAutoExtract: true,
			// Active goal: no YAML yet; baseline on with product defaults.
			ActiveGoalEnabled:      true,
			ActiveGoalMaxDepth:     8,
			ActiveGoalMaxTextRunes: 500,
			// Context offload: no YAML yet; baseline on (bulky tool/paste → workspace).
			ContextOffloadEnabled: true,
			// DP-2a defaults: feature masters default off, but the threshold and
			// prefilter width are populated so GET reports real effective values
			// once an operator turns decide + tool routing on.
			DecideToolThreshold:        12,
			DecideToolPreTopK:          16,
			DecideToolStickyEnabled:    true,
			DecideToolStickyMinJaccard: 0.7,
			// DP-3 (redirected): prune master defaults off, but size/cap
			// defaults are populated so GET reports real effective values.
			DecideToolPruneThreshold: 500,
			DecideToolPruneMaxJudged: 8,
			// DP-4: route fallback defaults off (opt-in); the length floor is
			// populated so GET reports a real effective trigger.
			DecideRouteMinRunes: 400,
		},
		Creds: runtimecfg.Credentials{
			OperatorToken: operatorToken,
			AdminToken:    adminToken,
			Operators:     operators,
		},
		PublicBaseURL: strings.TrimSpace(cfg.Runtime.PublicBaseURL),
		Observability: runtimecfg.Observability{
			LogFormat:        logFormat,
			OtelOTLPEndpoint: strings.TrimSpace(cfg.Runtime.OtelOTLPEndpoint),
			OtelServiceName:  strings.TrimSpace(cfg.Runtime.OtelServiceName),
		},
	}
}

// buildRuntimeHolder constructs the hot-reload settings holder from the config
// baseline (knobs from cfg, credentials already resolved by the caller) and
// applies any persisted KV overrides. Corrupt/unreadable KV is logged and
// ignored (falls back to YAML); it never blocks startup.
func buildRuntimeHolder(cfg config.Config, st store.Store, operatorToken, adminToken string, operators []controlplane.Operator) *runtimecfg.Holder {
	logFmt := strings.TrimSpace(cfg.Runtime.LogFormat)
	if v := strings.TrimSpace(os.Getenv("BAIZE_LOG_FORMAT")); v != "" {
		logFmt = v
	}
	h := runtimecfg.New(runtimeBaseline(cfg, operatorToken, adminToken, operators, logFmt))
	if err := h.Load(context.Background(), st); err != nil {
		log.Printf("runtimecfg: load persisted overrides failed (using YAML baseline): %v", err)
	}
	return h
}
