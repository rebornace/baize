# 阶段二 2B：微信适配器迁移与跨平台子进程托管 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 把进程内微信渠道迁移为"通用 webhook 渠道实例（name=weixin, source=weixin）+ 独立适配器进程 `cmd/weixin-adapter`"，baize 核心删除进程内 weixin 特例，并新增微信出站真发图/文件、入站 CDN AES 解密能力；默认部署经跨平台子进程托管"装 baize 即用微信"，管理 URL/前端/JSON 字段零改动。

**架构：** 适配器进程承载 iLink 私有协议（扫码登录、长轮询、发消息、媒体加解密/上传、凭据持久化），经 2A 的 JSON-over-HMAC webhook 协议与 baize 通信；baize 的 webhook 渠道增强为动态 account、通用渠道设置热更新、管理面 HMAC 代理、子进程托管；api 层用通用 `ManagedChannel` 接口 + `/v0/settings/channels/{name}/...` 路由取代微信专用 handler。big-bang：任务 11 原子切换（引入 weixin webhook 实例与删除进程内 weixin 同时发生，避免 source 冲突）。

**技术栈：** Go（标准库 `net/http`、`os/exec`、`crypto/aes|cipher|md5|rand|hmac`、`encoding/base64|hex`）、现有 `internal/webhooksig`、`internal/channel`、`internal/channel/webhook`；React 前端零改动。测试：Go `testing` + `httptest`，子进程用 test-fixture 子程序；真机微信为手工验收。

**规格：** `docs/superpowers/specs/2026-09-07-phase2b-weixin-adapter.md`（§8 功能对等回归清单 16 项、§15 媒体协议）。

**铁律：**
- 微信功能零丢失、零回归；§8 清单逐项有测试。
- 每个任务结束 `go build ./... && go test ./...`（相关包）全绿、gofmt/vet 干净；频繁 commit。
- big-bang 前（任务 1–10）不得破坏现有进程内 weixin：空 `channels:` 行为与现状一致（webhook 仍 DeclarativeOnly）。
- 前端零改动（管理 URL `/v0/settings/channels/weixin/*` 与 JSON 字段 `ticket/qr_url/status/agent_id/allowlist/assignee/enabled/running/reason` 逐一对等）。

---

## 文件结构

**核心（baize）修改/新增：**
- `internal/channel/channel.go`（改）：新增 `AccountFromConvID` helper。
- `internal/channel/runtime.go`（改）：`OutboundExtras` 在 context_token 外回填 `account`；`rememberContextToken` 同时缓存 account。
- `internal/channel/manager.go`（新）：`ManagedChannel` 可选接口 + `ChannelSettings`/`AdapterStatus` DTO。
- `internal/channel/webhook/config.go`（改）：新增 `AdminURL`、`AdapterCommand`、`AdapterArgs`、`AdapterAutostart`、`AdapterCredsDir`、`AdapterBaizeURL` 字段；secret 在 autostart 时可空（运行期生成）。
- `internal/channel/webhook/channel.go`（改）：动态 account（`activeAccount` 缓存）、出站 account 解析、`Start/Stop` 接线 supervisor/admin、实现 `ManagedChannel`。
- `internal/channel/webhook/inbound.go`（改）：account 取 `msg.Account` 优先、写 `activeAccount`；allowlist 读热更新字段。
- `internal/channel/webhook/settings.go`（新）：`Settings` DTO + `LoadSettings/SaveSettings`（`data/channels/webhook/<name>/settings.json` 原子写）+ 热应用到 Runtime/allowlist。
- `internal/channel/webhook/admin.go`（新）：管理面 HMAC 代理客户端（login start/poll、logout、status、start/stop）+ `running/reason` 合并。
- `internal/channel/webhook/supervisor.go`（新）：跨平台子进程托管（`os/exec` 拉起、`/healthz` 轮询、关停 Kill、随机 secret 生成与注入）。
- `internal/api/server_channel_managed.go`（新）：通用 `/v0/settings/channels/{name}/...` handler（GET/PUT 设置、login/logout 代理）。
- `internal/api/server.go`（改）：注册通用渠道管理路由；删除 5 条微信硬编码路由。
- `internal/controlplane/acl.go`（改）：微信专用 5 条规则改为通用 `{id}` 规则。
- `internal/bootstrap/bootstrap.go`（改）：移除 weixin blank import 与进程内装配依赖；webhook 实例装配 supervisor/admin/settings；生成并注入 secret。
- `configs/minimal.yaml`、`configs/demo.yaml`（改）：显式声明 webhook-weixin 实例 + autostart。
- 删除：`internal/channel/weixin/`（整包）、`internal/api/server_channel_weixin.go`。

**适配器（新）：**
- `cmd/weixin-adapter/main.go`（新）：flag 配置 + HTTP 服务 + 信号处理 + 长轮询生命周期。
- `cmd/weixin-adapter/config.go`（新）：适配器配置（baize URL、secret、addr、creds 目录、iLink base_url）。
- `cmd/weixin-adapter/inbound.go`（新）：iLink 长轮询 → 组装入站 payload（含 account/媒体 base64）→ 签名 POST baize。
- `cmd/weixin-adapter/outbound.go`（新）：`/outbound` 验签 → 文本 SendMessage / 媒体走上传真发。
- `cmd/weixin-adapter/admin.go`（新）：`/admin/login/start|status`、`/admin/logout`、`/admin/status`、`/admin/start|stop`（HMAC 验签）。
- `cmd/weixin-adapter/internal/weixinlink/`（搬迁+新增）：`ilink.go`、`client.go`、`creds.go`、`fake.go`、`media.go`（新：AES 加解密 + 入站解密下载 + 出站三步上传）+ 测试。

**测试：**
- 各新文件对应 `*_test.go`；`tests/integration/weixin_adapter_test.go`（新，假适配器双向闭环 + §8 可自动化项）；子进程托管用 test-fixture 子程序。

---

## 任务 1：`channel.AccountFromConvID` + 出站 extras 回填 account

**文件：**
- 修改：`internal/channel/channel.go`（新增 helper）
- 修改：`internal/channel/runtime.go`（`rememberContextToken` 同时缓存 account；`OutboundExtras` 回填 account）
- 测试：`internal/channel/runtime_test.go`（新增用例）

- [ ] **步骤 1：编写失败的测试**

在 `internal/channel/runtime_test.go` 末尾追加（白盒测试，包内可直接调用 `rememberContextToken`/`OutboundExtras`）：

```go
func TestOutboundExtrasIncludesAccountAndContextToken(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{}}
	rt, _ := newTestRuntime(t, runs)
	convID := "weixin:acct-1:peer-1"
	rt.rememberContextToken(convID, map[string]string{
		"context_token": "tok-abc",
		"account":       "acct-1",
	})
	ex := rt.OutboundExtras(convID)
	if ex == nil {
		t.Fatal("OutboundExtras returned nil")
	}
	if ex["context_token"] != "tok-abc" {
		t.Fatalf("context_token = %q, want tok-abc", ex["context_token"])
	}
	if ex["account"] != "acct-1" {
		t.Fatalf("account = %q, want acct-1", ex["account"])
	}
	// Only token, no account -> still returns map with context_token.
	rt.rememberContextToken("weixin:acct-2:peer-2", map[string]string{"context_token": "t2"})
	if ex2 := rt.OutboundExtras("weixin:acct-2:peer-2"); ex2["context_token"] != "t2" || ex2["account"] != "" {
		t.Fatalf("ex2 = %+v", ex2)
	}
	// Unknown conversation -> nil.
	if got := rt.OutboundExtras("weixin:nope:x"); got != nil {
		t.Fatalf("unknown conv = %+v, want nil", got)
	}
}

func TestAccountFromConvID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"weixin:acct-1:peer-1", "acct-1"},
		{"webhook:bot9:snowflake", "bot9"},
		{"weixin:acct-1:peer:with:colon", "acct-1"},
		{"ui:abc", ""},
		{"no-colon", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := AccountFromConvID(c.in); got != c.want {
			t.Errorf("AccountFromConvID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/channel/ -run 'TestOutboundExtrasIncludesAccount|TestAccountFromConvID' -v`
预期：编译失败 `undefined: AccountFromConvID`。

- [ ] **步骤 3：实现**

在 `internal/channel/channel.go` 末尾（`SourceFromConvID` 之后）新增：

```go
// AccountFromConvID returns the middle segment of "<source>:<account>:<peer>"
// (the account). It returns "" when the id has fewer than two segments.
func AccountFromConvID(convID string) string {
	first := strings.IndexByte(convID, ':')
	if first < 0 {
		return ""
	}
	rest := convID[first+1:]
	second := strings.IndexByte(rest, ':')
	if second < 0 {
		return ""
	}
	return rest[:second]
}
```

在 `internal/channel/runtime.go` 中：把 `tokens map[string]string` 扩展为同时缓存 account。最小改动——新增并行 map：

```go
	tokenMu sync.Mutex
	tokens  map[string]string // conversation_id -> context_token
	accts   map[string]string // conversation_id -> account
```

`rememberContextToken` 改为同时记录 account（函数名保留，避免改调用点；也可改名 `rememberExtras` 并保留薄封装，二选一，推荐直接扩展）：

```go
func (r *Runtime) rememberContextToken(conversationID string, extras map[string]string) {
	if r == nil || extras == nil {
		return
	}
	r.tokenMu.Lock()
	defer r.tokenMu.Unlock()
	if r.tokens == nil {
		r.tokens = make(map[string]string)
	}
	if r.accts == nil {
		r.accts = make(map[string]string)
	}
	if tok := strings.TrimSpace(extras["context_token"]); tok != "" {
		r.tokens[conversationID] = tok
	}
	if acct := strings.TrimSpace(extras["account"]); acct != "" {
		r.accts[conversationID] = acct
	}
}
```

`OutboundExtras` 回填两者：

```go
func (r *Runtime) OutboundExtras(conversationID string) map[string]string {
	if r == nil {
		return nil
	}
	r.tokenMu.Lock()
	defer r.tokenMu.Unlock()
	tok := r.tokens[conversationID]
	acct := r.accts[conversationID]
	if tok == "" && acct == "" {
		return nil
	}
	out := map[string]string{}
	if tok != "" {
		out["context_token"] = tok
	}
	if acct != "" {
		out["account"] = acct
	}
	return out
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/channel/ -v`
预期：PASS（含原有 router/outbound/runtime 测试不回归）。

- [ ] **步骤 5：Commit**

```bash
git add internal/channel/channel.go internal/channel/runtime.go internal/channel/runtime_test.go
git commit -m "feat(channel): AccountFromConvID helper + outbound extras carry account"
```




## 任务 2：建立适配器 iLink 库 `cmd/weixin-adapter/internal/weixinlink`（搬迁）

**文件：**
- 创建（拷贝自 `internal/channel/weixin/`，仅改包名）：
  - `cmd/weixin-adapter/internal/weixinlink/ilink.go`
  - `cmd/weixin-adapter/internal/weixinlink/client.go`
  - `cmd/weixin-adapter/internal/weixinlink/creds.go`
  - `cmd/weixin-adapter/internal/weixinlink/fake.go`
  - `cmd/weixin-adapter/internal/weixinlink/client_test.go`
- 不删除原 `internal/channel/weixin/`（big-bang 在任务 11 才删；期间两包并存、各自编译）。

- [ ] **步骤 1：拷贝并改包名**

把上述 5 个文件从 `internal/channel/weixin/` 复制到 `cmd/weixin-adapter/internal/weixinlink/`，每个文件：
- 首行 `package weixin` → `package weixinlink`。
- 不改任何 import（这些文件只依赖标准库；`client.go`/`ilink.go`/`creds.go`/`fake.go`/`client_test.go` 均不 import `github.com/rebornace/baize/...`）。
- `client_test.go` 内引用的 `NewClient/NewFake/GetQR/PollLogin/GetUpdates/SendMessage/DownloadMedia` 与类型 `Update/OutboundMessage/MediaRef`、常量 `LoginStatus*`/`DefaultBaseURL` 均为包内符号，改名后无需改动。

> 注意：`channel.go`、`settings.go`、`channel_test.go`、`bootstrap_test.go` **不搬**（它们是进程内渠道的轮询/装配/白名单逻辑，2B 由适配器 `inbound.go` 与 baize webhook settings 取代；任务 11 随包删除）。

- [ ] **步骤 2：运行测试验证通过**

运行：`go test ./cmd/weixin-adapter/internal/weixinlink/ -v`
预期：PASS（原协议级测试全绿，证明搬迁无行为变化）。同时 `go build ./...` 仍绿（旧 weixin 包并存）。

- [ ] **步骤 3：Commit**

```bash
git add cmd/weixin-adapter/internal/weixinlink/
git commit -m "feat(weixin-adapter): seed weixinlink iLink client library (moved from channel/weixin)"
```




## 任务 3：`weixinlink` AES-128-ECB 加解密 + 入站媒体解密

**文件：**
- 创建：`cmd/weixin-adapter/internal/weixinlink/media.go`（crypto helpers + `DownloadMediaDecrypted` + `resolveAESKey`）
- 修改：`cmd/weixin-adapter/internal/weixinlink/fake.go`（Fake 增加 `DownloadMediaDecrypted`）
- 测试：`cmd/weixin-adapter/internal/weixinlink/media_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `cmd/weixin-adapter/internal/weixinlink/media_test.go`：

```go
package weixinlink

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAESRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef") // 16 bytes
	for _, plain := range [][]byte{
		[]byte("a"),
		[]byte("exactly 16 bytes"), // len 16 -> full block of padding
		[]byte("hello weixin image bytes"),
		bytes.Repeat([]byte{0xAB}, 100),
	} {
		ct := encryptAES128ECB(key, plain)
		if len(ct)%16 != 0 {
			t.Fatalf("ciphertext not block-aligned: %d", len(ct))
		}
		if bytes.Equal(ct, plain) {
			t.Fatal("ciphertext equals plaintext")
		}
		got, err := decryptAES128ECB(key, ct)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if !bytes.Equal(got, plain) {
			t.Fatalf("round-trip mismatch: got %q want %q", got, plain)
		}
	}
}

func TestDecryptRejectsBadPadding(t *testing.T) {
	key := []byte("0123456789abcdef")
	bad := bytes.Repeat([]byte{0xFF}, 32) // not valid PKCS7
	if _, err := decryptAES128ECB(key, bad); err == nil {
		t.Fatal("expected padding error")
	}
}

func TestResolveAESKey(t *testing.T) {
	raw := []byte("0123456789abcdef")
	hexKey := hex.EncodeToString(raw)                          // image_item.aeskey
	b64Raw := base64.StdEncoding.EncodeToString(raw)           // format A
	b64Hex := base64.StdEncoding.EncodeToString([]byte(hexKey)) // format B
	for _, in := range []string{hexKey, b64Raw, b64Hex} {
		got, err := resolveAESKey(in)
		if err != nil {
			t.Fatalf("resolveAESKey(%q): %v", in, err)
		}
		if !bytes.Equal(got, raw) {
			t.Fatalf("resolveAESKey(%q) = %x", in, got)
		}
	}
	if _, err := resolveAESKey(""); err == nil {
		t.Fatal("empty key should error")
	}
	if _, err := resolveAESKey("not-a-key"); err == nil {
		t.Fatal("garbage key should error")
	}
}

