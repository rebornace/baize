import { type FormEvent, useCallback, useEffect, useMemo, useState } from 'react'
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
import { formatKeyValueMap, parseKeyValueLines } from './McpSettings'
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
    if (!id) {
      return { ok: false, message: `第 ${i + 1} 行：id 不能为空` }
    }
    if (!CHANNEL_ID_RE.test(id)) {
      return {
        ok: false,
        message: `第 ${i + 1} 行：id 须匹配 ^[a-z][a-z0-9_-]{0,63}$`,
      }
    }
    if (!agentId) {
      return { ok: false, message: `第 ${i + 1} 行：agent_id 不能为空` }
    }
    if (seen.has(id)) {
      return { ok: false, message: `重复的 channel id：${id}` }
    }
    seen.add(id)

    const headersParsed = parseKeyValueLines(row.headersText ?? '')
    if (!headersParsed.ok) {
      return { ok: false, message: `第 ${i + 1} 行：${headersParsed.message}` }
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

function apiErrorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
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

export function InboxSettings() {
  const [rows, setRows] = useState<ChannelFormRow[]>([])
  const [defaultAgentId, setDefaultAgentId] = useState('ticket-agent')
  const [skillCatalog, setSkillCatalog] = useState<SkillSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [rowBusy, setRowBusy] = useState<string | null>(null)
  const [secretModal, setSecretModal] = useState<{ id: string; secret: string } | null>(null)

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
      setError(null)
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const updateRow = (index: number, patch: Partial<ChannelFormRow>) => {
    setRows((prev) => prev.map((row, i) => (i === index ? { ...row, ...patch } : row)))
  }

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    const validated = validateChannelsForm(rows)
    if (!validated.ok) {
      setError(validated.message)
      return
    }
    setSubmitting(true)
    setError(null)
    setStatus(null)
    try {
      await putInboxChannels(validated.channels)
      const refreshed = await getInboxChannels()
      setRows(channelsToForm(refreshed))
      setStatus(`已保存 ${refreshed.length} 个 Inbox Channel`)
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  const onCopyUrl = async (id: string) => {
    const trimmed = id.trim()
    if (!trimmed) {
      setError('请先填写 channel id')
      return
    }
    const url = inboxUrlFor(window.location.origin, trimmed)
    try {
      await navigator.clipboard.writeText(url)
      setStatus(`已复制入站 URL：${url}`)
      setError(null)
    } catch (err) {
      setError(`复制失败：${apiErrorMessage(err)}`)
    }
  }

  const onRotate = async (id: string) => {
    const trimmed = id.trim()
    if (!trimmed) {
      setError('请先保存带 id 的 Channel')
      return
    }
    setRowBusy(`rotate:${trimmed}`)
    setError(null)
    setStatus(null)
    try {
      const { secret } = await rotateInboxSecret(trimmed)
      setSecretModal({ id: trimmed, secret })
      const refreshed = await getInboxChannels()
      setRows(channelsToForm(refreshed))
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setRowBusy(null)
    }
  }

  const onTest = async (id: string) => {
    const trimmed = id.trim()
    if (!trimmed) {
      setError('请先保存带 id 的 Channel')
      return
    }
    setRowBusy(`test:${trimmed}`)
    setError(null)
    setStatus(null)
    try {
      const result = await testInboxChannel(trimmed)
      setStatus(`测试投递成功：delivery=${result.delivery_id} run=${result.run_id}`)
    } catch (err) {
      setError(`测试投递失败：${apiErrorMessage(err)}`)
    } finally {
      setRowBusy(null)
    }
  }

  const busy = submitting || rowBusy !== null

  return (
    <div className="settings-section settings-inbox">
      <h1 className="settings-heading">Inbox</h1>
      <div className="settings-meta">
        <p>
          配置入站 Webhook Channel。外部系统以 HMAC 签名 POST 到{' '}
          <code>{'{origin}'}/v0/inbox/{'{channel_id}'}</code>
          ，Runtime 创建 Run（可选续聊）；与出站 Webhook 配对形成生产集成闭环。
        </p>
        <p>
          入站 URL 形如 <code>https://&lt;host&gt;/v0/inbox/{'{channel_id}'}</code>
          。签名示例（curl / OpenSSL / Python）见仓库 README「生产集成：Webhook Inbox」。
        </p>
        <pre className="settings-muted">{`外部系统 --HMAC--> POST /v0/inbox/{id} --> Run
                                         |
                                         +--> 出站 Webhook（可选覆盖）`}</pre>
      </div>
      {loading && <p className="settings-muted">加载中…</p>}
      {!loading && error && <p className="settings-error">{error}</p>}
      {!loading && status && <p className="settings-muted">{status}</p>}
      {!loading && (
        <form className="settings-form" onSubmit={(e) => void onSubmit(e)}>
          {rows.length === 0 && (
            <p className="settings-empty">尚未配置 Channel，点击下方「添加 Channel」。</p>
          )}
          {rows.map((row, index) => {
            const rowKey = row.id.trim() || `new-${index}`
            const rotating = rowBusy === `rotate:${row.id.trim()}`
            const testing = rowBusy === `test:${row.id.trim()}`
            const agentList = collectAgentOptions(defaultAgentId, [row, ...rows])
            return (
              <fieldset key={rowKey} className="settings-inbox-row" disabled={busy}>
                <legend className="settings-subheading">
                  Channel {row.id.trim() || `#${index + 1}`}
                  {row.secret_hint ? (
                    <span className="settings-muted"> · secret …{row.secret_hint}</span>
                  ) : null}
                </legend>
                <label className="settings-field">
                  <span className="settings-field-label">id</span>
                  <input
                    className="settings-input"
                    value={row.id}
                    onChange={(e) => updateRow(index, { id: e.target.value })}
                    placeholder="alerts"
                    required
                  />
                </label>
                <label className="settings-field">
                  <span className="settings-field-label">agent_id</span>
                  <select
                    className="settings-select"
                    value={
                      agentList.includes(row.agent_id.trim())
                        ? row.agent_id.trim()
                        : (agentList[0] ?? '')
                    }
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
                  </select>
                </label>
                <label className="settings-login-toggle">
                  <input
                    type="checkbox"
                    checked={row.enabled}
                    onChange={(e) => updateRow(index, { enabled: e.target.checked })}
                  />
                  <span>启用</span>
                </label>
                <label className="settings-field">
                  <span className="settings-field-label">description</span>
                  <input
                    className="settings-input"
                    value={row.description}
                    onChange={(e) => updateRow(index, { description: e.target.value })}
                    placeholder="运维告警入口"
                  />
                </label>
                <div className="settings-field">
                  <span className="settings-field-label">skills（多选）</span>
                  {skillCatalog.length === 0 ? (
                    <p className="settings-muted">无可用 Skill</p>
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
                <label className="settings-field">
                  <span className="settings-field-label">出站 webhook_url（可选覆盖）</span>
                  <input
                    className="settings-input"
                    value={row.webhook_url}
                    onChange={(e) => updateRow(index, { webhook_url: e.target.value })}
                    placeholder="https://example.com/hooks/baize"
                  />
                </label>
                <label className="settings-field">
                  <span className="settings-field-label">webhook headers（每行 KEY=VALUE）</span>
                  <textarea
                    className="settings-textarea"
                    value={row.headersText}
                    onChange={(e) => updateRow(index, { headersText: e.target.value })}
                    rows={3}
                    placeholder="Authorization=Bearer ${API_TOKEN}"
                  />
                </label>
                <div className="settings-toolbar">
                  <button
                    type="button"
                    className="btn ghost sm"
                    disabled={busy || !row.id.trim()}
                    onClick={() => void onCopyUrl(row.id)}
                  >
                    复制入站 URL
                  </button>
                  <button
                    type="button"
                    className="btn ghost sm"
                    disabled={busy || !row.id.trim()}
                    onClick={() => void onRotate(row.id)}
                  >
                    {rotating ? '轮换中…' : '轮换 Secret'}
                  </button>
                  <button
                    type="button"
                    className="btn ghost sm"
                    disabled={busy || !row.id.trim()}
                    onClick={() => void onTest(row.id)}
                  >
                    {testing ? '测试中…' : '发送测试'}
                  </button>
                  <button
                    type="button"
                    className="btn danger sm"
                    disabled={busy}
                    onClick={() => setRows((prev) => prev.filter((_, i) => i !== index))}
                  >
                    删除
                  </button>
                </div>
              </fieldset>
            )
          })}
          <div className="settings-toolbar">
            <button
              type="button"
              className="btn ghost"
              disabled={busy}
              onClick={() =>
                setRows((prev) => [
                  ...prev,
                  { ...emptyRow(), agent_id: defaultAgentId || agents[0] || '' },
                ])
              }
            >
              添加 Channel
            </button>
            <button type="submit" className="btn primary" disabled={busy}>
              {submitting ? '保存中…' : '保存'}
            </button>
          </div>
        </form>
      )}
      {secretModal && (
        <div className="settings-drawer-backdrop" onClick={() => setSecretModal(null)}>
          <aside
            className="settings-drawer"
            onClick={(e) => e.stopPropagation()}
            role="dialog"
            aria-label="新 Secret"
          >
            <div className="settings-drawer-head">
              <h2 className="settings-subheading">Channel {secretModal.id} 新 Secret</h2>
              <button type="button" className="btn ghost sm" onClick={() => setSecretModal(null)}>
                关闭
              </button>
            </div>
            <p className="settings-meta">仅展示一次，请立即复制到外部系统配置。</p>
            <pre className="settings-muted">{secretModal.secret}</pre>
            <div className="settings-toolbar">
              <button
                type="button"
                className="btn primary sm"
                onClick={() => {
                  void navigator.clipboard.writeText(secretModal.secret).then(
                    () => setStatus('已复制新 Secret'),
                    (err) => setError(`复制失败：${apiErrorMessage(err)}`),
                  )
                }}
              >
                复制 Secret
              </button>
            </div>
          </aside>
        </div>
      )}
    </div>
  )
}
