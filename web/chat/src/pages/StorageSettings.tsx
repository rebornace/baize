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

  function requestSubmit(e: FormEvent) {
    e.preventDefault()
    // 两个错误来源分离：ack 行内错误对所有 driver 都展示；DSN 必填只挂 DSN Field
    const missingAck = !ack
    const missingDSN = driver === 'postgres' && !dsn.trim()
    setAckError(missingAck ? STORAGE.ackRequired : null)
    setDsnError(missingDSN ? STORAGE.postgresRequiresDSN : null)
    if (missingAck || missingDSN) return
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
        restart: true,
      })
      push({ tone: 'success', title: resp.message ?? STORAGE.restarting })
      setConfirmOpen(false)
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

          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? STORAGE.saving : STORAGE.saveRestart}
          </Button>
        </form>
      </Card>

      {info?.config_path && (
        <details className="settings-developer">
          <summary>{STORAGE.developer}</summary>
          <p className="settings-meta">
            配置：{info.config_path}
            {info.overlay_path ? ` · 覆盖：${info.overlay_path}` : ''}
          </p>
        </details>
      )}

      <ConfirmDialog
        open={confirmOpen}
        danger
        title={STORAGE.confirmRestartTitle}
        body={STORAGE.confirmRestartBody}
        confirmText={STORAGE.saveRestart}
        busy={busy}
        onCancel={() => setConfirmOpen(false)}
        onConfirm={() => void confirmAndSave()}
      />
    </div>
  )
}
