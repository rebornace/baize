import { deleteConnector } from '../api'

/**
 * confirmDeleteConnectorMessage builds the second-step confirmation
 * prompt shown before deleting a Connector wholesale. The message
 * includes the connector id and warns that all of its tools will be
 * removed, matching the spec for the settings-page delete UI.
 */
export function confirmDeleteConnectorMessage(id: string): string {
  return `确定删除 Connector "${id}" 吗？将移除其全部工具。`
}

export interface ConnectorDeleteDeps {
  /** window.confirm by default; injected so tests can drive both branches. */
  confirm: (message: string) => boolean
  /** deleteConnector by default; injected so tests can assert call counts. */
  deleteConnector: (id: string) => Promise<void>
}

/**
 * confirmAndDeleteConnector runs the two-step delete flow: ask the user
 * to confirm via `deps.confirm`, and only when confirmed call
 * `deps.deleteConnector`. Returns true when the connector was deleted,
 * false when the user cancelled (in which case deleteConnector is
 * never invoked and no network request is made).
 */
export async function confirmAndDeleteConnector(
  id: string,
  deps: ConnectorDeleteDeps,
): Promise<boolean> {
  if (!deps.confirm(confirmDeleteConnectorMessage(id))) return false
  await deps.deleteConnector(id)
  return true
}

/** Default deps bound to the real window.confirm and api.deleteConnector. */
export const defaultConnectorDeleteDeps: ConnectorDeleteDeps = {
  confirm: (message) =>
    typeof window !== 'undefined' && typeof window.confirm === 'function'
      ? window.confirm(message)
      : false,
  deleteConnector,
}
