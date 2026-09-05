# 运行时设置热更新（Runtime Settings Hot-Reload）实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 让引擎参数、控制面凭据、微信渠道启停三类设置可经 API 即时修改、不重启生效，并跨重启保留、跨副本一致。

**架构：** 新增 `internal/runtimecfg` 包，核心 `Holder` 持 `atomic.Pointer[Snapshot]`（读无锁）；快照含 `Knobs`（引擎参数）+ `Creds`（控制面凭据）。显式覆盖以 JSON 存入单个 DB 设置键 `runtime_settings`；PATCH 先写库再原子换快照，后台 TTL（20s）重读实现跨副本一致。消费点（`run.Engine`/`run.Compactor`/`api.Server`）注入窄读取接口，nil 时全部回退原字段，旧测试零改动。微信启停沿用 `settings.json`，补 `Enabled→Start/Stop` 调和。

**技术栈：** Go 1.22、标准库 `net/http`、`sync/atomic`、`encoding/json`；现有 `store.GetSetting/UpsertSetting` KV、`controlplane` 鉴权、`channel/weixin`。

**规格：** `docs/superpowers/specs/2026-09-05-runtime-settings-hot-reload-design.md`

**模块路径：** `github.com/rebornace/baize`

**关键既有事实（实现前必读）：**
- KV 接口在 `internal/store/store.go:339-340`：`GetSetting(key) ([]byte, bool, error)`、`UpsertSetting(key, []byte) error`；无 Delete（reset = 覆写空 JSON `{}`）。
- 设置键常量在 `internal/store/store.go:126-129`（`SettingKeyEventsWebhook` 等）。
- 鉴权 `controlplane.Tokens{Operator, Admin string; Operators []Operator}`（`internal/controlplane/auth.go:16`），`Operator{ID, Token string}`（`principal.go:5`）。
- 路由 ACL 在 `internal/controlplane/acl.go`（段匹配表 + `MinRole`）；handler 注册在 `internal/api/server.go` 的 `routes()`（微信在 `server.go:387-391`，路径 `/v0/settings/channels/weixin`）。
- 引擎字段：`Engine.MaxSteps/ToolTimeout/MaxMessages/Compactor`（`internal/run/engine.go:46-85`）；读取点 `toolTimeout()`(218)、`buildMessages`(243 用 `e.MaxMessages`)、`runLoop`(533 用 `e.MaxSteps`)、工具超时在 `engine.go:355` 与 `invokeTool`(795)。
- 压缩器 `Compactor.Threshold/ReserveTokens/KeepRecent`，`normalize()` 在 `MaybeCompact` 开头（`internal/run/compact.go:40-64`）。
- 微信 `Channel.Start(ctx) error`（缺凭证返回 error）、`Stop(ctx) error`、`IsStarted() bool`（`internal/channel/weixin/channel.go:81,112,142`）。
- bootstrap 装配：compactor `bootstrap.go:298-308`、engine `310-321`、`srv.OperatorToken/AdminToken/Operators` 在 `397-400`；凭据已由 `controlplane.ResolveSecret` 解析（`378-396`）；微信 `runCtx` 在 `596`、`srv.WeixinRunCtx=runCtx` 在 `608`。
- CLI 子命令在 `cmd/baize/main.go` 的 `switch os.Args[1]`（start/demo/serve）。
- 打开 store：`store.OpenWithOptions(driver, store.OpenOptions{SQLitePath: ...})`；配置 `cfg.Store.Driver`、`cfg.Store.SQLitePath`。

---

## 文件结构

**新建：**
- `internal/runtimecfg/runtimecfg.go` — `Knobs`、`Credentials`、`Snapshot` 类型；`Holder`（原子快照 + 基线 + 覆盖合并）；`Knobs()`/`Credentials()`/`Snapshot()` 读取；`New(base Snapshot)`。
- `internal/runtimecfg/persist.go` — KV JSON 结构 `persisted`、`Load(ctx, st)`（读 KV 覆盖、损坏回退）、`applyKnobsPatch`/`applyCredsPatch`（校验+合并）、`StartRefresh`（TTL 重读）。
- `internal/runtimecfg/runtimecfg_test.go` — 合并/回退/校验/TTL/损坏/nil/凭据 break-glass 测试。
- `internal/api/server_settings_runtime.go` — `GET/PATCH /v0/settings/runtime`、`GET/PATCH /v0/settings/credentials` 四个 handler + 请求/响应类型。
- `internal/api/server_settings_runtime_test.go` — 端点、脱敏、403/400/409、部分更新、reset、热改 gateTokens 测试。
- `internal/bootstrap/runtime_settings.go` — `buildRuntimeHolder(cfg, st)`：从 config 建基线 + 读 KV 覆盖。
- `cmd/baize/settings_reset.go` — `reset-credentials` 子命令实现（覆写 KV 凭据段）。

**修改：**
- `internal/store/store.go` — 加 `SettingKeyRuntimeSettings = "runtime_settings"` 常量。
- `internal/run/engine.go` — 加 `Settings KnobReader` 字段与 `currentKnobs()`；三处读取点改用快照；`Compactor` 调用传入开关。
- `internal/run/compact.go` — `Compactor` 加 `Settings KnobReader`；`MaybeCompact` 开头求值有效参数与开关。
- `internal/run/knobs.go`（新建小文件）— 定义 `KnobReader` 接口 + 默认值常量，避免 run 直接 import runtimecfg 的环（见任务 2 说明）。
- `internal/api/server.go` — 加 `Settings SettingsReader` 字段；`gateTokens()` 改读快照；`routes()` 注册 4 个新端点。
- `internal/controlplane/acl.go` + `acl_test.go` — 加 4 条路由规则与断言。
- `internal/api/server_channel_weixin.go` — `applyWeixinSettings` 扩展 Enabled→Start/Stop 调和；PUT 响应加 `running`。
- `internal/api/server_channel_weixin_test.go` — 补启停热应用测试。
- `internal/bootstrap/bootstrap.go` — 构造 holder、注入 engine/compactor/srv、`StartRefresh`。

**依赖方向（无环）：** `runtimecfg` 只 import `controlplane`、`store`、标准库；`run` import `runtimecfg` 并定义窄接口 `KnobReader`（`runtimecfg` 不 import `run`/`api`）；`api` 直接持 `*runtimecfg.Holder`（具体类型，nil 即回退）；`bootstrap` import 两者做装配。

---

## 任务 1：`runtimecfg` 核心类型与原子快照 Holder

**文件：**
- 创建：`internal/runtimecfg/runtimecfg.go`
- 测试：`internal/runtimecfg/runtimecfg_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `internal/runtimecfg/runtimecfg_test.go`：

```go
package runtimecfg

import (
	"testing"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
)

func baseSnapshot() Snapshot {
	return Snapshot{
		Knobs: Knobs{
			MaxMessages: 40, MaxSteps: 16,
			ToolTimeout: 60 * time.Second, CompactionEnabled: true,
			CompactThreshold: 0.8, CompactReserveTokens: 8000, CompactKeepRecent: 8,
		},
		Creds: Credentials{
			OperatorToken: "base-op", AdminToken: "base-adm",
			Operators: []controlplane.Operator{{ID: "alice", Token: "ta"}},
		},
	}
}

func TestNewHolderExposesBaseline(t *testing.T) {
	h := New(baseSnapshot())
	k := h.Knobs()
	if k.MaxSteps != 16 || k.MaxMessages != 40 || k.ToolTimeout != 60*time.Second {
		t.Fatalf("baseline knobs wrong: %+v", k)
	}
	if !k.CompactionEnabled {
		t.Fatal("compaction should default to enabled from baseline")
	}
	c := h.Credentials()
	if c.OperatorToken != "base-op" || c.AdminToken != "base-adm" || len(c.Operators) != 1 {
		t.Fatalf("baseline creds wrong: %+v", c)
	}
}

func TestNilHolderSafe(t *testing.T) {
	var h *Holder
	if h.Knobs() != (Knobs{}) {
		t.Fatal("nil holder Knobs must be zero value")
	}
	if got := h.Credentials(); got.OperatorToken != "" || len(got.Operators) != 0 {
		t.Fatalf("nil holder creds must be zero: %+v", got)
	}
}

func TestMergeSnapshotOverlaysOnlyProvided(t *testing.T) {
	base := baseSnapshot()
	steps := 24
	off := false
	snap := mergeSnapshot(base,
		knobsOverride{MaxSteps: &steps, CompactionEnabled: &off},
		credsOverride{AdminToken: "new-adm"})
	if snap.Knobs.MaxSteps != 24 {
		t.Fatalf("maxsteps override: %d", snap.Knobs.MaxSteps)
	}
	if snap.Knobs.MaxMessages != 40 {
		t.Fatalf("unset knob must keep baseline: %d", snap.Knobs.MaxMessages)
	}
	if snap.Knobs.CompactionEnabled {
		t.Fatal("compaction should be overridden to false")
	}
	if snap.Creds.AdminToken != "new-adm" {
		t.Fatalf("admin override: %q", snap.Creds.AdminToken)
	}
	if snap.Creds.OperatorToken != "base-op" {
		t.Fatalf("unset operator token must keep baseline: %q", snap.Creds.OperatorToken)
	}
	// baseline operator survives; runtime operators appended
	if len(snap.Creds.Operators) != 1 || snap.Creds.Operators[0].ID != "alice" {
		t.Fatalf("base operators must survive: %+v", snap.Creds.Operators)
	}
}

