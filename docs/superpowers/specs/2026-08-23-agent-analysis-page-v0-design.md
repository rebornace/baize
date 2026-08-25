# Agent 统计分析页 v0 设计规格

> 状态：已批准（2026-08-23）  
> 日期：2026-08-23  
> 前置：MCP 桥、Agent Skills、`activate_skill` 已落地  
> 取代：`2026-08-23-agent-html-interactive-report-v0-design.md`（固定 bar/line/pie 模板方案作废）  
> 依据：头脑风暴确认 — 主路径 section + 开放 ECharts option（省 token）；v1 含整页 HTML 逃逸；企业卖点含筛选、下钻、导出 PDF

---

## 1. 目标与成功标准

**目标：** 对话内生成**完整可交互的统计分析网页**（叙事、指标、多图、表、筛选、下钻、导出），由 Runtime 托管 HTML artifact，聊天 iframe 预览；**不**锁死几种图模板。

**成功标准：**

1. 内置工具 `create_analysis_page`：
   - **主路径 `format: sections`**：`datasets` + `filters` + `sections[]`（markdown / kpi / table / echarts）
   - **逃逸 `format: html`**：整页 HTML 字符串，版式完全自由
2. ECharts 区块支持 **完整 `option` JSON**（不限图种）；可选 **`binding` 简写**（从 dataset 生成 option，省 token）
3. 页面壳（非内容模板）统一提供：**筛选栏**、**导出 PDF**、多区块排版；筛选变更后绑定区块自动刷新
4. **下钻**：图表点击 → 同步全局筛选或打开明细表（见 §3.5）
5. `GET /v0/artifacts/{id}` 返回 HTML；鉴权绑定 `run_id`；iframe `sandbox`
6. 聊天 UI：工具结果含 `artifact_url` + `kind: analysis_page` → iframe 预览 + 新标签打开
7. 内置 Skill `data-analytics`（`examples/skills/data-analytics/`）
8. README：**主路径**本工具；**可选** AntV MCP 静态 PNG（文档说明，非开箱）
9. 测试：report 编译、filter/drilldown 运行时、artifact GET、集成 Run、UI 解析 `artifact_url`

**不做（本里程碑）：**

- 自研 26 种图引擎或 AntV 级 MCP 替代
- 筛选触发 **回连 Baize API / 企业 Connector** 拉新数据（v0 仅 **页内 datasets 客户端过滤/聚合**；再查数由 Agent 新 Run 调工具）
- Composer「+」Skill 多选、对话附件 CSV — **见** [Chat Skill 符号与附件 v0.1](2026-08-25-chat-skills-attachments-v0-design.md)（`@`/`/` 符号、多类型附件、vision 硬失败）
- 服务端 headless Chrome 出 PDF（v0 用浏览器打印 / 客户端导出，见 §3.6）
- 聊天内嵌 AntV **远程图片 URL** 专用组件（MCP 仍走 JSON 工具卡）

---

## 2. 产品路径

```
用户 / 模型
  → 调企业 Connector 工具聚合 JSON
  → create_analysis_page({ format: "sections", datasets, filters, sections })
     或 create_analysis_page({ format: "html", html })
  → Runtime 编译/存 HTML artifact
  → 返回 { artifact_id, artifact_url, kind: "analysis_page" }
  → 聊天 iframe + 筛选/下钻/导出 PDF 在页内交互
```

**与 AntV MCP 的差异（README 须写清）：**

| | `create_analysis_page` | AntV MCP |
|--|------------------------|----------|
| 产物 | **整页分析站**（多区块 + 筛选 + 下钻） | 多为 **单图** + 图片 URL |
| 叙事 | markdown / KPI / 表与图混排 | 无 |
| 托管 | Baize artifact + 对话嵌入 | 外部图床 URL |
| 灵活性 | sections + 完整 ECharts option；或整页 HTML | 26 个 `generate_*` 枚举 |

---

## 3. 内置工具 `create_analysis_page`

### 3.1 注册

- 内置 Registry 工具（同 `activate_skill`）
- 始终注册；描述标明用于数据分析 Skill
- 不需 HITL

### 3.2 请求 schema（摘要）

**公共字段：**

```json
{
  "title": "Q3 工单与 SLA 分析",
  "format": "sections",
  "theme": "light"
}
```

**`format: sections`（主路径，推荐）：**

