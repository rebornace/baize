import { type FormEvent, useCallback, useEffect, useMemo, useState } from 'react'
import { Inbox } from 'lucide-react'
import {
  getInboxChannels,
  getUIConfig,
  listSkills,
  putInboxChannels,
  rotateInboxSecret,
  testInboxChannel,
  type InboxChannel,
  type SkillSummary,
} from '../api'
import {
  Button,
  ConfirmDialog,
  EmptyState,
  Field,
  Input,
  Modal,
  PageHeader,
  Select,
  Textarea,
  ToastRegion,
  useToast,
} from '../components/ui'
import { INBOX, friendlyError, inboxSecretHint } from '../strings'
import { formatKeyValueMap, parseKeyValueLines } from './connectorForms/lines'
import { toggleSkillSelection } from './SkillsSettings'

const CHANNEL_ID_RE = /^[a-z][a-z0-9_-]{0,63}$/

export interface ChannelFormRow {
  id: string
  agent_id: string
  enabled: boolean
  skills: string[]
  description: string
  webhook_url: string
  headersText: string
  secret_hint?: string
}

function emptyRow(): ChannelFormRow {
  return {
    id: '',
    agent_id: '',
    enabled: true,
    skills: [],
    description: '',
    webhook_url: '',
    headersText: '',
  }
}

export function channelsToForm(channels: InboxChannel[]): ChannelFormRow[] {
  return channels.map((c) => ({
    id: c.id,
    agent_id: c.agent_id,
    enabled: c.enabled,
    skills: c.skills ?? [],
    description: c.description ?? '',
    webhook_url: c.webhook_url ?? '',
    headersText: formatKeyValueMap(c.webhook_headers),
    secret_hint: c.secret_hint,
  }))
}

export function validateChannelsForm(
  rows: Array<Pick<ChannelFormRow, 'id' | 'agent_id' | 'enabled'> & Partial<ChannelFormRow>>,
): { ok: true; channels: InboxChannel[] } | { ok: false; message: string } {
  const seen = new Set<string>()
  const channels: InboxChannel[] = []

  for (let i = 0; i < rows.length; i++) {
    const row = rows[i]
    const id = (row.id ?? '').trim()
    const agentId = (row.agent_id ?? '').trim()
    const rowPrefix = `第 ${i + 1} 条：`
    if (!id) {
      return { ok: false, message: `${rowPrefix}${INBOX.errIdRequired}` }
    }
    if (!CHANNEL_ID_RE.test(id)) {
      return { ok: false, message: `${rowPrefix}${INBOX.errIdFormat}` }
    }
    if (!agentId) {
      return { ok: false, message: `${rowPrefix}${INBOX.errAgentRequired}` }
    }
    if (seen.has(id)) {
      return { ok: false, message: `${rowPrefix}${INBOX.errIdDuplicate}` }
    }
    seen.add(id)

    const headersParsed = parseKeyValueLines(row.headersText ?? '')
    if (!headersParsed.ok) {
      return { ok: false, message: `${rowPrefix}${INBOX.errBadHeaderLine}` }
    }

    const description = (row.description ?? '').trim()
    const webhookUrl = (row.webhook_url ?? '').trim()
    const skills = (row.skills ?? []).map((s) => s.trim()).filter((s) => s !== '')

    const channel: InboxChannel = {
      id,
      agent_id: agentId,
      enabled: Boolean(row.enabled),
    }
    if (skills.length > 0) channel.skills = skills
    if (description) channel.description = description
    if (webhookUrl) channel.webhook_url = webhookUrl
    if (Object.keys(headersParsed.value).length > 0) {
      channel.webhook_headers = headersParsed.value
    }
    channels.push(channel)
  }

  return { ok: true, channels }
}

export function inboxUrlFor(origin: string, channelId: string): string {
  return `${origin.replace(/\/$/, '')}/v0/inbox/${channelId}`
}

