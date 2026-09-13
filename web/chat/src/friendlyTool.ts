export interface ToolCatalogEntry {
  name: string
  title?: string
  description?: string
}
export type ToolCatalog = ToolCatalogEntry[]

type ToolStatus =
  | 'running'
  | 'waiting_human'
  | 'succeeded'
  | 'failed'
  | 'approved'
  | 'rejected'

const PHRASE: Record<ToolStatus, (label: string) => string> = {
  running: (l) => `正在处理：${l}…`,
  waiting_human: (l) => `待确认：${l}`,
  succeeded: (l) => `已完成：${l}`,
  failed: (l) => `处理失败：${l}`,
  approved: (l) => `已同意：${l}`,
  rejected: (l) => `已拒绝：${l}`,
}

/** 工具技术名 -> 友好名：有 title 用 title，否则回退技术名。 */
export function friendlyToolName(name: string, catalog: ToolCatalog): string {
  const hit = catalog.find((t) => t.name === name)
  const title = hit?.title?.trim()
  return title || name
}

/** 按状态生成动作短语；不对任意工具名硬拼动词。 */
export function toolPhrase(name: string, status: ToolStatus, catalog: ToolCatalog): string {
  return PHRASE[status](friendlyToolName(name, catalog))
}
