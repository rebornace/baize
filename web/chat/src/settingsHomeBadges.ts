import { useEffect, useMemo, useState } from 'react'
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
import { getPack, subscribePack } from './locale/pack'
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
  const B = getPack().SETTINGS_BADGES
  if (s?.running === true) return { tone: 'success', text: B.weixinRunning }
  if (s?.enabled) {
    switch (s.reason) {
      case 'login_required': return { tone: 'warning', text: B.weixinLoginRequired }
      case 'start_failed': return { tone: 'warning', text: B.weixinStartFailed }
      default: return { tone: 'warning', text: B.weixinStopped }
    }
  }
  return { tone: 'neutral', text: B.weixinNone }
}

/**
 * Pure badge resolver. `data` shape depends on kind; pass the unwrapped API
 * payload. Returns null when the card should show no badge.
 */
export function resolveBadge(kind: BadgeKind, data: unknown): BadgeResult | null {
  const B = getPack().SETTINGS_BADGES
  switch (kind) {
    case 'models': {
      const n = (data as ModelProfile[]).length
      return n === 0
        ? { tone: 'warning', text: B.modelsNone }
        : { tone: 'success', text: B.modelsCount(n) }
    }
    case 'tools': {
      const tools = data as ToolInfo[]
      if (tools.length === 0) return null
      const n = enabledToolCount(tools)
      return n === 0
        ? { tone: 'warning', text: B.toolsNone }
        : { tone: 'success', text: B.toolsAvailable(n) }
    }
    case 'skills': {
      const n = ((data as { skills: SkillSummary[] }).skills ?? []).length
      return n === 0 ? null : { tone: 'neutral', text: B.skillsCount(n) }
    }
    case 'openapi': {
      const n = countConnectorsBySource(data as ToolInfo[], ['spec', 'extra'])
      return n === 0
        ? { tone: 'neutral', text: B.connectCta }
        : { tone: 'success', text: B.connectedCount(n) }
    }
    case 'mcp':
    case 'plugins': {
      const sources = kind === 'mcp' ? ['mcp'] : ['plugin']
      const n = countConnectorsBySource(data as ToolInfo[], sources)
      return n === 0 ? null : { tone: 'success', text: B.connectedCount(n) }
    }
    case 'mcpExport': {
      const n = (data as { id: string }[]).length
      return n === 0 ? null : { tone: 'neutral', text: B.exportCount(n) }
    }
    case 'weixin':
      return weixinBadge(data as WeixinChannelSettings)
    case 'webhook': {
      const cfg = data as EventsWebhookConfig
      return cfg.url && cfg.url.trim() !== ''
        ? { tone: 'success', text: B.webhookSet }
        : { tone: 'neutral', text: B.webhookUnset }
    }
    case 'inbox': {
      const n = (data as InboxChannel[]).length
      return n === 0 ? null : { tone: 'neutral', text: B.inboxCount(n) }
    }
    case 'store': {
      const driver = (data as StoreSettings).driver ?? ''
      return {
        tone: 'neutral',
        text: driver === 'sqlite' ? B.storeLocalFile : driver.toLowerCase(),
      }
    }
    case 'runtime': {
      const view = data as RuntimeKnobsView
      const overridden = view.overridden ?? {}
      const custom = Object.values(overridden).some(Boolean) || Boolean(view.public_base_url_overridden)
      return custom
        ? { tone: 'neutral', text: B.runtimeCustom }
        : { tone: 'neutral', text: B.runtimeDefault }
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

/** Raw payload per kind; `null` means the request failed (no badge). */
type RawMap = Partial<Record<BadgeKind, unknown | null>>

/** Kinds derived from the shared `/v0/tools` response. */
const TOOL_DERIVED_KINDS = ['tools', 'openapi', 'mcp', 'plugins'] as const

/**
 * Concurrently fetches badge data for the given nav items.
 *
 * The `/v0/tools` response is fetched at most once and reused for the
 * tools/openapi/mcp/plugins kinds. Admin-only kinds are never requested for
 * operators. Any single failed request resolves its kind to null without
 * affecting the others. Re-runs when `items`, `role` or `refreshKey` change.
 * Badge text re-resolves when the locale pack changes.
 */
export function useSettingsBadges(
  items: readonly SettingsNavItem[],
  role: SettingsRole,
  refreshKey: number,
): BadgeMap {
  const [raw, setRaw] = useState<RawMap>({})
  const [localeTick, setLocaleTick] = useState(0)

  useEffect(() => subscribePack(() => setLocaleTick((n) => n + 1)), [])

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
      // because [] would make openapi show a misleading connect CTA on error.
      const needTools = TOOL_DERIVED_KINDS.some((k) => kinds.has(k))
      const toolsP: Promise<ToolInfo[] | null> = needTools
        ? listTools().then((t) => t, () => null)
        : Promise.resolve([] as ToolInfo[])
      const tasks: Promise<void>[] = []
      const next: RawMap = {}

      if (kinds.has('models')) {
        tasks.push(listModelProfiles()
          .then((p) => { next.models = p })
          .catch(() => { next.models = null }))
      }
      if (kinds.has('skills')) {
        tasks.push(listSkills()
          .then((s) => { next.skills = s })
          .catch(() => { next.skills = null }))
      }
      if (kinds.has('weixin')) {
        tasks.push(getWeixinSettings()
          .then((s) => { next.weixin = s })
          .catch(() => { next.weixin = null }))
      }
      if (kinds.has('webhook')) {
        tasks.push(getEventsWebhook()
          .then((s) => { next.webhook = s })
          .catch(() => { next.webhook = null }))
      }
      if (kinds.has('inbox')) {
        tasks.push(getInboxChannels()
          .then((s) => { next.inbox = s })
          .catch(() => { next.inbox = null }))
      }
      if (kinds.has('store')) {
        tasks.push(getStoreSettings()
          .then((s) => { next.store = s })
          .catch(() => { next.store = null }))
      }
      if (kinds.has('runtime')) {
        tasks.push(getRuntimeSettings()
          .then((s) => { next.runtime = s })
          .catch(() => { next.runtime = null }))
      }
      if (kinds.has('mcpExport')) {
        tasks.push(listMCPExportIdentities()
          .then((s) => { next.mcpExport = s })
          .catch(() => { next.mcpExport = null }))
      }

      const tools = await toolsP
      if (tools === null) {
        for (const k of TOOL_DERIVED_KINDS) {
          if (kinds.has(k)) next[k] = null
        }
      } else {
        if (kinds.has('tools')) next.tools = tools
        if (kinds.has('openapi')) next.openapi = tools
        if (kinds.has('mcp')) next.mcp = tools
        if (kinds.has('plugins')) next.plugins = tools
      }

      await Promise.all(tasks)
      if (!cancelled) setRaw(next)
    }

    void run()
    return () => { cancelled = true }
  }, [itemsKey, refreshKey])

  return useMemo(() => {
    const next: BadgeMap = {}
    for (const [kind, data] of Object.entries(raw) as [BadgeKind, unknown | null][]) {
      next[kind] = data === null ? null : resolveBadge(kind, data)
    }
    return next
    // localeTick forces re-resolve when the UI language changes.
  }, [raw, localeTick])
}
