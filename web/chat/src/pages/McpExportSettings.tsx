import {
  Button,
  ConfirmDialog,
  Field,
  Input,
  Modal,
  PageHeader,
  ToastRegion,
} from '../components/ui'
import { MCP_EXPORTS } from '../strings'
import { McpExportIdentitiesSection } from './mcp-export/McpExportIdentitiesSection'
import { McpExportKeysSection } from './mcp-export/McpExportKeysSection'
import { McpExportToolsSection } from './mcp-export/McpExportToolsSection'
import {
  identityToForm,
  mcpExportEndpointUrl,
  toolExportMode,
  validateIdentityForm,
  type IdentityFormState,
} from './mcp-export/mcpExportHelpers'
import { useMcpExportSettings } from './mcp-export/useMcpExportSettings'

export {
  identityToForm,
  mcpExportEndpointUrl,
  toolExportMode,
  validateIdentityForm,
}
export type { IdentityFormState }

export function McpExportSettings() {
  const s = useMcpExportSettings()

  return (
    <div className="settings-panel">
      <PageHeader
        title={MCP_EXPORTS.title}
        description={
          <>
            {MCP_EXPORTS.intro} {MCP_EXPORTS.introToolsLink}
          </>
        }
      />

      {s.loading && <p className="settings-muted">加载中…</p>}
      {!s.loading && s.error && (
        <p className="ui-inline-error" role="alert">
          {s.error}
        </p>
      )}

      {!s.loading && s.settings && (
        <section className="settings-form">
          <h2 className="settings-subheading">{MCP_EXPORTS.endpointTitle}</h2>
          <p className="settings-meta">
            {s.settings.enabled ? MCP_EXPORTS.endpointEnabled : MCP_EXPORTS.endpointDisabled}
          </p>
          <Field label="endpoint">
            <Input value={s.endpointUrl} readOnly />
          </Field>
          <div className="settings-toolbar">
            <Button variant="secondary" size="sm" onClick={() => void s.onCopyEndpoint()}>
              {MCP_EXPORTS.copyEndpoint}
            </Button>
          </div>
          <h3 className="settings-subheading">{MCP_EXPORTS.exampleTitle}</h3>
          <pre className="settings-muted">{`{
  "mcpServers": {
    "baize-export": {
      "url": "${s.endpointUrl || 'https://<host>/v0/mcp/export'}",
      "headers": {
        "Authorization": "Bearer <导出 Key>"
      }
    }
  }
}`}</pre>
        </section>
      )}

      {!s.loading && (
        <McpExportToolsSection
          tools={s.tools}
          toolsError={s.toolsError}
          toolQuery={s.toolQuery}
          setToolQuery={s.setToolQuery}
          filteredTools={s.filteredTools}
          exportBusy={s.exportBusy}
          onExportChange={s.onExportChange}
        />
      )}

      {!s.loading && (
        <>
          <McpExportIdentitiesSection
            identities={s.identities}
            busy={s.busy}
            createForm={s.createForm}
            setCreateForm={s.setCreateForm}
            createFormError={s.createFormError}
            editingId={s.editingId}
            editForm={s.editForm}
            setEditForm={s.setEditForm}
            editFormError={s.editFormError}
            onCreateIdentity={s.onCreateIdentity}
            startEdit={s.startEdit}
            cancelEdit={s.cancelEdit}
            onSaveEdit={s.onSaveEdit}
            setConfirm={s.setConfirm}
          />
          <McpExportKeysSection
            identities={s.identities}
            keys={s.keys}
            busy={s.busy}
            keyName={s.keyName}
            setKeyName={s.setKeyName}
            keyIdentityId={s.keyIdentityId}
            setKeyIdentityId={s.setKeyIdentityId}
            keyFormError={s.keyFormError}
            onCreateKey={s.onCreateKey}
            identityName={s.identityName}
            setConfirm={s.setConfirm}
          />
        </>
      )}

      <ConfirmDialog
        open={s.confirm !== null}
        danger
        title={
          s.confirm?.kind === 'identity'
            ? MCP_EXPORTS.deleteIdentityTitle
            : MCP_EXPORTS.revokeKeyTitle
        }
        body={
          s.confirm
            ? s.confirm.kind === 'identity'
              ? MCP_EXPORTS.deleteIdentityBody(s.confirm.name)
              : MCP_EXPORTS.revokeKeyBody(s.confirm.name, s.confirm.prefix)
            : ''
        }
        confirmText={
          s.confirm?.kind === 'identity' ? MCP_EXPORTS.delete : MCP_EXPORTS.confirmRevoke
        }
        busy={s.busy}
        error={s.confirmError}
        onCancel={s.cancelConfirm}
        onConfirm={() => void s.runConfirm()}
      />

      <Modal
        open={s.tokenModal !== null}
        title={MCP_EXPORTS.tokenTitle}
        onClose={() => s.setTokenModal(null)}
        footer={
          <>
            <Button variant="secondary" onClick={() => void s.copyToken()}>
              {MCP_EXPORTS.copyToken}
            </Button>
            <Button variant="primary" onClick={() => s.setTokenModal(null)}>
              {MCP_EXPORTS.tokenSaved}
            </Button>
          </>
        }
      >
        {s.tokenModal && (
          <>
            <p className="settings-meta">{MCP_EXPORTS.tokenBody(s.tokenModal.name)}</p>
            <pre className="settings-muted">{s.tokenModal.token}</pre>
          </>
        )}
      </Modal>

      <ToastRegion toasts={s.toasts} onDismiss={s.dismiss} />
    </div>
  )
}
