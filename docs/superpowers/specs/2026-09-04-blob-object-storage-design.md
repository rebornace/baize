# 对象存储多源（BlobStore）v0 设计规格

> 状态：已交付（2026-09-13 账本对齐）
> 里程碑：开源首版之后、生产硬化（F）之前的企业集成扩展。承接 X3 中间件多源的同一心智模型：**通用接口 + 驱动注册表 + 配置选择 + 本地默认零依赖 + 外部驱动 blank-import 自注册**。
> 范围来源：backlog `docs/superpowers/notes/2026-08-28-oss-backlog-and-enterprise.md` 中 X3 明确延期的「对象存储可替换」。

---

## 1. 目标、总体架构与组件边界

### 1.1 目标

把**分析报告产物**的 HTML 字节存储从「本地磁盘」抽象为可替换的对象存储驱动：

- 默认 `file` 驱动：本地磁盘，零外部依赖，行为与今日完全一致（升级无迁移）。
- 新增 `s3` 驱动：基于 `github.com/minio/minio-go/v7`，**一套 S3 兼容协议**同时覆盖 AWS S3、自建 MinIO、阿里 OSS、腾讯 COS 等。
- 读取采用**代理读**：`GET /v0/artifacts/{id}` 仍由 Baize 从对象存储拉取后流式返回，对外 URL 与 ACL 不变，公开桶/私有桶皆兼容。

### 1.2 v0 范围边界

**范围内：**

- 仅分析报告 HTML 产物（`internal/artifact`）。
- 通用 `blob.Store` 接口 + 驱动注册表（复刻 X3 `middleware` 范式），为未来扩展预留。

**明确不做（v0 外）：**

- Connector 导入规格文件（`internal/connector/specstore`）、用户上传 Skill 包（`internal/skill/catalog`）的 blob 化——通用接口已预留扩展点，后续独立里程碑。
- 聊天附件/图片持久化（今日 base64 进内存、过队列、喂 LLM 后即弃，DB 仅存文件名）——属产品行为与 DB 模型变更，不做。
- 预签名 URL / 浏览器直连 S3 / CDN 加速。
- 对象生命周期、版本控制、服务端加密配置（走桶侧策略，YAGNI）。
- 引入 AWS 官方 SDK；只用 minio-go（S3 兼容）。

**非对象存储范畴（不动）：** SQLite/PG 数据库文件、微信 `creds.json`/`settings.json` 凭证、config YAML 与 `<name>.local.yaml` 覆盖层、`file:` 引用的 operator/auth 密钥、会话 fork/rollback（纯 SQL 行操作）、日志（stderr）。

### 1.3 新增包与组件

| 包/文件 | 职责 |
|---------|------|
| `internal/blob/blob.go` | 通用接口 `Store`（`Put/Get/Delete`）、`Options`、哨兵错误 `ErrNotFound` |
| `internal/blob/registry.go` | 驱动注册表：`DriverFactory`、`RegisterDriver`、`Open`、`ListDrivers` |
| `internal/blob/file/file.go` | 本地磁盘驱动（`file`，默认） |
| `internal/blob/memory/memory.go` | 内存驱动（`memory`，测试/未来 in-process 用） |
| `internal/blob/s3/s3.go` | S3 兼容驱动（`s3`，minio-go），`init()` 自注册 |
| `internal/artifact/store.go` | 业务接口 `Store`（`PutHTML`/`Get`）签名加 `ctx`，语义不变 |
| `internal/artifact/blob_store.go` | 新实现：字节走 `blob.Store`（key 前缀 `artifacts/` 内部固定），元数据走 SQL（取代 `FileStore` 直接落盘） |
| `internal/config/config.go` | 新增 `storage` 配置段 + 默认归一化 |
| `internal/bootstrap/bootstrap.go` | 按配置 `blob.Open`，注入 artifact 层 |
| `cmd/baize/main.go` | blank-import `internal/blob/s3` |
| `configs/*.yaml` | 加 `storage` 段注释示例 |

### 1.4 数据流

**写**（`analysis` 工具产出报告页）：

```
analysis.Invoker(ctx)
  → report.Build(req) 得 html
  → artifact.PutHTML(ctx, runID, html)
      → 生成 id = "art_" + 16 随机 hex
      → blob.Put(ctx, "artifacts/<id>.html", []byte(html), "text/html; charset=utf-8")
      → SQL INSERT artifacts(id, run_id, created_at)   // 元数据权威来源
      → 若 SQL 失败：blob.Delete(ctx, key) 回滚字节（幂等，失败仅记日志）
  → 返回 artifact_url = /v0/artifacts/<id>
```

**读**（`GET /v0/artifacts/{id}`）：

