import { useEffect, useState } from 'react'
import {
  getEventsWebhook,
  getInboxChannels,
  getRuntimeSettings,
  getStoreSettings,
  getWeixinSettings,
  listMCPExportIdentities,
  listModelProfiles,
  listSkills,
  listTools,
} from './api'
import type {
  EventsWebhookConfig,
  InboxChannel,
  ModelProfile,
  RuntimeKnobsView,
  SkillSummary,
  StoreSettings,
  ToolInfo,
  WeixinChannelSettings,
} from './api'
import type { BadgeKind, SettingsNavItem, SettingsRole } from './settingsNav'

export interface BadgeResult {
  tone: 'success' | 'warning' | 'neutral'
  text: string
}

/** Enabled tool count (enabled omitted/undefined counts as enabled, matching catalog semantics). */
export function enabledToolCount(tools: readonly ToolInfo[]): number {
  return tools.filter((t) => t.enabled !== false).length
}

/** Distinct connector_ids among tools whose source is in the accepted set. */
export function countConnectorsBySource(tools: readonly ToolInfo[], sources: readonly string[]): number {
  const ids = new Set<string>()
  for (const t of tools) {
    if (t.source && sources.includes(t.source) && t.connector_id) ids.add(t.connector_id)
  }
  return ids.size
}

function weixinBadge(s: WeixinChannelSettings | undefined): BadgeResult {
  if (s?.running === true) return { tone: 'success', text: '运行中' }
  if (s?.enabled) {
    switch (s.reason) {
      case 'login_required': return { tone: 'warning', text: '待登录' }
      case 'start_failed': return { tone: 'warning', text: '启动异常' }
      default: return { tone: 'warning', text: '已停用' }
    }
  }
  return { tone: 'neutral', text: '未接入' }
}

/**
 * Pure badge resolver. `data` shape depends on kind; pass the unwrapped API
 * payload. Returns null when the card should show no badge.
 */
export function resolveBadge(kind: BadgeKind, data: unknown): BadgeResult | null {
  switch (kind) {
    case 'models': {
      const n = (data as ModelProfile[]).length
      return n === 0 ? { tone: 'warning', text: '未配置' } : { tone: 'success', text: `已配置 ${n} 个` }
    }
    case 'tools': {
      const tools = data as ToolInfo[]
      if (tools.length === 0) return null
      const n = enabledToolCount(tools)
      return n === 0 ? { tone: 'warning', text: '未启用' } : { tone: 'success', text: `${n} 项可用` }
    }
    case 'skills': {
      const n = ((data as { skills: SkillSummary[] }).skills ?? []).length
      return n === 0 ? null : { tone: 'neutral', text: `${n} 个技能` }
    }
    case 'openapi': {
      const n = countConnectorsBySource(data as ToolInfo[], ['spec', 'extra'])
      return n === 0 ? { tone: 'neutral', text: '去接入' } : { tone: 'success', text: `已接 ${n} 个` }
    }
    case 'mcp':
    case 'plugins': {
      const sources = kind === 'mcp' ? ['mcp'] : ['plugin']
      const n = countConnectorsBySource(data as ToolInfo[], sources)
      return n === 0 ? null : { tone: 'success', text: `已接 ${n} 个` }
    }
    case 'mcpExport': {
      const n = (data as { id: string }[]).length
      return n === 0 ? null : { tone: 'neutral', text: `${n} 个出口` }
    }
    case 'weixin':
      return weixinBadge(data as WeixinChannelSettings)
    case 'webhook': {
      const cfg = data as EventsWebhookConfig
      return cfg.url && cfg.url.trim() !== ''
        ? { tone: 'success', text: '已设置' }
        : { tone: 'neutral', text: '未设置' }
    }
    case 'inbox': {
      const n = (data as InboxChannel[]).length
      return n === 0 ? null : { tone: 'neutral', text: `${n} 个收件地址` }
    }
    case 'store': {
      const driver = (data as StoreSettings).driver ?? ''
      return { tone: 'neutral', text: driver === 'sqlite' ? '本地文件' : driver.toLowerCase() }
    }
    case 'runtime': {
      const overridden = (data as RuntimeKnobsView).overridden ?? {}
      const custom = Object.values(overridden).some(Boolean)
      return { tone: 'neutral', text: custom ? '已自定义' : '默认' }
    }
    default: {
      const _exhaustive: never = kind
      return _exhaustive
    }
  }
}

// ---- useSettingsBadges ----

