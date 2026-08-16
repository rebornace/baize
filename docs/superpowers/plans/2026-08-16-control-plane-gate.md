# 控制面操作员 / 管理员口令门禁 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 两把可选静态口令挡住白泽 `/v0`：操作员只能对话 / 审批 / 账号；管理员才能改 Agent、Connector、Tools；没配口令时行为与今天相同。

**架构：** `internal/controlplane` 负责解析口令、比对 Bearer、查最低角色。`api.Server.Handler` 包一层门禁再交给现有 mux。`/ui` 用 `gate_enabled` + 解锁页 + `fetch` SSE。不改会话登录、HITL、Connector mode。

**技术栈：** Go 1.22+、现有 httptest、React + Vite、vitest。

**规格：** `docs/superpowers/specs/2026-08-16-control-plane-gate-design.md`

**全局约束：**
- 不在 `main` 上改代码：先 `git checkout -b feat/control-plane-gate`
- 不引入用户表、Cookie、SSO、`WWW-Authenticate`
- 口令不进 events / GET run / SSE / 日志
- `control_plane` 的 `env:` 为空 = 这把口令未配，进程照常启动（不要复用 `authcred` 的 vault 失败）
- commit 中文 `type(scope): 说明`；PowerShell 不要 bash HEREDOC
- Go：`C:\Users\Administrator\sdk\go\bin`；`GOPROXY=https://goproxy.cn,direct`
- 每步测试：`$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH; go test <pkg> -count=1`

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `internal/controlplane/secret.go` | `ResolveSecret`：空 / `env:` / `file:` / 明文 |
| `internal/controlplane/auth.go` | `Tokens`、`Role`、`Authenticate`、context |
| `internal/controlplane/acl.go` | `MinRole(method, path)` |
| `internal/controlplane/*_test.go` | 解析、比对、ACL 表 |
| `internal/config/config.go` | `ControlPlane.OperatorToken` / `AdminToken` |
| `internal/api/server.go` | `OperatorToken` / `AdminToken`；`Handler` 包门禁；`GET /v0/me`；`ui-config.gate_enabled` |
| `internal/api/server_gate_test.go` | 门开关 × 角色 × 接口 |
| `internal/bootstrap/bootstrap.go` | 解析口令注入 Server |
| `configs/default.yaml` / `configs/docker.yaml` | 注释中的空 `control_plane` |
| `web/chat/src/controlAuth.ts` | localStorage 口令与 `Authorization` 头 |
| `web/chat/src/parseSSE.ts` | fetch SSE 解析 |
| `web/chat/src/api.ts` | 统一带头；`getUIConfig` / `getMe`；`openRunStream` 改 fetch |
| `web/chat/src/pages/UnlockPage.tsx` | 解锁页 |
| `web/chat/src/pages/GateRoot.tsx` | 读 ui-config、挡未解锁路由 |
| `web/chat/src/pages/SettingsLayout.tsx` / `ChatPage.tsx` / `main.tsx` | 按角色显隐、退出 |
| `web/chat/src/settingsNav.ts` | 导航项纯函数 |
| `README.md` / `README.zh-CN.md` / `docs/architecture-and-plugin-protocol.md` | 文档 |
| `internal/ui/dist/**` | `npm run build` 产物 |

---

### 任务 1：controlplane 解析、比对与 ACL

**文件：**
- 创建：`internal/controlplane/secret.go`
- 创建：`internal/controlplane/auth.go`
- 创建：`internal/controlplane/acl.go`
- 创建：`internal/controlplane/secret_test.go`
- 创建：`internal/controlplane/auth_test.go`
- 创建：`internal/controlplane/acl_test.go`

- [ ] **步骤 1：写失败测试**

`secret_test.go`：