func TestDownloadMediaDecrypted(t *testing.T) {
	key := []byte("0123456789abcdef")
	plain := []byte("PNG-PLAINTEXT-BYTES")
	ciphertext := encryptAES128ECB(key, plain)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(ciphertext)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, srv.Client())
	ref := MediaRef{URL: srv.URL + "/f", AESKey: base64.StdEncoding.EncodeToString(key)}
	data, dec, err := c.DownloadMediaDecrypted(context.Background(), "tok", ref)
	if err != nil {
		t.Fatalf("DownloadMediaDecrypted: %v", err)
	}
	if !dec || !bytes.Equal(data, plain) {
		t.Fatalf("dec=%v data=%q", dec, data)
	}

	// No key -> raw bytes, decrypted=false.
	raw, dec2, err := c.DownloadMediaDecrypted(context.Background(), "tok", MediaRef{URL: srv.URL + "/f"})
	if err != nil || dec2 || !bytes.Equal(raw, ciphertext) {
		t.Fatalf("no-key: dec=%v err=%v", dec2, err)
	}

	// Key present but body not encrypted -> nil, decrypted=false (degrade).
	plainSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-encrypted-plain"))
	}))
	t.Cleanup(plainSrv.Close)
	c2 := NewClient(plainSrv.URL, plainSrv.Client())
	data3, dec3, err := c2.DownloadMediaDecrypted(context.Background(), "tok",
		MediaRef{URL: plainSrv.URL + "/f", AESKey: base64.StdEncoding.EncodeToString(key)})
	if err != nil {
		t.Fatalf("undecryptable should not error: %v", err)
	}
	if dec3 || data3 != nil {
		t.Fatalf("undecryptable: dec=%v data=%v", dec3, data3)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./cmd/weixin-adapter/internal/weixinlink/ -run 'TestAES|TestResolveAESKey|TestDownloadMediaDecrypted' -v`
预期：编译失败 `undefined: encryptAES128ECB / decryptAES128ECB / resolveAESKey / DownloadMediaDecrypted`。

- [ ] **步骤 3：实现 `media.go`**

创建 `cmd/weixin-adapter/internal/weixinlink/media.go`：

```go
package weixinlink

import (
	"context"
	"crypto/aes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// iLink media uses AES-128-ECB + PKCS7 padding for both inbound CDN downloads
// (decrypt) and outbound CDN uploads (encrypt).

// MediaDownloader downloads and (when keyed) decrypts inbound CDN media.
type MediaDownloader interface {
	// DownloadMediaDecrypted returns plaintext and decrypted=true when the
	// media carried a usable AES key and decrypted cleanly; raw bytes and
	// decrypted=false when no key is present; nil and decrypted=false when a
	// key was present but decryption failed (caller degrades to a filename
	// placeholder rather than forwarding ciphertext garbage).
	DownloadMediaDecrypted(ctx context.Context, token string, m MediaRef) (data []byte, decrypted bool, err error)
}

func encryptAES128ECB(key, plaintext []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic("weixinlink: invalid aes key: " + err.Error())
	}
	padded := pkcs7Pad(plaintext, block.BlockSize())
	out := make([]byte, len(padded))
	bs := block.BlockSize()
	for i := 0; i < len(padded); i += bs {
		block.Encrypt(out[i:i+bs], padded[i:i+bs])
	}
	return out
}

func decryptAES128ECB(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	bs := block.BlockSize()
	if len(ciphertext) == 0 || len(ciphertext)%bs != 0 {
		return nil, fmt.Errorf("weixinlink: ciphertext not a multiple of block size %d", bs)
	}
	out := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += bs {
		block.Decrypt(out[i:i+bs], ciphertext[i:i+bs])
	}
	return pkcs7Unpad(out, bs)
}

func pkcs7Pad(in []byte, blockSize int) []byte {
	pad := blockSize - len(in)%blockSize
	out := make([]byte, len(in)+pad)
	copy(out, in)
	for i := len(in); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out
}

func pkcs7Unpad(in []byte, blockSize int) ([]byte, error) {
	if len(in) == 0 || len(in)%blockSize != 0 {
		return nil, fmt.Errorf("weixinlink: invalid padded length")
	}
	pad := int(in[len(in)-1])
	if pad < 1 || pad > blockSize {
		return nil, fmt.Errorf("weixinlink: invalid PKCS7 padding %d", pad)
	}
	for i := len(in) - pad; i < len(in); i++ {
		if int(in[i]) != pad {
			return nil, fmt.Errorf("weixinlink: inconsistent PKCS7 padding")
		}
	}
	return in[:len(in)-pad], nil
}

// resolveAESKey parses the inbound media aes key in one of:
//   - image_item.aeskey: 32-char hex of the 16-byte key
//   - media.aes_key format A: base64(raw 16 bytes)
//   - media.aes_key format B: base64(32-char hex string)
func resolveAESKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("weixinlink: empty aes key")
	}
	if b, err := hex.DecodeString(s); err == nil && len(b) == 16 {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		if len(b) == 16 {
			return b, nil
		}
		if h := strings.TrimSpace(string(b)); len(h) == 32 {
			if kb, err := hex.DecodeString(h); err == nil && len(kb) == 16 {
				return kb, nil
			}
		}
	}
	return nil, fmt.Errorf("weixinlink: unrecognized aes key format")
}

// DownloadMediaDecrypted downloads CDN bytes and decrypts them when keyed.
func (c *Client) DownloadMediaDecrypted(ctx context.Context, _ string, m MediaRef) ([]byte, bool, error) {
	raw, err := c.DownloadMedia(ctx, "", m) // CDN needs no bearer token
	if err != nil {
		return nil, false, err
	}
	key, kerr := resolveAESKey(m.AESKey)
	if kerr != nil {
		return raw, false, nil // no usable key: forward raw
	}
	pt, derr := decryptAES128ECB(key, raw)
	if derr != nil {
		return nil, false, nil // key present but not decryptable: degrade
	}
	return pt, true, nil
}

var _ MediaDownloader = (*Client)(nil)
```

> `Client.DownloadMedia(ctx, token, media)` 已存在于迁入的 `client.go`（其 `token` 形参未使用，CDN 不需 bearer）。

- [ ] **步骤 4：给 Fake 加 `DownloadMediaDecrypted` + 搬迁客户端按项类型填 MIME**

在 `fake.go` 的 `Fake` 上新增：

```go
func (f *Fake) DownloadMediaDecrypted(context.Context, string, MediaRef) ([]byte, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]byte, len(f.MediaBytes))
	copy(out, f.MediaBytes)
	return out, false, nil // fake media is plaintext
}
```

同时，在搬迁后的 `client.go` 的 `mediaRefFromItem` 中补齐 `MIME`（现状只填 FileName，导致入站图片在 baize 侧无法按视觉附件路由）。把构造 `MediaRef` 的返回改为按项类型给 MIME：

```go
	mime := ""
	switch {
	case item.ImageItem != nil:
		mime = "image/jpeg" // iLink inbound images are JPEG; filename carries extension
	case item.FileItem != nil:
		mime = "application/octet-stream"
	}
	return &MediaRef{
		EncryptQueryParam: media.EncryptQueryParam,
		AESKey:            aesKey,
		FileName:          fileName,
		MIME:              mime,
	}
```

（`mediaRefFromItem` 原函数尾部 `return &MediaRef{...}` 据此替换；voice/video 仍 MIME 空，本期不处理。）

- [ ] **步骤 5：运行测试验证通过**

运行：`go test ./cmd/weixin-adapter/internal/weixinlink/ -v`
预期：PASS（原协议测试 + 新 crypto/解密测试）。

- [ ] **步骤 6：Commit**

```bash
git add cmd/weixin-adapter/internal/weixinlink/media.go cmd/weixin-adapter/internal/weixinlink/media_test.go cmd/weixin-adapter/internal/weixinlink/fake.go
git commit -m "feat(weixinlink): AES-128-ECB/PKCS7 + inbound CDN media decryption"
```




## 任务 4：`weixinlink` 出站三步上传（真发图片/文件）

**文件：**
- 创建：`cmd/weixin-adapter/internal/weixinlink/upload.go`（`SendImage`/`SendFile` + `getuploadurl` + CDN PUT + sendmessage item）
- 修改：`cmd/weixin-adapter/internal/weixinlink/ilink.go`（`ILink` 接口加 `SendImage`/`SendFile`）
- 修改：`cmd/weixin-adapter/internal/weixinlink/client.go`（加 CDN 上传端点常量与方法）
- 修改：`cmd/weixin-adapter/internal/weixinlink/fake.go`（Fake 记录上传/发送的媒体项）
- 测试：`cmd/weixin-adapter/internal/weixinlink/upload_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `cmd/weixin-adapter/internal/weixinlink/upload_test.go`（httptest 同时模拟 iLink API 与 CDN）：

```go
package weixinlink

import (
	"bytes"
	"context"
	"crypto/aes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestSendImageThreeStepUpload(t *testing.T) {
	var (
		mu        sync.Mutex
		gotUpload map[string]any
		cdnBody   []byte
		sendMsg   map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ilink/bot/getuploadurl":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotUpload)
			// CDN upload URL points back at this server under /cdn.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"upload_param":  "up-param",
				"upload_full_url": "",
			})
		case strings.HasPrefix(r.URL.Path, "/cdn/upload"):
			b, _ := io.ReadAll(r.Body)
			cdnBody = b
			// CDN returns the download handle in a response header.
			w.Header().Set("x-encrypted-param", "dl-param-XYZ")
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/ilink/bot/sendmessage":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &sendMsg)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	// NewClient sets CDNBaseURL to the default; override for the test.
	c := NewClient(srv.URL, srv.Client())
	c.CDNBaseURL = srv.URL + "/cdn"

	img := []byte("FAKE-PNG-BYTES")
	if err := c.SendImage(context.Background(), "tok", "peer@im.wechat", "pic.png", "image/png", img, "ctx-tok"); err != nil {
		t.Fatalf("SendImage: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	// Step 1: getuploadurl fields.
	if gotUpload["media_type"].(float64) != 1 {
		t.Fatalf("media_type=%v want 1", gotUpload["media_type"])
	}
	if gotUpload["to_user_id"] != "peer@im.wechat" {
		t.Fatalf("to_user_id=%v", gotUpload["to_user_id"])
	}
	if gotUpload["rawsize"].(float64) != float64(len(img)) {
		t.Fatalf("rawsize=%v", gotUpload["rawsize"])
	}
	if gotUpload["rawfilemd5"] == "" || gotUpload["aeskey"] == "" {
		t.Fatalf("missing md5/aeskey: %v", gotUpload)
	}
	if gotUpload["filesize"].(float64) != float64(len(cdnBody)) {
		t.Fatalf("filesize=%v cdn=%d", gotUpload["filesize"], len(cdnBody))
	}
	// CDN got ciphertext that decrypts back to the image with the advertised
	// aeskey (hex).
	keyBytes, err := hex.DecodeString(gotUpload["aeskey"].(string))
	if err != nil || len(keyBytes) != 16 {
		t.Fatalf("aeskey not 16-byte hex: %v", gotUpload["aeskey"])
	}
	block, _ := aes.NewCipher(keyBytes)
	if len(cdnBody)%16 != 0 {
		t.Fatalf("cdn body not block aligned: %d", len(cdnBody))
	}
	pt := make([]byte, len(cdnBody))
	for i := 0; i < len(cdnBody); i += 16 {
		block.Decrypt(pt[i:i+16], cdnBody[i:i+16])
	}
	if !bytes.Contains(pt, img) {
		t.Fatalf("decrypted CDN body does not contain image: %q", pt)
	}
	// Step 3: sendmessage image_item references x-encrypted-param + base64 key.
	msg := sendMsg["msg"].(map[string]any)
	if msg["context_token"] != "ctx-tok" || msg["to_user_id"] != "peer@im.wechat" {
		t.Fatalf("send msg envelope=%v", msg)
	}
	items := msg["item_list"].([]any)
	item := items[0].(map[string]any)
	if item["type"].(float64) != 2 {
		t.Fatalf("item type=%v want 2 (image)", item["type"])
	}
	imgItem := item["image_item"].(map[string]any)
	media := imgItem["media"].(map[string]any)
	if media["encrypt_query_param"] != "dl-param-XYZ" {
		t.Fatalf("encrypt_query_param=%v", media["encrypt_query_param"])
	}
	if media["aes_key"] != base64.StdEncoding.EncodeToString(keyBytes) {
		t.Fatalf("aes_key=%v", media["aes_key"])
	}
}

func TestSendFileItemTypeFour(t *testing.T) {
	var sendMsg map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ilink/bot/getuploadurl":
			_ = json.NewEncoder(w).Encode(map[string]any{"upload_param": "up"})
		case strings.HasPrefix(r.URL.Path, "/cdn/upload"):
			w.Header().Set("x-encrypted-param", "dl-file")
		case r.URL.Path == "/ilink/bot/sendmessage":
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &sendMsg)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, srv.Client())
	c.CDNBaseURL = srv.URL + "/cdn"

	if err := c.SendFile(context.Background(), "tok", "peer@im.wechat", "report.pdf", "application/pdf", []byte("PDFDATA"), ""); err != nil {
		t.Fatalf("SendFile: %v", err)
	}
	item := sendMsg["msg"].(map[string]any)["item_list"].([]any)[0].(map[string]any)
	if item["type"].(float64) != 4 {
		t.Fatalf("file item type=%v want 4", item["type"])
	}
	fileItem := item["file_item"].(map[string]any)
	if fileItem["file_name"] != "report.pdf" {
		t.Fatalf("file_name=%v", fileItem["file_name"])
	}
	if fileItem["media"].(map[string]any)["encrypt_query_param"] != "dl-file" {
		t.Fatalf("file media param missing")
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./cmd/weixin-adapter/internal/weixinlink/ -run 'TestSendImage|TestSendFile' -v`
预期：编译失败 `c.SendImage undefined` / `c.SendFile undefined`。

- [ ] **步骤 3：扩展 `ILink` 接口与 `Fake`**

在 `ilink.go` 的 `ILink` 接口中追加（`SendMessage` 之后）：

```go
	// SendImage uploads data to the iLink CDN and sends an image message item.
	SendImage(ctx context.Context, token, toUserID, filename, mime string, data []byte, contextToken string) error
	// SendFile uploads data to the iLink CDN and sends a file attachment item.
	SendFile(ctx context.Context, token, toUserID, filename, mime string, data []byte, contextToken string) error
```

在 `fake.go` 中：把 `Fake` 结构体在现有 `Sent []OutboundMessage` 字段后追加两个字段，并新增两个方法（签名与接口一致）：

```go
type FakeMedia struct {
	Token        string
	ToUserID     string
	FileName     string
	MIME         string
	Data         []byte
	ContextToken string
}

// 结构体追加字段：
//   SentImages []FakeMedia
//   SentFiles  []FakeMedia

func (f *Fake) SendImage(_ context.Context, token, toUserID, filename, mime string, data []byte, contextToken string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SentImages = append(f.SentImages, FakeMedia{Token: token, ToUserID: toUserID, FileName: filename, MIME: mime, Data: append([]byte(nil), data...), ContextToken: contextToken})
	return nil
}

func (f *Fake) SendFile(_ context.Context, token, toUserID, filename, mime string, data []byte, contextToken string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SentFiles = append(f.SentFiles, FakeMedia{Token: token, ToUserID: toUserID, FileName: filename, MIME: mime, Data: append([]byte(nil), data...), ContextToken: contextToken})
	return nil
}
```

- [ ] **步骤 4：给 `client.go` 的出站 wire 类型补字段**

在 `client.go` 的 `wireMediaItem` 结构体补两个出站字段（入站反序列化时 omitempty 忽略，无副作用）：

```go
type wireMediaItem struct {
	Media    *wireCDNMedia `json:"media,omitempty"`
	FileName string        `json:"file_name,omitempty"`
	AESKey   string        `json:"aeskey,omitempty"`
	MidSize  int64         `json:"mid_size,omitempty"` // outbound image
	Len      string        `json:"len,omitempty"`      // outbound file
}
```

并把 `SendMessage` 重构为复用新的 `sendItems`（DRY，client_id 生成集中到一处）。把现有 `SendMessage` 方法体替换为：

