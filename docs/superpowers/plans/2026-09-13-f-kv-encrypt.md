# F-KV：凭据 KV 加密实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 对 K1 范围秘密字段落库加密（AES-256-GCM，`bz1:` 信封）；主密钥仅 `BAIZE_SETTINGS_KEY`（M1）；无 key 拒写、密文无 key 拒启；有 key 启动时自动迁明文（P1）；进程内与 API 脱敏语义不变。

**架构：** 新增包 `internal/settingscrypto`（Seal / Open / 派生密钥 / ErrNoKey）。各持久化写路径在落库前 Seal，读路径 Open（兼容明文）。启动时 `MigratePlainSecrets` 扫描并就地加密。不整表加密 settings；不做 KMS / 密钥文件。

**技术栈：** Go 标准库 `crypto/aes`、`crypto/cipher`、`crypto/sha256`、`crypto/rand`、`encoding/base64`；现有 `store` / `runtimecfg` / `api` / `bootstrap`。提交中文 Conventional Commits。

规格：`docs/superpowers/specs/2026-09-13-f-production-hardening-design.md` §2（刀 **F-KV**；决策 K1/M1/P1）

**锁定约定（实现勿偏离）：**

| 码 | 含义 |
|----|------|
| K1 | 加密：`runtime_settings` 的 `operator_token` / `admin_token` / `operators[].token`；`model_profiles.api_key`；`inbox_channels[].secret`。`api_key_env` 不加密。`events_webhook` 当前无独立 `secret` 字段 → **本刀不改**（若日后加字段，再挂 Seal） |
| M1 | 仅 env `BAIZE_SETTINGS_KEY`；空 = 无 key |
| P1 | 有 key 启动时自动把明文 K1 就地加密写回 |
| 信封 | `bz1:` + RawURLEncoding(base64) of `nonce(12) ‖ ciphertext ‖ tag`；密钥 = `SHA-256(keyBytes)` |
| key 编码 | 若值匹配 `^[A-Za-z0-9+/]+=*$` 且 decode 后 ≥16 字节则按 base64；否则按 UTF-8 原始字节。**空串 = 无 key**。建议运维用 ≥32 字节熵（文档写明；代码对非空 key 均接受，单测可用短 key） |

**测试环境：** 凡写入 K1 秘密的测试须 `t.Setenv("BAIZE_SETTINGS_KEY", "test-settings-key-32bytes-ok!!")`（或包内 `settingscrypto.MustKeyForTest` 仅测试可见）。无 key 时断言拒写。

---

## 文件结构

创建：

- `internal/settingscrypto/crypto.go` — KeyFromEnv、Seal、Open、IsSealed、ErrNoKey、ErrCiphertext
- `internal/settingscrypto/crypto_test.go` — 向量 / 往返 / 无 key / 明文兼容
- `internal/settingscrypto/migrate.go` — `MigrateStore(st store.Store) error`（有 key 时扫 K1）
- `internal/settingscrypto/migrate_test.go`

修改：

- `internal/store/model_profiles.go` / `model_profiles_sql.go` — Upsert 前 Seal `APIKey`；Scan/Get 后 Open（失败向上返回）
- `internal/store/model_profiles_test.go` — 设 key；断言库内为 `bz1:`、Get 得明文
- `internal/runtimecfg/persist.go` — 持久化 creds 字段 Seal；加载时 Open
- `internal/runtimecfg/runtimecfg_test.go` / 相关 persist 测 — 设 key
- `internal/api/server_inbox.go` — `persistInboxChannels` Seal secrets；`loadInboxChannels` Open
- `internal/api/server.go`（或 events webhook 写入点）— **跳过**（无 secret 字段）
- `internal/bootstrap/bootstrap.go` — store 就绪后、对外服务前：`MigrateStore`；若 Open 遇密文且无 key → 启动失败
- `internal/api/server_*.go` 凭据 PATCH — 映射 `settingscrypto.ErrNoKey` → `400` + 明确 `code`（如 `settings_key_required`）
- `.env.example` — 增加 `BAIZE_SETTINGS_KEY=`
- `README.md` / `README.zh-CN.md` — 主密钥、fail-closed、迁移、与 reset-credentials 正交；删/改「凭据明文落库」过时句
- `docs/superpowers/notes/2026-09-13-spec-ledger.md` / 确认清单 / 母规格 §5.1 — 挂本计划

