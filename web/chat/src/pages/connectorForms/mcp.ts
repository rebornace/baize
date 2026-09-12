import type { ConnectorInfo, MCPConfig, ToolInfo } from '../../api'
import { CONNECTORS } from '../../strings'
import { CONNECTOR_ID_RE } from './validate'
import { formatKeyValueMap, parseArgsText, parseKeyValueLines } from './lines'

export type McpTransport = 'stdio' | 'http'

export interface McpFormValues {
  id: string
  transport: McpTransport
  command: string
  argsText: string
  envText: string
  url: string
  headersText: string
}

export type McpFieldErrors = Partial<Record<'id' | 'command' | 'url' | 'env' | 'headers', string>>

const HTTP_URL_RE = /^https?:\/\/\S+$/i

export type McpValidationResult =
  | { ok: true; id: string; mcp: MCPConfig }
  | { ok: false; fieldErrors: McpFieldErrors }

export function validateMcp(v: McpFormValues): McpValidationResult {
  const fieldErrors: McpFieldErrors = {}
  const id = v.id.trim()
  if (!id) fieldErrors.id = CONNECTORS.errIdRequired
  else if (!CONNECTOR_ID_RE.test(id)) fieldErrors.id = CONNECTORS.errIdPattern

  if (v.transport === 'stdio') {
    const command = v.command.trim()
    if (!command) fieldErrors.command = CONNECTORS.errCommandRequired
    const envParsed = parseKeyValueLines(v.envText)
    if (!envParsed.ok) fieldErrors.env = CONNECTORS.errEnvLine(badLine(v.envText))
    if (Object.keys(fieldErrors).length > 0) return { ok: false, fieldErrors }
    return {
      ok: true, id,
      mcp: {
        transport: 'stdio', command,
        args: parseArgsText(v.argsText),
        env: envParsed.ok && Object.keys(envParsed.value).length > 0 ? envParsed.value : undefined,
      },
    }
  }

  const url = v.url.trim()
  if (!url) fieldErrors.url = CONNECTORS.errMcpUrlRequired
  else if (!HTTP_URL_RE.test(url)) fieldErrors.url = CONNECTORS.errMcpUrlHttp
  const headersParsed = parseKeyValueLines(v.headersText)
  if (!headersParsed.ok) fieldErrors.headers = CONNECTORS.errHeadersLine(badLine(v.headersText))
  if (Object.keys(fieldErrors).length > 0) return { ok: false, fieldErrors }
  return {
    ok: true, id,
    mcp: {
      transport: 'http', url,
      headers: headersParsed.ok && Object.keys(headersParsed.value).length > 0 ? headersParsed.value : undefined,
    },
  }
}

/** 取首个非法 KEY=VALUE 行用于错误提示。 */
function badLine(text: string): string {
  for (const line of text.split('\n')) {
    const row = line.trim()
    if (row === '') continue
    const eq = row.indexOf('=')
    if (eq <= 0 || !row.slice(0, eq).trim()) return row
  }
  return ''
}

export function mcpSummary(mcp: MCPConfig | undefined): string {
  if (!mcp) return '—'
  if (mcp.transport === 'http') return CONNECTORS.mcpHttpSummary(mcp.url ?? '')
  return CONNECTORS.mcpStdioSummary(mcp.command ?? '')
}

export function connectorToMcpForm(c: ConnectorInfo): McpFormValues {
  const mcp = c.mcp
  return {
    id: c.id,
    transport: mcp?.transport === 'http' ? 'http' : 'stdio',
    command: mcp?.command ?? '',
    argsText: (mcp?.args ?? []).join('\n'),
    envText: formatKeyValueMap(mcp?.env),
    url: mcp?.url ?? '',
    headersText: formatKeyValueMap(mcp?.headers),
  }
}

export function mcpConnectorIds(tools: ToolInfo[]): string[] {
  const seen = new Set<string>()
  const ordered: string[] = []
  for (const t of tools) {
    if (t.source !== 'mcp' || !t.connector_id || seen.has(t.connector_id)) continue
    seen.add(t.connector_id)
    ordered.push(t.connector_id)
  }
  return ordered
}
