# 多模型配置与对话选模型（X2）实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法跟踪进度。

**目标：** 管理员在设置页维护多个命名模型 profile（存库、热切换、不重启），聊天框每条消息发送前可下拉选择模型，未选/无人值守入口落默认模型。

**架构：** 新增 `store.ModelProfile`（表 `model_profiles`，三驱动）+ `Run.ModelProfileID` 列；新增 `llm.Switch`（实现 `llm.Provider`，按 ctx 中的 profile ID 解析/缓存 Provider，编辑后按 UpdatedAt 热重建）；bootstrap 用 YAML `llm` 段种子默认 profile 并把同一个 Switch 注入 engine/server/微信 channel；新增 admin 管理 API + operator 只读列表；前端新增模型设置页与聊天下拉。

**技术栈：** Go 1.25（`net/http`、`database/sql`、modernc sqlite / pgx）、React + TypeScript + vitest。

**规格：** `docs/superpowers/specs/2026-08-30-multi-model-profiles-design.md`

**环境注意（Windows）：** go 不在默认 PATH；运行 go 命令前设
`$env:Path="C:\Users\Administrator\.local\go1.25.0\bin;"+$env:Path; $env:GOTOOLCHAIN="local"; $env:GOPROXY="https://goproxy.cn,direct"`。

---

## 文件结构

- 创建 `internal/store/model_profiles.go` — profile 类型、脱敏、Memory 与 SQLStore 的 CRUD（SQL 方言经现有 `s.exec/queryRow`，占位符用 `?`，与 mcp_export.go 同模式）。
- 创建 `internal/store/model_profiles_test.go` — 三驱动无关的 Memory 测试 + sqlite round-trip。
- 修改 `internal/store/store.go` — `ModelProfile` 类型、Store 接口 5 个方法、`Run.ModelProfileID`、`CreateRunInput.ModelProfileID`。
- 修改 `internal/store/memory.go` — `modelProfiles map`、runs 落 `ModelProfileID`。
- 修改 `internal/store/sqlite.go` / `postgres.go` — 建表 `model_profiles`、runs 加列迁移、CreateRun/GetRun 读写 `model_profile_id`。
- 创建 `internal/llm/switch.go` + `switch_test.go` — `Switch` Provider、ctx key、ProfileSource 接口、缓存与热重建。
- 修改 `internal/run/engine.go` — `runLoop` 调 Chat 前往 ctx 注入 profile ID。
- 创建 `internal/api/server_models.go` + `server_models_test.go` — 管理 API + 脱敏 + 校验。
- 修改 `internal/api/server.go` — 路由注册、`handlePostRun` 读 `model_profile_id`、startRunInput 透传。
- 修改 `internal/api/run_start.go` — `startRunInput.ModelProfileID` → `CreateRunInput`。
- 修改 `internal/controlplane/acl.go` — 模型路由角色。
- 修改 `internal/bootstrap/bootstrap.go` — 种子 profile、构建 Switch 并注入。
- 创建 `web/chat/src/pages/ModelSettings.tsx` + `.test.ts`；修改 `web/chat/src/api.ts`、`settingsNav.ts`、`main.tsx`、`ChatPage.tsx`。

---

## 任务 1：Store — ModelProfile 类型、接口与 Memory 实现

**文件：**
- 修改：`internal/store/store.go`（类型 + 接口 + Run 字段）
- 修改：`internal/store/memory.go`（map + runs 字段）
- 创建：`internal/store/model_profiles.go`（脱敏 + Memory CRUD）
- 测试：`internal/store/model_profiles_test.go`

- [ ] **步骤 1：在 store.go 增加类型与 Run 字段（先写失败测试）**

在 `internal/store/store.go` 的 `MCPExportKey` 类型附近新增：

```go
// ModelProfile is a named, selectable LLM configuration. APIKey is stored in
// plaintext locally (same trust tier as a Postgres DSN password) and is never
// returned verbatim by the API (see RedactAPIKey).
type ModelProfile struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Provider        string    `json:"provider"`
	BaseURL         string    `json:"base_url"`
	Model           string    `json:"model"`
	APIKey          string    `json:"api_key,omitempty"`
	APIKeyEnv       string    `json:"api_key_env,omitempty"`
	DisableThinking bool      `json:"disable_thinking"`
	SupportsVision  bool      `json:"supports_vision"`
	IsDefault       bool      `json:"is_default"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
```

给 `Run` 加字段（`IdentityID` 之后）：

```go
	ModelProfileID  string    `json:"model_profile_id,omitempty"`
```

给 `CreateRunInput` 加字段（`IdentityID` 之后）：

```go
	ModelProfileID  string
```

在 `Store` 接口（`LookupMCPExportKeyByHash` 之后）加方法：

```go
	UpsertModelProfile(p ModelProfile) (ModelProfile, error)
	GetModelProfile(id string) (ModelProfile, error)
	ListModelProfiles() ([]ModelProfile, error)
	DeleteModelProfile(id string) error
	SetDefaultModelProfile(id string) error
```

- [ ] **步骤 2：写失败测试** `internal/store/model_profiles_test.go`

```go
package store

import "testing"

