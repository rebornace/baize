import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  createMCPExportIdentity,
  createMCPExportKey,
  deleteMCPExportIdentity,
  getMCPExportSettings,
  listMCPExportIdentities,
  listMCPExportKeys,
  patchMCPExportIdentity,
  revokeMCPExportKey,
  type MCPExportIdentity,
  type MCPExportKey,
  type MCPExportSettings as MCPExportSettingsInfo,
} from '../api'
import {
  Button,
  ConfirmDialog,
  Field,
  Input,
  Modal,
  PageHeader,
  Select,
  Textarea,
  ToastRegion,
  useToast,
} from '../components/ui'
import { formatKeyValueMap, parseKeyValueLines } from './connectorForms/lines'
import { MCP_EXPORTS, mcpExportErrorText } from '../strings'

export interface IdentityFormState {
  name: string
  scheme: string
  headersText: string
}

const EMPTY_IDENTITY_FORM: IdentityFormState = {
  name: '',
  scheme: '',
  headersText: '',
}

export function mcpExportEndpointUrl(origin: string, endpointPath: string): string {
  const base = origin.replace(/\/$/, '')
  const path = endpointPath.startsWith('/') ? endpointPath : `/${endpointPath}`
  return `${base}${path}`
}

export function identityToForm(identity: MCPExportIdentity): IdentityFormState {
  return {
    name: identity.name,
    scheme: identity.scheme ?? '',
    headersText: formatKeyValueMap(identity.headers),
  }
}

export function validateIdentityForm(
  form: IdentityFormState,
):
  | { ok: true; name: string; scheme: string; headers: Record<string, string> }
  | { ok: false; message: string } {
  const name = form.name.trim()
  if (!name) {
    return { ok: false, message: MCP_EXPORTS.errNameRequired }
  }
  const headersParsed = parseKeyValueLines(form.headersText)
  if (!headersParsed.ok) {
    // parseKeyValueLines 的消息含旧文案，这里自行定位首个非法行，保证文案统一来自 MCP_EXPORTS。
    const badRow = form.headersText
      .split('\n')
      .map((line) => line.trim())
      .find((row) => row !== '' && (row.indexOf('=') <= 0 || row.slice(0, row.indexOf('=')).trim() === ''))
    return { ok: false, message: MCP_EXPORTS.errBadHeaderLine(badRow ?? '') }
  }
  return {
    ok: true,
    name,
    scheme: form.scheme.trim(),
    headers: headersParsed.value,
  }
}

function isKeyActive(key: MCPExportKey): boolean {
  return key.revoked_at == null || key.revoked_at === ''
}

type ConfirmState =
  | { kind: 'identity'; id: string; name: string }
  | { kind: 'key'; id: string; name: string; prefix: string }
  | null

