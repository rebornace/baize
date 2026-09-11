package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	mockticket "github.com/rebornace/baize/examples/mock-ticket"
	"github.com/rebornace/baize/internal/analysis"
	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/artifact"
	"github.com/rebornace/baize/internal/authcred"
	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/file"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/channelmedia"
	// Built-in channel: the generic out-of-process webhook channel registers
	// its Descriptor via init() so wireChannels discovers it generically.
	// Additional in-tree channels add their own blank import in
	// cmd/baize/main.go without touching bootstrap.
	_ "github.com/rebornace/baize/internal/channel/webhook"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/connector/httpplugin"
	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/inbox"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/middleware"
	// 内置默认 middleware 驱动（memory）随 bootstrap 一起注册：它是零配置
	// 默认驱动，且 bootstrap 包自身的测试/StartForTest 集成测试不经 main。
	// 后续 redis 等驱动同样以 blank import 方式注册（任务 9）。
	_ "github.com/rebornace/baize/internal/middleware/memory"
	"github.com/rebornace/baize/internal/plugincallback"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/webhook"
	"github.com/rebornace/baize/internal/workspace"
)

// Run starts Runtime (and optionally mock-ticket) in-process and blocks on the API server.
func Run(cfg config.Config, configPath string) error {
	ticketListen := strings.TrimSpace(cfg.MockTicket.Listen)
	useMockTicket := true
	switch strings.ToLower(ticketListen) {
	case "off", "false", "none", "-":
		useMockTicket = false
	case "":
		ticketListen = ":18080"
	}

	var ticketBase string
	if useMockTicket {
		go func() {
			log.Printf("mock-ticket listening on %s", ticketListen)
			if err := http.ListenAndServe(ticketListen, mockticket.NewHandler()); err != nil {
				log.Printf("mock-ticket server error: %v", err)
			}
		}()
		ticketBase = localHTTPBase(ticketListen)
		cfg.Connector.BaseURL = ticketBase
		if err := waitHealthy(ticketBase+"/healthz", 5*time.Second); err != nil {
			return fmt.Errorf("mock-ticket health check failed: %w", err)
		}
	} else {
		ticketBase = cfg.Connector.BaseURL
	}

	srv, closer, err := newAPIServer(cfg, configPath)
	if err != nil {
		return err
	}

	listen := cfg.Listen
	if listen == "" {
		listen = ":8080"
	}
	runtimeBase := localHTTPBase(listen)
	printCurlHints(runtimeBase, cfg.Agent.ID, ticketBase)
	logUIHint(cfg, runtimeBase)
	log.Printf("baize runtime listening on %s", listen)
	httpSrv := &http.Server{Addr: listen, Handler: srv.Handler()}
	srv.Shutdown = httpSrv.Shutdown
	srv.RestartProcess = Reexec
	sigCtx, stopSig := newShutdownSignalContext()
	defer stopSig()
	shutdownOnSignal(sigCtx, httpSrv)
	err = httpSrv.ListenAndServe()
	_ = closer.Close()
	return normalizeShutdownErr(err)
}

// Serve starts Runtime only (no mock-ticket) and blocks on the API server.
func Serve(cfg config.Config, configPath string) error {
	srv, closer, err := newAPIServer(cfg, configPath)
	if err != nil {
		return err
	}
	listen := cfg.Listen
	if listen == "" {
		listen = ":8080"
	}
	runtimeBase := localHTTPBase(listen)
	printCurlHints(runtimeBase, cfg.Agent.ID, cfg.Connector.BaseURL)
	logUIHint(cfg, runtimeBase)
	log.Printf("baize runtime listening on %s", listen)
	httpSrv := &http.Server{Addr: listen, Handler: srv.Handler()}
	srv.Shutdown = httpSrv.Shutdown
	srv.RestartProcess = Reexec
	sigCtx, stopSig := newShutdownSignalContext()
	defer stopSig()
	shutdownOnSignal(sigCtx, httpSrv)
	err = httpSrv.ListenAndServe()
	_ = closer.Close()
	return normalizeShutdownErr(err)
}

// shutdownOnSignal wires SIGINT/SIGTERM (docker stop / systemctl stop / Ctrl-C)
// to a graceful HTTP server shutdown. Without this Go's default action exits
// the process immediately: ListenAndServe never returns, so the closer (which
// stops supervised adapter children gracefully) never runs and adapters are
// orphaned/killed hard. The signal context is cross-platform; production
// passes a signal.NotifyContext bound to os.Interrupt/SIGTERM, and tests pass
// a plain cancelable context to exercise the cancel->Shutdown path
// deterministically (including on Windows, where delivering SIGTERM to a
// process group is awkward). When ctx is cancelled the HTTP server is shut
// down with a bounded grace window; ListenAndServe then returns
// ErrServerClosed and the caller runs its closer (channel Stop -> adapter
// terminate).
func shutdownOnSignal(ctx context.Context, srv *http.Server) {
	go func() {
		<-ctx.Done()
		log.Printf("baize received shutdown signal; draining HTTP server gracefully")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("baize graceful shutdown: %v", err)
		}
	}()
}

