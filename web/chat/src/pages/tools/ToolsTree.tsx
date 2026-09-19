import type { ToolInfo } from '../../api'
import { Badge, Button } from '../../components/ui'
import { canDeleteCatalogTool } from '../../toolCatalog'
import { TOOLS } from '../../strings'
import {
  enabledCount,
  flattenGroup,
  formatMethodPath,
  isToolEnabled,
  prefixExpandKey,
  toggleKey,
  toolRowKey,
} from './toolsSettingsHelpers'
import type { ToolsSettingsController } from './useToolsSettings'

type TreeProps = Pick<
  ToolsSettingsController,
  | 'tree'
  | 'readOnly'
  | 'rowBusy'
  | 'groupBusy'
  | 'toggling'
  | 'savingCopy'
  | 'editingKey'
  | 'setEditingKey'
  | 'draftTitle'
  | 'setDraftTitle'
  | 'draftDescription'
  | 'setDraftDescription'
  | 'deleting'
  | 'pendingDelete'
  | 'isConnectorOpen'
  | 'isPrefixOpen'
  | 'setExpandedConnectors'
  | 'setExpandedPrefixes'
  | 'onEnabledChange'
  | 'onRequireLoginChange'
  | 'onRequireApprovalChange'
  | 'beginDelete'
  | 'onGroupEnabled'
  | 'startEdit'
  | 'onSaveCopy'
>

