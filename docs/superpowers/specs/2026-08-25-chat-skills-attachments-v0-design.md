# Chat Skill 符号调用与多类型附件 v0.1 设计规格

> 状态：已批准（2026-08-25）  
> 日期：2026-08-25  
> 前置：Agent Skills、`activate_skill`、分析页、会话 Token、Chat UI 已落地  
> 依据：分析页规格 defer 项（Composer Skill / 附件）；头脑风暴选定 B 路线并修订  
> 分支建议：`feat/chat-skills-attachments`

---

## 1. 目标与成功标准

**目标：** 操作员在 Chat 中用 `@技能` / `/技能` 激活本 Run 的 Skill，并可上传常用文件；文档类抽取为文本注入上下文，图片在模型支持 vision 时多模态送入，不支持则硬失败，避免无效 Run 浪费 Token。

**一句话成功标准：** 输入 `@data-analytics` 并附带 xlsx/pdf（或支持 vision 时的 png）后发送 → Skill 按请求激活；文档内容进入本轮 user 上下文；图片在 `supports_vision=false` 时返回明确错误且不创建 Run。

### 做

| 层 | 内容 |
|----|------|
| Skill | 解析 `@id` / `/id`（等价）；覆盖本 Run `agent.skills`；未知 id → `400` |
| API | `POST /v0/runs` 支持 `skills[]`、`attachments[]`（见 §3） |
| 附件 | `.txt` `.md` `.csv` 原文；`.docx` `.xlsx` `.pdf` 抽文本；图片见 §4 |
| Vision | `llm.supports_vision`；有图且为 false → `400 vision_unsupported`，不启动 Run |
| ACL | `GET /v0/skills` 对 Operator 可读（补全）；写 Skill 仍 Admin |
| UI | 纸夹选文件；`@`/`/` 补全；错误码可读提示；气泡显示附件文件名 |

### 不做

- 独立 Files API / `file_id` 跨 Run 复用
- PDF 页渲染成图 + vision（本里程碑仅文本层抽取）
- `.xls` 老格式、加密 Office、任意可执行文件
- 静默「跳过图片继续跑」
- 会话级默认勾选 Skill 持久化
- 服务端自动调用 `create_analysis_page`（仍由模型决定）

---

## 2. Skill 符号调用

### 2.1 语法

- `@<skill_id>` 与 `/<skill_id>` 等价
- `skill_id`：与目录 id 一致（如 `data-analytics`）；字符集建议 `[a-zA-Z0-9][a-zA-Z0-9_-]*`
- 一条消息可多个；去重保序
- 解析后从 `input` **剥离**这些标记（及紧邻多余空白），剩余为真正用户文案

### 2.2 与 `skills` 字段

- 省略 `skills` 且正文无标记 → 使用 Agent 配置的默认 `skills`（现行为）
- 正文解析出 id，和/或 body.`skills` 非空 → **合并去重**后作为本 Run 激活列表（**覆盖** Agent 默认，不是追加到默认之上）
- 任一 id 不在目录 → `400 unknown_skill`，message 列出未知 id

### 2.3 引擎

- 在 `beginRunSkills(runID, resolvedSkills, system)` 使用解析结果
- 中途 `activate_skill` 仍可用（在已激活集合上追加）

### 2.4 UI

- 输入 `@` 或 `/` 时展示 Skill 补全（`GET /v0/skills`）
- 不强制「+」多选面板；纸夹仅用于附件

---

## 3. `POST /v0/runs` 附件 API

### 3.1 请求（JSON 主路径）

```json
{
  "agent_id": "…",
  "conversation_id": "…",
  "input": "@data-analytics 根据附件做看板",
  "skills": ["data-analytics"],
  "attachments": [
    {
      "filename": "pets.xlsx",
      "media_type": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
      "content_base64": "…"
    }
  ],
  "session_token": "…",
  "webhook_url": "…"
}
```

| 字段 | 说明 |
|------|------|
| `skills` | 可选；与正文 `@`/`/` 合并 |
| `attachments` | 可选；见下表 |

### 3.2 支持类型