```go
func (c *Client) SendMessage(ctx context.Context, token string, msg OutboundMessage) error {
	return c.sendItems(ctx, token, msg.ToUserID, msg.ContextToken, []wireItem{{
		Type:     itemTypeText,
		TextItem: &wireTextItem{Text: msg.Text},
	}})
}

// sendItems posts a sendmessage with the given item list, stamping auth,
// client_id, message type/state, base_info, and context_token.
func (c *Client) sendItems(ctx context.Context, token, toUserID, contextToken string, items []wireItem) error {
	body := sendMessageRequest{
		Msg: wireOutboundMessage{
			ToUserID:     toUserID,
			ClientID:     fmt.Sprintf("baize:%d-%d", time.Now().UnixMilli(), rand.Intn(1<<16)),
			MessageType:  messageTypeBot,
			MessageState: messageStateFinish,
			ContextToken: contextToken,
			ItemList:     items,
		},
		BaseInfo: baseInfo{ChannelVersion: channelVersion},
	}
	req, err := c.newAuthJSONRequest(ctx, http.MethodPost, c.BaseURL+pathSendMessage, token, body)
	if err != nil {
		return fmt.Errorf("weixin sendmessage: %w", err)
	}
	if err := c.doJSON(req, nil); err != nil {
		return fmt.Errorf("weixin sendmessage: %w", err)
	}
	return nil
}
```

- [ ] **步骤 5：实现 `upload.go`**

创建 `cmd/weixin-adapter/internal/weixinlink/upload.go`：

```go
package weixinlink

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	pathGetUploadURL = "/ilink/bot/getuploadurl"
	mediaTypeImage   = 1 // getuploadurl media_type: image
	mediaTypeFile    = 3 // getuploadurl media_type: file
	itemTypeImage    = 2 // sendmessage item type: image
	itemTypeFile     = 4 // sendmessage item type: file
)

type uploadURLResponse struct {
	UploadParam   string `json:"upload_param"`
	UploadFullURL string `json:"upload_full_url"`
	Ret           int    `json:"ret"`
	ErrCode       int    `json:"errcode"`
	ErrMsg        string `json:"errmsg"`
}

// SendImage encrypts data, uploads it to the iLink CDN, and sends an image
// message item to toUserID. contextToken is the inbound message's token.
func (c *Client) SendImage(ctx context.Context, token, toUserID, filename, mime string, data []byte, contextToken string) error {
	dlParam, aesKeyB64, cipherLen, err := c.uploadMedia(ctx, token, toUserID, mediaTypeImage, data)
	if err != nil {
		return fmt.Errorf("weixin upload image: %w", err)
	}
	return c.sendItems(ctx, token, toUserID, contextToken, []wireItem{{
		Type: itemTypeImage,
		ImageItem: &wireMediaItem{
			Media:   &wireCDNMedia{EncryptQueryParam: dlParam, AESKey: aesKeyB64, EncryptType: 1},
			MidSize: cipherLen,
		},
	}})
}

// SendFile encrypts data, uploads it, and sends a file attachment item.
func (c *Client) SendFile(ctx context.Context, token, toUserID, filename, mime string, data []byte, contextToken string) error {
	dlParam, aesKeyB64, _, err := c.uploadMedia(ctx, token, toUserID, mediaTypeFile, data)
	if err != nil {
		return fmt.Errorf("weixin upload file: %w", err)
	}
	name := strings.TrimSpace(filename)
	if name == "" {
		name = "file.bin"
	}
	return c.sendItems(ctx, token, toUserID, contextToken, []wireItem{{
		Type: itemTypeFile,
		FileItem: &wireMediaItem{
			Media:    &wireCDNMedia{EncryptQueryParam: dlParam, AESKey: aesKeyB64, EncryptType: 1},
			FileName: name,
			Len:      strconv.Itoa(len(data)),
		},
	}})
}

// uploadMedia performs the three-step iLink outbound media flow: generate an
// AES key + encrypt (AES-128-ECB/PKCS7), request a CDN upload URL, PUT the
// ciphertext, and return the CDN download handle (x-encrypted-param), the
// base64 AES key for the sendmessage item, and the ciphertext length.
func (c *Client) uploadMedia(ctx context.Context, token, toUserID string, mediaType int, data []byte) (string, string, int64, error) {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return "", "", 0, fmt.Errorf("gen aes key: %w", err)
	}
	filekeyRaw := make([]byte, 16)
	if _, err := rand.Read(filekeyRaw); err != nil {
		return "", "", 0, fmt.Errorf("gen filekey: %w", err)
	}
	filekeyHex := hex.EncodeToString(filekeyRaw)
	ciphertext := encryptAES128ECB(key, data)
	sum := md5.Sum(data)

	reqBody := map[string]any{
		"filekey":        filekeyHex,
		"media_type":     mediaType,
		"to_user_id":     toUserID,
		"rawsize":        len(data),
		"rawfilemd5":     hex.EncodeToString(sum[:]),
		"filesize":       len(ciphertext),
		"no_need_thumb":  true,
		"aeskey":         hex.EncodeToString(key),
		"base_info":      map[string]string{"channel_version": channelVersion},
	}
	preq, err := c.newAuthJSONRequest(ctx, http.MethodPost, c.BaseURL+pathGetUploadURL, token, reqBody)
	if err != nil {
		return "", "", 0, err
	}
	var up uploadURLResponse
	if err := c.doJSON(preq, &up); err != nil {
		return "", "", 0, fmt.Errorf("getuploadurl: %w", err)
	}
	if up.Ret != 0 || up.ErrCode != 0 {
		return "", "", 0, fmt.Errorf("getuploadurl: ret=%d errcode=%d errmsg=%s", up.Ret, up.ErrCode, up.ErrMsg)
	}
	uploadURL := strings.TrimSpace(up.UploadFullURL)
	if uploadURL == "" {
		uploadURL = strings.TrimRight(c.CDNBaseURL, "/") +
			"/upload?encrypted_query_param=" + url.QueryEscape(up.UploadParam) +
			"&filekey=" + url.QueryEscape(filekeyHex)
	}

	putReq, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(ciphertext))
	if err != nil {
		return "", "", 0, err
	}
	putReq.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.HTTP.Do(putReq)
	if err != nil {
		return "", "", 0, fmt.Errorf("cdn upload: %w", err)
	}
	dlParam := resp.Header.Get("x-encrypted-param")
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", 0, fmt.Errorf("cdn upload: http %s", resp.Status)
	}
	if dlParam == "" {
		return "", "", 0, fmt.Errorf("cdn upload: missing x-encrypted-param response header")
	}
	return dlParam, base64.StdEncoding.EncodeToString(key), int64(len(ciphertext)), nil
}

var _ = json.Marshal // keep encoding/json import if future fields need it
```

> 若 `encoding/json` 未被使用导致编译错误，删除该 import 与末尾的 `var _ = json.Marshal`（`uploadURLResponse` 由 `doJSON` 反序列化，不需要直接引用 json；实际可去掉 json import）。

- [ ] **步骤 6：运行测试验证通过**

运行：`go test ./cmd/weixin-adapter/internal/weixinlink/ -v`
预期：PASS（含上传三步、文件项、原协议与入站解密测试）。

- [ ] **步骤 7：Commit**

```bash
git add cmd/weixin-adapter/internal/weixinlink/
git commit -m "feat(weixinlink): outbound image/file send via getuploadurl + CDN upload"
```




## 任务 5：适配器骨架——配置、`Adapter`、`/healthz`、main 装配

**文件：**
- 创建：`cmd/weixin-adapter/config.go`（flag 解析）
- 创建：`cmd/weixin-adapter/adapter.go`（`Adapter` 结构 + 依赖 + `/healthz`）
- 创建：`cmd/weixin-adapter/main.go`（flag → 构造 → HTTP 服务 + 信号关停）
- 测试：`cmd/weixin-adapter/config_test.go`、`cmd/weixin-adapter/adapter_test.go`

- [ ] **步骤 1：编写失败的测试**

`cmd/weixin-adapter/config_test.go`：

```go
package main

import (
	"testing"
)

func TestParseFlagsDefaults(t *testing.T) {
	cfg, err := parseFlags([]string{"-baize=http://127.0.0.1:8080/v0/channels/weixin/inbound", "-secret=s"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaizeInboundURL == "" || cfg.Secret != "s" {
		t.Fatalf("cfg=%+v", cfg)
	}
	if cfg.Addr == "" {
		t.Fatal("addr default empty")
	}
	if cfg.CredsDir == "" {
		t.Fatal("creds dir default empty")
	}
}

func TestParseFlagsRequiresSecretAndBaize(t *testing.T) {
	if _, err := parseFlags([]string{"-baize=x"}); err == nil {
		t.Fatal("expected error for missing secret")
	}
	if _, err := parseFlags([]string{"-secret=s"}); err == nil {
		t.Fatal("expected error for missing baize url")
	}
}
```

`cmd/weixin-adapter/adapter_test.go`：

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
)

func TestHealthz(t *testing.T) {
	a := &Adapter{ilink: weixinlink.NewFake()}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	a.handleHealthz(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz=%d", rec.Code)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./cmd/weixin-adapter/ -run 'TestParseFlags|TestHealthz' -v`
预期：编译失败 `undefined: parseFlags / Adapter`。

- [ ] **步骤 3：实现 `config.go`**

```go
package main

import (
	"errors"
	"flag"
	"strings"
)

// Config is the adapter process configuration, supplied via command-line
// flags by baize (autostart) or by an operator (standalone deployment).
type Config struct {
	BaizeInboundURL string // baize inbound webhook to POST messages to
	Secret          string // shared HMAC secret with baize
	Addr            string // adapter listen address (admin/outbound/healthz)
	CredsDir        string // iLink creds.json directory (default ./data/channels/weixin)
	ILinkBaseURL    string // override iLink API base (tests)
}

func parseFlags(args []string) (Config, error) {
	fs := flag.NewFlagSet("weixin-adapter", flag.ContinueOnError)
	var cfg Config
	fs.StringVar(&cfg.BaizeInboundURL, "baize", "", "baize inbound webhook URL")
	fs.StringVar(&cfg.Secret, "secret", "", "shared HMAC secret")
	fs.StringVar(&cfg.Addr, "addr", "127.0.0.1:8090", "listen address")
	fs.StringVar(&cfg.CredsDir, "creds", "./data/channels/weixin", "creds directory")
	fs.StringVar(&cfg.ILinkBaseURL, "ilink-base", "", "iLink API base URL override")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if strings.TrimSpace(cfg.Secret) == "" {
		return cfg, errors.New("weixin-adapter: -secret is required")
	}
	if strings.TrimSpace(cfg.BaizeInboundURL) == "" {
		return cfg, errors.New("weixin-adapter: -baize (inbound url) is required")
	}
	return cfg, nil
}
```

- [ ] **步骤 4：实现 `adapter.go`（骨架）**

```go
package main

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
)

// Adapter holds the weixin-adapter process dependencies and runtime state.
type Adapter struct {
	ilink weixinlink.ILink // iLink client (real or fake in tests)

	baizeInboundURL string
	secret          string
	credsDir        string

	mu       sync.Mutex
	account  string
	token    string
	polling  bool
	cancel   func() // stops the poll loop
}

func (a *Adapter) handleHealthz(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	polling := a.polling
	hasCreds := a.token != "" && a.account != ""
	a.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":          "ok",
		"polling":         polling,
		"has_credentials": hasCreds,
	})
}

// routes returns the adapter's HTTP mux. Endpoints are added in later tasks;
// /healthz is always present.
func (a *Adapter) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleHealthz)
	return mux
}
```

- [ ] **步骤 5：实现 `main.go`**

```go
// Command weixin-adapter is the out-of-process WeChat (iLink) adapter for
// baize. It logs into iLink via QR (driven by baize's admin proxy), long-polls
// inbound messages and signs them to baize's webhook, and serves /outbound for
// baize to send text/images/files back to WeChat.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
)

func main() {
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
	client := weixinlink.NewClient(cfg.ILinkBaseURL, nil)
	a := &Adapter{
		ilink:           client,
		baizeInboundURL: cfg.BaizeInboundURL,
		secret:          cfg.Secret,
		credsDir:        cfg.CredsDir,
	}
	// Load any persisted credentials at startup (baize may call /admin/start).
	if acct, tok, lerr := weixinlink.LoadCreds(cfg.CredsDir); lerr == nil {
		a.setCredentials(acct, tok)
	}

	srv := &http.Server{Addr: cfg.Addr, Handler: a.routes()}
	go func() {
		log.Printf("weixin-adapter listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a.stopPolling()
	_ = srv.Shutdown(ctx)
}
```

> `a.setCredentials` 与 `a.stopPolling` 在任务 6 实现；为让任务 5 独立编译，本步先在 `adapter.go` 加最小存取方法（任务 6 扩展）：

```go
func (a *Adapter) setCredentials(account, token string) {
	a.mu.Lock()
	a.account = account
	a.token = token
	a.mu.Unlock()
}

func (a *Adapter) stopPolling() {
	a.mu.Lock()
	cancel := a.cancel
	a.cancel = nil
	a.polling = false
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
```

- [ ] **步骤 6：运行测试验证通过**

运行：`go build ./cmd/weixin-adapter/... && go test ./cmd/weixin-adapter/ -v`
预期：PASS；`go build ./...` 全绿。

- [ ] **步骤 7：Commit**

```bash
git add cmd/weixin-adapter/
git commit -m "feat(weixin-adapter): process skeleton (config, adapter, healthz, main)"
```




## 任务 6：适配器管理面 `/admin/*` + 轮询生命周期 + 文本入站转发

**文件：**
- 创建：`cmd/weixin-adapter/admin.go`（login start/status、logout、status、start/stop，HMAC 验签）
- 创建：`cmd/weixin-adapter/poller.go`（轮询 goroutine 生命周期 + 文本入站组装/签名 POST）
- 修改：`cmd/weixin-adapter/adapter.go`（`routes()` 挂载 `/admin/*` 与 `/outbound` 占位；新增轮询字段）
- 测试：`cmd/weixin-adapter/admin_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `cmd/weixin-adapter/admin_test.go`：

```go
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
	"github.com/rebornace/baize/internal/webhooksig"
)

const testSecret = "adm-secret"

// signedReq builds a baize->adapter admin request with a valid HMAC signature.
func signedReq(t *testing.T, method, target, secret string, body []byte) *http.Request {
	t.Helper()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := webhooksig.Sign(secret, ts, body)
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Baize-Channel-Timestamp", ts)
	req.Header.Set("X-Baize-Channel-Signature", sig)
	return req
}

func newTestAdapter(t *testing.T, fake *weixinlink.Fake, baizeURL string) *Adapter {
	t.Helper()
	dir := t.TempDir()
	a := &Adapter{
		ilink:           fake,
		baizeInboundURL: baizeURL,
		secret:          testSecret,
		credsDir:        dir,
		emptyWait:       5 * time.Millisecond,
		httpClient:      &http.Client{Timeout: 2 * time.Second},
	}
	return a
}

func TestAdminRequiresSignature(t *testing.T) {
	a := newTestAdapter(t, weixinlink.NewFake(), "")
	req := httptest.NewRequest(http.MethodPost, "/admin/logout", nil)
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned admin = %d, want 401", rec.Code)
	}
}