// newShutdownSignalContext returns a context cancelled on SIGINT/SIGTERM (or
// Ctrl-C). Callers must invoke the returned stop func to release the signal
// registration (typically deferred).
func newShutdownSignalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// normalizeShutdownErr maps an intentional, signal-driven shutdown
// (ErrServerClosed from ListenAndServe after srv.Shutdown) to a clean nil exit.
func normalizeShutdownErr(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// StartForTest starts mock-ticket and Runtime on ephemeral ports for integration tests.
// cfg.Connector.BaseURL is overwritten with the actual ticket URL.
func StartForTest(t testing.TB, cfg config.Config) (runtimeURL, ticketURL string, shutdown func()) {
	t.Helper()

	ticketLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen mock-ticket: %v", err)
	}
	ticketURL = "http://" + ticketLn.Addr().String()
	ticketSrv := &http.Server{Handler: mockticket.NewHandler()}
	go func() { _ = ticketSrv.Serve(ticketLn) }()

	if err := waitHealthy(ticketURL+"/healthz", 5*time.Second); err != nil {
		_ = ticketSrv.Close()
		t.Fatalf("mock-ticket health check failed: %v", err)
	}

	cfg.Connector.BaseURL = ticketURL
	if cfg.Run.MaxSteps <= 0 {
		cfg.Run.MaxSteps = 16
	}

	apiSrv, storeClose, err := newAPIServer(cfg, "")
	if err != nil {
		_ = ticketSrv.Close()
		t.Fatalf("new api server: %v", err)
	}

	runtimeLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = storeClose.Close()
		_ = ticketSrv.Close()
		t.Fatalf("listen runtime: %v", err)
	}
	runtimeURL = "http://" + runtimeLn.Addr().String()
	httpSrv := &http.Server{Handler: apiSrv.Handler()}
	go func() { _ = httpSrv.Serve(runtimeLn) }()

	if err := waitHealthy(runtimeURL+"/healthz", 5*time.Second); err != nil {
		_ = httpSrv.Close()
		_ = storeClose.Close()
		_ = ticketSrv.Close()
		t.Fatalf("runtime health check failed: %v", err)
	}

	shutdown = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
		_ = ticketSrv.Shutdown(ctx)
		_ = storeClose.Close()
	}
	return runtimeURL, ticketURL, shutdown
}

