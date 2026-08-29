import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { getStoreSettings, putStoreSettings, type StoreSettings } from '../api'

export function StorageSettings() {
  const [info, setInfo] = useState<StoreSettings | null>(null)
  const [driver, setDriver] = useState('sqlite')
  const [sqlitePath, setSQLitePath] = useState('./data/baize.db')
  const [dsn, setDSN] = useState('')
  const [ack, setAck] = useState(false)
  const [status, setStatus] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    setError(null)
    try {
      const s = await getStoreSettings()
      setInfo(s)
      setDriver(s.driver || 'sqlite')
      setSQLitePath(s.sqlite_path || './data/baize.db')
      setDSN('')
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (!ack) {
      setError('请勾选确认：切换存储不会自动迁移数据')
      return
    }
    if (driver === 'postgres' && !dsn.trim() && !info?.dsn_redacted) {
      setError('PostgreSQL 需要填写 DSN')
      return
    }
    if (
      !window.confirm(
        '将保存存储配置并重启 Baize。进行中的 Run 会中断；数据不会从旧库自动迁移。继续？',
      )
    ) {
      return
    }
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      const resp = await putStoreSettings({
        driver,
        sqlite_path: sqlitePath.trim(),
        dsn: dsn.trim(),
        acknowledge_no_migrate: true,
        restart: true,
      })
      setStatus(resp.message ?? '正在重启…')
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="settings-panel">
      <h2>存储</h2>
      <p className="settings-hint">
        选择 SQL 主存储驱动。保存后进程将自动重启；<strong>不会</strong>从旧库迁移数据。
      </p>
      {info?.config_path && (
        <p className="settings-meta">
          配置：{info.config_path}
          {info.overlay_path ? ` · 覆盖：${info.overlay_path}` : null}
        </p>
      )}
      {error && <p className="settings-error">{error}</p>}
      {status && <p className="settings-ok">{status}</p>}
      <form onSubmit={(e) => void onSubmit(e)} className="settings-form">
        <label>
          驱动
          <select value={driver} onChange={(e) => setDriver(e.target.value)} disabled={busy}>
            {(info?.drivers ?? ['memory', 'sqlite', 'postgres']).map((d) => (
              <option key={d} value={d}>
                {d}
              </option>
            ))}
          </select>
        </label>
        {driver === 'sqlite' && (
          <label>
            SQLite 路径
            <input
              value={sqlitePath}
              onChange={(e) => setSQLitePath(e.target.value)}
              disabled={busy}
            />
          </label>
        )}
        {driver === 'postgres' && (
          <label>
            DSN
            <input
              type="password"
              placeholder={info?.dsn_redacted || 'postgres://user:pass@host:5432/baize?sslmode=disable'}
              value={dsn}
              onChange={(e) => setDSN(e.target.value)}
              disabled={busy}
            />
          </label>
        )}
        <label className="settings-checkbox">
          <input type="checkbox" checked={ack} onChange={(e) => setAck(e.target.checked)} disabled={busy} />
          我了解：切换存储不会自动迁移数据，旧库文件/连接中的数据需自行处理
        </label>
        <button type="submit" disabled={busy}>
          {busy ? '保存并重启…' : '保存并重启'}
        </button>
      </form>
    </div>
  )
}
