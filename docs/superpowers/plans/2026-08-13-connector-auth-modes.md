# Connector 默认凭证模式 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 用显式 `auth.mode`（`static` / `passthrough` / `vault_ref`）提供无会话身份时的默认 HTTP Headers；删除 `bearer_env`；有会话身份时三种模式都不覆盖捕获凭证。

**架构：** 新增 `internal/authcred` 在注册时解析 static / vault_ref 得到 DefaultHeaders；passthrough 把白名单请求头写入 Run 私有字段，经 context 注入作为本次 DefaultHeaders。现有 `OpenAPISecurityResolver` 选身份顺序不变。

**技术栈：** Go 1.22+、现有 httptest、SQLite（modernc）、YAML。

**规格：** `docs/superpowers/specs/2026-08-13-connector-auth-modes-design.md`

**全局约束：**
- 删除所有 `bearer_env` / `BearerEnv` / `resolveBearerHeaders`，不做旧字段别名
- 秘密不进 events、不进 `GET /v0/runs/{id}`（`PassthroughHeaders` 用 `json:"-"`）
- GET connector 只回显 mode 与引用字符串，不回显解析后的 token
- 解析失败用错误 `invalid_auth`（PUT 为 HTTP 400），不得部分注册
- commit message 中文：`type(scope): 说明`

---

## 文件结构（将创建/修改）

| 路径 | 职责 |
|------|------|
| `internal/authcred/expand.go` | `static` 的 `${ENV}` 展开 |
| `internal/authcred/vault.go` | `vault_ref` 的 `env:` / `file:` |
| `internal/authcred/passthrough.go` | 按白名单从 `http.Header` 抽取 |
| `internal/authcred/config.go` | `Mode` 常量、`Config` 形状、`ResolveDefaults`、`ErrInvalidAuth` |
| `internal/authcred/*_test.go` | 上列单测 |
| `internal/config/config.go` | `Auth` 换成新形状；删除 `BearerEnv` |
| `internal/config/auth_test.go` | Load 缺省 mode=static |
| `configs/default.yaml` | `mode: static` + `${BAIZE_CONNECTOR_TOKEN}` |
| `internal/store/store.go` | `Connector.Auth`；`Run.PassthroughHeaders` `json:"-"`；`CreateRunInput` / `SetPassthroughHeaders` |
| `internal/store/memory.go` / `sqlite.go` | 持久化透传头（SQLite JSON 列）；Connector.Auth 内存即可（与现 Connector 一致） |
| `internal/identity/context.go` | `WithPassthroughHeaders` / `PassthroughHeadersFrom` |
| `internal/run/engine.go` | `injectAuthCtxFromRun` 注入透传头 |
| `internal/connector/openapi/register.go` | 闭包：passthrough 时用 ctx 头作 DefaultHeaders |
| `internal/bootstrap/bootstrap.go` | 用 `authcred.ResolveDefaults`；删除 `resolveBearerHeaders` |
| `internal/api/server.go` | PUT/GET auth；POST run/resume 抽白名单头；`invalid_auth` |
| `tests/integration/connector_auth_modes_test.go` | 三模式 + 脱敏 + 身份优先 |
| `README.md` / `README.zh-CN.md` / `docs/architecture-and-plugin-protocol.md` | 去掉 bearer_env，写三种 mode |

---

### 任务 1：authcred 解析器（static / vault_ref / passthrough 抽取）