func newAPIServer(cfg config.Config, configPath string) (*api.Server, io.Closer, error) {
	// newLLM validates the YAML llm section at startup and provides the
	// provider for the mock/demo path. Real deployments replace it with the
	// hot-reloadable Switch once the store (and its model profiles) is open.
	baseProvider, err := newLLM(cfg)
	if err != nil {
		return nil, nil, err
	}

	// Resolve sidecar callback wiring (Phase 2). An empty secret triggers an
	// ephemeral key so single-process dev still works; an explicit secret is
	// required for multi-process deployments where the API and the sidecar
	// invoke path must share the key.
	callbackSecret, secretEphemeral := resolveCallbackSecret(cfg.Runtime.CallbackHMACSecret)
	if secretEphemeral {
		log.Printf("runtime.callback_hmac_secret empty: generated ephemeral secret (not valid across restarts)")
	}
	callbackTTL := time.Duration(cfg.Runtime.CallbackTokenTTLSec) * time.Second
	if callbackTTL <= 0 {
		callbackTTL = time.Duration(config.DefaultCallbackTokenTTLSec) * time.Second
	}
	callbackPublicBase := strings.TrimSpace(cfg.Runtime.PublicBaseURL)
	callbackSigner := httpplugin.CallbackSigner(plugincallback.Issue)
	callbackLimiter := plugincallback.NewLimiter(plugincallback.DefaultBudget, plugincallback.DefaultWindow)
	callbackCfg := connector.CallbackConfig{
		Signer:     callbackSigner,
		Secret:     callbackSecret,
		PublicBase: callbackPublicBase,
		TTL:        callbackTTL,
	}

	st, err := store.OpenWithOptions(cfg.Store.Driver, store.OpenOptions{
		SQLitePath: cfg.Store.SQLitePath,
		DSN:        cfg.Store.DSN,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open store: %w", err)
	}
	closer := &storeAndMCPCloser{inner: storeCloser(st)}

	// Resolve the active LLM provider. The mock/demo path (provider unset or
	// "mock", matching newLLM) keeps the YAML-built provider and neither seeds
	// profiles nor builds a Switch. Real deployments seed a default profile
	// from the YAML llm section on first boot and route through the Switch,
	// which resolves providers from stored profiles and hot-reloads on edit.
	provider := baseProvider
	switch strings.ToLower(cfg.LLM.Provider) {
	case "", "mock":
		// demo/test path: keep baseProvider as-is.
	default:
		if err := seedModelProfile(st, cfg); err != nil {
			_ = closer.Close()
			return nil, nil, err
		}
		provider = llm.NewSwitch(&llm.StoreProfileSource{Store: st})
	}

	reg := tool.NewRegistry()

	skillCat, err := skill.LoadCatalog(cfg.SkillBuiltinDirs(), cfg.Skills.UserDir)
	if err != nil {
		_ = closer.Close()
		return nil, nil, fmt.Errorf("load skill catalog: %w", err)
	}

	st.UpsertAgent(store.Agent{
		ID:     cfg.Agent.ID,
		System: cfg.Agent.System,
		Skills: append([]string(nil), cfg.Agent.Skills...),
	})

	messages, identities, err := openConversationAndIdentities(st, cfg)
	if err != nil {
		_ = closer.Close()
		return nil, nil, fmt.Errorf("open conversation/identity stores: %w", err)
	}

	if err := registerConnector(st, reg, cfg, identities, callbackCfg); err != nil {
		_ = closer.Close()
		return nil, nil, err
	}
	loadStoredConnectors(st, reg, cfg, identities, callbackCfg)
	if n := len(st.ListConnectors()); n > 0 {
		ids := make([]string, 0, n)
		for _, c := range st.ListConnectors() {
			ids = append(ids, c.ID)
		}
		log.Printf("persisted connectors restored from store: %v", ids)
	}

	// Artifact store needs the underlying SQL backend; eventbus.Notify wraps st
	// and would break a direct type assertion below.
	var sqlBackend store.SQLBackend
	if sb, ok := st.(store.SQLBackend); ok {
		sqlBackend = sb
	}

	hub := eventbus.NewHub()
	st = eventbus.Notify(st, hub)

	webhookCfg, err := loadEventsWebhook(st, cfg)
	if err != nil {
		_ = closer.Close()
		return nil, nil, fmt.Errorf("load events webhook: %w", err)
	}
	dispatcher := webhook.NewDispatcher(st, webhookCfg)
	dispatcher.Attach(hub)
	webhookWorkerCtx, webhookWorkerCancel := context.WithCancel(context.Background())
	go dispatcher.StartWorker(webhookWorkerCtx)
	closer.stops = append(closer.stops, webhookWorkerCancel)

	inboxReg := inbox.NewRegistry()
	if err := seedInboxChannels(cfg, st, inboxReg); err != nil {
		_ = closer.Close()
		return nil, nil, fmt.Errorf("seed inbox channels: %w", err)
	}

	// Context compaction folds older history into a rolling summary before a
	// run when the prompt approaches the model context limit. On the
	// mock/demo path StoreProfileSource resolves no profile, so MaybeCompact
	// self-disables (returns false, nil) — no extra guard needed.
	var compactor *run.Compactor
	// Build the compactor whenever deps exist; the on/off switch is the hot
	// knob (baseline = cfg.CompactEnabled()), evaluated per-run in MaybeCompact.
	if messages != nil && provider != nil {
		compactor = &run.Compactor{
			Messages:      messages,
			LLM:           provider,
			Profiles:      &llm.StoreProfileSource{Store: st},
			Threshold:     cfg.Conversation.CompactThreshold,
			ReserveTokens: cfg.Conversation.CompactReserveOutput,
			KeepRecent:    cfg.Conversation.CompactRecentMessages,
		}
	}

	engine := &run.Engine{
		Store:       st,
		LLM:         provider,
		Tools:       reg,
		Gate:        run.NewGate(),
		MaxSteps:    cfg.Run.MaxSteps,
		ToolTimeout: time.Duration(cfg.Run.ToolTimeoutSec) * time.Second,
		Messages:    messages,
		MaxMessages: cfg.Conversation.MaxMessages,
		Compactor:   compactor,
		Identities:  identities,
		Skills:      skillCat,
	}
	srv := api.NewServer(st, reg, engine)
	cfgCopy := cfg
	srv.Config = &cfgCopy
	srv.ConfigPath = configPath
	srv.Hub = hub
	srv.Webhook = dispatcher
	srv.Inbox = inboxReg
	srv.InboxLimiter = inbox.NewRateLimiter(inbox.DefaultRateLimit, inbox.DefaultRateWindow)
	srv.MCPExportEnabled = cfg.MCPExportEnabled()
	srv.SkillCatalog = skillCat
	srv.Identities = identities
	srv.Messages = messages
	srv.DefaultAgentID = cfg.Agent.ID
	srv.LLM = provider
	srv.CallbackSecret = callbackSecret
	srv.CallbackLimiter = callbackLimiter
	srv.CallbackSigner = callbackSigner
	srv.CallbackPublicBase = callbackPublicBase
	srv.CallbackTTL = callbackTTL

	// channelMedia is set when a blob store is available (sqlBackend present)
	// and feeds both the channel Runtime (persist inbound images) and the API
	// (serve them back with the conversation ACL).
	var channelMedia *channelmedia.Store
	if sqlBackend != nil {
		blobStore, err := openBlobStore(context.Background(), cfg)
		if err != nil {
			_ = closer.Close()
			return nil, nil, fmt.Errorf("open blob store: %w", err)
		}
		artStore, err := artifact.NewStore(blobStore, sqlBackend)
		if err != nil {
			_ = closer.Close()
			return nil, nil, fmt.Errorf("open artifact store: %w", err)
		}
		reg.RegisterSpec(analysis.ToolSpec(), analysis.Invoker(artStore))
		srv.Artifacts = artStore

		// Inbound channel images (e.g. WeChat) persist to the same blob store
		// under "channel-media/" so they render inline in the web UI, and are
		// served back by the API with the owning conversation's ACL.
		channelMedia = channelmedia.New(blobStore)
		srv.ChannelMedia = channelMedia
		// Web-uploaded chat attachments persist to the same channel-media
		// namespace so they render inline / download from the user bubble.
		srv.ChatMedia = channelMedia

		// Per-conversation file workspace reuses the same blob.Store
		// (artifacts use the "artifacts/" prefix; workspace uses
		// "workspaces/"). Register the 5 built-in file tools (no approval
		// gate) and wire both the API (attachment persistence) and the
		// engine (cold-resume image rebuild).
		ws := workspace.New(blobStore, workspace.WithVision(func() bool {
			return provider != nil && provider.SupportsVision()
		}))
		for _, tm := range workspace.Tools(ws) {
			reg.RegisterSpecApproved(tm.Spec, tm.Invoker, false)
		}
		srv.Workspace = ws
		engine.ImagePartResolver = func(convID, wsPath string) (llm.ContentPart, bool) {
			return ws.ResolveImagePart(context.Background(), convID, wsPath)
		}
	}
	srv.AuthMode = authcred.NormalizeMode(cfg.Connector.Auth.Mode)
	srv.AuthWhitelist = cfg.Connector.Auth.Passthrough.Headers

	op, err := controlplane.ResolveSecret(cfg.ControlPlane.OperatorToken)
	if err != nil {
		_ = closer.Close()
		return nil, nil, fmt.Errorf("control_plane.operator_token: %w", err)
	}
	adm, err := controlplane.ResolveSecret(cfg.ControlPlane.AdminToken)
	if err != nil {
		_ = closer.Close()
		return nil, nil, fmt.Errorf("control_plane.admin_token: %w", err)
	}
	var operators []controlplane.Operator
	for _, entry := range cfg.ControlPlane.Operators {
		token, err := controlplane.ResolveSecret(entry.Token)
		if err != nil {
			_ = closer.Close()
			return nil, nil, fmt.Errorf("control_plane.operators[%s]: %w", entry.ID, err)
		}
		if token == "" {
			continue
		}
		operators = append(operators, controlplane.Operator{ID: entry.ID, Token: token})
	}
	srv.OperatorToken = op
	srv.AdminToken = adm
	srv.Operators = operators

	// Hot-reloadable runtime settings: build from config baseline + persisted KV
	// overrides, inject into engine/compactor/server, and start a TTL refresh so
	// cross-replica PATCHes converge. nil holder never happens here (always
	// built), but consumers still nil-guard for tests.
	runtimeHolder := buildRuntimeHolder(cfg, st, op, adm, operators)
	engine.Settings = runtimeHolder
	srv.Settings = runtimeHolder
	if compactor != nil {
		compactor.Settings = runtimeHolder
	}
	refreshCtx, refreshCancel := context.WithCancel(context.Background())
	closer.stops = append(closer.stops, refreshCancel)
	go runtimeHolder.StartRefresh(refreshCtx, st, runtimeRefreshInterval)

	if dir := dataDir(cfg); dir != "" {
		srv.DataDir = dir
	}
	// Long-lived context for channel inbound loops; cancelled first on
	// shutdown (per-channel Stop closures run after and drain their loops).
	runCtx, runCancel := context.WithCancel(context.Background())
	closer.stops = append(closer.stops, runCancel)
	if _, err := wireChannels(channelDeps{
		srv:            srv,
		st:             st,
		messages:       messages,
		engine:         engine,
		provider:       provider,
		defaultAgentID: srv.DefaultAgentID,
		channelMedia:   channelMedia,
		runCtx:         runCtx,
		closer:         closer,
		cfg:            cfg,
	}); err != nil {
		_ = closer.Close()
		return nil, nil, fmt.Errorf("wire channels: %w", err)
	}

	// 装配 middleware 驱动（队列/事件总线/限流）。此后三个入队点（HTTP
	// Dispatch、run_start、渠道 AfterCreateRun）走驱动队列 + worker 池竞争
	// 消费，不再走 nil 队列的本地 goroutine；崩溃调和器周期性把租约过期的
	// 孤儿 run 重新入队。memory 为默认驱动；redis 驱动在任务 9 接入。
	// driver 名在此再归一一次：config.Normalize 已保证默认 memory，但测试
	// 可能直接构造 config.Config 而不经过 Normalize。
	driver := strings.ToLower(strings.TrimSpace(cfg.Middleware.Driver))
	if driver == "" {
		driver = "memory"
	}
	mw, err := middleware.Open(context.Background(), driver, middleware.Options{
		WorkerConcurrency: cfg.Middleware.WorkerConcurrency,
		LeaseTTL:          time.Duration(cfg.Middleware.LeaseTTLSec) * time.Second,
		ReconcileInterval: time.Duration(cfg.Middleware.ReconcileIntervalSec) * time.Second,
		Redis: middleware.RedisOptions{
			Addr:          cfg.Middleware.Redis.Addr,
			DB:            cfg.Middleware.Redis.DB,
			Username:      cfg.Middleware.Redis.Username,
			Password:      redisPasswordFromEnv(cfg.Middleware.Redis.PasswordEnv),
			Stream:        cfg.Middleware.Redis.Stream,
			ConsumerGroup: cfg.Middleware.Redis.ConsumerGroup,
			EventsChannel: cfg.Middleware.Redis.EventsChannel,
		},
	})
	if err != nil {
		_ = closer.Close()
		return nil, nil, fmt.Errorf("open middleware driver %q: %w", driver, err)
	}
	srv.Queue = mw.Queue
	srv.LeaseTTL = time.Duration(cfg.Middleware.LeaseTTLSec) * time.Second

	// redis 驱动下用分布式 BudgetLimiter 覆盖 inbox/callback 入站限流；
	// memory 驱动不设 Gate，继续走进程内 InboxLimiter/CallbackLimiter。
	// 内存默认限流器仍保留（gate 优先），既有测试可依赖字段非 nil。
	if driver == "redis" {
		if bl, ok := mw.Limiter.(middleware.BudgetLimiter); ok {
			srv.InboxGate = func(channelID string) bool {
				return bl.AllowBudget("baize:rl:inbox:"+channelID, inbox.DefaultRateLimit, inbox.DefaultRateWindow)
			}
			srv.CallbackGate = func(runID string) bool {
				return bl.AllowBudget("baize:rl:cb:"+runID, plugincallback.DefaultBudget, plugincallback.DefaultWindow)
			}
		}
	}

	// 跨副本发布：本地 AppendEvent 经 Notify → Hub.Publish 后，再经 Bus
	// 发出 RunEventNudge。跳过 external.nudge，避免 BridgeToHub 回环。
	// memory bus 的 PublishRunEvent 只写本地 channel，语义无接受。
	hub.OnEvent(func(runID string, ev eventbus.IndexedEvent) {
		if ev.Event.Type == "external.nudge" {
			return
		}
		if err := mw.Bus.PublishRunEvent(context.Background(), runID, int64(ev.Index)); err != nil {
			log.Printf("middleware: publish run event nudge: %v", err)
		}
	})

	// 事件总线桥接：redis 驱动（任务 9）实现 BridgeToHub/Start，把跨副本
	// nudge 注入本地 Hub 并启动 Pub/Sub 订阅；memory 驱动的 bus 不实现该
	// 接口，类型断言失败即安全跳过（SSE/webhook 直接走进程内 Hub）。
	type hubBridge interface {
		BridgeToHub(hub *eventbus.Hub)
		Start(ctx context.Context)
	}
	mwCtx, mwCancel := context.WithCancel(context.Background())
	if b, ok := mw.Bus.(hubBridge); ok {
		b.BridgeToHub(hub)
		b.Start(mwCtx)
	}
	stopWorkers := mw.StartWorkers(mwCtx, srv)
	stopReconciler := mw.StartReconciler(mwCtx, st, time.Duration(cfg.Middleware.ReconcileIntervalSec)*time.Second)
	// closer.stops 逆序执行：本闭包最后追加、最先运行——先取消总线订阅并
	// 停 worker/调和器（等待在飞 job 收尾），再关队列；随后才轮到 IM 渠道、
	// webhook worker 与 store 的关闭，保证关停期间不再有新 job 入队/执行。
	closer.stops = append(closer.stops, func() {
		mwCancel()
		stopWorkers()
		stopReconciler()
		_ = mw.Close()
	})
	return srv, closer, nil
}

// redisPasswordFromEnv resolves the redis password from the named environment
// variable. An empty env name means no password.
func redisPasswordFromEnv(env string) string {
	if env == "" {
		return ""
	}
	return os.Getenv(env)
}

// openBlobStore builds the configured object-storage driver. The file driver
// roots under dataDir when storage.file.root_dir is unset, preserving the
// historical <dataDir>/artifacts layout.
func openBlobStore(ctx context.Context, cfg config.Config) (blob.Store, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.Storage.Driver))
	if driver == "" {
		driver = "file"
	}
	opts := blob.Options{
		File: blob.FileOptions{RootDir: cfg.Storage.File.RootDir},
		S3: blob.S3Options{
			Endpoint:   cfg.Storage.S3.Endpoint,
			Region:     cfg.Storage.S3.Region,
			Bucket:     cfg.Storage.S3.Bucket,
			Prefix:     cfg.Storage.S3.Prefix,
			AccessKey:  s3CredFromEnv(cfg.Storage.S3.AccessKeyEnv),
			SecretKey:  s3CredFromEnv(cfg.Storage.S3.SecretKeyEnv),
			UseSSL:     cfg.StorageUseSSL(),
			PathStyle:  cfg.Storage.S3.PathStyle,
			AutoCreate: cfg.Storage.S3.AutoCreateBucket,
		},
	}
	if driver == "file" && opts.File.RootDir == "" {
		opts.File.RootDir = dataDir(cfg)
		if opts.File.RootDir == "" {
			opts.File.RootDir = "./data"
		}
	}
	return blob.Open(ctx, driver, opts)
}