| 扩展名 | media_type（示例） | 处理 |
|--------|-------------------|------|
| `.txt` `.md` `.csv` | `text/plain` 等 | UTF-8 文本；解码失败 → `400` |
| `.docx` | OOXML word | 抽段落为文本/Markdown |
| `.xlsx` | OOXML sheet | 抽表为 Markdown 或 CSV 文本 |
| `.pdf` | `application/pdf` | **仅抽文本层**；无可用文本 → `400 empty_pdf_text` |
| `.png` `.jpg` `.jpeg` `.webp` `.gif` | `image/*` | 见 §4 |

其它扩展名 → `400 unsupported_attachment`。

### 3.3 限额（默认，可后续配置化）

- 单次最多 **5** 个附件
- 解码后合计 ≤ **8 MiB**
- 单文件抽取文本 ≤ **64 KiB** 字符；超出截断并标注 `…[truncated]`
- 图片送模型前长边 ≤ **2048px**（仅 `supports_vision=true` 时）

### 3.4 注入与落库

- **文本类：** 拼入本轮送给 LLM 的 user 内容，例如：

```
【附件: pets.xlsx】
| A | B |
...
```

- **图片：** 仅在当次 `Execute` 的 `llm.Message` Parts 中携带；**不**写入 SQLite `messages.content` 的 base64
- **落库气泡：** 用户可见原文（已剥 skill 标记）+ 附件文件名列表（如 `（附件：a.xlsx, b.png）`）

---

## 4. Vision 与错误码

### 4.1 配置

```yaml
llm:
  supports_vision: false   # 默认 false；换支持识图的模型时显式 true
```

`GET /v0/ui-config`（或等价）回显 `supports_vision`，供前端发送前拦截。

### 4.2 规则

| 条件 | 行为 |
|------|------|
| 含图片且 `supports_vision=false` | **不创建 Run**；`400` + `vision_unsupported`：说明换模型或去掉图片 |
| 含图片且 `supports_vision=true` | 缩略后作为 multimodal image part |
| 仅文档 | 不依赖 vision |

**禁止**把图片降级为「仅文件名」后继续执行。

### 4.3 其它错误码（摘要）

| code | 场景 |
|------|------|
| `unknown_skill` | skill id 不在目录 |
| `unsupported_attachment` | 类型不允许 |
| `attachment_too_large` / `too_many_attachments` | 超限 |
| `empty_pdf_text` | PDF 无文本层 |
| `vision_unsupported` | 有图但模型不支持识图 |
| `invalid_attachment` | base64/编码损坏 |

---

## 5. LLM 层改动要点

- `llm.Message`：可选 `Parts []ContentPart`（`type: text | image_base64`）
- `openai_compatible`：存在 image part 时 `content` 使用数组形式
- `Mock`：可配置接受/拒绝含图请求，便于单测
- 历史轮次重建：落库无图字节 → 多轮后图片**仅存在于附带图的那一轮**；若产品需要「图一直留在上下文」，列为 v0.2（需持久化或 Files）

---

## 6. UI

- Composer：附件按钮；chip 可移除；`@`/`/` 补全
- 发送：组 `attachments` + `input`；若本地有图且 `supports_vision=false`，前端可先提示（服务端仍为权威）
- 错误：展示 `error.message`，不插入成功态助手气泡

---

## 7. 测试

1. Skill：多 `@`/`/`、剥离、未知 id、覆盖默认 skills  
2. 附件：md/csv/docx/xlsx/pdf 抽取进 LLM 消息；非法类型 400  
3. PDF 无文本 → `empty_pdf_text`  
4. 有图 + `supports_vision=false` → 400 且无 Run  
5. 有图 + true → OpenAI 请求含 image part（假上游）  
6. UI：解析/错误展示轻测；`npm run build` + dist  

---

## 8. 文档

- README 中英：Skill 符号、支持的附件类型、`supports_vision`、硬失败语义  
- 分析页规格 defer 项可标注「见本规格 v0.1」

---

## 9. 非目标回顾

侧车 `callback_urls`、Connector DELETE、Webhook 重试、SDK、OTel、工作流 DSL、PDF 页图 vision —— 均不在本里程碑。

---

*请审查本规格。批准后进入 `writing-plans` 实现计划，再按 subagent-driven-development 实现。*
