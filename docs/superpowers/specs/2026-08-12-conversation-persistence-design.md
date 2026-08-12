# Baize 设计规格：对话上下文与身份持久化

> 状态：已批准  
> 日期：2026-08-12  
> 前置：会话身份库（session-auth，进程内 IdentityStore）、Chat UI、`conversation_id` 挂在 Run  
> 问题背景：每次 `Execute` 仅注入 `system + 当前 user`，模型无多轮记忆；Identity 仅内存，重启后「已登录」丢失。对标可用开源 Agent，两者均需具备。

---

## 0. 动机

当前能力断层：

1. **对话上下文**：UI 有气泡，引擎无 transcript；下一句不知道上一句查过哪台设备。  
2. **登录身份**：同进程同 `conversation_id` 可复用 Token，但重启 Runtime 后 IdentityStore 清空，用户感觉「又忘了登录」。

**目标定位：** 以独立 Conversation Thread 持久化聊天记忆，并以 SQLite 持久化会话身份；Run/Events 继续做审计，不再充当聊天记忆的唯一来源。

---

## 1. 目标与成功标准

**目标：** 同一 `conversation_id` 下，多轮对话可衔接；登录状态可跨 Runtime 重启保持（在 sqlite 部署下）。

**成功标准：**

1. 登录 → 重启 Runtime → 同会话账号面板仍在，受保护调用无需再登录  
2. 「查设备列表」后再问「第一个的详情」→ 模型能衔接（在消息窗口内）  
3. 刷新 `/ui` → 气泡历史仍在  
4. 「新对话」→ 历史与身份均隔离  
5. `conversation.max_messages` 裁剪后新 Run 仍可完成，不因历史过长直接失败  
6. Identities 列表与 events 仍不出现完整 token 明文  

---

## 2. 范围

### 做

| 层 | 内容 |
|----|------|
| Message Store | 按 `conversation_id` 持久化 `user` / `assistant`（可选 `system_note`） |
| Engine | `Execute` 注入窗口化历史 messages |
| Identity Store | SQLite 实现（与 runs 同库）；API 路径不变 |
| API | `GET/DELETE .../messages`；runs 创建时写 user message；终态写 assistant |
| UI | 启动加载历史；可选「清空聊天」；账号面板复用持久化结果 |
| 配置 | `conversation.max_messages`、`persist_identities` |
| 测试 | 窗口注入、身份重启、API、清空历史不影响身份 |

### 不做（本版）

- 向量长期记忆、跨设备账号、云端漫游  
- OAuth / refresh 全家桶  
- 把完整 tool JSON 写入 Message  
- Identity at-rest 加密（与 `.env` / 本地 db 同信任边界；文档标明）  
- 自动生成 conversation title（可后续）  

---

## 3. 架构

```
UI
  │  conversation_id
  │  GET messages（回放） / POST runs（新一轮）
  ▼
API
  │  写 user message → CreateRun → Execute
  │  Run 终态 → 写 assistant message
  ▼
Engine
  │  system + window(history) + current user → ReAct
  │  HITL / Tools 不变；Invoke 仍走 Identity Resolve
  ▼
SQLite（store.driver=sqlite）
  ├── runs / events          （审计，已有）
  ├── messages               （对话记忆，新建）
  └── identities             （会话凭证，新建；替换纯内存）
```

原则：

- **审计与记忆分离**：Events 保留工具细节；Messages 只保留对人可读的轮次文本  
- **身份按会话隔离**：与现有 session-auth 一致  
- **memory 驱动**：messages / identities 可用内存实现，便于测试  

---

## 4. 数据模型

### 4.1 Conversation（可选轻量表）

| 字段 | 说明 |
|------|------|
| `id` | `conversation_id` |
| `created_at` / `updated_at` | 审计 |
| `title` | 本版可空 |

无独立行时，仅靠 messages / identities / runs 上的 `conversation_id` 也可工作；实现可先 upsert 空 title。

### 4.2 Message

| 字段 | 说明 |
|------|------|
| `id` | `msg_` + uuid |
| `conversation_id` | 归属 |
| `role` | `user` \| `assistant` \| `system_note` |
| `content` | 纯文本；不含完整 tool JSON / 凭证 |
| `run_id` | 可选；assistant 关联 Run |
| `created_at` | 排序 |

**写入时机：**

