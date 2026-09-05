# Agent 会话文件工作区（Agent Workspace Files）v0 设计规格

> 日期：2026-09-04（§4 图片能力于 2026-09-05 增补）
> 状态：已批准（分节头脑风暴 §1–§4 全部确认）
> 依赖：BlobStore 抽象（`internal/blob`，已交付，含 file/s3 驱动）
> 归属里程碑：Agent 能力增强（非 F 生产硬化）

---

## 1. 目标、范围与整体架构

### 1.1 背景与问题

Baize 的 Agent 目前**没有任何文件读写工具**，且用户上传的聊天附件是 base64
内存即弃：文本在发送当轮被抽取并内联进提示词，图片走当轮多模态注入，原始文件
不落盘。后果：

- 后续轮次 / 新会话中，Agent 无法再读到之前上传的文档；
- 图片**只在当轮可见**：持久化气泡仅存文件名（`server.go` 注释
  "Image bytes never enter SQLite"），历史回放只用文本（`engine.go` `buildMessages`
  历史分支只取 `m.Content`，`userParts` 传 `nil`），模型跨轮看不到图片内容；
- Agent 无法把长任务的中间结果落盘、跨轮次反复引用；
- 缺少「持久工作目录 / 长期文件记忆」，处理多文档、长任务时能力受限。

BlobStore 抽象（file/s3）已交付但仅用于分析报告产物（`artifacts/` 前缀）。

### 1.2 目标

为 Agent 提供一个**按会话隔离的持久文件工作区**：

1. 用户上传的**文本类附件持久化**保存，后续轮次 / 新会话可读（按需读取，见 §3.2）；
2. 用户上传的**图片附件持久化**保存，Agent 可通过 `read_image` **按需看图**
   （不自动随历史重注入，控制 token 成本，见 §4）；
3. Agent 通过内置文件工具在会话工作区内 `list / read / write / delete` 文件、
   `read_image` 看图，跨轮次、跨 run 持久（同一 conversation_id 内）；
4. 存储复用已有 `blob.Store`（file/s3 同一套配置），多副本 / 容器无状态 /
   企业对象存储能力自动继承。

### 1.3 V0 范围

- 给 `blob.Store` 增加枚举能力 `List(ctx, prefix)`，file / memory / s3 三驱动实现。
- 新增 `internal/workspace` 包：基于 `blob.Store` 的会话工作区服务 + 5 个内置工具
  （`list_files` / `read_file` / `write_file` / `delete_file` / `read_image`）。
- 文本类附件在发送时写入工作区 `uploads/`；当轮提示注入可用清单。
- 图片类附件在发送时**持久化**到工作区 `uploads/`（字节落 blob），当轮仍走
  多模态注入（零回归）；跨轮靠 `read_image` 按需读图（§4）。
- 打通**工具结果多模态通道**：工具可返回图片 part，引擎将其编码进 tool 消息
  （§4.2）；事件不存图片字节，冷恢复从 blob 重取（§4.3）。
- bootstrap 用**同一个 blob.Store 实例**装配（artifacts 用 `artifacts/` 前缀、
  workspace 用 `workspaces/` 前缀）。

### 1.4 V0 非目标（明确不做）

- 全局共享区 / 跨会话文件库 / 知识库检索；
- 文件版本历史 / 回收站；
- 图片**自动随历史重注入**（V0 仅按需 `read_image`；自动窗口重注入列入 backlog）；
- 非图片二进制附件（音频 / 视频 / 可执行文件等）的持久化与专用工具；
- 文件局部编辑 / patch 工具（`write_file` 为整文件覆盖）；
- UI 文件浏览器 / 文件下载 HTTP 端点（V0 仅 Agent 侧可见，不面向 Chat UI）；
- MCP 子进程 / 连接器与工作区共享 cwd；
- 大对象流式读写（V0 文本 ≤ 256 KiB、图片 ≤ 10 MiB，均 `[]byte`）。

### 1.5 组件与数据流

