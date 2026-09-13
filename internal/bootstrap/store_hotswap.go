package bootstrap

import (
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/inbox"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/webhook"
)

// storeRuntime holds every live reference that must flip together on Store hot-swap.
type storeRuntime struct {
	mu sync.Mutex

	hub         *eventbus.Hub
	closer      *storeAndMCPCloser
	srv         *api.Server
	engine      *run.Engine
	compactor   *run.Compactor
	dispatcher  *webhook.Dispatcher
	profiles    *llm.StoreProfileSource
	reg         *tool.Registry
	inboxReg    *inbox.Registry
	holder      *runtimecfg.Holder
	callbackCfg connector.CallbackConfig

	configPath string
	cfg        *config.Config

	// wrapped is the Notify-wrapped store exposed to API/engine.
	wrapped atomic.Value // store.Store
	// effective is the driver/path/dsn actually open in-process.
	effective config.StoreOverlay
	raw       io.Closer
}

func (rt *storeRuntime) current() store.Store {
	if rt == nil {
		return nil
	}
	v := rt.wrapped.Load()
	if v == nil {
		return nil
	}
	st, _ := v.(store.Store)
	return st
}

func (rt *storeRuntime) setWrapped(st store.Store) {
	rt.wrapped.Store(st)
}

// EffectiveStore returns the in-process store overlay (not necessarily YAML).
func (rt *storeRuntime) EffectiveStore() config.StoreOverlay {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.effective
}

// StoreConfigMismatch reports whether layered YAML store differs from the open store.
func (rt *storeRuntime) StoreConfigMismatch() bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.cfg == nil {
		return false
	}
	return storeOverlayMismatch(rt.effective, config.StoreOverlay{
		Driver:     rt.cfg.Store.Driver,
		SQLitePath: rt.cfg.Store.SQLitePath,
		DSN:        rt.cfg.Store.DSN,
	})
}

func storeOverlayMismatch(a, b config.StoreOverlay) bool {
	ad := strings.ToLower(strings.TrimSpace(a.Driver))
	bd := strings.ToLower(strings.TrimSpace(b.Driver))
	if ad == "" {
		ad = "memory"
	}
	if bd == "" {
		bd = "memory"
	}
	if ad != bd {
		return true
	}
	switch ad {
	case "sqlite":
		return strings.TrimSpace(a.SQLitePath) != strings.TrimSpace(b.SQLitePath)
	case "postgres":
		return strings.TrimSpace(a.DSN) != strings.TrimSpace(b.DSN)
	default:
		return false
	}
}

// HotSwap opens a new store from overlay, swaps all live refs, then closes the old raw store.
// On Open/Migrate failure the current store is left untouched (caller may have already written overlay).
func (rt *storeRuntime) HotSwap(overlay config.StoreOverlay) error {
	if rt == nil {
		return fmt.Errorf("store runtime not initialized")
	}
	driver := strings.ToLower(strings.TrimSpace(overlay.Driver))
	if driver == "" {
		driver = "memory"
	}
	sqlitePath := overlay.SQLitePath
	if driver == "sqlite" && strings.TrimSpace(sqlitePath) == "" {
		sqlitePath = "./data/baize.db"
	}

	newRaw, err := store.OpenWithOptions(driver, store.OpenOptions{
		SQLitePath: sqlitePath,
		DSN:        overlay.DSN,
	})
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	newCloser := storeCloser(newRaw)

	if err := store.MigrateStore(newRaw); err != nil {
		_ = newCloser.Close()
		return fmt.Errorf("migrate store: %w", err)
	}

	cfgSnap := *rt.cfg
	cfgSnap.Store.Driver = driver
	cfgSnap.Store.SQLitePath = sqlitePath
	cfgSnap.Store.DSN = overlay.DSN

	messages, identities, err := openConversationAndIdentities(newRaw, cfgSnap)
	if err != nil {
		_ = newCloser.Close()
		return fmt.Errorf("open conversation/identity: %w", err)
	}

	newRaw.UpsertAgent(store.Agent{
		ID:     cfgSnap.Agent.ID,
		System: cfgSnap.Agent.System,
		Skills: append([]string(nil), cfgSnap.Agent.Skills...),
	})

	wrapped := eventbus.Notify(newRaw, rt.hub)

	rt.mu.Lock()
	oldRaw := rt.raw
	oldIDs := connectorIDs(rt.current())

	rt.setWrapped(wrapped)
	rt.raw = newCloser
	rt.effective = config.StoreOverlay{
		Driver:     driver,
		SQLitePath: sqlitePath,
		DSN:        overlay.DSN,
	}
	rt.cfg.Store.Driver = driver
	rt.cfg.Store.SQLitePath = sqlitePath
	rt.cfg.Store.DSN = overlay.DSN
	if rt.closer != nil {
		rt.closer.inner = newCloser
	}

	rt.srv.Store = wrapped
	rt.srv.Messages = messages
	rt.srv.Identities = identities

	rt.engine.Store = wrapped
	rt.engine.Messages = messages
	rt.engine.Identities = identities
	if meta, ok := messages.(conversation.MetaStore); ok {
		rt.engine.Meta = meta
	}

	if rt.compactor != nil {
		rt.compactor.Messages = messages
	}
	if rt.dispatcher != nil {
		rt.dispatcher.SetStore(wrapped)
	}
	if rt.profiles != nil {
		rt.profiles.Store = wrapped
	}

	rt.updateChannelRuntimesLocked(wrapped, messages)
	rt.mu.Unlock()

	for _, id := range oldIDs {
		rt.reg.UnregisterConnector(id)
	}
	if err := registerConnector(wrapped, rt.reg, cfgSnap, identities, rt.callbackCfg); err != nil {
		log.Printf("store hot-swap: register YAML connector: %v", err)
	}
	loadStoredConnectors(wrapped, rt.reg, cfgSnap, identities, rt.callbackCfg)

	if rt.inboxReg != nil {
		if err := seedInboxChannels(cfgSnap, wrapped, rt.inboxReg); err != nil {
			log.Printf("store hot-swap: reload inbox channels: %v", err)
		}
	}

	if oldRaw != nil {
		if err := oldRaw.Close(); err != nil {
			log.Printf("store hot-swap: close old store: %v", err)
		}
	}
	return nil
}