```json
{
  "format": "sections",
  "datasets": {
    "tickets": {
      "columns": ["id", "status", "priority", "created_at"],
      "rows": [
        ["T1", "open", "high", "2026-08-01"],
        ["T2", "closed", "low", "2026-08-02"]
      ]
    },
    "by_status": {
      "columns": ["status", "count"],
      "rows": [["open", 12], ["closed", 8]]
    }
  },
  "filters": [
    {
      "id": "status",
      "type": "select",
      "label": "状态",
      "dataset": "tickets",
      "field": "status",
      "options": ["open", "closed"],
      "default": ""
    },
    {
      "id": "date_range",
      "type": "date_range",
      "label": "创建时间",
      "dataset": "tickets",
      "field": "created_at"
    }
  ],
  "sections": [
    {
      "type": "markdown",
      "content": "## 结论\n本周关闭率上升…"
    },
    {
      "type": "kpi",
      "items": [
        { "label": "总工单", "binding": { "dataset": "tickets", "aggregate": "count" } },
        { "label": "开放", "binding": { "dataset": "tickets", "aggregate": "count", "where": { "status": "open" } } }
      ]
    },
    {
      "type": "echarts",
      "id": "chart_status",
      "title": "按状态",
      "binding": {
        "dataset": "tickets",
        "chart": "bar",
        "category": "status",
        "value": { "aggregate": "count" }
      },
      "drilldown": {
        "on": "click",
        "action": "set_filter",
        "filter_id": "status",
        "value_from": "category"
      }
    },
    {
      "type": "table",
      "title": "明细",
      "binding": { "dataset": "tickets", "columns": ["id", "status", "priority"] }
    },
    {
      "type": "echarts",
      "id": "chart_custom",
      "title": "自定义",
      "option": {
        "xAxis": { "type": "category", "data": ["A", "B"] },
        "yAxis": { "type": "value" },
        "series": [{ "type": "line", "data": [1, 2] }]
      }
    }
  ]
}
```

**`format: html`（v1 逃逸，版式完全自由）：**

```json
{
  "format": "html",
  "html": "<!DOCTYPE html>…完整自包含页…"
}
```

| 约束 | 说明 |
|------|------|
| 大小 | `html` ≤ **512 KiB**；超限 `is_error` |
| 脚本 | 允许 `<script>`（页内交互）；iframe `sandbox="allow-scripts"`，**无** `allow-same-origin` |
| 外链 | 默认禁止加载外部脚本（仅允许内联 + 页壳注入的 ECharts/PDF 库）；`html` 模式文档说明风险 |
| 注入壳 | 若 `html` 仅为 `<body>` 片段，服务端可包一层最小 document + CSP |

**`sections` 与 `html` 互斥**；同时出现 → `is_error`。

### 3.3 Section 类型（v0）

| `type` | 说明 |
|--------|------|
| `markdown` | 分析叙事（服务端转安全 HTML 或 marked 渲染，禁止 raw script） |
| `kpi` | 指标卡；`items[]` 支持静态 `value` 或 `binding` 聚合 |
| `table` | 数据表；`binding` 或静态 `columns`/`rows` |
| `echarts` | **二选一**：`option`（完整 ECharts）或 `binding`（简写） |
| `row` | 可选；子 `sections` 横向排列（实现计划定栅格） |

未知 `type` → `is_error`。

### 3.4 Binding 简写（省 token）

`binding` 在客户端由 **页内运行时**（`report-runtime.js`）对 `datasets` 执行：

- `filter`：应用当前 `filters` 状态
- `aggregate`：`count` \| `sum` \| `avg`（`sum`/`avg` 需 `field`）
- `groupBy`：隐含于 `category` + `value` 配置
- 输出 ECharts option 或表行

模型复杂版式仍用 **`option`** 直传；简单图用 **`binding`**，减少 token。

### 3.5 筛选（企业卖点）

- 筛选栏由 `filters[]` 自动生成，位于页眉下
- 变更筛选 → 重算所有带 `binding` 的 section；带静态 `option` 的 echarts **不**自动重算（模型应用 binding 或自写 html）
- `default: ""` 表示「全部」
- 筛选仅作用于声明的 `dataset` 行（客户端）

### 3.6 下钻（企业卖点）

`drilldown` 挂在 `echarts` section：

| `action` | 行为 |
|----------|------|
| `set_filter` | 点击扇区/柱/点 → 设置 `filter_id` 取值（`value_from`: `category` \| `seriesName` \| `name`） |
| `detail` | 点击 → 打开侧栏/模态 **明细表**，`match`: `{ field, from }` 过滤 `dataset` |
| `goto_section` | 点击 → 滚动到 `target_section_id` 并可选 `set_filter` |

同一页可多个图表联动同一 `filter_id`（全局状态）。

### 3.7 导出 PDF（企业卖点）

页壳按钮 **「导出 PDF」**（sections 与 html 模式均注入，除非 `html` 整页已含同名控件且 `export_pdf: false`）：

- **v0 实现**：`window.print()` + `@media print` 样式（隐藏筛选栏按钮、分页友好）
- **增强（同里程碑可选）**：内联 **html2canvas + jsPDF**（embed 静态 JS，无 CDN）一键下载 PDF
- 不把 PDF 文件存为新 artifact（v0）；用户本地保存

### 3.8 响应

