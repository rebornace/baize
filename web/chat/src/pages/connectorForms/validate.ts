import { CONNECTORS } from '../../strings'
import type { ConnectionFormValues, FieldErrors } from './types'

export const CONNECTOR_ID_RE = /^[a-z][a-z0-9_-]{0,63}$/
const HTTP_URL_RE = /^https?:\/\/\S+$/i

export type ValidationResult =
  | { ok: true; id: string; baseUrl: string }
  | { ok: false; fieldErrors: FieldErrors }

export function validateConnection(v: ConnectionFormValues): ValidationResult {
  const fieldErrors: FieldErrors = {}
  const id = v.id.trim()
  const baseUrl = v.baseUrl.trim()

  if (!id) fieldErrors.id = CONNECTORS.errIdRequired
  else if (!CONNECTOR_ID_RE.test(id)) fieldErrors.id = CONNECTORS.errIdPattern

  if (!baseUrl) fieldErrors.baseUrl = CONNECTORS.errBaseUrlRequired
  else if (!HTTP_URL_RE.test(baseUrl)) fieldErrors.baseUrl = CONNECTORS.errBaseUrlHttp

  if (v.kind === 'openapi' && !v.hasSpec && !v.editing) {
    fieldErrors.spec = CONNECTORS.errSpecRequired
  }

  if (Object.keys(fieldErrors).length > 0) return { ok: false, fieldErrors }
  return { ok: true, id, baseUrl }
}