**文件：**
- 创建：`internal/authcred/config.go`
- 创建：`internal/authcred/expand.go`
- 创建：`internal/authcred/vault.go`
- 创建：`internal/authcred/passthrough.go`
- 创建：`internal/authcred/authcred_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
package authcred_test

func TestResolveStaticExpandsEnv(t *testing.T) {
	t.Setenv("BAIZE_CONNECTOR_TOKEN", "tok-static")
	got, err := authcred.ResolveDefaults(authcred.Config{
		Mode: authcred.ModeStatic,
		Static: authcred.Static{Headers: map[string]string{
			"Authorization": "Bearer ${BAIZE_CONNECTOR_TOKEN}",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["Authorization"] != "Bearer tok-static" {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveStaticMissingEnv(t *testing.T) {
	_, err := authcred.ResolveDefaults(authcred.Config{
		Mode: authcred.ModeStatic,
		Static: authcred.Static{Headers: map[string]string{
			"Authorization": "Bearer ${MISSING_TOKEN_XYZ}",
		}},
	})
	if !errors.Is(err, authcred.ErrInvalidAuth) {
		t.Fatalf("err=%v", err)
	}
}

func TestResolveVaultEnvAndFile(t *testing.T) {
	t.Setenv("VAULT_ENV_TOK", "from-env")
	p := filepath.Join(t.TempDir(), "tok")
	if err := os.WriteFile(p, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := authcred.ResolveDefaults(authcred.Config{
		Mode: authcred.ModeVaultRef,
		VaultRef: authcred.VaultRef{Headers: map[string]string{
			"X-Env":  "env:VAULT_ENV_TOK",
			"X-File": "file:" + p,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["X-Env"] != "from-env" || got["X-File"] != "from-file" {
		t.Fatalf("got %+v", got)
	}
}

func TestResolveVaultUnknownPrefix(t *testing.T) {
	_, err := authcred.ResolveDefaults(authcred.Config{
		Mode:     authcred.ModeVaultRef,
		VaultRef: authcred.VaultRef{Headers: map[string]string{"Authorization": "secret:x"}},
	})
	if !errors.Is(err, authcred.ErrInvalidAuth) {
		t.Fatalf("err=%v", err)
	}
}

func TestResolvePassthroughHasNoDefaults(t *testing.T) {
	got, err := authcred.ResolveDefaults(authcred.Config{Mode: authcred.ModePassthrough})
	if err != nil || len(got) != 0 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestPickPassthroughWhitelist(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer IN")
	h.Set("Cookie", "secret=1")
	h.Set("X-Ignored", "nope")
	got := authcred.PickHeaders(h, []string{"Authorization"})
	if got["Authorization"] != "Bearer IN" || len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	empty := authcred.PickHeaders(h, nil) // nil → 缺省仅 Authorization
	if empty["Authorization"] != "Bearer IN" {
		t.Fatalf("default whitelist: %+v", empty)
	}
	none := authcred.PickHeaders(h, []string{}) // 显式空 → 无头
	if len(none) != 0 {
		t.Fatalf("explicit empty: %+v", none)
	}
}

func TestUnknownMode(t *testing.T) {
	_, err := authcred.ResolveDefaults(authcred.Config{Mode: "ldap"})
	if !errors.Is(err, authcred.ErrInvalidAuth) {
		t.Fatalf("err=%v", err)
	}
}
```

空 `mode` 在 `ResolveDefaults` 内视为 `static`。

- [ ] **步骤 2：运行确认失败**

```bash
go test ./internal/authcred/ -count=1
```

预期：FAIL（包不存在）

- [ ] **步骤 3：最少实现**

```go
package authcred

var ErrInvalidAuth = errors.New("invalid_auth")

const (
	ModeStatic      = "static"
	ModePassthrough = "passthrough"
	ModeVaultRef    = "vault_ref"
)

type Config struct {
	Mode        string    `json:"mode,omitempty" yaml:"mode"`
	Static      Static    `json:"static,omitempty" yaml:"static"`
	Passthrough PassThru  `json:"passthrough,omitempty" yaml:"passthrough"`
	VaultRef    VaultRef  `json:"vault_ref,omitempty" yaml:"vault_ref"`
}

type Static struct {
	Headers map[string]string `json:"headers,omitempty" yaml:"headers"`
}
type PassThru struct {
	Headers []string `json:"headers,omitempty" yaml:"headers"`
}
type VaultRef struct {
	Headers map[string]string `json:"headers,omitempty" yaml:"headers"`
}

func NormalizeMode(mode string) string {
	if strings.TrimSpace(mode) == "" {
		return ModeStatic
	}
	return strings.TrimSpace(mode)
}

func ResolveDefaults(cfg Config) (map[string]string, error)
func PickHeaders(h http.Header, whitelist []string) map[string]string
```

