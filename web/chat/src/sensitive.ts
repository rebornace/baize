/** Keys whose values must never appear in UI dumps (case-insensitive). */
export const SENSITIVE_KEYS = new Set([
  'accesstoken',
  'access_token',
  'refreshtoken',
  'refresh_token',
  'idtoken',
  'id_token',
  'token',
  'password',
  'secret',
  'authorization',
])

/** Recursively replace sensitive object fields with `[redacted]`. */
export function redactSensitive(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map(redactSensitive)
  }
  if (value && typeof value === 'object') {
    const out: Record<string, unknown> = {}
    for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
      if (SENSITIVE_KEYS.has(k.toLowerCase())) {
        out[k] = '[redacted]'
      } else {
        out[k] = redactSensitive(v)
      }
    }
    return out
  }
  return value
}