export function McpExportSettings() {
  const { toasts, push, dismiss } = useToast()
  const [settings, setSettings] = useState<MCPExportSettingsInfo | null>(null)
  const [identities, setIdentities] = useState<MCPExportIdentity[]>([])
  const [keys, setKeys] = useState<MCPExportKey[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const [createForm, setCreateForm] = useState<IdentityFormState>(EMPTY_IDENTITY_FORM)
  const [createFormError, setCreateFormError] = useState<string | null>(null)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editForm, setEditForm] = useState<IdentityFormState>(EMPTY_IDENTITY_FORM)
  const [editFormError, setEditFormError] = useState<string | null>(null)

  const [keyName, setKeyName] = useState('')
  const [keyIdentityId, setKeyIdentityId] = useState('')
  const [keyFormError, setKeyFormError] = useState<string | null>(null)
  const [tokenModal, setTokenModal] = useState<{ name: string; token: string } | null>(null)

  const [confirm, setConfirm] = useState<ConfirmState>(null)
  const [confirmError, setConfirmError] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [settingsBody, ids, keyList] = await Promise.all([
        getMCPExportSettings(),
        listMCPExportIdentities(),
        listMCPExportKeys(),
      ])
      setSettings(settingsBody)
      setIdentities(ids)
      setKeys(keyList)
      setError(null)
      setKeyIdentityId((prev) => {
        if (prev && ids.some((i) => i.id === prev)) return prev
        return ids[0]?.id ?? ''
      })
    } catch (err) {
      setError(mcpExportErrorText(err).title)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const endpointUrl =
    settings != null
      ? mcpExportEndpointUrl(
          typeof window !== 'undefined' ? window.location.origin : '',
          settings.endpoint_path,
        )
      : ''

  const onCopyEndpoint = async () => {
    if (!endpointUrl) return
    try {
      await navigator.clipboard.writeText(endpointUrl)
      push({ tone: 'success', title: MCP_EXPORTS.endpointCopied })
    } catch {
      push({ tone: 'error', title: MCP_EXPORTS.copyFailed })
    }
  }

  const onCreateIdentity = async (e: FormEvent) => {
    e.preventDefault()
    const validated = validateIdentityForm(createForm)
    if (!validated.ok) {
      setCreateFormError(validated.message)
      return
    }
    setBusy(true)
    setCreateFormError(null)
    try {
      await createMCPExportIdentity({
        name: validated.name,
        scheme: validated.scheme || undefined,
        headers: Object.keys(validated.headers).length > 0 ? validated.headers : undefined,
      })
      setCreateForm(EMPTY_IDENTITY_FORM)
      push({ tone: 'success', title: MCP_EXPORTS.savedIdentity })
      await load()
    } catch (err) {
      setCreateFormError(mcpExportErrorText(err).title)
    } finally {
      setBusy(false)
    }
  }

  const startEdit = (identity: MCPExportIdentity) => {
    setEditingId(identity.id)
    setEditForm(identityToForm(identity))
    setEditFormError(null)
  }

  const cancelEdit = () => {
    setEditingId(null)
    setEditForm(EMPTY_IDENTITY_FORM)
    setEditFormError(null)
  }

  const onSaveEdit = async (e: FormEvent) => {
    e.preventDefault()
    if (!editingId) return
    const validated = validateIdentityForm(editForm)
    if (!validated.ok) {
      setEditFormError(validated.message)
      return
    }
    setBusy(true)
    setEditFormError(null)
    try {
      await patchMCPExportIdentity(editingId, {
        name: validated.name,
        scheme: validated.scheme,
        headers: validated.headers,
      })
      setEditingId(null)
      setEditForm(EMPTY_IDENTITY_FORM)
      push({ tone: 'success', title: MCP_EXPORTS.savedIdentity })
      await load()
    } catch (err) {
      setEditFormError(mcpExportErrorText(err).title)
    } finally {
      setBusy(false)
    }
  }

  const onCreateKey = async (e: FormEvent) => {
    e.preventDefault()
    const name = keyName.trim()
    const identityId = keyIdentityId.trim()
    if (!name) {
      setKeyFormError(MCP_EXPORTS.errKeyNameRequired)
      return
    }
    if (!identityId) {
      setKeyFormError(MCP_EXPORTS.errKeyIdentityRequired)
      return
    }
    setBusy(true)
    setKeyFormError(null)
    try {
      const created = await createMCPExportKey({ name, identity_id: identityId })
      setKeyName('')
      setTokenModal({ name: created.name, token: created.token })
      await load()
    } catch (err) {
      setKeyFormError(mcpExportErrorText(err).title)
    } finally {
      setBusy(false)
    }
  }

  const runConfirm = async () => {
    if (!confirm) return
    setBusy(true)
    setConfirmError(null)
    try {
      if (confirm.kind === 'identity') {
        await deleteMCPExportIdentity(confirm.id)
        if (editingId === confirm.id) {
          setEditingId(null)
          setEditForm(EMPTY_IDENTITY_FORM)
        }
        push({ tone: 'success', title: MCP_EXPORTS.deletedIdentity(confirm.name) })
      } else {
        await revokeMCPExportKey(confirm.id)
        push({ tone: 'success', title: MCP_EXPORTS.revokedKey(confirm.name) })
      }
      setConfirm(null)
      await load()
    } catch (err) {
      // 弹窗保持打开，错误内联展示，允许重试。
      setConfirmError(mcpExportErrorText(err).title)
    } finally {
      setBusy(false)
    }
  }

  const copyToken = async () => {
    if (!tokenModal) return
    try {
      await navigator.clipboard.writeText(tokenModal.token)
      push({ tone: 'success', title: MCP_EXPORTS.tokenCopied })
    } catch {
      push({ tone: 'error', title: MCP_EXPORTS.copyFailed })
    }
  }

  const identityName = (id: string) => identities.find((i) => i.id === id)?.name ?? id

  return (
    <div className="settings-panel">
      <PageHeader
        title={MCP_EXPORTS.title}
        description={
          <>
            {MCP_EXPORTS.intro}{' '}
            <Link to="/settings/tools" className="settings-link">
              {MCP_EXPORTS.introToolsLink}
            </Link>
          </>
        }
      />

      {loading && <p className="settings-muted">加载中…</p>}
      {!loading && error && (
        <p className="ui-inline-error" role="alert">
          {error}
        </p>
      )}

      {!loading && settings && (
        <section className="settings-form">
          <h2 className="settings-subheading">{MCP_EXPORTS.endpointTitle}</h2>
          <p className="settings-meta">
            {settings.enabled ? MCP_EXPORTS.endpointEnabled : MCP_EXPORTS.endpointDisabled}
          </p>
          <Field label="endpoint">
            <Input value={endpointUrl} readOnly />
          </Field>
          <div className="settings-toolbar">
            <Button variant="secondary" size="sm" onClick={() => void onCopyEndpoint()}>
              {MCP_EXPORTS.copyEndpoint}
            </Button>
          </div>
          <h3 className="settings-subheading">{MCP_EXPORTS.exampleTitle}</h3>
          <pre className="settings-muted">{`{
  "mcpServers": {
    "baize-export": {
      "url": "${endpointUrl || 'https://<host>/v0/mcp/export'}",
      "headers": {
        "Authorization": "Bearer <导出 Key>"
      }
    }
  }
}`}</pre>
        </section>
      )}

      {!loading && (
        <>
          <section className="settings-form">
            <h2 className="settings-subheading">{MCP_EXPORTS.identityTitle}</h2>
            <p className="settings-meta">{MCP_EXPORTS.identityIntro}</p>
            {identities.length === 0 && (
              <p className="settings-empty">{MCP_EXPORTS.identityEmpty}</p>
            )}
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
                            onChange={(e) =>
                              setEditForm((f) => ({ ...f, headersText: e.target.value }))
                            }
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
                        {!active ? (
                          <span className="settings-muted"> · {MCP_EXPORTS.revoked}</span>
                        ) : null}
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
                  placeholder="cursor-dev"
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
                <Button
                  type="submit"
                  variant="primary"
                  size="sm"
                  disabled={busy || identities.length === 0}
                >
                  {MCP_EXPORTS.createKey}
                </Button>
              </div>
            </form>
          </section>
        </>
      )}

      <ConfirmDialog
        open={confirm !== null}
        danger
        title={
          confirm?.kind === 'identity'
            ? MCP_EXPORTS.deleteIdentityTitle
            : MCP_EXPORTS.revokeKeyTitle
        }
        body={
          confirm
            ? confirm.kind === 'identity'
              ? MCP_EXPORTS.deleteIdentityBody(confirm.name)
              : MCP_EXPORTS.revokeKeyBody(confirm.name, confirm.prefix)
            : ''
        }
        confirmText={
          confirm?.kind === 'identity' ? MCP_EXPORTS.delete : MCP_EXPORTS.confirmRevoke
        }
        busy={busy}
        error={confirmError}
        onCancel={() => {
          if (!busy) {
            setConfirm(null)
            setConfirmError(null)
          }
        }}
        onConfirm={() => void runConfirm()}
      />

      <Modal
        open={tokenModal !== null}
        title={MCP_EXPORTS.tokenTitle}
        onClose={() => setTokenModal(null)}
        footer={
          <>
            <Button variant="secondary" onClick={() => void copyToken()}>
              {MCP_EXPORTS.copyToken}
            </Button>
            <Button variant="primary" onClick={() => setTokenModal(null)}>
              {MCP_EXPORTS.tokenSaved}
            </Button>
          </>
        }
      >
        {tokenModal && (
          <>
            <p className="settings-meta">{MCP_EXPORTS.tokenBody(tokenModal.name)}</p>
            <pre className="settings-muted">{tokenModal.token}</pre>
          </>
        )}
      </Modal>

      <ToastRegion toasts={toasts} onDismiss={dismiss} />
    </div>
  )
}