| 时机 | 动作 |
|------|------|
| `POST /v0/runs` 创建成功 | 追加 `user` = input |
| Run `succeeded` 且有 output | 追加 `assistant` = output |
| Run `failed` | 追加简短 `system_note` 或 `assistant`（如「运行失败：…」） |
| 工具调用过程 | **不**写入 Message（看 Events） |

**窗口：** Engine 取最近 `max_messages` 条（默认 **40**）。若窗口末尾已是本轮刚写入的 `user`，组装时不再重复追加当前 user。

### 4.3 Identity（持久化）

沿用现有 Identity 字段；存储改为表 `identities`（或等价）：

- `credential_headers_json` 等与凭证相关列仅存 Store，不进 messages / list 脱敏视图  
- 进程重启后 `List(conversation_id)` 可恢复  
- `persist_identities: false` 或 `store.driver: memory` 时保持进程内行为  

---

## 5. API

| 方法 | 路径 | 行为 |
|------|------|------|
| GET | `/v0/conversations/{id}/messages` | 时间正序；无敏感 header |
| DELETE | `/v0/conversations/{id}/messages` | 清空聊天历史；**不清** identities |
| 现有 | identities CRUD | 实现改为可持久化 Store |
| 现有 | `POST /v0/runs` | 落 user message；响应字段不变（仍含 `conversation_id`） |

Engine 组装顺序：

```
system(agent)
+ history（窗口内 user/assistant/system_note）
+ 当前 user（若末尾已是本轮 user 则跳过）
→ ReAct
→ 终态写 assistant / 失败短注
```

---

## 6. UI

- 启动 / 刷新：`GET .../messages` 渲染气泡；再拉 identities  
- 发送：仍 `POST /v0/runs` + 轮询 events（HITL / 工具过程）  
- 新对话：新 `conversation_id` → 空历史 + 空身份  
- 「清空聊天」（可选按钮）：`DELETE .../messages`，保留登录账号  
- 「退出账号」：现有 DELETE identity，不变  

---

## 7. 配置

```yaml
conversation:
  max_messages: 40           # <=0 则用默认 40
  persist_identities: true   # sqlite 下持久化身份；测试/memory 可 false
```

- `store.driver: sqlite`：messages 与 identities（当 persist 为 true）落 `sqlite_path`  
- README 注明：`data/baize.db` 可能含会话凭证，勿提交、勿当公开物  

---

## 8. 安全

- 完整凭证仅存 Identity Store；禁止写入 messages / events 明文  
- list identities 继续脱敏  
- 本版不做 at-rest 加密；信任模型 = 本地单机 / 受信部署（与 `.env` 一致）  
- 同进程多对话靠 `conversation_id` 隔离  

---

## 9. 迁移与兼容

- 启动 `CREATE TABLE IF NOT EXISTS` messages / identities；已有 runs/events 不受影响  
- 旧客户端不调 messages API：服务端仍写 messages；只是 UI 不回放  
- 不传 `conversation_id`：仍服务端生成并返回（现行为）  
- session-auth 的 Resolve / Capture / HITL 路径不变  

---

## 10. 测试计划

| 用例 | 期望 |
|------|------|
| 两轮 Run 同 conversation | 第二轮 LLM 输入含第一轮 user+assistant（script LLM 断言） |
| `max_messages` 裁剪 | 只注入最近 N 条；Run 成功 |
| Identity SQLite 重启 | 新 Open 后 List/Resolve 仍有 Bearer |
| GET messages | 顺序正确；无 JWT 明文 |
| DELETE messages | 历史空；identities 仍在 |
| 失败 Run | 历史中有失败短注；下一轮仍可进行 |
| memory 驱动 | 单测不依赖磁盘 |

---

## 11. 开放决策（已锁定）

| 决策 | 选择 |
|------|------|
| 总方案 | B：独立 Conversation Thread + 身份 SQLite 持久化 |
| 历史来源 | Message 表，非 Events 回放 |
| Tool 原文 | 不进 Message |
| 默认窗口 | 40 messages |
| 失败轮次 | 写入短注，避免空洞 |
| 清空聊天 | 可不清身份 |
| 加密 | 本版不做 |

---

## 12. 实现顺序建议（非正式计划）

1. Store：messages 表 + Identity SQLite  
2. API：messages GET/DELETE；PostRun / 终态钩子写 message  
3. Engine：注入窗口历史  
4. UI：加载历史 + 清空聊天  
5. 配置与 README  
6. 集成测试  

正式任务拆解在规格批准后由实现计划给出。
