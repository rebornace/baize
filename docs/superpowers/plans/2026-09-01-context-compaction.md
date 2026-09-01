# 对话上下文压缩 / 滚动摘要（X4）实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法跟踪进度。

**目标：** 长对话按「模型上下文长度 × 阈值」触发滚动摘要压缩，prompt 变为 system + 滚动摘要 + 近期原文 + 当前输入；原始消息全保留，摘要持久化、可重算；失败静默回退硬窗口。

**架构：** 会话存储新增 `conversation_summaries` 派生表（滚动摘要 + 游标，三驱动）；新增 `run.Compactor` 在 `buildMessages` 后同步内联增量折叠（用默认模型摘要）；模型 profile 新增 `context_tokens` 字段贯通 store/llm/api/UI；`conversation.compact_*` YAML 配置。

**技术栈：** Go 1.25（`net/http`、`database/sql`、modernc sqlite / pgx）、React + TypeScript + vitest。

**规格：** `docs/superpowers/specs/2026-09-01-context-compaction-design.md`

**环境注意（Windows）：** go 不在默认 PATH；运行 go 命令前在 PowerShell 设
`$env:Path="C:\Users\Administrator\.local\go1.25.0\bin;"+$env:Path; $env:GOTOOLCHAIN="local"; $env:GOPROXY="https://goproxy.cn,direct"`。

---

## 文件结构

- 创建 `internal/conversation/summary_rolling.go` — `RollingSummary` 类型 + Memory 实现（Get/Upsert/Clear）。
- 修改 `internal/conversation/store.go` — 接口加 3 方法；`MemoryStore` 加 `summaries map`；`Clear` 联动清摘要。
- 修改 `internal/conversation/sqlite.go` / `sql_dialect.go` — 建表 `conversation_summaries` + SQL CRUD；`TruncateFrom` 联动清摘要。
- 创建 `internal/run/tokens.go` + `tokens_test.go` — token 估算纯函数。
- 创建 `internal/run/compact.go` + `compact_test.go` — `Compactor`。
- 修改 `internal/run/engine.go` — `Engine.Compactor` 字段 + `ExecuteWithOpts` 调用 + `EventContextCompacted` 常量。
- 修改 `internal/store/store.go` / `model_profiles.go` / `model_profiles_sql.go` / `sqlite.go` / `postgres.go` — `ContextTokens` 字段、列、迁移、归一。
- 修改 `internal/llm/profile_source.go` / `switch.go` — `ModelProfileView.ContextTokens` + toView。
- 修改 `internal/api/server_models.go` — payload `context_tokens`（PATCH `*int`）。
- 修改 `internal/config/config.go` — `conversation.compact_*` 配置与默认归一。
- 修改 `internal/bootstrap/bootstrap.go` — 种子 ContextTokens；构造 Compactor 注入 Engine。
- 前端：`web/chat/src/api.ts`、`web/chat/src/pages/ModelSettings.tsx` + 测试。

---

## 任务 1：会话存储 — RollingSummary 类型、接口与 Memory 实现

**文件：**
- 创建：`internal/conversation/summary_rolling.go`
- 修改：`internal/conversation/store.go`（接口 + MemoryStore + Clear）
- 测试：`internal/conversation/summary_rolling_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `internal/conversation/summary_rolling_test.go`：

```go
package conversation

import "testing"

func TestMemoryRollingSummaryUpsertGetClear(t *testing.T) {
	s := NewMemoryStore()
	if _, ok := s.GetRollingSummary("c1"); ok {
		t.Fatal("expected no summary initially")
	}
	s.UpsertRollingSummary(RollingSummary{
		ConversationID:         "c1",
		Summary:                "用户在做报销项目",
		CoversThroughMessageID: "msg_10",
		CoversThroughOrder:     9,
	})
	got, ok := s.GetRollingSummary("c1")
	if !ok || got.Summary != "用户在做报销项目" || got.CoversThroughOrder != 9 {
		t.Fatalf("unexpected summary: %+v ok=%v", got, ok)
	}
	// 增量更新（同会话覆盖）
	s.UpsertRollingSummary(RollingSummary{
		ConversationID:         "c1",
		Summary:                "用户在做报销项目；已决定用 SQLite",
		CoversThroughMessageID: "msg_20",
		CoversThroughOrder:     19,
	})
	got, _ = s.GetRollingSummary("c1")
	if got.CoversThroughOrder != 19 || got.Summary == "用户在做报销项目" {
		t.Fatalf("update not applied: %+v", got)
	}
	// Clear 清空会话时联动删除摘要
	s.Append("c1", Message{Role: RoleUser, Content: "hi"})
	s.Clear("c1")
	if _, ok := s.GetRollingSummary("c1"); ok {
		t.Fatal("summary should be cleared with conversation")
	}
}

