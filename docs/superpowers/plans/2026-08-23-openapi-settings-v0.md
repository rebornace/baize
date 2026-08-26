# OpenAPI 设置页 v0 实现计划

> **工作者：** subagent-driven-development

**规格：** `docs/superpowers/specs/2026-08-23-openapi-settings-v0-design.md`（已批准）

**分支：** `feat/openapi-settings`

**目标：** 设置 → OpenAPI 上传企业接口文档（OpenAPI 3 / Swagger 2 / Postman v2.1），`spec_content` + `import_format` 归一化为 OAS3 后发现 Tools；curl 降为附录。

**架构：** `internal/connector/specimport` 识别并转换 → `specstore` 落盘 `openapi.normalized.json` → 现有 `connector.Apply` + `openapi.LoadTools`。UI 对称 `PluginSettings`/`McpSettings`。

**技术栈：** kin-openapi（校验 + Swagger2 转换）、Go、React/Vite。

**全局约束：**
- commit 中文 `type(scope): 说明`
- Go：`$env:GOPROXY='https://goproxy.cn,direct'`；`$env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH`
- UI 后 `npm run build` + `internal/ui/dist/**`
- 不做 PDF/Word/WSDL（整删 Connector 已实现，见 `docs/superpowers/specs/2026-08-26-connector-delete-and-callback-urls-v0-design.md`）

**实现选定：**
- Swagger 2：`github.com/getkin/kin-openapi/openapi2conv` → OAS3
- Postman v2.1：包内 `postman.go` 最小转换（method、url、path、query、json body → paths）；集成测试用 `examples/spec-import/postman-min.json`
- `data/connectors/{id}/` 相对 sqlite 父目录（与 artifact 同模式）
- GET connector 增加 `import_format_detected`（存 connectors 表新列 `import_format` 或 settings json — 实现选 `import_format TEXT` migrate）
- 编辑 PUT：无 `spec_content` 时沿用 Store 已有 `spec` 路径

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `internal/connector/specimport/detect.go` | `DetectFormat` |
| `internal/connector/specimport/normalize.go` | `Normalize(content, format, baseURL)` |
| `internal/connector/specimport/postman.go` | Postman → OAS3 |
| `internal/connector/specimport/*_test.go` | 三格式样例 |
| `internal/connector/specstore/store.go` | 写 imported + normalized |
| `internal/store/sqlite.go` | `connectors.import_format` 列（可选） |
| `internal/api/server.go` | PUT/GET 扩展 |
| `internal/api/server_openapi_put_test.go` | spec_content 集成 |
| `examples/spec-import/*` | 夹具 |
| `web/chat/src/pages/OpenApiSettings.tsx` | 设置页 |
| `web/chat/src/specImport.ts` | 客户端预检测 |
| `web/chat/src/pages/ToolsSettings.tsx` | 空状态链接 |
| `web/chat/src/settingsNav.ts` + `main.tsx` | 路由 |
| README.md / README.zh-CN.md | UI 主路径 + 格式表 |

---

### 任务 0：分支

- [ ] `git checkout -b feat/openapi-settings`

### 任务 1：specimport + specstore + 夹具

- [ ] `examples/spec-import/`：minimal openapi3.json、swagger2.json、postman-min.json
- [ ] `specimport` 包 + 单测三格式
- [ ] `specstore.Write(id, content, normalized []byte)` 
- [ ] Commit `feat(specimport): 多格式归一化 OpenAPI 3`

### 任务 2：Store + API PUT/GET

- [ ] `handlePutConnector`：`spec_content`、`import_format`；编辑沿用 spec
- [ ] `patchServersURL(baseURL)` 后 Apply
- [ ] GET `import_format_detected`
- [ ] `server_openapi_put_test.go`
- [ ] Commit `feat(api): OpenAPI spec_content 上传注册`

### 任务 3：OpenApiSettings UI

- [ ] `OpenApiSettings.tsx` + `specImport.ts` + 测试
- [ ] 导航、路由、`api.ts` 类型
- [ ] Commit `feat(ui): OpenAPI 设置页与文档上传`

### 任务 4：Tools 空状态 + README + dist

- [ ] ToolsSettings 链接与说明
- [ ] README 中英
- [ ] `npm run build` + dist
- [ ] `go test ./...`
- [ ] Commit `docs: OpenAPI 设置 UI 主路径`

---

*子代理逐任务实现。*