```go
package controlplane

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSecretEmpty(t *testing.T) {
	got, err := ResolveSecret("")
	if err != nil || got != "" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	got, err = ResolveSecret("   ")
	if err != nil || got != "" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestResolveSecretEnvUnsetIsEmpty(t *testing.T) {
	t.Setenv("BAIZE_OPERATOR_TOKEN_TEST_UNSET", "")
	_ = os.Unsetenv("BAIZE_OPERATOR_TOKEN_TEST_MISSING")
	got, err := ResolveSecret("env:BAIZE_OPERATOR_TOKEN_TEST_UNSET")
	if err != nil || got != "" {
		t.Fatalf("unset env must be empty, not error: %q %v", got, err)
	}
	got, err = ResolveSecret("env:BAIZE_OPERATOR_TOKEN_TEST_MISSING")
	if err != nil || got != "" {
		t.Fatalf("missing env must be empty, not error: %q %v", got, err)
	}
}

func TestResolveSecretEnvSet(t *testing.T) {
	t.Setenv("BAIZE_OPERATOR_TOKEN_TEST_SET", " op-secret ")
	got, err := ResolveSecret("env:BAIZE_OPERATOR_TOKEN_TEST_SET")
	if err != nil || got != "op-secret" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestResolveSecretPlaintext(t *testing.T) {
	got, err := ResolveSecret("plain-token")
	if err != nil || got != "plain-token" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestResolveSecretFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tok")
	if err := os.WriteFile(p, []byte(" file-secret \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveSecret("file:" + p)
	if err != nil || got != "file-secret" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	_, err = ResolveSecret("file:" + filepath.Join(dir, "missing"))
	if err == nil {
		t.Fatal("missing file must error")
	}
}
```

`auth_test.go`：

```go
package controlplane

import "testing"

func TestAuthenticateGateOff(t *testing.T) {
	tok := Tokens{}
	if tok.Enabled() {
		t.Fatal("empty tokens must disable gate")
	}
}

func TestAuthenticateRoles(t *testing.T) {
	tok := Tokens{Operator: "op", Admin: "adm"}
	if !tok.Enabled() {
		t.Fatal("expected enabled")
	}
	role, ok := Authenticate("Bearer adm", tok)
	if !ok || role != RoleAdmin {
		t.Fatalf("admin: %q %v", role, ok)
	}
	role, ok = Authenticate("Bearer op", tok)
	if !ok || role != RoleOperator {
		t.Fatalf("operator: %q %v", role, ok)
	}
	if _, ok := Authenticate("Bearer nope", tok); ok {
		t.Fatal("bad token")
	}
	if _, ok := Authenticate("", tok); ok {
		t.Fatal("missing")
	}
	if _, ok := Authenticate("op", tok); ok {
		t.Fatal("must require Bearer prefix")
	}
}

func TestAuthenticateSameValueIsAdmin(t *testing.T) {
	tok := Tokens{Operator: "same", Admin: "same"}
	role, ok := Authenticate("Bearer same", tok)
	if !ok || role != RoleAdmin {
		t.Fatalf("got %q %v", role, ok)
	}
}

func TestAuthenticateOperatorOnly(t *testing.T) {
	tok := Tokens{Operator: "op"}
	role, ok := Authenticate("Bearer op", tok)
	if !ok || role != RoleOperator {
		t.Fatalf("got %q %v", role, ok)
	}
}

func TestRoleAtLeast(t *testing.T) {
	if !RoleAdmin.AtLeast(RoleOperator) || !RoleAdmin.AtLeast(RoleAdmin) {
		t.Fatal("admin is superset")
	}
	if RoleOperator.AtLeast(RoleAdmin) {
		t.Fatal("operator must not satisfy admin")
	}
	if !RoleOperator.AtLeast(RoleOperator) {
		t.Fatal("operator satisfies operator")
	}
}
```

`acl_test.go`：

