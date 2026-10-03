import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Badge, Button, Card, ConfirmDialog, DropdownMenu, EmptyState, PageHeader } from '../ui'
import { CONNECTORS, connectorErrorText, permissionSummary } from '../../strings'
import type { ConnectorKind } from '../../pages/connectorForms/types'

export interface ConnectorRowData {
  id: string
  baseUrl?: string
  /** MCP 连接的传输方式摘要（如「本地程序 · npx」）；优先于 baseUrl 展示。 */
  summary?: string
  toolCount: number
  loginNames: string[]
  approvalNames: string[]
  /** MCP HTTP：是否展示 OAuth 操作。 */
  supportsOAuth?: boolean
  /** MCP OAuth："" | "authorized" | "needs_reauth" */
  oauthStatus?: string
}

export interface ConnectorShellProps {
  kind: ConnectorKind
  rows: ConnectorRowData[]
  loading: boolean
  loadError: string | null
  onCreate: () => void
  onEdit: (id: string) => void
  onDelete: (id: string) => Promise<void> | void
  /** MCP HTTP：去授权 / 重新授权。 */
  onAuthorize?: (id: string) => Promise<void> | void
  /** MCP HTTP：断开授权。 */
  onDisconnectOAuth?: (id: string) => Promise<void> | void
}

function oauthBadge(status: string | undefined) {
  if (status === 'authorized') {
    return <Badge tone="success">{CONNECTORS.oauthAuthorized}</Badge>
  }
  if (status === 'needs_reauth') {
    return <Badge tone="warning">{CONNECTORS.oauthNeedsReauth}</Badge>
  }
  return null
}

export function ConnectorShell({
  kind, rows, loading, loadError, onCreate, onEdit, onDelete, onAuthorize, onDisconnectOAuth,
}: ConnectorShellProps) {
  const [pendingDelete, setPendingDelete] = useState<string | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const beginDelete = (id: string) => {
    setDeleteError(null)
    setPendingDelete(id)
  }

  const cancelDelete = () => {
    if (deleting) return
    setPendingDelete(null)
    setDeleteError(null)
  }

  const confirmDelete = async () => {
    if (!pendingDelete) return
    setDeleting(true)
    setDeleteError(null)
    try {
      await onDelete(pendingDelete)
      setPendingDelete(null)
      setDeleteError(null)
    } catch (e) {
      // 删除失败：保留弹窗，由 Shell 统一呈现中文错误（父页面 onDelete 可直接 reject）。
      setDeleteError(connectorErrorText(e).title)
    } finally {
      setDeleting(false)
    }
  }

  const labels = {
    openapi: { title: CONNECTORS.openapiTitle, desc: CONNECTORS.openapiDesc, add: CONNECTORS.addOpenapi, emptyTitle: CONNECTORS.openapiEmptyTitle, emptyDesc: CONNECTORS.openapiEmptyDesc },
    plugin: { title: CONNECTORS.pluginTitle, desc: CONNECTORS.pluginDesc, add: CONNECTORS.addPlugin, emptyTitle: CONNECTORS.pluginEmptyTitle, emptyDesc: CONNECTORS.pluginEmptyDesc },
    mcp: { title: CONNECTORS.mcpTitle, desc: CONNECTORS.mcpDesc, add: CONNECTORS.addMcp, emptyTitle: CONNECTORS.mcpEmptyTitle, emptyDesc: CONNECTORS.mcpEmptyDesc },
  }[kind]

  // 空状态卡片会自带唯一的新增 CTA；仅在它出现时隐藏页头按钮，避免两个同义入口。
  const showEmptyState = !loading && rows.length === 0 && !loadError

  return (
    <div className="settings-panel">
      <PageHeader
        title={labels.title}
        description={labels.desc}
        actions={showEmptyState
          ? undefined
          : <Button variant="primary" size="sm" onClick={onCreate}>{labels.add}</Button>}
      />

      {loadError && <p className="ui-inline-error" role="alert">{loadError}</p>}
      {loading && rows.length === 0 && <p className="settings-muted">加载中…</p>}

      {showEmptyState && (
        <EmptyState title={labels.emptyTitle} description={labels.emptyDesc}
          action={<Button variant="primary" onClick={onCreate}>{labels.add}</Button>} />
      )}

      {rows.length > 0 && (
        <div className="connector-list">
          {rows.map((row) => {
            const summary = permissionSummary(row.loginNames, row.approvalNames)
            const oauthItems = []
            if (row.supportsOAuth && onAuthorize) {
              const label = row.oauthStatus === 'authorized' || row.oauthStatus === 'needs_reauth'
                ? CONNECTORS.oauthReauthorize
                : CONNECTORS.oauthAuthorize
              oauthItems.push({
                id: 'oauth-authorize',
                label,
                onSelect: () => { void onAuthorize(row.id) },
              })
            }
            if (row.supportsOAuth && onDisconnectOAuth && (row.oauthStatus === 'authorized' || row.oauthStatus === 'needs_reauth')) {
              oauthItems.push({
                id: 'oauth-disconnect',
                label: CONNECTORS.oauthDisconnect,
                onSelect: () => { void onDisconnectOAuth(row.id) },
              })
            }
            return (
              <Card key={row.id} className="connector-card"
                title={row.id}
                description={row.summary ?? row.baseUrl ?? '—'}
                trailing={
                  <>
                    {oauthBadge(row.oauthStatus)}
                    <Badge tone="info">{CONNECTORS.toolCount(row.toolCount)}</Badge>
                  </>
                }>
                {summary && <p className="connector-perm">{summary}</p>}
                <div className="connector-card-actions">
                  <Link to="/settings/tools" className="btn ghost sm">{CONNECTORS.toolsLink}</Link>
                  <DropdownMenu
                    triggerLabel={`${row.id} 操作`}
                    items={[
                      ...oauthItems,
                      { id: 'edit', label: CONNECTORS.menuEdit, onSelect: () => onEdit(row.id) },
                      { id: 'delete', label: CONNECTORS.menuDelete, destructive: true, onSelect: () => beginDelete(row.id) },
                    ]}
                  />
                </div>
              </Card>
            )
          })}
        </div>
      )}

      <ConfirmDialog
        open={pendingDelete !== null}
        danger
        title={CONNECTORS.deleteTitle}
        body={CONNECTORS.deleteBody}
        confirmText={CONNECTORS.deleteOk}
        busy={deleting}
        error={deleteError}
        onCancel={cancelDelete}
        onConfirm={() => void confirmDelete()}
      />
    </div>
  )
}