- `${ENV}`：用正则 `\$\{([A-Za-z_][A-Za-z0-9_]*)\}` 替换；缺 env 或结果 trim 后为空 → `ErrInvalidAuth`
- `env:NAME` / `file:path`：`os.Getenv` / `os.ReadFile` + `strings.TrimSpace`；目录用 `os.Stat` 拒绝
- `PickHeaders`：`whitelist == nil` 当作 `[]string{"Authorization"}`；`len==0` 返回空 map；空值丢掉
- 未知 mode → `ErrInvalidAuth`
- `passthrough` 的 `ResolveDefaults` 返回 `nil, nil`

- [ ] **步骤 4：跑通测试**

```bash
go test ./internal/authcred/ -count=1
```

- [ ] **步骤 5：Commit** `feat(authcred): static/vault_ref 解析与透传白名单`

---

### 任务 2：配置形状替换 bearer_env

**文件：**
- 修改：`internal/config/config.go`
- 创建：`internal/config/auth_test.go`
- 修改：`configs/default.yaml`

- [ ] **步骤 1：失败测试**

```go
func TestLoadAuthModeDefaultStatic(t *testing.T) {
	path := writeConfig(t, "store:\n  driver: memory\nconnector:\n  auth:\n    static:\n      headers:\n        Authorization: Bearer ${BAIZE_CONNECTOR_TOKEN}\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Connector.Auth.Mode != "" && cfg.Connector.Auth.Mode != "static" {
		t.Fatalf("mode=%q", cfg.Connector.Auth.Mode)
	}
	if cfg.Connector.Auth.Static.Headers["Authorization"] != "Bearer ${BAIZE_CONNECTOR_TOKEN}" {
		t.Fatalf("headers=%v", cfg.Connector.Auth.Static.Headers)
	}
}

func TestLoadAuthVaultRef(t *testing.T) {
	path := writeConfig(t, "connector:\n  auth:\n    mode: vault_ref\n    vault_ref:\n      headers:\n        Authorization: env:BAIZE_CONNECTOR_TOKEN\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Connector.Auth.Mode != "vault_ref" {
		t.Fatalf("mode=%q", cfg.Connector.Auth.Mode)
	}
	if cfg.Connector.Auth.VaultRef.Headers["Authorization"] != "env:BAIZE_CONNECTOR_TOKEN" {
		t.Fatalf("%v", cfg.Connector.Auth.VaultRef.Headers)
	}
}
```

确认 `config.Config.Connector.Auth` **没有** `BearerEnv` 字段（编译期：测试文件不得引用它）。

- [ ] **步骤 2：改 `config.go`**

把 `Auth` 换成：

```go
Auth struct {
	Mode        string `yaml:"mode"`
	Static      struct {
		Headers map[string]string `yaml:"headers"`
	} `yaml:"static"`
	Passthrough struct {
		Headers []string `yaml:"headers"`
	} `yaml:"passthrough"`
	VaultRef struct {
		Headers map[string]string `yaml:"headers"`
	} `yaml:"vault_ref"`
	Capture struct { /* 保持现有字段 */ } `yaml:"capture"`
}
```

删除 `BearerEnv`。

`configs/default.yaml`：

```yaml
  auth:
    mode: static
    static:
      headers:
        Authorization: "Bearer ${BAIZE_CONNECTOR_TOKEN}"
    capture:
      tool_name_glob: "*login*"
      # …其余 capture 保持
```

- [ ] **步骤 3：修复编译**