func TestAdminLoginStartStatusAndAutoPoll(t *testing.T) {
	// baize inbound receiver: capture the first forwarded message.
	var (
		mu      sync.Mutex
		gotBody map[string]any
		gotCh   = make(chan struct{}, 1)
	)
	baize := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		// Verify adapter->baize signature too.
		if err := webhooksig.Verify(testSecret, r.Header.Get("X-Baize-Channel-Timestamp"), b,
			r.Header.Get("X-Baize-Channel-Signature"), time.Now(), 300*time.Second); err != nil {
			t.Errorf("baize inbound bad signature: %v", err)
		}
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		mu.Lock()
		gotBody = m
		mu.Unlock()
		select {
		case gotCh <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(baize.Close)

	fake := weixinlink.NewFake()
	fake.Updates = []weixinlink.Update{{
		PeerID:       "peer@im.wechat",
		Text:         "你好适配器",
		ContextToken: "ctx-1",
	}}
	a := newTestAdapter(t, fake, baize.URL)

	// login/start
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, signedReq(t, http.MethodPost, "/admin/login/start", testSecret, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("login/start = %d body=%s", rec.Code, rec.Body.String())
	}
	var ls struct {
		Ticket string `json:"ticket"`
		QRURL  string `json:"qr_url"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &ls)
	if ls.Ticket == "" || ls.QRURL == "" {
		t.Fatalf("login start resp=%+v", ls)
	}

	// login/status: pending then success.
	callStatus := func() string {
		r := httptest.NewRecorder()
		a.routes().ServeHTTP(r, signedReq(t, http.MethodGet, "/admin/login/status?ticket="+ls.Ticket, testSecret, nil))
		var st struct{ Status string `json:"status"` }
		_ = json.Unmarshal(r.Body.Bytes(), &st)
		return st.Status
	}
	if s := callStatus(); s != weixinlink.LoginStatusPending {
		t.Fatalf("poll1=%s", s)
	}
	if s := callStatus(); s != weixinlink.LoginStatusSuccess {
		t.Fatalf("poll2=%s", s)
	}

	// success -> credentials persisted + polling started.
	if !a.isPolling() {
		t.Fatal("expected polling after login success")
	}
	if _, err := os.Stat(filepath.Join(a.credsDir, "creds.json")); err != nil {
		t.Fatalf("creds.json not persisted: %v", err)
	}

	// status endpoint reflects credentials + polling + account.
	rec2 := httptest.NewRecorder()
	a.routes().ServeHTTP(rec2, signedReq(t, http.MethodGet, "/admin/status", testSecret, nil))
	var st struct {
		HasCredentials bool   `json:"has_credentials"`
		Polling        bool   `json:"polling"`
		AccountID      string `json:"account_id"`
	}
	_ = json.Unmarshal(rec2.Body.Bytes(), &st)
	if !st.HasCredentials || !st.Polling || st.AccountID == "" {
		t.Fatalf("status=%+v", st)
	}

	// Inbound text message was forwarded to baize with account/peer/text.
	select {
	case <-gotCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for inbound forward")
	}
	mu.Lock()
	m := gotBody
	mu.Unlock()
	if m["event"] != "message" || m["text"] != "你好适配器" {
		t.Fatalf("forwarded body=%v", m)
	}
	if m["account"] != fake.AccountID {
		t.Fatalf("account=%v want %s", m["account"], fake.AccountID)
	}
	peer, _ := m["peer"].(map[string]any)
	if peer["id"] != "peer@im.wechat" || m["context_token"] != "ctx-1" {
		t.Fatalf("peer/ctx=%v", m)
	}

	// stop -> polling false; start -> true.
	a.routes().ServeHTTP(httptest.NewRecorder(), signedReq(t, http.MethodPost, "/admin/stop", testSecret, nil))
	if a.isPolling() {
		t.Fatal("still polling after stop")
	}
	a.routes().ServeHTTP(httptest.NewRecorder(), signedReq(t, http.MethodPost, "/admin/start", testSecret, nil))
	if !a.isPolling() {
		t.Fatal("not polling after start")
	}

	// logout -> credentials cleared + polling stopped + creds file removed.
	a.routes().ServeHTTP(httptest.NewRecorder(), signedReq(t, http.MethodPost, "/admin/logout", testSecret, nil))
	if a.isPolling() {
		t.Fatal("polling after logout")
	}
	if a.hasCredentials() {
		t.Fatal("credentials present after logout")
	}
	if _, err := os.Stat(filepath.Join(a.credsDir, "creds.json")); !os.IsNotExist(err) {
		t.Fatalf("creds.json should be removed, err=%v", err)
	}
}

func TestAdminStartWithoutCredentials(t *testing.T) {
	a := newTestAdapter(t, weixinlink.NewFake(), "")
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, signedReq(t, http.MethodPost, "/admin/start", testSecret, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("start without creds = %d, want 409", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "login") == false {
		t.Fatalf("body should mention login required: %s", rec.Body.String())
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./cmd/weixin-adapter/ -run 'TestAdmin' -v`
预期：编译失败 `a.routes` 未挂载 admin / `a.isPolling` 等未定义。

- [ ] **步骤 3：实现 `admin.go`**

```go
package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
	"github.com/rebornace/baize/internal/webhooksig"
)

const (
	adminBodyLimit = 1 << 16
	adminSkew      = 300 * time.Second
)

var errLoginRequired = errors.New("login required")

// adminGuard verifies the baize->adapter HMAC signature before dispatching an
// admin endpoint. GET requests carry an empty body (baize signs the empty
// body); POST bodies are read here for verification.
func (a *Adapter) adminGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, adminBodyLimit))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
			return
		}
		if err := webhooksig.Verify(a.secret, r.Header.Get("X-Baize-Channel-Timestamp"), body,
			r.Header.Get("X-Baize-Channel-Signature"), time.Now(), adminSkew); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
			return
		}
		next(w, r)
	}
}

func (a *Adapter) handleAdminLoginStart(w http.ResponseWriter, r *http.Request) {
	ticket, qrURL, err := a.ilink.GetQR(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ticket": ticket, "qr_url": qrURL})
}

func (a *Adapter) handleAdminLoginStatus(w http.ResponseWriter, r *http.Request) {
	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if ticket == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ticket required"})
		return
	}
	status, accountID, token, err := a.ilink.PollLogin(r.Context(), ticket)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if status == weixinlink.LoginStatusSuccess {
		a.setCredentials(accountID, token)
		if err := weixinlink.SaveCreds(a.credsDir, accountID, token); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save creds: " + err.Error()})
			return
		}
		if err := a.startPolling(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "start polling: " + err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (a *Adapter) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	a.stopPolling()
	a.mu.Lock()
	a.account = ""
	a.token = ""
	a.mu.Unlock()
	_ = weixinlink.ClearCreds(a.credsDir)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (a *Adapter) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	st := map[string]any{
		"has_credentials": a.token != "" && a.account != "",
		"polling":         a.polling,
		"account_id":      a.account,
	}
	a.mu.Unlock()
	writeJSON(w, http.StatusOK, st)
}

func (a *Adapter) handleAdminStart(w http.ResponseWriter, r *http.Request) {
	if err := a.startPolling(); err != nil {
		if errors.Is(err, errLoginRequired) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "login_required"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (a *Adapter) handleAdminStop(w http.ResponseWriter, r *http.Request) {
	a.stopPolling()
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
```

- [ ] **步骤 4：实现 `poller.go`（轮询生命周期 + 文本入站转发）**

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
	"github.com/rebornace/baize/internal/webhooksig"
)

const defaultEmptyPollWait = time.Second

// startPolling launches the iLink long-poll loop. It returns errLoginRequired
// when no credentials are present. Idempotent: a no-op when already polling.
func (a *Adapter) startPolling() error {
	a.mu.Lock()
	if a.polling {
		a.mu.Unlock()
		return nil
	}
	token := a.token
	if strings.TrimSpace(token) == "" || strings.TrimSpace(a.account) == "" {
		a.mu.Unlock()
		return errLoginRequired
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.polling = true
	a.mu.Unlock()
	go a.pollLoop(ctx, token)
	return nil
}

func (a *Adapter) pollLoop(ctx context.Context, token string) {
	wait := a.emptyWait
	if wait <= 0 {
		wait = defaultEmptyPollWait
	}
	cursor := ""
	for {
		if ctx.Err() != nil {
			return
		}
		updates, next, err := a.ilink.GetUpdates(ctx, token, cursor)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			continue
		}
		if next != "" {
			cursor = next
		}
		for _, u := range updates {
			if err := a.handleInboundUpdate(ctx, u); err != nil && ctx.Err() == nil {
				log.Printf("weixin-adapter: inbound update: %v", err)
			}
		}
		if len(updates) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
		}
	}
}

// handleInboundUpdate drops group chats and forwards a DM to baize. Media is
// attached in Task 8; this task forwards text/account/peer/context_token.
func (a *Adapter) handleInboundUpdate(ctx context.Context, u weixinlink.Update) error {
	peer := strings.TrimSpace(u.PeerID)
	if peer == "" || strings.Contains(peer, "@chatroom") {
		return nil
	}
	return a.forwardInbound(ctx, peer, u.Text, u.ContextToken, nil)
}

var inboundSeq uint64

// forwardInbound signs and POSTs one inbound message to baize's webhook.
func (a *Adapter) forwardInbound(ctx context.Context, peer, text, contextToken string, atts []map[string]any) error {
	seq := atomic.AddUint64(&inboundSeq, 1)
	payload := map[string]any{
		"event":           "message",
		"account":         a.currentAccount(),
		"peer":            map[string]any{"id": peer, "name": peer},
		"text":            text,
		"context_token":   contextToken,
		"idempotency_key": "wx-" + strconv.FormatUint(seq, 10),
	}
	if len(atts) > 0 {
		payload["attachments"] = atts
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baizeInboundURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Baize-Protocol", "v0")
	req.Header.Set("X-Baize-Channel-Timestamp", ts)
	req.Header.Set("X-Baize-Channel-Signature", webhooksig.Sign(a.secret, ts, body))

	hc := a.httpClient
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &httpStatusError{code: resp.StatusCode}
	}
	return nil
}

type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string { return "baize inbound http " + strconv.Itoa(e.code) }

func (a *Adapter) currentAccount() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.account
}

func (a *Adapter) isPolling() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.polling
}

func (a *Adapter) hasCredentials() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.token != "" && a.account != ""
}
```

- [ ] **步骤 5：修改 `adapter.go`（字段 + 挂载 admin 路由）**

把 `Adapter` 结构体字段块替换/扩展为（新增 `emptyWait`、`httpClient`）：

```go
type Adapter struct {
	ilink weixinlink.ILink

	baizeInboundURL string
	secret          string
	credsDir        string

	emptyWait  time.Duration
	httpClient *http.Client

	mu      sync.Mutex
	account string
	token   string
	polling bool
	cancel  func()
}
```

并把 `routes()` 替换为：

```go
func (a *Adapter) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleHealthz)
	mux.HandleFunc("POST /admin/login/start", a.adminGuard(a.handleAdminLoginStart))
	mux.HandleFunc("GET /admin/login/status", a.adminGuard(a.handleAdminLoginStatus))
	mux.HandleFunc("POST /admin/logout", a.adminGuard(a.handleAdminLogout))
	mux.HandleFunc("GET /admin/status", a.adminGuard(a.handleAdminStatus))
	mux.HandleFunc("POST /admin/start", a.adminGuard(a.handleAdminStart))
	mux.HandleFunc("POST /admin/stop", a.adminGuard(a.handleAdminStop))
	return mux
}
```

`adapter.go` import 块加入 `"time"`（`net/http`、`sync`、`encoding/json`、weixinlink 已有）。

- [ ] **步骤 6：运行测试验证通过**

运行：`go build ./cmd/weixin-adapter/... && go test ./cmd/weixin-adapter/ -v`
预期：PASS（healthz + admin 全流程 + 无凭据 409）。`go build ./...` 全绿。

- [ ] **步骤 7：Commit**

```bash
git add cmd/weixin-adapter/
git commit -m "feat(weixin-adapter): admin plane (login/logout/status/start/stop) + polling + text inbound"
```




## 任务 7：适配器 `/outbound`（文本 + 真发媒体）与入站媒体附件上报

**文件：**
- 创建：`cmd/weixin-adapter/outbound.go`（验签 + 解析出站 + 文本 SendMessage / 图片 SendImage / 文件 SendFile）
- 修改：`cmd/weixin-adapter/poller.go`（`handleInboundUpdate` 下载/解密媒体并装入 attachments）
- 修改：`cmd/weixin-adapter/adapter.go`（`routes()` 挂载 `POST /outbound`）
- 测试：`cmd/weixin-adapter/outbound_test.go`、扩充 `admin_test.go`（入站媒体）

- [ ] **步骤 1：编写失败的测试**

创建 `cmd/weixin-adapter/outbound_test.go`：

```go
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
)

// postOutbound signs and posts an outbound payload to the adapter.
func postOutbound(t *testing.T, a *Adapter, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(payload)
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, signedReq(t, http.MethodPost, "/outbound", testSecret, body))
	return rec
}

func TestOutboundRejectsBadSignature(t *testing.T) {
	a := newTestAdapter(t, weixinlink.NewFake(), "")
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/outbound", bytes.NewReader([]byte("{}"))))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned outbound = %d", rec.Code)
	}
}