// s3CredFromEnv resolves an S3 credential from the named environment variable.
func s3CredFromEnv(env string) string {
	if env == "" {
		return ""
	}
	return os.Getenv(env)
}

// channelDeps carries the shared assembly dependencies into the generic
// channel wiring loop. Each Bootstrapper channel builds its own Runtime from
// these via channel.BuildDeps.
type channelDeps struct {
	srv            *api.Server
	st             store.Store
	messages       conversation.Store
	engine         *run.Engine
	provider       llm.Provider
	defaultAgentID string
	channelMedia   *channelmedia.Store
	runCtx         context.Context
	closer         *storeAndMCPCloser
	cfg            config.Config
}

// wireChannels iterates every registered channel Descriptor, builds the
// channel, and assembles it: channels implementing channel.Bootstrapper build
// their own Runtime (from persisted per-channel settings), are added to the
// outbound router, and conditionally start their inbound loop. The router is
// wired as the single Outbound/OutboundExtras for both the api server and the
// run engine; non-Bootstrapper channels are registered for future use only.
//
// When cfg.Channels is empty (section omitted), every registered channel is
// auto-wired once by its type name — identical to the pre-declarative-config
// behavior — except Descriptor.DeclarativeOnly types (the webhook channel),
// which require per-instance opaque config and support multiple instances, so
// they are never auto-wired on the legacy path. With no in-process IM channel
// registered, the legacy path wires zero IM channels; IM accounts are declared
// explicitly via channels: (webhook instances). When cfg.Channels lists
// entries, each entry wires one named instance (the entry name, defaulting to
// the type); a type is wired only when its entry is enabled:true, while a
// built-in default not listed at all (Descriptor.EnabledByDefault) stays
// wired — this keeps partial declarative configs backward compatible. An
// explicit enabled:false always wins. Duplicate instance
// names, duplicate instance sources, or unknown channel types fail fast.
// Per-entry config.creds_dir overrides the descriptor DefaultCredsDir; other
// opaque config keys (including "name" and "source") are passed through.
func wireChannels(d channelDeps) (*channel.Router, error) {
	meta, ok := d.messages.(conversation.MetaStore)
	if !ok {
		return nil, fmt.Errorf("conversation store does not support meta")
	}
	declarative := len(d.cfg.Channels) > 0

	// resolveModel performs task-aware Auto routing for inbound channel
	// messages (which have no manual model picker). It is read live on every
	// inbound message so adding/editing a model in Settings takes effect
	// without restart. Returns the picked profile id ("" = Switch primary),
	// whether an image turn can reach a vision-capable model, and whether any
	// model is configured at all. The policy is shared with the interactive web
	// path via llm.ResolveModel.
	resolveModel := func(sig llm.TaskSignals) (id string, visionOK, hasModels bool) {
		if d.st == nil {
			return "", false, false
		}
		list, err := d.st.ListModelProfiles()
		if err != nil || len(list) == 0 {
			return "", false, len(list) > 0
		}
		sel, ok := llm.ResolveModel(llm.AutoProfileID, sig, llm.RoutingProfilesFrom(list))
		return sel.ProfileID, ok, true
	}

	router := channel.NewRouter()
	deps := channel.BuildDeps{
		Store:          d.st,
		Meta:           meta,
		Messages:       d.messages,
		DefaultAgentID: d.defaultAgentID,
		Media:          d.channelMedia,
		ResolveModel:   resolveModel,
		AfterCreateRun: func(ctx context.Context, runRec *store.Run, userParts []llm.ContentPart) error {
			d.srv.Dispatch(context.Background(), middleware.Job{
				RunID:     runRec.ID,
				Kind:      middleware.KindRun,
				AgentID:   runRec.AgentID,
				Input:     runRec.Input,
				UserParts: api.PartsToMiddleware(userParts),
			})
			return nil
		},
		ResumeHITL: func(ctx context.Context, runID string, approve bool, comment string) error {
			return d.engine.ContinueFromHITL(ctx, runID, run.Decision{Approve: approve, Comment: comment})
		},
		// api.Server implements channel.RouteRegistrar: channels such as the
		// webhook channel mount their own inbound HTTP routes during Bootstrap.
		Routes:  d.srv,
		DataDir: dataDir(d.cfg),
	}

	// seenSource tracks the SourceSourced.Source() each successfully built
	// instance registered under. The source-keyed router silently overwrites
	// duplicate sources, so fail fast instead of hiding one instance.
	seenSource := map[string]string{}

	// assemble builds and wires one instance. instName is the api handle key
	// (descriptor type name for the legacy path; configured instance name for
	// declarative). overrides carries the instance's opaque config keys; chCfg
	// seeds "name" (per-instance name read by multi-instance factories such as
	// webhook) and the descriptor default creds dir.
	assemble := func(desc channel.Descriptor, instName string, overrides map[string]string) error {
		chCfg := channel.Config{"name": instName, "creds_dir": desc.DefaultCredsDir}
		for k, v := range overrides {
			chCfg[k] = v
		}
		ch, err := desc.Build(chCfg)
		if err != nil {
			return fmt.Errorf("open channel %s: %w", instName, err)
		}
		if ss, ok := ch.(channel.SourceSourced); ok {
			src := strings.TrimSpace(ss.Source())
			if src != "" {
				if prior, dup := seenSource[src]; dup {
					return fmt.Errorf("duplicate channel source %q (instances %q and %q)", src, prior, instName)
				}
				seenSource[src] = instName
			}
		}
		handle := &api.ChannelHandle{
			Name:     instName,
			Channel:  ch,
			CredsDir: chCfg["creds_dir"],
			RunCtx:   d.runCtx,
		}
		if bs, isBoot := ch.(channel.Bootstrapper); isBoot {
			rt, dir, start, err := bs.Bootstrap(deps)
			if err != nil {
				return fmt.Errorf("bootstrap channel %s: %w", instName, err)
			}
			handle.Runtime = rt
			if dir != "" {
				handle.CredsDir = dir
			}
			router.Add(ch)
			router.BindRuntime(rt)

			// Shutdown: Stop closures run in reverse registration order.
			stopCh := ch
			d.closer.stops = append(d.closer.stops, func() {
				stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = stopCh.Stop(stopCtx)
			})

			if start {
				if err := ch.Start(d.runCtx); err != nil {
					log.Printf("%s channel: start skipped: %v", instName, err)
				} else {
					log.Printf("%s channel: started (creds_dir=%s)", instName, handle.CredsDir)
				}
			}
		}
		d.srv.RegisterChannel(handle)
		return nil
	}

	if !declarative {
		// Legacy/back-compat: with no channels: section, auto-wire every
		// registered descriptor once by its type name. DeclarativeOnly types
		// (webhook) are skipped: they require per-instance opaque config and
		// support multiple instances, so there is no sensible single instance
		// to build here.
		for _, desc := range channel.Descriptors() {
			if desc.DeclarativeOnly {
				continue
			}
			if err := assemble(desc, desc.Name, nil); err != nil {
				return nil, err
			}
		}
	} else {
		seenName := map[string]bool{}
		listedType := map[string]bool{}
		for _, cc := range d.cfg.Channels {
			listedType[strings.TrimSpace(cc.Type)] = true
		}
		for _, cc := range d.cfg.Channels {
			typ := strings.TrimSpace(cc.Type)
			if typ == "" {
				continue
			}
			desc, ok := channel.Describe(typ)
			if !ok {
				return nil, fmt.Errorf("unknown channel type %q", typ)
			}
			instName := strings.TrimSpace(cc.Name)
			if instName == "" {
				instName = typ
			}
			if seenName[instName] {
				return nil, fmt.Errorf("duplicate channel instance name %q", instName)
			}
			seenName[instName] = true
			if !cc.Enabled {
				log.Printf("%s channel: disabled by config; skipped", instName)
				continue
			}
			if err := assemble(desc, instName, cc.Config); err != nil {
				return nil, err
			}
		}
		// Built-in defaults not explicitly listed stay wired (back-compat).
		for _, desc := range channel.Descriptors() {
			if desc.EnabledByDefault && !listedType[desc.Name] {
				if err := assemble(desc, desc.Name, nil); err != nil {
					return nil, err
				}
			}
		}
	}

	d.srv.Outbound = router
	d.srv.OutboundExtras = router.Extras
	d.engine.Meta = meta
	d.engine.Outbound = router
	d.engine.OutboundExtras = router.Extras
	return router, nil
}

