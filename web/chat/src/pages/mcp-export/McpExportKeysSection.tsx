import { Button, Field, Input, Select } from '../../components/ui'
import { MCP_EXPORTS } from '../../strings'
import { isKeyActive } from './mcpExportHelpers'
import type { McpExportSettingsController } from './useMcpExportSettings'

type Props = Pick<
  McpExportSettingsController,
  | 'identities'
  | 'keys'
  | 'busy'
  | 'keyName'
  | 'setKeyName'
  | 'keyIdentityId'
  | 'setKeyIdentityId'
  | 'keyFormError'
  | 'onCreateKey'
  | 'identityName'
  | 'setConfirm'
>

export function McpExportKeysSection({
  identities,
  keys,
  busy,
  keyName,
  setKeyName,
  keyIdentityId,
  setKeyIdentityId,
  keyFormError,
  onCreateKey,
  identityName,
  setConfirm,
}: Props) {
  return (
    <section className="settings-form">
      <h2 className="settings-subheading">{MCP_EXPORTS.keyTitle}</h2>
      <p className="settings-meta">{MCP_EXPORTS.keyIntro}</p>
      {keys.length === 0 && <p className="settings-empty">{MCP_EXPORTS.keyEmpty}</p>}
      {keys.length > 0 && (
        <ul className="settings-list">
          {keys.map((key) => {
            const active = isKeyActive(key)
            return (
              <li key={key.id} className="settings-list-item">
                <span className="settings-tool-line">
                  <span className="settings-tool-title">{key.name}</span>
                  <span className="settings-muted">
                    {' '}
                    · {key.prefix}… · {identityName(key.identity_id)}
                  </span>
                  {!active ? <span className="settings-muted"> · {MCP_EXPORTS.revoked}</span> : null}
                </span>
                <div className="settings-toolbar">
                  <Button
                    type="button"
                    variant="danger"
                    size="sm"
                    disabled={busy || !active}
                    onClick={() =>
                      setConfirm({
                        kind: 'key',
                        id: key.id,
                        name: key.name,
                        prefix: key.prefix,
                      })
                    }
                  >
                    {active ? MCP_EXPORTS.revoke : MCP_EXPORTS.revoked}
                  </Button>
                </div>
              </li>
            )
          })}
        </ul>
      )}

      <form className="settings-form" onSubmit={(e) => void onCreateKey(e)}>
        <h3 className="settings-subheading">{MCP_EXPORTS.createKey}</h3>
        <Field label={MCP_EXPORTS.keyName} required>
          <Input
            value={keyName}
            onChange={(e) => setKeyName(e.target.value)}
            disabled={busy || identities.length === 0}
            placeholder={MCP_EXPORTS.phKeyName}
          />
        </Field>
        <Field label={MCP_EXPORTS.keyBindIdentity} required>
          <Select
            value={keyIdentityId}
            onChange={(e) => setKeyIdentityId(e.target.value)}
            disabled={busy || identities.length === 0}
          >
            {identities.length === 0 ? (
              <option value="">{MCP_EXPORTS.keyNeedIdentityFirst}</option>
            ) : (
              identities.map((i) => (
                <option key={i.id} value={i.id}>
                  {i.name} ({i.id})
                </option>
              ))
            )}
          </Select>
        </Field>
        {keyFormError && (
          <p className="ui-inline-error" role="alert">
            {keyFormError}
          </p>
        )}
        <div className="settings-toolbar">
          <Button type="submit" variant="primary" size="sm" disabled={busy || identities.length === 0}>
            {MCP_EXPORTS.createKey}
          </Button>
        </div>
      </form>
    </section>
  )
}