```
Chat 发送（带附件）
  └─ attach.Process
       ├─ 文本：当轮内联进提示词（现状不变）
       │    └─ 新增 SaveUpload(convID, name, text) → blob.Put("workspaces/<conv>/uploads/<file>")
       └─ 图片：当轮多模态注入 userParts（现状不变）
            └─ 新增 SaveUploadBytes(convID, name, bytes, mime)
                 → blob.Put("workspaces/<conv>/uploads/<img>", bytes, mime)

Agent run（引擎在调工具前已把 conversation_id 注入 ctx，engine.go:821）
  └─ LLM 调用文件工具
       └─ workspace 工具 invoker：identity.ConversationIDFrom(ctx) → convID
            ├─ 文本工具 → Service.{List,Read,Write,Delete}File → blob.{List,Get,Put,Delete}
            └─ read_image → Service.ReadImage → blob.Get → 返回图片 part
                 └─ 引擎把图片 part 编码进 tool 消息（多模态工具结果，§4.2）

装配（bootstrap）
  openBlobStore(cfg) → blobStore（同一个）
       ├─ artifact.NewStore(blobStore, sqlBackend)   // artifacts/ 前缀
       └─ workspace.New(blobStore, WithVision(provider.SupportsVision))  // workspaces/ 前缀
            ├─ 注册 5 个内置工具到 tool.Registry
            └─ 注入 api.Server 供附件（文本+图片）持久化
```

---

## 2. blob.List、工作区布局与路径安全

### 2.1 `blob.Store` 增加 List

`internal/blob/blob.go`：

```go
// ListEntry 是一次 List 返回的对象元数据。
type ListEntry struct {
    Key  string // 相对 store root 的完整 key（用 "/" 分隔，含前缀）
    Size int64
}

// Store 新增方法：
// List 枚举给定 key 前缀下的对象。prefix 为空枚举全部。
// 前缀不存在（无对象）时返回空切片、nil error，不报错。
List(ctx context.Context, prefix string) ([]ListEntry, error)
```

驱动实现：

- **file**（`internal/blob/file/file.go`）：`filepath.WalkDir(filepath.Join(root, FromSlash(prefix)))`，
  对每个常规文件计算相对 `root` 的路径并用 `/` 归一作为 Key；`os.IsNotExist`
  （前缀目录不存在）返回空切片；`Size` 取 `info.Size()`。
- **memory**（`internal/blob/memory/memory.go`）：持读锁遍历 map，`strings.HasPrefix(key, prefix)`
  过滤，Size 取 `len(data)`。
- **s3**（`internal/blob/s3/s3.go`）：`minio.Client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true})`
  遍历 channel，收集 `object.Key`（去掉 bucket 维度，key 即对象键）与 `object.Size`；
  Recursive 递归列出（V0 工作区文件量小，不做分隔符分页）。

artifacts 不使用 List，行为不受影响。`var _ blob.Store` 断言在三驱动处继续生效
（接口加方法后编译期强制三驱动都实现）。

### 2.2 工作区 key 布局

| 内容 | blob key |
|---|---|
| 工作区根前缀 | `workspaces/<convID>/` |
| 上传附件 | `workspaces/<convID>/uploads/<文件名>` |
| Agent 写的文件 | `workspaces/<convID>/<Agent 给的相对路径>` |

- 工具对外暴露的**逻辑路径**是去掉工作区根前缀后的相对路径，如
  `uploads/report.pdf`、`notes/summary.md`。Agent 不感知 blob key / convID。
- convID 来自 `identity.ConversationIDFrom(ctx)`。

### 2.3 路径安全（核心）

所有工具入参的逻辑路径统一经 `safeRelPath(p string) (string, error)` 处理：

1. 转 `/`：`filepath.ToSlash`；
2. `path.Clean`；
3. 拒绝空路径 / 绝对路径（`path.IsAbs` 或以 `/` 开头）/ Windows 盘符；
4. clean 后若等于 `".."` 或以 `"../"` 开头 → 返回错误；
5. 结果作为相对片段拼到 `workspaces/<convID>/` 之后。

效果：所有访问被约束在该会话前缀内，无法逃逸到其他会话或 store 根。
非法路径 → 工具返回 `isError=true` + 清晰消息（不 panic）。

附件文件名净化：`filepath.Base`、去控制字符与路径分隔、避免 `..`；重名采用
覆盖（同一次上传同名文件覆盖写；V0 不做去重后缀）。

convID 为空（无会话的机器调用 / MCP 导出路径）→ 工具返回明确错误
`"workspace requires a conversation context"`（isError=true），不 panic。

### 2.4 文本类内置工具（4 个）

注册为内置全局工具（与 `create_analysis_page` 同一注册点），
`require_approval=false`、`require_login=false`。第 5 个工具 `read_image`
（图片）见 §4.4。

