# EXT-SoR：Busabase 只读外接示例

验证切片：用白泽既有 **OpenAPI** 或 **MCP** 连接器路径，只读查询 Busabase 类团队沉淀。规格与探测见 [`docs/superpowers/specs/2026-10-21-ext-sor-busabase-design.md`](../../docs/superpowers/specs/2026-10-21-ext-sor-busabase-design.md)、[`docs/superpowers/notes/2026-10-21-ext-sor-busabase-probe.md`](../../docs/superpowers/notes/2026-10-21-ext-sor-busabase-probe.md)。

<p><strong style="color:red">只读：禁止通过 Agent 创建、更新或删除 Busabase 记录；写入请在 Busabase UI 由人完成。</strong></p>

## 占位 OpenAPI vs 生产 spec

| 本目录 | 生产 / Cloud |
|--------|----------------|
| `openapi.readonly.example.yaml` — **形状占位**，路径为简化的 `/tables/...`，**不是** Busabase 真实 REST 面 | 运行实例或 Cloud 导出：`GET https://busabase.com/api/v1/openapi.json`（OpenAPI 3.1.x，见探测笔记） |

**生产请改用实例导出的 `openapi.json` 并配置真实 `base_url`；本仓库占位文件仅用于说明连接器字段形状。** 可将下载的 spec 放到 [`examples/local/`](../local/README.md)（gitignore），再在连接器里改 `spec:` 指向该文件。

示例中的 `YOUR-BUSABASE-HOST`、`${BUSABASE_TOKEN}` 等为**非生产占位**，勿提交真实密钥。

## OpenAPI 连接器（首选）

1. 打开 [`connector.openapi.example.yaml`](connector.openapi.example.yaml)，将 `base_url` 改为你的 Busabase 主机（自托管或 Cloud）。
2. 设置环境变量 `BUSABASE_TOKEN`（Cloud / 需密钥的自托管）；多 space 时在 `auth.static.headers` 增加 `x-busabase-space`（见 [Busabase OpenAPI 文档](https://busabase.com/docs/openapi)）。
3. 合并进本地配置（与 [`configs/config.local.yaml`](../../configs/config.local.yaml) 中 `connector:` 块同形）——替换或注释掉现有 `mock-ticket` 示例，例如：

```yaml
connector:
  id: busabase-readonly
  type: openapi
  spec: examples/ext-sor-busabase/openapi.readonly.example.yaml   # 生产改为 examples/local/openapi.json 等
  base_url: https://YOUR-BUSABASE-HOST
  auth:
    mode: static
    static:
      headers:
        Authorization: "Bearer ${BUSABASE_TOKEN}"
```

4. 重启 Runtime / 重新 `PUT` 连接器，使工具目录刷新。

## MCP 连接器（附录）

探测确认存在 Streamable HTTP MCP（见探测笔记）。模板：[`connector.mcp.example.yaml`](connector.mcp.example.yaml)。

- **本地 Desktop**：默认 `http://localhost:15419/api/mcp`，通常可省略 `headers`。
- **Cloud**：`https://busabase.com/api/mcp`；OAuth 或 `Authorization: Bearer ${BUSABASE_TOKEN}`。

MCP 鉴权写在 `mcp.headers`，**不用** `connector.auth`（见 [`2026-08-23-mcp-bridge-v0-design.md`](../../docs/superpowers/specs/2026-08-23-mcp-bridge-v0-design.md) §4.1）。

## 启用只读 Skill

1. 将 [`skills/busabase-readonly/`](skills/busabase-readonly/SKILL.md) 复制或链接到你的 Skill 目录，或在 `configs/config.local.yaml` 的 `agent.skills` 列表中加入 `busabase-readonly`（路径规则与现有 `baize-help`、`ticket-triage` 一致）。
2. 确认连接器已注册且对应工具（OpenAPI 的 `list_*` 或 MCP 发现工具）在目录中 **enabled**。
3. Skill 文案已声明 **禁止写入**；Agent 应仅调用列/查类工具。

## 可选活测（有实例时）

1. 按上文配置 OpenAPI 或 MCP，并设置有效凭证（不写进 git）。
2. 启动白泽，在对话中启用 `busabase-readonly` Skill，发起一次只读问题（例如「有哪些表？」「某表前几条记录？」）。
3. 将日期与结果附录到 [`2026-10-21-ext-sor-busabase-probe.md`](../../docs/superpowers/notes/2026-10-21-ext-sor-busabase-probe.md)「活测」小节。

无实例时不阻塞本示例交付。