```json
{
  "artifact_id": "art_...",
  "artifact_url": "/v0/artifacts/art_...",
  "kind": "analysis_page",
  "format": "sections",
  "section_count": 5
}
```

**不把 HTML 正文回传模型**（省 token）。

### 3.9 校验与错误

- `datasets` 行宽须与 `columns` 一致
- `binding.dataset` / `filters[].dataset` 须存在于 `datasets`
- `echarts` 须含 `option` 或 `binding`
- 工具失败 → `is_error` + 简短原因，Run 不失败

---

## 4. HTML 生成与页内运行时

| 模式 | 生成方式 |
|------|----------|
| `sections` | Go `internal/report`：页壳模板 + 内联 `window.__BAIZE_PAGE__` JSON + embed ECharts + `report-runtime.js` |
| `html` | 原样或包 document；同样注入 PDF 按钮脚本（可配置） |

- **页壳是模板，内容不是**：壳提供布局、主题 CSS、筛选栏、PDF 按钮；**不**限制图种
- ECharts：**embed 静态 JS**（企业内网无外网 CDN）
- `report-runtime.js`：filter 状态、binding 编译、drilldown、echarts init/update

---

## 5. Artifact 存储与 API

| 项 | 约定 |
|----|------|
| 存储 | `{data_dir}/artifacts/{id}.html` 或 SQLite blob |
| 关联 | `run_id`、可选 `conversation_id` |
| GET | `/v0/artifacts/{id}` — 控制面 token；校验可读该 run |
| 响应头 | `Content-Type: text/html; charset=utf-8`；`Content-Security-Policy: default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'`（实现计划细化） |
| 生命周期 | v0 不自动清理 |

---

## 6. 聊天 UI

- 工具卡 / `AnalysisPagePreview`：`result.kind === "analysis_page"` 且 `artifact_url` 存在时：
  - `<iframe sandbox="allow-scripts" src={artifact_url} />`（默认高 480px，可拖拽调高）
  - 「在新标签页打开」
- 助手气泡仍纯文本；分析页在工具结果区展示
- 不解析模型回复中的 raw HTML

---

## 7. Skill `data-analytics`

```markdown
---
name: data-analytics
description: 多源数据统计与完整交互分析页（筛选、下钻、导出 PDF）
tools:
  - list_tickets
  - get_ticket
  - create_analysis_page
---

# 数据分析

1. 从企业工具拉 JSON，在模型侧重聚合为 `datasets`（勿把原始大 JSON 塞进单个 section）
2. 优先 `format: sections` + `binding` 省 token；复杂图用 `echarts.option`
3. 需要版式完全自由时用 `format: html`
4. 需要静态 PNG 时由管理员配置 AntV MCP（见 README）
```

- 落盘：`examples/skills/data-analytics/`
- **不**写入默认 `agent.skills`；`activate_skill` 或设置页勾选启用

---

## 8. 文档（README）

新增 **「数据分析与报表」**：

- **推荐：** `create_analysis_page` → 完整交互分析页（筛选 / 下钻 / PDF）
- **Token：** sections + binding 为主；整页 html 备选
- **可选：** AntV MCP → 单图 PNG URL，适合 Office，非完整分析站

---

## 9. 实现组件

| 组件 | 职责 |
|------|------|
| `internal/report` | sections → HTML；html 逃逸校验与包装 |
| `internal/report/runtime.js` | 筛选、binding、下钻、ECharts 更新 |
| `internal/artifact` | 存取、ID、run 关联 |
| `internal/api` | GET artifacts；内置 tool 注册 |
| `internal/tool/builtin` | `create_analysis_page` invoker |
| `web/chat` | iframe 预览、`artifact_url` 解析 |
| `examples/skills/data-analytics` | Skill 包 |
| `static/` 或 embed | ECharts、可选 jsPDF 二进制 |

---

## 10. 测试

- `binding` 聚合与 filter 后行集一致
- drilldown `set_filter` / `detail` 行为（runtime 单测或 Playwright 轻量）
- `format: html` 大小限制与拒绝外部 script src
- artifact GET 鉴权
- 集成：mock LLM 调工具 → GET HTML 含 `__BAIZE_PAGE__`
- `AnalysisPagePreview.test.ts`

---

## 11. Token 策略（给模型 / Skill 正文）

| 场景 | 建议 |
|------|------|
| 常规多图 + 表 + 结论 | `sections` + `datasets` + `binding` |
| 单张复杂/custom 图 | 该 section 用 `option` |
| 版式/交互规则难表达 | `format: html`（接受更高 token） |

---

## 12. 与旧规格关系

`create_interactive_report` + 固定 `charts[].type: bar|line|pie` **不实现**。若代码分支已开，改为本规格工具名与 schema。

---

*批准后分支 `feat/analysis-page` + `writing-plans` 实现计划。*