func TestOutboundTextSent(t *testing.T) {
	fake := weixinlink.NewFake()
	a := newTestAdapter(t, fake, "")
	a.setCredentials("bot@im.bot", "tok")
	rec := postOutbound(t, a, map[string]any{
		"kind": "assistant", "peer": map[string]any{"id": "peer@im.wechat"},
		"text": "【助手】你好", "context_token": "ctx-9",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("outbound = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(fake.Sent) != 1 || fake.Sent[0].Text != "【助手】你好" || fake.Sent[0].ContextToken != "ctx-9" {
		t.Fatalf("Sent=%+v", fake.Sent)
	}
}

func TestOutboundImageAndFile(t *testing.T) {
	fake := weixinlink.NewFake()
	a := newTestAdapter(t, fake, "")
	a.setCredentials("bot@im.bot", "tok")

	png := []byte("PNGBYTES")
	pdf := []byte("PDFBYTES")
	rec := postOutbound(t, a, map[string]any{
		"kind": "assistant", "peer": map[string]any{"id": "peer@im.wechat"},
		"text": "【助手】看图",
		"media": []map[string]any{
			{"name": "a.png", "mime": "image/png", "content_base64": base64.StdEncoding.EncodeToString(png)},
			{"name": "b.pdf", "mime": "application/pdf", "content_base64": base64.StdEncoding.EncodeToString(pdf)},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("outbound = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(fake.SentImages) != 1 || string(fake.SentImages[0].Data) != "PNGBYTES" || fake.SentImages[0].FileName != "a.png" {
		t.Fatalf("images=%+v", fake.SentImages)
	}
	if len(fake.SentFiles) != 1 || string(fake.SentFiles[0].Data) != "PDFBYTES" || fake.SentFiles[0].FileName != "b.pdf" {
		t.Fatalf("files=%+v", fake.SentFiles)
	}
}

func TestOutboundWithoutCredentials(t *testing.T) {
	a := newTestAdapter(t, weixinlink.NewFake(), "")
	rec := postOutbound(t, a, map[string]any{"peer": map[string]any{"id": "p"}, "text": "x"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("outbound without creds = %d want 409", rec.Code)
	}
}
```

入站媒体：在 `admin_test.go` 追加（Fake 的 `MediaBytes` 会被 `DownloadMediaDecrypted` 返回）：

```go
func TestInboundMediaForwardedAsAttachment(t *testing.T) {
	var gotBody map[string]any
	gotCh := make(chan struct{}, 1)
	baize := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		select {
		case gotCh <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(baize.Close)

	fake := weixinlink.NewFake()
	fake.MediaBytes = []byte("IMAGE-BYTES")
	fake.Updates = []weixinlink.Update{{
		PeerID: "peer@im.wechat",
		Text:   "看图",
		Media:  []weixinlink.MediaRef{{FileName: "pic.png", MIME: "image/png"}},
	}}
	a := newTestAdapter(t, fake, baize.URL)
	a.setCredentials(fake.AccountID, fake.Token)
	if err := a.startPolling(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-gotCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	atts, _ := gotBody["attachments"].([]any)
	if len(atts) != 1 {
		t.Fatalf("attachments=%v", gotBody["attachments"])
	}
	att := atts[0].(map[string]any)
	if att["name"] != "pic.png" {
		t.Fatalf("att name=%v", att["name"])
	}
	dec, err := base64.StdEncoding.DecodeString(att["content_base64"].(string))
	if err != nil || string(dec) != "IMAGE-BYTES" {
		t.Fatalf("att bytes=%v err=%v", att["content_base64"], err)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./cmd/weixin-adapter/ -run 'TestOutbound|TestInboundMedia' -v`
预期：编译失败（`/outbound` 未挂载、`handleOutbound` 未定义）。

- [ ] **步骤 3：实现 `outbound.go`**

```go
package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
	"github.com/rebornace/baize/internal/webhooksig"
)

const outboundBodyLimit = 10 << 20

type outboundPayload struct {
	Kind         string `json:"kind"`
	Peer         struct{ ID string `json:"id"` } `json:"peer"`
	Text         string `json:"text"`
	ContextToken string `json:"context_token"`
	Media        []struct {
		Name          string `json:"name"`
		MIME          string `json:"mime"`
		ContentBase64 string `json:"content_base64"`
	} `json:"media"`
}

// handleOutbound receives baize -> adapter messages. Text is sent via
// SendMessage; image/* media via SendImage, other files via SendFile (real
// CDN upload). Media failures are logged but do not fail the request (text is
// already delivered; outbound must not interrupt the run).
func (a *Adapter) handleOutbound(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, outboundBodyLimit))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	if err := webhooksig.Verify(a.secret, r.Header.Get("X-Baize-Channel-Timestamp"), body,
		r.Header.Get("X-Baize-Channel-Signature"), time.Now(), adminSkew); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		return
	}
	var p outboundPayload
	if err := json.Unmarshal(body, &p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	peer := strings.TrimSpace(p.Peer.ID)
	if peer == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "peer.id required"})
		return
	}
	token := a.currentToken()
	if token == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "login_required"})
		return
	}

	if strings.TrimSpace(p.Text) != "" {
		if err := a.ilink.SendMessage(r.Context(), token, weixinlink.OutboundMessage{
			ToUserID:     peer,
			Text:         p.Text,
			ContextToken: p.ContextToken,
		}); err != nil {
			log.Printf("weixin-adapter: outbound text: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "send failed"})
			return
		}
	}
	for _, m := range p.Media {
		data, derr := base64.StdEncoding.DecodeString(m.ContentBase64)
		if derr != nil {
			log.Printf("weixin-adapter: outbound media %s bad base64: %v", m.Name, derr)
			continue
		}
		var serr error
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.MIME)), "image/") {
			serr = a.ilink.SendImage(r.Context(), token, peer, m.Name, m.MIME, data, p.ContextToken)
		} else {
			serr = a.ilink.SendFile(r.Context(), token, peer, m.Name, m.MIME, data, p.ContextToken)
		}
		if serr != nil {
			log.Printf("weixin-adapter: outbound media %s: %v", m.Name, serr)
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **步骤 4：轮询下载/解密媒体 + `currentToken`**

在 `poller.go` 增加 `encoding/base64` import，把 `handleInboundUpdate` 替换为下载媒体版本，并新增 `currentToken`：

```go
func (a *Adapter) handleInboundUpdate(ctx context.Context, u weixinlink.Update) error {
	peer := strings.TrimSpace(u.PeerID)
	if peer == "" || strings.Contains(peer, "@chatroom") {
		return nil
	}
	atts := []map[string]any{}
	if md, ok := a.ilink.(weixinlink.MediaDownloader); ok {
		for _, m := range u.Media {
			data, _, err := md.DownloadMediaDecrypted(ctx, a.currentToken(), m)
			if err != nil {
				log.Printf("weixin-adapter: download media %s: %v", m.FileName, err)
				continue
			}
			if len(data) == 0 {
				// Key present but undecryptable: skip bytes (do not forward
				// ciphertext garbage); the message text still goes through.
				log.Printf("weixin-adapter: media %s undecryptable; skipped", m.FileName)
				continue
			}
			name := strings.TrimSpace(m.FileName)
			if name == "" {
				name = "media.bin"
			}
			mime := strings.TrimSpace(m.MIME)
			if mime == "" {
				mime = "application/octet-stream"
			}
			atts = append(atts, map[string]any{
				"name":           name,
				"mime":           mime,
				"content_base64": base64.StdEncoding.EncodeToString(data),
			})
		}
	}
	return a.forwardInbound(ctx, peer, u.Text, u.ContextToken, atts)
}

func (a *Adapter) currentToken() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.token
}
```

- [ ] **步骤 5：挂载 `/outbound`**

在 `adapter.go` 的 `routes()` 中加一行（`/admin/*` 之后）：

```go
	mux.HandleFunc("POST /outbound", a.handleOutbound)
```

- [ ] **步骤 6：运行测试验证通过**

运行：`go build ./cmd/weixin-adapter/... && go test ./cmd/weixin-adapter/ -v`
预期：PASS（文本出站、图片/文件走 SendImage/SendFile、无凭据 409、入站媒体附件）。`go build ./...` 全绿。

- [ ] **步骤 7：Commit**

```bash
git add cmd/weixin-adapter/
git commit -m "feat(weixin-adapter): /outbound text+image/file send, inbound media attachments"
```




## 任务 8：webhook 配置扩展（admin/适配器/secret 生成）+ 动态 account

**文件：**
- 修改：`internal/channel/webhook/config.go`（新字段 + secret 在 autostart 时可空）
- 修改：`internal/channel/webhook/channel.go`（`activeAccount` 缓存 + 出站 account 解析 + secret 生成）
- 修改：`internal/channel/webhook/inbound.go`（account 取 `msg.Account` 优先并写缓存）
- 测试：`internal/channel/webhook/config_test.go`、`channel_test.go`、`inbound_test.go`

- [ ] **步骤 1：编写失败的测试**

`config_test.go` 追加：

```go
func TestParseConfigAutostartAllowsEmptySecret(t *testing.T) {
	c, err := parseConfig("weixin", map[string]string{
		"source":           "weixin",
		"outbound_url":     "http://127.0.0.1:8090/outbound",
		"assignee":         "channel:weixin",
		"adapter_autostart": "true",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !c.AdapterAutostart {
		t.Fatal("AdapterAutostart not parsed")
	}
	if c.Secret == "" {
		t.Fatal("secret should be generated for autostart")
	}
	if c.OutboundSecret != c.Secret {
		t.Fatal("outbound secret should fall back to generated secret")
	}
}

func TestParseConfigAdminAndAdapterFields(t *testing.T) {
	c, err := parseConfig("bot", map[string]string{
		"secret":          "s",
		"outbound_url":    "http://h/o",
		"assignee":        "a",
		"admin_url":       "http://127.0.0.1:8090",
		"adapter_command": "weixin-adapter",
		"adapter_args":    "-addr=127.0.0.1:8090,-creds=./data/channels/weixin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.AdminURL != "http://127.0.0.1:8090" || c.AdapterCommand != "weixin-adapter" {
		t.Fatalf("admin/cmd = %q %q", c.AdminURL, c.AdapterCommand)
	}
	if len(c.AdapterArgs) != 2 || c.AdapterArgs[0] != "-addr=127.0.0.1:8090" {
		t.Fatalf("adapter args = %v", c.AdapterArgs)
	}
}

func TestParseConfigSecretRequiredWithoutAutostart(t *testing.T) {
	if _, err := parseConfig("x", map[string]string{"outbound_url": "http://h/o", "assignee": "a"}); err == nil {
		t.Fatal("expected error for missing secret when not autostart")
	}
}
```

`channel_test.go` 追加（动态 account）：

```go
func TestDynamicAccountInboundThenOutbound(t *testing.T) {
	// Inbound carries the post-login account; outbound must use it (not the
	// static config account/name).
	got := make(chan OutboundMessage, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m OutboundMessage
		_ = json.NewDecoder(r.Body).Decode(&m)
		got <- m
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c, err := openFromConfig("weixin", map[string]string{
		"source":       "weixin",
		"secret":       "s",
		"outbound_url": srv.URL,
		"assignee":     "channel:weixin",
		// no static account: defaults to name "weixin" until first inbound.
	})
	if err != nil {
		t.Fatal(err)
	}
	// White-box: attach a runtime the way Bootstrap would, then learn the
	// post-login account from a (simulated) inbound.
	c.rt = &channel.Runtime{Assignee: "channel:weixin", DefaultAgentID: "ag", Source: "weixin"}
	c.setActiveAccount("bot@im.bot")

	if err := c.SendText(context.Background(), "peer@im.wechat", "hi", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	m := <-got
	if m.Account != "bot@im.bot" {
		t.Fatalf("outbound account = %q want bot@im.bot", m.Account)
	}
	if m.ConversationID != "weixin:bot@im.bot:peer@im.wechat" {
		t.Fatalf("conv id = %q", m.ConversationID)
	}
}
```

> 该测试文件已 import `context`、`net/http/httptest`、`encoding/json`、`github.com/rebornace/baize/internal/channel`（参照文件既有 import；缺则补）。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/channel/webhook/ -run 'TestParseConfig|TestDynamicAccount' -v`
预期：编译失败（新字段/方法未定义）。

- [ ] **步骤 3：扩展 `instanceConfig` 与 `parseConfig`**

`config.go` 的 `instanceConfig` 追加字段：

```go
	AdminURL         string
	AdapterAutostart bool
	AdapterCommand   string
	AdapterArgs      []string
	AdapterCredsDir  string
```

`parseConfig` 中（在 secret 校验之前）解析：

```go
	c.AdminURL = get("admin_url")
	c.AdapterCommand = get("adapter_command")
	c.AdapterCredsDir = get("adapter_creds_dir")
	if v := get("adapter_autostart"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return c, fmt.Errorf("webhook: invalid adapter_autostart %q: %w", v, err)
		}
		c.AdapterAutostart = b
	}
	if raw := get("adapter_args"); raw != "" {
		for _, a := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' }) {
			if a = strings.TrimSpace(a); a != "" {
				c.AdapterArgs = append(c.AdapterArgs, a)
			}
		}
	}
```

把 secret 必填校验改为（autostart 时允许空，由 `openFromConfig` 生成）：

```go
	if c.Secret == "" && !c.AdapterAutostart {
		return c, fmt.Errorf("webhook: missing required config %q for instance %q (or set adapter_autostart=true to generate one)", "secret", name)
	}
```

`openFromConfig` 在 `parseConfig` 后、构造 Channel 前生成 secret：

```go
	if cfg.Secret == "" && cfg.AdapterAutostart {
		secret, err := generateSecret()
		if err != nil {
			return nil, fmt.Errorf("webhook: generate adapter secret: %w", err)
		}
		cfg.Secret = secret
		if cfg.OutboundSecret == "" {
			cfg.OutboundSecret = secret
		}
	}
```

新增 `internal/channel/webhook/secret.go`：

```go
package webhook

import (
	"crypto/rand"
	"encoding/hex"
)

// generateSecret returns a 32-byte random HMAC secret, hex-encoded, used for
// autostart adapters where baize and the child process share an ephemeral key.
func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
```

- [ ] **步骤 4：动态 account（`channel.go` + `inbound.go`）**

`channel.go`：`Channel` 结构体加 `accountMu sync.RWMutex; activeAccount string`，新增：

```go
func (c *Channel) setActiveAccount(acct string) {
	acct = strings.TrimSpace(acct)
	if acct == "" {
		return
	}
	c.accountMu.Lock()
	c.activeAccount = acct
	c.accountMu.Unlock()
}

func (c *Channel) activeAccountOr(extras map[string]string) string {
	if extras != nil {
		if a := strings.TrimSpace(extras["account"]); a != "" {
			return a
		}
	}
	c.accountMu.RLock()
	a := c.activeAccount
	c.accountMu.RUnlock()
	if a != "" {
		return a
	}
	return c.cfg.Account
}
```

`SendText`/`SendMedia` 中把 `c.cfg.Account`（两处：`ConversationID` 与 `Account`）替换为 `acct := c.activeAccountOr(extras)`，即：

```go
	acct := c.activeAccountOr(extras)
	msg := OutboundMessage{
		...
		ConversationID: channel.ConvID(c.cfg.Source, acct, peerID),
		Account:        acct,
		...
	}
```

`inbound.go`：在解析 `peer` 之后、构造 extras 处，改为动态 account：

```go
	acct := strings.TrimSpace(msg.Account)
	if acct == "" {
		acct = c.cfg.Account
	}
	c.setActiveAccount(acct)
	...
	extras := map[string]string{"account": acct}
```

（白名单仍用 `c.cfg.Allowlist`，任务 9 改为热更新字段。）

- [ ] **步骤 5：运行测试验证通过**

运行：`go test ./internal/channel/webhook/ -v`
预期：PASS（含 2A 既有测试 + 新配置/动态 account 测试）。

- [ ] **步骤 6：Commit**

```bash
git add internal/channel/webhook/
git commit -m "feat(webhook): adapter/admin config, autostart secret generation, dynamic account"
```




## 任务 9：通用渠道设置持久化 + 热更新 + `channel.ManagedChannel`

**文件：**
- 创建：`internal/channel/manager.go`（`ManagedChannel` 接口 + `ChannelSettings`/`AdapterStatus`/`LoginTicket` DTO）
- 创建：`internal/channel/webhook/settings.go`（设置文件读写 + 热应用）
- 修改：`internal/channel/bootstrap.go`（`BuildDeps` 加 `DataDir`）
- 修改：`internal/bootstrap/bootstrap.go`（装配 deps 传 `DataDir`）
- 修改：`internal/channel/webhook/channel.go`（settings 状态、allowlist 热更新、实现 `ManagedChannel`、`adminClient` 接缝）
- 修改：`internal/channel/webhook/inbound.go`（白名单读热更新字段）
- 测试：`internal/channel/webhook/settings_test.go`、`channel_test.go`

- [ ] **步骤 1：编写失败的测试**

`settings_test.go`：

```go
package webhook

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/channel"
)

func TestSettingsDefaultsAndHotApply(t *testing.T) {
	dir := t.TempDir()
	c, err := openFromConfig("weixin", map[string]string{
		"source": "weixin", "secret": "s", "outbound_url": "http://h/o",
		"assignee": "channel:weixin", "agent_id": "ag-cfg", "admin_url": "http://127.0.0.1:9",
	})
	if err != nil {
		t.Fatal(err)
	}
	c.settingsDir = dir // white-box: persist under temp dir
	rt := &channel.Runtime{Source: "weixin"}
	c.rt = rt
	if err := c.loadSettings(); err != nil {
		t.Fatal(err)
	}
	// Defaults: enabled, assignee/agent from config, allowlist open.
	s := c.GetSettings()
	if !s.Enabled || s.Assignee != "channel:weixin" || s.AgentID != "ag-cfg" {
		t.Fatalf("default settings=%+v", s)
	}
	if !c.peerAllowed("anyone@im.wechat") {
		t.Fatal("empty allowlist should allow all")
	}

	// Hot update: restrict allowlist, change assignee/agent, disable.
	st := c.UpdateSettings(channel.ChannelSettings{
		Assignee: "op:alice", AgentID: "ag-2",
		Allowlist: []string{"allowed@im.wechat"}, Enabled: false,
	})
	if st.Running || st.Reason != "" {
		t.Fatalf("disabled -> running=false reason empty, got %+v", st)
	}
	if rt.Assignee != "op:alice" || rt.DefaultAgentID != "ag-2" {
		t.Fatalf("runtime not hot-applied: %+v", rt)
	}
	if c.peerAllowed("allowed@im.wechat") != true || c.peerAllowed("other@im.wechat") != false {
		t.Fatal("allowlist not hot-applied")
	}
	// Persisted to disk.
	if _, err := loadSettingsFile(dir); err != nil {
		t.Fatalf("settings not persisted: %v", err)
	}

	// Re-enable without credentials -> login_required (admin client is nil in
	// this test, so status falls back to enabled-only reporting).
	st2 := c.UpdateSettings(channel.ChannelSettings{
		Assignee: "op:alice", AgentID: "ag-2", Allowlist: []string{"allowed@im.wechat"}, Enabled: true,
	})
	_ = st2
}

