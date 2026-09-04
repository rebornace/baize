# Agent 会话文件工作区（Agent Workspace Files）v0 设计规格

> 日期：2026-09-04
> 状态：已批准（分节头脑风暴 §1–§3 全部确认）
> 依赖：BlobStore 抽象（`internal/blob`，已交付，含 file/s3 驱动）
> 归属里程碑：Agent 能力增强（非 F 生产硬化）

---

## 1. 目标、范围与整体架构

### 1.1 背景与问题

Baize 的 Agent 目前**没有任何文件读写工具**，且用户上传的聊天附件是 base64
内存即弃：文本在发送当轮被抽取并内联进提示词，图片走当轮多模态注入，原始文件
不落盘。后果：

- 后续轮次 / 新会话中，Agent 无法再读到之前上传的文档；
- Agent 无法把长任务的中间结果落盘、跨轮次反复引用；
- 缺少「持久工作目录 / 长期文件记忆」，处理多文档、长任务时能力受限。

BlobStore 抽象（file/s3）已交付但仅用于分析报告产物（`artifacts/` 前缀）。

### 1.2 目标

为 Agent 提供一个**按会话隔离的持久文件工作区**：

1. 用户上传的**文本类附件持久化**保存，后续轮次 / 新会话可读；
2. Agent 通过内置文件工具在会话工作区内 `list / read / write / delete` 文件，
   跨轮次、跨 run 持久（同一 conversation_id 内）；
3. 存储复用已有 `blob.Store`（file/s3 同一套配置），多副本 / 容器无状态 /
   企业对象存储能力自动继承。

### 1.3 V0 范围

- 给 `blob.Store` 增加枚举能力 `List(ctx, prefix)`，file / memory / s3 三驱动实现。
- 新增 `internal/workspace` 包：基于 `blob.Store` 的会话工作区服务 + 4 个内置工具。
- 文本类附件在发送时写入工作区 `uploads/`；当轮提示注入可用清单。
- bootstrap 用**同一个 blob.Store 实例**装配（artifacts 用 `artifacts/` 前缀、
  workspace 用 `workspaces/` 前缀）。

### 1.4 V0 非目标（明确不做）

- 全局共享区 / 跨会话文件库 / 知识库检索；
- 文件版本历史 / 回收站；
- **图片 / 二进制附件持久化**（图片维持当轮多模态注入，不跨轮次）；
- 文件局部编辑 / patch 工具（`write_file` 为整文件覆盖）；
- UI 文件浏览器 / 文件下载 HTTP 端点（V0 仅 Agent 侧可见，不面向 Chat UI）；
- MCP 子进程 / 连接器与工作区共享 cwd；
- 大对象流式读写（V0 文本小文件，`[]byte`）。

### 1.5 组件与数据流

```
Chat 发送（带文本附件）
  └─ attach.Process 抽取文本
       ├─ 当轮：文本内联进提示词（现状不变）
       └─ 新增：workspace.Service.SaveUpload(convID, filename, text)
                 └─ blob.Put("workspaces/<conv>/uploads/<file>", text)

Agent run（引擎在调工具前已把 conversation_id 注入 ctx，engine.go:821）
  └─ LLM 调用文件工具
       └─ workspace 工具 invoker：identity.ConversationIDFrom(ctx) → convID
            └─ workspace.Service.{List,Read,Write,Delete}File(convID, path)
                 └─ blob.{List,Get,Put,Delete}("workspaces/<conv>/<path>")

装配（bootstrap）
  openBlobStore(cfg) → blobStore（同一个）
       ├─ artifact.NewStore(blobStore, sqlBackend)   // artifacts/ 前缀
       └─ workspace.New(blobStore)                    // workspaces/ 前缀
            ├─ 注册 4 个内置工具到 tool.Registry
            └─ 注入 api.Server 供附件持久化
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

### 2.4 四个内置工具

注册为内置全局工具（与 `create_analysis_page` 同一注册点），
`require_approval=false`、`require_login=false`。

| 工具 | 入参 | 返回（content map） | 说明 |
|---|---|---|---|
| `list_files` | `path?`（默认工作区根） | `{path, entries:[{name, type:"file"\|"dir", size?}]}` | 按 `/` 分段把扁平 key 聚合成一层树形：既有文件、又有以其为前缀的子路径 → 目录 |
| `read_file` | `path` | `{path, content}` | UTF-8 文本；超 64 KiB 截断并附 `…[truncated]`；缺失 → isError not-found |
| `write_file` | `path`, `content` | `{path, bytes}` | 整文件 Put；超 256 KiB 报错；自动建父"目录"（blob 扁平，无需真建） |
| `delete_file` | `path` | `{path, deleted:true}` | Delete 幂等；不存在也返回成功 |

工具描述（写入 LLM spec 的 description）明确：这是**按会话持久**的工作区；
上传的文件在 `uploads/` 下，跨轮次可用；路径相对工作区根，不可用绝对路径/`..`。

`list_files` 的目录聚合：列出 `workspaces/<conv>/<path>/` 前缀下所有 key，
去掉该前缀后按第一段 `/` 分组——第一段后还有内容的为 `dir`，否则为 `file`。

---

## 3. 附件持久化、提示注入、装配、错误处理与测试

### 3.1 附件落工作区（`internal/api`）

- 聊天发送处理器（`server.go` 约 1380–1530，`attach.Process` 之后）：
  对**文本类抽取结果** `texts []attach.Extracted`（含 text/md/csv 原文与
  docx/xlsx/pdf 抽取文本），逐条 `workspace.Service.SaveUpload(convID, ex.Filename, ex.Text)`，
  内容为可读 UTF-8 文本（Agent `read_file` 读到文字而非二进制）。
- 图片类 `images` 不落工作区（维持当轮多模态注入）。
- 写失败**不阻断聊天**（当轮仍内联文本）：`log.Printf` 警告并继续。
- `workspace.Service` 作为新依赖注入 `api.Server`（新字段，与 `Artifacts` 同级），
  由 bootstrap 构造。会话 ID 取发送请求的 `conversation_id`（与 run 归属一致）。

### 3.2 提示注入（让 Agent 知道文件可用）

- **上传当轮**：在该用户消息文本末尾追加一行清单，例如：
  `[工作区] 以下文件已保存，后续可用 read_file 读取：uploads/report.pdf, uploads/data.csv`
  （仅当有文本附件成功保存时追加；图片不列入）。
- **后续轮次**：不自动注入全量清单（避免噪声）；Agent 可随时 `list_files` 自行发现。
  工具 description 已说明 `uploads/` 的存在。

### 3.3 装配（`internal/bootstrap`）

- 复用现有 `blobStore`（`openBlobStore` 产物）：`ws := workspace.New(blobStore)`。
- 注册 4 个工具到 tool registry（`ws.Tools()` 返回 `[]tool.Meta/Invoker` 或在
  bootstrap 逐个 `RegisterSpecApproved(spec, inv, false)`）。
- 把 `ws` 传入 `api.Server`（新字段 `Workspace`）。
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
| 附件落工作区失败 | 记日志警告，不阻断聊天 |
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
- **API**：带文本附件发送后，blob 中存在 `workspaces/<conv>/uploads/<file>`，
  且经工具可读到内容；附件落盘失败不阻断发送（可注入失败 blob 验证）。
- **bootstrap**：装配后 tool registry 含 4 个文件工具、`api.Server` 持有 workspace 服务。

### 3.6 范围边界

不做：全局共享区、版本/回收站、图片二进制持久化、局部编辑、UI 文件浏览器/下载
端点、MCP 共享 cwd、流式大对象。这些进入 backlog 作为后续候选。