`internal/bootstrap/bootstrap.go` 的 `resolveBearerHeaders(cfg.Connector.Auth.BearerEnv)` 暂时改为 `Headers: nil`，并删掉 `resolveBearerHeaders` 函数（任务 4 会接上真正解析）。若因此有测试失败，一并改掉对 `BearerEnv` 的引用。

```bash
go test ./internal/config/ ./internal/bootstrap/ -count=1
```

- [ ] **步骤 4：Commit** `feat(config): auth.mode 替换 bearer_env`

---

### 任务 3：Store 持久化透传头与 Connector.Auth 形状

**文件：**
- 修改：`internal/store/store.go`
- 修改：`internal/store/memory.go`
- 修改：`internal/store/sqlite.go`
- 修改：`internal/store/store_test.go`
- 修改：`internal/store/sqlite_test.go`

- [ ] **步骤 1：失败测试**

```go
func TestCreateRunPersistsPassthroughHeaders(t *testing.T) {
	s := store.NewMemory()
	r, err := s.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "hi",
		PassthroughHeaders: map[string]string{"Authorization": "Bearer SECRET"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRun(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PassthroughHeaders["Authorization"] != "Bearer SECRET" {
		t.Fatalf("%+v", got)
	}
}

func TestSetPassthroughHeadersOverwrites(t *testing.T) {
	s := store.NewMemory()
	r, err := s.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi",
		PassthroughHeaders: map[string]string{"Authorization": "Bearer OLD"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPassthroughHeaders(r.ID, map[string]string{"Authorization": "Bearer NEW"}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetRun(r.ID)
	if got.PassthroughHeaders["Authorization"] != "Bearer NEW" {
		t.Fatalf("%+v", got)
	}
}

func TestConnectorStoresAuthConfig(t *testing.T) {
	s := store.NewMemory()
	s.UpsertConnector(store.Connector{
		ID: "c", Type: "openapi", Spec: "x.yaml", BaseURL: "http://x",
		Auth: store.ConnectorAuth{Mode: "vault_ref", VaultRef: store.VaultRefAuth{
			Headers: map[string]string{"Authorization": "env:TOK"},
		}},
	})
	c, err := s.GetConnector("c")
	if err != nil {
		t.Fatal(err)
	}
	if c.Auth.Mode != "vault_ref" || c.Auth.VaultRef.Headers["Authorization"] != "env:TOK" {
		t.Fatalf("%+v", c.Auth)
	}
}
```

SQLite 同测：`TestSQLiteCreateRunPersistsPassthroughHeaders`，Create → Get 后头还在。

- [ ] **步骤 2：类型**

```go
type ConnectorAuth struct {
	Mode        string       `json:"mode,omitempty"`
	Static      StaticAuth   `json:"static,omitempty"`
	Passthrough PassThruAuth `json:"passthrough,omitempty"`
	VaultRef    VaultRefAuth `json:"vault_ref,omitempty"`
}
type StaticAuth struct {
	Headers map[string]string `json:"headers,omitempty"`
}
type PassThruAuth struct {
	Headers []string `json:"headers,omitempty"`
}
type VaultRefAuth struct {
	Headers map[string]string `json:"headers,omitempty"`
}

type Connector struct {
	// 现有字段…
	Auth ConnectorAuth `json:"auth,omitempty"`
}

type Run struct {
	// 现有字段…
	PassthroughHeaders map[string]string `json:"-"`
}

type CreateRunInput struct {
	// 现有字段…
	PassthroughHeaders map[string]string
}

// Store 增加：
SetPassthroughHeaders(runID string, headers map[string]string) error
```

SQLite：`migrateRunsColumns` 增加 `passthrough_json TEXT`；Create/Get 读写 JSON；`SetPassthroughHeaders` UPDATE 该列。Connector 仍内存 map（与现网一致），`Auth` 跟着 `UpsertConnector` 走。

- [ ] **步骤 3：跑通**

```bash
go test ./internal/store/ -count=1
```

- [ ] **步骤 4：Commit** `feat(store): Run 透传头与 Connector.Auth`

---