func TestMemoryRollingSummaryForkDoesNotCopy(t *testing.T) {
	s := NewMemoryStore()
	m1, _ := s.Append("c1", Message{Role: RoleUser, Content: "first"})
	s.Append("c1", Message{Role: RoleUser, Content: "second"})
	s.UpsertRollingSummary(RollingSummary{ConversationID: "c1", Summary: "S", CoversThroughMessageID: m1.ID, CoversThroughOrder: 0})
	newID, _, err := s.Fork("c1", m1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetRollingSummary(newID); ok {
		t.Fatal("forked conversation must not inherit rolling summary")
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/conversation/ -run RollingSummary -count=1`
预期：FAIL，编译报错 `GetRollingSummary undefined` 等。

- [ ] **步骤 3：实现类型与 Memory**

创建 `internal/conversation/summary_rolling.go`：

```go
package conversation

import "time"

// RollingSummary is the derived, per-conversation compaction state: a rolling
// summary of older messages plus a cursor pointing at the last summarized
// message. Raw messages are never deleted; this record can be dropped and
// recomputed at any time.
type RollingSummary struct {
	ConversationID         string    `json:"conversation_id"`
	Summary                string    `json:"summary"`
	CoversThroughMessageID string    `json:"covers_through_message_id"`
	CoversThroughOrder     int       `json:"covers_through_order"`
	UpdatedAt              time.Time `json:"updated_at"`
}
```

修改 `internal/conversation/store.go`：

接口 `Store` 内（在 `SetRunID` 后）加：

```go
	// Rolling summary (context compaction). Derived from messages; safe to drop.
	GetRollingSummary(conversationID string) (RollingSummary, bool)
	UpsertRollingSummary(s RollingSummary) error
	ClearRollingSummary(conversationID string)
```

`MemoryStore` 结构体加字段：

```go
type MemoryStore struct {
	mu        sync.RWMutex
	msgs      map[string][]Message
	meta      map[string]Meta
	summaries map[string]RollingSummary
}
```

`NewMemoryStore` 初始化 `summaries: map[string]RollingSummary{}`。

加三个方法：

```go
func (s *MemoryStore) GetRollingSummary(conversationID string) (RollingSummary, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.summaries[conversationID]
	return v, ok
}

func (s *MemoryStore) UpsertRollingSummary(sum RollingSummary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.summaries == nil {
		s.summaries = map[string]RollingSummary{}
	}
	if sum.UpdatedAt.IsZero() {
		sum.UpdatedAt = time.Now().UTC()
	}
	s.summaries[sum.ConversationID] = sum
	return nil
}

func (s *MemoryStore) ClearRollingSummary(conversationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.summaries, conversationID)
}
```

把现有 `Clear` 方法改为联动：

```go
func (s *MemoryStore) Clear(conversationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.msgs, conversationID)
	delete(s.summaries, conversationID)
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/conversation/ -count=1`
预期：PASS（Memory；SQLite 此时因接口未实现会编译失败——见任务 2，本任务先给 SQLite 加临时桩或直接在任务 2 实现。为保持包可编译，本任务在 `sqlite.go` 临时加三个返回零值的桩方法，任务 2 替换为真实实现）。

临时桩（追加到 `internal/conversation/sqlite.go`，任务 2 删除替换）：

```go
func (s *SQLiteStore) GetRollingSummary(conversationID string) (RollingSummary, bool) { return RollingSummary{}, false }
func (s *SQLiteStore) UpsertRollingSummary(sum RollingSummary) error                   { return nil }
func (s *SQLiteStore) ClearRollingSummary(conversationID string)                        {}
```

- [ ] **步骤 5：gofmt 与提交**

运行：`gofmt -l internal/conversation`（应无输出；gofmt 在 `C:\Users\Administrator\.local\go1.25.0\bin\gofmt.exe`）。

```bash
git add internal/conversation/summary_rolling.go internal/conversation/summary_rolling_test.go internal/conversation/store.go internal/conversation/sqlite.go
git commit -m "feat(conversation): RollingSummary 类型、接口与 Memory 实现"
```

---

## 任务 2：会话存储 — SQL 建表、CRUD 与 TruncateFrom 联动

**文件：**
- 修改：`internal/conversation/sqlite.go`（建表 + CRUD 真实实现，替换任务 1 桩）
- 修改：`internal/conversation/sql_dialect.go`（postgres 建表）
- 测试：`internal/conversation/summary_rolling_test.go`（追加 SQLite 用例）

- [ ] **步骤 1：编写失败的测试**

追加到 `internal/conversation/summary_rolling_test.go`：

```go
package conversation

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newSQLiteConv(t *testing.T) *SQLiteStore {
	t.Helper()
	// 私有 :memory: + 单连接，保证每个测试拿到全新隔离库，避免跨测试污染。
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	s, err := OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSQLiteRollingSummaryRoundTrip(t *testing.T) {
	s := newSQLiteConv(t)
	if _, ok := s.GetRollingSummary("c1"); ok {
		t.Fatal("expected no summary")
	}
	if err := s.UpsertRollingSummary(RollingSummary{
		ConversationID:         "c1",
		Summary:                "旧对话摘要",
		CoversThroughMessageID: "msg_5",
		CoversThroughOrder:     4,
	}); err != nil {
		t.Fatal(err)
	}
	got, ok := s.GetRollingSummary("c1")
	if !ok || got.Summary != "旧对话摘要" || got.CoversThroughOrder != 4 || got.CoversThroughMessageID != "msg_5" {
		t.Fatalf("round trip failed: %+v ok=%v", got, ok)
	}
}

func TestSQLiteTruncateClearsSummary(t *testing.T) {
	s := newSQLiteConv(t)
	m1, _ := s.Append("c1", Message{Role: RoleUser, Content: "a"})
	m2, _ := s.Append("c1", Message{Role: RoleUser, Content: "b"})
	s.UpsertRollingSummary(RollingSummary{ConversationID: "c1", Summary: "S", CoversThroughMessageID: m2.ID, CoversThroughOrder: 1})
	if _, err := s.TruncateFrom("c1", m1.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetRollingSummary("c1"); ok {
		t.Fatal("truncate must clear rolling summary")
	}
}
```

注意：`newSQLiteConv` 若与 `sqlite_test.go` 里既有 helper 重名则改名（如 `newRSQLite`）；`modernc.org/sqlite` 的驱动名以既有测试文件 import 为准。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/conversation/ -run "RollingSummary|TruncateClears" -count=1`
预期：FAIL（桩永远返回空/不持久化，round-trip 断言失败）。

- [ ] **步骤 3：建表**

在 `internal/conversation/sqlite.go` 的 `sqliteMessagesSchema` 常量末尾（`conversation_meta` 建表之后）加：

```sql
CREATE TABLE IF NOT EXISTS conversation_summaries (
  conversation_id TEXT PRIMARY KEY,
  summary TEXT NOT NULL,
  covers_through_message_id TEXT NOT NULL,
  covers_through_order INTEGER NOT NULL,
  updated_at TEXT NOT NULL
);
```

在 `internal/conversation/sql_dialect.go` 的 postgres 分支（`pgMeta` 常量旁）加等价建表（`covers_through_order INTEGER`）：

```go
const pgSummaries = `
CREATE TABLE IF NOT EXISTS conversation_summaries (
  conversation_id TEXT PRIMARY KEY,
  summary TEXT NOT NULL,
  covers_through_message_id TEXT NOT NULL,
  covers_through_order INTEGER NOT NULL,
  updated_at TEXT NOT NULL
);`
```

并在 postgres 分支 `db.Exec(pgMeta)` 后再 `db.Exec(pgSummaries)`。

- [ ] **步骤 4：用真实 CRUD 替换桩**

删除任务 1 的三个桩方法，在 `sqlite.go` 实现：

```go
func (s *SQLiteStore) GetRollingSummary(conversationID string) (RollingSummary, bool) {
	var rs RollingSummary
	var updated string
	err := s.queryRow(
		`SELECT conversation_id, summary, covers_through_message_id, covers_through_order, updated_at
		 FROM conversation_summaries WHERE conversation_id = ?`, conversationID,
	).Scan(&rs.ConversationID, &rs.Summary, &rs.CoversThroughMessageID, &rs.CoversThroughOrder, &updated)
	if err != nil {
		return RollingSummary{}, false
	}
	if t, perr := time.Parse(time.RFC3339Nano, updated); perr == nil {
		rs.UpdatedAt = t
	}
	return rs, true
}

func (s *SQLiteStore) UpsertRollingSummary(sum RollingSummary) error {
	if sum.UpdatedAt.IsZero() {
		sum.UpdatedAt = time.Now().UTC()
	}
	_, err := s.exec(
		`INSERT INTO conversation_summaries
		   (conversation_id, summary, covers_through_message_id, covers_through_order, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(conversation_id) DO UPDATE SET
		   summary = excluded.summary,
		   covers_through_message_id = excluded.covers_through_message_id,
		   covers_through_order = excluded.covers_through_order,
		   updated_at = excluded.updated_at`,
		sum.ConversationID, sum.Summary, sum.CoversThroughMessageID, sum.CoversThroughOrder,
		sum.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteStore) ClearRollingSummary(conversationID string) {
	_, _ = s.exec(`DELETE FROM conversation_summaries WHERE conversation_id = ?`, conversationID)
}
```

注意：postgres 不支持 SQLite 的 `ON CONFLICT ... DO UPDATE` 同一语法？——实际上 postgres 也支持 `INSERT ... ON CONFLICT ... DO UPDATE`（语法相同，占位符经 `s.q()` 重绑定）。确认 `s.exec` 已对 postgres 重绑定 `?`→`$n`（sql_dialect.go 已做）。若 postgres 表主键名一致，该语句两驱动通用。

- [ ] **步骤 5：TruncateFrom 与 Clear 联动**

找到 `sqlite.go` 的 `TruncateFrom`（SQL 实现），在成功删除消息后、return 前加：

```go
	s.ClearRollingSummary(conversationID)
```

SQLite 的 `Clear(conversationID)`（若有删 messages 的实现）同样在删除后加 `s.ClearRollingSummary(conversationID)`。

- [ ] **步骤 6：运行测试验证通过**

运行：`go test ./internal/conversation/ -count=1`
预期：PASS（Memory + SQLite 全绿）。

- [ ] **步骤 7：gofmt 与提交**

`gofmt -l internal/conversation` 无输出。

```bash
git add internal/conversation/sqlite.go internal/conversation/sql_dialect.go internal/conversation/summary_rolling_test.go
git commit -m "feat(conversation): conversation_summaries 表与滚动摘要 SQL 持久化"
```

联动的精确位置（`sqlite.go`）：
- `Clear`（约 142-144 行，`DELETE FROM messages`）后加 `s.ClearRollingSummary(conversationID)`。
- `TruncateFrom`（约 146 行起）在 `res, err := s.exec(DELETE ... created_at >= ?)` 成功、`err == nil` 之后加 `s.ClearRollingSummary(conversationID)`（在 return deleted 之前）。
- `Fork` 无需改动（新会话 ID 天然无摘要行）。

---

## 任务 3：Token 估算纯函数

**文件：**
- 创建：`internal/run/tokens.go`
- 测试：`internal/run/tokens_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `internal/run/tokens_test.go`：

```go
package run

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/llm"
)

func TestEstimateTextTokens(t *testing.T) {
	if EstimateTextTokens("") != 0 {
		t.Fatal("empty -> 0")
	}
	ascii := EstimateTextTokens(strings.Repeat("a", 40)) // ~10 tokens
	if ascii < 8 || ascii > 14 {
		t.Fatalf("ascii 40 chars ~10 tokens, got %d", ascii)
	}
	cjk := EstimateTextTokens(strings.Repeat("中", 40)) // ~40 tokens
	if cjk < 36 || cjk > 48 {
		t.Fatalf("cjk 40 chars ~40 tokens, got %d", cjk)
	}
	if cjk <= ascii {
		t.Fatalf("cjk must cost more per char than ascii: cjk=%d ascii=%d", cjk, ascii)
	}
}

func TestEstimateMessagesTokensAddsOverhead(t *testing.T) {
	one := EstimateMessagesTokens([]llm.Message{{Role: llm.RoleUser, Content: strings.Repeat("a", 40)}})
	three := EstimateMessagesTokens([]llm.Message{
		{Role: llm.RoleSystem, Content: "sys"},
		{Role: llm.RoleUser, Content: "hi"},
		{Role: llm.RoleAssistant, Content: "hello"},
	})
	if one <= 0 {
		t.Fatal("one message should be > 0")
	}
	// 三条消息的结构开销（每条 +4）应使 three 明显大于单条短消息之外的基线
	if three < 12 {
		t.Fatalf("three messages should include per-message overhead, got %d", three)
	}
}

func TestEstimateToolsTokens(t *testing.T) {
	if EstimateToolsTokens(nil) != 0 {
		t.Fatal("nil tools -> 0")
	}
	tools := []llm.ToolSpec{{Name: "get_weather", Description: strings.Repeat("d", 40)}}
	if EstimateToolsTokens(tools) < 8 {
		t.Fatalf("tool with 40-char desc should count tokens, got %d", EstimateToolsTokens(tools))
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/run/ -run "Estimate" -count=1`
预期：FAIL，`undefined: EstimateTextTokens`。

- [ ] **步骤 3：实现**

创建 `internal/run/tokens.go`：

```go
package run

import (
	"unicode"

	"github.com/rebornace/baize/internal/llm"
)

// perMessageTokens is the structural overhead added per chat message (role
// markers, separators). Conservative on purpose.
const perMessageTokens = 4

// EstimateTextTokens approximates token count without a tokenizer:
// CJK characters count ~1 token each; other (ASCII-ish) content counts ~1
// token per 4 bytes. This deliberately over-estimates slightly so compaction
// triggers before a real context overflow.
func EstimateTextTokens(s string) int {
	if s == "" {
		return 0
	}
	tokens := 0
	for _, r := range s {
		if isCJK(r) {
			tokens++
		}
	}
	// Non-CJK bytes: total runes minus CJK runes is a rough proxy; use byte
	// length for the ASCII remainder to stay simple and conservative.
	cjkBytes := 0
	for _, r := range s {
		if isCJK(r) {
			cjkBytes += utf8RuneLen(r)
		}
	}
	other := len(s) - cjkBytes
	tokens += other / 4
	if other%4 != 0 {
		tokens++
	}
	return tokens
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		(r >= 0x3040 && r <= 0x30FF) ||   // hiragana/katakana
		(r >= 0xAC00 && r <= 0xD7AF)     // hangul
}

func utf8RuneLen(r rune) int {
	switch {
	case r < 0x80:
		return 1
	case r < 0x800:
		return 2
	case r < 0x10000:
		return 3
	default:
		return 4
	}
}

// EstimateMessagesTokens sums content tokens across messages plus per-message
// overhead. Multimodal Parts (images) are ignored token-wise (images are not
// compacted in v0); text parts are counted.
func EstimateMessagesTokens(msgs []llm.Message) int {
	total := 0
	for _, m := range msgs {
		total += perMessageTokens
		if len(m.Parts) > 0 {
			for _, p := range m.Parts {
				if p.Type == "text" {
					total += EstimateTextTokens(p.Text)
				}
			}
			continue
		}
		total += EstimateTextTokens(m.Content)
	}
	return total
}

// EstimateToolsTokens approximates the cost of tool schemas by name+description.
func EstimateToolsTokens(tools []llm.ToolSpec) int {
	total := 0
	for _, t := range tools {
		total += EstimateTextTokens(t.Name) + EstimateTextTokens(t.Description) + perMessageTokens
	}
	return total
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/run/ -run "Estimate" -count=1`
预期：PASS。

- [ ] **步骤 5：gofmt 与提交**

`gofmt -l internal/run` 无输出。

```bash
git add internal/run/tokens.go internal/run/tokens_test.go
git commit -m "feat(run): 轻量 token 估算（CJK/ASCII 保守口径）"
```

---

## 任务 4：模型 profile 新增 `context_tokens` 字段（store + llm 贯通）

**文件：**
- 修改：`internal/store/store.go`（`ModelProfile` 结构）
- 修改：`internal/store/sqlite.go`（建表 DDL + 幂等加列迁移）
- 修改：`internal/store/postgres.go`（DDL + 加列迁移）
- 修改：`internal/store/model_profiles_sql.go`（列清单、scan、INSERT、UPDATE）
- 修改：`internal/llm/switch.go`（`ModelProfileView`）、`internal/llm/profile_source.go`（`toView`）
- 修改：`internal/bootstrap/bootstrap.go`（种子 profile 设 `ContextTokens`）
- 测试：`internal/store/model_profiles_test.go`（追加 round-trip 断言）

- [ ] **步骤 1：编写失败的测试**

追加到 `internal/store/model_profiles_test.go` 的 SQLite round-trip 测试（或新增 `TestSQLiteModelProfileContextTokens`）：

```go
func TestSQLiteModelProfileContextTokens(t *testing.T) {
	st := newSQLiteProfileStore(t) // 复用文件里既有 helper；若无则照该文件建内存 sqlite
	got, err := st.UpsertModelProfile(ModelProfile{
		Name: "ctx", Provider: "openai_compatible", BaseURL: "http://x", Model: "m",
		APIKey: "sk-abcdefgh1234", ContextTokens: 200000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ContextTokens != 200000 {
		t.Fatalf("ContextTokens not persisted: %d", got.ContextTokens)
	}
	again, err := st.GetModelProfile(got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.ContextTokens != 200000 {
		t.Fatalf("ContextTokens round trip failed: %d", again.ContextTokens)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/store/ -run ContextTokens -count=1`
预期：FAIL（`ContextTokens undefined` 或恒为 0）。

- [ ] **步骤 3：struct 与视图**

`internal/store/store.go` 的 `ModelProfile` 在 `SupportsVision` 后加：

```go
	ContextTokens   int       `json:"context_tokens"`
```

`internal/llm/switch.go` 的 `ModelProfileView` 在 `SupportsVision` 后加：

```go
	ContextTokens  int
```

`internal/llm/profile_source.go` 的 `toView` 加一行：

```go
		ContextTokens:  p.ContextTokens,
```

- [ ] **步骤 4：DDL 与迁移**

`internal/store/sqlite.go` 的 `model_profiles` 建表（约 130 行 `supports_vision INTEGER,` 附近）加列：

```sql
  context_tokens INTEGER NOT NULL DEFAULT 128000,
```

并对**已存在的旧库**做幂等加列（照 `runs` 表 `migrateRunsColumns` 的既有迁移方式；若 model_profiles 没有等价迁移函数，在打开 store 的迁移处补一次 `ALTER TABLE model_profiles ADD COLUMN context_tokens INTEGER NOT NULL DEFAULT 128000`，并忽略「duplicate column」错误）。

`internal/store/postgres.go` 的 `model_profiles` 建表（约 112 行 `supports_vision BOOLEAN,` 附近）加：

```sql
  context_tokens INTEGER NOT NULL DEFAULT 128000,
```

postgres 旧库同样幂等 `ALTER TABLE model_profiles ADD COLUMN IF NOT EXISTS context_tokens INTEGER NOT NULL DEFAULT 128000`（照 X2 给 runs 加列的 `ADD COLUMN IF NOT EXISTS` 方式）。

- [ ] **步骤 5：SQL CRUD**

在 `internal/store/model_profiles_sql.go` 中，**完全照 `supports_vision` 的穿线方式**加 `context_tokens`：

- `upsertModelProfileColumns` 常量的列清单末尾（`is_default` 前或后，保持与 DDL 顺序一致）加 `context_tokens`。
- `scanModelProfile`：加 `&p.ContextTokens`（INTEGER 直接 scan 进 int，无需 NullBool；若该列用 `COALESCE`/可空则用 sql.NullInt64 兜底，但 DDL 是 NOT NULL DEFAULT，直接 int 即可）。
- INSERT 的列与 VALUES 占位符加 `context_tokens` 与对应 `?`，参数加 `p.ContextTokens`。
- UPDATE 的 `SET ...` 子句加 `context_tokens=?` 与参数。
- Memory 实现（`model_profiles.go`）存的是结构体本身，无需改；但确保 Upsert 新建分支不会把 ContextTokens 清零（结构体整体存入，天然保留）。

- [ ] **步骤 6：种子默认值**

`internal/bootstrap/bootstrap.go` 的 `seedModelProfile` 里构造的 `store.ModelProfile` 加：

```go
		ContextTokens: 128000,
```

- [ ] **步骤 7：运行测试验证通过**

运行：`go build ./... && go test ./internal/store/ ./internal/llm/ ./internal/bootstrap/ -count=1`
预期：PASS。`gofmt -l internal/store internal/llm internal/bootstrap` 无输出。

- [ ] **步骤 8：提交**

```bash
git add internal/store/store.go internal/store/sqlite.go internal/store/postgres.go internal/store/model_profiles_sql.go internal/store/model_profiles_test.go internal/llm/switch.go internal/llm/profile_source.go internal/bootstrap/bootstrap.go
git commit -m "feat(store): model profile 增加 context_tokens 上下文字段"
```

---

## 任务 5：Compactor —— 阈值判断、增量折叠、prompt 重组

**文件：**
- 创建：`internal/run/compact.go`
- 测试：`internal/run/compact_test.go`

本任务只实现纯逻辑与对 `conversation.Store`/`llm.ProfileSource` 的交互；引擎接入在任务 6。

- [ ] **步骤 1：编写失败的测试**

创建 `internal/run/compact_test.go`。测试用 fake LLM 与 `conversation.NewMemoryStore()`：

```go
package run

import (
	"context"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
)

// fakeLLM returns a canned summary and records the messages it received.
type fakeCompactLLM struct {
	reply string
	got   []llm.Message
}

func (f *fakeCompactLLM) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	f.got = msgs
	return llm.Message{Role: llm.RoleAssistant, Content: f.reply}, nil
}
func (f *fakeCompactLLM) SupportsVision() bool { return false }

// fakeProfiles is a minimal llm.ProfileSource.
type fakeProfiles struct{ def llm.ModelProfileView }

func (f fakeProfiles) DefaultModelProfile() (llm.ModelProfileView, error) { return f.def, nil }
func (f fakeProfiles) ModelProfileByID(id string) (llm.ModelProfileView, error) {
	return f.def, nil
}

func seedConv(t *testing.T, ms conversation.Store, convID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		role := conversation.RoleUser
		if i%2 == 1 {
			role = conversation.RoleAssistant
		}
		ms.Append(convID, conversation.Message{Role: role, Content: strings.Repeat("内容", 100)})
	}
}

func TestMaybeCompactNoProfileSkips(t *testing.T) {
	ms := conversation.NewMemoryStore()
	seedConv(t, ms, "c", 50)
	// ProfileSource returns an empty view (ID==""): mock/demo path with no
	// configured profile -> compaction disabled entirely.
	c := &Compactor{Messages: ms, LLM: &fakeCompactLLM{reply: "摘要"},
		Profiles:      fakeProfiles{def: llm.ModelProfileView{}},
		Threshold:     0.8, ReserveTokens: 8000, KeepRecent: 8}
	changed, err := c.MaybeCompact(context.Background(), "c", nil, "p")
	if err != nil || changed {
		t.Fatalf("no profile must skip: changed=%v err=%v", changed, err)
	}
	if _, ok := ms.GetRollingSummary("c"); ok {
		t.Fatal("no summary when profile missing")
	}
}

func TestMaybeCompactUnderThresholdSkips(t *testing.T) {
	ms := conversation.NewMemoryStore()
	seedConv(t, ms, "c", 4)
	c := &Compactor{Messages: ms, LLM: &fakeCompactLLM{reply: "摘要"},
		Profiles:      fakeProfiles{def: llm.ModelProfileView{ID: "p", ContextTokens: 100000}},
		Threshold:     0.8, ReserveTokens: 8000, KeepRecent: 8}
	// budget = 100000*0.8-8000 = 72000；4 条消息（约 800 token）远低于此
	changed, err := c.MaybeCompact(context.Background(), "c", nil, "p")
	if err != nil || changed {
		t.Fatalf("under threshold must skip: changed=%v err=%v", changed, err)
	}
	if _, ok := ms.GetRollingSummary("c"); ok {
		t.Fatal("should not compact under threshold")
	}
}
```

注意：`ContextTokens <= 0` 的 profile 会被归一为默认 128000（仍正常压缩，只是阈值宽松）；真正「禁用」压缩的情形是 ProfileSource 拿不到任何 profile（`view.ID==""`，mock/demo 路径）或全局 `compact_enabled: false`（bootstrap 不构造 Compactor）。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/run/ -run MaybeCompact -count=1`
预期：FAIL，`undefined: Compactor`。

- [ ] **步骤 3：把失败测试改成最终签名并补触发/折叠用例**

把上面两个测试里的 `MaybeCompact` 调用统一为最终签名：

```go
changed, err := c.MaybeCompact(context.Background(), "c", nil, "p")
```

并追加两个核心用例：

```go
func TestMaybeCompactTriggersAndFolds(t *testing.T) {
	ms := conversation.NewMemoryStore()
	seedConv(t, ms, "c", 40) // 40 条，每条 200 个 CJK（内容x100）≈ 大量 token
	llmF := &fakeCompactLLM{reply: "这是滚动摘要"}
	c := &Compactor{
		Messages: ms, LLM: llmF,
		Profiles:     fakeProfiles{def: llm.ModelProfileView{ID: "p", ContextTokens: 4000}},
		Threshold:    0.7, ReserveTokens: 400, KeepRecent: 8,
		SummaryTimeout: 0, // 0 用默认
	}
	changed, err := c.MaybeCompact(context.Background(), "c", nil, "p")
	if err != nil || !changed {
		t.Fatalf("expected compaction: changed=%v err=%v", changed, err)
	}
	sum, ok := ms.GetRollingSummary("c")
	if !ok || sum.Summary != "这是滚动摘要" {
		t.Fatalf("summary not persisted: %+v ok=%v", sum, ok)
	}
	// 游标应覆盖到 recent 窗口之前那条；recent=8，40 条 => 折叠 0..31，cursor order=31
	if sum.CoversThroughOrder != 31 {
		t.Fatalf("cursor order = %d, want 31", sum.CoversThroughOrder)
	}
	// 摘要 LLM 必须收到了转录文本
	if len(llmF.got) < 2 {
		t.Fatalf("expected system + transcript messages, got %d", len(llmF.got))
	}
}

func TestMaybeCompactIncrementalExtendsCursor(t *testing.T) {
	ms := conversation.NewMemoryStore()
	seedConv(t, ms, "c", 40)
	c := &Compactor{
		Messages: ms, LLM: &fakeCompactLLM{reply: "摘要一"},
		Profiles:      fakeProfiles{def: llm.ModelProfileView{ID: "p", ContextTokens: 4000}},
		Threshold:     0.7, ReserveTokens: 400, KeepRecent: 8,
	}
	if _, err := c.MaybeCompact(context.Background(), "c", nil, "p"); err != nil {
		t.Fatal(err)
	}
	first, _ := ms.GetRollingSummary("c")
	// 再来 20 条，recent 窗口下移，应增量折叠
	seedConv(t, ms, "c", 20)
	c.LLM = &fakeCompactLLM{reply: "摘要二"}
	changed, err := c.MaybeCompact(context.Background(), "c", nil, "p")
	if err != nil || !changed {
		t.Fatalf("expected incremental compaction: changed=%v err=%v", changed, err)
	}
	second, _ := ms.GetRollingSummary("c")
	if second.CoversThroughOrder <= first.CoversThroughOrder {
		t.Fatalf("cursor must advance: %d -> %d", first.CoversThroughOrder, second.CoversThroughOrder)
	}
}
```

- [ ] **步骤 4：运行测试验证失败**

运行：`go test ./internal/run/ -run MaybeCompact -count=1`
预期：FAIL，`undefined: Compactor`。

- [ ] **步骤 5：实现 Compactor**

创建 `internal/run/compact.go`：

```go
package run

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
)

const (
	defaultCompactThreshold    = 0.8
	defaultCompactReserve      = 8000
	defaultCompactKeepRecent   = 8
	defaultCompactSummaryWait  = 60 * time.Second
	defaultContextTokens       = 128000
	compactSummarySystemPrompt = `你是对话摘要助手。请把给定的多轮对话压缩成简洁的中文摘要，保留：用户的目标与偏好、已做出的关键决定、待办与未决事项、关键事实（文件名、ID、数据、结论）。不要编造对话中没有的信息。
若提供了「已有摘要」，请在其基础上增量整合新对话，输出一份完整、自洽的最新摘要（不要罗列「新增/旧摘要」的边界）。`
)

// Compactor produces a rolling summary of older conversation messages once the
// estimated prompt size approaches the active model's context limit. It never
// deletes raw messages; the summary is a derived record keyed by conversation.
type Compactor struct {
	Messages conversation.Store
	// LLM generates summaries. Pass the llm.Switch: summaries are invoked on a
	// bare context (no per-run profile id) so the Switch resolves the DEFAULT
	// profile, independent of the model chosen for this run.
	LLM      llm.Provider
	Profiles llm.ProfileSource

	Threshold      float64       // fraction of context that may be used before folding
	ReserveTokens  int           // headroom reserved for answer + tools + current turn
	KeepRecent     int           // number of newest messages always kept verbatim
	SummaryTimeout time.Duration // cap on the summarization call
}

func (c *Compactor) normalize() {
	if c.Threshold <= 0 {
		c.Threshold = defaultCompactThreshold
	}
	if c.ReserveTokens <= 0 {
		c.ReserveTokens = defaultCompactReserve
	}
	if c.KeepRecent <= 0 {
		c.KeepRecent = defaultCompactKeepRecent
	}
	if c.SummaryTimeout <= 0 {
		c.SummaryTimeout = defaultCompactSummaryWait
	}
}

// MaybeCompact folds older messages into a rolling summary when the projected
// prompt (tools + existing summary + full history) exceeds the budget derived
// from the run's model context limit. It returns changed=true when a new
// summary was persisted. Failures are returned to the caller; the engine logs
// and continues with the hard window (compaction never blocks a reply).
func (c *Compactor) MaybeCompact(ctx context.Context, convID string, tools []llm.ToolSpec, profileID string) (bool, error) {
	if c == nil || c.Messages == nil || c.LLM == nil || c.Profiles == nil || convID == "" {
		return false, nil
	}
	c.normalize()

	view, err := c.resolveView(profileID)
	if err != nil || view.ID == "" {
		return false, nil // no usable profile -> compaction disabled (mock/demo path)
	}
	if view.ContextTokens <= 0 {
		view.ContextTokens = defaultContextTokens // normalize unconfigured profile
	}
	budget := int(float64(view.ContextTokens)*c.Threshold) - c.ReserveTokens
	if budget < 1000 {
		budget = 1000
	}

	full := c.Messages.List(convID)
	if len(full) == 0 {
		return false, nil
	}
	existing, hasSummary := c.Messages.GetRollingSummary(convID)

	projected := EstimateToolsTokens(tools) + EstimateTextTokens(existing.Summary) + EstimateMessagesTokens(toLLMMessages(full))
	if projected <= budget {
		return false, nil
	}

	keepStart := len(full) - c.KeepRecent
	if keepStart <= 0 {
		return false, nil // everything is "recent"; nothing foldable
	}
	covered := 0
	if hasSummary {
		covered = existing.CoversThroughOrder + 1
	}
	if keepStart <= covered {
		return false, nil // no new messages beyond the cursor to fold
	}
	newFold := full[covered:keepStart]

	newSummary, err := c.summarize(ctx, existing.Summary, newFold)
	if err != nil {
		return false, err
	}

	rec := conversation.RollingSummary{
		ConversationID:         convID,
		Summary:                newSummary,
		CoversThroughMessageID: full[keepStart-1].ID,
		CoversThroughOrder:     keepStart - 1,
	}
	if err := c.Messages.UpsertRollingSummary(rec); err != nil {
		return false, err
	}
	return true, nil
}

func (c *Compactor) resolveView(profileID string) (llm.ModelProfileView, error) {
	if profileID != "" {
		if v, err := c.Profiles.ModelProfileByID(profileID); err == nil && v.ID != "" {
			return v, nil
		}
	}
	v, err := c.Profiles.DefaultModelProfile()
	if err != nil {
		return llm.ModelProfileView{}, err
	}
	return v, nil
}

func (c *Compactor) summarize(ctx context.Context, prior string, fold []conversation.Message) (string, error) {
	msgs := []llm.Message{{Role: llm.RoleSystem, Content: compactSummarySystemPrompt}}
	var b strings.Builder
	if prior != "" {
		b.WriteString("已有摘要：\n")
		b.WriteString(prior)
		b.WriteString("\n\n请在已有摘要基础上，整合以下新对话，输出完整最新摘要。\n\n新对话：\n")
	} else {
		b.WriteString("请把以下多轮对话压缩成滚动摘要：\n\n")
	}
	b.WriteString(renderTranscript(fold))
	msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: b.String()})

	// Bare context: no per-run profile id => Switch uses the DEFAULT model.
	sumCtx, cancel := context.WithTimeout(context.Background(), c.SummaryTimeout)
	defer cancel()
	out, err := c.LLM.Chat(sumCtx, msgs, nil)
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}
	return strings.TrimSpace(out.Content), nil
}

func renderTranscript(msgs []conversation.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case conversation.RoleUser:
			b.WriteString("[用户] ")
		case conversation.RoleAssistant:
			b.WriteString("[助手] ")
		default:
			continue // skip tool / system_note noise
		}
		b.WriteString(m.Content)
		b.WriteString("\n\n")
	}
	return b.String()
}

// toLLMMessages converts persisted messages for token estimation (content only).
func toLLMMessages(msgs []conversation.Message) []llm.Message {
	out := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, llm.Message{Role: llm.Role(m.Role), Content: m.Content})
	}
	return out
}
```

注意：`llm.Role(m.Role)` —— `conversation.Role` 与 `llm.Role` 底层都是 string 常量且取值一致（`"user"`/`"assistant"`/`"system"`），可直接转换。

- [ ] **步骤 6：运行测试验证通过**

运行：`go test ./internal/run/ -run "MaybeCompact|Estimate" -count=1`
预期：PASS。若 `TestMaybeCompactTriggersAndFolds` 的 cursor 断言不符（31），以 `len(full)-KeepRecent-1 = 40-8-1 = 31` 为准核对。

- [ ] **步骤 7：gofmt 与提交**

`gofmt -l internal/run` 无输出。

```bash
git add internal/run/compact.go internal/run/compact_test.go
git commit -m "feat(run): Compactor 滚动摘要压缩（阈值触发/增量折叠）"
```

---

## 任务 6：引擎接入 Compactor（压缩 + prompt 注入摘要 + 事件）

**文件：**
- 修改：`internal/run/engine.go`（常量、`Engine.Compactor` 字段、`ExecuteWithOpts`、`buildMessages`）
- 测试：`internal/run/compact_engine_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `internal/run/compact_engine_test.go`，验证 buildMessages 在有摘要时注入摘要块、无摘要时行为不变：

```go
package run

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
)

func TestBuildMessagesInjectsRollingSummary(t *testing.T) {
	ms := conversation.NewMemoryStore()
	e := &Engine{Messages: ms, MaxMessages: 40}
	conv := "c1"
	for i := 0; i < 6; i++ {
		role := conversation.RoleUser
		if i%2 == 1 {
			role = conversation.RoleAssistant
		}
		ms.Append(conv, conversation.Message{Role: role, Content: "旧消息内容"})
	}
	ms.UpsertRollingSummary(conversation.RollingSummary{
		ConversationID: conv, Summary: "此前对话：用户在做报销系统",
		CoversThroughMessageID: "x", CoversThroughOrder: 1,
	})
	msgs := e.buildMessages("系统提示", conv, "现在的问题", nil)
	if msgs[0].Role != "system" || msgs[0].Content != "系统提示" {
		t.Fatalf("first message must be the real system prompt: %+v", msgs[0])
	}
	found := false
	for _, m := range msgs {
		if m.Role == "user" && strings.Contains(m.Content, "此前对话：用户在做报销系统") {
			found = true
		}
	}
	if !found {
		t.Fatalf("rolling summary block not injected; got %d messages", len(msgs))
	}
	// 最后一条必须是当前输入
	if msgs[len(msgs)-1].Content != "现在的问题" {
		t.Fatalf("last message must be current input, got %q", msgs[len(msgs)-1].Content)
	}
}

func TestBuildMessagesNoSummaryUnchanged(t *testing.T) {
	ms := conversation.NewMemoryStore()
	e := &Engine{Messages: ms, MaxMessages: 40}
	ms.Append("c", conversation.Message{Role: conversation.RoleUser, Content: "你好"})
	msgs := e.buildMessages("sys", "c", "在吗", nil)
	for _, m := range msgs {
		if strings.Contains(m.Content, "对话历史摘要") {
			t.Fatal("must not inject summary block when none exists")
		}
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/run/ -run BuildMessages -count=1`
预期：FAIL（摘要未注入）。

- [ ] **步骤 3：加事件常量与 Engine 字段**

`internal/run/engine.go` 事件常量区（`EventWorkflowPrefix` 附近）加：

```go
	// EventContextCompacted records a rolling-summary compaction before a run.
	EventContextCompacted = "context.compacted"
```

`Engine` 结构体（`MaxMessages int` 字段后）加：

```go
	// Compactor optionally folds older history into a rolling summary before a
	// run when the prompt approaches the model's context limit. nil = disabled
	// (hard sliding window only).
	Compactor *Compactor
```

- [ ] **步骤 4：ExecuteWithOpts 里在 buildMessages 前触发压缩**

在 `ExecuteWithOpts` 中，`messages := e.buildMessages(...)` **之前**插入：

```go
	if e.Compactor != nil && runRec.ConversationID != "" {
		specs := e.specsForRun(runID)
		changed, cerr := e.Compactor.MaybeCompact(ctx, runRec.ConversationID, specs, runRec.ModelProfileID)
		if cerr != nil {
			// Compaction is best-effort: log an event and continue with the
			// hard window. Never block the reply.
			_ = e.Store.AppendEvent(runID, store.Event{
				Type: EventLLMError,
				Data: map[string]any{"error": "context compaction skipped: " + cerr.Error()},
			})
		} else if changed {
			_ = e.Store.AppendEvent(runID, store.Event{
				Type: EventContextCompacted,
				Data: map[string]any{"note": "older history folded into rolling summary"},
			})
		}
	}
```

注意：`runRec.ModelProfileID` 已在该函数作用域内（`runRec` 来自 `e.Store.GetRun(runID)`）。`ctx` 用于取消传播；Compactor 内部摘要调用用独立 `context.Background()` + 超时（见 compact.go），不受 run 取消影响而中断已开始的摘要。

- [ ] **步骤 5：buildMessages 注入摘要块**

修改 `buildMessages`，在 system 消息之后、窗口历史循环之前插入摘要块。把函数开头改为：

```go
	messages := []llm.Message{{Role: llm.RoleSystem, Content: system}}
	if e.Messages != nil && conversationID != "" {
		if sum, ok := e.Messages.GetRollingSummary(conversationID); ok && strings.TrimSpace(sum.Summary) != "" {
			messages = append(messages, llm.Message{
				Role: llm.RoleUser,
				Content: "以下是此前对话的滚动摘要，供你理解上下文（更早的完整对话已被压缩）：\n\n" +
					sum.Summary,
			})
		}
		for _, m := range e.Messages.ListWindow(conversationID, e.MaxMessages) {
			// ...保持原有 switch 不变...
		}
	}
```

确保 `engine.go` 已 import `strings`（若未 import 则加）。摘要块用 `RoleUser`：它是一条上下文指令；紧接着的真实历史/当前输入照常追加。当前输入去重逻辑（`last.Content == input`）不受影响，因为摘要内容不等于 input。

- [ ] **步骤 6：运行测试验证通过**

运行：`go test ./internal/run/ -count=1`
预期：PASS（含任务 3/5 的全部用例）。

- [ ] **步骤 7：gofmt 与提交**

`gofmt -l internal/run` 无输出。

```bash
git add internal/run/engine.go internal/run/compact_engine_test.go
git commit -m "feat(run): 引擎接入滚动摘要压缩并在 prompt 注入摘要"
```

---

## 任务 7：配置项与 bootstrap 注入 Compactor

**文件：**
- 修改：`internal/config/config.go`（`Conversation` 结构 + 默认归一）
- 修改：`internal/config/conversation_test.go`（默认值测试）
- 修改：`internal/bootstrap/bootstrap.go`（构造 Compactor 并注入 Engine）

- [ ] **步骤 1：编写失败的测试**

追加到 `internal/config/conversation_test.go`：

```go
func TestLoadConversationCompactDefaults(t *testing.T) {
	path := writeConfig(t, "store:\n  driver: memory\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.CompactEnabled() {
		t.Fatal("CompactEnabled should default to true")
	}
	if cfg.Conversation.CompactThreshold != 0.8 {
		t.Fatalf("CompactThreshold=%v want 0.8", cfg.Conversation.CompactThreshold)
	}
	if cfg.Conversation.CompactReserveOutput != 8000 {
		t.Fatalf("CompactReserveOutput=%d want 8000", cfg.Conversation.CompactReserveOutput)
	}
	if cfg.Conversation.CompactRecentMessages != 8 {
		t.Fatalf("CompactRecentMessages=%d want 8", cfg.Conversation.CompactRecentMessages)
	}
}

func TestLoadConversationCompactExplicit(t *testing.T) {
	path := writeConfig(t, "store:\n  driver: memory\nconversation:\n  compact_enabled: false\n  compact_threshold: 0.5\n  compact_reserve_output: 2000\n  compact_recent_messages: 6\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CompactEnabled() {
		t.Fatal("compact_enabled:false must disable compaction")
	}
	if cfg.Conversation.CompactThreshold != 0.5 ||
		cfg.Conversation.CompactReserveOutput != 2000 ||
		cfg.Conversation.CompactRecentMessages != 6 {
		t.Fatalf("explicit compact config not parsed: %+v", cfg.Conversation)
	}
}
```

注意：`writeConfig`/`Load` 以该测试文件既有写法为准。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/config/ -run Compact -count=1`
预期：FAIL（字段 undefined 或恒为零值）。

- [ ] **步骤 3：加配置字段**

`internal/config/config.go` 的 `Conversation` 结构（`MaxMessages` 后）加：

```go
	Conversation struct {
		MaxMessages       int     `yaml:"max_messages"`
		PersistIdentities *bool   `yaml:"persist_identities"`
		// Context compaction (rolling summary). CompactEnabled defaults to
		// true (use *bool to distinguish unset from explicit false). Threshold
		// is the fraction of the model context that may be used before older
		// history is folded into a rolling summary (out-of-range => 0.8);
		// ReserveOutput reserves headroom for the answer; RecentMessages is the
		// number of newest messages always kept verbatim.
		CompactEnabled        *bool   `yaml:"compact_enabled"`
		CompactThreshold      float64 `yaml:"compact_threshold"`
		CompactReserveOutput  int     `yaml:"compact_reserve_output"`
		CompactRecentMessages int     `yaml:"compact_recent_messages"`
	} `yaml:"conversation"`
```

（即把现有 `Conversation struct{...}` 整体替换为上面这版。）

默认归一：在 `config.go` 约 260 行 `if cfg.Conversation.MaxMessages <= 0 { ... = 40 }` 之后加：

```go
	if cfg.Conversation.CompactEnabled == nil {
		v := true
		cfg.Conversation.CompactEnabled = &v
	}
	if cfg.Conversation.CompactThreshold <= 0 || cfg.Conversation.CompactThreshold >= 1 {
		cfg.Conversation.CompactThreshold = 0.8
	}
	if cfg.Conversation.CompactReserveOutput <= 0 {
		cfg.Conversation.CompactReserveOutput = 8000
	}
	if cfg.Conversation.CompactRecentMessages <= 0 {
		cfg.Conversation.CompactRecentMessages = 8
	}
```

并加便捷方法（照 `MCPExportEnabled` 的既有写法）：

```go
// CompactEnabled reports whether context compaction is on (default true).
func (c Config) CompactEnabled() bool {
	if c.Conversation.CompactEnabled == nil {
		return true
	}
	return *c.Conversation.CompactEnabled
}
```

- [ ] **步骤 4：运行配置测试验证通过**

运行：`go test ./internal/config/ -count=1`
预期：PASS。

- [ ] **步骤 5：bootstrap 构造并注入 Compactor**

`internal/bootstrap/bootstrap.go`：`provider` 在真实路径已被赋值为 `llm.NewSwitch(...)`（约 224 行）。在 `engine := &run.Engine{...}`（约 288 行）**之前**构造 Compactor：

```go
	var compactor *run.Compactor
	if cfg.CompactEnabled() && messages != nil && provider != nil {
		compactor = &run.Compactor{
			Messages:       messages,
			LLM:            provider,
			Profiles:       &llm.StoreProfileSource{Store: st},
			Threshold:      cfg.Conversation.CompactThreshold,
			ReserveTokens:  cfg.Conversation.CompactReserveOutput,
			KeepRecent:     cfg.Conversation.CompactRecentMessages,
		}
	}
```

在 `engine := &run.Engine{...}` 字面量里加字段：

```go
		Compactor: compactor,
```

说明：mock/demo 路径下 `Profiles`（StoreProfileSource）查不到 profile 时 `MaybeCompact` 直接返回 `(false, nil)`，压缩自动禁用，不影响演示。真实路径 provider 是 Switch，摘要调用在 bare context 上解析到默认 profile（与设计一致）。

- [ ] **步骤 6：全量构建验证**

运行：`go build ./... && go test ./internal/config/ ./internal/bootstrap/ -count=1`
预期：PASS。`gofmt -l internal/config internal/bootstrap` 无输出。

- [ ] **步骤 7：提交**

```bash
git add internal/config/config.go internal/config/conversation_test.go internal/bootstrap/bootstrap.go
git commit -m "feat(config): conversation 压缩配置并在 bootstrap 注入 Compactor"
```

---

## 任务 8：模型管理 API 支持 `context_tokens`

**文件：**
- 修改：`internal/api/server_models.go`（payload + POST + PATCH）
- 测试：`internal/api/server_models_test.go`（追加断言；若文件名不同则找 `model_profile` 相关 api 测试）

- [ ] **步骤 1：编写失败的测试**

在模型 profile 的 API 测试文件中追加（照该文件既有的建 server / 发请求 helper 写法）：

```go
func TestModelProfileContextTokensRoundTrip(t *testing.T) {
	// 用既有 helper 建带内存 store 的 server（参考 TestModelProfilesPatchFieldLevelMerge）
	srv := newModelProfileTestServer(t)
	// POST 带 context_tokens
	body := `{"name":"ctx-model","base_url":"http://x","model":"m","api_key":"sk-abcdefgh1234","context_tokens":200000}`
	res := postJSON(t, srv, "/v0/admin/model-profiles", body) // 以既有 helper 为准
	if res.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", res.Code, res.Body.String())
	}
	var created struct {
		Profile struct {
			ID            string `json:"id"`
			ContextTokens int    `json:"context_tokens"`
		} `json:"profile"`
	}
	decodeJSON(t, res, &created)
	if created.Profile.ContextTokens != 200000 {
		t.Fatalf("context_tokens not echoed on create: %d", created.Profile.ContextTokens)
	}

	// PATCH 改成 32000，其它字段不应丢
	patch := patchJSON(t, srv, "/v0/admin/model-profiles/"+created.Profile.ID, `{"context_tokens":32000}`)
	if patch.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", patch.Code, patch.Body.String())
	}
	var updated struct {
		Profile struct {
			ContextTokens  int    `json:"context_tokens"`
			SupportsVision bool   `json:"supports_vision"`
			Model          string `json:"model"`
		} `json:"profile"`
	}
	decodeJSON(t, patch, &updated)
	if updated.Profile.ContextTokens != 32000 {
		t.Fatalf("patch context_tokens failed: %d", updated.Profile.ContextTokens)
	}
	if updated.Profile.Model != "m" {
		t.Fatalf("PATCH must preserve unrelated fields, model=%q", updated.Profile.Model)
	}
}
```

注意：helper 名（`newModelProfileTestServer`/`postJSON`/`patchJSON`/`decodeJSON`）以测试文件里既有的为准；不要新造重复 helper。核心是断言 create 回显、PATCH 合并且不丢其它字段。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/api/ -run ContextTokens -count=1`
预期：FAIL（`context_tokens` 恒为 0）。

- [ ] **步骤 3：payload 加字段**

`modelProfilePayload`（约 16-26 行）在 `SupportsVision` 后加：

```go
	ContextTokens   *int    `json:"context_tokens"`
```

- [ ] **步骤 4：POST 创建分支**

在 `handlePostModelProfile` 构造 `prof`（约 86-95 行）处加：

```go
	prof := store.ModelProfile{
		// ...原有字段...
		SupportsVision:  p.SupportsVision != nil && *p.SupportsVision,
	}
	if p.ContextTokens != nil && *p.ContextTokens > 0 {
		prof.ContextTokens = *p.ContextTokens
	}
```

（`ContextTokens` 为 nil 或 <=0 时落库默认 128000，由 DB DEFAULT 保证。）

- [ ] **步骤 5：PATCH 合并分支**

在 `handlePatchModelProfile` 的字段合并区（约 156-159 行 `if p.SupportsVision != nil {...}` 附近）加：

```go
	if p.ContextTokens != nil && *p.ContextTokens > 0 {
		updated.ContextTokens = *p.ContextTokens
	}
```

`updated` 来自 `existing`（字段级合并），不传 `context_tokens` 时保持原值。

- [ ] **步骤 6：运行测试验证通过**

运行：`go build ./... && go test ./internal/api/ -run ModelProfile -count=1`
预期：PASS。`gofmt -l internal/api` 无输出。

- [ ] **步骤 7：提交**

```bash
git add internal/api/server_models.go internal/api/server_models_test.go
git commit -m "feat(api): 模型 profile API 支持 context_tokens 字段"
```

---

## 任务 9：前端模型设置页支持上下文长度

**文件：**
- 修改：`web/chat/src/api.ts`（`ModelProfile` 类型）
- 修改：`web/chat/src/pages/ModelSettings.tsx`（表单状态、payload、输入框）
- 测试：`web/chat/src/pages/ModelSettings.test.tsx`

- [ ] **步骤 1：编写失败的测试**

在 `ModelSettings.test.tsx` 追加（照既有 `buildCreatePayload`/`buildPatchPayload` 单测写法）：

```tsx
it('buildCreatePayload includes context_tokens', () => {
  const res = buildCreatePayload({
    ...EMPTY_PROFILE_FORM,
    name: 'm', baseUrl: 'http://x', model: 'm', apiKey: 'sk-abcdefgh1234',
    contextTokens: 200000,
  })
  expect(res.ok).toBe(true)
  if (res.ok) expect(res.payload.context_tokens).toBe(200000)
})

it('buildPatchPayload sends context_tokens only when changed', () => {
  const original = profile({ id: 'mp_1', name: 'm', context_tokens: 128000 })
  const unchanged = buildPatchPayload(
    { ...profileToForm(original) }, original,
  )
  expect(unchanged.context_tokens).toBeUndefined()
  const changed = buildPatchPayload(
    { ...profileToForm(original), contextTokens: 32000 }, original,
  )
  expect(changed.context_tokens).toBe(32000)
})
```

注意：测试里的 `profile(...)` factory（该文件约 22 行）需补 `context_tokens: 128000` 默认字段（否则 TS 报缺字段）。`ChatPageModelSelect.test.tsx` 与其他用到 `ModelProfile` 的 factory（约 8 行、`api.ts` 相关）同样补 `context_tokens: 128000`。

- [ ] **步骤 2：运行测试验证失败**

运行：`cd web/chat && npx vitest run src/pages/ModelSettings.test.tsx`（或仓库既有 `npm test` 脚本）
预期：FAIL（`context_tokens` / `contextTokens` 不存在）。

- [ ] **步骤 3：api.ts 类型**

`web/chat/src/api.ts` 的 `ModelProfile`（约 538-548 行）在 `supports_vision` 后加：

```ts
  context_tokens: number
```

`ModelProfileInput`（`Partial<Omit<ModelProfile, ...>>`）会自动包含 `context_tokens`，无需额外改；确认 omit 列表没有把它排除。

- [ ] **步骤 4：表单状态与 payload**

`ModelSettings.tsx`：

`ProfileFormState` 加字段：

```ts
  contextTokens: number
```

`EMPTY_PROFILE_FORM` 加：

```ts
  contextTokens: 128000,
```

`profileToForm` 加：

```ts
    contextTokens: p.context_tokens > 0 ? p.context_tokens : 128000,
```

`ModelProfilePayload` 加：

```ts
  context_tokens?: number
```

`buildCreatePayload` 的 payload 对象加：

```ts
      context_tokens: form.contextTokens > 0 ? form.contextTokens : 128000,
```

`buildPatchPayload` 在 supports_vision/disable_thinking 差异判断附近加：

```ts
  const contextTokens = Math.floor(Number(form.contextTokens))
  if (contextTokens > 0 && contextTokens !== original.context_tokens) {
    payload.context_tokens = contextTokens
  }
```

- [ ] **步骤 5：表单输入框**

在 `ModelProfileForm` 的 JSX 里（`supports_vision`/`disable_thinking` 复选框附近）加一个数字输入：

```tsx
        <label className="field">
          <span>上下文长度（tokens，留空/0 用默认 128000）</span>
          <input
            type="number"
            min={1024}
            step={1000}
            value={form.contextTokens}
            onChange={(e) => setForm({ ...form, contextTokens: Number(e.target.value) })}
          />
        </label>
```

（`setForm`/`form` 以该组件既有的状态变量名为准；className 沿用同表单其它字段。）

- [ ] **步骤 6：运行前端测试与构建**

运行：`cd web/chat && npx vitest run && npm run build`
预期：测试 PASS，构建成功。

- [ ] **步骤 7：提交**

后端 `internal/ui/dist` 由嵌入前端构建产物更新（照 X2 的做法，前端构建产物在 `internal/ui/dist`）。构建后一并提交：

```bash
git add web/chat/src/api.ts web/chat/src/pages/ModelSettings.tsx web/chat/src/pages/ModelSettings.test.tsx web/chat/src/pages/ChatPageModelSelect.test.tsx internal/ui/dist
git commit -m "feat(web): 模型设置支持配置上下文长度 context_tokens"
```

---

## 任务 10：文档、全量验证与收尾

**文件：**
- 修改：`docs/superpowers/plans/2026-09-01-context-compaction.md`（勾选完成项）
- 修改：`docs/superpowers/notes/2026-08-28-oss-backlog-and-enterprise.md`（X4 标记已交付）
- 视项目惯例更新 README/配置示例（`configs/` 下示例 yaml 注释 `conversation.compact_*`）

- [ ] **步骤 1：后端全量验证**

PowerShell（先设 go 环境，见文档头部）：

```powershell
$env:Path="C:\Users\Administrator\.local\go1.25.0\bin;"+$env:Path; $env:GOTOOLCHAIN="local"; $env:GOPROXY="https://goproxy.cn,direct"
go build ./...
go test ./... -count=1
gofmt -l .
```

预期：构建通过；全部测试 PASS；`gofmt -l .` 无输出（有输出则 `gofmt -w <file>` 后重测）。

- [ ] **步骤 2：前端全量验证**

```powershell
cd web/chat; npx vitest run; npm run build
```

预期：PASS + 构建成功；`internal/ui/dist` 产物更新并已在任务 9 提交（若有残留变更在此一并提交）。

- [ ] **步骤 3：手工冒烟（可选但建议）**

以 mock provider 启动，连续发送 >KeepRecent 条消息确认不报错；再配一个真实/小 `context_tokens`（如 4000）的 profile，发足够长对话，确认 run 事件流出现 `context.compacted`，且后续回复仍正常、长对话记忆通过摘要保留。

- [ ] **步骤 4：更新 backlog 与计划勾选**

把 backlog 文档里 X4（对话历史摘要/滚动摘要）条目标记为「已交付」，附简短说明（滚动摘要 + token 触发 + 持久化 + 可配置窗口）。勾选本计划所有步骤复选框。

- [ ] **步骤 5：提交文档**

commit message 用 UTF-8（Windows PowerShell 用 `[System.IO.File]::WriteAllText` 或 `git commit -F` 文件方式，避免中文乱码，照 X2 收尾经验）：

```bash
git add docs/superpowers/plans/2026-09-01-context-compaction.md docs/superpowers/notes/2026-08-28-oss-backlog-and-enterprise.md configs/
git commit -m "docs: X4 上下文压缩收尾，标记 backlog 已交付"
```

- [ ] **步骤 6：合并与双仓推送（按 finishing-a-development-branch / 用户惯例）**

合并 `feat/context-compaction-v0` 到 `main`，推 `real`，再用 `scripts/export-public.ps1` 同步 `public`（照 X1/X2 收尾；public 推送需 `all` 权限绕过沙箱，网络超时则重试）。

---
