# CLEAN-AUDIT 盘点清单

> 日期：2026-09-15  
> 状态：**待产品确认**  
> 规格：[`../specs/2026-09-15-oss-quality-cleanup-design.md`](../specs/2026-09-15-oss-quality-cleanup-design.md)  
> 约束：本文件只读盘点结果；确认前不得开 CONTRACT 实现。

## 0. 产品确认栏

- [ ] 契约白名单已审阅
- [ ] 结构热点优先级已审阅
- [ ] 明确不做无异议
- [ ] 批准进入 CLEAN-CONTRACT

确认人 / 日期：

## 1. 契约白名单（HTTP）

| METHOD path | 处置 | 理由 |
|-------------|------|------|
| DELETE /v0/connectors/{id} | 保留 | |
| DELETE /v0/connectors/{id}/tools/{name} | 保留 | |
| DELETE /v0/conversations/{id} | 保留 | |
| DELETE /v0/conversations/{id}/identities | 保留 | |
| DELETE /v0/conversations/{id}/identities/{iid} | 保留 | |
| DELETE /v0/conversations/{id}/messages | 保留 | |
| DELETE /v0/settings/mcp-export/identities/{id} | 保留 | |
| DELETE /v0/settings/mcp-export/keys/{id} | 保留 | |
| DELETE /v0/settings/memory/{id} | 保留 | |
| DELETE /v0/settings/models/{id} | 保留 | |
| DELETE /v0/skills/{id} | 保留 | |
| GET /healthz | 保留 | |
| GET /v0/agents/{id} | 保留 | |
| GET /v0/artifacts/{id} | 保留 | |
| GET /v0/channels/media/{conv}/{object} | 保留 | |
| GET /v0/connectors | 保留 | |
| GET /v0/connectors/{id} | 保留 | |
| GET /v0/connectors/{id}/mcp/oauth/callback | 保留 | |
| GET /v0/connectors/{id}/mcp/oauth/status | 保留 | |
| GET /v0/conversations | 保留 | |
| GET /v0/conversations/{id}/identities | 保留 | |
| GET /v0/conversations/{id}/messages | 保留 | |
| GET /v0/me | 保留 | |
| GET /v0/runs/{id} | 保留 | |
| GET /v0/runs/{id}/events | 保留 | |
| GET /v0/runs/{id}/stream | 保留 | |
| GET /v0/settings/channels/{name} | 保留 | |
| GET /v0/settings/channels/{name}/login/status | 保留 | |
| GET /v0/settings/channels/{name}/outbound-deliveries | 保留 | |
| GET /v0/settings/credentials | 保留 | |
| GET /v0/settings/events-webhook | 保留 | |
| GET /v0/settings/events-webhook/deliveries | 保留 | |
| GET /v0/settings/inbox-channels | 保留 | |
| GET /v0/settings/mcp-export | 保留 | |
| GET /v0/settings/mcp-export/identities | 保留 | |
| GET /v0/settings/mcp-export/identities/{id} | 保留 | |
| GET /v0/settings/mcp-export/keys | 保留 | |
| GET /v0/settings/memory | 保留 | |
| GET /v0/settings/models | 保留 | |
| GET /v0/settings/runtime | 保留 | |
| GET /v0/settings/store | 保留 | |
| GET /v0/skills | 保留 | |
| GET /v0/skills/{id} | 保留 | |
| GET /v0/tools | 保留 | |
| GET /v0/ui-config | 保留 | |
| HANDLE /ui/ | 保留 | |
| HANDLE /v0/mcp/export | 保留 | |
| HANDLE /v0/mcp/export/ | 保留 | |
| PATCH /v0/settings/credentials | 保留 | |
| PATCH /v0/settings/mcp-export/identities/{id} | 保留 | |
| PATCH /v0/settings/memory/{id} | 保留 | |
| PATCH /v0/settings/models/{id} | 保留 | |
| PATCH /v0/settings/runtime | 保留 | |
| PATCH /v0/tools/{name} | 保留 | |
| POST /v0/channels/{name}/inbound | 保留 | |
| POST /v0/connectors/{id}/mcp/oauth/disconnect | 保留 | |
| POST /v0/connectors/{id}/mcp/oauth/start | 保留 | |
| POST /v0/connectors/{id}/tools | 保留 | |
| POST /v0/conversations/{id}/fork | 保留 | |
| POST /v0/conversations/{id}/identities | 保留 | |
| POST /v0/conversations/{id}/identities/{iid}/default | 保留 | |
| POST /v0/conversations/{id}/messages/{message_id}/rollback | 保留 | |
| POST /v0/inbox/{channel_id} | 保留 | |
| POST /v0/runs | 保留 | |
| POST /v0/runs/{id}/cancel | 保留 | |
| POST /v0/runs/{id}/plugin-callbacks | 保留 | |
| POST /v0/runs/{id}/resume | 保留 | |
| POST /v0/settings/channels/{name}/login/start | 保留 | |
| POST /v0/settings/channels/{name}/logout | 保留 | |
| POST /v0/settings/channels/{name}/outbound-deliveries/{id}/retry | 保留 | |
| POST /v0/settings/channels/{name}/process/restart | 保留 | |
| POST /v0/settings/channels/{name}/process/start | 保留 | |
| POST /v0/settings/channels/{name}/process/stop | 保留 | |
| POST /v0/settings/events-webhook/deliveries/{id}/retry | 保留 | |
| POST /v0/settings/events-webhook/test | 保留 | |
| POST /v0/settings/inbox-channels/{id}/rotate-secret | 保留 | |
| POST /v0/settings/inbox-channels/{id}/test | 保留 | |
| POST /v0/settings/mcp-export/identities | 保留 | |
| POST /v0/settings/mcp-export/keys | 保留 | |
| POST /v0/settings/memory | 保留 | |
| POST /v0/settings/models | 保留 | |
| POST /v0/settings/reload | 保留 | |
| POST /v0/settings/store/restart | 保留 | |
| POST /v0/skills | 保留 | |
| PUT /v0/agents/{id} | 保留 | |
| PUT /v0/connectors/{id} | 保留 | |
| PUT /v0/settings/channels/{name} | 保留 | |
| PUT /v0/settings/events-webhook | 保留 | |
| PUT /v0/settings/inbox-channels | 保留 | |
| PUT /v0/settings/store | 保留 | |