### 任务 4：注册解析 + invoke 使用 Run 透传头

**文件：**
- 修改：`internal/identity/context.go`
- 修改：`internal/run/engine.go`
- 修改：`internal/connector/openapi/register.go`
- 修改：`internal/connector/openapi/register_opts.go`（可选：`AuthMode string`）
- 修改：`internal/bootstrap/bootstrap.go`
- 测试：`internal/connector/openapi/register_test.go`
- 测试：`internal/identity` 或 `internal/run` 若需覆盖注入

- [ ] **步骤 1：失败测试（register_test.go 追加）**

在现有 `TestRegisterWithOptsResolvesCapturedIdentity` 旁增加：

```go
func TestRegisterPassthroughUsesContextHeaders(t *testing.T) {
	var lastAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	spec := writeMinimalSpec(t) // 复用该文件已有 helper；若无则写含 GET /health 的最小 spec
	st := store.NewMemory()
	reg := tool.NewRegistry()
	_, _, err := openapi.RegisterWithOpts(st, reg, openapi.RegisterOpts{
		ID: "c", SpecPath: spec, BaseURL: srv.URL,
		Identities: identity.NewMemoryStore(),
		Resolver:   authresolve.OpenAPISecurityResolver{},
		AuthMode:   "passthrough",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := identity.WithPassthroughHeaders(context.Background(), map[string]string{
		"Authorization": "Bearer FROM_RUN",
	})
	content, isErr, invErr := reg.Invoke(ctx, /* 该 spec 的 tool 名 */, map[string]any{})
	if invErr != nil || isErr {
		t.Fatalf("invoke content=%v isErr=%v err=%v", content, isErr, invErr)
	}
	if lastAuth != "Bearer FROM_RUN" {
		t.Fatalf("Authorization=%q", lastAuth)
	}
}
```

`RegisterOpts` 增加 `AuthMode string`（空 = 非 passthrough，继续用 `opts.Headers` 作 DefaultHeaders）。

- [ ] **步骤 2：context + engine**

```go
// identity/context.go
func WithPassthroughHeaders(ctx context.Context, h map[string]string) context.Context
func PassthroughHeadersFrom(ctx context.Context) map[string]string
```

`injectAuthCtxFromRun`：若 `len(runRec.PassthroughHeaders)>0`，`ctx = identity.WithPassthroughHeaders(ctx, runRec.PassthroughHeaders)`。

- [ ] **步骤 3：register 闭包**

在拼 `ResolveInput` 处：

```go
defaultHeaders := opts.Headers
if opts.AuthMode == "passthrough" {
	if h := identity.PassthroughHeadersFrom(ctx); len(h) > 0 {
		defaultHeaders = h
	} else {
		defaultHeaders = nil
	}
}
in := authresolve.ResolveInput{
	Identities:      opts.Identities.List(conv),
	SecuritySchemes: route.Security,
	DefaultHeaders:  defaultHeaders,
	ForceIdentityID: force,
}
```

无 Resolver 时 passthrough 也应：`overlay = identity.PassthroughHeadersFrom(ctx)` 否则 `opts.Headers`。

- [ ] **步骤 4：bootstrap**

```go
authCfg := authcred.Config{
	Mode: cfg.Connector.Auth.Mode,
	Static: authcred.Static{Headers: cfg.Connector.Auth.Static.Headers},
	Passthrough: authcred.PassThru{Headers: cfg.Connector.Auth.Passthrough.Headers},
	VaultRef: authcred.VaultRef{Headers: cfg.Connector.Auth.VaultRef.Headers},
}
headers, err := authcred.ResolveDefaults(authCfg)
if err != nil {
	return err
}
_, _, err = openapi.RegisterWithOpts(..., RegisterOpts{
	Headers:  headers,
	AuthMode: authcred.NormalizeMode(cfg.Connector.Auth.Mode),
	// Identities / Resolver / Capture 保持
})
```

