# Chat Skill 符号调用与多类型附件 v0.1 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 实现 `@`/`/` Skill 覆盖本 Run、多类型附件抽取注入上下文，以及图片在无 vision 时硬失败。

**架构：** 纯函数包解析 Skill 与抽取附件 → `POST /v0/runs` 校验后拼装 user 文本/Parts → Engine `beginRunSkills` 用解析结果；OpenAI Provider 支持 multimodal content；Chat Composer 补全与附件上传。

**技术栈：** Go 1.22+、`excelize`（xlsx）、轻量 docx/pdf 文本抽取库、标准 `image` 缩略、Vite/React。

**规格：** `docs/superpowers/specs/2026-08-25-chat-skills-attachments-v0-design.md`（已批准）  
**分支：** `feat/chat-skills-attachments`  
**全局约束：** commit 中文 Conventional；改 UI 后 `npm test` + `npm run build` 并提交 `internal/ui/dist/**`。

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `internal/skillparse/parse.go` | 从 input 解析 `@`/`/` skill id，返回 cleaned input + ids |
| `internal/skillparse/parse_test.go` | 解析单测 |
| `internal/attach/extract.go` | 按类型解码 base64、抽文本、缩略图；限额与错误码 |
| `internal/attach/extract_test.go` | 各类型与错误路径 |
| `internal/attach/testdata/*` | 最小 md/csv/docx/xlsx/pdf/png fixture |
| `internal/llm/provider.go` | `ContentPart`、`Message.Parts`、`SupportsVision()` |
| `internal/llm/openai.go` | multimodal `content` 数组编码 |
| `internal/llm/openai_test.go` | 含图请求体断言 |
| `internal/llm/mock.go` | 可拒绝/接受含图 |
| `internal/config/config.go` | `LLM.SupportsVision` |
| `internal/bootstrap/bootstrap.go` | 注入 vision 标志到 API/LLM |
| `internal/api/server.go` | PostRun 解析 skills/attachments；ui-config；ACL skills GET |
| `internal/api/server_attachments_test.go` | API 400/成功路径 |
| `internal/controlplane/acl.go` | `GET /v0/skills` → Operator |
| `internal/run/engine.go` | Execute 接受 per-run skills + Parts；buildMessages |
| `internal/run/skills_overlay.go` | 若需：显式 resolved skills 入口 |
| `web/chat/src/skillMention.ts` | 前端预览解析（可选与后端一致） |
| `web/chat/src/api.ts` | createRun attachments；ui-config.supports_vision；listSkills operator |
| `web/chat/src/components/Composer.tsx` | 附件 + @/ 补全 |
| `web/chat/src/pages/ChatPage.tsx` | 传 attachments；错误展示 |
| `configs/minimal.yaml` / `demo.yaml` / example | `supports_vision: false` |
| `README.md` / `README.zh-CN.md` | 用法说明 |

**依赖（任务内 `go get`）：**

- `github.com/xuri/excelize/v2` — xlsx  
- `github.com/ledongthuc/pdf` — pdf 文本（或等价维护中的抽取库；选定后在任务中写死 import）  
- docx：优先 `archive/zip` + 读 `word/document.xml` 抽文本（少依赖）；若过繁再用成熟库  

---

### 任务 1：Skill 输入解析

**文件：**
- 创建：`internal/skillparse/parse.go`
- 创建：`internal/skillparse/parse_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
func TestParseMentions(t *testing.T) {
	cleaned, ids := skillparse.Parse(`请用 @data-analytics 和 /ticket-triage 分析`)
	if cleaned != "请用  和  分析" && !strings.Contains(cleaned, "分析") {
		// 精确期望：剥标记后规范化空白为单空格
	}
	// 期望 ids == []string{"data-analytics", "ticket-triage"}
}
func TestParseNoMention(t *testing.T) {
	cleaned, ids := skillparse.Parse("普通问题")
	if cleaned != "普通问题" || len(ids) != 0 {
		t.Fatal(cleaned, ids)
	}
}
```

期望 cleaned 精确为：`请用 和 分析`（连续空白压成单空格）或规格约定的规范化；在实现里固定一种并写进测试。

- [ ] **步骤 2：** `go test ./internal/skillparse -count=1` → FAIL（包不存在）

- [ ] **步骤 3：实现** `Parse(input string) (cleaned string, ids []string)`  
  - 正则：`(^|[\s])[@/]([a-zA-Z0-9][a-zA-Z0-9_-]*)\b`  
  - 去重保序；返回 cleaned

- [ ] **步骤 4：** 测试 PASS

- [ ] **步骤 5：Commit** `feat(skillparse): 解析 @/ Skill 提及`

---

### 任务 2：附件抽取包

**文件：**
- 创建：`internal/attach/*.go` + `testdata/`

- [ ] **步骤 1：失败测试**

覆盖：
- text/md/csv round-trip  
- 非法 media → `ErrUnsupported`（映射 API `unsupported_attachment`）  
- 超限 `ErrTooLarge` / `ErrTooMany`  
- 空 PDF → `ErrEmptyPDFText`  
- png → `KindImage` + 缩略后非空 bytes  

```go
type AttachmentIn struct {
	Filename   string
	MediaType  string
	ContentB64 string
}

type Extracted struct {
	Filename string
	Kind     string // "text" | "image"
	Text     string // Kind=text
	ImageMIME string
	ImageBytes []byte // Kind=image，已缩略
}

func Process(atts []AttachmentIn, opts Options) (texts []Extracted, images []Extracted, err error)
```

- [ ] **步骤 2：** FAIL

