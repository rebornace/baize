import type { ToolInfo } from '../../api'
import { groupToolsTree, pathPrefixGroup } from '../../toolCatalog'
import { toolErrorText } from '../../strings'

export const HTTP_METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE']

export interface AddFormState {
  name: string
  method: string
  path: string
  title: string
  description: string
  schema: string
}

export const EMPTY_FORM: AddFormState = {
  name: '',
  method: 'GET',
  path: '',
  title: '',
  description: '',
  schema: '{}',
}

export function toolErrorLabel(err: unknown): string {
  const f = toolErrorText(err)
  return f.detail ? `${f.title} ${f.detail}` : f.title
}

export function toolRowKey(t: ToolInfo): string {
  return `${t.connector_id}:${t.name}`
}

export function prefixExpandKey(connectorId: string, prefix: string): string {
  return `${connectorId}::${prefix}`
}

export function formatMethodPath(t: ToolInfo): string {
  const method = (t.method ?? '').toUpperCase()
  const path = t.path?.trim()
  if (path) return method ? `${method} ${path}` : path
  return method
}

export function isToolEnabled(t: ToolInfo): boolean {
  return t.enabled ?? true
}

export function enabledCount(rows: ToolInfo[]): number {
  return rows.filter(isToolEnabled).length
}

export function flattenGroup(prefixes: { tools: ToolInfo[] }[]): ToolInfo[] {
  return prefixes.flatMap((p) => p.tools)
}

export function openApiConnectorIds(tools: ToolInfo[]): string[] {
  const seen = new Set<string>()
  const ordered: string[] = []
  for (const t of tools) {
    if (t.source === 'plugin') continue
    if (!t.connector_id) continue
    if (seen.has(t.connector_id)) continue
    seen.add(t.connector_id)
    ordered.push(t.connector_id)
  }
  return ordered
}

export function toggleKey(prev: Set<string>, key: string): Set<string> {
  const next = new Set(prev)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  return next
}

export function formatGroupPatchSummary(
  ok: number,
  total: number,
  failures: { name: string; reason: string }[],
): string {
  const shown = failures.slice(0, 5)
  const parts = [`已更新 ${ok}/${total}`, ...shown.map((f) => `${f.name}：${f.reason}`)]
  if (failures.length > 5) {
    parts.push(`其余 ${failures.length - 5} 条省略`)
  }
  return parts.join('；')
}

export function expandKeysForTool(t: ToolInfo): { connectorId: string; prefixKey: string } {
  const connectorId = t.connector_id || ''
  return {
    connectorId,
    prefixKey: prefixExpandKey(connectorId, pathPrefixGroup(t.path)),
  }
}

export function insertToolSorted(list: ToolInfo[], created: ToolInfo): ToolInfo[] {
  return [...list, created].sort((a, b) => a.name.localeCompare(b.name))
}

export function addExpandKey(prev: Set<string>, key: string): Set<string> {
  const next = new Set(prev)
  next.add(key)
  return next
}

export function defaultExpandedSets(tools: ToolInfo[]): {
  connectors: Set<string>
  prefixes: Set<string>
} {
  const tree = groupToolsTree(tools)
  if (tree.length !== 1) {
    return { connectors: new Set(), prefixes: new Set() }
  }
  const group = tree[0]
  return {
    connectors: new Set([group.connectorId]),
    prefixes: new Set(group.prefixes.map((p) => prefixExpandKey(group.connectorId, p.prefix))),
  }
}
