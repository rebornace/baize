import { type FormEvent, useCallback, useEffect, useState } from 'react'
import {
  getStoreSettings,
  putStoreSettings,
  type StoreSettings,
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
