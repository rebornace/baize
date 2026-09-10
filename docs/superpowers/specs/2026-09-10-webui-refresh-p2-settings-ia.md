# P2 设置信息架构（Settings IA）设计规格

- 日期：2026-09-10
- 阶段：WebUI 体验刷新 P2（路线图见 `2026-09-09-webui-experience-refresh-design.md`）
- 状态：待评审
- 范围：设置首页（人话卡片总览）+ 侧边导航分组 + 卡片实时状态徽标 + 运营人员权限放开（少量后端 ACL）

## 1. 目标与非目标

### 1.1 目标

1. 管理员打开「设置」首先看到**人话卡片首页**：每个设置项是一张「图标 + 中文名 + 一句话说明 + 状态徽标」的卡片，按助手 / 连接 / 消息 / 系统四组排列，点击进入对应页。
2. 全部命名去技术化（不出现 OpenAPI、Tools、Channels 等英文术语作为标题）。
3. 卡片状态来自现有只读接口的实时聚合，失败静默降级，**不新增后端只读接口**。
4. 首页承担「配置健康总览 + 新手引导」：模型未配置时给出醒目提示与直达入口；不做向导弹窗、不加示例数据。
5. 运营人员（operator）也有设置首页与合理访问权：
   - 可只读查看模型、助手功能、技能、运行参数；
   - 可查看微信状态并扫码登录自己的微信账号；
   - 对仅管理员可配置的项可见卡片全貌（知道系统里有什么），但加锁不可进入。

### 1.2 非目标（P2 明确不做）

- 不重构任何设置子页面的内部表单与布局（P3 做人话化重构）；仅对运营可见的页面做控件级只读 gate。
- 不修改任何接口的返回结构；不新增后端只读接口。
- locked 页面（业务系统、MCP、插件、MCP 导出、消息回调、外部来信、存储）本轮不做只读模式，P3 统一处理。
- 不做徽标轮询、新手向导弹窗、示例数据。
- 不做渠道多实例管理 UI，仅让导航数据结构与卡片布局天然容纳未来多渠道（每渠道一项一卡）。
- 账号卡不挂状态徽标（现有 `/v0/me` 数据不足以表达有意义状态，避免误导）。

## 2. 信息架构与命名

### 2.1 分组与卡片（侧边导航与首页卡片的唯一数据源）

| 分组 | 卡片名 | 路由 | 运营访问级别 | 卡片说明 |
|---|---|---|---|---|
| 助手 assistant | 模型 | `/settings/models` | read | 管理对话与理解用的模型；「智能选择」会按任务自动切换（未来向量、音频等模型在此扩展分类） |
| 助手 assistant | 助手功能 | `/settings/tools` | read | 助手能调用的功能开关，如查订单、建工单；可设置是否需你确认或登录 |
| 助手 assistant | 技能 | `/settings/skills` | read | 可复用的操作流程，对话里输入 `@` 或 `/` 即可调用 |
| 连接 connect | 业务系统 | `/settings/openapi` | locked | 粘贴接口文档即可连上公司的订单、工单等系统，不用写代码 |
| 连接 connect | MCP | `/settings/mcp` | locked | 接入标准 MCP 工具服务，扩展助手能力 |
| 连接 connect | 插件 | `/settings/plugins` | locked | 接入独立部署的插件程序 |
| 连接 connect | MCP 导出 | `/settings/mcp-export` | locked | 把助手能力以标准 MCP 方式开放给其他客户端 |
| 消息 messaging | 微信 | `/settings/channels/weixin` | login | 接入微信账号，让客户/同事通过微信与助手对话（未来钉钉、飞书各占一张卡） |
| 消息 messaging | 消息回调 | `/settings/webhooks` | locked | 有新消息或事件时，主动推送到你指定的地址 |
| 消息 messaging | 外部来信 | `/settings/inbox` | locked | 生成专属收件地址，外部系统来信自动变成对话 |
| 系统 system | 账号 | `/settings/identities` | full | 管理能登录、使用助手的人员 |
| 系统 system | 存储 | `/settings/storage` | locked | 对话和数据保存在哪里（本地文件或数据库） |
| 系统 system | 运行参数 | `/settings/runtime` | read | 压缩、超时等运行行为，修改后即时生效 |

命名决策记录：

- 原「业务系统接口（OpenAPI）」太僵硬 → 定名「业务系统」，说明文字承载“通过接口文档连接”的含义。
- 渠道按**实例**命名：现在只有「微信」一张卡；导航数据每渠道一项，未来加飞书/钉钉自动多出卡片，不预设抽象统称。
- 原「能做什么（Tools）」语义不清 → 定名「助手功能」。
- 模型页标题用「模型」而非「AI 模型」，为未来向量、音频等模型类型留扩展空间；说明文字注明当前管理对话/理解类模型。
- 「运行时」改为「运行参数」。

