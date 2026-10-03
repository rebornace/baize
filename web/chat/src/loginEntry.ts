/** True when a tool.result content object is the login gate payload. */
export function isLoginRequiredContent(result: unknown): boolean {
  if (result == null || typeof result !== 'object') return false
  return (result as { code?: unknown }).code === 'login_required'
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