func TestStatusReconcilesViaAdminClient(t *testing.T) {
	c, _ := openFromConfig("weixin", map[string]string{
		"source": "weixin", "secret": "s", "outbound_url": "http://h/o", "assignee": "a",
	})
	c.rt = &channel.Runtime{Source: "weixin"}
	_ = c.loadSettings()

	// No admin client: enabled -> reported running (cannot probe adapter).
	if st := c.Status(); !st.Running {
		t.Fatalf("no-admin enabled status=%+v", st)
	}

	// With a fake admin client: reconcile login_required / start_failed / ok.
	c.admin = &fakeAdmin{hasCreds: false, polling: false}
	if st := c.Status(); st.Running || st.Reason != "login_required" {
		t.Fatalf("no-creds status=%+v", st)
	}
	c.admin = &fakeAdmin{hasCreds: true, polling: false}
	if st := c.Status(); st.Running || st.Reason != "start_failed" {
		t.Fatalf("has-creds-not-polling status=%+v", st)
	}
	c.admin = &fakeAdmin{hasCreds: true, polling: true}
	if st := c.Status(); !st.Running || st.Reason != "" {
		t.Fatalf("polling status=%+v", st)
	}
	c.admin = &fakeAdmin{err: context.DeadlineExceeded}
	if st := c.Status(); st.Running || st.Reason != "start_failed" {
		t.Fatalf("unreachable status=%+v", st)
	}
}

type fakeAdmin struct {
	hasCreds, polling bool
	err               error
	started, stopped  bool
}

func (f *fakeAdmin) Status(context.Context) (bool, bool, error) { return f.hasCreds, f.polling, f.err }
func (f *fakeAdmin) Start(context.Context) error              { f.started = true; return f.err }
func (f *fakeAdmin) Stop(context.Context) error               { f.stopped = true; return f.err }
func (f *fakeAdmin) LoginStart(context.Context) (string, string, error) {
	return "t", "qr", f.err
}
func (f *fakeAdmin) LoginPoll(context.Context, string) (string, error) {
	return "success", f.err
}
func (f *fakeAdmin) Logout(context.Context) error { return f.err }
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/channel/webhook/ -run 'TestSettings|TestStatus' -v`
预期：编译失败（`ManagedChannel`/`loadSettings`/`peerAllowed`/`c.admin` 未定义）。

- [ ] **步骤 3：创建 `internal/channel/manager.go`**

```go
package channel

import "context"

// ChannelSettings are the hot-updatable per-instance settings exposed via the
// generic /v0/settings/channels/{name} management plane.
type ChannelSettings struct {
	Assignee  string   `json:"assignee"`
	AgentID   string   `json:"agent_id"`
	Allowlist []string `json:"allowlist"`
	Enabled   bool     `json:"enabled"`
}

// AdapterStatus is the reconciled running state returned alongside settings.
// Reason is "" when running or intentionally disabled; "login_required" when
// credentials are missing; "start_failed" when the adapter is not polling or
// unreachable.
type AdapterStatus struct {
	Running bool   `json:"running"`
	Reason  string `json:"reason,omitempty"`
}

// LoginTicket is the QR login start response.
type LoginTicket struct {
	Ticket string `json:"ticket"`
	QRURL  string `json:"qr_url"`
}

// ManagedChannel is implemented by channels with a settings + adapter
// management plane (the webhook channel). The api layer drives it generically
// so no channel-specific handlers are needed.
type ManagedChannel interface {
	GetSettings() ChannelSettings
	UpdateSettings(ChannelSettings) AdapterStatus
	Status() AdapterStatus
	LoginStart(ctx context.Context) (LoginTicket, error)
	LoginPoll(ctx context.Context, ticket string) (status string, err error)
	Logout(ctx context.Context) error
}
```

- [ ] **步骤 4：`BuildDeps` 加 `DataDir`**

`internal/channel/bootstrap.go` 的 `BuildDeps` 加字段：

```go
	// DataDir is the baize data directory for persisted per-channel state
	// (e.g. <dataDir>/channels/webhook/<name>/settings.json). Empty means
	// in-memory only (tests).
	DataDir string
```

`internal/bootstrap/bootstrap.go` 构造 `deps := channel.BuildDeps{...}` 处加一行 `DataDir: dataDir(d.cfg),`。

- [ ] **步骤 5：实现 `internal/channel/webhook/settings.go`**

```go
package webhook

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/rebornace/baize/internal/channel"
)

const settingsFileName = "settings.json"

// loadSettingsFile reads the persisted per-instance settings. A missing file
// returns the zero value (caller overlays config defaults).
func loadSettingsFile(dir string) (channel.ChannelSettings, error) {
	var out channel.ChannelSettings
	data, err := os.ReadFile(filepath.Join(dir, settingsFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func saveSettingsFile(dir string, s channel.ChannelSettings) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, settingsFileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// loadSettings overlays persisted settings over config baseline and applies
// them hot to the Runtime/allowlist. Safe to call when settingsDir is empty
// (tests): uses in-memory defaults only.
func (c *Channel) loadSettings() error {
	dir := strings.TrimSpace(c.settingsDir)
	st := channel.ChannelSettings{
		Assignee:  c.cfg.Assignee,
		AgentID:   c.cfg.AgentID,
		Enabled:   true,
		Allowlist: []string{},
	}
	if dir != "" {
		data, err := os.ReadFile(filepath.Join(dir, settingsFileName))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			var persisted channel.ChannelSettings
			if err := json.Unmarshal(data, &persisted); err != nil {
				return err
			}
			if strings.TrimSpace(persisted.Assignee) != "" {
				st.Assignee = persisted.Assignee
			}
			if strings.TrimSpace(persisted.AgentID) != "" {
				st.AgentID = persisted.AgentID
			}
			if persisted.Allowlist != nil {
				st.Allowlist = persisted.Allowlist
			}
			// Enabled defaults true; only an explicit persisted false disables.
			var raw map[string]any
			if json.Unmarshal(data, &raw) == nil {
				if v, ok := raw["enabled"].(bool); ok {
					st.Enabled = v
				}
			}
		}
	}
	c.applySettings(st)
	return nil
}

// applySettings updates the in-memory settings, Runtime assignee/agent, and the
// hot allowlist, and notifies the adapter start/stop.
func (c *Channel) applySettings(st channel.ChannelSettings) {
	c.settingsMu.Lock()
	c.settings = st
	c.allowlist = toSet(st.Allowlist)
	c.settingsMu.Unlock()
	if c.rt != nil {
		if id := strings.TrimSpace(st.Assignee); id != "" {
			c.rt.Assignee = id
		}
		if id := strings.TrimSpace(st.AgentID); id != "" {
			c.rt.DefaultAgentID = id
		}
	}
}

func toSet(ids []string) map[string]bool {
	set := map[string]bool{}
	for _, id := range ids {
		if v := strings.TrimSpace(id); v != "" {
			set[v] = true
		}
	}
	return set
}

// peerAllowed reports whether peer passes the hot allowlist. Empty = open.
func (c *Channel) peerAllowed(peer string) bool {
	c.settingsMu.RLock()
	defer c.settingsMu.RUnlock()
	if len(c.allowlist) == 0 {
		return true
	}
	return c.allowlist[peer]
}

// GetSettings returns a copy of the current settings.
func (c *Channel) GetSettings() channel.ChannelSettings {
	c.settingsMu.RLock()
	defer c.settingsMu.RUnlock()
	out := c.settings
	if out.Allowlist == nil {
		out.Allowlist = []string{}
	}
	return out
}

// UpdateSettings persists (when a dir is set), applies hot, reconciles the
// adapter enabled state, and returns the reconciled status.
func (c *Channel) UpdateSettings(st channel.ChannelSettings) channel.AdapterStatus {
	if st.Allowlist == nil {
		st.Allowlist = []string{}
	}
	c.applySettings(st)
	if dir := strings.TrimSpace(c.settingsDir); dir != "" {
		if err := saveSettingsFile(dir, st); err != nil {
			log.Printf("webhook %s: save settings: %v", c.cfg.Name, err)
		}
	}
	c.reconcileEnabled(st.Enabled)
	return c.Status()
}

// Status reconciles running/reason from local enabled state and the adapter.
func (c *Channel) Status() channel.AdapterStatus {
	c.settingsMu.RLock()
	enabled := c.settings.Enabled
	c.settingsMu.RUnlock()
	if !enabled {
		return channel.AdapterStatus{Running: false, Reason: ""}
	}
	if c.admin == nil {
		// No adapter management plane: nothing to probe; report running when
		// enabled (third-party adapters manage their own lifecycle).
		return channel.AdapterStatus{Running: true}
	}
	hasCreds, polling, err := c.admin.Status(c.bgCtx())
	if err != nil {
		log.Printf("webhook %s: adapter status: %v", c.cfg.Name, err)
		return channel.AdapterStatus{Running: false, Reason: "start_failed"}
	}
	if !hasCreds {
		return channel.AdapterStatus{Running: false, Reason: "login_required"}
	}
	if !polling {
		return channel.AdapterStatus{Running: false, Reason: "start_failed"}
	}
	return channel.AdapterStatus{Running: true}
}

// reconcileEnabled starts/stops the adapter polling to match enabled.
func (c *Channel) reconcileEnabled(enabled bool) {
	if c.admin == nil {
		return
	}
	ctx := c.bgCtx()
	if enabled {
		if err := c.admin.Start(ctx); err != nil {
			log.Printf("webhook %s: adapter start: %v", c.cfg.Name, err)
		}
	} else {
		if err := c.admin.Stop(ctx); err != nil {
			log.Printf("webhook %s: adapter stop: %v", c.cfg.Name, err)
		}
	}
}
```

新增一个极小日志辅助（避免在多处直接 import log；也可直接用 `log.Printf`）。在 `settings.go` 顶部把 `log` 加入 import 块，日志直接用 `log.Printf(...)`（上文各 `logPrintf` 调用点即 `log.Printf`）。

- [ ] **步骤 6：`channel.go` 加字段与方法 + `inbound.go` 用热白名单**

先创建 `internal/channel/webhook/admin.go` 定义管理面接缝（HTTP 实现在任务 10）：

```go
package webhook

import "context"

// adminClient talks to the out-of-process adapter's management plane
// (/admin/*). The HTTP implementation is added in a later task; tests inject
// fakes. All methods return an error when the adapter is unreachable.
type adminClient interface {
	// Status returns hasCredentials and polling from the adapter /admin/status.
	Status(ctx context.Context) (hasCreds bool, polling bool, err error)
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	LoginStart(ctx context.Context) (ticket, qrURL string, err error)
	// LoginPoll polls QR login; status is "pending"|"success"|"expired".
	LoginPoll(ctx context.Context, ticket string) (status string, err error)
	Logout(ctx context.Context) error
}
```

`Channel` 结构体追加：

```go
	settingsDir string
	settings    channel.ChannelSettings
	settingsMu  sync.RWMutex
	allowlist   map[string]bool
	admin       adminClient
```

（`sync` 已在 `inbound.go` 用到；`channel.go` 需 import `"sync"`、`channel` 已有。）新增 `bgCtx`：

```go
func (c *Channel) bgCtx() context.Context { return context.Background() }
```

`Bootstrap` 中：解析 settings 目录并 `loadSettings`——在 `c.rt = rt` 之后加：

```go
	if dir := strings.TrimSpace(deps.DataDir); dir != "" {
		c.settingsDir = filepath.Join(dir, "channels", "webhook", c.cfg.Name)
	}
	if err := c.loadSettings(); err != nil {
		return nil, "", false, fmt.Errorf("webhook: load settings: %w", err)
	}
	c.reconcileEnabled(c.GetSettings().Enabled)
```

（`Bootstrap` 需 import `"path/filepath"`；`start` 返回值：enabled 且有凭据才启动轮询——但 webhook 自身 Start 是 no-op，适配器轮询由 supervisor/admin 负责，故 `Bootstrap` 仍返回 `start=true` 无害；实际适配器启停在任务 10/11 supervisor 接线。）

`inbound.go` 白名单判断由 `len(c.cfg.Allowlist) > 0 && !c.cfg.Allowlist[peer]` 改为：

```go
	if !c.peerAllowed(peer) {
		log.Printf("webhook %s: peer %q not in allowlist; ignored", c.cfg.Name, peer)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
```

- [ ] **步骤 7：运行测试验证通过**

运行：`go test ./internal/channel/webhook/ ./internal/channel/ ./internal/bootstrap/ -v`
预期：PASS。`go build ./...` 全绿。

- [ ] **步骤 8：Commit**

```bash
git add internal/channel/ internal/bootstrap/
git commit -m "feat(webhook): generic per-instance settings persistence + hot apply + ManagedChannel"
```




## 任务 10：管理面 HTTP 客户端 + 跨平台子进程托管 + 接线

**文件：**
- 修改：`internal/channel/webhook/admin.go`（追加 `httpAdminClient` 实现）
- 创建：`internal/channel/webhook/supervisor.go`（跨平台 `os/exec` 托管）
- 修改：`internal/channel/bootstrap.go`（`BuildDeps` 加 `SelfBaseURL`）
- 修改：`internal/bootstrap/bootstrap.go`（传 `SelfBaseURL`）
- 修改：`internal/channel/webhook/config.go`（`AdapterBaizeURL` 字段）
- 修改：`internal/channel/webhook/channel.go`（`Bootstrap` 建 admin client；`Start/Stop` 托管子进程）
- 测试：`internal/channel/webhook/admin_test.go`、`supervisor_test.go`

- [ ] **步骤 1：编写失败的测试**

`admin_test.go`（httptest 假适配器管理面，断言 HMAC 签名与转发）：

```go
package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPAdminClientStatusAndActions(t *testing.T) {
	var gotSig, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.Path
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-Baize-Channel-Signature") == "" {
			t.Errorf("missing signature on %s", r.URL.Path)
		}
		gotSig = r.Header.Get("X-Baize-Channel-Signature")
		switch r.URL.Path {
		case "/admin/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"has_credentials": true, "polling": true, "account_id": "bot@im.bot"})
		case "/admin/login/start":
			_ = json.NewEncoder(w).Encode(map[string]string{"ticket": "tk", "qr_url": "qr"})
		case "/admin/login/status":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		default:
			w.WriteHeader(http.StatusOK)
		}
		_ = body
	}))
	t.Cleanup(srv.Close)

	ac := newHTTPAdminClient(srv.URL, "secret")
	hasCreds, polling, err := ac.Status(context.Background())
	if err != nil || !hasCreds || !polling {
		t.Fatalf("status: %v %v %v", hasCreds, polling, err)
	}
	if !strings.HasPrefix(gotPath, "GET /admin/status") || gotSig == "" {
		t.Fatalf("status call path=%q sig=%q", gotPath, gotSig)
	}
	tk, qr, err := ac.LoginStart(context.Background())
	if err != nil || tk != "tk" || qr != "qr" {
		t.Fatalf("login start: %q %q %v", tk, qr, err)
	}
	st, err := ac.LoginPoll(context.Background(), "tk")
	if err != nil || st != "success" {
		t.Fatalf("login poll: %q %v", st, err)
	}
	if err := ac.Logout(context.Background()); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if err := ac.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
}

func TestHTTPAdminClientUnreachable(t *testing.T) {
	ac := newHTTPAdminClient("http://127.0.0.1:1", "s") // closed port
	if _, _, err := ac.Status(context.Background()); err == nil {
		t.Fatal("expected error for unreachable adapter")
	}
}
```

`supervisor_test.go`（用一个 test-fixture 子程序：`go run` 一个最小 healthz HTTP server）：

```go
package webhook

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeAdapterSource is a tiny HTTP server used as a supervised child process.
const fakeAdapterSource = `package main
import ("net/http"; "os")
func main() {
	addr := os.Getenv("FAKE_ADDR")
	if addr == "" { addr = "127.0.0.1:0" }
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request){ w.WriteHeader(200) })
	go http.ListenAndServe(addr, nil)
	select {}
}
`