```
handleGetArtifact
  → ACL 校验（controlplane，不变）
  → artifact.Get(ctx, id)
      → SQL SELECT run_id FROM artifacts WHERE id=?   // 未知 id → 404
      → blob.Get(ctx, "artifacts/<id>.html")
  → 流式返回 HTML（Content-Type: text/html; charset=utf-8）
```

元数据（id→run_id 映射、ACL、created_at）始终以 SQL 为权威来源；对象存储只承载 HTML 字节。这与今日 `FileStore`「SQL 存元数据、磁盘存字节」的分工一致，仅把字节层换成可替换驱动。

---

## 2. blob 通用接口、驱动注册表与配置

### 2.1 通用接口（`internal/blob/blob.go`）

刻意做成**字节存储**而非 HTML 语义，供未来 connector 规格 / skill 包 / 附件复用：

```go
package blob

import (
	"context"
	"errors"
)

// ErrNotFound 表示对象不存在。各驱动把底层「不存在」错误映射为此哨兵。
var ErrNotFound = errors.New("blob: object not found")

// Store 是可替换的对象存储抽象。key 为驱动内相对键，用 "/" 分隔
// （如 "artifacts/art_xxx.html"）。
type Store interface {
	// Put 写入对象。contentType 用于 S3 Content-Type，file 驱动可忽略。
	Put(ctx context.Context, key string, data []byte, contentType string) error
	// Get 读取对象；不存在返回包装了 ErrNotFound 的错误。
	Get(ctx context.Context, key string) ([]byte, error)
	// Delete 删除对象；不存在视为成功（幂等）。
	Delete(ctx context.Context, key string) error
}
```

- 不引入 `List` / `PresignURL` / 目录概念（YAGNI：v0 代理读用不到；对象存储本无真目录）。
- key 由调用方（artifact 层）拼出；file 驱动映射 `filepath.Join(dir, key)`，s3 驱动映射 `<bucket>/<prefix>/<key>`。

### 2.2 注册表（`internal/blob/registry.go`）

复刻 X3 `middleware` 注册表范式：

```go
type DriverFactory func(ctx context.Context, opts Options) (Store, error)

func RegisterDriver(name string, f DriverFactory)
func Open(ctx context.Context, driver string, opts Options) (Store, error)
func ListDrivers() []string
```

- `file` 与 `memory` 随 `internal/blob` 子包默认可用（`file` 由 `internal/blob/file` 的 `init()` 注册，`memory` 同理）。
- `s3` 在 `internal/blob/s3` 包 `init()` 自注册；`cmd/baize/main.go` blank-import `_ "github.com/rebornace/baize/internal/blob/s3"`。
- `Open` 未知 driver 返回明确错误；同名重复注册 panic（与 X3 一致，暴露编程错误）。

### 2.3 驱动

**`file`（默认，`internal/blob/file/file.go`）—— 零依赖：**

- `Put`：`os.MkdirAll(filepath.Dir(path), 0o755)` + `os.WriteFile(path, data, 0o644)`。
- `Get`：`os.ReadFile`；`os.IsNotExist(err)` → 包装 `ErrNotFound`。
- `Delete`：`os.Remove`；忽略 `IsNotExist`（幂等）。
- 根目录来自 `Options.File.RootDir`；artifact key 前缀 `artifacts/` 落到 `<rootDir>/artifacts/`，与今日 `<dataDir>/artifacts/` 路径完全一致。

**`memory`（`internal/blob/memory/memory.go`）—— 测试/进程内：**

- `map[string][]byte` + `sync.RWMutex`；`Get` 缺失返回 `ErrNotFound`；`Delete` 幂等。

**`s3`（`internal/blob/s3/s3.go`）—— minio-go：**

- 依赖 `github.com/minio/minio-go/v7` + `github.com/minio/minio-go/v7/pkg/credentials`。
- 客户端：`minio.New(endpoint, opts)`，`Credentials: credentials.NewStaticV4(accessKey, secretKey, "")`，`Secure: useSSL`，`Region: region`。
- `Put`：`client.PutObject(ctx, bucket, objectKey, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: contentType})`，`objectKey = prefix + "/" + key`。
- `Get`：`client.GetObject(ctx, ...)` 后 `io.ReadAll`；用 `minio.ToErrorResponse(err)`，`Code == "NoSuchKey"`（或 `"NoSuchBucket"`）→ 包装 `ErrNotFound`。
- `Delete`：`client.RemoveObject(ctx, ...)`；忽略 NoSuchKey/NoSuchBucket（幂等）。
- 启动校验（在工厂内，`Open` 时执行，fail-fast）：`BucketExists`；桶不存在时 `auto_create_bucket=true` 则 `MakeBucket`（带 region），否则返回明确错误（含 bucket/endpoint）。凭据（access/secret key 从环境变量读出后）为空 → 报错，不静默降级。
- minio-go client 自管理连接池，无持久 fd 需关闭；`Store` 实现**不**强制 `Close`。

