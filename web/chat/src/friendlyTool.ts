import { getPack } from './locale/pack'

export interface ToolCatalogEntry {
  name: string
  title?: string
  description?: string
  /** Connector owning this tool; used for login_required → login-<id> skill. */
  connector_id?: string
}
export type ToolCatalog = ToolCatalogEntry[]

type ToolStatus =
  | 'running'
  | 'waiting_human'
  | 'succeeded'
  | 'failed'
  | 'approved'
  | 'rejected'

/** 工具技术名 -> 友好名：有 title 用 title，否则回退技术名。 */
export function friendlyToolName(name: string, catalog: ToolCatalog): string {
  const hit = catalog.find((t) => t.name === name)
  const title = hit?.title?.trim()
  return title || name
}

/** 按状态生成动作短语；不对任意工具名硬拼动词。文案随当前 locale pack。 */
export function toolPhrase(name: string, status: ToolStatus, catalog: ToolCatalog): string {
  const label = friendlyToolName(name, catalog)
  const chat = getPack().CHAT
  switch (status) {
    case 'running':
      return chat.toolRunning(label)
    case 'waiting_human':
      return chat.toolWaitingHuman(label)
    case 'succeeded':
      return chat.toolSucceeded(label)
    case 'failed':
      return chat.toolFailed(label)
    case 'approved':
      return chat.toolApproved(label)
    case 'rejected':
      return chat.toolRejected(label)
    default: {
      const _exhaustive: never = status
      return _exhaustive
    }
  }
}