| 工具 | 入参 | 返回（content map） | 说明 |
|---|---|---|---|
| `list_files` | `path?`（默认工作区根） | `{path, entries:[{name, type:"file"\|"dir", size?}]}` | 按 `/` 分段把扁平 key 聚合成一层树形：既有文件、又有以其为前缀的子路径 → 目录。图片文件同样列出 |
| `read_file` | `path` | `{path, content}` | UTF-8 文本；超 64 KiB 截断并附 `…[truncated]`；缺失 → isError not-found；对图片/二进制路径返回提示改用 `read_image` |
| `write_file` | `path`, `content` | `{path, bytes}` | 整文件 Put；超 256 KiB 报错；自动建父"目录"（blob 扁平，无需真建） |
| `delete_file` | `path` | `{path, deleted:true}` | Delete 幂等；不存在也返回成功（文本/图片通用） |

工具描述（写入 LLM spec 的 description）明确：这是**按会话持久**的工作区；
上传的文件在 `uploads/` 下，跨轮次可用；路径相对工作区根，不可用绝对路径/`..`。

`list_files` 的目录聚合：列出 `workspaces/<conv>/<path>/` 前缀下所有 key，
去掉该前缀后按第一段 `/` 分组——第一段后还有内容的为 `dir`，否则为 `file`。

---

## 3. 附件持久化、提示注入、装配、错误处理与测试

### 3.1 附件落工作区（`internal/api`）

- 聊天发送处理器（`server.go` 约 1380–1530，`attach.Process` 之后）：
  - **文本类**抽取结果 `texts []attach.Extracted`（含 text/md/csv 原文与
    docx/xlsx/pdf 抽取文本），逐条
    `workspace.Service.SaveUpload(convID, ex.Filename, ex.Text)`，
    内容为可读 UTF-8 文本（Agent `read_file` 读到文字而非二进制）。
  - **图片类** `images`（含 `ImageBytes/ImageMIME/Filename`），逐条
    `workspace.Service.SaveUploadBytes(convID, img.Filename, img.ImageBytes, img.ImageMIME)`
    → `blob.Put("workspaces/<conv>/uploads/<name>", bytes, mime)`。图片与文本同处
    `uploads/`，靠扩展名/MIME 区分。当轮多模态注入（`userParts`）保持不变。
- 写失败**不阻断聊天**（当轮仍内联文本 / 注入图片）：`log.Printf` 警告并继续。
- `workspace.Service` 作为新依赖注入 `api.Server`（新字段，与 `Artifacts` 同级），
  由 bootstrap 构造。会话 ID 取发送请求的 `conversation_id`（与 run 归属一致）。

### 3.2 处理时机与提示注入（上传处理一次，之后按需读取）

**文件解析 / 抽取只在上传时发生一次**，之后不重复处理、也不每轮重发：

- **上传当轮**：文本抽取结果内联进当轮提示词（现状，保证模型当轮一定看得到）；
  同时内容落 blob 一次；在该用户消息文本末尾追加一行清单，例如：
  `[工作区] 以下文件已保存，后续可用 read_file / read_image 按需读取：uploads/report.pdf, uploads/shot.png`。
- **后续轮次 / 新会话**：历史气泡只含文件名（`displayText` 的 `（附件：…）`），
  **不自动重发文件/图片内容**（避免 token 随轮次累积膨胀）。Agent 需要时自行调用
  `read_file`（文本，直接返回 blob 中已存好的文本，**不重新解析原文件**）或
  `read_image`（图片，§4）按需取回。
- 工具 description 已说明 `uploads/` 的存在；Agent 也可随时 `list_files` 自行发现。

| | 解析/抽取 | 当轮提示词 | 后续轮次 |
|---|---|---|---|
| 现状 | 上传时 1 次 | 文本全文内联 / 图片多模态 | 内容丢失（仅文件名） |
| 本特性 | 上传时 1 次 | 文本全文内联 / 图片多模态 + 落 blob | 不自动重发，`read_file`/`read_image` 按需读 |

### 3.3 装配（`internal/bootstrap`）

- 复用现有 `blobStore`（`openBlobStore` 产物）：
  `ws := workspace.New(blobStore, workspace.WithVision(func() bool { return llmProvider.SupportsVision() }))`。
- 注册 5 个工具到 tool registry（`ws.Tools()` 返回 spec/invoker，或在 bootstrap
  逐个 `RegisterSpecApproved(spec, inv, false)`）。
- 把 `ws` 传入 `api.Server`（新字段 `Workspace`）；同时把图片解析回调
  （`ws.ImagePartResolver`，§4.3）注入 `run.Engine` 供冷恢复重建图片 part。
- artifacts 与 workspace 共用同一 blob.Store、同一套 file/s3 配置；
  file 驱动下工作区落在 `<dataDir>/workspaces/<conv>/...`。

### 3.4 错误处理与上限