```go
package controlplane

import "testing"

func TestMinRoleTable(t *testing.T) {
	cases := []struct {
		method, path string
		want         Role
	}{
		{"GET", "/v0/ui-config", RoleNone},
		{"GET", "/v0/me", RoleOperator},
		{"POST", "/v0/runs", RoleOperator},
		{"POST", "/v0/runs/r1/resume", RoleOperator},
		{"GET", "/v0/runs/r1", RoleOperator},
		{"GET", "/v0/runs/r1/events", RoleOperator},
		{"GET", "/v0/runs/r1/stream", RoleOperator},
		{"GET", "/v0/conversations", RoleOperator},
		{"GET", "/v0/conversations/c1/messages", RoleOperator},
		{"DELETE", "/v0/conversations/c1/messages", RoleOperator},
		{"GET", "/v0/conversations/c1/identities", RoleOperator},
		{"POST", "/v0/conversations/c1/identities/i1/default", RoleOperator},
		{"DELETE", "/v0/conversations/c1/identities/i1", RoleOperator},
		{"DELETE", "/v0/conversations/c1/identities", RoleOperator},
		{"PUT", "/v0/agents/a1", RoleAdmin},
		{"PUT", "/v0/connectors/c1", RoleAdmin},
		{"GET", "/v0/connectors/c1", RoleAdmin},
		{"GET", "/v0/tools", RoleAdmin},
		{"PATCH", "/v0/tools/create_ticket", RoleAdmin},
		{"GET", "/v0/unknown", RoleAdmin},
		{"POST", "/v0/runs/r1/resume/extra", RoleAdmin},
	}
	for _, tc := range cases {
		got := MinRole(tc.method, tc.path)
		if got != tc.want {
			t.Errorf("%s %s: got %q want %q", tc.method, tc.path, got, tc.want)
		}
	}
}
```

- [ ] **步骤 2：运行确认失败**

```powershell
$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH
go test ./internal/controlplane -count=1
```

预期：FAIL（包不存在或符号未定义）。

- [ ] **步骤 3：最少实现**

`secret.go`：`ResolveSecret(ref string) (string, error)`。Trim 后空 → `("", nil)`。`env:NAME` → `strings.TrimSpace(os.Getenv(NAME))`，空也返回 `("", nil)`，**不要**返回 `authcred.ErrInvalidAuth`。`file:path`：读文件 Trim；缺文件或目录 → `fmt.Errorf(...)`。其它字符串当明文返回。

`auth.go`：

```go
type Role string

const (
	RoleNone     Role = ""
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

type Tokens struct {
	Operator string
	Admin    string
}

func (t Tokens) Enabled() bool {
	return t.Operator != "" || t.Admin != ""
}

func (r Role) AtLeast(min Role) bool {
	switch min {
	case RoleNone:
		return true
	case RoleOperator:
		return r == RoleOperator || r == RoleAdmin
	case RoleAdmin:
		return r == RoleAdmin
	default:
		return false
	}
}
```

`Authenticate(authorizationHeader string, t Tokens) (Role, bool)`：要求前缀 `Bearer `（注意空格）；取出 token。用 `crypto/subtle.ConstantTimeCompare`：管理员口令非空则先比；长度不等时仍对 want 做一次比较（补零长度副本）再返回不匹配。对上管理员 → `RoleAdmin`。再比操作员。对上 → `RoleOperator`。否则 `(RoleNone, false)`。两把相同则先命中管理员。

context：

```go
type roleKey struct{}

func WithRole(ctx context.Context, role Role) context.Context {
	return context.WithValue(ctx, roleKey{}, role)
}

func RoleFrom(ctx context.Context) Role {
	role, _ := ctx.Value(roleKey{}).(Role)
	return role
}
```

`acl.go`：`MinRole(method, path string) Role`。先规范化 path（去掉 query，保留 path）。`GET /v0/ui-config` → `RoleNone`。其余按规格 §5 用分段匹配（`{id}` 一段非空且不含 `/`）。更具体的 `/v0/runs/{id}/stream` 必须在 `/v0/runs/{id}` 之前。未匹配的 `/v0` 前缀 → `RoleAdmin`。非 `/v0` 也返回 `RoleAdmin`（HTTP 层不会用到，测试里 `/v0/unknown` 覆盖失败关闭）。

- [ ] **步骤 4：测试通过**

```powershell
$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH
go test ./internal/controlplane -count=1
```

预期：PASS。

- [ ] **步骤 5：Commit**

```powershell
git add internal/controlplane
git commit -m "feat(auth): 控制面口令解析与路由最低角色表"
```

---

### 任务 2：API 门禁、`/v0/me`、`ui-config.gate_enabled`

**文件：**
- 修改：`internal/api/server.go`（`Server` 字段、`Handler`、`routes`、`handleUIConfig`、新增 `handleMe`）
- 创建：`internal/api/server_gate_test.go`

- [ ] **步骤 1：写失败测试**（`internal/api/server_gate_test.go`，`package api`）

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func gateServer(t *testing.T, operator, admin string) *Server {
	t.Helper()
	st := store.NewMemory()
	srv := NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.OperatorToken = operator
	srv.AdminToken = admin
	return srv
}