### 2.4 配置（`internal/config/config.go` 新增 `storage` 段）

命名取 `storage`（贴近用户心智「产物存哪」），与 X3 `middleware` 段风格对齐：

```yaml
storage:
  driver: file            # file（默认）| s3
  file:
    root_dir: ""          # 留空则用 dataDir（sqlite 为 .db 所在目录，postgres 为 ./data）
  s3:
    endpoint: ""          # s3.amazonaws.com / play.min.io / oss-cn-hangzhou.aliyuncs.com ...
    region: ""
    bucket: ""
    prefix: "baize"       # key 前缀，多实例/多环境隔离
    access_key_env: "S3_ACCESS_KEY"   # 仅记环境变量名，密钥不落 YAML
    secret_key_env: "S3_SECRET_KEY"
    use_ssl: true
    path_style: false     # MinIO/自建常需 true；AWS 用 false
    auto_create_bucket: false
```

默认归一化（`applyDefaults`）：`driver=file`；`s3.prefix=baize`、`use_ssl=true`、`access_key_env=S3_ACCESS_KEY`、`secret_key_env=S3_SECRET_KEY`、`auto_create_bucket=false`、`path_style=false`。密钥**只从环境变量**读（与 X3 redis `password_env` 同一安全约定）。

`file.root_dir` 留空时，bootstrap 以现有 `dataDir(cfg)` 作为 blob 根，使 `artifacts/` 子路径与今日布局字节级一致。

`file.root_dir` 留空时，bootstrap 以现有 `dataDir(cfg)` 作为 blob 根，使 `artifacts/` 子路径与今日布局字节级一致。

### 2.5 `Options` 与 artifact 适配

`blob.Options` 携带各驱动参数：

```go
type Options struct {
	File FileOptions
	S3   S3Options
}
type FileOptions struct{ RootDir string }
type S3Options struct {
	Endpoint, Region, Bucket, Prefix string
	AccessKey, SecretKey             string // 由 bootstrap 从环境变量解析后填入
	UseSSL, PathStyle, AutoCreateBucket bool
}
```

**artifact 层适配**：`internal/artifact` 的业务接口加 `ctx`，语义不变：

```go
type Store interface {
	PutHTML(ctx context.Context, runID string, html string) (id string, err error)
	Get(ctx context.Context, id string) (html string, runID string, err error)
}
```

- 新增 `artifact.NewStore(blobs blob.Store, backend store.SQLBackend) (Store, error)`：保留现有 SQL 元数据逻辑（建 `artifacts` 表、插 id/run_id/created_at、查 run_id），仅把字节 I/O 换成 `blobs.Put/Get/Delete`，key = `artifacts/<id>.html`，contentType = `text/html; charset=utf-8`。
- 删除/退役 `FileStore` 中直接 `os.WriteFile`/`os.ReadFile`/`os.Remove` 的字节路径（建目录逻辑移入 `blob/file` 驱动）；SQL schema 与 `q()` 重绑定逻辑保留。
- 调用点改为传 ctx：
  - `internal/analysis/tool.go` 的 `Invoker`：`art.PutHTML(ctx, runID, html)`（ctx 现成）。
  - `internal/api/server.go` 的 `handleGetArtifact`：`srv.Artifacts.Get(r.Context(), id)`。
- bootstrap：`blobStore, _ := blob.Open(ctx, cfg.Storage.Driver, opts)`；`artStore, _ := artifact.NewStore(blobStore, sqlBackend)`；`reg.RegisterSpec(analysis.ToolSpec(), analysis.Invoker(artStore))`；`srv.Artifacts = artStore`。仅在 `sqlBackend != nil` 时装配（与今日一致）。

---

## 3. 错误处理、测试与范围边界

### 3.1 错误处理