func TestMergeAppendsRuntimeOperators(t *testing.T) {
	base := baseSnapshot()
	snap := mergeSnapshot(base, knobsOverride{}, credsOverride{
		Operators: []operatorEntry{{ID: "bob", Token: "tb"}},
	})
	if len(snap.Creds.Operators) != 2 {
		t.Fatalf("want base+runtime = 2 operators, got %+v", snap.Creds.Operators)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/runtimecfg/`
预期：FAIL / 编译失败（`undefined: Snapshot` 等）。

- [ ] **步骤 3：编写最少实现**

创建 `internal/runtimecfg/runtimecfg.go`：

```go
// Package runtimecfg holds hot-reloadable runtime settings (engine knobs and
// control-plane credentials) behind an atomic snapshot. Reads are lock-free;
// updates serialize on a mutex and atomically swap the snapshot pointer.
//
// A nil *Holder is safe to use and returns zero values; consumers treat that
// as "fall back to YAML/struct defaults".
package runtimecfg

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
)

// Knobs is the effective engine-tuning snapshot.
type Knobs struct {
	MaxMessages          int
	MaxSteps             int
	ToolTimeout          time.Duration
	CompactionEnabled    bool
	CompactThreshold     float64
	CompactReserveTokens int
	CompactKeepRecent    int
}

// Credentials is the effective control-plane credential set.
type Credentials struct {
	OperatorToken string
	AdminToken    string
	Operators     []controlplane.Operator
}

// Snapshot is an immutable view of all hot-reloadable settings.
type Snapshot struct {
	Knobs Knobs
	Creds Credentials
}

// operatorEntry is a runtime-added named operator (id + plaintext token).
type operatorEntry struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

// knobsOverride holds the persisted KV delta for engine knobs. Pointer fields
// are nil when "not overridden" (keep baseline); a non-nil pointer (even to 0
// for compaction off) is an explicit override.
type knobsOverride struct {
	MaxMessages          *int     `json:"max_messages,omitempty"`
	MaxSteps             *int     `json:"max_steps,omitempty"`
	ToolTimeoutSeconds   *int     `json:"tool_timeout_seconds,omitempty"`
	CompactionEnabled    *bool    `json:"compaction_enabled,omitempty"`
	CompactThreshold     *float64 `json:"compact_threshold,omitempty"`
	CompactReserveTokens *int     `json:"compact_reserve_tokens,omitempty"`
	CompactKeepRecent    *int     `json:"compact_keep_recent,omitempty"`
}

// credsOverride holds the persisted KV delta for control-plane credentials.
// Empty OperatorToken/AdminToken means "not overridden" (use config baseline,
// break-glass). Operators are runtime-added (appended to baseline).
type credsOverride struct {
	OperatorToken string          `json:"operator_token,omitempty"`
	AdminToken    string          `json:"admin_token,omitempty"`
	Operators     []operatorEntry `json:"operators,omitempty"`
}

// Holder stores the config baseline plus the current atomic Snapshot. The
// baseline is never mutated at runtime; KV overrides (ko/co) are layered on
// top to produce the effective snapshot.
type Holder struct {
	base Snapshot

	mu  sync.Mutex
	cur atomic.Pointer[Snapshot]

	ko knobsOverride
	co credsOverride
}

// New builds a Holder seeded from the config baseline.
func New(base Snapshot) *Holder {
	h := &Holder{base: base}
	snap := mergeSnapshot(base, knobsOverride{}, credsOverride{})
	h.cur.Store(&snap)
	return h
}

// Snapshot returns the current effective snapshot. Safe on a nil receiver.
func (h *Holder) Snapshot() Snapshot {
	if h == nil {
		return Snapshot{}
	}
	if p := h.cur.Load(); p != nil {
		return *p
	}
	return h.base
}

// Knobs returns the effective engine knobs.
func (h *Holder) Knobs() Knobs { return h.Snapshot().Knobs }

// Credentials returns the effective control-plane credentials.
func (h *Holder) Credentials() Credentials { return h.Snapshot().Creds }

// mergeSnapshot layers override pointers/values on top of the config baseline.
// Numeric knob overrides that are nil keep the baseline; credential token
// overrides that are empty keep the baseline; runtime operators are appended
// to the baseline operators.
func mergeSnapshot(base Snapshot, ko knobsOverride, co credsOverride) Snapshot {
	s := base
	k := s.Knobs
	if ko.MaxMessages != nil {
		k.MaxMessages = *ko.MaxMessages
	}
	if ko.MaxSteps != nil {
		k.MaxSteps = *ko.MaxSteps
	}
	if ko.ToolTimeoutSeconds != nil {
		k.ToolTimeout = time.Duration(*ko.ToolTimeoutSeconds) * time.Second
	}
	if ko.CompactionEnabled != nil {
		k.CompactionEnabled = *ko.CompactionEnabled
	}
	if ko.CompactThreshold != nil {
		k.CompactThreshold = *ko.CompactThreshold
	}
	if ko.CompactReserveTokens != nil {
		k.CompactReserveTokens = *ko.CompactReserveTokens
	}
	if ko.CompactKeepRecent != nil {
		k.CompactKeepRecent = *ko.CompactKeepRecent
	}
	s.Knobs = k

	c := s.Creds
	if co.OperatorToken != "" {
		c.OperatorToken = co.OperatorToken
	}
	if co.AdminToken != "" {
		c.AdminToken = co.AdminToken
	}
	ops := make([]controlplane.Operator, 0, len(base.Creds.Operators)+len(co.Operators))
	ops = append(ops, base.Creds.Operators...)
	for _, e := range co.Operators {
		ops = append(ops, controlplane.Operator{ID: e.ID, Token: e.Token})
	}
	c.Operators = ops
	s.Creds = c
	return s
}

func credentialsConfigured(c Credentials) bool {
	if c.OperatorToken != "" || c.AdminToken != "" {
		return true
	}
	for _, op := range c.Operators {
		if op.Token != "" {
			return true
		}
	}
	return false
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/runtimecfg/`
预期：PASS（4 个测试全绿）。

- [ ] **步骤 5：Commit**

```bash
git add internal/runtimecfg/runtimecfg.go internal/runtimecfg/runtimecfg_test.go
git commit -m "feat(runtimecfg): add atomic snapshot Holder with Knobs/Credentials"
```

---

## 任务 2：KV 持久化、字段校验、PATCH 合并与 TTL 刷新

**文件：**
- 创建：`internal/runtimecfg/persist.go`
- 测试：追加到 `internal/runtimecfg/runtimecfg_test.go`

- [ ] **步骤 1：编写失败的测试**

追加到 `internal/runtimecfg/runtimecfg_test.go`（需 import `"github.com/rebornace/baize/internal/store"`、`"context"`）：

```go
func TestApplyKnobsPatchValidates(t *testing.T) {
	h := New(baseSnapshot())
	cases := []struct {
		name string
		p    KnobsPatch
		ok   bool
	}{
		{"steps high", KnobsPatch{MaxSteps: ptr(101)}, false},
		{"steps low", KnobsPatch{MaxSteps: ptr(0)}, false},
		{"messages high", KnobsPatch{MaxMessages: ptr(501)}, false},
		{"timeout high", KnobsPatch{ToolTimeoutSeconds: ptr(601)}, false},
		{"threshold high", KnobsPatch{CompactThreshold: ptr(0.96)}, false},
		{"threshold low", KnobsPatch{CompactThreshold: ptr(0.09)}, false},
		{"reserve low", KnobsPatch{CompactReserveTokens: ptr(200)}, false},
		{"keeprecent high", KnobsPatch{KeepRecent: ptr(101)}, false},
		{"valid steps", KnobsPatch{MaxSteps: ptr(32)}, true},
		{"valid threshold", KnobsPatch{CompactThreshold: ptr(0.7)}, true},
		{"compaction off", KnobsPatch{CompactionEnabled: ptr(false)}, true},
		{"empty patch", KnobsPatch{}, true},
	}
	for _, tc := range cases {
		err := h.ValidateKnobs(tc.p)
		if tc.ok && err != nil {
			t.Errorf("%s: unexpected err %v", tc.name, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("%s: expected validation error", tc.name)
		}
	}
}

func TestApplyKnobsPatchOverlaysAndReports(t *testing.T) {
	h := New(baseSnapshot())
	off := false
	if err := h.ApplyKnobs(context.Background(), nil, KnobsPatch{MaxSteps: ptr(32), CompactionEnabled: &off}); err != nil {
		t.Fatal(err)
	}
	k := h.Knobs()
	if k.MaxSteps != 32 || k.CompactionEnabled {
		t.Fatalf("overlay not applied: %+v", k)
	}
	if k.MaxMessages != 40 {
		t.Fatalf("unset field must keep baseline: %d", k.MaxMessages)
	}
	ov := h.KnobsOverride()
	if ov.MaxSteps == nil || *ov.MaxSteps != 32 || ov.CompactionEnabled == nil || *ov.CompactionEnabled {
		t.Fatalf("override state wrong: %+v", ov)
	}
}

func TestApplyCredsPatchRotateAndOperators(t *testing.T) {
	h := New(baseSnapshot())
	ctx := context.Background()
	// rotate admin
	if err := h.ApplyCreds(ctx, nil, CredsPatch{AdminToken: "new-adm"}); err != nil {
		t.Fatal(err)
	}
	if h.Credentials().AdminToken != "new-adm" {
		t.Fatal("admin token not rotated")
	}
	// add operator bob
	if err := h.ApplyCreds(ctx, nil, CredsPatch{AddOperators: []OperatorInput{{ID: "bob", Token: "tb"}}}); err != nil {
		t.Fatal(err)
	}
	if len(h.Credentials().Operators) != 2 {
		t.Fatalf("want 2 operators, got %d", len(h.Credentials().Operators))
	}
	// duplicate id -> conflict
	if err := h.ApplyCreds(ctx, nil, CredsPatch{AddOperators: []OperatorInput{{ID: "bob", Token: "x"}}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate operator id must be ErrConflict, got %v", err)
	}
	// cannot remove a config-baseline operator
	if err := h.ApplyCreds(ctx, nil, CredsPatch{RemoveOperators: []string{"alice"}}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("removing config operator must be ErrBadRequest, got %v", err)
	}
	// removing a non-existent runtime operator -> bad request
	if err := h.ApplyCreds(ctx, nil, CredsPatch{RemoveOperators: []string{"nobody"}}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("removing unknown operator must be ErrBadRequest, got %v", err)
	}
	// remove runtime operator bob ok
	if err := h.ApplyCreds(ctx, nil, CredsPatch{RemoveOperators: []string{"bob"}}); err != nil {
		t.Fatalf("remove runtime operator: %v", err)
	}
	if len(h.Credentials().Operators) != 1 {
		t.Fatalf("want 1 operator after remove, got %d", len(h.Credentials().Operators))
	}
}

func TestApplyCredsResetRestoresBaseline(t *testing.T) {
	h := New(baseSnapshot())
	ctx := context.Background()
	if err := h.ApplyCreds(ctx, nil, CredsPatch{AdminToken: "new-adm"}); err != nil {
		t.Fatal(err)
	}
	if err := h.ApplyCreds(ctx, nil, CredsPatch{Reset: true}); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if h.Credentials().AdminToken != "base-adm" {
		t.Fatalf("reset must restore baseline, got %q", h.Credentials().AdminToken)
	}
	// reset together with another field -> bad request
	if err := h.ApplyCreds(ctx, nil, CredsPatch{Reset: true, AdminToken: "x"}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("reset with other fields must be ErrBadRequest, got %v", err)
	}
	// Note: with merge semantics an empty override slot always keeps the config
	// baseline, so a PATCH can never empty a configured gate (break-glass holds);
	// the lockout guard in ApplyCreds is defense-in-depth and is structurally
	// unreachable via the API.
}

func TestLoadFromStoreAppliesOverride(t *testing.T) {
	st := store.NewMemory()
	raw := []byte(`{"knobs":{"max_steps":24},"creds":{"admin_token":"kv-adm"}}`)
	if err := st.UpsertSetting(store.SettingKeyRuntimeSettings, raw); err != nil {
		t.Fatal(err)
	}
	h := New(baseSnapshot())
	if err := h.Load(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	if h.Knobs().MaxSteps != 24 || h.Credentials().AdminToken != "kv-adm" {
		t.Fatalf("KV override not loaded: knobs=%+v", h.Knobs())
	}
	if h.Knobs().MaxMessages != 40 {
		t.Fatalf("unset knob must keep baseline: %d", h.Knobs().MaxMessages)
	}
}

func TestLoadCorruptJSONFallsBack(t *testing.T) {
	st := store.NewMemory()
	_ = st.UpsertSetting(store.SettingKeyRuntimeSettings, []byte(`{not json`))
	h := New(baseSnapshot())
	if err := h.Load(context.Background(), st); err != nil {
		t.Fatalf("corrupt JSON must not error: %v", err)
	}
	if h.Knobs().MaxSteps != 16 {
		t.Fatalf("corrupt KV must fall back to baseline: %d", h.Knobs().MaxSteps)
	}
}

func ptr[T any](v T) *T { return &v }
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/runtimecfg/`
预期：编译失败（`undefined: KnobsPatch`、`ValidateKnobs`、`ApplyKnobs`、`ApplyCreds`、`Load`、`KnobsOverride`、`CredsPatch`、`OperatorInput`、`store.SettingKeyRuntimeSettings`）。

- [ ] **步骤 3：加 store 键常量**

在 `internal/store/store.go` 现有 `SettingKeyInboxChannels`（~129 行）后追加：

```go
// SettingKeyRuntimeSettings is the settings KV key for hot-reloadable runtime
// settings (engine knobs + control-plane credential overrides).
const SettingKeyRuntimeSettings = "runtime_settings"
```

- [ ] **步骤 4：编写 persist.go 实现**

创建 `internal/runtimecfg/persist.go`：

```go
package runtimecfg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/rebornace/baize/internal/store"
)

// KnobsPatch is a partial engine-knob update (nil fields = leave unchanged).
type KnobsPatch struct {
	MaxMessages          *int     `json:"max_messages,omitempty"`
	MaxSteps             *int     `json:"max_steps,omitempty"`
	ToolTimeoutSeconds   *int     `json:"tool_timeout_seconds,omitempty"`
	CompactionEnabled    *bool    `json:"compaction_enabled,omitempty"`
	CompactThreshold     *float64 `json:"compact_threshold,omitempty"`
	CompactReserveTokens *int     `json:"compact_reserve_tokens,omitempty"`
	KeepRecent           *int     `json:"compact_keep_recent,omitempty"`
}

// OperatorInput is one add_operator entry {id, token}.
type OperatorInput struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

// CredsPatch is a partial control-plane credential update.
type CredsPatch struct {
	OperatorToken   string          `json:"operator_token,omitempty"`
	AdminToken      string          `json:"admin_token,omitempty"`
	AddOperators    []OperatorInput `json:"add_operators,omitempty"`
	RemoveOperators []string        `json:"remove_operators,omitempty"`
	Reset           bool            `json:"reset,omitempty"`
}

// persisted is the on-disk KV shape (delta only; absent = baseline).
type persisted struct {
	Knobs knobsOverride `json:"knobs,omitempty"`
	Creds credsOverride `json:"creds,omitempty"`
}

// Exported error sentinels so the API layer can map them to HTTP status codes
// (ErrBadRange/ErrBadRequest -> 400, ErrConflict -> 409) via errors.Is.
var (
	ErrBadRange   = errors.New("value out of allowed range")
	ErrConflict   = errors.New("operator id already exists")
	ErrBadRequest = errors.New("invalid credential patch")
)

// HTTPStatus maps a runtimecfg error to an HTTP status code. Unknown errors 500.
func HTTPStatus(err error) int {
	switch {
	case errors.Is(err, ErrConflict):
		return 409
	case errors.Is(err, ErrBadRange), errors.Is(err, ErrBadRequest):
		return 400
	default:
		return 500
	}
}

// ValidateKnobs checks field-level ranges. Returns a descriptive error.
func (h *Holder) ValidateKnobs(p KnobsPatch) error {
	if p.MaxMessages != nil && (*p.MaxMessages < 1 || *p.MaxMessages > 500) {
		return fmt.Errorf("%w: max_messages must be 1-500", ErrBadRange)
	}
	if p.MaxSteps != nil && (*p.MaxSteps < 1 || *p.MaxSteps > 100) {
		return fmt.Errorf("%w: max_steps must be 1-100", ErrBadRange)
	}
	if p.ToolTimeoutSeconds != nil && (*p.ToolTimeoutSeconds < 1 || *p.ToolTimeoutSeconds > 600) {
		return fmt.Errorf("%w: tool_timeout_seconds must be 1-600", ErrBadRange)
	}
	if p.CompactThreshold != nil && (*p.CompactThreshold < 0.1 || *p.CompactThreshold > 0.95) {
		return fmt.Errorf("%w: compact_threshold must be 0.1-0.95", ErrBadRange)
	}
	if p.CompactReserveTokens != nil && (*p.CompactReserveTokens < 256 || *p.CompactReserveTokens > 100000) {
		return fmt.Errorf("%w: compact_reserve_tokens must be 256-100000", ErrBadRange)
	}
	if p.KeepRecent != nil && (*p.KeepRecent < 0 || *p.KeepRecent > 100) {
		return fmt.Errorf("%w: compact_keep_recent must be 0-100", ErrBadRange)
	}
	return nil
}

// KnobsOverride returns a copy of the current knob delta (for GET effective/overridden).
func (h *Holder) KnobsOverride() knobsOverride {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ko
}

// ApplyKnobs validates, merges, persists, and atomically swaps the snapshot.
func (h *Holder) ApplyKnobs(ctx context.Context, st store.Store, p KnobsPatch) error {
	if err := h.ValidateKnobs(p); err != nil {
		return err
	}
	h.mu.Lock()
	next := h.ko
	if p.MaxMessages != nil {
		next.MaxMessages = p.MaxMessages
	}
	if p.MaxSteps != nil {
		next.MaxSteps = p.MaxSteps
	}
	if p.ToolTimeoutSeconds != nil {
		next.ToolTimeoutSeconds = p.ToolTimeoutSeconds
	}
	if p.CompactionEnabled != nil {
		next.CompactionEnabled = p.CompactionEnabled
	}
	if p.CompactThreshold != nil {
		next.CompactThreshold = p.CompactThreshold
	}
	if p.CompactReserveTokens != nil {
		next.CompactReserveTokens = p.CompactReserveTokens
	}
	if p.KeepRecent != nil {
		next.CompactKeepRecent = p.KeepRecent
	}
	if err := h.persistLocked(ctx, st, next, h.co); err != nil {
		return err
	}
	h.ko = next
	h.swapLocked()
	h.mu.Unlock()
	return nil
}

// ApplyCreds validates, merges, persists, and swaps. allowEmptyGate=false means
// the change must not leave a previously-configured gate with no valid token
// (lockout guard); pass true from the API (baseline always present).
func (h *Holder) ApplyCreds(ctx context.Context, st store.Store, p CredsPatch) error {
	if p.Reset && (p.OperatorToken != "" || p.AdminToken != "" ||
		len(p.AddOperators) > 0 || len(p.RemoveOperators) > 0) {
		return fmt.Errorf("%w: reset cannot combine with other fields", ErrBadRequest)
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	next := h.co
	if p.Reset {
		next = credsOverride{}
	} else {
		if p.OperatorToken != "" {
			next.OperatorToken = p.OperatorToken
		}
		if p.AdminToken != "" {
			next.AdminToken = p.AdminToken
		}
		// remove (runtime only). Rebuild the slice once: any id that is a
		// config-baseline operator is rejected; any id absent from the runtime
		// set is rejected. A fresh slice (not in-place truncation) avoids
		// aliasing the backing array.
		if len(p.RemoveOperators) > 0 {
			removing := map[string]bool{}
			for _, id := range p.RemoveOperators {
				if h.hasBaseOperator(id) {
					return fmt.Errorf("%w: operator %q is from config and cannot be removed", ErrBadRequest, id)
				}
				removing[id] = true
			}
			kept := make([]operatorEntry, 0, len(next.Operators))
			for _, e := range next.Operators {
				if removing[e.ID] {
					continue
				}
				kept = append(kept, e)
			}
			if len(kept) != len(next.Operators)-len(p.RemoveOperators) {
				return fmt.Errorf("%w: one or more operators to remove not found in runtime set", ErrBadRequest)
			}
			next.Operators = kept
		}
		// add (no duplicate against effective set = base + runtime)
		for _, in := range p.AddOperators {
			if in.ID == "" || in.Token == "" {
				return fmt.Errorf("%w: add_operators entries require id and token", ErrBadRequest)
			}
			if h.effectiveHasOperator(next, in.ID) {
				return fmt.Errorf("%w: %q", ErrConflict, in.ID)
			}
			next.Operators = append(next.Operators, operatorEntry{ID: in.ID, Token: in.Token})
		}
	}

	// Lockout guard: if baseline configured a gate, the result must still have
	// at least one valid credential slot.
	effective := mergeSnapshot(h.base, h.ko, next).Creds
	if credentialsConfigured(h.base.Creds) && !credentialsConfigured(effective) {
		return fmt.Errorf("%w: refusing to clear all credentials (would lock out the gate)", ErrBadRequest)
	}

	if err := h.persistLocked(ctx, st, h.ko, next); err != nil {
		return err
	}
	h.co = next
	h.swapLocked()
	return nil
}

func (h *Holder) hasBaseOperator(id string) bool {
	for _, op := range h.base.Creds.Operators {
		if op.ID == id {
			return true
		}
	}
	return false
}

func (h *Holder) effectiveHasOperator(next credsOverride, id string) bool {
	if h.hasBaseOperator(id) {
		return true
	}
	for _, e := range next.Operators {
		if e.ID == id {
			return true
		}
	}
	return false
}

func (h *Holder) swapLocked() {
	snap := mergeSnapshot(h.base, h.ko, h.co)
	h.cur.Store(&snap)
}

// persistLocked writes the merged delta to the KV. Caller holds h.mu.
func (h *Holder) persistLocked(ctx context.Context, st store.Store, ko knobsOverride, co credsOverride) error {
	if st == nil {
		return nil // tests / no-store: swap in-memory only
	}
	raw, err := json.Marshal(persisted{Knobs: ko, Creds: co})
	if err != nil {
		return err
	}
	return st.UpsertSetting(store.SettingKeyRuntimeSettings, raw)
}

// Load reads the KV delta and applies it on top of the baseline. Corrupt JSON
// logs a warning and keeps the baseline (never blocks startup).
func (h *Holder) Load(ctx context.Context, st store.Store) error {
	if st == nil {
		return nil
	}
	raw, ok, err := st.GetSetting(store.SettingKeyRuntimeSettings)
	if err != nil || !ok || len(raw) == 0 {
		return err
	}
	var p persisted
	if err := json.Unmarshal(raw, &p); err != nil {
		log.Printf("runtimecfg: ignoring corrupt runtime_settings KV: %v", err)
		return nil
	}
	h.mu.Lock()
	h.ko = p.Knobs
	h.co = p.Creds
	h.swapLocked()
	h.mu.Unlock()
	return nil
}

// StartRefresh periodically re-reads the KV so changes made on another replica
// propagate. It blocks until ctx is cancelled; run it in a goroutine. Failures
// keep the last good snapshot.
func (h *Holder) StartRefresh(ctx context.Context, st store.Store, interval time.Duration) {
	if h == nil || st == nil || interval <= 0 {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := h.Load(ctx, st); err != nil {
				log.Printf("runtimecfg: refresh failed (keeping last snapshot): %v", err)
			}
		}
	}
}
```

注意：任务 2 测试里 `ApplyKnobs`/`ApplyCreds` 原签名是两参（无 ctx/store）。实现采用三参 `(ctx, st, patch)`。**把测试调用统一改为三参**，例如 `h.ApplyKnobs(context.Background(), nil, KnobsPatch{...})` 与 `h.ApplyCreds(context.Background(), nil, CredsPatch{...})`。`nil` store 表示仅内存换快照（persistLocked 已处理 nil）。

- [ ] **步骤 5：运行测试验证通过**

运行：`go test ./internal/runtimecfg/ ./internal/store/`
预期：PASS。若 `kept := next.Operators[:0]` 在删除时与底层数组别名有顾虑，改为新建切片：

```go
kept := make([]operatorEntry, 0, len(next.Operators))
```

（删除循环内用新建切片，避免原地截断的别名问题。）

- [ ] **步骤 6：Commit**

```bash
git add internal/runtimecfg/persist.go internal/runtimecfg/runtimecfg_test.go internal/store/store.go
git commit -m "feat(runtimecfg): KV persistence, field validation, cred merge, TTL refresh"
```

---

## 任务 3：引擎与压缩器消费热旋钮（`internal/run`）

**文件：**
- 创建：`internal/run/knobs.go`
- 修改：`internal/run/engine.go`（字段 + 三个读取点）
- 修改：`internal/run/compact.go`（`Compactor.Settings` + `effectiveCompaction()` + `MaybeCompact` 用局部值）
- 测试：`internal/run/knobs_test.go`（新建，白盒 `package run`）

- [ ] **步骤 1：编写失败的测试**

创建 `internal/run/knobs_test.go`：

```go
package run

import (
	"testing"
	"time"

	"github.com/rebornace/baize/internal/runtimecfg"
)

type fakeKnobs struct{ k runtimecfg.Knobs }

func (f fakeKnobs) Knobs() runtimecfg.Knobs { return f.k }

func TestEffectiveMaxSteps(t *testing.T) {
	// override wins
	e := &Engine{MaxSteps: 16, Settings: fakeKnobs{k: runtimecfg.Knobs{MaxSteps: 5}}}
	if got := e.effectiveMaxSteps(); got != 5 {
		t.Fatalf("override maxsteps=%d", got)
	}
	// nil settings -> struct field
	e2 := &Engine{MaxSteps: 20}
	if got := e2.effectiveMaxSteps(); got != 20 {
		t.Fatalf("field maxsteps=%d", got)
	}
	// zero everywhere -> default 16
	e3 := &Engine{}
	if got := e3.effectiveMaxSteps(); got != 16 {
		t.Fatalf("default maxsteps=%d", got)
	}
	// snapshot zero (not overridden) falls back to field
	e4 := &Engine{MaxSteps: 20, Settings: fakeKnobs{k: runtimecfg.Knobs{}}}
	if got := e4.effectiveMaxSteps(); got != 20 {
		t.Fatalf("zero snapshot must fall back: %d", got)
	}
}

func TestEffectiveMaxMessages(t *testing.T) {
	e := &Engine{MaxMessages: 40, Settings: fakeKnobs{k: runtimecfg.Knobs{MaxMessages: 12}}}
	if got := e.effectiveMaxMessages(); got != 12 {
		t.Fatalf("override maxmessages=%d", got)
	}
	if got := (&Engine{}).effectiveMaxMessages(); got != 40 {
		t.Fatalf("default maxmessages=%d", got)
	}
}

func TestToolTimeoutHot(t *testing.T) {
	e := &Engine{ToolTimeout: 30 * time.Second, Settings: fakeKnobs{k: runtimecfg.Knobs{ToolTimeout: 5 * time.Second}}}
	if got := e.toolTimeout(); got != 5*time.Second {
		t.Fatalf("override timeout=%v", got)
	}
	e2 := &Engine{ToolTimeout: 30 * time.Second}
	if got := e2.toolTimeout(); got != 30*time.Second {
		t.Fatalf("field timeout=%v", got)
	}
	if got := (&Engine{}).toolTimeout(); got != DefaultToolTimeout {
		t.Fatalf("default timeout=%v", got)
	}
}

func TestCompactorEffectiveCompaction(t *testing.T) {
	// nil settings, zero fields -> enabled + code defaults
	c := &Compactor{}
	en, th, res, keep := c.effectiveCompaction()
	if !en || th != defaultCompactThreshold || res != defaultCompactReserve || keep != defaultCompactKeepRecent {
		t.Fatalf("defaults: en=%v th=%v res=%d keep=%d", en, th, res, keep)
	}
	// explicit disable via snapshot
	c2 := &Compactor{Settings: fakeKnobs{k: runtimecfg.Knobs{CompactionEnabled: false}}}
	if en, _, _, _ := c2.effectiveCompaction(); en {
		t.Fatal("compaction must be disabled by snapshot")
	}
	// threshold/reserve/keep overrides
	c3 := &Compactor{Threshold: 0.8, ReserveTokens: 8000, KeepRecent: 8,
		Settings: fakeKnobs{k: runtimecfg.Knobs{
			CompactionEnabled: true, CompactThreshold: 0.5,
			CompactReserveTokens: 400, CompactKeepRecent: 3}}}
	en, th, res, keep = c3.effectiveCompaction()
	if !en || th != 0.5 || res != 400 || keep != 3 {
		t.Fatalf("overrides: en=%v th=%v res=%d keep=%d", en, th, res, keep)
	}
	// struct fields used when snapshot leaves them zero
	c4 := &Compactor{Threshold: 0.7, ReserveTokens: 1000, KeepRecent: 5,
		Settings: fakeKnobs{k: runtimecfg.Knobs{CompactionEnabled: true}}}
	_, th, res, keep = c4.effectiveCompaction()
	if th != 0.7 || res != 1000 || keep != 5 {
		t.Fatalf("field fallback: th=%v res=%d keep=%d", th, res, keep)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/run/ -run 'Effective|ToolTimeoutHot|CompactorEffective'`
预期：编译失败（`undefined: KnobReader`、`e.effectiveMaxSteps` 等）。

- [ ] **步骤 3：创建 `internal/run/knobs.go`**

```go
package run

import "github.com/rebornace/baize/internal/runtimecfg"

// KnobReader provides the hot-reloadable engine knobs snapshot.
// *runtimecfg.Holder satisfies this interface; nil means "use YAML/struct
// defaults" (legacy behavior, zero changes to existing tests).
type KnobReader interface {
	Knobs() runtimecfg.Knobs
}

func (e *Engine) effectiveMaxSteps() int {
	if e.Settings != nil {
		if n := e.Settings.Knobs().MaxSteps; n > 0 {
			return n
		}
	}
	if e.MaxSteps > 0 {
		return e.MaxSteps
	}
	return 16
}

func (e *Engine) effectiveMaxMessages() int {
	if e.Settings != nil {
		if n := e.Settings.Knobs().MaxMessages; n > 0 {
			return n
		}
	}
	if e.MaxMessages > 0 {
		return e.MaxMessages
	}
	return 40
}
```

- [ ] **步骤 4：改 `internal/run/engine.go`**

在 `Engine` 结构体（`ImagePartResolver` 字段后，~79 行）加字段：

```go
	// Settings optionally supplies hot-reloadable engine knobs. nil = use the
	// struct fields above (YAML defaults); non-nil overrides per-field.
	Settings KnobReader
```

改 `toolTimeout()`（~218 行）为：

```go
func (e *Engine) toolTimeout() time.Duration {
	if e.Settings != nil {
		if d := e.Settings.Knobs().ToolTimeout; d > 0 {
			return d
		}
	}
	if e.ToolTimeout > 0 {
		return e.ToolTimeout
	}
	return DefaultToolTimeout
}
```

改 `buildMessages` 历史窗口（~243 行）：

```go
		hist := e.Messages.ListWindow(conversationID, e.effectiveMaxMessages())
```

改 `runLoop` 步数上界（~533-535 行），把：

```go
	maxSteps := e.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 16
	}
```

替换为：

```go
	// Read once at loop start so the step bound is stable for the whole run
	// even if an operator hot-patches knobs mid-run.
	maxSteps := e.effectiveMaxSteps()
```

- [ ] **步骤 5：改 `internal/run/compact.go`**

`Compactor` 结构体加字段（`KeepRecent` 后）：

```go
	// Settings optionally supplies hot-reloadable knobs (compaction switch +
	// thresholds). nil = use the struct fields (YAML defaults). Read inside
	// MaybeCompact rather than mutating shared fields, to avoid races across
	// concurrent runs.
	Settings KnobReader
```

新增方法（放在 `normalize()` 前）：

```go
// effectiveCompaction resolves the hot-reloadable compaction switch and
// thresholds, layering snapshot overrides over struct fields over code
// defaults. enabled=false means compaction is turned off for this run.
func (c *Compactor) effectiveCompaction() (enabled bool, threshold float64, reserve, keep int) {
	threshold, reserve, keep = c.Threshold, c.ReserveTokens, c.KeepRecent
	enabled = true
	if c.Settings != nil {
		k := c.Settings.Knobs()
		if !k.CompactionEnabled {
			enabled = false
		}
		if k.CompactThreshold > 0 {
			threshold = k.CompactThreshold
		}
		if k.CompactReserveTokens > 0 {
			reserve = k.CompactReserveTokens
		}
		if k.CompactKeepRecent > 0 {
			keep = k.CompactKeepRecent
		}
	}
	if threshold <= 0 {
		threshold = defaultCompactThreshold
	}
	if reserve <= 0 {
		reserve = defaultCompactReserve
	}
	if keep <= 0 {
		keep = defaultCompactKeepRecent
	}
	return enabled, threshold, reserve, keep
}
```

改 `MaybeCompact`：删除 `c.normalize()` 调用（~64 行），替换为局部求值，并把后续对 `c.Threshold/c.ReserveTokens/c.KeepRecent` 的引用改为局部变量。把：

```go
	if c == nil || c.Messages == nil || c.LLM == nil || c.Profiles == nil || convID == "" {
		return false, nil
	}
	c.normalize()
```

改为：

```go
	if c == nil || c.Messages == nil || c.LLM == nil || c.Profiles == nil || convID == "" {
		return false, nil
	}
	enabled, threshold, reserve, keep := c.effectiveCompaction()
	if !enabled {
		return false, nil // compaction switched off at runtime
	}
	summaryTimeout := c.SummaryTimeout
	if summaryTimeout <= 0 {
		summaryTimeout = defaultCompactSummaryWait
	}
```

然后把 budget/keepStart 两行改为用局部值：

```go
	budget := int(float64(view.ContextTokens)*threshold) - reserve
```

```go
	keepStart := len(full) - keep
```

把 summarize 调用改为传入超时：

```go
	newSummary, err := c.summarize(ctx, summaryTimeout, existing.Summary, newFold)
```

并把 `summarize` 签名与内部超时改为：

```go
func (c *Compactor) summarize(ctx context.Context, timeout time.Duration, prior string, fold []conversation.Message) (string, error) {
```

```go
	sumCtx, cancel := context.WithTimeout(context.Background(), timeout)
```

`normalize()` 若不再被引用则删除（避免死代码；确认无测试直接调用后删除）。

- [ ] **步骤 6：运行测试验证通过**

运行：`go test ./internal/run/`
预期：PASS（含既有 compact/engine 测试 + 新 knobs 测试）。若有测试直接调用 `c.normalize()`，改为通过 `effectiveCompaction()` 断言。

- [ ] **步骤 7：Commit**

```bash
git add internal/run/knobs.go internal/run/knobs_test.go internal/run/engine.go internal/run/compact.go
git commit -m "feat(run): engine and compactor consume hot-reloadable knobs"
```

---

## 任务 4：API 层接入 Holder——`gateTokens`、ACL、路由注册

**文件：**
- 修改：`internal/api/server.go`（`Server.Settings` 字段 + `gateTokens()` + `routes()` 注册）
- 修改：`internal/controlplane/acl.go`（4 条路由规则）
- 修改：`internal/controlplane/acl_test.go`（断言）
- 测试：追加到 `internal/api/server_gate_test.go`

- [ ] **步骤 1：编写失败的测试**

追加到 `internal/api/server_gate_test.go`（该文件已是 `package api_test`；确保 import 含 `context`、`time`、`github.com/rebornace/baize/internal/controlplane`、`github.com/rebornace/baize/internal/runtimecfg`——缺哪个补哪个）：

```go
func TestGateTokensUsesRuntimeHolder(t *testing.T) {
	st, err := store.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.OperatorToken = "op"
	srv.AdminToken = "adm"

	// baseline from struct fields
	tok := srv.GateTokensForTest()
	if tok.Admin != "adm" || tok.Operator != "op" {
		t.Fatalf("baseline tokens wrong: %+v", tok)
	}

	// attach a holder with an overridden admin token -> gate uses it immediately
	base := runtimecfg.Snapshot{Creds: runtimecfg.Credentials{
		OperatorToken: "op", AdminToken: "adm",
		Operators:     []controlplane.Operator{{ID: "alice", Token: "ta"}},
	}}
	h := runtimecfg.New(base)
	if err := h.ApplyCreds(context.Background(), nil, runtimecfg.CredsPatch{AdminToken: "rotated"}); err != nil {
		t.Fatal(err)
	}
	srv.Settings = h
	tok = srv.GateTokensForTest()
	if tok.Admin != "rotated" {
		t.Fatalf("rotated admin not used: %q", tok.Admin)
	}
	if tok.Operator != "op" || len(tok.Operators) != 1 {
		t.Fatalf("operator/operators must come from snapshot: %+v", tok)
	}

	// reset -> back to baseline
	if err := h.ApplyCreds(context.Background(), nil, runtimecfg.CredsPatch{Reset: true}); err != nil {
		t.Fatal(err)
	}
	if srv.GateTokensForTest().Admin != "adm" {
		t.Fatalf("reset must restore baseline admin")
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/api/ -run GateTokensUsesRuntimeHolder`
预期：编译失败（`srv.Settings` undefined、`GateTokensForTest` undefined）。

- [ ] **步骤 3：改 `internal/api/server.go`**

在 `Server` 结构体凭据字段区（`Operators []controlplane.Operator` ~106 行后）加：

```go
	// Settings optionally supplies hot-reloadable engine knobs and control-plane
	// credentials. nil = use the static OperatorToken/AdminToken/Operators
	// fields above (legacy behavior; existing tests leave it nil).
	Settings *runtimecfg.Holder
```

并在 import 块加 `"github.com/rebornace/baize/internal/runtimecfg"`。

把 `gateTokens()`（~223 行）改为：

```go
func (s *Server) gateTokens() controlplane.Tokens {
	if s.Settings != nil {
		c := s.Settings.Credentials()
		return controlplane.Tokens{
			Operator:  c.OperatorToken,
			Admin:     c.AdminToken,
			Operators: c.Operators,
		}
	}
	return controlplane.Tokens{
		Operator:  s.OperatorToken,
		Admin:     s.AdminToken,
		Operators: s.Operators,
	}
}

// GateTokensForTest exposes the effective gate tokens for tests.
func (s *Server) GateTokensForTest() controlplane.Tokens { return s.gateTokens() }
```

在 `routes()` 里微信路由（~391 行 `s.mux.HandleFunc("PUT /v0/settings/channels/weixin", ...)`）后注册：

```go
	s.mux.HandleFunc("GET /v0/settings/runtime", s.handleGetRuntimeSettings)
	s.mux.HandleFunc("PATCH /v0/settings/runtime", s.handlePatchRuntimeSettings)
	s.mux.HandleFunc("GET /v0/settings/credentials", s.handleGetCredentials)
	s.mux.HandleFunc("PATCH /v0/settings/credentials", s.handlePatchCredentials)
```

- [ ] **步骤 4：改 ACL `internal/controlplane/acl.go`**

在 settings 路由表区（mcp-export 规则后、models 规则前，~67 行）加：

```go
	{method: "GET", segments: []string{"v0", "settings", "runtime"}, role: RoleOperator},
	{method: "PATCH", segments: []string{"v0", "settings", "runtime"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "credentials"}, role: RoleAdmin},
	{method: "PATCH", segments: []string{"v0", "settings", "credentials"}, role: RoleAdmin},
```

在 `internal/controlplane/acl_test.go` 的表中加断言：

```go
		{"GET", "/v0/settings/runtime", RoleOperator},
		{"PATCH", "/v0/settings/runtime", RoleAdmin},
		{"GET", "/v0/settings/credentials", RoleAdmin},
		{"PATCH", "/v0/settings/credentials", RoleAdmin},
```

- [ ] **步骤 5：创建 handler 文件（501 桩，任务 6 替换为真实实现）**

创建 `internal/api/server_settings_runtime.go`，先放四个返回 501 的桩 handler，使任务 4 可编译、路由可注册：

```go
package api

import "net/http"

func (s *Server) handleGetRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "pending task 5")
}
func (s *Server) handlePatchRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "pending task 5")
}
func (s *Server) handleGetCredentials(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "pending task 5")
}
func (s *Server) handlePatchCredentials(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "pending task 5")
}
```

- [ ] **步骤 6：运行测试验证通过**

运行：`go test ./internal/api/ ./internal/controlplane/ -run 'GateTokensUsesRuntimeHolder|TestMinRole|ACL'`
预期：PASS（gate token 热切换 + ACL 断言绿）。

- [ ] **步骤 7：Commit**

```bash
git add internal/api/server.go internal/api/server_settings_runtime.go internal/api/server_gate_test.go internal/controlplane/acl.go internal/controlplane/acl_test.go
git commit -m "feat(api): gate tokens read from runtime holder; register settings routes + ACL"
```

---

## 任务 5：运行时设置与凭据端点 handler

**文件：**
- 修改：`internal/runtimecfg/runtimecfg.go`（加 `CredentialsView()` 脱敏视图 + `KnobsView()`）
- 修改：`internal/api/server_settings_runtime.go`（把任务 4 的四个桩替换为真实 handler）
- 测试：`internal/api/server_settings_runtime_test.go`（新建）
- 测试：追加 `internal/runtimecfg/runtimecfg_test.go`（视图测试）

- [ ] **步骤 1：编写失败的测试（runtimecfg 视图）**

追加到 `internal/runtimecfg/runtimecfg_test.go`：

```go
func TestCredentialsViewMasksTokens(t *testing.T) {
	h := New(baseSnapshot())
	if err := h.ApplyCreds(context.Background(), nil, CredsPatch{
		AdminToken:   "rotated-adm",
		AddOperators: []OperatorInput{{ID: "bob", Token: "tb"}},
	}); err != nil {
		t.Fatal(err)
	}
	v := h.CredentialsView()
	if v.Source != "override" {
		t.Fatalf("source=%s want override", v.Source)
	}
	if !v.OperatorSet || !v.AdminSet {
		t.Fatalf("slots should be set: %+v", v)
	}
	// alice from config (baseline), bob from runtime
	src := map[string]string{}
	for _, op := range v.Operators {
		src[op.ID] = op.Source
	}
	if src["alice"] != "config" || src["bob"] != "runtime" {
		t.Fatalf("operator sources wrong: %+v", v.Operators)
	}
	// the view must never carry a token: marshal and assert no secret substring
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "rotated-adm") || strings.Contains(string(b), "tb") || strings.Contains(string(b), "ta") {
		t.Fatalf("credentials view leaked a token: %s", b)
	}
}

func TestKnobsViewMarksOverridden(t *testing.T) {
	h := New(baseSnapshot())
	_ = h.ApplyKnobs(context.Background(), nil, KnobsPatch{MaxSteps: ptr(32)})
	v := h.KnobsView()
	if v.Effective.MaxSteps != 32 || !v.Overridden.MaxSteps {
		t.Fatalf("maxsteps should be effective+overridden: %+v", v)
	}
	if v.Overridden.MaxMessages || v.Effective.MaxMessages != 40 {
		t.Fatalf("maxmessages should be baseline not overridden: %+v", v)
	}
}
```

（测试文件需 import `"encoding/json"`、`"strings"`。）

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/runtimecfg/ -run 'CredentialsView|KnobsView'`
预期：编译失败（`h.CredentialsView`/`h.KnobsView` undefined）。

- [ ] **步骤 3：实现 runtimecfg 视图方法**

在 `internal/runtimecfg/runtimecfg.go` 加（`KnobsView`/`CredsView` 类型与方法）：

```go
// KnobsFieldFlags reports which knobs are overridden in the KV.
type KnobsFieldFlags struct {
	MaxMessages, MaxSteps, ToolTimeout, CompactionEnabled,
	CompactThreshold, CompactReserveTokens, CompactKeepRecent bool
}

// KnobsView is the GET /settings/runtime body: effective values + override flags.
type KnobsView struct {
	Effective  Knobs          `json:"effective"`
	Overridden KnobsFieldFlags `json:"overridden"`
}

func (h *Holder) KnobsView() KnobsView {
	ko := h.KnobsOverride()
	return KnobsView{
		Effective: h.Knobs(),
		Overridden: KnobsFieldFlags{
			MaxMessages:          ko.MaxMessages != nil,
			MaxSteps:             ko.MaxSteps != nil,
			ToolTimeout:          ko.ToolTimeoutSeconds != nil,
			CompactionEnabled:    ko.CompactionEnabled != nil,
			CompactThreshold:     ko.CompactThreshold != nil,
			CompactReserveTokens: ko.CompactReserveTokens != nil,
			CompactKeepRecent:    ko.CompactKeepRecent != nil,
		},
	}
}

// OperatorView is a masked operator entry (id + source, never the token).
type OperatorView struct {
	ID     string `json:"id"`
	Source string `json:"source"` // "config" | "runtime"
}

// CredsView is the GET /settings/credentials body (no tokens, ever).
type CredsView struct {
	Source      string         `json:"source"` // "config" | "override"
	OperatorSet bool           `json:"operator_set"`
	AdminSet    bool           `json:"admin_set"`
	Operators   []OperatorView `json:"operators"`
}

func (h *Holder) CredentialsView() CredsView {
	c := h.Credentials()
	h.mu.Lock()
	hasOverride := h.co.OperatorToken != "" || h.co.AdminToken != "" || len(h.co.Operators) > 0
	runtimeOps := map[string]bool{}
	for _, e := range h.co.Operators {
		runtimeOps[e.ID] = true
	}
	h.mu.Unlock()

	ops := make([]OperatorView, 0, len(c.Operators))
	for _, op := range c.Operators {
		src := "config"
		if runtimeOps[op.ID] {
			src = "runtime"
		}
		ops = append(ops, OperatorView{ID: op.ID, Source: src})
	}
	source := "config"
	if hasOverride {
		source = "override"
	}
	return CredsView{
		Source:      source,
		OperatorSet: c.OperatorToken != "",
		AdminSet:    c.AdminToken != "",
		Operators:   ops,
	}
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/runtimecfg/`
预期：PASS。

- [ ] **步骤 5：Commit（视图部分）**

```bash
git add internal/runtimecfg/runtimecfg.go internal/runtimecfg/runtimecfg_test.go
git commit -m "feat(runtimecfg): add masked credentials view and knobs effective/override view"
```

---

## 任务 6：四个设置端点的 HTTP handler

**文件：**
- 修改：`internal/api/server_settings_runtime.go`（把任务 4 的四个桩替换为真实 handler）
- 测试：`internal/api/server_settings_runtime_test.go`（新建，`package api_test`）

- [ ] **步骤 1：编写失败的测试**

创建 `internal/api/server_settings_runtime_test.go`：

```go
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func runtimeSettingsServer(t *testing.T) (*api.Server, store.Store) {
	t.Helper()
	st := store.NewMemory()
	srv := api.NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	base := runtimecfg.Snapshot{
		Knobs: runtimecfg.Knobs{MaxMessages: 40, MaxSteps: 16, CompactionEnabled: true, CompactThreshold: 0.8},
		Creds: runtimecfg.Credentials{
			OperatorToken: "op", AdminToken: "adm",
			Operators: []controlplane.Operator{{ID: "alice", Token: "ta"}},
		},
	}
	h := runtimecfg.New(base)
	if err := h.Load(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	srv.Settings = h
	return srv, st
}

func doJSON(t *testing.T, srv *api.Server, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	return rr
}

func TestGetRuntimeSettings(t *testing.T) {
	srv, _ := runtimeSettingsServer(t)
	rr := doJSON(t, srv, http.MethodGet, "/v0/settings/runtime", "op", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"max_steps":16`) {
		t.Fatalf("expected effective max_steps, body=%s", rr.Body.String())
	}
}