func TestMemoryModelProfileCRUDAndDefault(t *testing.T) {
	s := NewMemory()

	p, err := s.UpsertModelProfile(ModelProfile{
		Name: "主力", Provider: "openai_compatible", BaseURL: "https://x/v1",
		Model: "m1", APIKey: "sk-secret-1234",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if p.ID == "" || p.CreatedAt.IsZero() {
		t.Fatalf("id/createdAt not set: %+v", p)
	}
	if err := s.SetDefaultModelProfile(p.ID); err != nil {
		t.Fatalf("set default: %v", err)
	}
	p2, _ := s.UpsertModelProfile(ModelProfile{
		Name: "廉价", Provider: "openai_compatible", BaseURL: "https://y/v1",
		Model: "m2", APIKeyEnv: "KEY2",
	})
	if err := s.SetDefaultModelProfile(p2.ID); err != nil {
		t.Fatalf("set default 2: %v", err)
	}

	list, _ := s.ListModelProfiles()
	defaults := 0
	for _, m := range list {
		if m.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("want exactly 1 default, got %d", defaults)
	}

	if got, err := s.GetModelProfile(p.ID); err != nil || got.APIKey != "sk-secret-1234" {
		t.Fatalf("store should keep raw key internally; got %q err=%v", got.APIKey, err)
	}

	if err := s.DeleteModelProfile(p2.ID); err == nil {
		t.Fatalf("deleting the default profile must be rejected")
	}
	if err := s.DeleteModelProfile(p.ID); err != nil {
		t.Fatalf("delete non-default: %v", err)
	}
	if _, err := s.GetModelProfile(p.ID); err == nil {
		t.Fatalf("expected not-found after delete")
	}
}

func TestMemoryUpsertRejectsEmptyNameAndDuplicate(t *testing.T) {
	s := NewMemory()
	if _, err := s.UpsertModelProfile(ModelProfile{Provider: "openai_compatible", Model: "m"}); err == nil {
		t.Fatal("empty name must be rejected")
	}
	if _, err := s.UpsertModelProfile(ModelProfile{Name: "dup", Provider: "openai_compatible", Model: "m", BaseURL: "u"}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := s.UpsertModelProfile(ModelProfile{Name: "dup", Provider: "openai_compatible", Model: "m2", BaseURL: "u2"}); err == nil {
		t.Fatal("duplicate name (different id) must be rejected")
	}
}
```

- [ ] **步骤 3：运行测试确认失败**

运行：`go test ./internal/store/ -run ModelProfile -count=1`
预期：编译失败（`s.UpsertModelProfile` 未定义）。

- [ ] **步骤 4：实现 — memory.go 加 map**

`internal/store/memory.go`：在 `Memory` 结构体（`mcpExportKeys` 附近）加：

```go
	modelProfiles   map[string]ModelProfile
```

`NewMemory()` 初始化 map 的地方加：

```go
		modelProfiles:   map[string]ModelProfile{},
```

CreateRun 落库处（`s.runs[...] = &Run{...}` 或等价构造）补 `ModelProfileID: in.ModelProfileID`。

- [ ] **步骤 5：实现 — model_profiles.go（脱敏 + Memory CRUD）**

创建 `internal/store/model_profiles.go`：

```go
package store

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RedactAPIKey masks a secret for API responses: keeps first 3 and last 4
// characters. Empty input returns "". Callers must never persist the redacted
// form back as the real key.
func RedactAPIKey(k string) string {
	k = strings.TrimSpace(k)
	if k == "" {
		return ""
	}
	if len(k) <= 8 {
		return "••••"
	}
	return k[:3] + "…" + k[len(k)-4:]
}

// IsRedactedAPIKey reports whether s looks like a redacted placeholder echoed
// back from the UI (contains the ellipsis) and therefore must not overwrite a
// stored key.
func IsRedactedAPIKey(s string) bool {
	return strings.Contains(s, "…") || strings.Contains(s, "•")
}

func (s *Memory) UpsertModelProfile(p ModelProfile) (ModelProfile, error) {
	if strings.TrimSpace(p.Name) == "" {
		return ModelProfile{}, fmt.Errorf("model profile name is required")
	}
	if strings.TrimSpace(p.Provider) == "" {
		p.Provider = "openai_compatible"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if p.ID == "" {
		p.ID = "mp_" + uuid.NewString()
		p.CreatedAt = now
		for _, ex := range s.modelProfiles {
			if ex.Name == p.Name {
				return ModelProfile{}, fmt.Errorf("model profile name %q already exists", p.Name)
			}
		}
	} else {
		ex, ok := s.modelProfiles[p.ID]
		if !ok {
			return ModelProfile{}, fmt.Errorf("model profile not found")
		}
		p.CreatedAt = ex.CreatedAt
		for _, other := range s.modelProfiles {
			if other.ID != p.ID && other.Name == p.Name {
				return ModelProfile{}, fmt.Errorf("model profile name %q already exists", p.Name)
			}
		}
		p.IsDefault = ex.IsDefault // default only changes via SetDefaultModelProfile
	}
	p.UpdatedAt = now
	s.modelProfiles[p.ID] = p
	return p, nil
}

func (s *Memory) GetModelProfile(id string) (ModelProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.modelProfiles[id]
	if !ok {
		return ModelProfile{}, fmt.Errorf("model profile not found")
	}
	return p, nil
}

func (s *Memory) ListModelProfiles() ([]ModelProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ModelProfile, 0, len(s.modelProfiles))
	for _, p := range s.modelProfiles {
		out = append(out, p)
	}
	return out, nil
}
func (s *Memory) DeleteModelProfile(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.modelProfiles[id]
	if !ok {
		return fmt.Errorf("model profile not found")
	}
	if p.IsDefault {
		return fmt.Errorf("cannot delete the default model profile; set another as default first")
	}
	delete(s.modelProfiles, id)
	return nil
}

func (s *Memory) SetDefaultModelProfile(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.modelProfiles[id]; !ok {
		return fmt.Errorf("model profile not found")
	}
	for k, p := range s.modelProfiles {
		p.IsDefault = (k == id)
		s.modelProfiles[k] = p
	}
	return nil
}
```

- [ ] **步骤 6：运行测试确认通过**

运行：`go test ./internal/store/ -run ModelProfile -count=1`
预期：PASS。

- [ ] **步骤 7：Commit**

```bash
git add internal/store/store.go internal/store/memory.go internal/store/model_profiles.go internal/store/model_profiles_test.go
git commit -m "feat(store): ModelProfile 类型、接口与 Memory CRUD"
```

---

## 任务 2：SQL 存储 — model_profiles 表 + runs.model_profile_id 列（sqlite + postgres）

**文件：**
- 修改：`internal/store/sqlite.go`（schema、迁移、CreateRun/GetRun 读写）
- 修改：`internal/store/postgres.go`（schema、迁移）
- 创建：`internal/store/model_profiles_sql.go`（SQLStore CRUD）
- 测试：`internal/store/model_profiles_test.go`（追加 sqlite round-trip + runs 列）

说明：`SQLite = SQLStore`，postgres 也返回 `*SQLStore`；两者共用 `model_profiles_sql.go` 的方法，仅建表 DDL/占位符差异已由现有 `s.exec`/`s.queryRow` 抽象处理（mcp_export.go 即此模式，占位符统一用 `?`）。

- [ ] **步骤 1：写失败测试（追加到 model_profiles_test.go）**

```go
import (
	"path/filepath"
	"testing"
)

func newSQLiteProfileStore(t *testing.T) *SQLStore {
	t.Helper()
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestSQLiteModelProfileRoundTrip(t *testing.T) {
	s := newSQLiteProfileStore(t)
	p, err := s.UpsertModelProfile(ModelProfile{
		Name: "主力", Provider: "openai_compatible", BaseURL: "https://x/v1",
		Model: "m1", APIKey: "sk-secret-1234", SupportsVision: true,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := s.SetDefaultModelProfile(p.ID); err != nil {
		t.Fatalf("default: %v", err)
	}
	got, err := s.GetModelProfile(p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.APIKey != "sk-secret-1234" || !got.SupportsVision || !got.IsDefault {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// edit: redacted key must not overwrite; UpdatedAt must advance
	updated, err := s.UpsertModelProfile(ModelProfile{
		ID: p.ID, Name: "主力", Provider: "openai_compatible",
		BaseURL: "https://x/v1", Model: "m1b", APIKey: RedactAPIKey("sk-secret-1234"),
		SupportsVision: true,
	})
	if err != nil {
		t.Fatalf("upsert edit: %v", err)
	}
	if updated.APIKey != "sk-secret-1234" {
		t.Fatalf("redacted key overwrote stored key: %q", updated.APIKey)
	}
	if updated.Model != "m1b" {
		t.Fatalf("model not updated: %q", updated.Model)
	}

	if err := s.DeleteModelProfile(p.ID); err == nil {
		t.Fatal("deleting default must be rejected on SQL store")
	}
}

func TestSQLiteRunPersistsModelProfileID(t *testing.T) {
	s := newSQLiteProfileStore(t)
	s.UpsertAgent(Agent{ID: "a"})
	r, err := s.CreateRun(CreateRunInput{AgentID: "a", Input: "hi", ModelProfileID: "mp_123"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	got, err := s.GetRun(r.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.ModelProfileID != "mp_123" {
		t.Fatalf("model_profile_id not persisted: %q", got.ModelProfileID)
	}
}
```

- [ ] **步骤 2：运行确认失败**

运行：`go test ./internal/store/ -run "SQLiteModelProfile|SQLiteRunPersists" -count=1`
预期：FAIL（表不存在 / 列不存在）。

- [ ] **步骤 3：建表 DDL**

`internal/store/sqlite.go` 的 `sqliteSchema` 常量中，在 mcp_export_keys 之后追加：

```sql
CREATE TABLE IF NOT EXISTS model_profiles (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  provider TEXT,
  base_url TEXT,
  model TEXT,
  api_key TEXT,
  api_key_env TEXT,
  disable_thinking INTEGER,
  supports_vision INTEGER,
  is_default INTEGER,
  created_at TEXT,
  updated_at TEXT
);
```

`internal/store/postgres.go` 的 schema 常量中追加（同列，类型用 `TEXT`/`BOOLEAN`，与该文件既有风格一致）：

```sql
CREATE TABLE IF NOT EXISTS model_profiles (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  provider TEXT,
  base_url TEXT,
  model TEXT,
  api_key TEXT,
  api_key_env TEXT,
  disable_thinking BOOLEAN,
  supports_vision BOOLEAN,
  is_default BOOLEAN,
  created_at TEXT,
  updated_at TEXT
);
```

- [ ] **步骤 4：runs 加列迁移**

`internal/store/sqlite.go` 的 `migrateRunsColumns` 列切片加入 `"model_profile_id"`：

```go
	for _, col := range []string{"conversation_id", "identity_id", "passthrough_json", "webhook_json", "model_profile_id"} {
```

postgres 若有等价 runs 列迁移（`ALTER TABLE ... ADD COLUMN IF NOT EXISTS`），同样加 `model_profile_id TEXT`；若 postgres runs 表在 CREATE 中已含 conversation_id 等列，则直接在其 `CREATE TABLE runs` 里加 `model_profile_id TEXT`。核对 `postgres.go` 现有 runs DDL 后按同风格处理。

- [ ] **步骤 5：CreateRun / GetRun 读写新列**

`internal/store/sqlite.go` CreateRun 的 INSERT（约 680 行）：列清单加 `model_profile_id`，VALUES 加一个 `?`，参数加 `r.ModelProfileID`：

```go
	`INSERT INTO runs (id, agent_id, input, status, output, error, created_at, hitl_json, conversation_id, identity_id, passthrough_json, webhook_json, model_profile_id)
	 VALUES (?, ?, ?, ?, '', '', ?, NULL, ?, ?, ?, ?, ?)`,
	r.ID, r.AgentID, r.Input, string(r.Status), r.CreatedAt.Format(time.RFC3339Nano),
	r.ConversationID, r.IdentityID, passthroughSQL, webhookSQL, r.ModelProfileID,
```

确保构造 `r` 时 `ModelProfileID: in.ModelProfileID`（与 Memory 一致）。

GetRun 的 SELECT（约 696 行）加列与扫描：

```go
	var modelProfileID sql.NullString
	err := s.queryRow(
		`SELECT id, agent_id, input, status, output, error, created_at, conversation_id, identity_id, passthrough_json, webhook_json, model_profile_id FROM runs WHERE id = ?`,
		id,
	).Scan(&r.ID, &r.AgentID, &r.Input, &status, &r.Output, &r.Error, &createdAt, &conversationID, &identityID, &passthroughSQL, &webhookSQL, &modelProfileID)
	// ... 既有解析之后：
	if modelProfileID.Valid {
		r.ModelProfileID = modelProfileID.String
	}
```

- [ ] **步骤 6：实现 SQLStore CRUD** — 创建 `internal/store/model_profiles_sql.go`

```go
package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const upsertModelProfileColumns = `name, provider, base_url, model, api_key, api_key_env, disable_thinking, supports_vision, is_default, created_at, updated_at`

func scanModelProfile(scanner interface{ Scan(...any) error }) (ModelProfile, error) {
	var p ModelProfile
	var createdAt, updatedAt string
	var disableThinking, supportsVision, isDefault sql.NullBool
	if err := scanner.Scan(&p.ID, &p.Name, &p.Provider, &p.BaseURL, &p.Model, &p.APIKey,
		&p.APIKeyEnv, &disableThinking, &supportsVision, &isDefault, &createdAt, &updatedAt); err != nil {
		return ModelProfile{}, err
	}
	p.DisableThinking = disableThinking.Bool
	p.SupportsVision = supportsVision.Bool
	p.IsDefault = isDefault.Bool
	if t, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
		p.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339Nano, updatedAt); err == nil {
		p.UpdatedAt = t
	}
	return p, nil
}

func (s *SQLStore) ListModelProfiles() ([]ModelProfile, error) {
	rows, err := s.query(`SELECT id, ` + upsertModelProfileColumns + ` FROM model_profiles ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelProfile
	for rows.Next() {
		p, err := scanModelProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *SQLStore) GetModelProfile(id string) (ModelProfile, error) {
	row := s.queryRow(`SELECT id, ` + upsertModelProfileColumns + ` FROM model_profiles WHERE id = ?`, id)
	p, err := scanModelProfile(row)
	if err == sql.ErrNoRows {
		return ModelProfile{}, fmt.Errorf("model profile not found")
	}
	return p, err
}

func (s *SQLStore) UpsertModelProfile(p ModelProfile) (ModelProfile, error) {
	if strings.TrimSpace(p.Name) == "" {
		return ModelProfile{}, fmt.Errorf("model profile name is required")
	}
	if strings.TrimSpace(p.Provider) == "" {
		p.Provider = "openai_compatible"
	}
	now := time.Now().UTC()
	if p.ID == "" {
		p.ID = "mp_" + uuid.NewString()
		p.CreatedAt = now
		p.UpdatedAt = now
		_, err := s.exec(
			`INSERT INTO model_profiles (id, `+upsertModelProfileColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			p.ID, p.Name, p.Provider, p.BaseURL, p.Model, p.APIKey, p.APIKeyEnv,
			p.DisableThinking, p.SupportsVision, p.IsDefault,
			p.CreatedAt.Format(time.RFC3339Nano), p.UpdatedAt.Format(time.RFC3339Nano),
		)
		if err != nil {
			if isUniqueViolation(err) {
				return ModelProfile{}, fmt.Errorf("model profile name %q already exists", p.Name)
			}
			return ModelProfile{}, err
		}
		return p, nil
	}

	existing, err := s.GetModelProfile(p.ID)
	if err != nil {
		return ModelProfile{}, err
	}
	// Redacted/empty key must not overwrite the stored secret.
	if p.APIKey == "" || IsRedactedAPIKey(p.APIKey) {
		p.APIKey = existing.APIKey
	}
	p.IsDefault = existing.IsDefault // default only via SetDefaultModelProfile
	p.CreatedAt = existing.CreatedAt
	p.UpdatedAt = now
	_, err = s.exec(
		`UPDATE model_profiles SET name=?, provider=?, base_url=?, model=?, api_key=?, api_key_env=?,
		   disable_thinking=?, supports_vision=?, is_default=?, updated_at=? WHERE id=?`,
		p.Name, p.Provider, p.BaseURL, p.Model, p.APIKey, p.APIKeyEnv,
		p.DisableThinking, p.SupportsVision, p.IsDefault,
		p.UpdatedAt.Format(time.RFC3339Nano), p.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ModelProfile{}, fmt.Errorf("model profile name %q already exists", p.Name)
		}
		return ModelProfile{}, err
	}
	return p, nil
}

func (s *SQLStore) DeleteModelProfile(id string) error {
	existing, err := s.GetModelProfile(id)
	if err != nil {
		return err
	}
	if existing.IsDefault {
		return fmt.Errorf("cannot delete the default model profile; set another as default first")
	}
	_, err = s.exec(`DELETE FROM model_profiles WHERE id = ?`, id)
	return err
}

func (s *SQLStore) SetDefaultModelProfile(id string) error {
	if _, err := s.GetModelProfile(id); err != nil {
		return err
	}
	if _, err := s.exec(`UPDATE model_profiles SET is_default = ?`, false); err != nil {
		return err
	}
	_, err := s.exec(`UPDATE model_profiles SET is_default = ? WHERE id = ?`, true, id)
	return err
}
```

注意：Memory 的 Upsert 也应在编辑分支应用「redacted/empty key 不覆盖」——在任务 1 的 Memory Upsert 更新分支中，于加锁后补充：

```go
		if p.APIKey == "" || IsRedactedAPIKey(p.APIKey) {
			p.APIKey = ex.APIKey
		}
```

- [ ] **步骤 7：运行测试确认通过**

运行：`go test ./internal/store/ -count=1`
预期：PASS（含 Memory 与 SQLite；postgres 无 DSN 时跳过）。

- [ ] **步骤 8：gofmt + Commit**

运行：`gofmt -l internal/store`（应无输出）
```bash
git add internal/store/sqlite.go internal/store/postgres.go internal/store/model_profiles_sql.go internal/store/model_profiles.go internal/store/model_profiles_test.go internal/store/memory.go
git commit -m "feat(store): model_profiles 表与 runs.model_profile_id 列（sqlite/postgres）"
```

---

## 任务 3：llm.Switch — 按 run 解析 Provider 的原子开关

**文件：**
- 创建：`internal/llm/switch.go`
- 测试：`internal/llm/switch_test.go`

`Switch` 实现 `llm.Provider`。它不直接持有模型配置，而是通过一个 `ProfileSource` 接口按 ID 取 profile；Provider 实例按 profile ID 缓存，并以 profile 的 `UpdatedAt` 判定是否需要热重建。

- [ ] **步骤 1：写失败测试** `internal/llm/switch_test.go`

```go
package llm

import (
	"context"
	"testing"
	"time"
)

// fakeProfileSource is an in-memory ProfileSource for tests.
type fakeProfileSource struct {
	def  ModelProfileView
	byID map[string]ModelProfileView
}

func (f *fakeProfileSource) DefaultModelProfile() (ModelProfileView, error) { return f.def, nil }
func (f *fakeProfileSource) ModelProfileByID(id string) (ModelProfileView, error) {
	p, ok := f.byID[id]
	if !ok {
		return ModelProfileView{}, context.Canceled // any non-nil error signals missing
	}
	return p, nil
}

// recordingProvider captures the model name it was built with.
type recordingProvider struct {
	model    string
	vision   bool
}

func (r *recordingProvider) Chat(ctx context.Context, messages []Message, tools []ToolSpec) (Message, error) {
	return Message{Content: "from:" + r.model}, nil
}
func (r *recordingProvider) SupportsVision() bool { return r.vision }

func TestSwitchResolvesExplicitProfile(t *testing.T) {
	src := &fakeProfileSource{byID: map[string]ModelProfileView{}}
	sw := NewSwitch(src)
	sw.build = func(v ModelProfileView) Provider {
		return &recordingProvider{model: v.Model, vision: v.SupportsVision}
	}
	src.byID["mp_a"] = ModelProfileView{ID: "mp_a", Model: "model-A", SupportsVision: true, UpdatedAt: time.Now()}
	src.def = ModelProfileView{ID: "mp_def", Model: "model-def"}

	ctx := WithModelProfileID(context.Background(), "mp_a")
	msg, err := sw.Chat(ctx, nil, nil)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if msg.Content != "from:model-A" {
		t.Fatalf("did not use explicit profile: %q", msg.Content)
	}
	// Default profile in this test has zero-value SupportsVision=false.
	if sw.SupportsVision() {
		t.Fatal("SupportsVision should reflect default profile (false here)")
	}
}

func TestSwitchFallsBackToDefault(t *testing.T) {
	src := &fakeProfileSource{
		def:  ModelProfileView{ID: "mp_def", Model: "model-def", SupportsVision: true},
		byID: map[string]ModelProfileView{},
	}
	sw := NewSwitch(src)
	used := ""
	sw.build = func(v ModelProfileView) Provider {
		used = v.Model
		return &recordingProvider{model: v.Model, vision: v.SupportsVision}
	}

	// No profile id in ctx -> default.
	if _, err := sw.Chat(context.Background(), nil, nil); err != nil {
		t.Fatalf("chat: %v", err)
	}
	if used != "model-def" {
		t.Fatalf("want default model-def, used %q", used)
	}
	// Deleted profile id -> default.
	ctx := WithModelProfileID(context.Background(), "mp_gone")
	if _, err := sw.Chat(ctx, nil, nil); err != nil {
		t.Fatalf("chat with missing profile should fall back, got %v", err)
	}
	if used != "model-def" {
		t.Fatalf("missing profile should fall back to default, used %q", used)
	}
	if !sw.SupportsVision() {
		t.Fatal("SupportsVision must reflect default profile vision=true")
	}
}

func TestSwitchRebuildsOnUpdate(t *testing.T) {
	src := &fakeProfileSource{byID: map[string]ModelProfileView{}}
	sw := NewSwitch(src)
	models := []string{}
	sw.build = func(v ModelProfileView) Provider {
		models = append(models, v.Model)
		return &recordingProvider{model: v.Model}
	}
	t0 := time.Now()
	src.byID["mp_a"] = ModelProfileView{ID: "mp_a", Model: "v1", UpdatedAt: t0}
	src.def = src.byID["mp_a"]

	ctx := WithModelProfileID(context.Background(), "mp_a")
	sw.Chat(ctx, nil, nil)
	sw.Chat(ctx, nil, nil) // cached, no rebuild
	// profile edited: UpdatedAt advances.
	src.byID["mp_a"] = ModelProfileView{ID: "mp_a", Model: "v2", UpdatedAt: t0.Add(time.Second)}
	src.def = src.byID["mp_a"]
	sw.Chat(ctx, nil, nil)

	if len(models) != 2 || models[0] != "v1" || models[1] != "v2" {
		t.Fatalf("expected rebuild after update (v1 then v2), got %v", models)
	}
}
```

- [ ] **步骤 2：运行确认失败**

运行：`go test ./internal/llm/ -run Switch -count=1`
预期：编译失败（`NewSwitch`/`WithModelProfileID` 未定义）。

- [ ] **步骤 3：实现 switch.go**

```go
package llm

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// ModelProfileView is the subset of store.ModelProfile the Switch needs.
// Defined here to avoid llm depending on the store package.
type ModelProfileView struct {
	ID              string
	Provider        string
	BaseURL         string
	Model           string
	APIKey          string
	APIKeyEnv       string
	DisableThinking bool
	SupportsVision  bool
	UpdatedAt       time.Time
}

// ProfileSource resolves model profiles (backed by store.Store in production).
type ProfileSource interface {
	DefaultModelProfile() (ModelProfileView, error)
	ModelProfileByID(id string) (ModelProfileView, error)
}

type ctxKey int

const modelProfileIDKey ctxKey = iota

// WithModelProfileID attaches a per-run model profile choice to the context.
func WithModelProfileID(ctx context.Context, profileID string) context.Context {
	if profileID == "" {
		return ctx
	}
	return context.WithValue(ctx, modelProfileIDKey, profileID)
}

// ModelProfileIDFromContext returns the per-run profile id, or "".
func ModelProfileIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(modelProfileIDKey).(string); ok {
		return v
	}
	return ""
}

type cachedProvider struct {
	prov      Provider
	updatedAt time.Time
}

// Switch is a Provider that resolves the active model per-run from a
// ProfileSource. Provider instances are cached by profile id and rebuilt when
// the profile's UpdatedAt advances (hot reload, no restart).
type Switch struct {
	src   ProfileSource
	mu    sync.Mutex
	cache map[string]*cachedProvider

	// build constructs a Provider from a profile. Overridable in tests.
	build func(ModelProfileView) Provider
}

func NewSwitch(src ProfileSource) *Switch {
	s := &Switch{src: src, cache: map[string]*cachedProvider{}}
	s.build = s.defaultBuild
	return s
}

func (s *Switch) defaultBuild(v ModelProfileView) Provider {
	key := v.APIKey
	if key == "" && v.APIKeyEnv != "" {
		key = os.Getenv(v.APIKeyEnv)
	}
	p := NewOpenAI(v.BaseURL, key, v.Model)
	p.DisableThinking = v.DisableThinking
	p.VisionSupported = v.SupportsVision
	return p
}

func (s *Switch) providerFor(ctx context.Context) (Provider, error) {
	id := ModelProfileIDFromContext(ctx)
	view, err := s.src.ModelProfileByID(id)
	if err != nil || id == "" || view.ID == "" {
		view, err = s.src.DefaultModelProfile()
		if err != nil {
			return nil, fmt.Errorf("no usable model profile: %w", err)
		}
		if view.ID == "" {
			return nil, fmt.Errorf("no model profile configured")
		}
	}
	return s.cached(view), nil
}

func (s *Switch) cached(v ModelProfileView) Provider {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.cache[v.ID]; ok && !v.UpdatedAt.After(c.updatedAt) {
		return c.prov
	}
	prov := s.build(v)
	s.cache[v.ID] = &cachedProvider{prov: prov, updatedAt: v.UpdatedAt}
	return prov
}

func (s *Switch) Chat(ctx context.Context, messages []Message, tools []ToolSpec) (Message, error) {
	prov, err := s.providerFor(ctx)
	if err != nil {
		return Message{}, err
	}
	return prov.Chat(ctx, messages, tools)
}

// SupportsVision reflects the DEFAULT profile (used by unattended entry points
// and attachment gating before a run's profile is known).
func (s *Switch) SupportsVision() bool {
	view, err := s.src.DefaultModelProfile()
	if err != nil || view.ID == "" {
		return false
	}
	return s.cached(view).SupportsVision()
}
```

- [ ] **步骤 4：运行确认通过**

运行：`go test ./internal/llm/ -run Switch -count=1`
预期：PASS。（注：第一个测试 `TestSwitchResolvesExplicitProfile` 中默认 profile 未设 vision，断言 `!sw.SupportsVision()` 为 true，即期望 false——实现里默认 def.SupportsVision 为零值 false，符合。）

- [ ] **步骤 5：gofmt + Commit**

```bash
gofmt -w internal/llm/switch.go internal/llm/switch_test.go
git add internal/llm/switch.go internal/llm/switch_test.go
git commit -m "feat(llm): Switch 按 run 解析 Provider 的原子开关（热切换）"
```

---

## 任务 4：引擎把 run 的 ModelProfileID 注入 ctx

**文件：**
- 修改：`internal/run/engine.go`（`runLoop` 调 Chat 前）

`runLoop(ctx, runID, messages)` 已能拿到 `runID`；从 store 读 run 得到 `ModelProfileID`，用 `llm.WithModelProfileID` 包一层 ctx 再调 `Chat`。

- [ ] **步骤 1：写失败测试** — 追加到 `internal/run/engine_test.go`

用一个记录 ctx profile id 的 stub LLM：

```go
type ctxCaptureLLM struct {
	scriptLLM
	gotProfileID string
}

func (c *ctxCaptureLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	c.gotProfileID = llm.ModelProfileIDFromContext(ctx)
	return llm.Message{Role: llm.RoleAssistant, Content: "done"}, nil
}

func TestExecuteInjectsModelProfileID(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a"})
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi", ModelProfileID: "mp_42"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	lm := &ctxCaptureLLM{}
	eng := &Engine{Store: st, LLM: lm, Tools: tool.NewRegistry(), MaxSteps: 4, Messages: msgStoreFor(lm)}
	if err := eng.ExecuteWithOpts(context.Background(), r.ID, agent.Def{ID: "a"}, "hi", RunOptions{}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if lm.gotProfileID != "mp_42" {
		t.Fatalf("profile id not propagated to LLM ctx: %q", lm.gotProfileID)
	}
}
```

说明：参照该文件既有用法构造 `Engine`（消息存储用测试里已有的 helper；若没有持久化消息需求可传 nil 并确保 buildMessages 容忍 nil——现有代码 `if e.Messages != nil` 已容忍）。若 `scriptLLM` 不满足接口，直接让 `ctxCaptureLLM` 实现 `Chat` + `SupportsVision() bool { return false }`。

- [ ] **步骤 2：运行确认失败**

运行：`go test ./internal/run/ -run InjectsModelProfileID -count=1`
预期：FAIL（`gotProfileID` 为空）。

- [ ] **步骤 3：实现** — `internal/run/engine.go` `runLoop`

在 `for step` 循环内、调用 `e.LLM.Chat` 之前注入：

```go
		specs := e.specsForRun(runID)
		chatCtx := ctx
		if rec, err := e.Store.GetRun(runID); err == nil && rec != nil && rec.ModelProfileID != "" {
			chatCtx = llm.WithModelProfileID(ctx, rec.ModelProfileID)
		}
		msg, err := e.LLM.Chat(chatCtx, messages, specs)
```

（替换原来的 `msg, err := e.LLM.Chat(ctx, messages, specs)`。）

- [ ] **步骤 4：运行确认通过**

运行：`go test ./internal/run/ -count=1`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/run/engine.go internal/run/engine_test.go
git commit -m "feat(run): 引擎把 run 的模型 profile 注入 LLM 调用上下文"
```

---

## 任务 5：bootstrap — ProfileSource 适配器、种子、注入 Switch

**文件：**
- 创建：`internal/llm/profile_source.go`（把 store.ModelProfile 适配为 ModelProfileView 的 ProfileSource）
- 修改：`internal/bootstrap/bootstrap.go`（种子 + 构建 Switch + 注入 engine/server/微信）

- [ ] **步骤 1：ProfileSource 适配器** — `internal/llm/profile_source.go`

```go
package llm

import (
	"fmt"

	"github.com/rebornace/baize/internal/store"
)

// StoreProfileSource adapts store.Store to the llm.ProfileSource interface.
type StoreProfileSource struct {
	Store ModelProfileStore
}

// ModelProfileStore is the subset of store.Store used by the source.
type ModelProfileStore interface {
	ListModelProfiles() ([]store.ModelProfile, error)
	GetModelProfile(id string) (store.ModelProfile, error)
}

func toView(p store.ModelProfile) ModelProfileView {
	return ModelProfileView{
		ID:              p.ID,
		Provider:        p.Provider,
		BaseURL:         p.BaseURL,
		Model:           p.Model,
		APIKey:          p.APIKey,
		APIKeyEnv:       p.APIKeyEnv,
		DisableThinking: p.DisableThinking,
		SupportsVision:  p.SupportsVision,
		UpdatedAt:       p.UpdatedAt,
	}
}

func (s *StoreProfileSource) DefaultModelProfile() (ModelProfileView, error) {
	list, err := s.Store.ListModelProfiles()
	if err != nil {
		return ModelProfileView{}, err
	}
	for _, p := range list {
		if p.IsDefault {
			return toView(p), nil
		}
	}
	if len(list) > 0 {
		return toView(list[0]), nil
	}
	return ModelProfileView{}, fmt.Errorf("no model profile configured")
}

func (s *StoreProfileSource) ModelProfileByID(id string) (ModelProfileView, error) {
	p, err := s.Store.GetModelProfile(id)
	if err != nil {
		return ModelProfileView{}, err
	}
	return toView(p), nil
}
```

- [ ] **步骤 2：种子函数** — `internal/bootstrap/bootstrap.go`

新增（紧邻 `newLLM`）：

```go
// seedModelProfile ensures at least one profile exists, seeded from the YAML
// llm section on first boot. The seed key is read from the environment
// (api_key_env) and is NOT stored; the profile keeps api_key_env for resolution.
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
	name := cfg.LLM.Model
	if strings.TrimSpace(name) == "" {
		name = "默认模型"
	}
	_, err = st.UpsertModelProfile(store.ModelProfile{
		Name:            "默认模型",
		Provider:        "openai_compatible",
		BaseURL:         cfg.LLM.BaseURL,
		Model:           cfg.LLM.Model,
		APIKeyEnv:       env,
		DisableThinking: cfg.LLM.DisableThinking,
		SupportsVision:  cfg.LLM.SupportsVision,
		IsDefault:       true,
	})
	if err != nil {
		return fmt.Errorf("seed model profile: %w", err)
	}
	_ = name
	return nil
}
```

注意：Memory 的 `UpsertModelProfile` 新建分支不会强制 `IsDefault`；种子需要落 `is_default=true`。Memory INSERT 分支当前忽略传入的 `IsDefault`（新 profile 默认 false）。需在任务 1 的 Memory `UpsertModelProfile` 新建分支保留入参 `IsDefault`：新建时 `p.IsDefault` 保持调用方传入值（不要重置）。SQL INSERT 已写入 `p.IsDefault`。确认 Memory 新建分支不覆盖该字段。

- [ ] **步骤 3：构建 Switch 并注入** — `internal/bootstrap/bootstrap.go`

原来 `provider, err := newLLM(cfg)`（约 171 行）之后，改为：

```go
	baseProvider, err := newLLM(cfg)
	if err != nil {
		return nil, err
	}
	if err := seedModelProfile(st, cfg); err != nil {
		return nil, err
	}
	sw := llm.NewSwitch(&llm.StoreProfileSource{Store: st})
	provider := llm.Provider(sw)
	_ = baseProvider // 保留 newLLM 用于启动期校验配置可构造；或直接删除 baseProvider
```

要点：
- 之后所有使用 `provider` 的地方（`run.Engine{LLM: provider}`、`srv.LLM = provider`、`wireWeixinChannel(..., provider, ...)`）自动拿到 Switch，类型仍是 `llm.Provider`，无需改签名。
- 保留启动期对 YAML 配置的一次 `newLLM` 校验（若 provider 为 mock 也无妨——Switch 以 DB profile 为准；demo 模式种子的 BaseURL 可能为空，Switch 仅在真正 Chat 时才报错，与现状 mock 行为不冲突）。若希望 demo 仍走 mock，可在 `cfg.LLM.Provider == "mock"` 时不构建 Switch、直接用 mock provider——加判断：

```go
	if strings.ToLower(cfg.LLM.Provider) == "mock" {
		provider = baseProvider // mock demo path; skip Switch
	} else {
		if err := seedModelProfile(st, cfg); err != nil {
			return nil, err
		}
		provider = llm.NewSwitch(&llm.StoreProfileSource{Store: st})
	}
```

- [ ] **步骤 4：微信 channel SupportsVision**

`wireWeixinChannel` 内 `supportsVision := provider.SupportsVision()`（约 368 行）保持不变——现在 provider 是 Switch，`SupportsVision()` 动态返回默认 profile 能力。确认 channel.Runtime 在每条消息处理时读取该值而非进程启动缓存一次；若 `supportsVision` 只在 wiring 时算一次传入 Runtime，则改为传 provider 或在 AfterCreateRun 内实时调用。核对 `channel.Runtime` 字段：若 `SupportsVision bool` 是静态字段，可接受（默认模型通常稳定）；规格要求实时跟随，若改动小则在 AfterCreateRun 里用 `provider.SupportsVision()` 实时判定图片降级。本任务以「wiring 时取一次」为最小实现，实时跟随列为已知限制（默认模型变更后重启微信渠道或进程生效）。

- [ ] **步骤 5：构建确认**

运行：`go build ./internal/...`
预期：通过。

- [ ] **步骤 6：Commit**

```bash
git add internal/llm/profile_source.go internal/bootstrap/bootstrap.go internal/store/memory.go
git commit -m "feat(bootstrap): 种子默认模型 profile 并注入 llm.Switch"
```

---

## 任务 6：管理 API + ACL + 发消息透传 model_profile_id

**文件：**
- 创建：`internal/api/server_models.go`
- 测试：`internal/api/server_models_test.go`
- 修改：`internal/api/server.go`（路由 + handlePostRun 读字段）
- 修改：`internal/api/run_start.go`（startRunInput 透传）
- 修改：`internal/controlplane/acl.go`（路由角色）

- [ ] **步骤 1：ACL 路由角色** — `internal/controlplane/acl.go`

在 mcp-export 规则之后追加：

```go
	{method: "GET", segments: []string{"v0", "settings", "models"}, role: RoleOperator},
	{method: "POST", segments: []string{"v0", "settings", "models"}, role: RoleAdmin},
	{method: "PATCH", segments: []string{"v0", "settings", "models", "{id}"}, role: RoleAdmin},
	{method: "DELETE", segments: []string{"v0", "settings", "models", "{id}"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "models", "{id}", "default"}, role: RoleAdmin},
```

- [ ] **步骤 2：写失败测试** `internal/api/server_models_test.go`

参照 `server_mcp_export_test.go` 的 `withAdmin`/`withOperator` helper（若该文件已有同名 helper，直接复用，勿重复定义）。核心用例：

```go
func TestModelProfilesCRUDPermissionsAndRedaction(t *testing.T) {
	srv := newTestServer(t) // 复用该包既有的测试 Server 构造 helper

	// operator can read (empty list ok), cannot create
	opReq := withOperator(httptest.NewRequest(http.MethodGet, "/v0/settings/models", nil))
	opRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(opRR, opReq)
	if opRR.Code != http.StatusOK {
		t.Fatalf("operator GET models: code=%d", opRR.Code)
	}

	createBody := `{"name":"主力","provider":"openai_compatible","base_url":"https://x/v1","model":"m1","api_key":"sk-secret-1234"}`
	opPost := withOperator(httptest.NewRequest(http.MethodPost, "/v0/settings/models", strings.NewReader(createBody)))
	opPostRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(opPostRR, opPost)
	if opPostRR.Code != http.StatusForbidden {
		t.Fatalf("operator POST must be forbidden, got %d", opPostRR.Code)
	}

	// admin creates
	adPost := withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/models", strings.NewReader(createBody)))
	adPost.Header.Set("Content-Type", "application/json")
	adRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(adRR, adPost)
	if adRR.Code != http.StatusCreated && adRR.Code != http.StatusOK {
		t.Fatalf("admin create: code=%d body=%s", adRR.Code, adRR.Body.String())
	}
	var created struct{ Profile store.ModelProfile `json:"profile"` }
	json.Unmarshal(adRR.Body.Bytes(), &created)
	if created.Profile.APIKey == "sk-secret-1234" {
		t.Fatal("create response must not echo raw api_key")
	}

	// list redacts key
	listReq := withAdmin(httptest.NewRequest(http.MethodGet, "/v0/settings/models", nil))
	listRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(listRR, listReq)
	if strings.Contains(listRR.Body.String(), "sk-secret-1234") {
		t.Fatal("list must not contain raw api_key")
	}

	// set default, then deleting default is rejected
	defReq := withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/models/"+created.Profile.ID+"/default", nil))
	defRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(defRR, defReq)
	if defRR.Code != http.StatusOK {
		t.Fatalf("set default: code=%d", defRR.Code)
	}
	delReq := withAdmin(httptest.NewRequest(http.MethodDelete, "/v0/settings/models/"+created.Profile.ID, nil))
	delRR := httptest.NewRecorder()
	srv.mux.ServeHTTP(delRR, delReq)
	if delRR.Code != http.StatusBadRequest {
		t.Fatalf("deleting default must be 400, got %d", delRR.Code)
	}
}

func TestModelProfilesRejectMissingCredentials(t *testing.T) {
	srv := newTestServer(t)
	body := `{"name":"nokey","provider":"openai_compatible","base_url":"https://x/v1","model":"m1"}`
	req := withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/models", strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("profile without api_key/api_key_env must be 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}
```

注意：测试 Server 构造 helper 名与 admin/operator 注入 helper 以包内现有为准（`server_mcp_export_test.go` / `server_test.go` 中查找 `func withAdmin`、`func newTestServer` 或等价），不要重复声明。

- [ ] **步骤 3：运行确认失败**

运行：`go test ./internal/api/ -run ModelProfiles -count=1`
预期：FAIL（404 / 路由不存在）。

- [ ] **步骤 4：实现 server_models.go**

```go
package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/store"
)

type modelProfilePayload struct {
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	BaseURL         string `json:"base_url"`
	Model           string `json:"model"`
	APIKey          string `json:"api_key"`
	APIKeyEnv       string `json:"api_key_env"`
	DisableThinking bool   `json:"disable_thinking"`
	SupportsVision  bool   `json:"supports_vision"`
	IsDefault       bool   `json:"is_default"`
}

func redactedProfile(p store.ModelProfile) store.ModelProfile {
	p.APIKey = store.RedactAPIKey(p.APIKey)
	return p
}

func (s *Server) handleListModelProfiles(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListModelProfiles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	out := make([]store.ModelProfile, 0, len(list))
	for _, p := range list {
		out = append(out, redactedProfile(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": out})
}

func (s *Server) handlePostModelProfile(w http.ResponseWriter, r *http.Request) {
	var p modelProfilePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := validateModelPayload(p, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_profile", err.Error())
		return
	}
	prof := store.ModelProfile{
		Name:            strings.TrimSpace(p.Name),
		Provider:        "openai_compatible",
		BaseURL:         strings.TrimSpace(p.BaseURL),
		Model:           strings.TrimSpace(p.Model),
		APIKey:          p.APIKey,
		APIKeyEnv:       strings.TrimSpace(p.APIKeyEnv),
		DisableThinking: p.DisableThinking,
		SupportsVision:  p.SupportsVision,
	}
	saved, err := s.Store.UpsertModelProfile(prof)
	if err != nil {
		writeError(w, http.StatusBadRequest, "upsert_failed", err.Error())
		return
	}
	if p.IsDefault {
		_ = s.Store.SetDefaultModelProfile(saved.ID)
		saved.IsDefault = true
	}
	writeJSON(w, http.StatusCreated, map[string]any{"profile": redactedProfile(saved)})
}

func (s *Server) handlePatchModelProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.Store.GetModelProfile(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "model profile not found")
		return
	}
	var p modelProfilePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := validateModelPayload(p, true); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_profile", err.Error())
		return
	}
	updated := store.ModelProfile{
		ID:              id,
		Name:            strings.TrimSpace(p.Name),
		Provider:        "openai_compatible",
		BaseURL:         strings.TrimSpace(p.BaseURL),
		Model:           strings.TrimSpace(p.Model),
		APIKey:          p.APIKey, // empty/redacted -> store keeps existing
		APIKeyEnv:       strings.TrimSpace(p.APIKeyEnv),
		DisableThinking: p.DisableThinking,
		SupportsVision:  p.SupportsVision,
	}
	saved, err := s.Store.UpsertModelProfile(updated)
	if err != nil {
		writeError(w, http.StatusBadRequest, "upsert_failed", err.Error())
		return
	}
	_ = existing
	writeJSON(w, http.StatusOK, map[string]any{"profile": redactedProfile(saved)})
}

func (s *Server) handleDeleteModelProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Store.DeleteModelProfile(id); err != nil {
		writeError(w, http.StatusBadRequest, "delete_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSetDefaultModelProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Store.SetDefaultModelProfile(id); err != nil {
		writeError(w, http.StatusBadRequest, "set_default_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func validateModelPayload(p modelProfilePayload, isPatch bool) error {
	if strings.TrimSpace(p.Name) == "" {
		return errString("name is required")
	}
	if strings.TrimSpace(p.BaseURL) == "" {
		return errString("base_url is required")
	}
	if strings.TrimSpace(p.Model) == "" {
		return errString("model is required")
	}
	// On create, at least one credential source must be present. On patch,
	// an empty/redacted key means "keep existing", so do not force it.
	if !isPatch && strings.TrimSpace(p.APIKey) == "" && strings.TrimSpace(p.APIKeyEnv) == "" {
		return errString("either api_key or api_key_env is required")
	}
	return nil
}

type errString string

func (e errString) Error() string { return string(e) }
```

- [ ] **步骤 5：注册路由** — `internal/api/server.go`（在 mcp-export 路由附近）

```go
	s.mux.HandleFunc("GET /v0/settings/models", s.handleListModelProfiles)
	s.mux.HandleFunc("POST /v0/settings/models", s.handlePostModelProfile)
	s.mux.HandleFunc("PATCH /v0/settings/models/{id}", s.handlePatchModelProfile)
	s.mux.HandleFunc("DELETE /v0/settings/models/{id}", s.handleDeleteModelProfile)
	s.mux.HandleFunc("POST /v0/settings/models/{id}/default", s.handleSetDefaultModelProfile)
```

- [ ] **步骤 6：发消息透传 model_profile_id**

`internal/api/server.go` `handlePostRun` 的匿名 body 结构加字段：

```go
	ModelProfileID string `json:"model_profile_id"`
```

在组装 `startRunInput` 的调用处（`run_start.go` 的 `startRun` 被调用处，约 1490 行附近）把 `body.ModelProfileID` 透传。

`internal/api/run_start.go`：`startRunInput` 加字段 `ModelProfileID string`，并在 `createIn` 中加：

```go
		ModelProfileID:     in.ModelProfileID,
```

- [ ] **步骤 7：运行确认通过**

运行：`go test ./internal/api/ ./internal/controlplane/ -count=1`
预期：PASS。

- [ ] **步骤 8：gofmt + Commit**

```bash
gofmt -w internal/api/server_models.go internal/api/server_models_test.go
git add internal/api/server_models.go internal/api/server_models_test.go internal/api/server.go internal/api/run_start.go internal/controlplane/acl.go
git commit -m "feat(api): 模型 profile 管理 API、ACL 与发消息透传 model_profile_id"
```

---

## 任务 7：前端 api.ts 封装 + 模型设置页

**文件：**
- 修改：`web/chat/src/api.ts`（类型 + CRUD 函数）
- 修改：`web/chat/src/settingsNav.ts`（admin 导航项）
- 修改：`web/chat/src/main.tsx`（路由）
- 创建：`web/chat/src/pages/ModelSettings.tsx`
- 测试：`web/chat/src/pages/ModelSettings.test.ts`
- 修改：`web/chat/src/settingsNav.test.ts`（导航断言）

参照 `McpExportSettings.tsx` 的页面结构与 `api.ts` 中 mcp-export 封装的写法（`authInit`、`parseJSON`、`ApiError`）。

- [ ] **步骤 1：api.ts 增加类型与函数**

```ts
export interface ModelProfile {
  id: string
  name: string
  provider: string
  base_url: string
  model: string
  api_key?: string
  api_key_env?: string
  disable_thinking: boolean
  supports_vision: boolean
  is_default: boolean
  created_at?: string
  updated_at?: string
}

export async function listModelProfiles(): Promise<ModelProfile[]> {
  const res = await fetch('/v0/settings/models', { headers: authInit() })
  const body = await parseJSON<{ profiles: ModelProfile[] }>(res)
  return body.profiles ?? []
}

export async function createModelProfile(p: Partial<ModelProfile>): Promise<ModelProfile> {
  const res = await fetch('/v0/settings/models', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(p),
  })
  const body = await parseJSON<{ profile: ModelProfile }>(res)
  return body.profile
}

export async function updateModelProfile(id: string, p: Partial<ModelProfile>): Promise<ModelProfile> {
  const res = await fetch(`/v0/settings/models/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(p),
  })
  const body = await parseJSON<{ profile: ModelProfile }>(res)
  return body.profile
}

export async function deleteModelProfile(id: string): Promise<void> {
  const res = await fetch(`/v0/settings/models/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: authInit(),
  })
  if (!res.ok) throw new ApiError(res.status, 'delete_failed', res.statusText)
}

export async function setDefaultModelProfile(id: string): Promise<void> {
  const res = await fetch(`/v0/settings/models/${encodeURIComponent(id)}/default`, {
    method: 'POST',
    headers: authInit(),
  })
  if (!res.ok) throw new ApiError(res.status, 'set_default_failed', res.statusText)
}
```

另在 `CreateRunOptions`（api.ts 中 `createRun` 的 options 类型）加可选字段 `modelProfileId?: string`，并在 `createRun` body 组装处加：

```ts
  if (options?.modelProfileId) body.model_profile_id = options.modelProfileId
```

- [ ] **步骤 2：导航与路由**

`settingsNav.ts` admin 数组在「存储」前加：

```ts
        { to: '/settings/models', label: '模型' },
```

`main.tsx` 按现有设置页路由同款加 `/settings/models` → `ModelSettings`（懒加载或直接 import，与 McpExportSettings 一致）。

`settingsNav.test.ts` 断言 admin 列表含 `{ to: '/settings/models', label: '模型' }`。

- [ ] **步骤 3：写失败测试** `ModelSettings.test.ts`

参照 `McpExportSettings.test.ts` 的渲染/mock 模式，断言：
- 渲染时调用 `listModelProfiles` 并展示 profile 名称；
- 默认 profile 显示「默认」徽章，且其删除按钮禁用；
- 填表提交调用 `createModelProfile`（name/base_url/model/api_key）。

（用 vitest mock `../api`；断言调用参数即可，不追求完整交互。）

- [ ] **步骤 4：运行确认失败**

运行：`npx vitest run src/pages/ModelSettings.test.ts src/settingsNav.test.ts`
预期：FAIL（模块/组件不存在）。

- [ ] **步骤 5：实现 ModelSettings.tsx**

页面包含：
- 列表表格：名称、provider、base_url、model、标志（vision/thinking）、Key 脱敏值、默认徽章；每行「设为默认」（非默认时可用）、「编辑」、「删除」（默认禁用）。
- 新建/编辑表单（受控组件）：name、base_url、model、api_key（type=password，占位「留空则不修改」）、api_key_env、supports_vision 复选、disable_thinking 复选、is_default 复选（仅新建或非默认时）。
- 提交调用 create/update；编辑提交时若 api_key 输入框为空则不传该字段（后端保留原值）。
- 操作后 `listModelProfiles()` 刷新；错误用页面现有 toast/错误文本模式展示。

样式复用现有设置页 className/CSS（与 McpExportSettings、存储页一致），不引入新依赖。

- [ ] **步骤 6：运行确认通过**

运行：`npx vitest run src/pages/ModelSettings.test.ts src/settingsNav.test.ts`
预期：PASS。

- [ ] **步骤 7：Commit**

```bash
git add web/chat/src/api.ts web/chat/src/settingsNav.ts web/chat/src/settingsNav.test.ts web/chat/src/main.tsx web/chat/src/pages/ModelSettings.tsx web/chat/src/pages/ModelSettings.test.ts
git commit -m "feat(ui): 模型设置页（profile 增删改、设默认）"
```

---

## 任务 8：聊天框模型下拉

**文件：**
- 修改：`web/chat/src/pages/ChatPage.tsx`（加载 profiles、下拉、发送带 model_profile_id）
- 测试：可并入现有 ChatPage 测试或新增 `ChatPageModelSelect.test.ts`

- [ ] **步骤 1：行为**

- 组件挂载时（admin/operator 均可）调用 `listModelProfiles()`，存入 state。
- 输入区附近渲染 `<select>`：选项为 profiles，值为 id；默认选中 `is_default` 的 profile；本地 state `selectedModelId` 在本次会话内保持，刷新后回到默认。
- 发送时 `createRun(agentId, text, sentConversationId, { ..., modelProfileId: selectedModelId || undefined })`。
- profiles 加载失败或为空时不显示下拉（回落默认模型），不阻断发送。

- [ ] **步骤 2：测试（vitest）**

mock `listModelProfiles` 返回两个 profile（一个 is_default）；渲染 ChatPage；断言：
- 下拉存在且默认选中 default profile 的 id；
- 切换下拉后触发发送，断言 `createRun` 被调用时 options 含 `modelProfileId` 为所选 id。

若 ChatPage 现有测试因依赖较多难以挂载，可将下拉抽为一个小组件 `<ModelSelect profiles value onChange />` 放 `web/chat/src/components/ModelSelect.tsx` 并单测其回调，ChatPage 只负责取数与传参。

- [ ] **步骤 3：运行确认通过**

运行：`npx vitest run src/pages src/components 2>$null; npx vitest run`
预期：全部 PASS。

- [ ] **步骤 4：构建前端产物**

运行：`cd web/chat && npm run build`
预期：构建成功；`internal/ui/dist` 产物更新（项目将 dist 纳入仓库）。把变更的 `internal/ui/dist/**` 一并提交。

- [ ] **步骤 5：Commit**

```bash
git add web/chat/src/pages/ChatPage.tsx web/chat/src/components/ModelSelect.tsx internal/ui/dist
git commit -m "feat(ui): 聊天框模型下拉，按消息选择模型"
```

---

## 任务 9：文档与全量验证

**文件：**
- 修改：`README.md`、`README.zh-CN.md`（多模型配置小节）
- 修改：`docs/architecture-and-plugin-protocol.md`（如提及 LLM 配置处）
- 修改：`docs/superpowers/notes/2026-08-28-oss-backlog-and-enterprise.md`（X2 标记已交付）——该文件不进 public。

- [ ] **步骤 1：文档**

在 README 的配置/设置小节补充：设置页「模型」可配置多个 OpenAI 兼容模型、设默认、聊天时按消息切换；API Key 存本地库、界面脱敏；无人值守入口用默认模型。中英双份。

- [ ] **步骤 2：全量后端测试**

```powershell
$env:Path="C:\Users\Administrator\.local\go1.25.0\bin;"+$env:Path; $env:GOTOOLCHAIN="local"; $env:GOPROXY="https://goproxy.cn,direct"
go build ./...
go test ./... -count=1
gofmt -l internal cmd
```
预期：构建通过、测试全绿、gofmt 无输出。

- [ ] **步骤 3：全量前端测试**

```bash
cd web/chat && npx vitest run && npm run build
```
预期：全绿、构建成功。

- [ ] **步骤 4：提交文档与产物**

```bash
git add README.md README.zh-CN.md docs/architecture-and-plugin-protocol.md docs/superpowers/notes/2026-08-28-oss-backlog-and-enterprise.md internal/ui/dist
git commit -m "docs: 多模型配置与对话选模型（X2）使用说明"
```

- [ ] **步骤 5：finishing**

按 superpowers:finishing-a-development-branch 合并 `feat/multi-model-profiles-v0` 到 main、删分支/worktree，再推 real 与 public 双仓（public 经 `scripts/export-public.ps1` + `_public_git` 镜像提交；`docs/superpowers` 不进 public）。

---

## 自检对照

- 规格 §1 成功标准 1-8 → 任务 1/2（持久化）、任务 8（下拉按消息）、任务 3/4（默认回退/热切换）、任务 6（脱敏/删默认拒绝）、任务 5（种子）、任务 6 ACL（operator 只读/admin 写）。
- 规格 §2 数据模型 → 任务 1/2；§3 Switch → 任务 3/4/5；§4 API → 任务 6；§5 权限 → 任务 6 ACL；§6 前端 → 任务 7/8；§7 边界（删非默认回退、无 profile 报错、并发 UpdatedAt、脱敏往返）→ 任务 2/3/6；§8 测试 → 各任务 TDD 步骤。
- 类型一致性：`store.ModelProfile` 字段名与 `llm.ModelProfileView`、前端 `ModelProfile`（snake_case）一致；`UpsertModelProfile` 在 Memory 与 SQLStore 签名一致（返回 `(ModelProfile, error)`）；`llm.WithModelProfileID/ModelProfileIDFromContext` 在任务 3 定义、任务 4 使用；`ProfileSource` 两方法在任务 3 定义、任务 5 实现适配器。


