import { type FormEvent, useCallback, useEffect, useState } from 'react'
import {
  getAttachmentRetention,
  getMessageRetention,
  getRunRetention,
  getStoreSettings,
  getWorkspaceAttachmentRetention,
  putAttachmentRetention,
  putMessageRetention,
  putRunRetention,
  putStoreSettings,
  putWorkspaceAttachmentRetention,
  type StoreSettings,
  type WorkspaceAttachmentRetentionItem,
} from '../api'
import {
  Button,
  Card,
  ConfirmDialog,
  Field,
  Input,
  PageHeader,
  Select,
  ToastRegion,
  useToast,
} from '../components/ui'
import { driverLabel, friendlyError, STORAGE } from '../strings'

const FALLBACK_DRIVERS = ['memory', 'sqlite', 'postgres']

type ConfirmMode = 'hot' | 'restart'

export function StorageSettings() {
  const [info, setInfo] = useState<StoreSettings | null>(null)
  const [driver, setDriver] = useState('sqlite')
  const [sqlitePath, setSQLitePath] = useState('./data/baize.db')
  const [dsn, setDSN] = useState('')
  const [ack, setAck] = useState(false)
  const [ackError, setAckError] = useState<string | null>(null)
  const [dsnError, setDsnError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [confirmMode, setConfirmMode] = useState<ConfirmMode>('hot')
  const [retentionDays, setRetentionDays] = useState('0')
  const [retentionBusy, setRetentionBusy] = useState(false)
  const [runRetentionDays, setRunRetentionDays] = useState('0')
  const [runRetentionBusy, setRunRetentionBusy] = useState(false)
  const [messageRetentionDays, setMessageRetentionDays] = useState('0')
  const [messageRetentionBusy, setMessageRetentionBusy] = useState(false)
  const [workspaceRetention, setWorkspaceRetention] = useState<
    WorkspaceAttachmentRetentionItem[] | null
  >(null)
  const [workspaceDrafts, setWorkspaceDrafts] = useState<Record<string, string>>({})
  const [workspaceBusy, setWorkspaceBusy] = useState<string | null>(null)
  const { toasts, push, dismiss } = useToast()

  const load = useCallback(async () => {
    try {
      const s = await getStoreSettings()
      setInfo(s)
      setDriver(s.driver || 'sqlite')
      setSQLitePath(s.sqlite_path || './data/baize.db')
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    }
  }, [push])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    const loadRetention = async () => {
      try {
        const r = await getAttachmentRetention()
        setRetentionDays(String(r.days))
      } catch {
        // Optional section; keep the default when the endpoint is unavailable.
      }
      try {
        const r = await getRunRetention()
        setRunRetentionDays(String(r.days))
      } catch {
        // Optional section; keep the default when the endpoint is unavailable.
      }
      try {
        const r = await getMessageRetention()
        setMessageRetentionDays(String(r.days))
      } catch {
        // Optional section; keep the default when the endpoint is unavailable.
      }
      try {
        const r = await getWorkspaceAttachmentRetention()
        setWorkspaceRetention(Array.isArray(r.items) ? r.items : [])
        const drafts: Record<string, string> = {}
        for (const item of r.items ?? []) {
          drafts[item.workspace_id] = String(item.days)
        }
        setWorkspaceDrafts(drafts)
      } catch {
        setWorkspaceRetention(null)
      }
    }
    void loadRetention()
  }, [])

  const saveRetention = async () => {
    const n = Number.parseInt(retentionDays, 10)
    if (!Number.isFinite(n) || n < 0 || n > 3650) {
      push({ tone: 'error', title: STORAGE.retentionInvalid })
      return
    }
    setRetentionBusy(true)
    try {
      const r = await putAttachmentRetention(n)
      setRetentionDays(String(r.days))
      push({ tone: 'success', title: STORAGE.retentionSaved })
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setRetentionBusy(false)
    }
  }

  const saveWorkspaceRetention = async (item: WorkspaceAttachmentRetentionItem) => {
    const raw = workspaceDrafts[item.workspace_id] ?? String(item.days)
    const days = Number.parseInt(raw, 10)
    if (!Number.isFinite(days) || days < -1 || days > 3650) {
      push({ tone: 'error', title: STORAGE.workspaceRetentionInvalid })
      return
    }
    setWorkspaceBusy(item.workspace_id)
    try {
      const next = await putWorkspaceAttachmentRetention(item.workspace_id, days)
      setWorkspaceRetention((prev) =>
        prev
          ? prev.map((row) =>
              row.workspace_id === next.workspace_id
                ? { ...row, days: next.days, effective_days: next.effective_days }
                : row,
            )
          : prev,
      )
      setWorkspaceDrafts((prev) => ({ ...prev, [next.workspace_id]: String(next.days) }))
      push({ tone: 'success', title: STORAGE.retentionSaved })
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setWorkspaceBusy(null)
    }
  }

  const saveRunRetention = async () => {
    const n = Number.parseInt(runRetentionDays, 10)
    if (!Number.isFinite(n) || n < 0 || n > 3650) {
      push({ tone: 'error', title: STORAGE.runRetentionInvalid })
      return
    }
    setRunRetentionBusy(true)
    try {
      const r = await putRunRetention(n)
      setRunRetentionDays(String(r.days))
      push({ tone: 'success', title: STORAGE.runRetentionSaved })
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setRunRetentionBusy(false)
    }
  }

  const saveMessageRetention = async () => {
    const n = Number.parseInt(messageRetentionDays, 10)
    if (!Number.isFinite(n) || n < 0 || n > 3650) {
      push({ tone: 'error', title: STORAGE.messageRetentionInvalid })
      return
    }
    setMessageRetentionBusy(true)
    try {
      const r = await putMessageRetention(n)
      setMessageRetentionDays(String(r.days))
      push({ tone: 'success', title: STORAGE.messageRetentionSaved })
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setMessageRetentionBusy(false)
    }
  }

  const drivers = info?.drivers?.length ? info.drivers : FALLBACK_DRIVERS

  function validateForm(): boolean {
    const missingAck = !ack
    const missingDSN = driver === 'postgres' && !dsn.trim()
    setAckError(missingAck ? STORAGE.ackRequired : null)
    setDsnError(missingDSN ? STORAGE.postgresRequiresDSN : null)
    return !(missingAck || missingDSN)
  }

  function requestSubmit(e: FormEvent) {
    e.preventDefault()
    if (!validateForm()) return
    setConfirmMode('hot')
    setConfirmOpen(true)
  }

  function requestRestart() {
    if (!validateForm()) return
    setConfirmMode('restart')
    setConfirmOpen(true)
  }

  async function confirmAndSave() {
    setBusy(true)
    try {
      const resp = await putStoreSettings({
        driver,
        sqlite_path: sqlitePath.trim(),
        dsn: dsn.trim(),
        acknowledge_no_migrate: true,
        restart: confirmMode === 'restart',
      })
      const title =
        confirmMode === 'restart'
          ? (resp.message ?? STORAGE.restarting)
          : (resp.message ?? STORAGE.hotSwapped)
      push({ tone: 'success', title })
      setConfirmOpen(false)
      await load()
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="settings-panel">
      <PageHeader title={STORAGE.title} description={STORAGE.description} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />

      <Card>
        <form className="storage-form" onSubmit={requestSubmit}>
          <Field label={STORAGE.driverField}>
            <Select value={driver} onChange={(e) => { setDriver(e.target.value); setDsnError(null); setAckError(null) }} disabled={busy}>
              {drivers.map((d) => (
                <option key={d} value={d}>
                  {driverLabel(d)}
                </option>
              ))}
            </Select>
          </Field>

          {driver === 'sqlite' && (
            <Field label={STORAGE.sqlitePath} hint={STORAGE.sqliteHint}>
              <Input value={sqlitePath} onChange={(e) => setSQLitePath(e.target.value)} disabled={busy} />
            </Field>
          )}

          {driver === 'postgres' && (
            <Field
              label={STORAGE.dsn}
              hint={
                info?.dsn_redacted
                  ? `已保存：${info.dsn_redacted}，重新保存需再次填写完整连接串`
                  : '连接串形如 host=... user=... password=... dbname=...，仅保存在服务端配置。'
              }
              error={dsnError ?? undefined}
            >
              <Input
                type="password"
                value={dsn}
                onChange={(e) => { setDSN(e.target.value); setDsnError(null) }}
                disabled={busy}
              />
            </Field>
          )}

          <label className="ui-checkbox-row">
            <input
              type="checkbox"
              checked={ack}
              aria-invalid={ackError ? true : undefined}
              aria-describedby={ackError ? 'storage-ack-error' : undefined}
              onChange={(e) => { setAck(e.target.checked); setAckError(null) }}
              disabled={busy}
            />
            <span>{STORAGE.ack}</span>
          </label>
          {ackError && (
            <p className="ui-inline-error" id="storage-ack-error" data-testid="storage-ack-error">
              {ackError}
            </p>
          )}

          <div className="storage-actions">
            <Button type="submit" variant="primary" disabled={busy}>
              {busy && confirmMode === 'hot' ? STORAGE.saving : STORAGE.saveHotSwap}
            </Button>
            <Button type="button" variant="secondary" disabled={busy} onClick={requestRestart}>
              {STORAGE.saveRestart}
            </Button>
          </div>
        </form>
      </Card>

      {info?.config_path && (
        <details className="settings-developer">
          <summary>{STORAGE.developer}</summary>
          <p className="settings-meta">
            {STORAGE.developerConfig(info.config_path)}
            {info.overlay_path ? ` · ${STORAGE.developerOverlay(info.overlay_path)}` : ''}
            {info.effective_driver ? ` · ${STORAGE.developerRunning(info.effective_driver)}` : ''}
            {info.store_config_mismatch ? ` · ${STORAGE.developerMismatch}` : ''}
          </p>
        </details>
      )}

      <Card>
        <form
          className="storage-form"
          onSubmit={(e) => {
            e.preventDefault()
            void saveRetention()
          }}
        >
          <h2 className="settings-subheading">{STORAGE.retentionTitle}</h2>
          <p className="settings-muted">{STORAGE.retentionHint}</p>
          <Field label={STORAGE.retentionDaysLabel}>
            <Input
              type="number"
              min={0}
              max={3650}
              value={retentionDays}
              disabled={retentionBusy}
              onChange={(e) => setRetentionDays(e.target.value)}
              data-testid="attachment-retention-days"
            />
          </Field>
          <div className="storage-actions">
            <Button type="submit" variant="primary" disabled={retentionBusy}>
              {STORAGE.retentionSave}
            </Button>
          </div>
        </form>
      </Card>

      <Card>
        <h2 className="settings-subheading">{STORAGE.workspaceRetentionTitle}</h2>
        <p className="settings-muted">{STORAGE.workspaceRetentionHint}</p>
        {workspaceRetention == null ? null : workspaceRetention.length === 0 ? (
          <p className="settings-muted">{STORAGE.workspaceRetentionEmpty}</p>
        ) : (
          workspaceRetention.map((item) => (
            <div className="agent-budget-row" key={item.workspace_id}>
              <span className="agent-budget-id">{item.name || item.workspace_id}</span>
              <span className="settings-muted">
                {STORAGE.workspaceRetentionEffective(item.effective_days)}
              </span>
              <Input
                type="number"
                min={-1}
                max={3650}
                value={workspaceDrafts[item.workspace_id] ?? String(item.days)}
                disabled={workspaceBusy === item.workspace_id}
                onChange={(e) =>
                  setWorkspaceDrafts((prev) => ({
                    ...prev,
                    [item.workspace_id]: e.target.value,
                  }))
                }
                data-testid={`workspace-retention-days-${item.workspace_id}`}
              />
              <Button
                type="button"
                size="sm"
                variant="secondary"
                disabled={workspaceBusy === item.workspace_id}
                onClick={() => void saveWorkspaceRetention(item)}
                data-testid={`workspace-retention-save-${item.workspace_id}`}
              >
                {STORAGE.retentionSave}
              </Button>
            </div>
          ))
        )}
      </Card>

      <Card>
        <form
          className="storage-form"
          onSubmit={(e) => {
            e.preventDefault()
            void saveRunRetention()
          }}
        >
          <h2 className="settings-subheading">{STORAGE.runRetentionTitle}</h2>
          <p className="settings-muted">{STORAGE.runRetentionHint}</p>
          <Field label={STORAGE.runRetentionDaysLabel}>
            <Input
              type="number"
              min={0}
              max={3650}
              value={runRetentionDays}
              disabled={runRetentionBusy}
              onChange={(e) => setRunRetentionDays(e.target.value)}
              data-testid="run-retention-days"
            />
          </Field>
          <div className="storage-actions">
            <Button type="submit" variant="primary" disabled={runRetentionBusy}>
              {STORAGE.runRetentionSave}
            </Button>
          </div>
        </form>
      </Card>

      <Card>
        <form
          className="storage-form"
          onSubmit={(e) => {
            e.preventDefault()
            void saveMessageRetention()
          }}
        >
          <h2 className="settings-subheading">{STORAGE.messageRetentionTitle}</h2>
          <p className="settings-muted">{STORAGE.messageRetentionHint}</p>
          <Field label={STORAGE.messageRetentionDaysLabel}>
            <Input
              type="number"
              min={0}
              max={3650}
              value={messageRetentionDays}
              disabled={messageRetentionBusy}
              onChange={(e) => setMessageRetentionDays(e.target.value)}
              data-testid="message-retention-days"
            />
          </Field>
          <div className="storage-actions">
            <Button type="submit" variant="primary" disabled={messageRetentionBusy}>
              {STORAGE.messageRetentionSave}
            </Button>
          </div>
        </form>
      </Card>

      <ConfirmDialog
        open={confirmOpen}
        danger={confirmMode === 'restart'}
        title={confirmMode === 'restart' ? STORAGE.confirmRestartTitle : STORAGE.confirmHotTitle}
        body={confirmMode === 'restart' ? STORAGE.confirmRestartBody : STORAGE.confirmHotBody}
        confirmText={confirmMode === 'restart' ? STORAGE.saveRestart : STORAGE.saveHotSwap}
        busy={busy}
        onCancel={() => setConfirmOpen(false)}
        onConfirm={() => void confirmAndSave()}
      />
    </div>
  )
}
