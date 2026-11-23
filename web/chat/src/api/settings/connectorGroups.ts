import { authInit, parseJSON } from '../http'

export type ConnectorGroup = {
  id: string
  connectors: string[]
}

export type ConnectorGroupsView = {
  groups: ConnectorGroup[]
  source?: 'kv' | 'yaml' | string
}

export async function getConnectorGroups(): Promise<ConnectorGroupsView> {
  const res = await fetch('/v0/settings/connector-groups', { headers: authInit() })
  return parseJSON<ConnectorGroupsView>(res)
}

export async function putConnectorGroups(groups: ConnectorGroup[]): Promise<ConnectorGroupsView> {
  const res = await fetch('/v0/settings/connector-groups', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ groups }),
  })
  return parseJSON<ConnectorGroupsView>(res)
}