func TestPatchRuntimeSettingsHot(t *testing.T) {
	srv, st := runtimeSettingsServer(t)
	rr := doJSON(t, srv, http.MethodPatch, "/v0/settings/runtime", "adm",
		map[string]any{"max_steps": 32, "compaction_enabled": false})
	if rr.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", rr.Code, rr.Body.String())
	}
	if srv.Settings.Knobs().MaxSteps != 32 || srv.Settings.Knobs().CompactionEnabled {
		t.Fatalf("knobs not applied: %+v", srv.Settings.Knobs())
	}
	// Persists across a fresh holder over the same store (simulates restart /
	// another replica loading the KV).
	h2 := runtimecfg.New(runtimecfg.Snapshot{Knobs: runtimecfg.Knobs{MaxSteps: 16, CompactionEnabled: true}})
	if err := h2.Load(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	if h2.Knobs().MaxSteps != 32 || h2.Knobs().CompactionEnabled {
		t.Fatalf("override must survive reload: %+v", h2.Knobs())
	}
}

func TestPatchRuntimeSettingsInvalid(t *testing.T) {
	srv, _ := runtimeSettingsServer(t)
	rr := doJSON(t, srv, http.MethodPatch, "/v0/settings/runtime", "adm",
		map[string]any{"max_steps": 999})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestPatchRuntimeSettingsForbiddenForOperator(t *testing.T) {
	srv, _ := runtimeSettingsServer(t)
	rr := doJSON(t, srv, http.MethodPatch, "/v0/settings/runtime", "op",
		map[string]any{"max_steps": 32})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestCredentialsGetMasks(t *testing.T) {
	srv, _ := runtimeSettingsServer(t)
	rr := doJSON(t, srv, http.MethodGet, "/v0/settings/credentials", "adm", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, secret := range []string{"\"adm\"", "\"op\"", "\"ta\""} {
		if strings.Contains(body, secret) {
			t.Fatalf("credentials leaked %s: %s", secret, body)
		}
	}
	if !strings.Contains(body, `"source":"config"`) {
		t.Fatalf("expected source config, body=%s", body)
	}
}

func TestCredentialsRotateAddConflictRemoveReset(t *testing.T) {
	srv, _ := runtimeSettingsServer(t)
	// rotate admin
	rr := doJSON(t, srv, http.MethodPatch, "/v0/settings/credentials", "adm",
		map[string]any{"admin_token": "new-adm"})
	if rr.Code != http.StatusOK {
		t.Fatalf("rotate status=%d %s", rr.Code, rr.Body.String())
	}
	// new admin token works immediately
	rr2 := doJSON(t, srv, http.MethodGet, "/v0/settings/credentials", "new-adm", nil)
	if rr2.Code != http.StatusOK {
		t.Fatalf("rotated admin not effective, status=%d", rr2.Code)
	}
	// add operator bob
	rr = doJSON(t, srv, http.MethodPatch, "/v0/settings/credentials", "new-adm",
		map[string]any{"add_operators": []map[string]any{{"id": "bob", "token": "tb"}}})
	if rr.Code != http.StatusOK {
		t.Fatalf("add op status=%d %s", rr.Code, rr.Body.String())
	}
	// duplicate -> 409
	rr = doJSON(t, srv, http.MethodPatch, "/v0/settings/credentials", "new-adm",
		map[string]any{"add_operators": []map[string]any{{"id": "bob", "token": "x"}}})
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rr.Code)
	}
	// remove config operator alice -> 400
	rr = doJSON(t, srv, http.MethodPatch, "/v0/settings/credentials", "new-adm",
		map[string]any{"remove_operators": []string{"alice"}})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 removing config op, got %d", rr.Code)
	}
	// reset -> baseline admin works again
	rr = doJSON(t, srv, http.MethodPatch, "/v0/settings/credentials", "new-adm",
		map[string]any{"reset": true})
	if rr.Code != http.StatusOK {
		t.Fatalf("reset status=%d %s", rr.Code, rr.Body.String())
	}
	rr3 := doJSON(t, srv, http.MethodGet, "/v0/settings/credentials", "adm", nil)
	if rr3.Code != http.StatusOK {
		t.Fatalf("baseline admin not restored, status=%d", rr3.Code)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/api/ -run 'RuntimeSettings|Credentials'`
预期：handler 是 501 桩 → 断言失败。

- [ ] **步骤 3：实现 handler**

把 `internal/api/server_settings_runtime.go` 整体替换为：

```go
package api

import (
	"encoding/json"
	"net/http"

	"github.com/rebornace/baize/internal/runtimecfg"
)

// runtimeKnobsJSON is the wire shape for engine knobs (duration as seconds).
type runtimeKnobsJSON struct {
	MaxMessages          int     `json:"max_messages"`
	MaxSteps             int     `json:"max_steps"`
	ToolTimeoutSeconds   int     `json:"tool_timeout_seconds"`
	CompactionEnabled    bool    `json:"compaction_enabled"`
	CompactThreshold     float64 `json:"compact_threshold"`
	CompactReserveTokens int     `json:"compact_reserve_tokens"`
	CompactKeepRecent    int     `json:"compact_keep_recent"`
}

func knobsToJSON(k runtimecfg.Knobs) runtimeKnobsJSON {
	return runtimeKnobsJSON{
		MaxMessages:          k.MaxMessages,
		MaxSteps:             k.MaxSteps,
		ToolTimeoutSeconds:   int(k.ToolTimeout.Seconds()),
		CompactionEnabled:    k.CompactionEnabled,
		CompactThreshold:     k.CompactThreshold,
		CompactReserveTokens: k.CompactReserveTokens,
		CompactKeepRecent:    k.CompactKeepRecent,
	}
}

func (s *Server) handleGetRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	if s.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime_settings_unavailable", "runtime settings not wired")
		return
	}
	view := s.Settings.KnobsView()
	writeJSON(w, http.StatusOK, map[string]any{
		"effective":  knobsToJSON(view.Effective),
		"overridden": view.Overridden,
	})
}

