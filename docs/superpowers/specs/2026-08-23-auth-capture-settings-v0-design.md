# 登录捕获（auth.capture）设置页 v0 设计规格

> 状态：已批准（2026-08-23，延续规划队列）  
> 日期：2026-08-23  
> 前置：会话登录门闸、企业执行回调 Tools 扩展区已落地  
> 依据：`2026-08-15-session-login-tool-gate-design.md` §5.1；HTTP 插件设置页 v0「capture 仍 YAML」  
> **后续：** HTTP 插件 capture UI、插件/OpenAPI 页 preserve capture、列表摘要见 `2026-08-26-login-capture-settings-v1-design.md`

---

## 1. 目标与成功标准

**目标：** 管理员在 **设置 → Tools** 为 `type: openapi` Connector 配置 `auth.capture`，无需手改 YAML/curl。

**成功标准：**

1. 仅 **OpenAPI** Connector 组头展示「登录捕获」表单（HTTP/MCP 不展示）
2. 字段：`tool_name_glob`、`token_json_paths`、`label_json_paths`、`header_template`、`default_scheme`（可选）
3. GET connector 回显已生效配置（含服务端缺省补全后的 glob/paths）
4. 保存合并现有「执行回调 URL」同一 `PUT`（保留 `auth.mode` 与其它 auth 块）
5. `tool_name_glob: "__none__"` 可关闭捕获（UI 说明）
6. `CaptureSettings.test.ts` 纯函数测试；`npm run build` + dist
7. README 中英补充 Tools 页可配登录捕获

**不做：**

- HTTP 插件 / MCP 的登录捕获 UI
- 新后端 API
- 捕获规则可视化测试按钮（v1）

---

## 2. UI

在 Tools 页 Connector 组头（与执行回调 URL 同区）：

| 字段 | 控件 |
|------|------|
| `tool_name_glob` | 单行；提示 `__none__` 关闭 |
| `token_json_paths` | 多行，每行一个 JSON path |
| `label_json_paths` | 多行 |
| `header_template` | 单行，如 `Bearer {{token}}` |
| `default_scheme` | 单行可选 |

按钮：**保存 Connector 设置**（回调 URL + 捕获一并提交）。

---

## 3. 实现

- 复用 `putConnector`；`buildCaptureAuth` / `captureToDraft` 纯函数 + 测试
- `ToolsSettings.tsx` 扩展 state 与 openapi 表单区
- 无 Go 变更（PUT/GET 已支持 `auth.capture`）

---

*批准后 `feat/auth-capture-settings` 实现。*
