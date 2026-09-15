import { ApiError } from './api'
import { getPack, setPack, subscribePack } from './locale/pack'
import { zhPack } from './locales/zh'

// Default to Chinese so unit tests without LocaleProvider stay green.
setPack(zhPack)

export type TierId = 'light' | 'standard' | 'power'

function liveGroup<K extends keyof typeof zhPack>(key: K): (typeof zhPack)[K] {
  return new Proxy({} as object, {
    get(_target, prop) {
      if (typeof prop === 'symbol') return undefined
      const section = getPack()[key] as Record<string, unknown>
      const val = section[prop]
      return typeof val === 'function' ? (val as (...a: unknown[]) => unknown).bind(section) : val
    },
  }) as (typeof zhPack)[K]
}

export let AUTO_LABEL = zhPack.AUTO_LABEL
export let VISION_LABEL = zhPack.VISION_LABEL
export let WORKFLOW_PREPARING = zhPack.WORKFLOW_PREPARING

subscribePack((p) => {
  AUTO_LABEL = p.AUTO_LABEL
  VISION_LABEL = p.VISION_LABEL
  WORKFLOW_PREPARING = p.WORKFLOW_PREPARING
})

export const TIER_LABELS = liveGroup('TIER_LABELS')
export const ACTIONS = liveGroup('ACTIONS')
export const HITL = liveGroup('HITL')
export const WELCOME = liveGroup('WELCOME')
export const CHAT = liveGroup('CHAT')
export const MEMORY = liveGroup('MEMORY')
export const ACCOUNTS = liveGroup('ACCOUNTS')
export const STORAGE = liveGroup('STORAGE')
export const WEBHOOKS = liveGroup('WEBHOOKS')
export const RUNTIME = liveGroup('RUNTIME')
export const MODELS = liveGroup('MODELS')
export const SKILLS = liveGroup('SKILLS')
export const LOGIN_AT = liveGroup('LOGIN_AT')
export const TOOLS = liveGroup('TOOLS')
export const CONNECTORS = liveGroup('CONNECTORS')
export const MCP_EXPORTS = liveGroup('MCP_EXPORTS')
export const WEIXIN = liveGroup('WEIXIN')
export const INBOX = liveGroup('INBOX')
export const LOCALE = liveGroup('LOCALE')
export const WORKFLOW = liveGroup('WORKFLOW')

/** 内部档位 id -> 对外叫法；未知值按「标准」。 */
export function tierLabel(tier?: string): string {
  const labels = getPack().TIER_LABELS
  if (tier === 'light' || tier === 'standard' || tier === 'power') {
    return labels[tier]
  }
  return labels.standard
}

export interface FriendlyError {
  title: string
  /** 技术细节，默认收起，供「详情」展开。 */
  detail?: string
}

function isNetworkish(e: unknown): boolean {
  const msg = e instanceof Error ? e.message : String(e ?? '')
  return /failed to fetch|networkerror|load failed|network request/i.test(msg)
}

/** 把任意抛出值翻译为人话标题；未知错误附带可展开的技术 detail。 */
export function friendlyError(e: unknown): FriendlyError {
  const p = getPack()
  if (e instanceof ApiError) {
    const codeTitle = (p.CODE_TITLE as Record<string, string>)[e.code]
    if (codeTitle) return { title: codeTitle }
    if (e.status === 401 || e.status === 403) {
      return { title: p.ERRORS.unauthorized }
    }
    if (e.status >= 500) {
      return { title: p.CODE_TITLE.internal_error, detail: `${e.code}: ${e.message}` }
    }
    return { title: p.ERRORS.generic, detail: `${e.code}: ${e.message}` }
  }
  if (isNetworkish(e)) {
    return { title: p.ERRORS.network }
  }
  const detail = e instanceof Error ? e.message : String(e ?? '')
  return { title: p.ERRORS.unknown, detail: detail || undefined }
}

/** 身份来源 -> 人话；未知来源原值兜底，不吞信息。 */
export function identitySourceLabel(source: string): string {
  const map = getPack().IDENTITY_SOURCES as Record<string, string>
  return map[source] ?? source
}

/** 存储驱动 -> 人话选项；未知驱动原值兜底。提交值仍用英文 driver。 */
export function driverLabel(driver: string): string {
  return (getPack().DRIVER_LABELS as Record<string, string>)[driver] ?? driver
}