### 2.2 访问级别定义

| 级别 | 含义 | 首页卡 | 侧边栏 |
|---|---|---|---|
| `full` | 正常可进可改（账号页对两种角色均如此） | 可点 | 可见 |
| `read` | 可进入只读页，所有写控件隐藏/禁用 | 可点 | 可见 |
| `login` | 微信专用：可看状态、可扫码登录/轮询；退出登录、进程启停、改配置仅管理员 | 可点 | 可见 |
| `locked` | 仅管理员可配置 | 管理员可点；运营可见卡片+锁标、不可点、不参与 Tab 聚焦 | 仅管理员可见 |

侧边栏分组裁剪：运营视图中「连接」组四项全部 locked → 该组标题与项目都不渲染。

## 3. 后端 ACL 改动

仅放开 4 条规则（`internal/controlplane/acl.go`），其余一律不动：

| 接口 | 现状 | 改为 | 安全性说明 |
|---|---|---|---|
| `GET /v0/tools` | RoleAdmin | RoleOperator | 工具清单不含密钥；用于只读展示助手功能 |
| `GET /v0/settings/channels/{name}` | RoleAdmin | RoleOperator | 返回体仅 `assignee/agent_id/allowlist/enabled/running/reason`，无任何密钥 |
| `POST /v0/settings/channels/{name}/login/start` | RoleAdmin | RoleOperator | 扫码登录运营必需；只返回 ticket 与二维码 URL |
| `GET /v0/settings/channels/{name}/login/status` | RoleAdmin | RoleOperator | 仅查询扫码轮询状态 |

仍严格仅 RoleAdmin（运营调用返回 403，前端以已有「权限」错误文案 Toast 兜底）：

- 微信：`PUT …/channels/{name}`、`logout`、`process/start|stop|restart`；
- 工具/技能/模型的一切写操作（POST/PATCH/DELETE）；
- `GET/PUT /v0/settings/events-webhook`、`inbox-channels`、`store`、`mcp-export*`、`credentials`；
- `GET /v0/skills/{id}`（详情）仍 admin；运营只用 `GET /v0/skills` 列表（本就 RoleOperator）。

后端测试：在现有 ACL 测试中新增用例，断言上述 4 个接口 operator 放行，且微信 PUT/logout/process 与 webhook/store/credentials GET 对 operator 仍为 admin 要求。

## 4. 卡片状态徽标

### 4.1 徽标数据映射（全部复用 `api.ts` 现有函数，无新接口）

| 卡片 | 数据函数 | 徽标规则 |
|---|---|---|
| 模型 | `listModelProfiles()` | 0 个 → warn「未配置」；>0 → ok「已配置 n 个」 |
| 助手功能 | `listTools()` | 启用项 n>0 → ok「n 项可用」；有工具但全部停用 → warn「未启用」；工具总数 0 → 不显示徽标 |
| 技能 | `listSkills()`（返回 `{skills}`） | n>0 → muted「n 个技能」；0 个不显示徽标 |
| 业务系统 | `listTools()` 中 `source ∈ {spec,extra}` | 去重 connector 后 n>0 → ok「已接 n 个」；0 → muted「去接入」 |
| MCP | `listTools()` 中 `source === 'mcp'` | n>0 → ok「已接 n 个」；0 不显示徽标 |
| 插件 | `listTools()` 中 `source === 'plugin'` | n>0 → ok「已接 n 个」；0 不显示徽标 |
| MCP 导出 | `listMCPExportIdentities()` | n>0 → muted「n 个出口」；0 不显示 |
| 微信 | `getWeixinSettings()` | `running===true` → ok「运行中」；`enabled` 但未运行 → warn（文案随 `reason`：login_required→「待登录」，start_failed→「启动异常」，其他→「已停用」）；未启用 → muted「未接入」。字段缺失按「未接入」降级 |
| 消息回调 | `getEventsWebhook()` | `url` 非空 → ok「已设置」；空 → muted |
| 外部来信 | `getInboxChannels()` | n>0 → muted「n 个收件地址」；0 不显示 |
| 存储 | `getStoreSettings()` | muted 显示 driver 人话名：`sqlite`→「本地文件」，其余原样小写 |
| 运行参数 | `getRuntimeSettings()` | `overridden` 非空 → muted「已自定义」；否则 muted「默认」 |
| 账号 | — | 无徽标 |

