import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Badge, Button, Card, ConfirmDialog, DropdownMenu, EmptyState, PageHeader } from '../ui'
import { CONNECTORS, connectorErrorText, permissionSummary } from '../../strings'
import type { ConnectorKind } from '../../pages/connectorForms/types'

export interface ConnectorRowData {
  id: string
  baseUrl?: string
  toolCount: number
  loginNames: string[]
  approvalNames: string[]
}

export interface ConnectorShellProps {
  kind: ConnectorKind
  rows: ConnectorRowData[]
  loading: boolean
  loadError: string | null
  onCreate: () => void
  onEdit: (id: string) => void
  onDelete: (id: string) => Promise<void> | void
}

export function ConnectorShell({ kind, rows, loading, loadError, onCreate, onEdit, onDelete }: ConnectorShellProps) {
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

  const isOpenapi = kind === 'openapi'
  const title = isOpenapi ? CONNECTORS.openapiTitle : CONNECTORS.pluginTitle
  const description = isOpenapi ? CONNECTORS.openapiDesc : CONNECTORS.pluginDesc
  const addLabel = isOpenapi ? CONNECTORS.addOpenapi : CONNECTORS.addPlugin
  const emptyTitle = isOpenapi ? CONNECTORS.openapiEmptyTitle : CONNECTORS.pluginEmptyTitle
  const emptyDesc = isOpenapi ? CONNECTORS.openapiEmptyDesc : CONNECTORS.pluginEmptyDesc

  return (
    <div className="settings-panel">
      <PageHeader
        title={title}
        description={description}
        actions={<Button variant="primary" size="sm" onClick={onCreate}>{addLabel}</Button>}
      />

      {loadError && <p className="ui-inline-error" role="alert">{loadError}</p>}
      {loading && rows.length === 0 && <p className="settings-muted">加载中…</p>}

      {!loading && rows.length === 0 && !loadError && (
        <EmptyState title={emptyTitle} description={emptyDesc}
          action={<Button variant="primary" onClick={onCreate}>{addLabel}</Button>} />
      )}

      {rows.length > 0 && (
        <div className="connector-list">
          {rows.map((row) => {
            const summary = permissionSummary(row.loginNames, row.approvalNames)
            return (
              <Card key={row.id} className="connector-card"
                title={row.id}
                description={row.baseUrl || '—'}
                trailing={<Badge tone="info">{CONNECTORS.toolCount(row.toolCount)}</Badge>}>
                {summary && <p className="connector-perm">{summary}</p>}
                <div className="connector-card-actions">
                  <Link to="/settings/tools" className="btn ghost sm">{CONNECTORS.toolsLink}</Link>
                  <DropdownMenu
                    triggerLabel={`${row.id} 操作`}
                    items={[
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
