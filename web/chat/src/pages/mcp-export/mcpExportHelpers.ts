import type { MCPExportIdentity, MCPExportKey, ToolExportMode, ToolInfo } from '../../api'
import { MCP_EXPORTS } from '../../strings'
import { formatKeyValueMap, parseKeyValueLines } from '../connectorForms/lines'

export const EXPORT_OPTIONS: { value: ToolExportMode; label: string }[] = [
  { value: 'default', label: MCP_EXPORTS.exportDefault },
  { value: 'force_allow', label: MCP_EXPORTS.exportForceAllow },
  { value: 'force_deny', label: MCP_EXPORTS.exportForceDeny },
]

export function toolExportMode(t: ToolInfo): ToolExportMode {
  if (t.export === 'force_allow' || t.export === 'force_deny') return t.export
  return 'default'
}

export interface IdentityFormState {
  name: string
  scheme: string
  headersText: string
}

export const EMPTY_IDENTITY_FORM: IdentityFormState = {
  name: '',
  scheme: '',
  headersText: '',
}

export function mcpExportEndpointUrl(origin: string, endpointPath: string): string {
  const base = origin.replace(/\/$/, '')
  const path = endpointPath.startsWith('/') ? endpointPath : `/${endpointPath}`
  return `${base}${path}`
}

export function identityToForm(identity: MCPExportIdentity): IdentityFormState {
  return {
    name: identity.name,
    scheme: identity.scheme ?? '',
    headersText: formatKeyValueMap(identity.headers),
  }
}

export function validateIdentityForm(
  form: IdentityFormState,
):
  | { ok: true; name: string; scheme: string; headers: Record<string, string> }
  | { ok: false; message: string } {
  const name = form.name.trim()
  if (!name) {
    return { ok: false, message: MCP_EXPORTS.errNameRequired }
  }
  const headersParsed = parseKeyValueLines(form.headersText)
  if (!headersParsed.ok) {
    // parseKeyValueLines 的消息含旧文案，这里自行定位首个非法行，保证文案统一来自 MCP_EXPORTS。
    const badRow = form.headersText
      .split('\n')
      .map((line) => line.trim())
      .find((row) => row !== '' && (row.indexOf('=') <= 0 || row.slice(0, row.indexOf('=')).trim() === ''))
    return { ok: false, message: MCP_EXPORTS.errBadHeaderLine(badRow ?? '') }
  }
  return {
    ok: true,
    name,
    scheme: form.scheme.trim(),
    headers: headersParsed.value,
  }
}

export function isKeyActive(key: MCPExportKey): boolean {
  return key.revoked_at == null || key.revoked_at === ''
}

export type ConfirmState =
  | { kind: 'identity'; id: string; name: string }
  | { kind: 'key'; id: string; name: string; prefix: string }
  | null