func (s *Server) handlePatchRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	if s.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime_settings_unavailable", "runtime settings not wired")
		return
	}
	var patch runtimecfg.KnobsPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := s.Settings.ApplyKnobs(r.Context(), s.Store, patch); err != nil {
		writeError(w, runtimecfg.HTTPStatus(err), "invalid_settings", err.Error())
		return
	}
	view := s.Settings.KnobsView()
	writeJSON(w, http.StatusOK, map[string]any{
		"effective":  knobsToJSON(view.Effective),
		"overridden": view.Overridden,
	})
}

func (s *Server) handleGetCredentials(w http.ResponseWriter, r *http.Request) {
	if s.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime_settings_unavailable", "runtime settings not wired")
		return
	}
	writeJSON(w, http.StatusOK, s.Settings.CredentialsView())
}

func (s *Server) handlePatchCredentials(w http.ResponseWriter, r *http.Request) {
	if s.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime_settings_unavailable", "runtime settings not wired")
		return
	}
	var patch runtimecfg.CredsPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := s.Settings.ApplyCreds(r.Context(), s.Store, patch); err != nil {
		writeError(w, runtimecfg.HTTPStatus(err), "invalid_credentials", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.Settings.CredentialsView())
}
```

注意：`KnobsView.Overridden` 是结构体 `KnobsFieldFlags`，`writeJSON` 会序列化为 `{max_messages:bool,...}`；若希望 wire 键名与旋钮一致，给 `KnobsFieldFlags` 字段加 json tag。在 `internal/runtimecfg/runtimecfg.go` 的 `KnobsFieldFlags` 上补：

```go
type KnobsFieldFlags struct {
	MaxMessages          bool `json:"max_messages"`
	MaxSteps             bool `json:"max_steps"`
	ToolTimeout          bool `json:"tool_timeout_seconds"`
	CompactionEnabled    bool `json:"compaction_enabled"`
	CompactThreshold     bool `json:"compact_threshold"`
	CompactReserveTokens bool `json:"compact_reserve_tokens"`
	CompactKeepRecent    bool `json:"compact_keep_recent"`
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/api/ -run 'RuntimeSettings|Credentials'`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/api/server_settings_runtime.go internal/api/server_settings_runtime_test.go internal/runtimecfg/runtimecfg.go
git commit -m "feat(api): runtime settings + credentials endpoints (hot patch, masked get)"
```

---

## 任务 7：微信渠道 Enabled 热启停

**文件：**
- 修改：`internal/channel/weixin/channel.go`（加 `HasCredentials()`）
- 修改：`internal/api/server_channel_weixin.go`（`applyWeixinSettings` 调和启停 + PUT 响应加 `running`）
- 测试：追加 `internal/api/server_channel_weixin_test.go`

- [ ] **步骤 1：编写失败的测试**

追加到 `internal/api/server_channel_weixin_test.go`：

```go
func TestWeixinEnabledStartWithCreds(t *testing.T) {
	srv, _, _ := weixinTestServer(t)
	// simulate a logged-in channel (creds present in memory)
	srv.WeixinChannel.SetCredentials("acct-1", "tok-1")

	putBody := jsonBody(t, map[string]any{"enabled": true, "assignee": "bob"})
	req := httptest.NewRequest(http.MethodPut, "/v0/settings/channels/weixin", putBody)
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Enabled bool   `json:"enabled"`
		Running bool   `json:"running"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Enabled || !resp.Running || resp.Reason != "" {
		t.Fatalf("expected enabled+running, got %+v", resp)
	}
	if !srv.WeixinChannel.IsStarted() {
		t.Fatal("channel should be started after enable with creds")
	}
}