不改：F-HOT、UI 交互形状、GET 脱敏算法、整表 settings 加密、渠道适配器 `secret.key` 文件（非本刀 K1 表）。

---

## 任务 1：`settingscrypto` 核心（TDD）

**文件：**
- 创建：`internal/settingscrypto/crypto.go`
- 创建：`internal/settingscrypto/crypto_test.go`

- [ ] **步骤 1：写失败测试**

```go
package settingscrypto_test

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/settingscrypto"
)

func TestSealOpenRoundTrip(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", "test-settings-key-32bytes-ok!!")
	key, err := settingscrypto.KeyFromEnv()
	if err != nil || key == nil {
		t.Fatalf("KeyFromEnv: %v", err)
	}
	sealed, err := settingscrypto.Seal(key, "sk-secret-1234")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, "bz1:") || sealed == "sk-secret-1234" {
		t.Fatalf("sealed=%q", sealed)
	}
	plain, err := settingscrypto.Open(key, sealed)
	if err != nil || plain != "sk-secret-1234" {
		t.Fatalf("open=%q err=%v", plain, err)
	}
}

func TestOpenPlaintextPassthrough(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", "test-settings-key-32bytes-ok!!")
	key, _ := settingscrypto.KeyFromEnv()
	got, err := settingscrypto.Open(key, "already-plain")
	if err != nil || got != "already-plain" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestSealWithoutKey(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", "")
	key, err := settingscrypto.KeyFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if key != nil {
		t.Fatal("empty env must yield nil key")
	}
	_, err = settingscrypto.Seal(nil, "x")
	if !errors.Is(err, settingscrypto.ErrNoKey) {
		t.Fatalf("want ErrNoKey, got %v", err)
	}
}

func TestOpenCiphertextWithoutKey(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", "test-settings-key-32bytes-ok!!")
	key, _ := settingscrypto.KeyFromEnv()
	sealed, _ := settingscrypto.Seal(key, "secret")
	t.Setenv("BAIZE_SETTINGS_KEY", "")
	_, err := settingscrypto.Open(nil, sealed)
	if !errors.Is(err, settingscrypto.ErrCiphertext) {
		t.Fatalf("want ErrCiphertext, got %v", err)
	}
}
```

（补 `errors` import。）

- [ ] **步骤 2：运行确认失败**

```bash
go test ./internal/settingscrypto/ -count=1
```

预期：FAIL（包不存在或符号未定义）。

- [ ] **步骤 3：最少实现**

`crypto.go` 要点：

```go
package settingscrypto

const Prefix = "bz1:"

var (
	ErrNoKey       = errors.New("BAIZE_SETTINGS_KEY is not set")
	ErrCiphertext  = errors.New("encrypted secret requires BAIZE_SETTINGS_KEY")
	ErrCorrupt     = errors.New("corrupt sealed secret")
)

// Key is a 32-byte AES key. nil means "no key configured".
type Key []byte

func KeyFromEnv() (Key, error) { /* read BAIZE_SETTINGS_KEY; empty -> nil,nil; else derive */ }

func IsSealed(s string) bool { return strings.HasPrefix(s, Prefix) }

func Seal(key Key, plaintext string) (string, error) {
	if len(key) == 0 {
		return "", ErrNoKey
	}
	if plaintext == "" || IsSealed(plaintext) {
		return plaintext, nil
	}
	// AES-GCM seal → Prefix + rawURL base64
}

func Open(key Key, value string) (string, error) {
	if !IsSealed(value) {
		return value, nil
	}
	if len(key) == 0 {
		return "", ErrCiphertext
	}
	// decrypt
}
```