`source` 字面量以 `ToolInfo.source` 实际取值为准（已核实为 `spec`/`extra`/`mcp`/`plugin`），在实现处加注释说明与三个连接器设置页的筛选保持一致。

### 4.2 解析与获取分层

- `resolveBadge(kind, data): { tone: 'ok'|'warn'|'muted'; text: string } | null`：**纯函数**，输入接口返回数据，输出徽标或 null（不显示）。每种 kind 的三态、空数据、字段缺失都在单测覆盖。
- `useSettingsBadges(items, role, refreshKey)` hook：入参含当前角色；从 items 收集去重的 badge kind，并**跳过该角色无 GET 权限的 kind**（见 4.3）；挂载与 `refreshKey` 变化时**并发**发 GET；单个 kind 失败 → 该 kind 记为 null（静默，不弹错、不阻塞其他卡）。
- 徽标位初始为小 Spinner（复用 P0 Spinner），数据回来后替换；失败则留空（不显示 Spinner 常驻）。
- 完成度提示条复用 `models` 同一批数据，不额外发请求。
- 刷新时机：进入首页拉一次；路由切回首页（组件重新激活）自动重拉；页头「刷新状态」按钮递增 `refreshKey` 手动重拉。微信状态不轮询。

### 4.3 运营视图的徽标降级

运营对 locked 卡片的数据源无 GET 权限（webhook/inbox/store/mcp-export 等）。这些 kind 在运营上下文不发请求，卡片统一显示 muted「仅管理员」锁标，不产生 403 噪音。运营可访问的 read/login 卡（models/tools/skills/runtime/weixin）正常拉取真实徽标。

## 5. 视觉与交互

### 5.1 路由

- `/settings` 的 index 路由由现有的「跳转组件」替换为真正的 `SettingsHome`（两种角色都渲染首页，不再跳转）。
- 侧边导航「设置」标题下方新增第一项「总览」→ `/settings`（end 匹配高亮），其下细分隔线后接四个分组。

### 5.2 首页结构（自上而下）

1. 页头：大标题「设置」+ 克制副标题「管理助手的模型、能力和对外连接」；右侧低调的「刷新状态」图标按钮（lucide RefreshCw，加载中转圈）。
2. 完成度提示条：仅当模型数为 0 出现，使用 `--warning` 浅底警示样式：
   - 管理员：文案「先添加一个模型，助手才能开始对话」+ 主按钮「去添加」→ `/settings/models`；
   - 运营：文案「还没有可用模型，请联系管理员添加」，无按钮。
   - 模型配置后重拉数据，提示条自动消失。
3. 分组卡片区：四组依次排列；每组一个小号灰色组标题（助手 / 连接 / 消息 / 系统，与侧边栏一致）。
4. 卡片栅格：`repeat(auto-fill, minmax(248px, 1fr))`，gap 取 `--space-4`；平板两列、手机单列。

### 5.3 卡片

复用 P0 `components/ui/Card`（已支持 icon/trailing/整卡可点/Enter+Space 键盘激活）：

- 布局：头部行左侧 lucide 图标（20px，`--text-2` 色，不调色避免彩虹图标）+ 卡片名；右侧 `trailing` 放徽标。下方一句说明（`--text-2`，最多两行）。
- 徽标：新建 `components/ui/Badge.tsx`，三态 `ok`（绿底绿字）、`warn`（琥珀底琥珀字）、`muted`（灰底灰字），全部使用 tokens，暗色下对比度满足 WCAG AA（小文本 4.5:1，实现后抽测）。
- 可点卡 hover：`--hover` 底 + 描边轻微加强；无位移动画。
- locked 卡（运营）：不挂 onClick、无 role/tabIndex（不进 Tab 序列），视觉降低不透明度并在 trailing 显示 Lock 图标 +「仅管理员」；cursor 为 default。
- 焦点：可点卡键盘聚焦使用 `--focus-ring`。

### 5.4 侧边导航

- 管理员：总览 + 分隔线 + 四分组（组标题小号灰色，组间 `--space-4` 间距），共 13 项；运营：总览 + 助手 3 项（模型/助手功能/技能）+ 消息 1 项（微信）+ 系统 2 项（运行参数/账号）；「连接」组整组不渲染。
- 运营在账号页之外看到的导航项均可进入（read/login/full），不存在点了 403 的导航项。
- 移动端沿用 P0 抽屉导航，分组结构与桌面一致。
- 现有「返回聊天」链接与主题切换按钮位置、行为保持不变。

### 5.5 只读页面控件 gate（仅控件级，不重排布局）

以下页面读取 `useGate().role`，运营访问时：