func TestWeixinEnabledNoCredsReportsLoginRequired(t *testing.T) {
	srv, _, _ := weixinTestServer(t) // channel has no creds

	putBody := jsonBody(t, map[string]any{"enabled": true})
	req := httptest.NewRequest(http.MethodPut, "/v0/settings/channels/weixin", putBody)
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Running bool   `json:"running"`
		Reason  string `json:"reason"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Running || resp.Reason != "login_required" {
		t.Fatalf("expected running:false login_required, got %+v", resp)
	}
	if srv.WeixinChannel.IsStarted() {
		t.Fatal("channel must not start without creds")
	}
}

func TestWeixinDisableStopsButKeepsCreds(t *testing.T) {
	srv, _, _ := weixinTestServer(t)
	srv.WeixinChannel.SetCredentials("acct-1", "tok-1")
	// enable first -> running
	enable := jsonBody(t, map[string]any{"enabled": true})
	r1 := httptest.NewRequest(http.MethodPut, "/v0/settings/channels/weixin", enable)
	r1.Header.Set("Authorization", "Bearer adm")
	r1.Header.Set("Content-Type", "application/json")
	rr1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr1, r1)
	if !srv.WeixinChannel.IsStarted() {
		t.Fatal("setup: channel should be running")
	}

	// disable -> stopped, but creds retained
	disable := jsonBody(t, map[string]any{"enabled": false})
	r2 := httptest.NewRequest(http.MethodPut, "/v0/settings/channels/weixin", disable)
	r2.Header.Set("Authorization", "Bearer adm")
	r2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, r2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("disable status=%d %s", rr2.Code, rr2.Body.String())
	}
	if srv.WeixinChannel.IsStarted() {
		t.Fatal("channel should be stopped after disable")
	}
	if !srv.WeixinChannel.HasCredentials() {
		t.Fatal("disable must NOT clear credentials (only logout does)")
	}

	// re-enable -> starts again directly (creds still present)
	r3 := httptest.NewRequest(http.MethodPut, "/v0/settings/channels/weixin", enable)
	r3.Header.Set("Authorization", "Bearer adm")
	r3.Header.Set("Content-Type", "application/json")
	rr3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr3, r3)
	if !srv.WeixinChannel.IsStarted() {
		t.Fatal("re-enable should restart using retained creds")
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/api/ -run 'WeixinEnabled|WeixinDisable'`
预期：编译失败（`HasCredentials` undefined）或断言失败（响应无 `running`）。

- [ ] **步骤 3：给 Channel 加 `HasCredentials`**

在 `internal/channel/weixin/channel.go` 的 `IsStarted()`（~81 行）后加：

```go
// HasCredentials reports whether account/token are set in memory.
func (c *Channel) HasCredentials() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.TrimSpace(c.token) != "" && strings.TrimSpace(c.accountID) != ""
}
```

- [ ] **步骤 4：改 `applyWeixinSettings` 与 PUT handler**

在 `internal/api/server_channel_weixin.go`，把 `applyWeixinSettings`（~149 行）替换为返回运行态并按 `Enabled` 调和：

```go
// weixinPutResponse is the saved settings plus the reconciled runtime state.
type weixinPutResponse struct {
	WeixinChannelSettings
	Running bool   `json:"running"`
	Reason  string `json:"reason,omitempty"` // "login_required" | "start_failed"
}

func (s *Server) applyWeixinSettings(settings WeixinChannelSettings) (running bool, reason string) {
	s.weixinMu.Lock()
	defer s.weixinMu.Unlock()
	if s.WeixinRuntime != nil {
		if id := strings.TrimSpace(settings.Assignee); id != "" {
			s.WeixinRuntime.Assignee = id
		}
		if id := strings.TrimSpace(settings.AgentID); id != "" {
			s.WeixinRuntime.DefaultAgentID = id
		}
	}
	ch := s.WeixinChannel
	if ch == nil {
		return false, ""
	}
	if settings.Enabled {
		if ch.IsStarted() {
			return true, ""
		}
		if !ch.HasCredentials() {
			return false, "login_required" // login success auto-starts when Enabled
		}
		if err := ch.Start(s.weixinRunCtx()); err != nil {
			return false, "start_failed"
		}
		return ch.IsStarted(), ""
	}
	// Disabled: stop the poll loop but KEEP credentials (unlike logout), so a
	// re-enable can Start directly without a new QR login.
	if ch.IsStarted() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = ch.Stop(stopCtx)
	}
	return false, ""
}
```

把 `handlePutWeixinSettings` 末尾的：

```go
	s.applyWeixinSettings(body)
	writeJSON(w, http.StatusOK, body)
```

改为：

```go
	running, reason := s.applyWeixinSettings(body)
	writeJSON(w, http.StatusOK, weixinPutResponse{
		WeixinChannelSettings: body,
		Running:               running,
		Reason:                reason,
	})
```

`context` 已在该文件 import；`time` **未** import——需在 import 块加 `"time"`（Stop 的 5s 超时 `time.Second` 用到）。

- [ ] **步骤 5：运行测试验证通过**

运行：`go test ./internal/api/ ./internal/channel/weixin/ -run Weixin`
预期：PASS（既有 `TestWeixinSettingsGetPut` 只对 PUT 检查状态码、对 GET 解码 settings，GET 响应不变，故不受影响）。

- [ ] **步骤 6：Commit**

```bash
git add internal/channel/weixin/channel.go internal/api/server_channel_weixin.go internal/api/server_channel_weixin_test.go
git commit -m "feat(weixin): hot apply enabled flag (Start/Stop, keep creds on disable)"
```

---

## 任务 8：bootstrap 装配 Holder 并启动 TTL 刷新

**文件：**
- 创建：`internal/bootstrap/runtime_settings.go`
- 修改：`internal/bootstrap/bootstrap.go`（构造 holder、注入、起刷新 goroutine；compactor 构造条件放宽）
- 测试：追加 `internal/bootstrap/control_plane_test.go`（或新建 `runtime_settings_test.go`）冒烟断言

- [ ] **步骤 1：编写失败的测试**

创建 `internal/bootstrap/runtime_settings_test.go`（`package bootstrap`）：

```go
package bootstrap

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
)

func TestBuildRuntimeHolderBaselineAndOverride(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{}
	cfg.Run.MaxSteps = 16
	cfg.Run.ToolTimeoutSec = 60
	cfg.Conversation.MaxMessages = 40
	cfg.Conversation.CompactThreshold = 0.8

	ops := []controlplane.Operator{{ID: "alice", Token: "ta"}}
	h := buildRuntimeHolder(cfg, st, "op", "adm", ops)
	if h.Knobs().MaxSteps != 16 || h.Credentials().AdminToken != "adm" {
		t.Fatalf("baseline wrong: knobs=%+v admin=%q", h.Knobs(), h.Credentials().AdminToken)
	}

	// persist an override, then rebuild (simulates restart) -> override loaded
	steps := 33
	if err := h.ApplyKnobs(context.Background(), st, runtimecfg.KnobsPatch{MaxSteps: &steps}); err != nil {
		t.Fatal(err)
	}
	h2 := buildRuntimeHolder(cfg, st, "op", "adm", ops)
	if h2.Knobs().MaxSteps != 33 {
		t.Fatalf("override must survive rebuild: %d", h2.Knobs().MaxSteps)
	}
	if h2.Knobs().MaxMessages != 40 {
		t.Fatalf("unset knob must keep config baseline: %d", h2.Knobs().MaxMessages)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/bootstrap/ -run BuildRuntimeHolder`
预期：编译失败（`undefined: buildRuntimeHolder`）。

- [ ] **步骤 3：创建 `internal/bootstrap/runtime_settings.go`**

```go
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
```

- [ ] **步骤 4：在 `newAPIServer` 装配**

在 `internal/bootstrap/bootstrap.go`，凭据解析并赋值之后（~400 行 `srv.Operators = operators` 之后）插入：

```go
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
```

同时把 compactor 的构造条件从「仅 config 启用时」放宽为「依赖就绪即构造」，使运行时可热开/热关压缩（开关由 `effectiveCompaction` 求值）。把 `bootstrap.go:299`：

```go
	if cfg.CompactEnabled() && messages != nil && provider != nil {
```

改为：

```go
	// Build the compactor whenever deps exist; the on/off switch is the hot
	// knob (baseline = cfg.CompactEnabled()), evaluated per-run in MaybeCompact.
	if messages != nil && provider != nil {
```

（`engine.Compactor = compactor` 现有赋值保持不变；compactor 非 nil 但开关关闭时 `MaybeCompact` 立即返回 `false,nil`。）

- [ ] **步骤 5：运行测试验证通过**

运行：`go test ./internal/bootstrap/ ./internal/run/ ./internal/api/`
预期：PASS。`StartForTest` 走同一 `newAPIServer`，holder 会被装配；TTL goroutine 随 closer 取消。

- [ ] **步骤 6：Commit**

```bash
git add internal/bootstrap/runtime_settings.go internal/bootstrap/runtime_settings_test.go internal/bootstrap/bootstrap.go
git commit -m "feat(bootstrap): wire runtime settings holder + TTL refresh into engine/compactor/server"
```

---

## 任务 9：本地 break-glass CLI 子命令 `reset-credentials`

**文件：**
- 创建：`cmd/baize/settings_reset.go`
- 修改：`cmd/baize/main.go`（加 case + usage）
- 测试：`cmd/baize/settings_reset_test.go`（把核心逻辑做成可测函数）

说明：轮换 admin 口令后若丢失新口令，运维需要一条不走 HTTP 门禁的本地恢复路径。该命令直接覆写 KV `runtime_settings` 中的凭据覆盖段为空（knobs 保留），重启/TTL 后即回落到 YAML break-glass 口令。

- [ ] **步骤 1：编写失败的测试**

创建 `cmd/baize/settings_reset_test.go`：

```go
package main

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
)

func TestResetCredentialsOverrideClearsCredsKeepsKnobs(t *testing.T) {
	st := store.NewMemory()
	base := runtimecfg.Snapshot{}
	h := runtimecfg.New(base)
	steps := 30
	if err := h.ApplyKnobs(context.Background(), st, runtimecfg.KnobsPatch{MaxSteps: &steps}); err != nil {
		t.Fatal(err)
	}
	if err := h.ApplyCreds(context.Background(), st, runtimecfg.CredsPatch{AdminToken: "lost-adm"}); err != nil {
		t.Fatal(err)
	}

	if err := resetCredentialsInStore(context.Background(), st); err != nil {
		t.Fatal(err)
	}

	// reload into a fresh holder (no config baseline here) -> creds override gone
	h2 := runtimecfg.New(runtimecfg.Snapshot{})
	if err := h2.Load(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	if h2.Credentials().AdminToken != "" {
		t.Fatalf("cred override must be cleared, got %q", h2.Credentials().AdminToken)
	}
	// knobs override must survive
	if h2.Knobs().MaxSteps != 30 {
		t.Fatalf("knob override must survive reset, got %d", h2.Knobs().MaxSteps)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./cmd/baize/ -run ResetCredentials`
预期：编译失败（`undefined: resetCredentialsInStore`）。

- [ ] **步骤 3：实现 `cmd/baize/settings_reset.go`**

```go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/store"
)

// resetCredentialsInStore clears the credential override in the runtime_settings
// KV while preserving any engine-knob override. After this (and a restart or TTL
// refresh) the control plane falls back to the YAML/env break-glass tokens.
func resetCredentialsInStore(ctx context.Context, st store.Store) error {
	raw, ok, err := st.GetSetting(store.SettingKeyRuntimeSettings)
	if err != nil {
		return err
	}
	persisted := struct {
		Knobs json.RawMessage `json:"knobs"`
		Creds json.RawMessage `json:"creds"`
	}{}
	if ok && len(raw) > 0 {
		if err := json.Unmarshal(raw, &persisted); err != nil {
			// Corrupt blob: overwrite wholesale with empty creds.
			persisted.Knobs = nil
		}
	}
	out := map[string]any{"creds": map[string]any{}}
	if len(persisted.Knobs) > 0 && string(persisted.Knobs) != "null" {
		out["knobs"] = persisted.Knobs
	}
	b, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return st.UpsertSetting(store.SettingKeyRuntimeSettings, b)
}

// runResetCredentials implements `baize reset-credentials -config <path>`.
func runResetCredentials(args []string) error {
	fs := flag.NewFlagSet("reset-credentials", flag.ContinueOnError)
	cfgPath := fs.String("config", startConfigPath(), "path to config yaml (for store driver/path)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	st, err := store.OpenWithOptions(cfg.Store.Driver, store.OpenOptions{
		SQLitePath: cfg.Store.SQLitePath,
		DSN:        cfg.Store.DSN,
	})
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	if c, ok := st.(interface{ Close() error }); ok {
		defer c.Close()
	}
	if err := resetCredentialsInStore(context.Background(), st); err != nil {
		return fmt.Errorf("reset credentials: %w", err)
	}
	log.Printf("control-plane credential overrides cleared; restart or wait for TTL refresh to use YAML tokens")
	return nil
}
```

注意：`store.OpenOptions` 字段名以 `internal/store/registry.go` 为准（`SQLitePath`、`DSN` 已确认存在）；`cfg.Store.DSN` 若字段名不同，用实际的 Postgres DSN 字段（实现时 `grep 'DSN' internal/config/config.go` 核对）。

- [ ] **步骤 4：在 `main.go` 接入**

在 `cmd/baize/main.go` 的 `switch os.Args[1]` 加 case（`default` 前）：

```go
	case "reset-credentials":
		if err := runResetCredentials(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
```

并在 `printUsage()` 加一行：

```go
	fmt.Println("  reset-credentials  clear hot-updated control-plane tokens (fall back to YAML break-glass)")
```

- [ ] **步骤 5：运行测试验证通过**

运行：`go test ./cmd/baize/`
预期：PASS。再 `go build ./...` 确认 CLI 编译。

- [ ] **步骤 6：Commit**

```bash
git add cmd/baize/settings_reset.go cmd/baize/settings_reset_test.go cmd/baize/main.go
git commit -m "feat(cli): reset-credentials break-glass command to clear hot-updated tokens"
```

---

## 任务 10：全量验证与文档

**文件：**
- 修改：`README.md`（在配置/运维段补一小节「运行时热更新设置」，列出端点与 `reset-credentials`）

- [ ] **步骤 1：全量构建与测试**

运行：

```bash
go build ./...
go vet ./...
gofmt -l .
go test ./...
```

预期：构建通过、vet 无告警、gofmt 无输出、全部测试 PASS。

- [ ] **步骤 2：手工冒烟（可选，需真实栈）**

`baize demo` 启动后：
1. `GET /v0/settings/runtime`（Bearer admin）→ 返回 effective knobs；
2. `PATCH /v0/settings/runtime` `{"max_steps":20}` → 再次 GET 生效；重启进程后仍生效（KV 持久）；
3. `PATCH /v0/settings/credentials` `{"admin_token":"new"}` → 用新口令立即通过、旧口令 401；
4. `GET /v0/settings/credentials` → 响应无任何 token 字段；
5. 微信 `PUT /v0/settings/channels/weixin` `{"enabled":false}` → 轮询停止、凭证保留；`{"enabled":true}` → 恢复。

- [ ] **步骤 3：更新 README**

在 README 运维/配置段补一小节，说明：哪些设置可热改、四个端点及权限、凭据不回传明文、`baize reset-credentials` 恢复路径。

- [ ] **步骤 4：最终 Commit**

```bash
git add README.md
git commit -m "docs: document runtime hot-reload settings endpoints and reset-credentials"
```

---