| 情况 | 行为 |
|---|---|
| 路径逃逸 / 绝对路径 / 空路径 | 工具 isError=true + 清晰消息，不 panic |
| convID 为空 | 工具 isError=true：`workspace requires a conversation context` |
| `read_file` 文件缺失 | isError=true，not-found 消息 |
| `delete_file` 不存在 | 幂等成功 `{deleted:true}` |
| `list_files` 前缀不存在 | 返回空 entries，不报错 |
| `write_file` content > 256 KiB | isError=true，文件过大 |
| `read_file` content > 64 KiB | 返回前 64 KiB + `…[truncated]` |
| `read_file` 读到图片/二进制 | isError 或返回提示「请用 read_image」 |
| `read_image` 非图片内容 | isError=true，提示不是图片 |
| `read_image` 图片 > 10 MiB | isError=true，图片过大 |
| `read_image` 当前模型不支持视觉 | isError=true，提示切换视觉模型（对齐上传 `vision_unsupported`） |
| 工具结果图片 part 但 provider 拒绝 / 冷恢复重取失败 | 降级为文本提示「image available at <path>，call read_image」，不崩溃（§4.3/§4.6） |
| 附件落工作区失败（文本或图片） | 记日志警告，不阻断聊天 |
| blob 驱动错误（s3 5xx 等） | 工具 isError=true，错误上抛（不伪造成 not-found） |

### 3.5 测试

- **blob 三驱动 List**：
  - file：写多个文件（含子目录），按前缀 List 返回正确 key/size；空前缀目录返回空。
  - memory：Put 后按前缀过滤正确。
  - s3：httptest mock ListObjects（XML），断言递归列举与 prefix 透传。
- **workspace 包**（用 memory 驱动）：
  - 路径安全：`../x`、`/abs`、`..`、`a/../../b` 均被拒；会话隔离（A  conv 读不到 B）。
  - 四工具往返：write→read→list（含目录聚合）→delete；read 缺失 not-found；
    delete 幂等；write 超限报错；read 截断标记。
  - convID 为空返回明确错误。
  - `SaveUpload` 写入 `uploads/<name>` 且可被 `read_file("uploads/<name>")` 读到。
  - `SaveUploadBytes` + `ReadImage`：图片可经 `read_image` 返回图片 part + 元数据；
    非图片内容 / 超 10 MiB / 非视觉模型均 isError。
- **工具结果多模态通道（§4.2）**：假 invoker 返回带图片 part 的 content →
  断言 tool 消息走 `Parts` 且 openai 编码出 `image_url`；ToolResult 事件 content
  **不含 base64**、含 `image_refs` 指针。
- **冷恢复（§4.3）**：预置 blob 图片 + 事件 image_refs → resolver 重建 Parts；
  resolver 为 nil / 重取失败 → 降级为文本提示，不报错。
- **API**：带文本附件发送后，blob 中存在 `workspaces/<conv>/uploads/<file>`；
  带图片附件发送后存在 `workspaces/<conv>/uploads/<img>`，当轮仍多模态注入，
  下一轮 `read_image` 可查看；附件落盘失败不阻断发送（可注入失败 blob 验证）。
- **bootstrap**：装配后 tool registry 含 5 个文件工具、`api.Server` 持有 workspace
  服务、`run.Engine` 持有图片解析回调。

### 3.6 范围边界

不做：全局共享区、版本/回收站、图片自动随历史重注入、非图片二进制（音视频等）、
局部编辑、UI 文件浏览器/下载端点、MCP 共享 cwd、流式大对象。这些进入 backlog
作为后续候选。

---

## 4. 图片持久化与按需读图（多模态工具结果通道）

> 本节于 2026-09-05 增补。设计决策：图片**持久化**到工作区 blob，但**不自动随
> 历史重注入**（token 成本高）；Agent 用 `read_image` **按需看图**，图片经
> 「工具结果多模态通道」回传。

### 4.1 上传时图片持久化

- 聊天处理器在 `attach.Process` 后，对 `imageExts`（含 `ImageBytes/ImageMIME/Filename`）
  逐条 `ws.SaveUploadBytes(convID, filename, bytes, mime)` →
  `blob.Put("workspaces/<conv>/uploads/<name>", bytes, mime)`。
- 图片与文本同在 `uploads/`，靠扩展名 / MIME 区分；文件名净化规则复用 §2.3。
  失败仅 `log.Printf` 警告、不阻断发送；当轮多模态注入（`userParts`）保持不变。

### 4.2 工具结果多模态通道（核心）