func connectorIDs(st store.Store) []string {
	if st == nil {
		return nil
	}
	list := st.ListConnectors()
	ids := make([]string, 0, len(list))
	for _, c := range list {
		ids = append(ids, c.ID)
	}
	return ids
}

func (rt *storeRuntime) updateChannelRuntimesLocked(st store.Store, messages conversation.Store) {
	if rt.srv == nil {
		return
	}
	var meta conversation.MetaStore
	if m, ok := messages.(conversation.MetaStore); ok {
		meta = m
	}
	rt.srv.ForEachChannel(func(h *api.ChannelHandle) {
		if h == nil {
			return
		}
		if h.Runtime != nil {
			h.Runtime.Runs = st
			h.Runtime.Messages = messages
			if meta != nil {
				h.Runtime.Meta = meta
			}
		}
		// Option A: channels that hold a durable outbox (webhook) expose
		// SetStore; hot-swap flips their persist pointer with the live store.
		if setter, ok := h.Channel.(interface{ SetStore(store.Store) }); ok {
			setter.SetStore(st)
		}
	})
}

// reconcileStoreProxy always reads from the live storeRuntime (hot-swap safe).
type reconcileStoreProxy struct {
	rt *storeRuntime
}

func (p reconcileStoreProxy) ListRunsForReconcile(limit int) ([]*store.Run, error) {
	st := p.rt.current()
	if st == nil {
		return nil, nil
	}
	return st.ListRunsForReconcile(limit)
}

// ReloadLayeredConfig re-reads base+overlay YAML, refreshes runtimecfg baseline,
// and updates Server.Config. It does not hot-swap Store.
func (rt *storeRuntime) ReloadLayeredConfig() error {
	if rt == nil || strings.TrimSpace(rt.configPath) == "" {
		return fmt.Errorf("no config path")
	}
	overlayPath := config.LocalOverlayPath(rt.configPath)
	cfg, _, err := config.LoadLayered(rt.configPath, overlayPath)
	if err != nil {
		return err
	}

	op, err := controlplane.ResolveSecret(cfg.ControlPlane.OperatorToken)
	if err != nil {
		return fmt.Errorf("control_plane.operator_token: %w", err)
	}
	adm, err := controlplane.ResolveSecret(cfg.ControlPlane.AdminToken)
	if err != nil {
		return fmt.Errorf("control_plane.admin_token: %w", err)
	}
	var operators []controlplane.Operator
	for _, entry := range cfg.ControlPlane.Operators {
		token, err := controlplane.ResolveSecret(entry.Token)
		if err != nil {
			return fmt.Errorf("control_plane.operators[%s]: %w", entry.ID, err)
		}
		if token == "" {
			continue
		}
		operators = append(operators, controlplane.Operator{ID: entry.ID, Token: token})
	}

	base := runtimecfg.Snapshot{
		Knobs: runtimecfg.Knobs{
			MaxMessages:           cfg.Conversation.MaxMessages,
			MaxSteps:              cfg.Run.MaxSteps,
			ToolTimeout:           time.Duration(cfg.Run.ToolTimeoutSec) * time.Second,
			CompactionEnabled:     cfg.CompactEnabled(),
			CompactThreshold:      cfg.Conversation.CompactThreshold,
			CompactReserveTokens:  cfg.Conversation.CompactReserveOutput,
			CompactKeepRecent:     cfg.Conversation.CompactRecentMessages,
			CompactSummaryTimeout: 60 * time.Second,
		},
		Creds: runtimecfg.Credentials{
			OperatorToken: op,
			AdminToken:    adm,
			Operators:     operators,
		},
	}

	rt.mu.Lock()
	*rt.cfg = cfg
	rt.srv.Config = rt.cfg
	rt.srv.OperatorToken = op
	rt.srv.AdminToken = adm
	rt.srv.Operators = operators
	mismatch := storeOverlayMismatch(rt.effective, config.StoreOverlay{
		Driver:     cfg.Store.Driver,
		SQLitePath: cfg.Store.SQLitePath,
		DSN:        cfg.Store.DSN,
	})
	holder := rt.holder
	rt.mu.Unlock()

	if holder != nil {
		holder.ReplaceBaseline(base)
	}
	if mismatch {
		log.Printf("store config mismatch after reload: YAML driver=%q effective=%q; use PUT /v0/settings/store to hot-swap",
			cfg.Store.Driver, rt.EffectiveStore().Driver)
	}
	return nil
}
