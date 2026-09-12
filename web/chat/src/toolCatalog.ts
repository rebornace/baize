import type { ToolInfo } from './api'

export function canDeleteCatalogTool(source: string): boolean {
  return source === 'extra'
}

const VERSION_SEG = /^v\d+(\.\d+)*$/i

export function pathPrefixGroup(path: string | undefined): string {
  if (path == null || path.trim() === '') return '其他'
  const segs = path.split('/').filter((s) => s.length > 0)
  let i = 0
  while (i < segs.length) {
    const seg = segs[i]
    if (seg.toLowerCase() === 'api' || VERSION_SEG.test(seg)) {
      i += 1
      continue
    }
    break
  }
  if (i >= segs.length) return '其他'
  return `/${segs[i]}`
}

export function toolMatchesQuery(t: ToolInfo, q: string): boolean {
  const needle = q.trim().toLowerCase()
  if (needle === '') return true
  const hay = [t.title, t.name, t.path, t.description, t.method]
  return hay.some((p) => (p ?? '').toLowerCase().includes(needle))
}

export interface ToolPrefixGroup {
  prefix: string
  tools: ToolInfo[]
}

export interface ConnectorGroup {
  connectorId: string
  prefixes: ToolPrefixGroup[]
}

export function groupToolsTree(tools: ToolInfo[]): ConnectorGroup[] {
  const connectorOrder: string[] = []
  const byConnector = new Map<string, ToolInfo[]>()
  for (const t of tools) {
    const id = t.connector_id || ''
    if (!byConnector.has(id)) {
      connectorOrder.push(id)
      byConnector.set(id, [])
    }
    byConnector.get(id)!.push(t)
  }
  return connectorOrder.map((connectorId) => {
    const rows = byConnector.get(connectorId) ?? []
    const prefixOrder: string[] = []
    const byPrefix = new Map<string, ToolInfo[]>()
    for (const t of rows) {
      const prefix = pathPrefixGroup(t.path)
      if (!byPrefix.has(prefix)) {
        prefixOrder.push(prefix)
        byPrefix.set(prefix, [])
      }
      byPrefix.get(prefix)!.push(t)
    }
    prefixOrder.sort((a, b) => {
      if (a === '其他') return 1
      if (b === '其他') return -1
      return a.localeCompare(b)
    })
    return {
      connectorId,
      prefixes: prefixOrder.map((prefix) => ({ prefix, tools: byPrefix.get(prefix) ?? [] })),
    }
  })
}