func dataDir(cfg config.Config) string {
	switch strings.ToLower(cfg.Store.Driver) {
	case "sqlite":
		if strings.TrimSpace(cfg.Store.SQLitePath) != "" {
			return filepath.Dir(cfg.Store.SQLitePath)
		}
	case "postgres":
		return "./data"
	}
	return ""
}

// loadEventsWebhook returns the persisted events webhook config, seeding from
// YAML defaults when the settings KV is empty.
func loadEventsWebhook(st store.Store, cfg config.Config) (webhook.Config, error) {
	raw, ok, err := st.GetSetting(store.SettingKeyEventsWebhook)
	if err != nil {
		return webhook.Config{}, err
	}
	if ok && len(raw) > 0 {
		var out webhook.Config
		if err := json.Unmarshal(raw, &out); err != nil {
			return webhook.Config{}, err
		}
		return out, nil
	}
	def := webhook.Config{
		URL:     cfg.Events.Webhook.URL,
		Headers: cfg.Events.Webhook.Headers,
	}
	if def.Headers == nil {
		def.Headers = map[string]string{}
	}
	b, err := json.Marshal(def)
	if err != nil {
		return webhook.Config{}, err
	}
	if err := st.UpsertSetting(store.SettingKeyEventsWebhook, b); err != nil {
		return webhook.Config{}, err
	}
	return def, nil
}

