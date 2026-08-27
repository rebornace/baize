# Agent HTML 交互报表 v0 设计规格

> 状态：**已作废** — 由 `2026-08-23-agent-analysis-page-v0-design.md` 取代（固定 bar/line/pie 模板与 AntV MCP 差异不足）  
> 日期：2026-08-23  
> 前置：MCP 桥、Agent Skills、`activate_skill` 已落地  
> 依据：头脑风暴（主路径 HTML 交互报表；静态图仅文档说明 AntV MCP 可选）

---

## 1. 目标与成功标准

**目标：** 对话内可生成**可交互**统计报表（悬停、缩放、图例等），以 Runtime 托管的 HTML 页面呈现；通过内置 Skill 包 + 内置工具接入，不强迫自研 26 种图引擎。

**成功标准：**

1. 内置工具 `create_interactive_report`：接受结构化 `charts[]`（+ 标题/说明），服务端生成含 ECharts 的自包含 HTML，存 artifact
2. `GET /v0/artifacts/{id}` 返回 HTML（`Content-Type: text/html`）；带基础 CSP / 仅本 Run 可读（见 §4）
3. 聊天 UI：工具结果含 `artifact_url` 时，主区展示 **iframe 预览** +「新标签页打开」
4. 内置 Skill `data-analytics`（`examples/skills/data-analytics/`）：流程 + `tools` 含 `create_interactive_report` 与企业读数工具名（mock-ticket 示例）
5. 模型仍可通过 `activate_skill` 激活；**不**默认写入 `agent.skills`（避免收窄工单场景）
6. README 中英：**主路径** HTML 交互报表；**可选** 静态图 via AntV MCP（`@antv/mcp-server-chart`）— 仅文档，不开箱预置
7. 测试：artifact API、HTML 生成、集成 Run 调工具返回 URL、UI 纯函数

**不做（v0）：**

- 模型直接提交任意 HTML（`create_html_report` 开放 HTML 字符串）— token 与安全留 v1
- Composer「+」Skill 多选、对话附件 CSV（可 v0.1 同批或下一里程碑）
- 自研 AntV 级 26 种图；v0 支持 bar/line/pie + 简单 table，schema 可扩展
- 聊天内嵌远程 AntV **图片 URL** 的专用渲染（用户走 MCP 时仍见 JSON；文档说明即可）

---

## 2. 产品路径

```
用户 / 模型
  → 调企业 Connector 工具拿 JSON
  → create_interactive_report({ title, charts: [...] })
  → Runtime 写 HTML artifact
  → 返回 { artifact_id, artifact_url, kind: "html_report" }
  → 聊天 iframe 预览 + 可新开标签页
```

**静态图（可选，仅文档）：** 管理员自注册 MCP `npx -y @antv/mcp-server-chart`，配 Skill 或 `activate_skill`；返回**图片 URL**，非本规格主路径。

---

## 3. 内置工具 `create_interactive_report`

### 3.1 注册

- 与 `activate_skill` 类似：**内置 Registry 工具**，非 Connector 目录行
- 当 `skills` 目录非空或始终注册（实现选后者更简单，工具描述标明「用于数据分析 Skill」）
- 不需 HITL；不需 `require_login`

### 3.2 请求 schema（摘要）

```json
{
  "title": "本月工单统计",
  "description": "可选说明",
  "charts": [
    {
      "id": "by_status",
      "title": "按状态",
      "type": "bar",
      "labels": ["open", "closed"],
      "series": [{ "name": "数量", "values": [12, 8] }]
    },
    {
      "id": "trend",
      "title": "近 7 日",
      "type": "line",
      "labels": ["Mon", "Tue"],
      "series": [{ "name": "新建", "values": [3, 5] }]
    }
  ]
}
```

| `type` v0 | 说明 |
|-----------|------|
| `bar` | 柱状 |
| `line` | 折线 |
| `pie` | 饼图（单 series） |
| `table` | 简单 HTML 表（labels + 一行或多行 series） |

未知 `type` → tool `is_error`。