/** 把模型设置页相关异常翻译为人话标题；未知错误附技术详情。 */
export function modelErrorText(e: unknown): FriendlyError {
  const M = getPack().MODELS
  if (e instanceof ApiError) {
    if (e.code === 'not_found') return { title: M.errNotFound }
    if (e.code === 'invalid_request') return { title: M.errInvalidRequest }
    if (e.code === 'internal_error' || e.status >= 500) {
      return { title: M.errInternal, detail: `${e.code}: ${e.message}` }
    }
    return { title: M.errGeneric, detail: `${e.code}: ${e.message}` }
  }
  return friendlyError(e)
}

/** 把技能设置页相关异常翻译为人话标题；未知错误附技术详情。 */
export function skillErrorText(e: unknown): FriendlyError {
  const S = getPack().SKILLS
  if (e instanceof ApiError) {
    if (e.code === 'not_found') return { title: S.errNotFound }
    if (e.code === 'invalid_request') return { title: S.errInvalidRequest }
    if (e.code === 'internal_error' || e.status >= 500) {
      return { title: S.errInternal, detail: `${e.code}: ${e.message}` }
    }
    return { title: S.errGeneric, detail: `${e.code}: ${e.message}` }
  }
  return friendlyError(e)
}

/** 把助手功能页相关异常翻译为人话标题；未知错误附技术详情。 */
export function toolErrorText(e: unknown): FriendlyError {
  const T = getPack().TOOLS
  if (e instanceof ApiError) {
    if (e.code === 'not_found') return { title: T.errNotFound }
    if (e.code === 'invalid_request') return { title: T.errInvalidRequest }
    if (e.code === 'internal_error' || e.status >= 500) {
      return { title: T.errInternal, detail: `${e.code}: ${e.message}` }
    }
    return { title: T.errGeneric, detail: `${e.code}: ${e.message}` }
  }
  return friendlyError(e)
}

/** 外部来信通道列表上的密钥尾号展示。 */
export function inboxSecretHint(tail: string): string {
  return getPack().inboxSecretHint(tail)
}

/** 把连接器相关异常翻译为 {title, detail?}；未知错误给出通用标题与可展开技术详情。 */
export function connectorErrorText(e: unknown): FriendlyError {
  const p = getPack()
  if (e instanceof ApiError) {
    const byCode = p.CONNECTOR_CODE_TITLES[e.code as keyof typeof p.CONNECTOR_CODE_TITLES]
    if (byCode) return { title: byCode }
    if (e.code === 'invalid_mcp') {
      if (/command is required|unsupported mcp transport/.test(e.message)) {
        return { title: p.CONNECTOR_EXTRA.incompleteConfig }
      }
      if (/url is required/.test(e.message)) return { title: p.CONNECTOR_EXTRA.needRemoteUrl }
      if (/401|403|unauthor|forbidden/i.test(e.message)) {
        return {
          title: p.CONNECTOR_EXTRA.needAuth,
          detail: `${e.code}: ${e.message}`,
        }
      }
      return { title: p.CONNECTORS.errMcpConnect, detail: `${e.code}: ${e.message}` }
    }
    if (/base_url is required/.test(e.message)) return { title: p.CONNECTOR_EXTRA.needBaseUrl }
    if (/spec is required/.test(e.message)) return { title: p.CONNECTOR_EXTRA.needSpec }
    return { title: p.CONNECTORS.errorGeneric, detail: `${e.code}: ${e.message}` }
  }
  return friendlyError(e)
}

/** 把「对外提供能力」页身份/密钥接口异常翻译为人话标题；未知错误附技术详情。 */
export function mcpExportErrorText(e: unknown): FriendlyError {
  const M = getPack().MCP_EXPORTS
  if (e instanceof ApiError) {
    if (e.code === 'not_found') {
      if (/key/i.test(e.message)) return { title: M.errKeyNotFound }
      return { title: M.errIdentityNotFound }
    }
    if (e.code === 'internal_error') {
      return { title: M.errInternal, detail: `${e.code}: ${e.message}` }
    }
    if (e.code === 'invalid_request') {
      if (/name is required/.test(e.message)) return { title: M.errNameRequired }
      if (/identity/.test(e.message)) return { title: M.errIdentityRequired }
      return { title: M.errInvalidRequest, detail: `${e.code}: ${e.message}` }
    }
    return { title: M.errGeneric, detail: `${e.code}: ${e.message}` }
  }
  return friendlyError(e)
}

/** 连接卡上的一行权限摘要；两项皆空返回 null。 */
export function permissionSummary(loginNames: string[], approvalNames: string[]): string | null {
  const p = getPack()
  const parts: string[] = []
  if (loginNames.length > 0) parts.push(p.permissionSummaryLogin(loginNames.length))
  if (approvalNames.length > 0) parts.push(p.permissionSummaryApproval(approvalNames.length))
  return parts.length > 0 ? parts.join(' · ') : null
}