func TestSupervisorStartsAndStopsChild(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte(fakeAdapterSource), 0o600); err != nil {
		t.Fatal(err)
	}
	// Pick a fixed loopback port for the child healthz.
	addr := "127.0.0.1:18099"
	sup := &supervisor{
		command:    "go",
		args:       []string{"run", src},
		healthzURL: "http://" + addr + "/healthz",
		env:        []string{"FAKE_ADDR=" + addr},
		timeout:    30 * time.Second,
	}
	ctx := context.Background()
	if err := sup.start(ctx); err != nil {
		t.Fatalf("supervisor start: %v", err)
	}
	if !sup.running() {
		t.Fatal("supervisor not running after start")
	}
	if err := sup.stop(ctx); err != nil {
		t.Fatalf("supervisor stop: %v", err)
	}
	if sup.running() {
		t.Fatal("supervisor still running after stop")
	}
}

func TestResolveAdapterCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(resolveExeName("weixin-adapter"), ".exe") {
			t.Fatal("windows should append .exe")
		}
	} else {
		if resolveExeName("weixin-adapter") != "weixin-adapter" {
			t.Fatal("non-windows should not append suffix")
		}
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/channel/webhook/ -run 'TestHTTPAdminClient|TestSupervisor|TestResolveAdapterCommand' -v`
预期：编译失败（`newHTTPAdminClient`/`supervisor`/`resolveExeName` 未定义）。

- [ ] **步骤 3：实现 `httpAdminClient`（admin.go 追加）**

```go
import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/rebornace/baize/internal/webhooksig"
)

type httpAdminClient struct {
	baseURL string
	secret  string
	hc      *http.Client
}

func newHTTPAdminClient(baseURL, secret string) *httpAdminClient {
	return &httpAdminClient{baseURL: strings.TrimRight(baseURL, "/"), secret: secret, hc: &http.Client{Timeout: 10 * time.Second}}
}

func (c *httpAdminClient) do(ctx context.Context, method, path string, query url.Values, out any) error {
	full := c.baseURL + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	// GET requests carry an empty body; sign that empty body so the adapter's
	// adminGuard (which reads the body) verifies the same bytes.
	var body []byte
	req, err := http.NewRequestWithContext(ctx, method, full, bytes.NewReader(body))
	if err != nil {
		return err
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("X-Baize-Protocol", ProtocolVersion)
	req.Header.Set("X-Baize-Channel-Timestamp", ts)
	req.Header.Set("X-Baize-Channel-Signature", webhooksig.Sign(c.secret, ts, body))
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("adapter %s: http %d: %s", path, resp.StatusCode, string(data))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("adapter %s: decode: %w", path, err)
		}
	}
	return nil
}

func (c *httpAdminClient) Status(ctx context.Context) (bool, bool, error) {
	var st struct {
		HasCredentials bool `json:"has_credentials"`
		Polling        bool `json:"polling"`
	}
	if err := c.do(ctx, http.MethodGet, "/admin/status", nil, &st); err != nil {
		return false, false, err
	}
	return st.HasCredentials, st.Polling, nil
}
func (c *httpAdminClient) Start(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/admin/start", nil, nil)
}
func (c *httpAdminClient) Stop(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/admin/stop", nil, nil)
}
func (c *httpAdminClient) Logout(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/admin/logout", nil, nil)
}
func (c *httpAdminClient) LoginStart(ctx context.Context) (string, string, error) {
	var out struct {
		Ticket string `json:"ticket"`
		QRURL  string `json:"qr_url"`
	}
	if err := c.do(ctx, http.MethodPost, "/admin/login/start", nil, &out); err != nil {
		return "", "", err
	}
	return out.Ticket, out.QRURL, nil
}
func (c *httpAdminClient) LoginPoll(ctx context.Context, ticket string) (string, error) {
	var out struct {
		Status string `json:"status"`
	}
	q := url.Values{}
	q.Set("ticket", ticket)
	if err := c.do(ctx, http.MethodGet, "/admin/login/status", q, &out); err != nil {
		return "", err
	}
	return out.Status, nil
}

var _ adminClient = (*httpAdminClient)(nil)
```

> admin.go 顶部已有 `package webhook` 与 `import "context"`；把新增 import 合并进同一 import 块（含 `strings`）。

- [ ] **步骤 4：实现 `supervisor.go`**

```go
package webhook

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// supervisor manages an adapter child process for autostart instances. It is
// cross-platform: it execs the command directly (no shell), polls /healthz
// until ready, and kills the process on stop (TerminateProcess on Windows,
// SIGKILL-equivalent elsewhere).
type supervisor struct {
	command    string
	args       []string
	env        []string
	healthzURL string
	timeout    time.Duration

	cmd *exec.Cmd
}

func (s *supervisor) start(ctx context.Context) error {
	if s.command == "" {
		return fmt.Errorf("webhook supervisor: empty adapter command")
	}
	resolved := resolveAdapterPath(s.command)
	cmd := exec.CommandContext(ctx, resolved, s.args...)
	cmd.Env = append(os.Environ(), s.env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("webhook supervisor: start %q: %w", s.command, err)
	}
	s.cmd = cmd
	timeout := s.timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if err := waitHealthz(ctx, s.healthzURL, timeout); err != nil {
		_ = s.kill()
		return fmt.Errorf("webhook supervisor: adapter never became healthy: %w", err)
	}
	return nil
}

func (s *supervisor) stop(ctx context.Context) error {
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	return s.kill()
}

func (s *supervisor) kill() error {
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	_ = s.cmd.Process.Kill() // cross-platform: Windows TerminateProcess / POSIX SIGKILL
	_ = s.cmd.Wait()
	s.cmd = nil
	return nil
}

func (s *supervisor) running() bool {
	return s.cmd != nil && s.cmd.ProcessState == nil
}

func waitHealthz(ctx context.Context, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("healthz timeout at %s", url)
}

// resolveExeName appends .exe on Windows.
func resolveExeName(name string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		return name + ".exe"
	}
	return name
}

// resolveAdapterPath finds the adapter executable via PATH (LookPath), then a
// ./bin/<name> fallback. The bare name has the platform exe suffix applied.
func resolveAdapterPath(command string) string {
	named := resolveExeName(command)
	if p, err := exec.LookPath(command); err == nil {
		return p
	}
	if p, err := exec.LookPath(named); err == nil {
		return p
	}
	// ./bin fallback (build artifacts land in bin/).
	bin := "bin" + string(os.PathSeparator) + named
	if p, err := exec.LookPath(bin); err == nil {
		return p
	}
	return named // let exec report the failure with the resolved name
}
```

- [ ] **步骤 5：`config.go` 加 `AdapterBaizeURL`；`BuildDeps` 加 `SelfBaseURL`**

`instanceConfig` 追加：`AdapterBaizeURL string`。`parseConfig` 加 `c.AdapterBaizeURL = get("adapter_baize_url")`。

`internal/channel/bootstrap.go` 的 `BuildDeps` 加字段：

```go
	// SelfBaseURL is baize's own loopback base URL, passed to autostart
	// adapter children so they POST inbound messages back to baize. Empty in
	// tests / when the listen port is not known at assembly time; the webhook
	// supervisor then falls back to config adapter_baize_url, then the
	// loopback default http://127.0.0.1:8080.
	SelfBaseURL string
```

`internal/bootstrap/bootstrap.go` 构造 `deps` 处**不必**填 `SelfBaseURL`（装配期尚不知监听端口，留空走回退）。

- [ ] **步骤 6：`channel.go` 接线 admin client + supervisor**

`openFromConfig`：构造 `Channel` 后不改（admin/supervisor 在 Bootstrap 建，因需 deps.DataDir/SelfBaseURL 与 settings）。

`Bootstrap` 中（任务 9 已加 settings 加载）之后追加：

```go
	// Management plane client (admin proxy + status).
	if c.cfg.AdminURL != "" {
		c.admin = newHTTPAdminClient(c.cfg.AdminURL, c.cfg.OutboundSecret)
	}
	// Autostart child process supervisor.
	if c.cfg.AdapterAutostart {
		// Resolve baize's loopback base for the adapter's -baize inbound URL:
		// deps.SelfBaseURL (when known) -> config adapter_baize_url -> default.
		baizeURL := strings.TrimSpace(deps.SelfBaseURL)
		if baizeURL == "" {
			baizeURL = strings.TrimSpace(c.cfg.AdapterBaizeURL)
		}
		if baizeURL == "" {
			baizeURL = "http://127.0.0.1:8080"
		}
		credsDir := c.cfg.AdapterCredsDir
		if credsDir == "" {
			credsDir = "./data/channels/" + c.cfg.Name
		}
		// Adapter inbound url = baize + inbound route; admin/outbound point at
		// the adapter's loopback addr (derived from AdminURL).
		inboundURL := strings.TrimRight(baizeURL, "/") + "/v0/channels/" + c.cfg.Name + "/inbound"
		args := append([]string(nil), c.cfg.AdapterArgs...)
		args = append(args,
			"-baize="+inboundURL,
			"-secret="+c.cfg.Secret,
			"-creds="+credsDir,
		)
		healthz := strings.TrimRight(c.cfg.AdminURL, "/") + "/healthz"
		c.sup = &supervisor{
			command:    c.cfg.AdapterCommand,
			args:       args,
			healthzURL: healthz,
		}
	}
```

`Channel` 结构体加 `sup *supervisor`；`Start` 改为（autostart 时拉起子进程 + 通知适配器 start）：

```go
func (c *Channel) Start(ctx context.Context) error {
	if c.sup != nil {
		if err := c.sup.start(ctx); err != nil {
			return err
		}
	}
	c.reconcileEnabled(c.GetSettings().Enabled)
	return nil
}
```

`Stop` 改为：

```go
func (c *Channel) Stop(ctx context.Context) error {
	if c.admin != nil {
		_ = c.admin.Stop(ctx)
	}
	if c.sup != nil {
		_ = c.sup.stop(ctx)
	}
	return nil
}
```

（webhook 注册描述符 `Start`/`Stop` 原为 no-op；现在 autostart 实例由 bootstrap 的 closer 与 `start` 分支驱动——wireChannels 对 Bootstrapper 的 `start==true` 会 `ch.Start(runCtx)`。webhook `Bootstrap` 返回的 `start`：autostart 且 enabled 时返回 true 以拉起子进程；否则 false。把 `Bootstrap` 的返回 `true` 改为 `return rt, dir, c.GetSettings().Enabled && c.cfg.AdapterAutostart, nil`。非 autostart 实例不拉子进程（适配器独立部署），其轮询由适配器自身管理。）

- [ ] **步骤 7：运行测试验证通过**

运行：`go test ./internal/channel/webhook/ ./internal/bootstrap/ -v`
预期：PASS（admin client、supervisor 用 fake 子程序拉起/健康/关停、跨平台后缀；2A/任务 8/9 测试不回归）。`go build ./...` 全绿。

- [ ] **步骤 8：Commit**

```bash
git add internal/channel/ internal/bootstrap/
git commit -m "feat(webhook): admin HTTP client + cross-platform adapter child-process supervisor"
```




## 任务 11：big-bang 核心切换——通用渠道管理面 + 删除进程内 weixin

本任务原子完成：通用 `/v0/settings/channels/{name}/...` handler 与 ACL 取代微信专用路由；webhook 渠道补齐 `ManagedChannel` 登录方法；删除 `internal/channel/weixin/` 与微信专用 handler/import；更新默认配置。结束后核心再无进程内 weixin 特例。

**文件：**
- 创建：`internal/api/server_channel_managed.go`
- 修改：`internal/api/server.go`（删 5 条微信路由，注册 5 条通用路由）
- 修改：`internal/controlplane/acl.go`（微信专用规则 → 通用 `{name}` 规则）
- 修改：`internal/channel/webhook/channel.go`（补 `LoginStart/LoginPoll/Logout`）
- 删除：`internal/api/server_channel_weixin.go`、`internal/api/server_channel_weixin_test.go`
- 删除：`internal/channel/weixin/`（整包）
- 修改：`internal/bootstrap/bootstrap.go`（删 weixin blank import）
- 修改：`cmd/baize/main.go`（删 weixin blank import）
- 修改：`internal/bootstrap/wire_channels_test.go`、`internal/api/channels_test.go`（去掉对 weixin 的依赖）
- 修改：`configs/minimal.yaml`、`configs/demo.yaml`（显式 webhook-weixin + autostart）
- 测试：`internal/api/server_channel_managed_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `internal/api/server_channel_managed_test.go`（外部包 `api_test`，用 admin token 鉴权，fake 渠道实现 `channel.Channel` + `channel.ManagedChannel`）：

```go
package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/store"
)

type fakeManaged struct {
	settings  channel.ChannelSettings
	status    channel.AdapterStatus
	loggedOut bool
}

func (f *fakeManaged) Name() string                                              { return "weixin" }
func (f *fakeManaged) Source() string                                            { return "weixin" }
func (f *fakeManaged) Start(context.Context) error                               { return nil }
func (f *fakeManaged) Stop(context.Context) error                                { return nil }
func (f *fakeManaged) SendText(context.Context, string, string, map[string]string) error { return nil }
func (f *fakeManaged) SendMedia(context.Context, string, string, string, []byte, map[string]string) error {
	return nil
}
func (f *fakeManaged) GetSettings() channel.ChannelSettings { return f.settings }
func (f *fakeManaged) UpdateSettings(s channel.ChannelSettings) channel.AdapterStatus {
	f.settings = s
	return f.status
}
func (f *fakeManaged) Status() channel.AdapterStatus { return f.status }
func (f *fakeManaged) LoginStart(context.Context) (channel.LoginTicket, error) {
	return channel.LoginTicket{Ticket: "tk", QRURL: "qr"}, nil
}
func (f *fakeManaged) LoginPoll(context.Context, string) (string, error) { return "success", nil }
func (f *fakeManaged) Logout(context.Context) error                     { f.loggedOut = true; return nil }

func managedTestServer(t *testing.T) *api.Server {
	t.Helper()
	ch := &fakeManaged{
		settings: channel.ChannelSettings{Assignee: "alice", AgentID: "ag", Enabled: true, Allowlist: []string{}},
		status:   channel.AdapterStatus{Running: true},
	}
	srv := api.NewServer(store.NewMemory(), nil, nil)
	srv.AdminToken = "adm"
	srv.RegisterChannel(&api.ChannelHandle{Name: "weixin", Channel: ch})
	return srv
}

func admReq(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Authorization", "Bearer adm")
	return req
}

func TestManagedGetSettings(t *testing.T) {
	srv := managedTestServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, admReq(http.MethodGet, "/v0/settings/channels/weixin"))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET=%d body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["assignee"] != "alice" || got["enabled"] != true || got["running"] != true {
		t.Fatalf("body=%v", got)
	}
}

func TestManagedOperatorForbidden(t *testing.T) {
	srv := managedTestServer(t)
	srv.OperatorToken = "op"
	req := httptest.NewRequest(http.MethodGet, "/v0/settings/channels/weixin", nil)
	req.Header.Set("Authorization", "Bearer op")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("operator GET=%d want 403", rec.Code)
	}
}

func TestManagedLoginLogoutRoutes(t *testing.T) {
	srv := managedTestServer(t)
	hit := func(method, path string) int {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, admReq(method, path))
		return rec.Code
	}
	if c := hit(http.MethodPost, "/v0/settings/channels/weixin/login/start"); c != http.StatusOK {
		t.Fatalf("login/start=%d", c)
	}
	if c := hit(http.MethodGet, "/v0/settings/channels/weixin/login/status?ticket=tk"); c != http.StatusOK {
		t.Fatalf("login/status=%d", c)
	}
	if c := hit(http.MethodPost, "/v0/settings/channels/weixin/logout"); c != http.StatusOK {
		t.Fatalf("logout=%d", c)
	}
	if c := hit(http.MethodGet, "/v0/settings/channels/unknown"); c != http.StatusNotFound {
		t.Fatalf("unknown channel=%d want 404", c)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/api/ -run 'TestManaged' -v`
预期：404（路由未注册）/ 编译失败（`channel.ManagedChannel` 已在任务 9 定义，但路由未挂）。

- [ ] **步骤 3：实现 `internal/api/server_channel_managed.go`**