- [ ] **步骤 3：实现抽取**  
  - Options：MaxCount=5, MaxTotalBytes=8<<20, MaxTextChars=64<<10, MaxImageEdge=2048  
  - xlsx：excelize 读首表或全部表拼 Markdown  
  - docx：zip 读 document.xml 去标签  
  - pdf：抽文本；trim 空 → ErrEmptyPDFText  
  - image：decode → resize → encode jpeg/png

- [ ] **步骤 4：** `go test ./internal/attach -count=1` PASS

- [ ] **步骤 5：Commit** `feat(attach): 多类型附件抽取与图片缩略`

---

### 任务 3：LLM multimodal + supports_vision 配置

**文件：**
- 修改：`internal/llm/provider.go`、`openai.go`、`mock.go`
- 修改：`internal/config/config.go`、`applyDefaults`
- 修改：`internal/bootstrap`（NewOpenAI 后设置；或 Engine/Server 持有 flag）
- 测试：`openai` 请求 JSON 含 `image_url` 或 `image_url.url` data URI

- [ ] **步骤 1：失败测试** — Message 带 Parts image → `toOpenAIMessages` 的 content 为数组

- [ ] **步骤 2：实现 ContentPart；OpenAI 编码；Config.LLM.SupportsVision；默认 false**

- [ ] **步骤 3：** Provider 增加 `SupportsVision() bool`（OpenAI 读配置字段；Mock 可设）

- [ ] **步骤 4：** 测试 PASS；Commit `feat(llm): multimodal Parts 与 supports_vision`

---

### 任务 4：API PostRun 接线 + ACL + ui-config

**文件：**
- 修改：`internal/api/server.go`（body、`createAndExecuteRun` 传 skills/parts）
- 修改：`internal/controlplane/acl.go` + `acl_test.go`：`GET /v0/skills` → `RoleOperator`
- 修改：`handleUIConfig` 增加 `supports_vision`
- 修改：`internal/run/engine.go`：`Execute` 或新方法接受 `RunOptions{Skills []string, UserParts []llm.ContentPart}`  
  - 若 `len(Skills)>0` 或显式空切片语义：规格为「有解析结果则覆盖」；**无提及且 omit skills → 用 agent.skills**；**解析/body 得到非空列表 → 覆盖**；**body `skills: []` 空数组视为显式清空激活（仅 activate_skill 工具）**——与规格「覆盖」一致：空数组 = 本 Run 无默认 skill。
- 测试：`server_attachments_test.go`

- [ ] **步骤 1：失败测试**

```go
// 未知 skill → 400 unknown_skill
// 有图 + server SupportsVision=false → 400 vision_unsupported，Store 无新 run
// 仅 md 附件 → 200，且 Messages 含附件文件名；LLM mock 收到含【附件:】的文本
```

- [ ] **步骤 2：实现**  
  1. Decode attachments → `attach.Process`  
  2. 若 images 非空 && !SupportsVision → writeError vision_unsupported  
  3. `skillparse.Parse` + merge body.skills → validate catalog → beginRunSkills  
  4. 拼 user content；Parts = text + images  
  5. Append 落库文本（含附件文件名行）  
  6. ui-config: `supports_vision`

- [ ] **步骤 3：** 测试 PASS

- [ ] **步骤 4：Commit** `feat(api): runs 支持 skills 与附件及 vision 硬失败`

---

### 任务 5：Chat UI

**文件：**
- `web/chat/src/skillMention.ts` + test  
- `api.ts`：`CreateRunOptions.attachments`、`skills`；`UIConfig.supports_vision`；`listSkills` 在 operator 角色可用  
- `Composer.tsx`：file input、chips、@ / 补全  
- `ChatPage.tsx`：onSend(text, files)；处理 vision_unsupported 文案  
- `style.css` 少量样式  
- `npm test`；`npm run build`；提交 dist

- [ ] **步骤 1：** `skillMention.test.ts` 与后端规则对齐的轻量解析（仅补全高亮，权威仍在服务端）

- [ ] **步骤 2：** Composer UI + createRun 传 base64

- [ ] **步骤 3：** 前端若 `supports_vision===false` 且选了图片 → 发送前 `setError`，不调用 API

- [ ] **步骤 4：** `npm test`；`npm run build`

- [ ] **步骤 5：Commit** `feat(ui): Composer Skill 补全与多类型附件`

---

### 任务 6：配置示例、README、规格交叉引用

- [ ] `configs/*.yaml` / `default.local.yaml.example` 增加注释行 `supports_vision: false`
- [ ] README 中英：§ Skill 符号、附件类型、vision 硬失败
- [ ] 分析页规格 defer 行改为指向本规格
- [ ] `go test ./... -count=1`（或至少 attach/skillparse/api/llm/run）
- [ ] Commit `docs: Chat Skill 符号与附件说明`

---

### 任务 7：收尾

- [ ] 按 `finishing-a-development-branch`：测绿、合并选项  
- [ ] 双仓推送按 `dual-remote.md`（用户要求时再推）

---

## 自检

| 规格章节 | 任务 |
|----------|------|
| §2 Skill 符号 | 1、4、5 |
| §3 附件 API / 类型 / 限额 / PDF | 2、4 |
| §4 Vision 硬失败 | 3、4、5 |
| §5 LLM Parts | 3 |
| §6 UI | 5 |
| §7 测试 | 各任务内 |
| §8 文档 | 6 |

**空 skills 数组语义：** 任务 4 已写死。  
**无占位符 TODO。**

---

*计划就绪后选执行方式：子代理驱动 / 本会话内联。*