Upsert 进 Store 的 Connector 带上 `Auth` 配置形状（mode + 引用，不是解析后的 headers）。

删除 `resolveBearerHeaders`。`withCaptureDefaults` 注释去掉「only sets bearer_env」。

- [ ] **步骤 5：**

```bash
go test ./internal/connector/openapi/ ./internal/bootstrap/ ./internal/run/ ./internal/identity/ -count=1
```

- [ ] **步骤 6：Commit** `feat(connector): 按 auth.mode 解析默认头并注入透传`

---

### 任务 5：API PUT/GET auth 与 POST run/resume 透传

**文件：**
- 修改：`internal/api/server.go`
- 修改：`internal/api/server_test.go`

Server 增加字段：

```go
type Server struct {
	// 现有…
	AuthWhitelist []string // passthrough 白名单；nil → 默认 Authorization；仅 mode=passthrough 时使用
	AuthMode      string
}
```

`baize start` 在构造 Server 后赋值 `srv.AuthMode` / `srv.AuthWhitelist`（bootstrap 里设）。PUT connector 成功后按 body.auth 更新这两项（单进程单活动 connector 与现网一致）。

- [ ] **步骤 1：失败测试**

```go
func TestPutConnectorVaultRefPersistsAuthShape(t *testing.T) {
	t.Setenv("PUT_VAULT_TOK", "secret-xyz")
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	spec := /* 现有测试用的 openapi 文件路径 helper */
	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{
			"type": "openapi", "spec": spec, "base_url": "http://x",
			"auth": map[string]any{
				"mode": "vault_ref",
				"vault_ref": map[string]any{"headers": map[string]string{"Authorization": "env:PUT_VAULT_TOK"}},
			},
		}))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "secret-xyz") {
		t.Fatal("leaked secret in PUT response")
	}
	if !strings.Contains(rr.Body.String(), "env:PUT_VAULT_TOK") {
		t.Fatalf("want ref in body=%s", rr.Body.String())
	}
}

func TestPutConnectorInvalidAuthDoesNotRegister(t *testing.T) {
	// vault_ref env 不存在 → 400 invalid_auth；GET tools 仍空或不含新 tool
}

func TestPostRunPassthroughHiddenFromGetRun(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	srv.AuthMode = "passthrough"
	st.UpsertAgent(store.Agent{ID: "a", System: "s"})
	req := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{"agent_id": "a", "input": "hi"}))
	req.Header.Set("Authorization", "Bearer HIDDEN_TOKEN")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d", rr.Code)
	}
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)
	runID := created["run_id"].(string)
	got, _ := st.GetRun(runID)
	if got.PassthroughHeaders["Authorization"] != "Bearer HIDDEN_TOKEN" {
		t.Fatalf("store=%+v", got.PassthroughHeaders)
	}
	get := httptest.NewRequest(http.MethodGet, "/v0/runs/"+runID, nil)
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, get)
	if strings.Contains(rr.Body.String(), "HIDDEN_TOKEN") {
		t.Fatal("GET run leaked token")
	}
}
```

PUT 解析：把 body.auth 映成 `authcred.Config`，`ResolveDefaults`；失败 `writeError(400, "invalid_auth", err.Error())`。成功则 `RegisterWithOpts` 带 Headers + AuthMode，Store Connector.Auth 存**配置形状**（body 原文规范化，不是解析值）。

POST run：`headers := authcred.PickHeaders(r.Header, s.AuthWhitelist)`，仅当 `NormalizeMode(s.AuthMode)==passthrough` 时写入 `CreateRunInput.PassthroughHeaders`。

Resume：若 mode=passthrough，`picked := PickHeaders(...)`；若 `len(picked)>0` 则 `SetPassthroughHeaders`。在 `ContinueFromHITL` **之前**写入。

GET connector 响应增加 `"auth": c.Auth`。

- [ ] **步骤 2：实现并跑**

```bash
go test ./internal/api/ -count=1
```

