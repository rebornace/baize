import { Button, Field, Input, Textarea } from '../../components/ui'
import { MCP_EXPORTS } from '../../strings'
import { formatKeyValueMap } from '../connectorForms/lines'
import type { McpExportSettingsController } from './useMcpExportSettings'

type Props = Pick<
  McpExportSettingsController,
  | 'identities'
  | 'busy'
  | 'createForm'
  | 'setCreateForm'
  | 'createFormError'
  | 'editingId'
  | 'editForm'
  | 'setEditForm'
  | 'editFormError'
  | 'onCreateIdentity'
  | 'startEdit'
  | 'cancelEdit'
  | 'onSaveEdit'
  | 'setConfirm'
>

export function McpExportIdentitiesSection({
  identities,
  busy,
  createForm,
  setCreateForm,
  createFormError,
  editingId,
  editForm,
  setEditForm,
  editFormError,
  onCreateIdentity,
  startEdit,
  cancelEdit,
  onSaveEdit,
  setConfirm,
}: Props) {
  return (
    <section className="settings-form">
      <h2 className="settings-subheading">{MCP_EXPORTS.identityTitle}</h2>
      <p className="settings-meta">{MCP_EXPORTS.identityIntro}</p>
      {identities.length === 0 && <p className="settings-empty">{MCP_EXPORTS.identityEmpty}</p>}
      {identities.length > 0 && (
        <ul className="settings-list">
          {identities.map((identity) => (
            <li key={identity.id} className="settings-list-item">
              {editingId === identity.id ? (
                <form className="settings-form" onSubmit={(e) => void onSaveEdit(e)}>
                  <Field label={MCP_EXPORTS.identityName} required>
                    <Input
                      value={editForm.name}
                      onChange={(e) => setEditForm((f) => ({ ...f, name: e.target.value }))}
                      disabled={busy}
                    />
                  </Field>
                  <Field label={MCP_EXPORTS.identityScheme}>
                    <Input
                      value={editForm.scheme}
                      onChange={(e) => setEditForm((f) => ({ ...f, scheme: e.target.value }))}
                      disabled={busy}
                      placeholder="Bearer"
                    />
                  </Field>
                  <Field label={MCP_EXPORTS.identityHeaders}>
                    <Textarea
                      rows={3}
                      value={editForm.headersText}
                      onChange={(e) => setEditForm((f) => ({ ...f, headersText: e.target.value }))}
                      disabled={busy}
                      placeholder={'Authorization=Bearer ${TOKEN}'}
                    />
                  </Field>
                  {editFormError && (
                    <p className="ui-inline-error" role="alert">
                      {editFormError}
                    </p>
                  )}
                  <div className="settings-toolbar">
                    <Button type="submit" variant="primary" size="sm" disabled={busy}>
                      {MCP_EXPORTS.save}
                    </Button>
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      disabled={busy}
                      onClick={cancelEdit}
                    >
                      {MCP_EXPORTS.cancel}
                    </Button>
                  </div>
                </form>
              ) : (
                <>
                  <span className="settings-tool-line">
                    <span className="settings-tool-title">{identity.name}</span>
                    <span className="settings-muted"> · {identity.id}</span>
                    {identity.scheme ? (
                      <span className="settings-muted"> · scheme={identity.scheme}</span>
                    ) : null}
                  </span>
                  {identity.headers && Object.keys(identity.headers).length > 0 ? (
                    <pre className="settings-muted">{formatKeyValueMap(identity.headers)}</pre>
                  ) : null}
                  <div className="settings-toolbar">
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      disabled={busy}
                      onClick={() => startEdit(identity)}
                    >
                      {MCP_EXPORTS.edit}
                    </Button>
                    <Button
                      type="button"
                      variant="danger"
                      size="sm"
                      disabled={busy}
                      onClick={() =>
                        setConfirm({
                          kind: 'identity',
                          id: identity.id,
                          name: identity.name,
                        })
                      }
                    >
                      {MCP_EXPORTS.delete}
                    </Button>
                  </div>
                </>
              )}
            </li>
          ))}
        </ul>
      )}

      <form className="settings-form" onSubmit={(e) => void onCreateIdentity(e)}>
        <h3 className="settings-subheading">{MCP_EXPORTS.createIdentity}</h3>
        <Field label={MCP_EXPORTS.identityName} required>
          <Input
            value={createForm.name}
            onChange={(e) => setCreateForm((f) => ({ ...f, name: e.target.value }))}
            disabled={busy}
            placeholder="Ops"
          />
        </Field>
        <Field label={MCP_EXPORTS.identityScheme}>
          <Input
            value={createForm.scheme}
            onChange={(e) => setCreateForm((f) => ({ ...f, scheme: e.target.value }))}
            disabled={busy}
            placeholder="Bearer"
          />
        </Field>
        <Field label={MCP_EXPORTS.identityHeaders}>
          <Textarea
            rows={3}
            value={createForm.headersText}
            onChange={(e) => setCreateForm((f) => ({ ...f, headersText: e.target.value }))}
            disabled={busy}
            placeholder="X-Team=ops"
          />
        </Field>
        {createFormError && (
          <p className="ui-inline-error" role="alert">
            {createFormError}
          </p>
        )}
        <div className="settings-toolbar">
          <Button type="submit" variant="primary" size="sm" disabled={busy}>
            {MCP_EXPORTS.createIdentity}
          </Button>
        </div>
      </form>
    </section>
  )
}