/** Kinds whose GET endpoint is admin-only; skipped (locked badge) for operators. */
const ADMIN_ONLY_KINDS = new Set<BadgeKind>([
  'webhook', 'inbox', 'store', 'mcpExport', 'openapi', 'mcp', 'plugins',
])

export type BadgeMap = Partial<Record<BadgeKind, BadgeResult | null>>

/** Kinds derived from the shared `/v0/tools` response. */
const TOOL_DERIVED_KINDS = ['tools', 'openapi', 'mcp', 'plugins'] as const

/**
 * Concurrently fetches badge data for the given nav items.
 *
 * The `/v0/tools` response is fetched at most once and reused for the
 * tools/openapi/mcp/plugins kinds. Admin-only kinds are never requested for
 * operators. Any single failed request resolves its kind to null without
 * affecting the others. Re-runs when `items`, `role` or `refreshKey` change.
 */
export function useSettingsBadges(
  items: readonly SettingsNavItem[],
  role: SettingsRole,
  refreshKey: number,
): BadgeMap {
  const [badges, setBadges] = useState<BadgeMap>({})

  // Callers may pass an inline filtered array whose identity changes every
  // render; key the fetch effect on the badge-kind set contents (plus role)
  // rather than the array reference, otherwise setBadges -> re-render -> new
  // array -> effect re-run would loop forever.
  const itemsKey = `${role}|${items.map((i) => i.badge ?? '').sort().join('|')}`

  useEffect(() => {
    let cancelled = false
    const kinds = new Set<BadgeKind>()
    for (const it of items) {
      if (!it.badge) continue
      if (role === 'operator' && ADMIN_ONLY_KINDS.has(it.badge)) continue
      kinds.add(it.badge)
    }

    async function run(): Promise<void> {
      // Shared tool list feeds tools/openapi/mcp/plugins. A rejected fetch
      // resolves to null (not []): the four derived kinds must then be null,
      // because [] would make openapi show a misleading "去接入" CTA on error.
      const needTools = TOOL_DERIVED_KINDS.some((k) => kinds.has(k))
      const toolsP: Promise<ToolInfo[] | null> = needTools
        ? listTools().then((t) => t, () => null)
        : Promise.resolve([] as ToolInfo[])
      const tasks: Promise<void>[] = []
      const next: BadgeMap = {}

      if (kinds.has('models')) {
        tasks.push(listModelProfiles()
          .then((p) => { next.models = resolveBadge('models', p) })
          .catch(() => { next.models = null }))
      }
      if (kinds.has('skills')) {
        tasks.push(listSkills()
          .then((s) => { next.skills = resolveBadge('skills', s) })
          .catch(() => { next.skills = null }))
      }
      if (kinds.has('weixin')) {
        tasks.push(getWeixinSettings()
          .then((s) => { next.weixin = resolveBadge('weixin', s) })
          .catch(() => { next.weixin = null }))
      }
      if (kinds.has('webhook')) {
        tasks.push(getEventsWebhook()
          .then((s) => { next.webhook = resolveBadge('webhook', s) })
          .catch(() => { next.webhook = null }))
      }
      if (kinds.has('inbox')) {
        tasks.push(getInboxChannels()
          .then((s) => { next.inbox = resolveBadge('inbox', s) })
          .catch(() => { next.inbox = null }))
      }
      if (kinds.has('store')) {
        tasks.push(getStoreSettings()
          .then((s) => { next.store = resolveBadge('store', s) })
          .catch(() => { next.store = null }))
      }
      if (kinds.has('runtime')) {
        tasks.push(getRuntimeSettings()
          .then((s) => { next.runtime = resolveBadge('runtime', s) })
          .catch(() => { next.runtime = null }))
      }
      if (kinds.has('mcpExport')) {
        tasks.push(listMCPExportIdentities()
          .then((s) => { next.mcpExport = resolveBadge('mcpExport', s) })
          .catch(() => { next.mcpExport = null }))
      }

      const tools = await toolsP
      if (tools === null) {
        for (const k of TOOL_DERIVED_KINDS) {
          if (kinds.has(k)) next[k] = null
        }
      } else {
        if (kinds.has('tools')) next.tools = resolveBadge('tools', tools)
        if (kinds.has('openapi')) next.openapi = resolveBadge('openapi', tools)
        if (kinds.has('mcp')) next.mcp = resolveBadge('mcp', tools)
        if (kinds.has('plugins')) next.plugins = resolveBadge('plugins', tools)
      }

      await Promise.all(tasks)
      if (!cancelled) setBadges(next)
    }

    void run()
    return () => { cancelled = true }
  }, [itemsKey, refreshKey])

  return badges
}
