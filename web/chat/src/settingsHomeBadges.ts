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
import type { BadgeKind } from './settingsNav'

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