function collectAgentOptions(defaultAgentId: string, rows: ChannelFormRow[]): string[] {
  const ordered: string[] = []
  const seen = new Set<string>()
  const push = (id: string) => {
    const t = id.trim()
    if (!t || seen.has(t)) return
    seen.add(t)
    ordered.push(t)
  }
  push(defaultAgentId)
  for (const row of rows) push(row.agent_id)
  return ordered
}

function channelTitle(row: ChannelFormRow, index: number): string {
  const id = row.id.trim()
  if (id) return id
  return `${INBOX.channelNew} #${index + 1}`
}

export function InboxSettings() {
  const [rows, setRows] = useState<ChannelFormRow[]>([])
  const [defaultAgentId, setDefaultAgentId] = useState('ticket-agent')
  const [skillCatalog, setSkillCatalog] = useState<SkillSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [formError, setFormError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [rowBusy, setRowBusy] = useState<string | null>(null)
  const [pendingRemoveIndex, setPendingRemoveIndex] = useState<number | null>(null)
  const [pendingRotateId, setPendingRotateId] = useState<string | null>(null)
  const [secretModal, setSecretModal] = useState<{ id: string; secret: string } | null>(null)
  const { toasts, push, dismiss } = useToast()

  const agents = useMemo(
    () => collectAgentOptions(defaultAgentId, rows),
    [defaultAgentId, rows],
  )

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [channels, cfg, skillsBody] = await Promise.all([
        getInboxChannels(),
        getUIConfig().catch(() => null),
        listSkills().catch(() => ({ skills: [] as SkillSummary[] })),
      ])
      if (cfg?.agent_id?.trim()) setDefaultAgentId(cfg.agent_id.trim())
      setSkillCatalog(skillsBody.skills ?? [])
      setRows(channelsToForm(channels))
      setFormError(null)
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: INBOX.loadFailed, detail: f.detail ?? f.title })
    } finally {
      setLoading(false)
    }
  }, [push])

  useEffect(() => {
    void load()
  }, [load])

  const updateRow = (index: number, patch: Partial<ChannelFormRow>) => {
    setRows((prev) => prev.map((row, i) => (i === index ? { ...row, ...patch } : row)))
  }

  const addRow = () => {
    setRows((prev) => [
      ...prev,
      { ...emptyRow(), agent_id: defaultAgentId || agents[0] || '' },
    ])
    setFormError(null)
  }

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    const validated = validateChannelsForm(rows)
    if (!validated.ok) {
      setFormError(validated.message)
      return
    }
    setSubmitting(true)
    setFormError(null)
    try {
      await putInboxChannels(validated.channels)
      const refreshed = await getInboxChannels()
      setRows(channelsToForm(refreshed))
      push({ tone: 'success', title: INBOX.toastSaved })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setSubmitting(false)
    }
  }

  const onCopyUrl = async (id: string) => {
    const trimmed = id.trim()
    if (!trimmed) {
      setFormError(INBOX.errIdRequired)
      return
    }
    const url = inboxUrlFor(window.location.origin, trimmed)
    try {
      await navigator.clipboard.writeText(url)
      push({ tone: 'success', title: INBOX.toastCopiedUrl, detail: url })
      setFormError(null)
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    }
  }

  const confirmRotate = async () => {
    const trimmed = pendingRotateId?.trim()
    if (!trimmed) {
      setPendingRotateId(null)
      return
    }
    setRowBusy(`rotate:${trimmed}`)
    try {
      const { secret } = await rotateInboxSecret(trimmed)
      setPendingRotateId(null)
      setSecretModal({ id: trimmed, secret })
      const refreshed = await getInboxChannels()
      setRows(channelsToForm(refreshed))
      push({ tone: 'success', title: INBOX.toastRotateOk })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
      setPendingRotateId(null)
    } finally {
      setRowBusy(null)
    }
  }

  const onTest = async (id: string) => {
    const trimmed = id.trim()
    if (!trimmed) {
      setFormError(INBOX.errIdRequired)
      return
    }
    setRowBusy(`test:${trimmed}`)
    try {
      const result = await testInboxChannel(trimmed)
      push({
        tone: 'success',
        title: INBOX.toastTestOk,
        detail: result.run_id || result.delivery_id || undefined,
      })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setRowBusy(null)
    }
  }

  const onCopySecret = async () => {
    if (!secretModal) return
    try {
      await navigator.clipboard.writeText(secretModal.secret)
      push({ tone: 'success', title: INBOX.toastCopiedSecret })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    }
  }

  const confirmRemove = () => {
    if (pendingRemoveIndex === null) return
    const index = pendingRemoveIndex
    setRows((prev) => prev.filter((_, i) => i !== index))
    setPendingRemoveIndex(null)
    setFormError(null)
  }

  const busy = submitting || rowBusy !== null

  return (
    <div className="settings-panel settings-inbox">
      <PageHeader title={INBOX.title} description={INBOX.description} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />

      {loading && <p className="settings-muted">加载中…</p>}

      {!loading && (
        <form className="settings-form" onSubmit={(e) => void onSubmit(e)}>
          {rows.length === 0 && (
            <EmptyState
              icon={<Inbox size={28} aria-hidden="true" />}
              title={INBOX.emptyTitle}
              description={INBOX.emptyDesc}
              action={
                <Button type="button" variant="primary" onClick={addRow}>
                  {INBOX.add}
                </Button>
              }
            />
          )}
          {rows.map((row, index) => {
            const rowKey = row.id.trim() || `new-${index}`
            const rotating = rowBusy === `rotate:${row.id.trim()}`
            const testing = rowBusy === `test:${row.id.trim()}`
            const agentList = collectAgentOptions(defaultAgentId, [row, ...rows])
            const selectedAgent = agentList.includes(row.agent_id.trim())
              ? row.agent_id.trim()
              : (agentList[0] ?? '')
            return (
              <fieldset key={rowKey} className="settings-inbox-row" disabled={busy}>
                <legend className="settings-subheading">
                  {channelTitle(row, index)}
                  {row.secret_hint ? (
                    <span className="settings-muted"> · {inboxSecretHint(row.secret_hint)}</span>
                  ) : null}
                </legend>
                <Field label={INBOX.idLabel} hint={INBOX.idHint}>
                  <Input
                    value={row.id}
                    onChange={(e) => updateRow(index, { id: e.target.value })}
                    placeholder="alerts"
                    required
                  />
                </Field>
                <Field label={INBOX.agentLabel}>
                  <Select
                    value={selectedAgent}
                    onChange={(e) => updateRow(index, { agent_id: e.target.value })}
                  >
                    {agentList.map((id) => (
                      <option key={id} value={id}>
                        {id}
                      </option>
                    ))}
                    {row.agent_id.trim() && !agentList.includes(row.agent_id.trim()) ? (
                      <option value={row.agent_id.trim()}>{row.agent_id.trim()}</option>
                    ) : null}
                  </Select>
                </Field>
                <label className="settings-login-toggle">
                  <input
                    type="checkbox"
                    checked={row.enabled}
                    onChange={(e) => updateRow(index, { enabled: e.target.checked })}
                  />
                  <span>{INBOX.enabledLabel}</span>
                </label>
                <Field label={INBOX.descriptionLabel}>
                  <Input
                    value={row.description}
                    onChange={(e) => updateRow(index, { description: e.target.value })}
                    placeholder="运维告警入口"
                  />
                </Field>
                <div className="settings-field">
                  <span className="settings-field-label">{INBOX.skillsLabel}</span>
                  {skillCatalog.length === 0 ? (
                    <p className="settings-muted">{INBOX.skillsEmpty}</p>
                  ) : (
                    <ul className="settings-list">
                      {skillCatalog.map((s) => (
                        <li key={s.id} className="settings-list-item">
                          <label className="settings-login-toggle">
                            <input
                              type="checkbox"
                              checked={row.skills.includes(s.id)}
                              onChange={(e) =>
                                updateRow(index, {
                                  skills: [
                                    ...toggleSkillSelection(
                                      new Set(row.skills),
                                      s.id,
                                      e.target.checked,
                                    ),
                                  ],
                                })
                              }
                            />
                            <span>{s.id}</span>
                          </label>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
                <details className="settings-advanced">
                  <summary>{INBOX.advanced}</summary>
                  <Field label={INBOX.overrideUrlLabel} hint={INBOX.overrideUrlHint}>
                    <Input
                      value={row.webhook_url}
                      onChange={(e) => updateRow(index, { webhook_url: e.target.value })}
                      placeholder="https://example.com/hooks/baize"
                    />
                  </Field>
                  <Field
                    label={INBOX.overrideHeadersLabel}
                    hint={INBOX.overrideHeadersHint}
                  >
                    <Textarea
                      value={row.headersText}
                      onChange={(e) => updateRow(index, { headersText: e.target.value })}
                      rows={3}
                      placeholder="Authorization=Bearer ${API_TOKEN}"
                    />
                  </Field>
                </details>
                <div className="settings-toolbar">
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={busy || !row.id.trim()}
                    onClick={() => void onCopyUrl(row.id)}
                  >
                    {INBOX.copyUrl}
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={busy || !row.id.trim()}
                    onClick={() => setPendingRotateId(row.id.trim())}
                  >
                    {rotating ? INBOX.rotating : INBOX.rotateSecret}
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={busy || !row.id.trim()}
                    onClick={() => void onTest(row.id)}
                  >
                    {testing ? INBOX.testing : INBOX.test}
                  </Button>
                  <Button
                    type="button"
                    variant="danger"
                    size="sm"
                    disabled={busy}
                    onClick={() => setPendingRemoveIndex(index)}
                  >
                    {INBOX.remove}
                  </Button>
                </div>
              </fieldset>
            )
          })}
          {formError && (
            <p className="ui-inline-error" role="alert">
              {formError}
            </p>
          )}
          <div className="settings-toolbar">
            {rows.length > 0 && (
              <Button type="button" variant="ghost" disabled={busy} onClick={addRow}>
                {INBOX.add}
              </Button>
            )}
            <Button type="submit" variant="primary" disabled={busy}>
              {submitting ? INBOX.saving : INBOX.save}
            </Button>
          </div>
        </form>
      )}

      <ConfirmDialog
        open={pendingRemoveIndex !== null}
        danger
        title={INBOX.confirmRemoveTitle}
        body={INBOX.confirmRemoveBody}
        confirmText={INBOX.confirmRemoveOk}
        onCancel={() => setPendingRemoveIndex(null)}
        onConfirm={confirmRemove}
      />

      <ConfirmDialog
        open={pendingRotateId !== null}
        danger
        title={INBOX.confirmRotateTitle}
        body={INBOX.confirmRotateBody}
        confirmText={INBOX.confirmRotateOk}
        busy={rowBusy?.startsWith('rotate:') ?? false}
        onCancel={() => setPendingRotateId(null)}
        onConfirm={() => void confirmRotate()}
      />

      <Modal
        open={secretModal !== null}
        title={INBOX.secretModalTitle}
        onClose={() => setSecretModal(null)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setSecretModal(null)}>
              {INBOX.secretClose}
            </Button>
            <Button variant="primary" onClick={() => void onCopySecret()}>
              {INBOX.secretCopy}
            </Button>
          </>
        }
      >
        {secretModal && (
          <>
            <p className="settings-meta">{INBOX.secretModalBody}</p>
            <pre className="settings-muted">{secretModal.secret}</pre>
          </>
        )}
      </Modal>
    </div>
  )
}
