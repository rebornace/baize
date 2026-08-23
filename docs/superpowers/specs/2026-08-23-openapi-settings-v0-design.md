# OpenAPI Connector 设置页 v0 设计规格

> 状态：已批准（2026-08-23）  
> 日期：2026-08-23  
> 前置：Tools 设置页、HTTP 插件设置页、MCP 设置页、工具目录已落地  
> 取代意向：`2026-08-23-http-plugin-settings-v0-design.md` §「不做」中「OpenAPI 仍 YAML/curl」  
> 依据：产品原则 — **管理员配置以 `/ui` 为主**；curl/YAML 仅作启动默认值与高级路径

---

## 1. 目标与成功标准

**目标：** 管理员在 **设置 → OpenAPI** 通过表单 + **上传企业常见接口文档文件** 注册或更新 `type: openapi` Connector，服务端**自动识别格式并归一化为 OpenAPI 3** 后发现全部 Tools，**无需 curl**。

**成功标准：**

1. 新页 `/settings/openapi`（admin）：列表已注册 OpenAPI Connector；**添加 / 编辑** 抽屉表单
2. 表单：`id`、`base_url`、**接口文档上传**（见 §3.4 支持格式）、`auth`、`require_approval`、`require_login`、`execution_callback_url`（可选）
3. 后端 `PUT` 支持 **`spec_content`** + 可选 **`import_format`**（`auto` 默认）；归一化后落盘再 `Apply`
4. UI 上传后展示**识别结果**（如「Postman Collection → 将转换为 OpenAPI」）与预估工具数（保存后刷新）
5. **编辑**时可只改 `base_url` / auth / 回调，不强制重新上传
6. 无法识别 / 转换失败 → `400 invalid_spec`（message 含格式提示）；`409 tool_conflict`；UI 展示 `error.code`
7. 链到 Tools；README：**主路径 UI**；列出支持格式；curl 降为附录
8. 测试：各格式样例转换单测 + UI 纯函数 + `spec_content` 集成

**不做（v0）：**

- `DELETE /v0/connectors/{id}`
- `auth.capture` 完整表单（仍 Tools 页 OpenAPI 组头）
- 在线编辑文档正文
- 从 URL 拉取远程文档（v1）
- **PDF / Word / HTML 网页**（非结构化，v0 不做 OCR/LLM 解析）
- **WSDL / SOAP**（非 REST 主路径；另里程碑或 HTTP 插件）
- **AsyncAPI**（事件型，v1 候选）

---

## 2. 产品路径

```
管理员 → /ui/settings/openapi
  → 填写 id、base_url、选择接口文档（OpenAPI / Swagger / Postman…）
  → 保存 → PUT spec_content [+ import_format]
  → 识别格式 → 转换为 OpenAPI 3 → 落盘 → Apply 发现 operations
  → /settings/tools 可见全部 Tools
```

内部流水线始终是 **OpenAPI 3 → LoadTools**；差异在 **导入适配器**，不是多套 Tool 引擎。

---

## 3. 后端：`spec_content` 与多格式导入

### 3.1 原则

| 层 | 职责 |
|----|------|
| **导入层** | 识别企业上传格式 → 输出 **OpenAPI 3 JSON** |
| **发现层** | 现有 `openapi.LoadTools`（kin-openapi）不变 |
| **落盘** | 保留 `imported.*`（审计/排错）+ `openapi.normalized.json`（Apply 用） |

企业文档五花八门，但 Baize 只需一种 canonical 格式；**转换在服务端一次完成**，避免 UI 或 Agent 各写一套。

### 3.2 PUT body 扩展

```json
{
  "type": "openapi",
  "base_url": "https://api.example.com",
  "spec_content": "文件全文",
  "import_format": "auto",
  "execution_callback_url": "https://...",
  "auth": { ... },
  "require_approval": ["create_ticket"],
  "require_login": ["get_ticket"]
}
```

| 字段 | 规则 |
|------|------|
| `spec` | 服务端路径（YAML 启动）；与 `spec_content` **互斥** |
| `spec_content` | ≤ **4 MiB** 文本 |
| `import_format` | 可选，默认 `auto`；见 §3.4 |
| 编辑且无新文件 | Store 已有 spec → **沿用**；否则 `400 spec required` |

### 3.3 包 `internal/connector/specimport`

```go
// NormalizeSpec(content, format string) (openapi3JSON []byte, detectedFormat string, err error)
```

流程：

1. `DetectFormat(content)` — `auto` 时按 JSON 根字段 / 文件结构判断
2. 调用对应 **Converter** → OpenAPI 3 文档（内存）
3. `Validate`（kin-openapi）→ 写入 `data/connectors/{id}/openapi.normalized.json`
4. 可选：原文件写入 `data/connectors/{id}/imported.{json|yaml}`

`GET /v0/connectors/{id}` 可选回显：

- `import_format_detected`（上次识别格式）
- `spec`（归一化文件路径）

### 3.4 v0 支持格式