- **对象不存在**：`blob.Get` 返回包装 `ErrNotFound` 的错误（file 包 `os.ErrNotExist`，s3 包 `NoSuchKey`/`NoSuchBucket`）。artifact `Get` 遇 `ErrNotFound` 维持今日语义（`"artifact not found"` → HTTP 404）。用 `errors.Is(err, blob.ErrNotFound)` 判定。
- **写失败回滚**：`PutHTML` 先 `blob.Put` 成功再插 SQL；SQL 失败则 `blob.Delete` 回滚字节。Delete 幂等，回滚本身失败仅 `log.Printf` 记录、不掩盖主错误。顺序与今日「先写文件再插库、库失败删文件」一致，不留孤儿字节。
- **S3 启动 fail-fast**：`s3` 工厂内 `BucketExists` 校验；桶不存在且 `auto_create_bucket=true` → `MakeBucket`，否则返回含 bucket/endpoint 的明确错误。凭据为空 → 直接报错。
- **S3 运行时错误**：`GetObject`/`PutObject` 的网络/权限错误原样包装返回；artifact 读取失败走现有 HTTP 错误处理（不吞错）。**不**静默 fallback 到本地磁盘（避免多副本下数据分裂）。
- **ctx 传播**：`Put/Get/Delete` 均收 ctx，minio 调用透传；请求取消即取消 S3 操作。
- **关停**：blob 驱动无后台 goroutine、无持久 fd（file 不持 fd，s3 client 自管连接池），故 `blob.Store` 不设 `Close`，与 X3 的 `Close func()` 不同。

### 3.2 测试策略

- **`blob` 注册表**：未知 driver 报错；注册/列举；file/memory 默认可用；重复注册 panic。
- **`file` 驱动**：`t.TempDir()` 下 Put→Get 往返；Get 不存在 → `errors.Is(ErrNotFound)`；Delete 幂等；key 含子目录自动建父目录；contentType 不影响往返。
- **`memory` 驱动**：往返 / 不存在 / 幂等删除。
- **`s3` 驱动**（不依赖真实 S3）：用 `httptest.Server` 伪造 S3 XML/HTTP 端点，验证：
  - PutObject 路径/bucket/key（含 prefix）拼装、Content-Type 头；
  - GetObject 正常返回字节；`NoSuchKey` XML → `ErrNotFound`；
  - RemoveObject 对 NoSuchKey 幂等；
  - 启动 `BucketExists` 404 + `auto_create_bucket=true` → 发 MakeBucket；`false` → 返回错误；
  - 凭据从环境变量读取的解析逻辑单测覆盖。
- **artifact 层**：用 `memory` blob + SQLite/内存 SQLBackend：
  - `PutHTML(ctx,...)`→`Get(ctx,...)` 往返且 run_id 正确；
  - SQL 插入失败（注入失败 backend）断言 blob 中无残留字节（回滚生效）；
  - 未知 id → not found；
  - ctx 透传（可由 s3 httptest 侧验证取消）。
- **配置**：默认 `storage.driver=file` 与显式 s3 配置解析、默认值归一化、密钥环境变量名解析。
- **全量回归**：`go build ./...` + `go test ./... -count=1` 全绿；`file` 驱动下 `<dataDir>/artifacts/art_*.html` 路径与今日完全一致（升级无迁移）。

### 3.3 范围边界（再确认）

- 不做 connector 规格、skill 包、聊天附件的 blob 化（通用接口已留好，后续独立里程碑）。
- 不做预签名 URL / 浏览器直连 / CDN。
- 不做对象生命周期、版本控制、SSE-C/SSE-KMS 配置（走桶侧策略）。
- 不引入 AWS 官方 SDK；仅 minio-go。
- 开源合规（已核实，2026-09-04）：**`minio-go/v7` 客户端 SDK 许可证为 Apache-2.0**（pkg.go.dev 与 minio-go 仓库 LICENSE 均标注 Apache-2.0）。MinIO *服务端* 自 2021 年改 AGPLv3，但客户端 SDK 独立保持 Apache-2.0，引入客户端库不继承 AGPL copyleft。故 `internal/blob/s3` 可随公开仓发布，无需拆模块。
- 公开导出：`internal/blob`、`internal/blob/file`、`internal/blob/memory`、`internal/blob/s3` 全部随公开仓发布。

---

## 4. 实现任务预览（供 writing-plans 展开）

1. **T1 config**：`storage` 配置段 + 默认归一化 + 测试。
2. **T2 blob 核心**：接口、`ErrNotFound`、`Options`、注册表 + 测试。
3. **T3 file + memory 驱动**：实现 + 单测。
4. **T4 artifact 适配**：`Store` 加 ctx、`NewStore(blobStore, backend)`、字节走 blob、调用点传 ctx + 单测（含回滚）。
5. **T5 bootstrap 装配**：`blob.Open` + 注入 artifact；`file.root_dir` 默认 dataDir；configs 示例。
6. **T6 s3 驱动**：minio-go（Apache-2.0）实现 + httptest 假端点测试；`cmd/baize/main.go` blank-import。
7. **T7 全量验证/文档/收尾**：`go build`/`go test` 全绿、configs 注释、README 生产集成小节（可选）、双仓推送。

> 依赖许可已在设计阶段核实：minio-go/v7 = Apache-2.0，直接 `go get` 即可，无需 T0 许可决策。