处置枚举：**保留** / **重命名** / **删除**。

## 2. 契约白名单（配置 / env / CLI）

| 键或命令 | 处置 | 理由 |
|----------|------|------|
| （待填） | 保留 | |

## 3. 契约白名单（Web 客户端 / 路由）

| 表面（api.ts 函数或 UI 路由） | 处置 | 理由 |
|------------------------------|------|------|
| （待填） | 保留 | |

## 4. 废弃 / 兼容 / 双写信号

| 位置 | 信号摘要 | 建议处置 | 理由 |
|------|----------|----------|------|
| （待填） | | 删除或保留 | |

## 5. 结构热点

| 路径 | 行数（约） | 建议拆法（一句话） | 本版优先级 |
|------|------------|--------------------|------------|
| （待填） | | | P0/P1/P2/保留 |

## 6. 门禁基线

| 工具 | 命令 | 问题数或退出码 | 备注 |
|------|------|----------------|------|
| golangci 全量 | | | |
| eslint | | | |
| gofmt -l | | | |

## 7. 性能候选（只标注，不测）

| 路径 | 为何值得测 | PERF-HOT 建议探针 |
|------|------------|-------------------|
| 聊天流式 `GET /v0/runs/{id}/stream` | | |
| 会话消息读写 | | | |
| blob / artifacts | | | |
| 渠道出站 | | | |

## 8. 公开文档处置（CONTRACT 执行）

| 路径 | 建议 | 理由 |
|------|------|------|
| `README.md` / `README.zh-CN.md` | 重写为产品向 | 规格已定 |
| 新建开发者文档 | 新建 | 规格已定 |
| `docs/architecture-and-plugin-protocol.md` | 删除 | 规格已定 |
| `docs/deployment.md` | 删除 | 规格已定 |

## 9. 明确不做（本轮 CLEAN）

- 插件公共 Go SDK、OTel、Playwright、新功能史诗
- 强制覆盖率挡合并
- 无证据的性能改动、伪造竞品对比
- （AUDIT 中发现但决定不做的项追加于此）

## 10. 复现命令备忘

### 任务 2：HTTP 路由全表

```powershell
$utf8NoBom = New-Object System.Text.UTF8Encoding $false
$routes = Select-String -Path internal\api\server.go -Pattern 'HandleFunc\("(GET|POST|PUT|PATCH|DELETE) ([^"]+)"' |
  ForEach-Object { if ($_.Line -match 'HandleFunc\("((?:GET|POST|PUT|PATCH|DELETE) [^"]+)"') { $Matches[1] } } |
  Sort-Object -Unique
$extra = @(
  'POST /v0/channels/{name}/inbound',
  'HANDLE /v0/mcp/export',
  'HANDLE /v0/mcp/export/',
  'HANDLE /ui/'
)
($routes + $extra | Sort-Object -Unique) | ForEach-Object { $_ } |
  Set-Content -Path docs\superpowers\notes\2026-09-15-clean-audit-routes.txt -Encoding $utf8NoBom
```

```powershell
Select-String -Path internal\**\*.go,cmd\**\*.go -Pattern 'RegisterRoute|POST /v0/channels/' |
  Where-Object { $_.Path -notmatch '_test\.go$' } |
  Select-Object -First 30 Path,LineNumber,Line
```

（任务 2 核对：`internal/api/server.go` 为 `HandleFunc`/`mux.Handle` 主表；**动态渠道入站** `POST /v0/channels/{name}/inbound` 由 `internal/channel/webhook/channel.go` 经 `api.Server.RegisterRoute` 按渠道名挂载（契约路径模板见上）。）

（**非本表：** 独立进程 `cmd/weixin-adapter` 另暴露适配器侧 HTTP（如 `/outbound`、`/admin/*`），不属于 baize core mux 白名单。）
