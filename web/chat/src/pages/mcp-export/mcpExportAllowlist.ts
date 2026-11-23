import type { ToolInfo } from '../../api'

/** Mirrors internal/mcpexport/write_detect.go signals for UI prefill heuristics. */
const MCP_WRITE_SIGNALS = [
  'insert',
  'update',
  'delete',
  'drop',
  'truncate',
  'execute_write',
] as const

export function isMcpWriteTool(name: string, description?: string): boolean {
  const lowerName = name.toLowerCase()
  const lowerDesc = (description ?? '').toLowerCase()
  return MCP_WRITE_SIGNALS.some(
    (signal) => lowerName.includes(signal) || lowerDesc.includes(signal),
  )
}

export function methodIsReadOnlyHttp(method?: string): boolean {
  switch ((method ?? '').trim().toUpperCase()) {
    case 'GET':
    case 'HEAD':
      return true
    default:
      return false
  }
}

/** Prefill candidates: GET|HEAD, not force_deny, optionally skip MCP write name signals. */
export function isReadOnlyExportCandidate(
  tool: ToolInfo,
  opts?: { skipWriteSignals?: boolean },
): boolean {
  if (tool.export === 'force_deny') return false
  if (opts?.skipWriteSignals !== false && isMcpWriteTool(tool.name, tool.description)) {
    return false
  }
  return methodIsReadOnlyHttp(tool.method)
}

export function readOnlyExportCandidateNames(tools: readonly ToolInfo[]): string[] {
  return tools.filter((t) => isReadOnlyExportCandidate(t)).map((t) => t.name)
}

export function toggleAllowedTool(prev: Set<string>, name: string, checked: boolean): Set<string> {
  const next = new Set(prev)
  if (checked) next.add(name)
  else next.delete(name)
  return next
}

/** Stable catalog order for allowed_tools arrays sent to the API. */
export function mergeAllowedToolSelection(
  previous: readonly string[],
  selected: Set<string>,
  catalogOrder: readonly string[],
): string[] {
  const out: string[] = []
  const seen = new Set<string>()
  for (const name of previous) {
    if (!selected.has(name) || seen.has(name)) continue
    out.push(name)
    seen.add(name)
  }
  for (const name of catalogOrder) {
    if (!selected.has(name) || seen.has(name)) continue
    out.push(name)
    seen.add(name)
  }
  return out
}

export function catalogToolOrder(tools: readonly ToolInfo[]): string[] {
  return tools.map((t) => t.name)
}

export function selectAllConnectorTools(
  tools: readonly ToolInfo[],
  connectorId: string,
  prev: Set<string>,
): Set<string> {
  const next = new Set(prev)
  for (const t of tools) {
    if ((t.connector_id || '') === connectorId) next.add(t.name)
  }
  return next
}

export function allowedToolsFromKey(key: { allowed_tools?: string[] | null }): Set<string> {
  return new Set(key.allowed_tools ?? [])
}
