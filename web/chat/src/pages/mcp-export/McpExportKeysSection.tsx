import { Link } from 'react-router-dom'
import { Button, Field, Input, Select } from '../../components/ui'
import type { ToolInfo } from '../../api'
import { MCP_EXPORTS } from '../../strings'
import { isKeyActive } from './mcpExportHelpers'
import { McpExportKeyAllowlistPicker } from './McpExportKeyAllowlistPicker'
import { McpExportKeySlotsEditor } from './McpExportKeySlotsEditor'
import type { McpExportSettingsController } from './useMcpExportSettings'

type Props = Pick<
  McpExportSettingsController,
  | 'identities'
  | 'keys'
  | 'tools'
  | 'busy'
  | 'keyName'
  | 'setKeyName'
  | 'keyIdentityId'
  | 'setKeyIdentityId'
  | 'keyAllowedTools'
  | 'setKeyAllowedTools'
  | 'keyFormError'
  | 'editingKeyId'
  | 'keyEditName'
  | 'setKeyEditName'
  | 'keyEditAllowedTools'
  | 'setKeyEditAllowedTools'
  | 'keyEditSlots'
  | 'setKeyEditSlots'
  | 'keyEditFormError'
  | 'onCreateKey'
  | 'startEditKey'
  | 'cancelEditKey'
  | 'onSaveEditKey'
  | 'identityName'
  | 'setConfirm'
>

function allowlistCountLabel(key: { allowed_tools?: string[] }): string {
  const n = key.allowed_tools?.length ?? 0
  return MCP_EXPORTS.keyAllowlistSelectedCount(n)
}

export function McpExportKeysSection({
  identities,
  keys,
  tools,
  busy,
  keyName,
  setKeyName,
  keyIdentityId,
  setKeyIdentityId,
  keyAllowedTools,
  setKeyAllowedTools,
  keyFormError,
  editingKeyId,
  keyEditName,
  setKeyEditName,
  keyEditAllowedTools,
  setKeyEditAllowedTools,
  keyEditSlots,
  setKeyEditSlots,
  keyEditFormError,
  onCreateKey,
  startEditKey,
  cancelEditKey,
  onSaveEditKey,
  identityName,
  setConfirm,
}: Props) {
  const catalog = tools as ToolInfo[] | null

  return (
    <section className="settings-form">
      <h2 className="settings-subheading">{MCP_EXPORTS.keyTitle}</h2>
      <p className="settings-meta">{MCP_EXPORTS.keyIntro}</p>
      <p className="settings-toolbar">
        <Link to="/settings/external-login" data-testid="mcp-export-configure-hosted-login">
          {MCP_EXPORTS.configureHostedLogin}
        </Link>
      </p>
      {keys.length === 0 && <p className="settings-empty">{MCP_EXPORTS.keyEmpty}</p>}
      {keys.length > 0 && (
        <ul className="settings-list">
          {keys.map((key) => {
            const active = isKeyActive(key)
            const slotCount = key.identity_slots?.length ?? 1
            return (
              <li key={key.id} className="settings-list-item">
                {editingKeyId === key.id ? (
                  <form className="settings-form" onSubmit={(e) => void onSaveEditKey(e)}>
                    <Field label={MCP_EXPORTS.keyName} required>
                      <Input
                        value={keyEditName}
                        onChange={(e) => setKeyEditName(e.target.value)}
                        disabled={busy}
                      />
                    </Field>
                    <McpExportKeySlotsEditor
                      identities={identities}
                      slots={keyEditSlots}
                      onChange={setKeyEditSlots}
                      disabled={busy}
                    />
                    <McpExportKeyAllowlistPicker
                      tools={catalog}
                      selected={keyEditAllowedTools}
                      onSelectedChange={setKeyEditAllowedTools}
                      disabled={busy}
                    />
                    {keyEditFormError && (
                      <p className="ui-inline-error" role="alert">
                        {keyEditFormError}
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
                        onClick={cancelEditKey}
                      >
                        {MCP_EXPORTS.cancel}
                      </Button>
                    </div>
                  </form>
                ) : (
                  <>
                    <span className="settings-tool-line">
                      <span className="settings-tool-title">{key.name}</span>
                      <span className="settings-muted">
                        {' '}
                        · {key.prefix}… · {identityName(key.identity_id)}
                        {slotCount > 1 ? ` · ${MCP_EXPORTS.keySlotsCount(slotCount)}` : ''}
                      </span>
                      {!active ? (
                        <span className="settings-muted"> · {MCP_EXPORTS.revoked}</span>
                      ) : (
                        <span className="settings-muted"> · {allowlistCountLabel(key)}</span>
                      )}
                    </span>
                    <div className="settings-toolbar">
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        disabled={busy || !active}
                        onClick={() => startEditKey(key)}
                      >
                        {MCP_EXPORTS.edit}
                      </Button>
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
                  </>
                )}
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
        <McpExportKeyAllowlistPicker
          tools={catalog}
          selected={keyAllowedTools}
          onSelectedChange={setKeyAllowedTools}
          disabled={busy || identities.length === 0}
        />
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