// seedInboxChannels loads channel config from the settings KV when present,
// otherwise seeds from YAML and persists generated secrets to the store.
func seedInboxChannels(cfg config.Config, st store.Store, reg *inbox.Registry) error {
	raw, ok, err := st.GetSetting(store.SettingKeyInboxChannels)
	if err != nil {
		return err
	}
	if ok && len(raw) > 0 {
		var channels []inbox.Channel
		if err := json.Unmarshal(raw, &channels); err != nil {
			return err
		}
		reg.Replace(channels)
		return nil
	}

	channels := append([]inbox.Channel(nil), cfg.Inbox.Channels...)
	for i := range channels {
		if strings.TrimSpace(channels[i].Secret) == "" {
			channels[i].Secret = inbox.GenerateSecret()
			log.Printf("warning: inbox channel %q had empty secret; generated and persisted to store", channels[i].ID)
		}
	}
	reg.Replace(channels)
	b, err := json.Marshal(channels)
	if err != nil {
		return err
	}
	return st.UpsertSetting(store.SettingKeyInboxChannels, b)
}

// openConversationAndIdentities builds the conversation message store and the
// identity store based on the configured driver.
//
//   - memory driver: both stores are in-memory (PersistIdentities is ignored).
//   - sqlite/postgres: messages and (by default) identities use the shared SQL DB.
func openConversationAndIdentities(st store.Store, cfg config.Config) (conversation.Store, identity.Store, error) {
	driver := strings.ToLower(cfg.Store.Driver)
	if driver == "" {
		driver = "memory"
	}

	if driver == "memory" {
		return conversation.NewMemoryStore(), identity.NewMemoryStore(), nil
	}

	sqlBackend, ok := st.(store.SQLBackend)
	if !ok {
		return conversation.NewMemoryStore(), identity.NewMemoryStore(), nil
	}
	db := sqlBackend.DB()
	dialect := sqlBackend.Dialect()

	msgs, err := conversation.OpenSQL(db, dialect)
	if err != nil {
		return nil, nil, err
	}

	persist := cfg.Conversation.PersistIdentities
	if persist != nil && !*persist {
		return msgs, identity.NewMemoryStore(), nil
	}
	identities, err := identity.OpenSQL(db, dialect)
	if err != nil {
		return nil, nil, err
	}
	return msgs, identities, nil
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

func storeCloser(st store.Store) io.Closer {
	if c, ok := st.(io.Closer); ok {
		return c
	}
	return nopCloser{}
}

type storeAndMCPCloser struct {
	inner io.Closer
	stops []func()
}

func (c *storeAndMCPCloser) Close() error {
	for i := len(c.stops) - 1; i >= 0; i-- {
		if c.stops[i] != nil {
			c.stops[i]()
		}
	}
	connector.CloseAllMCPSessions()
	if c.inner != nil {
		return c.inner.Close()
	}
	return nil
}

func registerConnector(st store.Store, reg *tool.Registry, cfg config.Config, identities identity.Store, cb connector.CallbackConfig) error {
	if strings.TrimSpace(cfg.Connector.ID) == "" {
		return nil
	}
	// YAML 省略 require_login 时传 nil，让 MergeCatalog 保留行上已持久化的
	// per-tool require_login；仅当 YAML 显式给出名单（含空数组）时才传指针，
	// 否则重启会把设置页勾选的 require_login 冲掉。
	var requireLogin *[]string
	if cfg.Connector.RequireLogin != nil {
		login := cfg.Connector.RequireLogin
		requireLogin = &login
	}
	// 同 require_login：YAML 省略 require_approval 时传 nil，保留设置页勾选的
	// per-tool 审批位；仅当 YAML 显式给出名单（含空数组=全部取消）时才传指针。
	var requireApproval *[]string
	if cfg.Connector.RequireApproval != nil {
		approval := cfg.Connector.RequireApproval
		requireApproval = &approval
	}
	_, _, err := connector.Apply(connector.ApplyInput{
		Store:                   st,
		Registry:                reg,
		Identities:              identities,
		ID:                      cfg.Connector.ID,
		Type:                    cfg.Connector.Type,
		Spec:                    cfg.Connector.Spec,
		BaseURL:                 cfg.Connector.BaseURL,
		ExecutionCallbackURL:    cfg.Connector.ExecutionCallbackURL,
		RequireApproval:         requireApproval,
		RequireApprovalMutating: cfg.Connector.RequireApprovalMutating,
		RequireLogin:            requireLogin,
		Auth: store.ConnectorAuth{
			Mode:        cfg.Connector.Auth.Mode,
			Static:      store.StaticAuth{Headers: cfg.Connector.Auth.Static.Headers},
			Passthrough: store.PassThruAuth{Headers: cfg.Connector.Auth.Passthrough.Headers},
			VaultRef:    store.VaultRefAuth{Headers: cfg.Connector.Auth.VaultRef.Headers},
			Capture: store.CaptureAuth{
				ToolNameGlob:   cfg.Connector.Auth.Capture.ToolNameGlob,
				TokenJSONPaths: cfg.Connector.Auth.Capture.TokenJSONPaths,
				LabelJSONPaths: cfg.Connector.Auth.Capture.LabelJSONPaths,
				HeaderTemplate: cfg.Connector.Auth.Capture.HeaderTemplate,
				DefaultScheme:  cfg.Connector.Auth.Capture.DefaultScheme,
			},
		},
		CallbackSigner:     cb.Signer,
		CallbackSecret:     cb.Secret,
		CallbackPublicBase: cb.PublicBase,
		CallbackTTL:        cb.TTL,
	})
	return err
}

// loadStoredConnectors re-applies every connector persisted in the Store other
// than the YAML-configured one. Apply re-merges the persisted catalog (so
// disabled rows and extra rows survive a restart) and re-registers only the
// enabled rows. Errors are logged but do not abort startup so a single bad
// connector cannot brick the runtime; the YAML connector is skipped because it
// was just registered by registerConnector.
func loadStoredConnectors(st store.Store, reg *tool.Registry, cfg config.Config, identities identity.Store, cb connector.CallbackConfig) {
	for _, c := range st.ListConnectors() {
		if c.ID == cfg.Connector.ID {
			continue
		}
		_, _, err := connector.Apply(connector.ApplyInput{
			Store:                st,
			Registry:             reg,
			Identities:           identities,
			ID:                   c.ID,
			Type:                 c.Type,
			Spec:                 c.Spec,
			BaseURL:              c.BaseURL,
			ExecutionCallbackURL: c.ExecutionCallbackURL,
			Auth:                 c.Auth,
			MCP:                  c.MCP,
			// 重放已持久化连接器时传 nil：保留 catalog 行上的 per-tool 审批位，
			// 不把 connector 行上聚合的名单当成"整表重写"（merged 行的审批位
			// 本来就从 store 行保留，不会丢数据）。
			RequireApproval:    nil,
			RequireLogin:       nil, // preserve persisted per-tool require_login
			CallbackSigner:     cb.Signer,
			CallbackSecret:     cb.Secret,
			CallbackPublicBase: cb.PublicBase,
			CallbackTTL:        cb.TTL,
		})
		if err != nil {
			log.Printf("loadStoredConnectors: %s: %v", c.ID, err)
		}
	}
}

// seedModelProfile ensures at least one profile exists, seeding a default from
// the YAML llm section on first boot. The API key is resolved from the
// environment (api_key_env) at call time and is never stored; the profile
// keeps api_key_env so the Switch can resolve the key per request.
func seedModelProfile(st store.Store, cfg config.Config) error {
	list, err := st.ListModelProfiles()
	if err != nil {
		return err
	}
	if len(list) > 0 {
		return nil
	}
	env := cfg.LLM.APIKeyEnv
	if env == "" {
		env = "BAIZE_API_KEY"
	}
	seedName := cfg.LLM.Model
	if seedName == "" {
		seedName = "配置模型"
	}
	if _, err := st.UpsertModelProfile(store.ModelProfile{
		Name:            seedName,
		Provider:        "openai_compatible",
		BaseURL:         cfg.LLM.BaseURL,
		Model:           cfg.LLM.Model,
		APIKeyEnv:       env,
		DisableThinking: cfg.LLM.DisableThinking,
		SupportsVision:  cfg.LLM.SupportsVision,
		ContextTokens:   128000,
		AutoTier:        llm.InferTier(cfg.LLM.Model),
	}); err != nil {
		return fmt.Errorf("seed model profile: %w", err)
	}
	return nil
}

func newLLM(cfg config.Config) (llm.Provider, error) {
	switch strings.ToLower(cfg.LLM.Provider) {
	case "", "mock":
		m := llm.NewMock()
		m.VisionSupported = cfg.LLM.SupportsVision
		return m, nil
	case "openai_compatible":
		env := cfg.LLM.APIKeyEnv
		if env == "" {
			env = "BAIZE_API_KEY"
		}
		p := llm.NewOpenAI(cfg.LLM.BaseURL, os.Getenv(env), cfg.LLM.Model)
		p.DisableThinking = cfg.LLM.DisableThinking
		p.VisionSupported = cfg.LLM.SupportsVision
		return p, nil
	default:
		return nil, fmt.Errorf("unknown llm.provider: %s", cfg.LLM.Provider)
	}
}

func waitHealthy(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	var last error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			last = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			last = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	if last == nil {
		last = fmt.Errorf("timeout")
	}
	return last
}

func localHTTPBase(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		// listen may be ":8080"
		if strings.HasPrefix(listen, ":") {
			return "http://127.0.0.1" + listen
		}
		return "http://" + listen
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// resolveCallbackSecret returns the configured HMAC secret, or a freshly
// generated 32-byte ephemeral secret when none is configured. The second
// return is true when the secret was ephemeral so the caller can log it.
// An ephemeral secret is only valid for the lifetime of this process; tokens
// issued before a restart are invalid afterwards, which is acceptable for
// local dev but not for multi-process deployments.
func resolveCallbackSecret(configured string) ([]byte, bool) {
	if s := strings.TrimSpace(configured); s != "" {
		return []byte(s), false
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		// rand.Read should never fail on modern systems; fall back to a
		// time-seeded-ish key to keep startup alive (still ephemeral).
		log.Printf("crypto/rand failed (%v); using weak ephemeral secret", err)
		for i := range buf {
			buf[i] = byte(time.Now().UnixNano() >> uint(i%8))
		}
	}
	return buf, true
}

func printCurlHints(runtimeBase, agentID, ticketBase string) {
	if agentID == "" {
		agentID = "ticket-agent"
	}
	if ticketBase == "" {
		ticketBase = "http://127.0.0.1:18080"
	}
	log.Printf("baize ready. try:")
	log.Printf(`  curl -s -X POST %s/v0/runs -H "Content-Type: application/json" -d "{\"agent_id\":\"%s\",\"input\":\"创建一个紧急工单：VPN 挂了\"}"`, runtimeBase, agentID)
	log.Printf(`  curl -s %s/tickets`, ticketBase)
}

func logUIHint(cfg config.Config, runtimeBase string) {
	if cfg.UI.Enabled {
		log.Printf("open %s/ui", runtimeBase)
	}
}
