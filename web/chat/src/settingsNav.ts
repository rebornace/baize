import {
  Boxes,
  Brain,
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
import { getPack, hasPack, setPack } from './locale/pack'
import { zhPack } from './locales/zh'

if (!hasPack()) setPack(zhPack)

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

type NavCopyKey = keyof ReturnType<typeof getPack>['SETTINGS_NAV']

type NavMeta = {
  to: string
  group: SettingsGroup
  icon: LucideIcon
  badge?: BadgeKind
  operator: SettingsAccess
  labelKey: NavCopyKey
  descKey: NavCopyKey
}

const ITEM_META: NavMeta[] = [
  {
    to: '/settings/models', group: 'assistant', icon: Cpu, operator: 'read', badge: 'models',
    labelKey: 'models', descKey: 'modelsDesc',
  },
  {
    to: '/settings/tools', group: 'assistant', icon: Wrench, operator: 'read', badge: 'tools',
    labelKey: 'tools', descKey: 'toolsDesc',
  },
  {
    to: '/settings/skills', group: 'assistant', icon: Sparkles, operator: 'read', badge: 'skills',
    labelKey: 'skills', descKey: 'skillsDesc',
  },
  {
    to: '/settings/memory', group: 'assistant', icon: Brain, operator: 'full',
    labelKey: 'memory', descKey: 'memoryDesc',
  },
  {
    to: '/settings/openapi', group: 'connect', icon: Network, operator: 'locked', badge: 'openapi',
    labelKey: 'openapi', descKey: 'openapiDesc',
  },
  {
    to: '/settings/mcp', group: 'connect', icon: Boxes, operator: 'locked', badge: 'mcp',
    labelKey: 'mcp', descKey: 'mcpDesc',
  },
  {
    to: '/settings/plugins', group: 'connect', icon: Puzzle, operator: 'locked', badge: 'plugins',
    labelKey: 'plugins', descKey: 'pluginsDesc',
  },
  {
    to: '/settings/mcp-export', group: 'connect', icon: Share2, operator: 'locked', badge: 'mcpExport',
    labelKey: 'mcpExport', descKey: 'mcpExportDesc',
  },
  {
    to: '/settings/channels/weixin', group: 'messaging', icon: MessageCircle,
    operator: 'login', badge: 'weixin',
    labelKey: 'weixin', descKey: 'weixinDesc',
  },
  {
    to: '/settings/webhooks', group: 'messaging', icon: Webhook, operator: 'locked', badge: 'webhook',
    labelKey: 'webhooks', descKey: 'webhooksDesc',
  },
  {
    to: '/settings/inbox', group: 'messaging', icon: Inbox, operator: 'locked', badge: 'inbox',
    labelKey: 'inbox', descKey: 'inboxDesc',
  },
  {
    to: '/settings/identities', group: 'system', icon: Users, operator: 'full',
    labelKey: 'identities', descKey: 'identitiesDesc',
  },
  {
    to: '/settings/storage', group: 'system', icon: Database, operator: 'locked', badge: 'store',
    labelKey: 'storage', descKey: 'storageDesc',
  },
  {
    to: '/settings/runtime', group: 'system', icon: SlidersHorizontal,
    operator: 'read', badge: 'runtime',
    labelKey: 'runtime', descKey: 'runtimeDesc',
  },
]

export const OVERVIEW_ITEM = {
  to: '/settings',
  get label() {
    return getPack().SETTINGS_NAV.overview
  },
  icon: LayoutGrid,
}

export const SETTINGS_GROUPS: { id: SettingsGroup; get label(): string }[] = [
  { id: 'assistant', get label() { return getPack().SETTINGS_NAV.groupAssistant } },
  { id: 'connect', get label() { return getPack().SETTINGS_NAV.groupConnect } },
  { id: 'messaging', get label() { return getPack().SETTINGS_NAV.groupMessaging } },
  { id: 'system', get label() { return getPack().SETTINGS_NAV.groupSystem } },
]

function buildItems(): SettingsNavItem[] {
  const nav = getPack().SETTINGS_NAV
  return ITEM_META.map((m) => ({
    to: m.to,
    group: m.group,
    icon: m.icon,
    badge: m.badge,
    operator: m.operator,
    label: String(nav[m.labelKey]),
    desc: String(nav[m.descKey]),
  }))
}

/** Full catalog; admin sees all 14, operator items carry their access level. */
export function settingsNavItems(_role: SettingsRole): SettingsNavItem[] {
  return buildItems()
}

/** Items rendered in the sidebar / reachable by a role: locked items are operator-invisible. */
export function visibleNavItems(role: SettingsRole): SettingsNavItem[] {
  const items = buildItems()
  if (role === 'admin') return items
  return items.filter((i) => i.operator !== 'locked')
}
