# UI-MCP-OAUTH：远程 HTTP MCP 的 OAuth 2.1 交互登录

> 状态：**已交付**（计划：[`plans/2026-09-14-mcp-oauth.md`](../plans/2026-09-14-mcp-oauth.md)）

> 日期：2026-09-14  
> 史诗：UI-MCP-OAUTH  
> 前置：P3-C MCP 连接器页（静态 Bearer / headers）；设置页 ACL 仅 admin；stdio MCP 凭 env  
> 备注：承接 P3-C defer 的「规范 OAuth 2.1 交互登录」；**不**与 LOGIN-SKILL / export_db_ro / P6 Memory 合并

---

## 1. 背景与动机

远程 Streamable HTTP MCP 目前只能靠静态请求头。需 OAuth 的服务只能手工贴 Token，无法发现、授权、刷新。P3-C 已明确该项单独立项。

运营角色不能改 MCP 连接器；Chat 只用管理员已接好的工具。因此「每操作员一套令牌」在现权限下无产品价值。

## 2. 已确认产品决策

| 议题 | 决策 |
|------|------|
| 身份粒度 | **连接器级**：管理员授权一次，整台 Runtime 共用令牌 |
| 谁可授权 | **仅 admin**（设置 → 外部工具服务）；运营不授权、不改权限模型 |
| 完整性 | 开源第一版做完整交互登录，不分期「先贴 Token」 |
| 传输 | 仅 **HTTP/Streamable HTTP** MCP；**stdio 不改**（仍 env） |
| 合并 | **独立史诗**；不并 LOGIN-SKILL / BLOB-CS / LLM-THINK / UI-I18N / F |

## 3. 成功标准

1. 管理员对需 OAuth 的远程 MCP 可在设置页完成：发现 → PKCE → 浏览器回调 → 令牌落库。  
2. 之后 Chat / Run 调用该连接器工具时自动带有效 access token。  
3. access 过期可刷新；刷新失败时设置页提示重登，工具侧有可读失败（非裸栈）。  
4. 令牌经现有秘密落库加密路径存储，不进日志明文。  
5. stdio MCP 与仅静态 header 的 HTTP MCP 行为不变。

## 4. 范围

### 做

- OAuth 2.1 交互（对齐 MCP 鉴权惯例）：401 / PRM 发现、PKCE、浏览器回调、（若服务支持）DCR、access/refresh 存储与刷新。  
- 设置页「去授权 / 重新授权」与连接器状态（已授权 / 需重登）。  
- 回调入口挂在 Runtime（admin 会话或一次性 state），不开放给运营自助。  
- 调用 MCP 时注入 Authorization；与现有静态 headers 的优先级在实现计划中写死（OAuth 覆盖同名 Authorization 为默认推荐）。

### 不做

- 运营自己授权；每控制面账号独立令牌。  
- 把 OpenAPI/HTTP 登录 capture（LOGIN-SKILL）与 MCP OAuth 合成一套 UI/存储。  
- MCP capture「需本人登录」工具语义（P3-C 已否）。  
- stdio OAuth。  
- 企业 IdP / SSO 多租户。

## 5. 架构草图

```
设置页（admin）→ 开始授权
        │
        ▼
发现授权端点（PRM / WWW-Authenticate）→ 打开浏览器（PKCE）
        │
        ▼
Runtime 回调 → 换 token → 加密写入连接器秘密存储
        │
        ▼
MCP HTTP 调用 ← 解析 access（可 refresh）← 存库令牌
```

## 6. 风险与依赖

- 各 MCP 服务对 DCR / PRM 支持不一致 → 计划中列兼容矩阵与降级（例如仅支持预登记 client_id）。  
- 回调 URL 需可从管理员浏览器访问（本地 `localhost` / 配置的 public base）。  
- 与 `settingscrypto` / Store 秘密字段对齐，避免第二套明文表。

## 7. 测试与验收（规格级）

- 授权成功后工具调用带 Bearer；刷新路径单测。  
- 无 refresh 且过期 → 设置页需重登；调用失败人话。  
- 运营账号无法触发授权 API（ACL）。  
- stdio / 静态 header 回归。

## 8. 后继（非本批）

- 运营在 Chat 遇 401 自助授权（须先扩权限模型）。  
- 每操作员令牌。  
- 设备码流程（若生态需要）。

## 9. 文档与账本

- 账本 / 确认清单：UI-MCP-OAUTH **已交付**。  
- P3-C §11 defer 的 OAuth 交互登录由本文承接（[`2026-09-12-webui-refresh-p3c-mcp-pages-design.md`](2026-09-12-webui-refresh-p3c-mcp-pages-design.md)）。