func TestGateOffAllowsAnonymous(t *testing.T) {
	srv := gateServer(t, "", "")
	req := httptest.NewRequest(http.MethodGet, "/v0/ui-config", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("code=%d", rr.Code)
	}
	var body struct {
		GateEnabled bool `json:"gate_enabled"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.GateEnabled {
		t.Fatal("gate must be off")
	}
	req = httptest.NewRequest(http.MethodPost, "/v0/runs", strings.NewReader(`{"agent_id":"a","input":"i"}`))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("anonymous POST /v0/runs code=%d body=%s", rr.Code, rr.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/v0/me", nil)
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"role":""`) {
		t.Fatalf("me=%d %s", rr.Code, rr.Body.String())
	}
}

func TestGateOnUnauthorized(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("healthz=%d", rr.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/v0/ui-config", nil)
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"gate_enabled":true`) {
		t.Fatalf("ui-config=%d %s", rr.Code, rr.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/v0/runs", strings.NewReader(`{"agent_id":"a","input":"i"}`))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 401 || !strings.Contains(rr.Body.String(), "unauthorized") {
		t.Fatalf("want 401 unauthorized, got %d %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Header().Get("WWW-Authenticate"), "Basic") {
		t.Fatal("must not send WWW-Authenticate Basic")
	}
	req = httptest.NewRequest(http.MethodPost, "/v0/runs", strings.NewReader(`{"agent_id":"a","input":"i"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer nope")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 401 || !strings.Contains(rr.Body.String(), "需要控制面口令") {
		t.Fatalf("bad token: %d %s", rr.Code, rr.Body.String())
	}
}

func TestGateOperatorForbiddenOnAdminRoutes(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()
	auth := func(r *http.Request) { r.Header.Set("Authorization", "Bearer op") }

	req := httptest.NewRequest(http.MethodGet, "/v0/me", nil)
	auth(req)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"role":"operator"`) {
		t.Fatalf("me=%d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v0/runs", strings.NewReader(`{"agent_id":"a","input":"i"}`))
	req.Header.Set("Content-Type", "application/json")
	auth(req)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("POST runs=%d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/tools", nil)
	auth(req)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 403 || !strings.Contains(rr.Body.String(), "forbidden") {
		t.Fatalf("GET tools=%d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/v0/connectors/x", strings.NewReader(`{"type":"openapi","spec":"examples/mock-ticket/openapi.yaml","base_url":"http://127.0.0.1:9"}`))
	req.Header.Set("Content-Type", "application/json")
	auth(req)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 403 || !strings.Contains(rr.Body.String(), "需要管理员口令") {
		t.Fatalf("PUT connector=%d %s", rr.Code, rr.Body.String())
	}
}

func TestGateAdminCanReadTools(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	req := httptest.NewRequest(http.MethodGet, "/v0/me", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"role":"admin"`) {
		t.Fatalf("me=%d %s", rr.Code, rr.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/v0/tools", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("tools=%d %s", rr.Code, rr.Body.String())
	}
}

func TestGateOperatorOnlyCannotPut(t *testing.T) {
	srv := gateServer(t, "op", "")
	req := httptest.NewRequest(http.MethodPut, "/v0/agents/a1", strings.NewReader(`{"system":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer op")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 403 {
		t.Fatalf("code=%d %s", rr.Code, rr.Body.String())
	}
}

func TestGateSameTokenIsAdmin(t *testing.T) {
	srv := gateServer(t, "same", "same")
	req := httptest.NewRequest(http.MethodGet, "/v0/tools", nil)
	req.Header.Set("Authorization", "Bearer same")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("code=%d %s", rr.Code, rr.Body.String())
	}
}

func TestGateSSEUnauthorizedNotEventStream(t *testing.T) {
	st := store.NewMemory()
	run, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "i"})
	srv := NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.OperatorToken = "op"
	req := httptest.NewRequest(http.MethodGet, "/v0/runs/"+run.ID+"/stream", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("code=%d", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		t.Fatalf("ct=%s", ct)
	}
}

func TestGateSSEOperatorOK(t *testing.T) {
	st := store.NewMemory()
	run, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "i"})
	_ = st.UpdateRun(run.ID, store.StatusSucceeded, "done", "")
	srv := NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.OperatorToken = "op"
	req := httptest.NewRequest(http.MethodGet, "/v0/runs/"+run.ID+"/stream", nil)
	req.Header.Set("Authorization", "Bearer op")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("code=%d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("ct=%s", rr.Header().Get("Content-Type"))
	}
}
```

`TestGateOffAllowsAnonymous` 里 `POST /v0/runs` 的 200 依赖现有 `handlePostRun`（缺 agent 可能 400）。查看 `handlePostRun`：无 agent 时若 DefaultAgentID 空可能失败。测试里 body 带 `"agent_id":"a"`，Store 无该 agent 时现有代码仍会建 Run（以测试里 fakeRunner 为准）。若现网返回 400，把断言改成「不是 401/403」且门关着不要求 Authorization。实现时以**现有无门禁行为**为准：与 `TestCreateRun` 同样的最小 body。若 `agent_id` 未知仍 200，保持 200；若 400，断言 `rr.Code != 401 && rr.Code != 403`。

- [ ] **步骤 2：运行确认失败**

```powershell
$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH
go test ./internal/api -count=1 -run TestGate
```

预期：FAIL（无 `OperatorToken` / `GET /v0/me` / `gate_enabled`）。

- [ ] **步骤 3：最少实现**

`Server` 增加：

```go
OperatorToken string
AdminToken    string
```

`routes()` 增加：`s.mux.HandleFunc("GET /v0/me", s.handleMe)`。

`Handler()`：

```go
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r2, ok := s.authorize(w, r)
		if !ok {
			return
		}
		s.mux.ServeHTTP(w, r2)
	})
}
```

`authorize`：

1. `path == "/healthz"` 或 `strings.HasPrefix(path, "/ui/")` 或 `path == "/ui"` → 原样放行
2. `tok := controlplane.Tokens{Operator: s.OperatorToken, Admin: s.AdminToken}`
3. `!tok.Enabled()` → 放行（不写 role）
4. `min := controlplane.MinRole(r.Method, r.URL.Path)`
5. `min == RoleNone` → 放行（公开 `/v0/ui-config`）
6. `role, ok := controlplane.Authenticate(r.Header.Get("Authorization"), tok)`；`!ok` → `writeError(401, "unauthorized", "需要控制面口令")`，返回 false
7. `!role.AtLeast(min)` → `writeError(403, "forbidden", "需要管理员口令")`
8. `return r.WithContext(controlplane.WithRole(r.Context(), role)), true`

`handleUIConfig`：

```go
type uiConfig struct {
	AgentID     string `json:"agent_id"`
	GateEnabled bool   `json:"gate_enabled"`
}

func (s *Server) handleUIConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, uiConfig{
		AgentID:     s.DefaultAgentID,
		GateEnabled: controlplane.Tokens{Operator: s.OperatorToken, Admin: s.AdminToken}.Enabled(),
	})
}
```

`handleMe`：`writeJSON(200, map[string]string{"role": string(controlplane.RoleFrom(r.Context()))})`。门关着时 role 为 `""`。JSON 必须是 `"role":""` 而不是省略字段。

不要给 401 加 `WWW-Authenticate`。

- [ ] **步骤 4：测试通过**

```powershell
$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH
go test ./internal/api -count=1
```

预期：PASS（含全部旧测试：门关着时 `Handler` 行为不变）。

- [ ] **步骤 5：Commit**

```powershell
git add internal/api/server.go internal/api/server_gate_test.go
git commit -m "feat(api): 控制面口令门禁与 /v0/me"
```

---

### 任务 3：配置、bootstrap、开箱 YAML

**文件：**
- 修改：`internal/config/config.go`（`ControlPlane` 结构）
- 修改：`internal/config/docker_yaml_test.go`（断言两把口令为空）
- 创建：`internal/config/control_plane_test.go`（Load YAML 字段）
- 修改：`internal/bootstrap/bootstrap.go`（`newAPIServer` 注入解析后的口令）
- 创建：`internal/bootstrap/control_plane_test.go`
- 修改：`configs/default.yaml`、`configs/docker.yaml`

- [ ] **步骤 1：写失败测试**

`internal/config/control_plane_test.go`：

```go
package config_test

import (
	"testing"

	"github.com/rebornace/baize/internal/config"
)

func TestLoadControlPlaneTokens(t *testing.T) {
	path := writeConfig(t, "control_plane:\n  operator_token: env:BAIZE_OPERATOR_TOKEN\n  admin_token: env:BAIZE_ADMIN_TOKEN\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ControlPlane.OperatorToken != "env:BAIZE_OPERATOR_TOKEN" {
		t.Fatalf("op=%q", cfg.ControlPlane.OperatorToken)
	}
	if cfg.ControlPlane.AdminToken != "env:BAIZE_ADMIN_TOKEN" {
		t.Fatalf("adm=%q", cfg.ControlPlane.AdminToken)
	}
}
```

`docker_yaml_test.go` 的两个现有测试末尾各加：

```go
if cfg.ControlPlane.OperatorToken != "" || cfg.ControlPlane.AdminToken != "" {
	t.Fatalf("open-box control_plane must be empty: %+v", cfg.ControlPlane)
}
```

`internal/bootstrap/control_plane_test.go`：用最小 YAML + `t.Setenv` 调导出的解析或通过 `newAPIServer`（若 `newAPIServer` 未导出，在 `bootstrap` 包测试文件用同包访问）。断言：环境变量未设置时 `newAPIServer` **不返回 error**；`srv.OperatorToken==""`；再 `t.Setenv("BAIZE_OPERATOR_TOKEN", "op1")` 且 YAML 为 `operator_token: env:BAIZE_OPERATOR_TOKEN` 时 `srv.OperatorToken=="op1"`。

最小 cfg 可手写 `config.Config{Listen: ":0", Store: {Driver: "memory"}, Agent: {ID: "a"}, Connector: {Type: "openapi", Spec: "examples/mock-ticket/openapi.yaml", BaseURL: "http://127.0.0.1:1"}, MockTicket: {Listen: "off"}}`，避免真听端口。`newAPIServer` 会 `registerConnector` 打 base_url；openapi 注册不需要上游存活。`MockTicket.Listen: "off"` 仅 `Run()` 使用；`newAPIServer` 仍会注册 connector。

若 `newAPIServer` 因 spec 路径失败，测试文件放在 `internal/bootstrap`，工作目录为包目录时 spec 相对仓库根：用 `filepath.Join("..", "..", "examples", "mock-ticket", "openapi.yaml")` 填 `cfg.Connector.Spec`。

- [ ] **步骤 2：运行确认失败**

```powershell
$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH
go test ./internal/config ./internal/bootstrap -count=1 -run "ControlPlane|DockerYAML|DefaultYAML"
```

预期：FAIL（无 `ControlPlane` 字段或 YAML 非空尚未加注释空段——空结构体应已是 `""`，docker 测试在加断言后若 YAML 误写了值会失败；先实现字段）。

- [ ] **步骤 3：最少实现**

`config.Config`：

```go
ControlPlane struct {
	OperatorToken string `yaml:"operator_token"`
	AdminToken    string `yaml:"admin_token"`
} `yaml:"control_plane"`
```

`configs/default.yaml` 与 `docker.yaml` 在 `ui:` 块附近增加（值必须空）：

```yaml
control_plane:
  operator_token: ""   # 或 env:BAIZE_OPERATOR_TOKEN
  admin_token: ""      # 或 env:BAIZE_ADMIN_TOKEN
```

`newAPIServer` 在 `api.NewServer` 之后：

```go
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
srv.OperatorToken = op
srv.AdminToken = adm
```

- [ ] **步骤 4：测试通过**

```powershell
$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH
go test ./internal/config ./internal/bootstrap ./internal/api -count=1
```

预期：PASS。

- [ ] **步骤 5：Commit**

```powershell
git add internal/config internal/bootstrap configs/default.yaml configs/docker.yaml
git commit -m "feat(config): 可选控制面口令并在启动时注入"
```

---

### 任务 4：Chat UI 解锁、按角色导航、fetch SSE

**文件：**
- 创建：`web/chat/src/controlAuth.ts`
- 创建：`web/chat/src/parseSSE.ts`
- 创建：`web/chat/src/parseSSE.test.ts`
- 创建：`web/chat/src/settingsNav.ts`
- 创建：`web/chat/src/settingsNav.test.ts`
- 创建：`web/chat/src/pages/UnlockPage.tsx`
- 创建：`web/chat/src/pages/GateRoot.tsx`
- 修改：`web/chat/src/api.ts`
- 修改：`web/chat/src/main.tsx`
- 修改：`web/chat/src/pages/ChatPage.tsx`
- 修改：`web/chat/src/pages/SettingsLayout.tsx`
- 修改：`web/chat/src/style.css`（解锁页沿用现有浅色变量，不要新主题）
- 构建：`internal/ui/dist/**`

- [ ] **步骤 1：写失败的前端测试**

`parseSSE.test.ts`：把下面这段文本喂给 `consumeSSE(buffer)`（或你实现的增量解析函数），断言得到一条默认事件（`id=1`，data 含 `llm.message`）和一条 `run.ended`。

```
id: 1
data: {"type":"llm.message","timestamp":"t","data":{"content":"hi"}}

event: run.ended
data: {"status":"succeeded"}

```

`settingsNav.test.ts`：

```ts
import { describe, expect, it } from 'vitest'
import { settingsNavItems } from './settingsNav'

describe('settingsNavItems', () => {
  it('operator only sees identities', () => {
    expect(settingsNavItems('operator')).toEqual([
      { to: '/settings/identities', label: '账号' },
    ])
  })
  it('admin sees all', () => {
    expect(settingsNavItems('admin').map((x) => x.to)).toEqual([
      '/settings/tools',
      '/settings/identities',
      '/settings/mcp',
      '/settings/plugins',
    ])
  })
})
```

- [ ] **步骤 2：运行确认失败**

```powershell
cd web/chat
npm test
```

预期：FAIL（模块不存在）。`npm test` 即 `vitest run`。若 `node_modules` 缺失：`npm ci --registry https://registry.npmmirror.com`。

- [ ] **步骤 3：实现前端**

`controlAuth.ts`：

```ts
export const CONTROL_TOKEN_KEY = 'baize.control_token'

export function readControlToken(): string {
  return localStorage.getItem(CONTROL_TOKEN_KEY)?.trim() ?? ''
}

export function writeControlToken(token: string): void {
  localStorage.setItem(CONTROL_TOKEN_KEY, token)
}

export function clearControlToken(): void {
  localStorage.removeItem(CONTROL_TOKEN_KEY)
}

export function authHeaders(gateEnabled: boolean): HeadersInit {
  if (!gateEnabled) return {}
  const token = readControlToken()
  if (!token) return {}
  return { Authorization: `Bearer ${token}` }
}
```

`api.ts`：增加 `let gateEnabled = false` 与 `setGateEnabled(v: boolean)`。所有 `fetch`（含 `openRunStream`）合并 `authHeaders(gateEnabled)`。`getUIConfig` **不要**带头（公开）。

```ts
export async function getUIConfig(): Promise<{ agent_id: string; gate_enabled: boolean }> {
  const res = await fetch('/v0/ui-config')
  return parseJSON(res)
}

export async function getMe(): Promise<{ role: string }> {
  const res = await fetch('/v0/me', { headers: { ...authHeaders(true) } })
  return parseJSON(res)
}
```

`openRunStream`：删除 `EventSource`。用 `fetch(url, { headers: authHeaders(gateEnabled) })`；若 `!res.ok` 或 Content-Type 不含 `event-stream`，调用 `onFatal`。用 `res.body.getReader()` + `parseSSE.ts` 增量解析：无 `event:` 的 data 帧 → `onEvent(JSON.parse(data), id)`；`event: run.ended` → `onEnded(status)` 并停止。`AbortController` 作为返回的 cancel 函数。门关着也走 fetch，不要保留 EventSource 分支。

`GateRoot.tsx`：挂载时 `getUIConfig()`。`gate_enabled===false` → `setGateEnabled(false)`，渲染 `children`。`true` 且无 token → `UnlockPage`。有 token → `getMe()`，401 则 `clearControlToken` 并显示解锁页；成功则把 `role` 放 React context（`createContext`，文件可放 `web/chat/src/gateContext.ts`：`role: 'operator' | 'admin'`）。

`UnlockPage.tsx`：一个 password input + 按钮「进入」。提交：`writeControlToken` → `getMe()`；失败显示「口令不对」并 clear。成功后由 GateRoot 重读。

`main.tsx`：`GateRoot` 包住 `Routes`。设置子路由：`tools` / `mcp` / `plugins` 在 `role!=='admin'` 时 `<Navigate to="/settings/identities" replace />`。`/settings` index：admin → tools，operator → identities。

`ChatPage.tsx`：左下角 `role==='admin'` 时链接「设置」到 `/settings/tools`，否则「账号」到 `/settings/identities`。增加「退出」按钮：`clearControlToken()` 后 `window.location.assign('/ui/')`（或回调 GateRoot 回到解锁页）。仅当 `gate_enabled` 时显示退出。

`SettingsLayout.tsx`：用 `settingsNavItems(role)` 渲染导航。

`getUIConfig` 现返回 `gate_enabled`；`ChatPage` 里读取 agent_id 的代码改为兼容新字段。

- [ ] **步骤 4：测试与构建**

```powershell
cd web/chat
npm test
npm run build
cd ..\..
$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH
go test ./... -count=1
```

预期：vitest PASS；`internal/ui/dist` 更新；Go 全绿。

- [ ] **步骤 5：Commit**

```powershell
git add web/chat internal/ui/dist
git commit -m "feat(ui): 控制面解锁页与按角色显隐设置"
```

---

### 任务 5：README 与架构草案

**文件：**
- 修改：`README.md`
- 修改：`README.zh-CN.md`
- 修改：`docs/architecture-and-plugin-protocol.md`

- [ ] **步骤 1：改文档（无单独失败测试；对照规格 §10）**

中文 `README.zh-CN.md`：

- 「操作员界面」：若配置了控制面口令，打开 `/ui` 先解锁；操作员只能进账号，改 Tools 需要管理员口令。左下角操作员显示「账号」。
- curl 示例下加一段（不要改默认 30 秒跑通，默认仍无口令）：

```markdown
若 `control_plane` 配置了口令，上述 `/v0` 请求需带：

`Authorization: Bearer <操作员或管理员口令>`

改 Connector / Tools 只能用管理员口令。这与对话里登录下游系统不是同一把钥匙。
```

- 设置说明：Tools「需要登录」仅管理员；账号页操作员可用。

英文 `README.md` 同一结构。中英文之间空格；中文用全角标点。

`docs/architecture-and-plugin-protocol.md` §3 表格下加一句：

可选控制面口令（`control_plane.operator_token` / `admin_token`）：挡住 Runtime 的 `/v0`。操作员可跑 Run / HITL / 会话身份；管理员可改 Agent、Connector、Tools。不是下游业务 IAM，也不是多租户 SSO。

- [ ] **步骤 2：全量测试**

```powershell
$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH
go test ./... -count=1
```

预期：PASS。

- [ ] **步骤 3：Commit**

```powershell
git add README.md README.zh-CN.md docs/architecture-and-plugin-protocol.md
git commit -m "docs(auth): 说明可选控制面操作员与管理员口令"
```

---

## 规格覆盖自检

| 规格 | 任务 |
|------|------|
| §1 没配口令行为不变 | 2（TestGateOff）、3（YAML 空） |
| §1 401/403/管理员超集 | 2 |
| §1 `/ui` 解锁与显隐 | 4 |
| §1 口令不进轨迹 | 2 不把 token 写入 Store；9 不新增事件字段 |
| §3 `env:` 空不启动失败 | 1 + 3 |
| §4 中间件 | 2 |
| §5 ACL | 1 MinRole + 2 HTTP |
| §6 ui-config / me / 错误体 | 2 |
| §7 EventSource→fetch、localStorage 键 | 4 |
| §8 不改会话登录 / HITL / mode | 全计划不碰那些包的行为 |
| §10 文档 | 5 |
| 未知 `/v0` 默认管理员 | 1 `TestMinRoleTable` `/v0/unknown` |

无占位符。类型名全程：`controlplane.Tokens`、`RoleOperator` / `RoleAdmin`、`Server.OperatorToken` / `AdminToken`、`CONTROL_TOKEN_KEY`、`gate_enabled`、错误 code `unauthorized` / `forbidden`。
