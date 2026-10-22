import type { ToolCatalog } from './friendlyTool'

/** True when a tool.result should show the Chat login actions (browser / skill). */
export function isLoginRequiredContent(result: unknown): boolean {
  if (result == null || typeof result !== 'object') return false
  const r = result as Record<string, unknown>
  if (r.code === 'login_required') return true

  // Fallback: raw 401/403 tool errors (before server remaps to login_required).
  const status = r.http_status
  if (status === 401 || status === 403 || status === '401' || status === '403') {
    return true
  }
  if (r.code === 401 || r.code === 403 || r.code === '401' || r.code === '403') {
    return true
  }
  const msg = typeof r.message === 'string' ? r.message.toLowerCase() : ''
  if (!msg) return false
  return (
    msg.includes('unauthorized') ||
    msg.includes('authentication is required') ||
    msg.includes('未登录') ||
    msg.includes('token已经过期') ||
    msg.includes('token已过期') ||
    msg.includes('登录已过期')
  )
}

/** Non-empty trimmed connector id, or undefined. */
export function resolveConnectorId(raw: string | undefined | null): string | undefined {
  const id = typeof raw === 'string' ? raw.trim() : ''
  return id || undefined
}

/**
 * Match backend loginmanage.NormalizeConnectorID: keep only [a-zA-Z0-9_-].
 */
export function normalizeConnectorID(id: string): string {
  return id.replace(/[^a-zA-Z0-9_-]+/g, '')
}

/**
 * Match backend loginmanage.SkillID: `login-<normalized>`, or undefined if empty.
 */
export function loginSkillID(connectorId: string): string | undefined {
  const n = normalizeConnectorID(connectorId.trim())
  if (!n) return undefined
  return `login-${n}`
}

/** Prefer connector_id stamped on the tool result; else catalog lookup. */
export function connectorIdFromLoginResult(
  result: unknown,
  toolName: string | undefined,
  catalog: ToolCatalog,
): string | undefined {
  if (result != null && typeof result === 'object') {
    const fromResult = resolveConnectorId(
      typeof (result as { connector_id?: unknown }).connector_id === 'string'
        ? (result as { connector_id: string }).connector_id
        : undefined,
    )
    if (fromResult) return fromResult
  }
  if (!toolName) return undefined
  return resolveConnectorId(catalog.find((t) => t.name === toolName)?.connector_id)
}

/** First connector id that needs browser/chat login among tool-like blocks. */
export function findLoginRequiredConnector(
  blocks: ReadonlyArray<{ kind: string; name?: string; result?: unknown }>,
  catalog: ToolCatalog,
): string | undefined {
  for (const b of blocks) {
    if (b.kind !== 'tool' || !b.name) continue
    if (!isLoginRequiredContent(b.result)) continue
    const id = connectorIdFromLoginResult(b.result, b.name, catalog)
    if (id) return id
  }
  return undefined
}

export type PendingLoginWait = {
  /** True when the run is paused for login (tool result and/or hitl.waiting). */
  active: boolean
  connectorId?: string
}

function isLoginHitlResumed(data: Record<string, unknown> | undefined): boolean {
  if (!data) return false
  if (data.kind === 'login') return true
  return data.decision === 'skip_login' || data.decision === 'skip_step'
}

/**
 * Detect a live login wait: prefer hitl.waiting kind=login (engine pause),
 * else fall back to login_required tool results + catalog connector_id.
 * A later login hitl.resumed (continue / skip) clears the wait even if the
 * earlier tool.result still says login_required.
 */
export function findPendingLoginWait(
  blocks: ReadonlyArray<{ kind: string; name?: string; result?: unknown }>,
  catalog: ToolCatalog,
  events?: ReadonlyArray<{ type: string; data?: Record<string, unknown> }>,
): PendingLoginWait {
  if (events?.length) {
    for (let i = events.length - 1; i >= 0; i--) {
      const ev = events[i]
      if (ev.type === 'hitl.resumed' && isLoginHitlResumed(ev.data)) {
        return { active: false }
      }
      if (ev.type === 'hitl.waiting' && ev.data?.kind === 'login') {
        return {
          active: true,
          connectorId: resolveConnectorId(
            typeof ev.data.connector_id === 'string' ? ev.data.connector_id : undefined,
          ),
        }
      }
    }
  }
  const fromBlocks = findLoginRequiredConnector(blocks, catalog)
  if (fromBlocks) {
    return { active: true, connectorId: fromBlocks }
  }
  for (const b of blocks) {
    if (b.kind !== 'tool' || !isLoginRequiredContent(b.result)) continue
    return { active: true }
  }
  return { active: false }
}

/**
 * Chat footer login banner: only while a live run is attached.
 * Scanning finished history would re-show the banner after the run ends,
 * because folded tool.result still says login_required and history has no
 * hitl.resumed events. Waiting runs are restored via liveRunId.
 */
export function resolveChatLoginBanner(
  liveRunId: string | null | undefined,
  liveBlocks: ReadonlyArray<{ kind: string; name?: string; result?: unknown }>,
  catalog: ToolCatalog,
  liveEvents?: ReadonlyArray<{ type: string; data?: Record<string, unknown> }>,
): PendingLoginWait {
  if (!liveRunId) return { active: false }
  return findPendingLoginWait(liveBlocks, catalog, liveEvents)
}
