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

  const showListChrome = s.tools !== null && s.tools.length > 0

  return (
    <div className="settings-panel settings-tools">
      <PageHeader title={TOOLS.title} description={TOOLS.description} />
      <ToastRegion toasts={s.toasts} onDismiss={s.dismiss} />
      {s.loadFailed && <p className="settings-error">{s.loadError}</p>}
      {s.tools === null && !s.loadFailed && <p className="settings-muted">{TOOLS.loading}</p>}
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
          {showListChrome && (
            <div className="settings-tools-list-shell">
              <div className="settings-toolbar settings-tools-toolbar">
                <input
                  className="settings-input"
                  value={s.query}
                  onChange={(e) => s.setQuery(e.target.value)}
                  placeholder={TOOLS.searchPlaceholder}
                  aria-label={TOOLS.searchAria}
                />
                {s.showAdd && !s.readOnly && (
                  <Button type="button" variant="secondary" size="sm" onClick={s.openAddModal}>
                    {TOOLS.addTool}
                  </Button>
                )}
              </div>
              {s.visible.length === 0 && <p className="settings-empty settings-tools-empty">无匹配</p>}
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
            </div>
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
