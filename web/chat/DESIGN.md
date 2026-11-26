# Chat UI 设计规范

面向 `web/chat` 的界面改动。令牌与规则以本文件 + `src/styles/tokens.css` 为准；
评审基准还包括 `baize-demo-ui` 技能与
`docs/superpowers/specs/2026-09-09-webui-experience-refresh-design.md`。

## 1. 目标

1. 第一眼愿意看，能直接录 30–60 秒演示。
2. 人话优先：默认露出常用名，系统名词/命令/错误码进二级或开发者区。
3. 双受众：终端用户能自己用，客户和决策者看视频能懂在干什么。
4. 明暗主题都能录，对比度达标。

## 2. 令牌

只允许引用 `tokens.css` 的变量；`css.guard.test.ts` 会拦截未定义变量、tokens 外色值和 `!important`。

### 颜色角色

| 变量 | 用途 |
|------|------|
| `--bg` / `--surface` / `--surface-2` / `--sidebar` | 页面底色、卡片、次级面、侧栏 |
| `--border` / `--border-strong` | 常规分隔、强调边 |
| `--text` / `--text-muted` / `--text-faint` | 正文、说明、弱化 |
| `--accent` / `--accent-soft` / `--on-accent` | 主操作、选中态 |
| `--success` / `--warning` / `--danger` + `-soft` | 语义状态 |

- `--text-faint` 只用于 12px 以上的装饰性文字；正文和元信息用 `--text-muted`。
- 语义色做徽章时，文字色必须保证 ≥4.5:1，必要时改描边式徽章。

### 字号（只取这五档）

| 变量 | 值 | 用途 |
|------|----|------|
| `--text-display-size` | 22px | 页面主标题 |
| `--text-title-size` | 18px | 区块/卡片标题 |
| `--text-body-size` | 14px | 正文、表单 |
| `--text-small-size` | 13px | 次要说明、侧栏条目 |
| `--text-meta-size` | 12px | 时间、计数、角标 |

新增字号前先问：能否用现有档位 + 字重/颜色解决。

### 间距与圆角

- 间距 4px 基准：`--space-1` 4 / `-2` 8 / `-3` 12 / `-4` 16 / `-5` 20 / `-6` 24 / `-8` 32 / `-10` 40。
- 圆角：控件 `--radius-sm` 8 或 `--radius` 10；卡片 `--radius-lg` 14；气泡 `--radius-md` 12；
  胶囊 `--radius-pill`。

## 3. 页面骨架

```
SettingsLayout
└── PageHeader（标题 + 一句说明 + 右侧操作）
    └── settings-panel（阅读宽度上限）
        └── Card / 表单区块
```

- 设置页统一用 `PageHeader` + `settings-panel`；不要自造页头。
- 同一页的区块标题只用一种层级：要么都用 `Card title`，要么都用 `settings-subheading`。
- 空状态放在列表应在的位置，不要挂在创建表单下方。
- 移动栏已显示页面名时，页面内不再重复一级标题。

## 4. 组件

| 场景 | 用 |
|------|----|
| 主/次/幽灵/危险操作 | `Button`（`variant`） |
| 可点击卡片 | `Card`（自带键盘支持） |
| 表单 | `Field` + `Input` / `Textarea` / `Select` |
| 开关、勾选 | 统一勾选组件；禁止裸 `<input type="checkbox">` 裸露系统样式 |
| 行内更多操作 | `DropdownMenu` |
| 弹窗 | `Modal`（已有焦点陷阱 / Escape / 焦点恢复） |
| 反馈 | `Badge` / `Toast` / `EmptyState` / `Spinner` |

规则：

- 原生 `<select>` 必须带 `className` 或改用 `Select`；原生 `details` 的三角不是设计资产，需自绘或收进开发者区。
- 链接样式用于 `<button>` 时必须有按钮 reset（`.settings-link` 已补），不要让系统边框漏出来。
- 图标只用 `lucide-react`，尺寸 12/14/16/20，`currentColor`。不要用 `✕`、`✓`、`▶`、`▼` 等裸字符。
- 危险操作不要做成卡片里最抢眼的实心按钮；用 `danger` 文本按钮 + 确认。

## 5. 文案

- 按钮/导航：动词 + 结果（「加入评测」「安装技能」）。
- 帮助文案 ≤2 句；技术细节进「高级 / 开发者」折叠区。
- 主路径不出现：内部命令（`/reload`）、工具名（`activate_skill`）、错误码（`ollama_not_running`）、
  主机名、goroutine、DB 连接数。
- 中英 locale 同步改；中文以客户能听懂为准，英文用常见 SaaS 词。

## 6. 响应式

| 宽度 | 期望 |
|------|------|
| >1200 | 设置卡片 3 列 |
| 721–1200 | 2 列 |
| ≤720 | 1 列；侧栏抽屉 |

- 卡片栅格用 `repeat(auto-fit, minmax(280px, 1fr))` 或明确的 1120 断点，禁止在 1024 仍挤 3 列。
- 会话列表行内操作不得常驻占宽；用悬停浮层或溢出菜单。

## 7. 验收

每个 UI 批次至少提供：

- 1440×900、1024×768、768×900 三档截图（明暗各一套，暗色至少覆盖改动页）。
- `npx tsc --noEmit`、`npx eslint src`、`npx vitest run`、`npm run build` 全绿。
- 对比度自查：正文 ≥4.5:1，大字 ≥3:1。

## 8. 批次状态

| 批次 | 内容 | 状态 |
|------|------|------|
| UI-0 | 补齐缺失令牌、CSS 守卫、本规范 | 已落地 |
| UI-1 | 会话行操作浮层、侧栏底部导航分组、首屏去 `/reload` | 已落地 |
| UI-2 | 原生控件替换、表单标签、告警类型中文化 | 已落地 |
| UI-3 | 栅格/页宽、导航层级、系统组位置 | 已落地 |
| UI-4 | 去调试输出、文案压缩、i18n 残留 | 已落地 |
| UI-5 | 对比度与可访问性收口 | 已落地 |
