import {
  Boxes,
  Cpu,
  Database,
  Inbox,
  LayoutGrid,
  MessageCircle,
  Network,
  Puzzle,
  Share2,
  SlidersHorizontal,
  Sparkles,
  Users,
  Webhook,
  Wrench,
  type LucideIcon,
} from 'lucide-react'

export type SettingsRole = 'operator' | 'admin'
export type SettingsGroup = 'assistant' | 'connect' | 'messaging' | 'system'
export type SettingsAccess = 'full' | 'read' | 'login' | 'locked'
export type BadgeKind =
  | 'models' | 'tools' | 'skills' | 'openapi' | 'mcp' | 'plugins'
  | 'mcpExport' | 'weixin' | 'webhook' | 'inbox' | 'store' | 'runtime'

export interface SettingsNavItem {
  to: string
  label: string
  group: SettingsGroup
  icon: LucideIcon
  desc: string
  badge?: BadgeKind
  /** Operator access level. Admin implicitly has full access to every item. */
  operator: SettingsAccess
}

export const OVERVIEW_ITEM = {
  to: '/settings',
  label: '总览',
  icon: LayoutGrid,
} as const

export const SETTINGS_GROUPS: { id: SettingsGroup; label: string }[] = [
  { id: 'assistant', label: '助手' },
  { id: 'connect', label: '连接' },
  { id: 'messaging', label: '消息' },
  { id: 'system', label: '系统' },
]

const ITEMS: SettingsNavItem[] = [
  // 助手
  {
    to: '/settings/models', label: '模型', group: 'assistant', icon: Cpu, operator: 'read',
    badge: 'models',
    desc: '管理对话与理解用的模型；「智能选择」会按任务自动切换（未来向量、音频等模型在此扩展分类）',
  },
  {
    to: '/settings/tools', label: '助手功能', group: 'assistant', icon: Wrench, operator: 'read',
    badge: 'tools',
    desc: '管理助手能调用的各项功能；可设置调用前是否需你确认或登录',
  },
  {
    to: '/settings/skills', label: '技能', group: 'assistant', icon: Sparkles, operator: 'read',
    badge: 'skills',
    desc: '可复用的操作流程，对话里输入 @ 或 / 即可调用',
  },
  // 连接
  {
    to: '/settings/openapi', label: '业务系统', group: 'connect', icon: Network, operator: 'locked',
    badge: 'openapi',
    desc: '上传一份接口文档，即可对接公司内部的各类业务系统，不用写代码',
  },
  {
    to: '/settings/mcp', label: '外部工具服务', group: 'connect', icon: Boxes, operator: 'locked',
    badge: 'mcp',
    desc: '接入标准 MCP 工具服务（本地子进程或远程 HTTP），扩展助手可用能力',
  },
  {
    to: '/settings/plugins', label: '插件', group: 'connect', icon: Puzzle, operator: 'locked',
    badge: 'plugins',
    desc: '接入独立部署的插件程序',
  },
  {
    to: '/settings/mcp-export', label: '对外提供能力', group: 'connect', icon: Share2, operator: 'locked',
    badge: 'mcpExport',
    desc: '把助手的能力以标准 MCP 服务对外开放，供其他客户端调用',
  },
  // 消息
  {
    to: '/settings/channels/weixin', label: '微信', group: 'messaging', icon: MessageCircle,
    operator: 'login', badge: 'weixin',
    desc: '接入微信账号，让客户/同事通过微信与助手对话（未来钉钉、飞书各占一张卡）',
  },
  {
    to: '/settings/webhooks', label: '消息回调', group: 'messaging', icon: Webhook, operator: 'locked',
    badge: 'webhook',
    desc: '有新消息或事件时，主动推送到你指定的地址',
  },
  {
    to: '/settings/inbox', label: '外部来信', group: 'messaging', icon: Inbox, operator: 'locked',
    badge: 'inbox',
    desc: '生成专属收件地址，外部系统来信自动变成对话',
  },
  // 系统
  {
    to: '/settings/identities', label: '账号', group: 'system', icon: Users, operator: 'full',
    desc: '管理能登录、使用助手的人员',
  },
  {
    to: '/settings/storage', label: '存储', group: 'system', icon: Database, operator: 'locked',
    badge: 'store',
    desc: '对话和数据保存在哪里（本地文件或数据库）',
  },
  {
    to: '/settings/runtime', label: '运行参数', group: 'system', icon: SlidersHorizontal,
    operator: 'read', badge: 'runtime',
    desc: '压缩、超时等运行行为，修改后即时生效',
  },
]

/** Full catalog; admin sees all 13, operator items carry their access level. */
export function settingsNavItems(_role: SettingsRole): SettingsNavItem[] {
  return ITEMS
}

/** Items rendered in the sidebar / reachable by a role: locked items are operator-invisible. */
export function visibleNavItems(role: SettingsRole): SettingsNavItem[] {
  if (role === 'admin') return ITEMS
  return ITEMS.filter((i) => i.operator !== 'locked')
}