```go
package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/channel"
)

// managedChannel resolves a registered channel that implements the generic
// management plane. It returns 404 when the name is unknown or the channel
// does not expose management (third-party / non-managed channels).
func (s *Server) managedChannel(w http.ResponseWriter, name string) (channel.ManagedChannel, bool) {
	h, ok := s.Channel(name)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "channel not configured")
		return nil, false
	}
	mc, ok := h.Channel.(channel.ManagedChannel)
	if !ok {
		writeError(w, http.StatusNotFound, "not_managed", "channel has no management plane")
		return nil, false
	}
	return mc, true
}

// settingsResponse is settings + reconciled status, matching the historical
// weixin settings JSON contract (agent_id/allowlist/assignee/enabled/running/reason).
type settingsResponse struct {
	channel.ChannelSettings
	Running bool   `json:"running"`
	Reason  string `json:"reason,omitempty"`
}

func (s *Server) handleGetChannelSettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	mc, ok := s.managedChannel(w, name)
	if !ok {
		return
	}
	st := mc.GetSettings()
	status := mc.Status()
	writeJSON(w, http.StatusOK, settingsResponse{ChannelSettings: st, Running: status.Running, Reason: status.Reason})
}

func (s *Server) handlePutChannelSettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	mc, ok := s.managedChannel(w, name)
	if !ok {
		return
	}
	var body channel.ChannelSettings
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.Allowlist == nil {
		body.Allowlist = []string{}
	}
	status := mc.UpdateSettings(body)
	writeJSON(w, http.StatusOK, settingsResponse{ChannelSettings: body, Running: status.Running, Reason: status.Reason})
}

func (s *Server) handleChannelLoginStart(w http.ResponseWriter, r *http.Request) {
	mc, ok := s.managedChannel(w, r.PathValue("name"))
	if !ok {
		return
	}
	ticket, err := mc.LoginStart(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "adapter_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ticket": ticket.Ticket, "qr_url": ticket.QRURL})
}

func (s *Server) handleChannelLoginStatus(w http.ResponseWriter, r *http.Request) {
	mc, ok := s.managedChannel(w, r.PathValue("name"))
	if !ok {
		return
	}
	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if ticket == "" {
		writeError(w, http.StatusBadRequest, "missing_ticket", "ticket query parameter is required")
		return
	}
	status, err := mc.LoginPoll(r.Context(), ticket)
	if err != nil {
		writeError(w, http.StatusBadGateway, "adapter_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (s *Server) handleChannelLogout(w http.ResponseWriter, r *http.Request) {
	mc, ok := s.managedChannel(w, r.PathValue("name"))
	if !ok {
		return
	}
	if err := mc.Logout(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, "adapter_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}
```

- [ ] **步骤 4：`server.go` 换路由**

删除这 5 行（`:410-414`）微信硬编码路由，替换为通用路由：

```go
	s.mux.HandleFunc("POST /v0/settings/channels/{name}/login/start", s.handleChannelLoginStart)
	s.mux.HandleFunc("GET /v0/settings/channels/{name}/login/status", s.handleChannelLoginStatus)
	s.mux.HandleFunc("POST /v0/settings/channels/{name}/logout", s.handleChannelLogout)
	s.mux.HandleFunc("GET /v0/settings/channels/{name}", s.handleGetChannelSettings)
	s.mux.HandleFunc("PUT /v0/settings/channels/{name}", s.handlePutChannelSettings)
```

删除 `server.go` 中 `weixinMu sync.Mutex` 字段（`:159`）及相关注释；确认无其它引用后移除。

- [ ] **步骤 5：`acl.go` 微信规则 → 通用 `{name}`**

把 `:55-59` 的 5 条 `.../channels/weixin/...` 规则替换为 `{name}` 通配（仍 RoleAdmin）：

```go
	{method: "POST", segments: []string{"v0", "settings", "channels", "{name}", "login", "start"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "channels", "{name}", "login", "status"}, role: RoleAdmin},
	{method: "POST", segments: []string{"v0", "settings", "channels", "{name}", "logout"}, role: RoleAdmin},
	{method: "GET", segments: []string{"v0", "settings", "channels", "{name}"}, role: RoleAdmin},
	{method: "PUT", segments: []string{"v0", "settings", "channels", "{name}"}, role: RoleAdmin},
```

> 确认 ACL 匹配器对 `{name}` 通配段的处理与既有 `{id}`（inbound 规则 `:47`）一致；若匹配器要求通配段语法不同（如 `*`），沿用 `{id}` 同款写法。

- [ ] **步骤 6：webhook 渠道实现 `LoginStart/LoginPoll/Logout`**

在 `internal/channel/webhook/admin.go`（或 channel.go）给 `*Channel` 追加，使其满足 `channel.ManagedChannel`：

```go
func (c *Channel) LoginStart(ctx context.Context) (channel.LoginTicket, error) {
	if c.admin == nil {
		return channel.LoginTicket{}, fmt.Errorf("webhook %s: no adapter admin plane configured", c.cfg.Name)
	}
	ticket, qr, err := c.admin.LoginStart(ctx)
	if err != nil {
		return channel.LoginTicket{}, err
	}
	return channel.LoginTicket{Ticket: ticket, QRURL: qr}, nil
}

func (c *Channel) LoginPoll(ctx context.Context, ticket string) (string, error) {
	if c.admin == nil {
		return "", fmt.Errorf("webhook %s: no adapter admin plane configured", c.cfg.Name)
	}
	return c.admin.LoginPoll(ctx, ticket)
}

func (c *Channel) Logout(ctx context.Context) error {
	if c.admin != nil {
		if err := c.admin.Logout(ctx); err != nil {
			return err
		}
	}
	return nil
}
```

（admin.go 需 import `"github.com/rebornace/baize/internal/channel"` 与 `"fmt"`。）编译期断言：在 channel.go 加 `var _ channel.ManagedChannel = (*Channel)(nil)`。

- [ ] **步骤 7：删除进程内 weixin 与专用 handler/import**

```bash
# 删除包与微信专用 handler/测试
rm -rf internal/channel/weixin
rm -f internal/api/server_channel_weixin.go internal/api/server_channel_weixin_test.go
```

- `internal/bootstrap/bootstrap.go`：删除 blank import `_ "github.com/rebornace/baize/internal/channel/weixin"`（`:30`）及其注释；webhook blank import 保留。
- `cmd/baize/main.go`：删除 `_ "github.com/rebornace/baize/internal/channel/weixin"`（`:13-14`）。
- `internal/bootstrap/wire_channels_test.go`：删除/改写引用 `weixin.` 与进程内 weixin 装配的用例——把"legacy 路径自动装配 weixin"的断言改为：legacy 路径（无 `channels:`）只装配非 DeclarativeOnly 描述符；删除 weixin 后该路径不装配任何 IM 渠道（webhook 为 DeclarativeOnly 被跳过）。涉及 weixin 的具体类型断言改为 webhook 声明式用例。
- `internal/api/channels_test.go`：把以 `*weixin.Channel` 为具体类型的句柄表用例改为 fake 渠道（实现 `channel.Channel`，可选 `ManagedChannel`）或 webhook 实例。

- [ ] **步骤 8：默认配置显式声明 webhook-weixin + autostart**

`configs/minimal.yaml` 的 `channels:` 段（追加/替换为）：

```yaml
channels:
  - name: weixin
    type: webhook
    enabled: true
    config:
      source: weixin
      # secret 留空：adapter_autostart=true 时 baize 生成随机 HMAC 并经 -secret 注入
      outbound_url: http://127.0.0.1:8090/outbound
      admin_url: http://127.0.0.1:8090
      assignee: channel:weixin
      supports_vision: "true"
      adapter_autostart: "true"
      adapter_command: weixin-adapter
      adapter_args: "-addr=127.0.0.1:8090,-creds=./data/channels/weixin"
      adapter_creds_dir: ./data/channels/weixin
      # allowlist 留空 = 放行全部私信；creds 复用 ./data/channels/weixin/creds.json 免重扫
```

`configs/demo.yaml` 加同样的 `channels:` 段（若已有 channels 段则合并）。

> `adapter_command: weixin-adapter` 由 `resolveAdapterPath` 经 PATH / `./bin/weixin-adapter(.exe)` 解析；构建需产出该二进制（`go build -o bin/weixin-adapter ./cmd/weixin-adapter`）。在 README/启动说明补一句"微信需先构建适配器：`go build -o bin/weixin-adapter ./cmd/weixin-adapter`"。

- [ ] **步骤 9：全量验证**

运行：
```bash
go build ./...
go vet ./...
gofmt -l .
go test ./...
```
预期：全绿。重点确认：
- 无任何对 `internal/channel/weixin` 的残留引用（`grep -r "channel/weixin" --include=*.go .` 应为空，适配器库路径 `cmd/weixin-adapter/internal/weixinlink` 不算）。
- `internal/controlplane` ACL 测试通过（通用 `{name}` 规则）。
- 前端无需改动（URL/字段不变）；可选 `cd web/chat && npm run build` 验证嵌入产物。

- [ ] **步骤 10：Commit**

```bash
git add -A
git commit -m "feat(2b): big-bang cutover — generic managed channel plane, drop in-process weixin, autostart adapter"
```




## 任务 12：端到端集成测试（weixin webhook 实例 + 假适配器管理面）

**文件：**
- 创建：`tests/integration/weixin_adapter_test.go`

用 `bootstrap.StartForTest` 起 baize，配置一个 `name=weixin, type=webhook, source=weixin` 实例，`outbound_url`/`admin_url` 指向一个假适配器 HTTP（捕获出站 + 实现 `/admin/*`）。覆盖 §8 中可自动化项：管理代理登录/状态、动态 account 双向闭环、出站前缀/kind/run_id、白名单热更新、管理面鉴权。

- [ ] **步骤 1：编写失败的测试**

```go
package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/bootstrap"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/webhooksig"
)

// fakeWeixinAdapter emulates the weixin-adapter process: it captures baize
// outbound posts and serves the /admin management plane.
type fakeWeixinAdapter struct {
	mu        sync.Mutex
	outbound  []map[string]any
	hasCreds  bool
	polling   bool
}

func (f *fakeWeixinAdapter) handler(secret string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// Verify baize->adapter signature on every endpoint.
		if err := webhooksig.Verify(secret, r.Header.Get("X-Baize-Channel-Timestamp"), body,
			r.Header.Get("X-Baize-Channel-Signature"), time.Now(), 5*time.Minute); err != nil {
			http.Error(w, "bad sig", http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/outbound":
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			f.mu.Lock()
			f.outbound = append(f.outbound, m)
			f.mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/admin/status":
			f.mu.Lock()
			_ = json.NewEncoder(w).Encode(map[string]any{"has_credentials": f.hasCreds, "polling": f.polling})
			f.mu.Unlock()
		case r.URL.Path == "/admin/login/start":
			_ = json.NewEncoder(w).Encode(map[string]string{"ticket": "tk", "qr_url": "qr"})
		case r.URL.Path == "/admin/login/status":
			f.mu.Lock()
			f.hasCreds, f.polling = true, true
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		case r.URL.Path == "/admin/logout":
			f.mu.Lock()
			f.hasCreds, f.polling = false, false
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "logged_out"})
		case r.URL.Path == "/admin/start" || r.URL.Path == "/admin/stop":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	})
}

func (f *fakeWeixinAdapter) outboundCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.outbound)
}

func TestWeixinAdapterE2E(t *testing.T) {
	const secret = "wx-e2e-secret"
	adapter := &fakeWeixinAdapter{}
	srv := httptest.NewServer(adapter.handler(secret))
	defer srv.Close()

	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	cfg.Agent.ID = "wx-agent"
	cfg.ControlPlane.AdminToken = "adm"
	cfg.Channels = []config.ChannelConfig{{
		Name: "weixin", Type: "webhook", Enabled: true,
		Config: map[string]string{
			"source":       "weixin",
			"secret":       secret,
			"outbound_url": srv.URL + "/outbound",
			"admin_url":    srv.URL,
			"assignee":     "channel:weixin",
			"agent_id":     "wx-agent",
		},
	}}
	runtimeURL, _, shutdown := bootstrap.StartForTest(t, cfg)
	defer shutdown()

	// Management plane: login start via generic route (admin token).
	admReq, _ := http.NewRequest(http.MethodPost, runtimeURL+"/v0/settings/channels/weixin/login/start", nil)
	admReq.Header.Set("Authorization", "Bearer adm")
	resp, err := http.DefaultClient.Do(admReq)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("login/start=%d body=%s", resp.StatusCode, b)
	}
	_ = resp.Body.Close()

	// Status before adapter "login": has_credentials=false -> login_required.
	getStatus := func() (bool, string) {
		r, _ := http.NewRequest(http.MethodGet, runtimeURL+"/v0/settings/channels/weixin", nil)
		r.Header.Set("Authorization", "Bearer adm")
		rr, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		var st map[string]any
		_ = json.NewDecoder(rr.Body).Decode(&st)
		_ = rr.Body.Close()
		running, _ := st["running"].(bool)
		reason, _ := st["reason"].(string)
		return running, reason
	}
	if running, reason := getStatus(); running || reason != "login_required" {
		t.Fatalf("pre-login status running=%v reason=%q", running, reason)
	}

	// Adapter->baize inbound with dynamic account (post-login ilink_bot_id).
	inbound := map[string]any{
		"event":   "message",
		"account": "bot@im.bot",
		"peer":    map[string]any{"id": "peer@im.wechat"},
		"text":    "你好微信",
	}
	body, _ := json.Marshal(inbound)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req, _ := http.NewRequest(http.MethodPost, runtimeURL+"/v0/channels/weixin/inbound", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Baize-Channel-Timestamp", ts)
	req.Header.Set("X-Baize-Channel-Signature", webhooksig.Sign(secret, ts, body))
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("inbound=%d body=%s", resp2.StatusCode, b)
	}
	_ = resp2.Body.Close()

	// Assistant reply flows back to the adapter with dynamic account + prefix.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && adapter.outboundCount() == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	if adapter.outboundCount() == 0 {
		t.Fatal("adapter never received outbound")
	}
	adapter.mu.Lock()
	out := adapter.outbound[0]
	adapter.mu.Unlock()
	if out["account"] != "bot@im.bot" {
		t.Fatalf("outbound account=%v want bot@im.bot", out["account"])
	}
	if conv, _ := out["conversation_id"].(string); conv != "weixin:bot@im.bot:peer@im.wechat" {
		t.Fatalf("conv=%v", out["conversation_id"])
	}
	if text, _ := out["text"].(string); !bytes.Contains([]byte(text), []byte("【助手】")) {
		t.Fatalf("assistant prefix missing: %q", text)
	}
	if kind, _ := out["kind"].(string); kind != "assistant" {
		t.Fatalf("kind=%v", kind)
	}
}
```

- [ ] **步骤 2：运行验证**

运行：`go test ./tests/integration/ -run TestWeixinAdapterE2E -v`
预期：PASS（若失败多为管理代理/动态 account 接线问题，回到任务 8-11 修）。

- [ ] **步骤 3：全量 + Commit**

```bash
go build ./... && go test ./...
git add tests/integration/weixin_adapter_test.go
git commit -m "test(integration): weixin webhook instance + adapter management plane E2E"
```

---

## 自检结论（规格覆盖）

- §4 库搬迁/媒体：任务 2/3/4；§5 动态 account：任务 1+8+12；§6 通用设置热更新：任务 9；§7 管理代理 + running/reason：任务 9/10/11+12；§8 回归清单：登录态(11/12)、白名单热更新(9/12)、群聊丢弃(6 适配器)、前缀/kind(12)、context_token(7 适配器)、入站媒体解密(3/7)、出站真发媒体(4/7)、历史 convID 兼容(1/8)、动态 account(8/12)、管理面鉴权(11/12)、autostart(10/11)；§9 跨平台子进程：任务 10；§10 默认配置/核心删除：任务 11；§15 媒体协议：任务 3/4。
- 真机扫码 + 私信收发图片/文件为手工验收（README 已声明 iLink 无 CI 真机测）。
- 前端零改动（URL/JSON 字段不变）。