bootstrap 创建 Server 后设置 AuthMode / AuthWhitelist。

- [ ] **步骤 3：Commit** `feat(api): connector auth 与 run 透传头`

---

### 任务 6：集成测试 + 文档清扫 bearer_env

**文件：**
- 创建：`tests/integration/connector_auth_modes_test.go`
- 修改：`README.md`、`README.zh-CN.md`
- 修改：`docs/architecture-and-plugin-protocol.md`
- 修改：规格状态已是已批准；计划文档自身不改规格语义

- [ ] **步骤 1：集成测试**

沿用 `tests/integration/session_auth_test.go` 的 spec helper / httptest 下游：

1. **static：** `t.Setenv` + `ResolveDefaults` + `RegisterWithOpts` + 无 conv invoke → Authorization 为展开值
2. **vault_ref file：** 临时文件 + 同上
3. **passthrough：** API Server `AuthMode=passthrough`，真实 `run.Engine` + script/mock LLM 调一个 GET tool；POST run 带 Authorization；断言下游 HTTP 头；`GET /v0/runs/{id}` 与 events JSON 不含 token
4. **身份优先：** vault_ref 默认头为 ENV；先 capture 登录；再 invoke → 下游为捕获 token 而非 ENV

若集成拉起完整 bootstrap 过重，允许用 `RegisterWithOpts` + `api.NewServer` + `run.Engine`（与现 `session_auth_test.go` 同级）。

- [ ] **步骤 2：**

```bash
go test ./tests/integration/ -count=1
go test ./... -count=1
```

全绿。`rg bearer_env` 在 `*.go` / `configs/` / `README*` / `docs/architecture-and-plugin-protocol.md` 中为零（`docs/superpowers` 历史规格可保留，不强制改旧设计文档）。

- [ ] **步骤 3：文档**

README 中英 Session identities 节：

- 删「`bearer_env` is a startup fallback」
- 改为：无会话身份时按 `connector.auth.mode`：`static`（`${ENV}`）、`passthrough`（POST `/v0/runs` 白名单头）、`vault_ref`（`env:` / `file:`）
- 本地配置示例换成：

```yaml
  auth:
    mode: static
    static:
      headers:
        Authorization: "Bearer ${BAIZE_CONNECTOR_TOKEN}"
```

架构 §4.4 第 2 条改为：

> 调用时由 Runtime 拼 HTTP 请求。无会话身份时默认头来自 Connector `auth.mode`：`static`（注册时展开 `${ENV}`）、`passthrough`（该 Run 的白名单请求头）、`vault_ref`（注册时解析 `env:` / `file:`）。有会话身份时 Identity 优先。完整凭证不出现在 events 与 GET run。

- [ ] **步骤 4：Commit** `test+docs(auth): 三种默认凭证模式与文档`

---

## 自检（对照规格）

| 规格要点 | 任务 |
|----------|------|
| static `${ENV}`；空 env 失败 | 1, 2, 4, 6 |
| passthrough 白名单、Run 私有、resume 覆盖、不进 GET/events | 1, 3, 5, 6 |
| vault_ref env/file；失败 invalid_auth；不静默 | 1, 5, 6 |
| 身份优先 | 4（沿用 Resolver）、6 |
| GET connector 引用不泄密 | 5, 6 |
| 删除 bearer_env | 2, 4, 6 |
| PUT 同一套 auth | 5 |
| 不做 Vault HTTP / UI 贴 token | 全计划未包含 |

无 TODO/待定占位；类型名统一为 `authcred.Config` / `authcred.ErrInvalidAuth` / `store.PassthroughHeaders` / `RegisterOpts.AuthMode`。

---

## 执行交接

计划已保存到 `docs/superpowers/plans/2026-08-13-connector-auth-modes.md`。

**两种执行方式：**

1. **子代理驱动（推荐）** — 每任务新子代理 + 任务间审查
2. **内联执行** — 本会话用 executing-plans 按任务推进并设检查点

选哪种方式？