派生：`sha256.Sum256(material)`；nonce 12 字节 `crypto/rand`。

- [ ] **步骤 4：测试通过**

```bash
go test ./internal/settingscrypto/ -count=1
```

预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/settingscrypto/
git commit -m "feat(settingscrypto): AES-GCM bz1 信封与主密钥派生"
```

---

## 任务 2：Model profile `api_key` 落库加解密

**文件：**
- 修改：`internal/store/model_profiles.go`
- 修改：`internal/store/model_profiles_sql.go`
- 修改：`internal/store/model_profiles_test.go`

- [ ] **步骤 1：扩展测试（先红）**

在现有 upsert/get 测里：

```go
t.Setenv("BAIZE_SETTINGS_KEY", "test-settings-key-32bytes-ok!!")
// upsert 后：若可直接读底层（Memory 可检查 s 内部，或 SQL 再查 raw）
// 断言 GetModelProfile 仍返回明文 APIKey
// 另测：无 key 时 Upsert 带非空 APIKey → 返回 ErrNoKey（或包装）
```

对 Memory：可在 upsert 后用未导出字段难测密封值 → 增加包内测试或通过「二次 Open 同一 Get」+「无 key Seal 失败」覆盖。SQL：可选 `SELECT api_key FROM model_profiles` 断言 `bz1:` 前缀（仅 sqlite 测）。

- [ ] **步骤 2：实现**

在 `UpsertModelProfile`（Memory + SQL）写入前：

```go
key, _ := settingscrypto.KeyFromEnv()
if p.APIKey != "" && !store.IsRedactedAPIKey(p.APIKey) {
    sealed, err := settingscrypto.Seal(key, p.APIKey)
    if err != nil {
        return ModelProfile{}, err
    }
    p.APIKey = sealed
}
```

注意：redacted / 空保留原逻辑**先于** Seal（现有 `IsRedactedAPIKey` 分支保留明文路径上的「不改 key」）；仅对新明文 key Seal。

读路径（scan / memory get）：`Open(key, p.APIKey)`；`ErrCiphertext` 向上返回。

- [ ] **步骤 3：跑测**

```bash
go test ./internal/store/ -count=1 -run ModelProfile
```

预期：PASS。修复全文件因拒写而红的用例（统一 `t.Setenv`）。

- [ ] **步骤 4：Commit**

```bash
git add internal/store/model_profiles.go internal/store/model_profiles_sql.go internal/store/model_profiles_test.go
git commit -m "feat(store): model profile api_key 落库加密"
```

---

## 任务 3：`runtime_settings` 凭据字段加解密

**文件：**
- 修改：`internal/runtimecfg/persist.go`
- 修改：`internal/runtimecfg/runtimecfg_test.go`（及相关 persist 测）

- [ ] **步骤 1：失败/扩展测试**

- 有 key：`UpdateCreds` 后从 store 读 raw JSON，断言 `admin_token` / `operator_token` 为 `bz1:` 前缀；`Credentials()` / 快照仍为明文。
- 无 key：写凭据 → `ErrNoKey`（API 层后续映射）。
- `Open` 兼容：手动 Upsert 明文 creds JSON，Load 仍成功。

- [ ] **步骤 2：实现**

在写入 `persisted.Creds` 前 Seal 三个 token 字段；`Load` / 反序列化后 Open。空 token 跳过。

辅助：

```go
func sealCreds(key settingscrypto.Key, c *credsOverride) error
func openCreds(key settingscrypto.Key, c *credsOverride) error
```

- [ ] **步骤 3：测试**

```bash
go test ./internal/runtimecfg/ -count=1
```

- [ ] **步骤 4：Commit**

```bash
git add internal/runtimecfg/
git commit -m "feat(runtimecfg): 控制面凭据 KV 加密落库"
```

---

## 任务 4：Inbox channel `secret` 加解密 + API ErrNoKey

**文件：**
- 修改：`internal/api/server_inbox.go`（`persistInboxChannels` / `loadInboxChannels`）
- 修改：`internal/api/server_inbox_test.go`
- 修改：凭据 PATCH handler（搜 `UpdateCreds` / `handlePatchCredentials`）— 映射 `ErrNoKey`
- 修改：模型 profile API 写路径错误映射（若尚未把 store 错误透出）

- [ ] **步骤 1：Inbox 测**

有 key：PUT/rotate secret 后 settings 原始 JSON 中 secret 为 `bz1:`；运行时校验 HMAC 仍用明文（registry 内明文）。

无 key：创建/轮换 secret → `400` + `settings_key_required`（或计划统一的 code）。

- [ ] **步骤 2：实现**

`persistInboxChannels`：对每个 `Secret` Seal。  
`loadInboxChannels`：Open；失败 → 返回 error（勿静默空 secret）。

凭据 / profile 写：

```go
if errors.Is(err, settingscrypto.ErrNoKey) {
    writeError(w, http.StatusBadRequest, "settings_key_required", "set BAIZE_SETTINGS_KEY to store secrets")
    return
}
```

- [ ] **步骤 3：测试**

```bash
go test ./internal/api/ -count=1 -run Inbox
go test ./internal/api/ -count=1 -run Credential
go test ./internal/api/ -count=1 -run ModelProfile
```

（按实际测试名调整；最终 `go test ./internal/api/ -count=1` 若过慢可先子集再全量。）

- [ ] **步骤 4：Commit**

```bash
git add internal/api/
git commit -m "feat(api): inbox secret 加密与无主密钥拒写"
```

---

## 任务 5：启动迁移（P1）+ 密文无 key 拒启

**文件：**
- 创建：`internal/settingscrypto/migrate.go`
- 创建：`internal/settingscrypto/migrate_test.go`
- 修改：`internal/bootstrap/bootstrap.go`
- 修改：`internal/bootstrap/*_test.go`（若有启动装配测）

- [ ] **步骤 1：迁移测试**

```go
func TestMigrateStoreEncryptsPlainAPIKey(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", "test-settings-key-32bytes-ok!!")
	st := store.NewMemory()
	// 先无 key 写入不可能（拒写）——改为：直接塞明文到 Memory 内部或临时关闭检查的测试钩子
	// 推荐：migrate 测试用 SQL/Memory 的测试辅助 UpsertRaw，或先 Seal 关闭时用 store 测试钩子
}
```

**裁定（写进实现）：** Memory/SQL 增加仅测试用的方式过困难时——`MigrateStore` 测用「手动构造已 Seal 与明文混合的 settings JSON + profile」，对 Memory 在 `settingscrypto_test` 通过 `store.Store` 公共 API：若无 key 不能写入明文，则：

1. 用 `t.Setenv` 空 key **不可**写；  
2. 迁移测改为：有 key 时写入（已是密文）再 Migrate 幂等；另用 **未导出测试** 在 `store` 包内插入明文 `APIKey` 到 memory map后调 `MigrateStore`。

在 `store` 包加：

```go
// PutModelProfilePlainForTest 仅测试：跳过 Seal，写入明文 api_key。
func (s *Memory) PutModelProfilePlainForTest(p ModelProfile) { ... }
```

仅 `*_test.go` 同包或 `Export` 到 `testing` build tag——**优先同包 `model_profiles_migrate_test.go` 直接改 map**（Memory 字段若未导出则加 `PutPlainAPIKeyForTest` 在 `export_test.go`）。

- [ ] **步骤 2：`MigrateStore`**

有 key：

1. List model profiles → 明文 api_key → Seal → Upsert  
2. GetSetting runtime_settings → open JSON → seal creds → Upsert  
3. GetSetting inbox_channels → seal secrets → Upsert  
4. 已 `bz1:` 跳过  

无 key：

- 若任一 K1 值为 `IsSealed` → `return ErrCiphertext`（bootstrap 包装为启动失败）  
- 否则 no-op

- [ ] **步骤 3：bootstrap 挂载**

在 `st` 打开成功后、HTTP Listen 前：

```go
if err := settingscrypto.MigrateStore(st); err != nil {
    _ = closer.Close()
    return nil, nil, fmt.Errorf("settings secrets: %w", err)
}
```

- [ ] **步骤 4：测试 + Commit**

```bash
go test ./internal/settingscrypto/ ./internal/bootstrap/ -count=1
git add internal/settingscrypto/migrate.go internal/settingscrypto/migrate_test.go internal/store/export_test.go internal/bootstrap/bootstrap.go
git commit -m "feat(bootstrap): 启动迁移明文秘密与密文无密钥拒启"
```

---

## 任务 6：文档与账本收口

**文件：**
- 修改：`.env.example`
- 修改：`README.md`、`README.zh-CN.md`（控制面凭据节；删除「凭据 KV 明文落库」非目标句，改为已支持加密）
- 修改：`docs/superpowers/notes/2026-09-13-spec-ledger.md`
- 修改：`docs/superpowers/notes/2026-09-13-v1-product-confirmation-checklist.md`
- 修改：`docs/superpowers/specs/2026-09-13-f-production-hardening-design.md` §5.1（F-KV 计划已就绪）
- 修改：`docs/superpowers/specs/2026-09-05-runtime-settings-hot-reload-design.md` 若仍写「凭据明文」→ 交叉引用 F-KV

- [ ] **步骤 1：`.env.example`**

```bash
BAIZE_API_KEY=
BAIZE_CONNECTOR_TOKEN=
# 设置项秘密落库主密钥（AES）。生产必填；未设置则拒写 api_key/口令/inbox secret，且无法启动已加密库。
BAIZE_SETTINGS_KEY=
```

- [ ] **步骤 2：README 短节（中英）**

说明：主密钥、启动自动迁移、无 key 拒写、密文无 key 拒启、`reset-credentials` 仍可用、与 YAML break-glass 正交。

- [ ] **步骤 3：账本**

- F 行：F-C1 已交付；**F-KV 计划** `plans/2026-09-13-f-kv-encrypt.md`；仍欠 F-HOT  
- 母规格 §5.1：F-KV 计划标「已就绪」

- [ ] **步骤 4：全量相关测试 + Commit**

```bash
go test ./internal/settingscrypto/ ./internal/store/ ./internal/runtimecfg/ ./internal/api/ ./internal/bootstrap/ -count=1
git add .env.example README.md README.zh-CN.md docs/superpowers/
git commit -m "docs: BAIZE_SETTINGS_KEY 与 F-KV 计划收口"
```

---

## 任务 7：DoD 自检

**文件：** 无强制代码

- [ ] **步骤 1：对照母规格 §2 / §4.2 F-KV 行**

| 要求 | 落点 |
|------|------|
| K1 字段加密 | 任务 2–4 |
| M1 仅 env | 任务 1 |
| P1 启动迁移 | 任务 5 |
| 无 key 拒写 / 密文无 key 拒启 | 任务 1/4/5 |
| GET 仍脱敏 | 未改 Redact 路径 |
| 不做 events_webhook（无 secret）/ KMS / 整表 | 遵守 |
| README | 任务 6 |

- [ ] **步骤 2：确认计划无占位步骤；实现分支勿把整史诗 F 标完成（仍欠 F-HOT）**

- [ ] **步骤 3：若实现已合并，ledger 标 F-KV 已交付（实现阶段）**

---

## 自检（写计划时）

1. **规格覆盖：** §2.1–2.6 均有任务；events_webhook 无字段已显式排除。  
2. **占位符：** 无「待定」实现步骤。  
3. **一致性：** 前缀 `bz1:`、env 名、ErrNoKey / ErrCiphertext 全文统一。

---

## 非目标

- F-HOT、DOC、UI 改版  
- 加密 webhook `headers`、渠道 `secret.key` 文件、整表 settings  
- 主密钥轮换 UI、多版本密钥、HKDF（母篇默认 SHA-256）
