import { Link } from 'react-router-dom'
import { Button, ConfirmDialog, PageHeader, ToastRegion } from '../components/ui'
import { TOOLS } from '../strings'
import { ToolsAddModal } from './tools/ToolsAddModal'
import { ToolsTree } from './tools/ToolsTree'
import {
  defaultExpandedSets,
  expandKeysForTool,
  insertToolSorted,
  prefixExpandKey,
} from './tools/toolsSettingsHelpers'
import { useToolsSettings } from './tools/useToolsSettings'

export {
  defaultExpandedSets,
  expandKeysForTool,
  insertToolSorted,
  prefixExpandKey,
}

export function ToolsSettings() {
  const s = useToolsSettings()

  const headerActions =
    s.tools === null ? undefined : (
      <div className="settings-toolbar">
        <input
          className="settings-input"
          value={s.query}
          onChange={(e) => s.setQuery(e.target.value)}
          placeholder="搜索"
          aria-label="搜索工具"
        />
        {s.showAdd && !s.readOnly && (
          <Button type="button" variant="secondary" size="sm" onClick={s.openAddModal}>
            {TOOLS.addTool}
          </Button>
        )}
      </div>
    )

  return (
    <div className="settings-section settings-tools">
      <PageHeader title={TOOLS.title} description={TOOLS.description} actions={headerActions} />
      <ToastRegion toasts={s.toasts} onDismiss={s.dismiss} />
      {s.loadFailed && <p className="settings-error">{s.loadError}</p>}
      {s.tools === null && !s.loadFailed && <p className="settings-muted">加载中…</p>}
      {s.tools !== null && (
        <>
          {s.tools.length === 0 && (
            <p className="settings-empty">
              尚未注册 Connector。{' '}
              {!s.readOnly && (
                <Link to="/settings/openapi" className="settings-link">
                  去 OpenAPI 设置注册
                </Link>
              )}
            </p>
          )}
          {s.tools.length > 0 && s.visible.length === 0 && <p className="settings-empty">无匹配</p>}
          {s.visible.length > 0 && (
            <ToolsTree
              tree={s.tree}
              readOnly={s.readOnly}
              rowBusy={s.rowBusy}
              groupBusy={s.groupBusy}
              toggling={s.toggling}
              savingCopy={s.savingCopy}
              editingKey={s.editingKey}
              setEditingKey={s.setEditingKey}
              draftTitle={s.draftTitle}
              setDraftTitle={s.setDraftTitle}
              draftDescription={s.draftDescription}
              setDraftDescription={s.setDraftDescription}
              deleting={s.deleting}
              pendingDelete={s.pendingDelete}
              isConnectorOpen={s.isConnectorOpen}
              isPrefixOpen={s.isPrefixOpen}
              setExpandedConnectors={s.setExpandedConnectors}
              setExpandedPrefixes={s.setExpandedPrefixes}
              onEnabledChange={s.onEnabledChange}
              onRequireLoginChange={s.onRequireLoginChange}
              onRequireApprovalChange={s.onRequireApprovalChange}
              beginDelete={s.beginDelete}
              onGroupEnabled={s.onGroupEnabled}
              startEdit={s.startEdit}
              onSaveCopy={s.onSaveCopy}
            />
          )}
        </>
      )}
      {!s.readOnly && (
        <ToolsAddModal
          addModalOpen={s.addModalOpen}
          addFormError={s.addFormError}
          form={s.form}
          setForm={s.setForm}
          formConnectorId={s.formConnectorId}
          setFormConnectorId={s.setFormConnectorId}
          openConnectorIds={s.openConnectorIds}
          submitting={s.submitting}
          onAddSubmit={s.onAddSubmit}
          closeAddModal={s.closeAddModal}
        />
      )}
      {!s.readOnly && (
        <ConfirmDialog
          open={!!s.pendingDelete}
          danger
          title={TOOLS.confirmDeleteTitle}
          body={TOOLS.confirmDeleteBody}
          confirmText={TOOLS.confirmDeleteOk}
          busy={s.deleting}
          error={s.deleteError}
          onCancel={s.cancelDelete}
          onConfirm={() => void s.confirmDelete()}
        />
      )}
    </div>
  )
}