export function ToolsTree(props: TreeProps) {
  const {
    tree,
    readOnly,
    rowBusy,
    groupBusy,
    toggling,
    savingCopy,
    editingKey,
    setEditingKey,
    draftTitle,
    setDraftTitle,
    draftDescription,
    setDraftDescription,
    deleting,
    pendingDelete,
    isConnectorOpen,
    isPrefixOpen,
    setExpandedConnectors,
    setExpandedPrefixes,
    onEnabledChange,
    onRequireLoginChange,
    onRequireApprovalChange,
    beginDelete,
    onGroupEnabled,
    startEdit,
    onSaveCopy,
  } = props

  const renderGroupButtons = (groupKey: string, rows: ToolInfo[]) => {
    if (readOnly) return null
    const lockGroups = groupBusy !== null || toggling !== null || savingCopy
    return (
      <span className="settings-group-actions" onClick={(e) => e.stopPropagation()}>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={lockGroups}
          onClick={() => {
            void onGroupEnabled(groupKey, rows, true)
          }}
        >
          {TOOLS.enableAll}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={lockGroups}
          onClick={() => {
            void onGroupEnabled(groupKey, rows, false)
          }}
        >
          {TOOLS.disableAll}
        </Button>
      </span>
    )
  }

  const renderTool = (t: ToolInfo) => {
    const key = toolRowKey(t)
    const canDelete = canDeleteCatalogTool(t.source ?? '')
    const isDeleting = deleting && pendingDelete != null && toolRowKey(pendingDelete) === key
    const isEditing = editingKey === key
    const methodPath = formatMethodPath(t)
    const schemaText = JSON.stringify(t.input_schema ?? {}, null, 2)
    const enabled = isToolEnabled(t)
    return (
      <li key={key} className="settings-tool-row">
        <div className="settings-list-item">
          <span className="settings-tool-line">
            <span className="settings-tool-title">{t.title || t.name}</span>
            {t.description ? <span className="settings-tool-desc">{t.description}</span> : null}
            {(methodPath !== '' || schemaText !== '{}') && (
              <details className="settings-tool-tech">
                <summary>{TOOLS.techDetails}</summary>
                {methodPath !== '' && <p className="settings-muted">{methodPath}</p>}
                {schemaText !== '{}' && <pre className="settings-tool-schema">{schemaText}</pre>}
              </details>
            )}
          </span>
          <span className="settings-tool-actions">
            {readOnly ? (
              <>
                <Badge tone={enabled ? 'success' : 'neutral'}>
                  {enabled ? TOOLS.statusEnabled : TOOLS.statusDisabled}
                </Badge>
                {t.require_login ? <Badge tone="info">{TOOLS.requireLogin}</Badge> : null}
                {t.require_approval ? (
                  <Badge tone="warning">{TOOLS.requireApprovalBadge}</Badge>
                ) : null}
              </>
            ) : (
              <>
                <label className="settings-login-toggle">
                  <input
                    type="checkbox"
                    checked={enabled}
                    disabled={rowBusy}
                    onChange={(e) => {
                      void onEnabledChange(t.name, e.target.checked)
                    }}
                  />
                  {TOOLS.enable}
                </label>
                <label className="settings-login-toggle">
                  <input
                    type="checkbox"
                    checked={Boolean(t.require_login)}
                    disabled={rowBusy}
                    onChange={(e) => {
                      void onRequireLoginChange(t.name, e.target.checked)
                    }}
                  />
                  {TOOLS.requireLogin}
                </label>
                <label className="settings-login-toggle">
                  <input
                    type="checkbox"
                    checked={Boolean(t.require_approval)}
                    disabled={rowBusy}
                    onChange={(e) => {
                      void onRequireApprovalChange(t.name, e.target.checked)
                    }}
                  />
                  {TOOLS.requireApprovalBadge}
                </label>
                {canDelete && (
                  <Button
                    type="button"
                    variant="danger"
                    size="sm"
                    disabled={isDeleting || rowBusy}
                    onClick={() => beginDelete(t)}
                  >
                    {TOOLS.confirmDeleteOk}
                  </Button>
                )}
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={savingCopy || (rowBusy && !isEditing)}
                  onClick={() => {
                    if (isEditing) {
                      setEditingKey(null)
                      return
                    }
                    startEdit(t)
                  }}
                >
                  {isEditing ? TOOLS.collapse : TOOLS.editCopy}
                </Button>
              </>
            )}
          </span>
        </div>
        {isEditing && !readOnly && (
          <div className="settings-tool-edit">
            <label className="settings-field">
              <span className="settings-field-label">{TOOLS.displayName}</span>
              <input
                className="settings-input"
                value={draftTitle}
                onChange={(e) => setDraftTitle(e.target.value)}
                disabled={savingCopy}
              />
            </label>
            <label className="settings-field">
              <span className="settings-field-label">{TOOLS.descriptionLabel}</span>
              <textarea
                className="settings-textarea"
                value={draftDescription}
                onChange={(e) => setDraftDescription(e.target.value)}
                disabled={savingCopy}
                rows={3}
              />
            </label>
            <div className="settings-tool-edit-actions">
              <Button
                type="button"
                variant="primary"
                size="sm"
                disabled={savingCopy}
                onClick={() => {
                  void onSaveCopy(t)
                }}
              >
                {savingCopy ? TOOLS.saving : TOOLS.save}
              </Button>
            </div>
          </div>
        )}
      </li>
    )
  }

  return (
    <div className="settings-tree">
      {tree.map((group) => {
        const connectorOpen = isConnectorOpen(group.connectorId)
        const groupRows = flattenGroup(group.prefixes)
        const connectorKey = `c:${group.connectorId}`
        return (
          <div key={group.connectorId} className="settings-group">
            <div className="settings-group-head">
              <button
                type="button"
                className="settings-group-toggle"
                onClick={() => setExpandedConnectors((prev) => toggleKey(prev, group.connectorId))}
              >
                {connectorOpen ? '▾' : '▸'} {group.connectorId || TOOLS.noConnector}
              </button>
              <span className="settings-group-meta">
                {TOOLS.groupMeta(groupRows.length, enabledCount(groupRows))}
              </span>
              {renderGroupButtons(connectorKey, groupRows)}
            </div>
            {connectorOpen && (
              <div className="settings-group-body">
                {group.prefixes.map((prefixGroup) => {
                  const pKey = prefixExpandKey(group.connectorId, prefixGroup.prefix)
                  const prefixOpen = isPrefixOpen(group.connectorId, prefixGroup.prefix)
                  const prefixBusyKey = `p:${pKey}`
                  return (
                    <div key={pKey} className="settings-group settings-group-nested">
                      <div className="settings-group-head">
                        <button
                          type="button"
                          className="settings-group-toggle"
                          onClick={() => setExpandedPrefixes((prev) => toggleKey(prev, pKey))}
                        >
                          {prefixOpen ? '▾' : '▸'} {prefixGroup.prefix}
                        </button>
                        <span className="settings-group-meta">
                          {TOOLS.groupMeta(prefixGroup.tools.length, enabledCount(prefixGroup.tools))}
                        </span>
                        {renderGroupButtons(prefixBusyKey, prefixGroup.tools)}
                      </div>
                      {prefixOpen && (
                        <ul className="settings-list settings-tree-tools">{prefixGroup.tools.map(renderTool)}</ul>
                      )}
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}