| `import_format` / 识别 | 典型扩展名 | 说明 |
|------------------------|------------|------|
| `openapi3` | `.json` `.yaml` `.yml` | OpenAPI 3.0 / 3.1，直通校验 |
| `swagger2` | `.json` `.yaml` `.yml` | Swagger 2.0 → 转 OpenAPI 3（`kin-openapi` 或专用 converter） |
| `postman` | `.json` | Postman Collection **v2.1**（`info.schema` 含 `v2.1.0`）；转 OpenAPI 3 |
| `auto` | 上述全部 | 按根字段优先级检测（见下） |

**`auto` 检测顺序（实现定稿）：**

1. `openapi` / `swagger` 字段 → Swagger 2 或 OAS3 细分  
2. `info.schema` 含 `postman` → Postman  
3. `openapi: "3.x"` → OpenAPI 3  
4. 否则 `invalid_spec` + message「未识别格式，请选 import_format 或换 OpenAPI/Postman 导出」

**UI `accept`：** `.json,.yaml,.yml`（Postman 多为 `.json`）

**v0.1 候选（本规格不实现，README 写「可提 Issue」）：** RAML、API Blueprint、Insomnia、部分云厂商网关导出 JSON（需样例后加适配器）。

**明确不支持：** PDF、Word、Markdown 说明页、WSDL、纯 HTML 文档站。

### 3.5 错误码

| 码 | 场景 |
|----|------|
| `invalid_spec` | 无法识别、转换失败、校验失败、无 operation |
| `unsupported_import_format` | `import_format` 枚举非法（v0 仅上表） |
| `tool_conflict` | 工具名冲突 |
| `invalid_request` | 超大、spec 与 spec_content 同时出现 |

### 3.6 与 `base_url` 的关系

Postman / 部分导出带 `variable` 或 host 占位；**仍以表单 `base_url` 为准**覆盖 `servers[0].url`（转换后 patch）。UI 说明：「若文档里 host 不对，以填写的 base_url 为准」。

---

## 4. UI：`OpenApiSettings.tsx`

对称 `PluginSettings.tsx` / `McpSettings.tsx`。

| 区域 | 内容 |
|------|------|
| 导航 | `settingsNav` 增加 **OpenAPI**（admin） |
| 说明 | 支持 **OpenAPI 3、Swagger 2、Postman Collection** 上传；自动转换；工具在 Tools 管理 |
| 列表 | `connector_id`、`base_url`、上次 `import_format_detected`（若有）、工具数 |
| 文件 | `accept=".json,.yaml,.yml"`；选文件后 **本地预检测**（轻量 JSON parse + 根字段）展示「识别为：…」 |
| 格式 | 下拉 `import_format`：`自动识别` / `OpenAPI 3` / `Swagger 2` / `Postman`（识别失败时手动指定） |
| auth | 同 Plugin |
| 保存 | `putConnector(..., { spec_content, import_format })` |

### 4.1 `api.ts`

```ts
export type ImportFormat = 'auto' | 'openapi3' | 'swagger2' | 'postman'

export interface PutConnectorBody {
  // ...
  spec_content?: string
  import_format?: ImportFormat
}
```

---

## 5. Tools 页补充（小改）

- 空状态「尚未注册 Connector」增加链接：**去 OpenAPI 设置注册**
- 「添加」抽屉说明：仅补 **单条 extra**；批量导入请用 **OpenAPI 设置**

---

## 6. 文档

README / README.zh-CN：

- **接到你的服务 → 有 OpenAPI**：**设置 → OpenAPI** 上传文档（支持 OpenAPI 3 / Swagger 2 / Postman v2.1）
- 表格列出支持格式与「不支持 PDF/Word/WSDL」
- curl `PUT` 移至「高级 / 自动化」小节
- 与插件、MCP、Webhook 同一「UI 优先」表述

---

## 7. 测试

| 层 | 内容 |
|----|------|
| Go | `specimport`：openapi3 / swagger2 / postman 样例各一例 → normalized JSON 含 paths |
| Go | `PUT` + `spec_content` → tools 发现；`import_format` 错误枚举 |
| Go | 超 4MiB、互斥字段 → 400 |
| TS | `detectImportFormat`（客户端预检）、`openApiConnectorIds` |
| 样例 | `examples/spec-import/`：最小 postman + swagger2 json（测试用） |

---

## 8. 实现组件

| 组件 | 职责 |
|------|------|
| `internal/connector/specimport` | 识别、转换、归一化落盘 |
| `internal/connector/specstore` | `data/connectors/{id}/` 路径管理 |
| `internal/api/server.go` | PUT `spec_content` / `import_format` |
| `web/chat/src/pages/OpenApiSettings.tsx` | 列表 + 上传 + 格式下拉 |
| `web/chat/src/specImport.ts` | 客户端预检测（与服务端检测对齐） |
| `examples/spec-import/` | 测试夹具 |

---

## 9. 与「全功能 UI 化」关系

本里程碑补齐 **OpenAPI 批量注册** 缺口。已 UI 化：Tools 目录、插件、MCP、Skills、Webhook、账号。仍依赖 curl/YAML 的其它项（若有）按同样模式后续里程碑补齐；**不以 curl 作为主文档路径**。

---

*批准后分支 `feat/openapi-settings` + 实现计划。*