- `ModelSettings`：隐藏「添加模型」表单、每张卡的编辑/删除/保存按钮与所有输入控件（disabled 或不渲染）；列表正常展示。
- `ToolsSettings`：隐藏启用开关、require_approval/require_login 编辑、连接器管理入口；列表正常展示。
- `SkillsSettings`：隐藏新建/删除按钮与上传表单；列表与详情查看正常。
- `RuntimeSettings`：隐藏 PATCH 保存与单项覆盖重置按钮；当前有效值只读展示。
- `WeixinChannelSettings`：登录区（扫码开始、登录状态轮询、二维码）运营可用；配置表单（agent_id/assignee/allowlist/enabled PUT）、退出登录、进程启动/停止/重启按钮仅管理员渲染。

写接口仍以后端 403 为最终防线；控件隐藏只是体验层。若运营通过残留交互触发 403，展示 `friendlyError` 的通用权限文案。

## 6. 工程改动清单

| 文件 | 改动 |
|---|---|
| `internal/controlplane/acl.go` | 4 条规则 RoleAdmin → RoleOperator |
| `internal/controlplane/*_test.go`（或现有 ACL 测试文件） | 新增放行/未放行用例 |
| `web/chat/src/settingsNav.ts` | 重写：分组、图标、说明、访问级别、badge kind 声明；导出 `SETTINGS_GROUPS` |
| `web/chat/src/settingsNav.test.ts` | 更新为新结构的角色/分组/唯一性断言 |
| `web/chat/src/settingsHomeBadges.ts`（新） | `BadgeKind` 类型、`resolveBadge` 纯函数、kind→取数映射、`useSettingsBadges(items, role, refreshKey)` |
| `web/chat/src/settingsHomeBadges.test.ts`（新） | 纯函数全态 + hook 并发/失败降级（jsdom + fetch mock） |
| `web/chat/src/pages/SettingsHome.tsx`（新） | 页头、刷新、完成度条、分组栅格、卡片渲染 |
| `web/chat/src/pages/SettingsHome.test.tsx`（新） | 两角色渲染、提示条、locked 卡不可点、导航跳转 |
| `web/chat/src/pages/SettingsLayout.tsx` | 总览入口、分组渲染、按角色过滤 |
| `web/chat/src/components/ui/Badge.tsx`（新）+ 样式 | ok/warn/muted 三态徽标 |
| `web/chat/src/styles/settings.css`（新，并入 main.tsx import） | 首页栅格、组标题、提示条、locked 卡；设置页通用样式仍留 style.css |
| `web/chat/src/main.tsx` | index 路由改为 `<SettingsHome/>`，删除 SettingsIndex 跳转 |
| `ModelSettings.tsx` / `ToolsSettings.tsx` / `SkillsSettings.tsx` / `RuntimeSettings.tsx` / `WeixinChannelSettings.tsx` | role gate 写控件 |
| 上述 5 页对应测试 | 运营视图不渲染写控件的断言 |
| `internal/ui/dist/**` | `npm run build` 重建嵌入产物 |

不新增 npm 依赖（lucide-react 已在 P0 引入）。

## 7. 测试策略

### 7.1 后端（Go）

- ACL：4 个新放开接口以 operator 身份返回非 403；微信 PUT、logout、process/start|stop|restart、webhook/inbox/store/credentials GET 以 operator 身份仍被拒。

### 7.2 前端（vitest，纯函数 + jsdom）

- `settingsNav`：admin 13 项+总览、operator 6 项（模型/助手功能/技能/微信/运行参数/账号）且无连接组；所有 `to` 唯一；每项 label/desc/icon/group 齐全；图标为合法组件。
- `resolveBadge`：每种 kind 的 ok/warn/muted/null 分支；微信 reason 三种取值；store driver 映射；skills/webhook 等空值不显示。
- `useSettingsBadges`：kind 去重并发；单个 reject 不影响其他；运营上下文 locked kind 不发请求。
- `SettingsHome`：admin 全卡可点且 href 正确；模型 0 时出现「去添加」；operator 模型 0 时提示无按钮；locked 卡无 role=button、不可聚焦；徽标 Spinner→内容的替换。
- `SettingsLayout`：admin 见总览与四组标题；operator 无连接组、无 locked 导航项。
- 五个只读页：operator 渲染后 DOM 中不存在新增/删除/保存/开关/进程类控件（按按钮文案或 testid 断言）。

### 7.3 人工走查（实现完成后）

桌面与手机视口、亮色与暗色各一遍：登录运营账号验证微信扫码流程可用、只读页无写控件、locked 卡不可点；登录管理员验证完整跳转与徽标实时性；断网/接口报错时卡片静默降级无红屏。


