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