- **不改 `tool.Invoker` 签名**（避免波及所有工具 / 测试）。在 `tool` 包新增保留键
  与两个助手：
  - `tool.WithImageParts(content map[string]any, parts ...llm.ContentPart) map[string]any`：
    把图片 part 放进 content 的保留键 `__baize_image_parts__`；
  - `tool.ExtractImageParts(content map[string]any) (cleaned map, parts []llm.ContentPart)`：
    引擎取出图片 part 并从 map 移除（cleaned 用于文本 JSON / 事件）。
- 引擎在**两处** `Tools.Invoke` 之后统一处理：`runLoop`（`engine.go` 约 720）与
  HITL 冷恢复（`engine.go` 约 354）。
  - 取出 parts；文本部分仍 `json.Marshal`；
  - **有图片** → tool 消息用 `Parts`：
    `[{type:text, text:<json>}, {type:image, ImageBytes, ImageMIME} ...]`
    （编码层 `openai.go` `toOpenAIMessages` 对任意角色都在 `len(Parts)>0` 时走
    `toOpenAIContentParts`，产出 tool 消息内的 `image_url`）；
  - **无图片** → 维持现状纯文本 `Content`。

### 4.3 事件持久化「不存图片字节」+ 冷恢复重取

- ToolResult 事件里，图片 part **被剥离**（不写 base64，避免事件膨胀 / 敏感落库），
  改写小指针：`"image_refs": [{"workspace_path": "uploads/x.png"}]`（配合
  `identity.RedactSensitive` 之前完成剥离）。
- `eventsAfterInput` 增加一个可注入解析器
  `Engine.ImagePartResolver func(convID, wsPath string) (llm.ContentPart, bool)`
  （由 workspace 服务实现：`blob.Get` 重建 image part）。冷恢复 HITL 重放时，按
  `image_refs` 重新拉 blob 重建 `Parts`。
- 解析器为 nil、convID 缺失或 blob 重取失败 → **降级为文本**
  （`(image available at uploads/x.png — call read_image to view)`），不报错、不崩 run。

### 4.4 新工具 `read_image`

- 注册为第 5 个内置全局工具，`require_approval=false`、`require_login=false`。
- 入参 `path`；`safeRelPath` 校验 → `blob.Get` → `http.DetectContentType` 必须
  以 `image/` 开头（`blob.Get` 不回传 contentType，故嗅探 MIME）。
- 返回 content `{path, content_type, bytes, note:"image attached to this tool result"}`，
  并经 `tool.WithImageParts` 附带图片 part（`ImageBytes` + 嗅探到的 MIME）。
- 上限 **10 MiB**；非图片 / 缺失 / 路径逃逸 / 无 conv → isError。
- **视觉门控**：workspace 服务注入 `visionOK func() bool`（bootstrap 接
  `provider.SupportsVision()`）；非视觉模型调用 → isError「当前模型不支持视觉，
  请切换视觉模型」，对齐上传时的 `vision_unsupported` 检查。

### 4.5 中间件队列

- **无需改动**：用户输入图片已有 data-URI 通道（`job_parts.go`
  `PartsToMiddleware` / `partsFromMiddleware`）；工具结果图片在**出队后**的引擎
  `runLoop` 内产生、直发 LLM，不回穿任务队列；冷恢复靠 §4.3 从 blob 重取。

### 4.6 风险与降级

- 各家大模型对 **tool 角色消息携带 image part** 兼容性不一（OpenAI 兼容编码支持，
  但第三方 / 代理可能拒绝）。V0 以 OpenAI 兼容编码为准并加测试；若实际 provider
  报错或冷恢复重取失败，靠 §4.3 的**文本降级**兜底，run 不崩溃。此项列入联调
  验证清单；若普遍不兼容，后续可改为「下一条 user 消息携带图片」的变体。

### 4.7 测试增量（图片）

- **通道**：假 invoker 经 `tool.WithImageParts` 返回图片 → tool 消息走 `Parts`、
  openai 编码含 `image_url`；事件 content 无 base64、含 `image_refs`。
- **冷恢复**：预置 blob 图片 + 事件 `image_refs` → resolver 重建 Parts；
  resolver 失败 / nil → 文本降级。
- **`read_image`**：上传图片后返回图片 part + 元数据；非图片 / 超 10 MiB /
  非视觉模型 / 路径逃逸 / 无 conv 均 isError。
- **API**：发图片附件 → blob 出现 `workspaces/<conv>/uploads/<img>`；当轮仍
  多模态；下一轮 `read_image` 可查看；落盘失败不阻断发送。