### 3.3 响应

```json
{
  "artifact_id": "art_...",
  "artifact_url": "/v0/artifacts/art_...",
  "kind": "html_report",
  "chart_count": 2
}
```

不把 HTML 正文塞回模型上下文（省 token）。

### 3.4 HTML 生成

- 包 `internal/report`（或 `internal/artifact`）：模板 + 内联 ECharts（CDN 或 embed 静态 JS，实现计划选定；企业内网优先 **embed**，避免外网 CDN）
- 自包含单页：多图垂直排列；ECharts `dataZoom` / tooltip 启用（交互）
- 禁止 artifact 内脚本访问 Baize API / cookie（iframe `sandbox` 见 §5）

---

## 4. Artifact 存储与 API

| 项 | 约定 |
|----|------|
| 路径 | `{data_dir}/artifacts/{id}.html` 或 SQLite blob；id 随机 |
| 关联 | 创建时记录 `run_id`（用于鉴权） |
| GET | `/v0/artifacts/{id}` — 同控制面门闸；校验请求者能否读该 `run_id`（admin 或持有 conversation 的会话；v0 可简化为 **仅需控制面 token + run 存在**） |
| 响应头 | `Content-Type: text/html; charset=utf-8`；`Cache-Control: private` |
| 生命周期 | v0 不清理；可后续按 run 清理 |

---

## 5. 聊天 UI

- `ToolCard` 或独立 `ReportPreview`：若 `result.artifact_url` 且 `kind === html_report`：
  - `<iframe sandbox="allow-scripts" src={artifact_url} />`（高度可调，默认 360px）
  - 链接「在新标签页打开」
- 助手纯文本气泡仍只显示文字；报表以工具卡片/嵌入区呈现
- 不解析模型回复里的 raw HTML

---

## 6. Skill `data-analytics`

```markdown
---
name: data-analytics
description: 多源数据统计与 HTML 交互报表
tools:
  - list_tickets
  - get_ticket
  - create_interactive_report
---

# 数据分析

1. 从企业工具拉取 JSON，在模型侧重聚合（不要整包塞进 report 参数）
2. 调用 create_interactive_report，charts 每项一种视图
3. 需要静态 PNG 时，可由管理员配置 AntV MCP（见 README）
```

- 落盘：`examples/skills/data-analytics/`
- **不**写入默认 `agent.skills`；用户或模型 `activate_skill` 后启用
- `configs/demo.yaml` 可不挂；文档说明即可

---

## 7. 文档（README）

新增小节 **「数据分析与报表」**：

- **推荐：** `activate_skill` / 设置勾选 `data-analytics` → `create_interactive_report` → 对话内交互 HTML
- **可选静态图：** 设置 → MCP 注册 `@antv/mcp-server-chart`，返回图片 URL；适合幻灯片/文档嵌入，交互弱于 HTML 报表

---

## 8. 实现组件

| 组件 | 职责 |
|------|------|
| `internal/report` | 结构化 charts → HTML |
| `internal/artifact` | 存取、ID、run 关联 |
| `internal/api` | GET artifacts；内置 tool 注册入口 |
| `internal/run` 或 `internal/tool/builtin` | `create_interactive_report` invoker |
| `web/chat` | iframe 预览、`artifact_url` 解析 |
| `examples/skills/data-analytics` | 内置 Skill 包 |

---

## 9. 测试

- `internal/report`：bar/line/pie/table HTML 含 ECharts option
- artifact round-trip GET
- 集成：mock LLM 调 `create_interactive_report` → GET 200 HTML
- `ReportPreview.test.ts`：从 tool result 解析 artifact_url

---

## 10. 与「+」Skill 选择器

本规格 **不强制** 同里程碑交付 Composer「+」多选；若并行可做 v0.1：`POST /v0/runs` 的 `skills[]` + 启用「+」。HTML 报表不依赖「+」，依赖 `activate_skill` 或设置页默认 Agent skills。

---

*批准后分支 `feat/html-interactive-report` + 实现计划。*
