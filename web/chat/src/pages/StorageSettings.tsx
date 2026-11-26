import { type FormEvent, useCallback, useEffect, useState } from 'react'
import {
  getAgentRunRetention,
  getArchiveRetention,
  getAttachmentRetention,
  getMessageRetention,
  getOperatorLimits,
  getRetentionPreview,
  getRetentionInterval,
  getRunRetention,
  getStoreSettings,
  getWorkspaceAttachmentRetention,
  getWorkspaceRetention,
  listBackups,
  createBackup,
  downloadBackup,
  stageRestore,
  putAgentRunRetention,
  putArchiveRetention,
  putAttachmentRetention,
  putMessageRetention,
  putOperatorLimits,
  putRetentionInterval,
  putRunRetention,
  putStoreSettings,
  putWorkspaceAttachmentRetention,
  putWorkspaceRetention,
  runRetentionNow,
  type AgentRunRetentionItem,
  type BackupInfo,
  type RetentionPreview,
  type StoreSettings,
  type WorkspaceAttachmentRetentionItem,
  type WorkspaceRetentionItem,
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
  const [archiveRetentionDays, setArchiveRetentionDays] = useState('0')
  const [archiveRetentionBusy, setArchiveRetentionBusy] = useState(false)
  const [retentionIntervalHours, setRetentionIntervalHours] = useState('6')
  const [retentionIntervalBusy, setRetentionIntervalBusy] = useState(false)
  const [rpmLimit, setRpmLimit] = useState('0')
  const [dailyRunLimit, setDailyRunLimit] = useState('0')
  const [concurrentLimit, setConcurrentLimit] = useState('0')
  const [limitsBusy, setLimitsBusy] = useState(false)
  const [backups, setBackups] = useState<BackupInfo[] | null>(null)
  const [backupBusy, setBackupBusy] = useState(false)
  const [restoreFile, setRestoreFile] = useState<File | null>(null)
  const [backupMessage, setBackupMessage] = useState<string | null>(null)
  const [workspaceRetention, setWorkspaceRetention] = useState<
    WorkspaceAttachmentRetentionItem[] | null
  >(null)
  const [workspaceDrafts, setWorkspaceDrafts] = useState<Record<string, string>>({})
  const [workspaceBusy, setWorkspaceBusy] = useState<string | null>(null)
  const [agentRunRetention, setAgentRunRetention] = useState<AgentRunRetentionItem[] | null>(null)
  const [agentRunDrafts, setAgentRunDrafts] = useState<Record<string, string>>({})
  const [agentRunGlobalDays, setAgentRunGlobalDays] = useState(0)
  const [agentRunBusy, setAgentRunBusy] = useState<string | null>(null)
  const [wsRetention, setWsRetention] = useState<WorkspaceRetentionItem[] | null>(null)
  const [wsMessageDrafts, setWsMessageDrafts] = useState<Record<string, string>>({})
  const [wsRunDrafts, setWsRunDrafts] = useState<Record<string, string>>({})
  const [wsRetentionBusy, setWsRetentionBusy] = useState<string | null>(null)
  const [preview, setPreview] = useState<RetentionPreview | null>(null)
  const [previewBusy, setPreviewBusy] = useState(false)
  const [previewError, setPreviewError] = useState<string | null>(null)
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
        const r = await getArchiveRetention()
        setArchiveRetentionDays(String(r.days))
      } catch {
        // Optional section; keep the default when the endpoint is unavailable.
      }
      try {
        const r = await getRetentionInterval()
        setRetentionIntervalHours(String(r.hours))
      } catch {
        // Optional section; keep the default when the endpoint is unavailable.
      }
      try {
        const r = await getOperatorLimits()
        setRpmLimit(String(r.requests_per_minute))
        setDailyRunLimit(String(r.runs_per_day))
        setConcurrentLimit(String(r.max_concurrent_runs))
      } catch {
        // Optional section; keep the default when the endpoint is unavailable.
      }
      try {
        const r = await listBackups()
        setBackups(r.items ?? [])
      } catch {
        setBackups(null)
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
      try {
        const r = await getAgentRunRetention()
        setAgentRunRetention(Array.isArray(r.items) ? r.items : [])
        setAgentRunGlobalDays(r.global_days ?? 0)
        const drafts: Record<string, string> = {}
        for (const item of r.items ?? []) {
          drafts[item.agent_id] = String(item.days)
        }
        setAgentRunDrafts(drafts)
      } catch {
        setAgentRunRetention(null)
      }
      try {
        const r = await getWorkspaceRetention()
        setWsRetention(Array.isArray(r.items) ? r.items : [])
        const messageDrafts: Record<string, string> = {}
        const runDrafts: Record<string, string> = {}
        for (const item of r.items ?? []) {
          messageDrafts[item.workspace_id] = String(item.message_days)
          runDrafts[item.workspace_id] = String(item.run_days)
        }
        setWsMessageDrafts(messageDrafts)
        setWsRunDrafts(runDrafts)
      } catch {
        setWsRetention(null)
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

  const saveAgentRunRetention = async (item: AgentRunRetentionItem) => {
    const raw = agentRunDrafts[item.agent_id] ?? String(item.days)
    const days = Number.parseInt(raw, 10)
    if (!Number.isFinite(days) || days < -1 || days > 3650) {
      push({ tone: 'error', title: STORAGE.agentRunRetentionInvalid })
      return
    }
    setAgentRunBusy(item.agent_id)
    try {
      const next = await putAgentRunRetention(item.agent_id, days)
      setAgentRunRetention((prev) =>
        prev
          ? prev.map((row) => (row.agent_id === next.agent_id ? { ...row, days: next.days } : row))
          : prev,
      )
      setAgentRunDrafts((prev) => ({ ...prev, [next.agent_id]: String(next.days) }))
      push({ tone: 'success', title: STORAGE.retentionSaved })
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setAgentRunBusy(null)
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

  const saveArchiveRetention = async () => {
    const n = Number.parseInt(archiveRetentionDays, 10)
    if (!Number.isFinite(n) || n < 0 || n > 3650) {
      push({ tone: 'error', title: STORAGE.archiveRetentionInvalid })
      return
    }
    setArchiveRetentionBusy(true)
    try {
      const r = await putArchiveRetention(n)
      setArchiveRetentionDays(String(r.days))
      push({ tone: 'success', title: STORAGE.archiveRetentionSaved })
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setArchiveRetentionBusy(false)
    }
  }

  const saveRetentionInterval = async () => {
    const n = Number.parseInt(retentionIntervalHours, 10)
    if (!Number.isFinite(n) || n < 1 || n > 168) {
      push({ tone: 'error', title: STORAGE.retentionIntervalInvalid })
      return
    }
    setRetentionIntervalBusy(true)
    try {
      const r = await putRetentionInterval(n)
      setRetentionIntervalHours(String(r.hours))
      push({ tone: 'success', title: STORAGE.retentionIntervalSaved })
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setRetentionIntervalBusy(false)
    }
  }

  const saveOperatorLimits = async () => {
    const rpm = Number.parseInt(rpmLimit, 10)
    const daily = Number.parseInt(dailyRunLimit, 10)
    const concurrent = Number.parseInt(concurrentLimit, 10)
    if (
      !Number.isFinite(rpm) || rpm < 0 || rpm > 100000 ||
      !Number.isFinite(daily) || daily < 0 || daily > 100000 ||
      !Number.isFinite(concurrent) || concurrent < 0 || concurrent > 10000
    ) {
      push({ tone: 'error', title: STORAGE.operatorLimitsInvalid })
      return
    }
    setLimitsBusy(true)
    try {
      const next = await putOperatorLimits({
        requests_per_minute: rpm,
        runs_per_day: daily,
        max_concurrent_runs: concurrent,
      })
      setRpmLimit(String(next.requests_per_minute))
      setDailyRunLimit(String(next.runs_per_day))
      setConcurrentLimit(String(next.max_concurrent_runs))
      push({ tone: 'success', title: STORAGE.operatorLimitsSaved })
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setLimitsBusy(false)
    }
  }

  const backupNow = async () => {
    setBackupBusy(true)
    setBackupMessage(null)
    try {
      const info = await createBackup()
      setBackups((prev) => [info, ...(prev ?? [])])
      setBackupMessage(STORAGE.backupCreated(info.name))
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBackupBusy(false)
    }
  }

  const restoreNow = async () => {
    if (!restoreFile) return
    setBackupBusy(true)
    setBackupMessage(null)
    try {
      const res = await stageRestore(restoreFile)
      setBackupMessage(res.message ?? STORAGE.backupRestoreStaged)
      setRestoreFile(null)
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBackupBusy(false)
    }
  }

  const saveWorkspaceMessageRunRetention = async (item: WorkspaceRetentionItem) => {    const messageRaw = wsMessageDrafts[item.workspace_id] ?? String(item.message_days)
    const runRaw = wsRunDrafts[item.workspace_id] ?? String(item.run_days)
    const messageDays = Number.parseInt(messageRaw, 10)
    const runDays = Number.parseInt(runRaw, 10)
    if (
      !Number.isFinite(messageDays) || messageDays < -1 || messageDays > 3650 ||
      !Number.isFinite(runDays) || runDays < -1 || runDays > 3650
    ) {
      push({ tone: 'error', title: STORAGE.workspaceMessageRunInvalid })
      return
    }
    setWsRetentionBusy(item.workspace_id)
    try {
      const next = await putWorkspaceRetention(item.workspace_id, {
        message_days: messageDays,
        run_days: runDays,
      })
      setWsRetention((prev) =>
        prev ? prev.map((row) => (row.workspace_id === next.workspace_id ? next : row)) : prev,
      )
      setWsMessageDrafts((prev) => ({ ...prev, [next.workspace_id]: String(next.message_days) }))
      setWsRunDrafts((prev) => ({ ...prev, [next.workspace_id]: String(next.run_days) }))
      push({ tone: 'success', title: STORAGE.retentionSaved })
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setWsRetentionBusy(null)
    }
  }

  const loadPreview = async () => {
    setPreviewBusy(true)
    setPreviewError(null)
    try {
      setPreview(await getRetentionPreview())
    } catch (e) {
      setPreviewError(STORAGE.previewFailed)
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setPreviewBusy(false)
    }
  }

  const runRetention = async () => {
    if (!preview) return
    if (!window.confirm(
      STORAGE.previewRunConfirm(
        preview.messages.total,
        preview.runs.total,
        preview.archived.total,
      ),
    )) {
      return
    }
    setPreviewBusy(true)
    setPreviewError(null)
    try {
      const res = await runRetentionNow()
      push({
        tone: 'success',
        title: STORAGE.previewRunDone(
          res.messages_deleted,
          res.runs_deleted,
          res.archived_deleted,
        ),
      })
      setPreview(await getRetentionPreview())
    } catch (e) {
      setPreviewError(STORAGE.previewFailed)
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setPreviewBusy(false)
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
        <h2 className="settings-subheading">{STORAGE.agentRunRetentionTitle}</h2>
        <p className="settings-muted">{STORAGE.agentRunRetentionHint}</p>
        {agentRunRetention == null ? null : agentRunRetention.length === 0 ? (
          <p className="settings-muted">{STORAGE.agentRunRetentionEmpty}</p>
        ) : (
          agentRunRetention.map((item) => {
            const days = item.days >= 0 ? item.days : agentRunGlobalDays
            return (
              <div className="agent-budget-row" key={item.agent_id}>
                <span className="agent-budget-id">{item.agent_id}</span>
                <span className="settings-muted">
                  {STORAGE.agentRunRetentionEffective(days)}
                </span>
                <Input
                  type="number"
                  min={-1}
                  max={3650}
                  value={agentRunDrafts[item.agent_id] ?? String(item.days)}
                  disabled={agentRunBusy === item.agent_id}
                  onChange={(e) =>
                    setAgentRunDrafts((prev) => ({
                      ...prev,
                      [item.agent_id]: e.target.value,
                    }))
                  }
                  data-testid={`agent-run-retention-days-${item.agent_id}`}
                />
                <Button
                  type="button"
                  size="sm"
                  variant="secondary"
                  disabled={agentRunBusy === item.agent_id}
                  onClick={() => void saveAgentRunRetention(item)}
                  data-testid={`agent-run-retention-save-${item.agent_id}`}
                >
                  {STORAGE.retentionSave}
                </Button>
              </div>
            )
          })
        )}
      </Card>

      <Card>
        <h2 className="settings-subheading">{STORAGE.workspaceMessageRunTitle}</h2>
        <p className="settings-muted">{STORAGE.workspaceMessageRunHint}</p>
        {wsRetention == null ? null : wsRetention.length === 0 ? (
          <p className="settings-muted">{STORAGE.workspaceMessageRunEmpty}</p>
        ) : (
          wsRetention.map((item) => (
            <div className="agent-budget-row" key={item.workspace_id}>
              <span className="agent-budget-id">{item.name || item.workspace_id}</span>
              <span className="settings-muted">
                {STORAGE.workspaceMessageRunEffective(
                  item.effective_message_days,
                  item.effective_run_days,
                )}
              </span>
              <label className="settings-muted">
                {STORAGE.workspaceMessageDays}
                <Input
                  type="number"
                  min={-1}
                  max={3650}
                  value={wsMessageDrafts[item.workspace_id] ?? String(item.message_days)}
                  disabled={wsRetentionBusy === item.workspace_id}
                  onChange={(e) =>
                    setWsMessageDrafts((prev) => ({
                      ...prev,
                      [item.workspace_id]: e.target.value,
                    }))
                  }
                  data-testid={`workspace-message-retention-${item.workspace_id}`}
                />
              </label>
              <label className="settings-muted">
                {STORAGE.workspaceRunDays}
                <Input
                  type="number"
                  min={-1}
                  max={3650}
                  value={wsRunDrafts[item.workspace_id] ?? String(item.run_days)}
                  disabled={wsRetentionBusy === item.workspace_id}
                  onChange={(e) =>
                    setWsRunDrafts((prev) => ({
                      ...prev,
                      [item.workspace_id]: e.target.value,
                    }))
                  }
                  data-testid={`workspace-run-retention-${item.workspace_id}`}
                />
              </label>
              <Button
                type="button"
                size="sm"
                variant="secondary"
                disabled={wsRetentionBusy === item.workspace_id}
                onClick={() => void saveWorkspaceMessageRunRetention(item)}
                data-testid={`workspace-message-run-retention-save-${item.workspace_id}`}
              >
                {STORAGE.retentionSave}
              </Button>
            </div>
          ))
        )}
      </Card>

      <Card>
        <h2 className="settings-subheading">{STORAGE.previewTitle}</h2>
        <p className="settings-muted">{STORAGE.previewHint}</p>
        <div className="storage-actions">
          <Button type="button" variant="secondary" disabled={previewBusy} onClick={() => void loadPreview()}
            data-testid="retention-preview-load">
            {previewBusy ? STORAGE.previewLoading : STORAGE.previewButton}
          </Button>
          {preview &&
          (preview.messages.total > 0 ||
            preview.runs.total > 0 ||
            preview.archived.total > 0) ? (
            <Button type="button" variant="danger" disabled={previewBusy} onClick={() => void runRetention()}
              data-testid="retention-preview-run">
              {STORAGE.previewRunNow}
            </Button>
          ) : null}
        </div>
        {previewError ? <p className="settings-error">{previewError}</p> : null}
        {preview ? (
          <div data-testid="retention-preview">
            <p className="settings-muted">
              {STORAGE.previewMessages(
                preview.messages.total,
                preview.messages.conversations.length,
              )}
            </p>
            <p className="settings-muted">
              {STORAGE.previewRuns(
                preview.runs.total,
                preview.runs.conversations.length,
                preview.runs.agents.length,
              )}
            </p>
            <p className="settings-muted">
              {STORAGE.previewArchived(preview.archived.total, preview.archived.days)}
            </p>
            {preview.runs.blocked ? (
              <p className="settings-error">{STORAGE.previewBlocked}</p>
            ) : null}
            {preview.messages.conversations.length > 0 ? (
              <details className="settings-developer">
                <summary>{STORAGE.previewDetails}</summary>
                <ul className="settings-muted">
                  {preview.messages.conversations.map((row) => (
                    <li key={row.conversation_id}>
                      {STORAGE.previewMessageItem(row.conversation_id, row.count, row.days)}
                    </li>
                  ))}
                </ul>
              </details>
            ) : null}
          </div>
        ) : null}
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

      <Card>
        <form
          className="storage-form"
          onSubmit={(e) => {
            e.preventDefault()
            void saveArchiveRetention()
          }}
        >
          <h2 className="settings-subheading">{STORAGE.archiveRetentionTitle}</h2>
          <p className="settings-muted">{STORAGE.archiveRetentionHint}</p>
          <Field label={STORAGE.archiveRetentionDaysLabel}>
            <Input
              type="number"
              min={0}
              max={3650}
              value={archiveRetentionDays}
              disabled={archiveRetentionBusy}
              onChange={(e) => setArchiveRetentionDays(e.target.value)}
              data-testid="archive-retention-days"
            />
          </Field>
          <div className="storage-actions">
            <Button type="submit" variant="primary" disabled={archiveRetentionBusy}>
              {STORAGE.archiveRetentionSave}
            </Button>
          </div>
        </form>
      </Card>

      <Card>
        <form
          className="storage-form"
          onSubmit={(e) => {
            e.preventDefault()
            void saveRetentionInterval()
          }}
        >
          <h2 className="settings-subheading">{STORAGE.retentionIntervalTitle}</h2>
          <p className="settings-muted">{STORAGE.retentionIntervalHint}</p>
          <Field label={STORAGE.retentionIntervalLabel}>
            <Input
              type="number"
              min={1}
              max={168}
              value={retentionIntervalHours}
              disabled={retentionIntervalBusy}
              onChange={(e) => setRetentionIntervalHours(e.target.value)}
              data-testid="retention-interval-hours"
            />
          </Field>
          <div className="storage-actions">
            <Button type="submit" variant="primary" disabled={retentionIntervalBusy}>
              {STORAGE.retentionIntervalSave}
            </Button>
          </div>
        </form>
      </Card>

      <Card>
        <form
          className="storage-form"
          onSubmit={(e) => {
            e.preventDefault()
            void saveOperatorLimits()
          }}
        >
          <h2 className="settings-subheading">{STORAGE.operatorLimitsTitle}</h2>
          <p className="settings-muted">{STORAGE.operatorLimitsHint}</p>
          <Field label={STORAGE.operatorLimitsRPM}>
            <Input
              type="number"
              min={0}
              value={rpmLimit}
              disabled={limitsBusy}
              onChange={(e) => setRpmLimit(e.target.value)}
              data-testid="operator-limit-rpm"
            />
          </Field>
          <Field label={STORAGE.operatorLimitsDaily}>
            <Input
              type="number"
              min={0}
              value={dailyRunLimit}
              disabled={limitsBusy}
              onChange={(e) => setDailyRunLimit(e.target.value)}
              data-testid="operator-limit-daily-runs"
            />
          </Field>
          <Field label={STORAGE.operatorLimitsConcurrent}>
            <Input
              type="number"
              min={0}
              value={concurrentLimit}
              disabled={limitsBusy}
              onChange={(e) => setConcurrentLimit(e.target.value)}
              data-testid="operator-limit-concurrent"
            />
          </Field>
          <div className="storage-actions">
            <Button type="submit" variant="primary" disabled={limitsBusy}>
              {STORAGE.operatorLimitsSave}
            </Button>
          </div>
        </form>
      </Card>

      <Card>
        <h2 className="settings-subheading">{STORAGE.backupTitle}</h2>
        <p className="settings-muted">{STORAGE.backupHint}</p>
        <div className="storage-actions">
          <Button type="button" variant="primary" disabled={backupBusy} onClick={() => void backupNow()}>
            {STORAGE.backupNow}
          </Button>
        </div>
        {backupMessage ? <p className="settings-muted">{backupMessage}</p> : null}
        {backups && backups.length > 0 ? (
          <ul className="settings-muted" data-testid="backup-list">
            {backups.map((b) => (
              <li key={b.name}>
                {b.name} · {b.size} B · {new Date(b.created_at).toLocaleString()}{' '}
                <button
                  type="button"
                  className="settings-link"
                  onClick={() => void downloadBackup(b.name).catch(() => undefined)}
                >
                  {STORAGE.backupDownload}
                </button>
              </li>
            ))}
          </ul>
        ) : (
          <p className="settings-muted">{STORAGE.backupEmpty}</p>
        )}
        <div className="storage-actions">
          <input
            type="file"
            accept=".db,application/vnd.sqlite3,application/octet-stream"
            disabled={backupBusy}
            onChange={(e) => setRestoreFile(e.target.files?.[0] ?? null)}
            data-testid="restore-file"
          />
          <Button
            type="button"
            variant="secondary"
            disabled={backupBusy || !restoreFile}
            onClick={() => void restoreNow()}
          >
            {STORAGE.backupRestore}
          </Button>
        </div>
        <p className="settings-muted">{STORAGE.backupRestoreHint}</p>
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
