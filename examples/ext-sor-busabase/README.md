# EXT-SoR：Busabase 只读外接示例（配方）

> **定位：** Busabase 是「完整课外接」的**实行示例**，不是白泽产品绑定。产品入口只用既有 **设置 → 业务系统（OpenAPI）** / MCP；本目录提供可复现的 GET-only spec、连接器 YAML 与只读 Skill。

验证切片：用白泽既有 OpenAPI 或 MCP 连接器路径，只读查询 Busabase 类团队沉淀。规格与探测见 [`docs/superpowers/specs/2026-10-21-ext-sor-busabase-design.md`](../../docs/superpowers/specs/2026-10-21-ext-sor-busabase-design.md)、[`docs/superpowers/notes/2026-10-21-ext-sor-busabase-probe.md`](../../docs/superpowers/notes/2026-10-21-ext-sor-busabase-probe.md)。纠偏说明：[`docs/superpowers/specs/2026-10-22-ext-sor-busabase-ui-design.md`](../../docs/superpowers/specs/2026-10-22-ext-sor-busabase-ui-design.md)。

<p><strong style="color:red">只读：禁止通过 Agent 创建、更新或删除 Busabase 记录；写入请在 Busabase UI 由人完成。</strong></p>

## 产品路径（既有 UI）

1. 白泽 **设置 → 业务系统** → 接入业务系统。  
2. 上传或粘贴本目录 [`openapi.readonly.json`](openapi.readonly.json)（或填文档 URL 后自行裁剪）。  
3. 连接器 id / base URL / Bearer 按实例填写（示例见 [`connector.openapi.example.yaml`](connector.openapi.example.yaml)）。  
4. 保存后于 **设置 → 技能** 启用 [`busabase-readonly`](skills/busabase-readonly/SKILL.md)（可选）。

**不要**依赖设置首页「Busabase」专属卡或 `?preset=` 深链——产品不绑定单一 SoR 厂商。

## GET-only spec（真相源）

| 文件 | 说明 |
|------|------|
| [`openapi.readonly.json`](openapi.readonly.json) | **入库真相源**：从 Busabase 公开 OpenAPI 裁剪的仅 GET 面 |
| [`openapi.readonly.example.yaml`](openapi.readonly.example.yaml) | **已弃用**：原 `/tables/...` 占位 stub |

**再生成**见 [`scripts/ext-sor-busabase/README.md`](../../scripts/ext-sor-busabase/README.md)。自托管或 Cloud 亦可 `GET …/api/v1/openapi.json` 后走同一脚本。

可将实例完整 spec 放到 [`examples/local/`](../local/README.md)（gitignore），再在连接器里改 `spec:`。示例中的 `YOUR-BUSABASE-HOST`、`${BUSABASE_TOKEN}` 为占位，勿提交真实密钥。

## OpenAPI 连接器（首选）

1. 打开 [`connector.openapi.example.yaml`](connector.openapi.example.yaml)，将 `base_url` 改为你的主机。  
2. 设置 `BUSABASE_TOKEN`；多 space 时增加 `x-busabase-space`（见 [OpenAPI 文档](https://busabase.com/docs/openapi)）。  
3. 合并进本地配置或经 UI `PUT` 连接器：

```yaml
connector:
  id: busabase-readonly
  type: openapi
  spec: examples/ext-sor-busabase/openapi.readonly.json
  base_url: https://YOUR-BUSABASE-HOST
  auth:
    mode: static
    static:
      headers:
        Authorization: "Bearer ${BUSABASE_TOKEN}"
```

4. 重启 Runtime / 重新保存连接器，使工具目录刷新。

## MCP 连接器（附录）

模板：[`connector.mcp.example.yaml`](connector.mcp.example.yaml)。本地 Desktop 默认 `http://localhost:15419/api/mcp`；Cloud 见探测笔记。MCP 鉴权写在 `mcp.headers`。

## 启用只读 Skill

将 [`skills/busabase-readonly/`](skills/busabase-readonly/SKILL.md) 纳入 Skill 目录或 `agent.skills`；确认对应工具 enabled。Skill 声明 **禁止写入**。

## 可选活测

有实例时配置 OpenAPI/MCP，对话中启用 Skill 做一次只读查询，结果附录到探测笔记。无实例不阻塞本示例。
