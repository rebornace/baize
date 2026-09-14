import type { LoginEntry } from './api'

/** Case-insensitive; aligned with backend loginentry sensitive key redaction. */
const SENSITIVE_FIELD = /password|passwd|secret|token|api_key/i

/** True when a tool.result content object is the login gate payload. */
export function isLoginRequiredContent(result: unknown): boolean {
  if (result == null || typeof result !== 'object') return false
  return (result as { code?: unknown }).code === 'login_required'
}

export type LoginFieldType = 'text' | 'password'

export interface LoginEntryField {
  name: string
  type: LoginFieldType
  required: boolean
}

export function isSensitiveLoginField(name: string): boolean {
  return SENSITIVE_FIELD.test(name)
}

/** Filter login picker rows by mention query (title + tool_name). */
export function filterLoginEntries(entries: LoginEntry[], query: string): LoginEntry[] {
  const needle = query.trim().toLowerCase()
  if (needle === '') return entries
  return entries.filter(
    (e) =>
      e.title.toLowerCase().includes(needle) || e.tool_name.toLowerCase().includes(needle),
  )
}

/**
 * Entries for login_required「去登录」picker.
 * Missing/blank connector_id must NOT fall back to all connectors — returns [].
 */
export function loginPickerEntriesForConnector(
  entries: LoginEntry[],
  connectorId: string | undefined | null,
): LoginEntry[] {
  const id = typeof connectorId === 'string' ? connectorId.trim() : ''
  if (!id) return []
  return entries.filter((e) => e.connector_id === id)
}

/** Non-empty trimmed connector id, or undefined. */
export function resolveConnectorId(raw: string | undefined | null): string | undefined {
  const id = typeof raw === 'string' ? raw.trim() : ''
  return id || undefined
}

/** Derive modal inputs from a catalog entry (required names only). */
export function fieldsFromEntry(
  entry: Pick<LoginEntry, 'required' | 'parameters'>,
): LoginEntryField[] {
  const names = entry.required ?? []
  return names.map((name) => ({
    name,
    type: isSensitiveLoginField(name) ? 'password' : 'text',
    required: true,
  }))
}
