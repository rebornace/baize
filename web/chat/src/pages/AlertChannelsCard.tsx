import { useEffect, useState } from 'react'
import {
  getAlertChannels,
  getSMTPSettings,
  putAlertChannels,
  putSMTPSettings,
  testAlertChannel,
  type AlertChannel,
  type SMTPSettings,
} from '../api/settings/alerts'
import { Button, Field, Input, Select } from '../components/ui'
import { METRICS } from '../strings'

const TYPES: AlertChannel['type'][] = ['feishu', 'dingtalk', 'wecom', 'generic', 'email']

function newChannel(): AlertChannel {
  return { id: '', name: '', type: 'generic', url: '', enabled: true }
}

function recipients(raw: string): string[] {
  return raw
    .split(/[,，]/)
    .map((v) => v.trim())
    .filter(Boolean)
}

export function AlertChannelsCard() {
  const [items, setItems] = useState<AlertChannel[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState<string | null>(null)
  const [failed, setFailed] = useState(false)
  const [smtp, setSmtp] = useState<SMTPSettings | null>(null)
  const [smtpPassword, setSmtpPassword] = useState('')
  const [smtpBusy, setSmtpBusy] = useState(false)
  const [smtpStatus, setSmtpStatus] = useState<string | null>(null)

  useEffect(() => {
    void getAlertChannels()
      .then((r) => setItems(r.items ?? []))
      .catch(() => setFailed(true))
    void getSMTPSettings()
      .then(setSmtp)
      .catch(() => undefined)
  }, [])

  const patch = (index: number, next: Partial<AlertChannel>) => {
    setItems((prev) =>
      prev ? prev.map((item, i) => (i === index ? { ...item, ...next } : item)) : prev,
    )
    setStatus(null)
  }

  const save = async () => {
    if (!items) return
    setBusy(true)
    setFailed(false)
    try {
      const r = await putAlertChannels(items)
      setItems(r.items ?? [])
      setStatus(METRICS.alertChannelSaved)
    } catch {
      setFailed(true)
    } finally {
      setBusy(false)
    }
  }

  const test = async (channel: AlertChannel) => {
    setBusy(true)
    setFailed(false)
    try {
      await testAlertChannel(channel)
      setStatus(METRICS.alertChannelTestOk)
    } catch {
      setFailed(true)
    } finally {
      setBusy(false)
    }
  }

  const saveSMTP = async () => {
    if (!smtp) return
    setSmtpBusy(true)
    setSmtpStatus(null)
    try {
      const next = await putSMTPSettings({
        host: smtp.host,
        port: smtp.port,
        username: smtp.username,
        from: smtp.from,
        security: smtp.security,
        password: smtpPassword || undefined,
      })
      setSmtp(next)
      setSmtpPassword('')
      setSmtpStatus(METRICS.smtpSaved)
    } catch {
      setSmtpStatus(METRICS.smtpFailed)
    } finally {
      setSmtpBusy(false)
    }
  }

  if (failed && !items) return <p className="settings-muted">{METRICS.alertChannelLoadFailed}</p>
  if (!items) return <p className="settings-muted">{METRICS.refreshing}</p>

  return (
    <section className="run-overview" data-testid="alert-channels">
      <div className="run-overview-head">
        <h2 className="settings-subheading">{METRICS.alertChannelTitle}</h2>
      </div>
      <p className="settings-muted">{METRICS.alertChannelHint}</p>
      {items.length === 0 ? <p className="settings-muted">{METRICS.alertChannelEmpty}</p> : null}
      {items.map((ch, index) => (
        <div className="alert-channel-row" key={`${ch.id}-${index}`}>
          <Input
            placeholder={METRICS.alertChannelName}
            aria-label={METRICS.alertChannelName}
            value={ch.name}
            disabled={busy}
            onChange={(e) => patch(index, { name: e.target.value })}
          />
          <Select
            aria-label={METRICS.alertChannelType}
            value={ch.type}
            disabled={busy}
            onChange={(e) => patch(index, { type: e.target.value as AlertChannel['type'] })}
          >
            {TYPES.map((type) => (
              <option key={type} value={type}>
                {METRICS.alertChannelTypeLabels[type] ?? type}
              </option>
            ))}
          </Select>
          {ch.type === 'email' ? (
            <Input
              placeholder={METRICS.alertChannelRecipients}
              aria-label={METRICS.alertChannelRecipients}
              value={(ch.to ?? []).join(', ')}
              disabled={busy}
              onChange={(e) => patch(index, { to: recipients(e.target.value) })}
            />
          ) : (
            <Input
              placeholder="https://..."
              aria-label={METRICS.alertChannelURL}
              value={ch.url}
              disabled={busy}
              onChange={(e) => patch(index, { url: e.target.value })}
            />
          )}
          <label className="settings-muted">
            <input
              type="checkbox"
              checked={ch.enabled}
              disabled={busy}
              onChange={(e) => patch(index, { enabled: e.target.checked })}
            />{' '}
            {METRICS.alertChannelEnabled}
          </label>
          <Button
            type="button"
            size="sm"
            variant="secondary"
            disabled={busy || (ch.type === 'email' ? (ch.to?.length ?? 0) === 0 : !ch.url)}
            onClick={() => void test(ch)}
          >
            {METRICS.alertChannelTest}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={busy}
            onClick={() => setItems((prev) => (prev ? prev.filter((_, i) => i !== index) : prev))}
          >
            {METRICS.alertChannelRemove}
          </Button>
        </div>
      ))}
      <div className="settings-form-actions">
        <Button
          type="button"
          size="sm"
          variant="secondary"
          disabled={busy || items.length >= 20}
          onClick={() => setItems((prev) => [...(prev ?? []), newChannel()])}
        >
          {METRICS.alertChannelAdd}
        </Button>
        <Button type="button" size="sm" variant="primary" disabled={busy} onClick={() => void save()}>
          {METRICS.alertChannelSave}
        </Button>
        {status ? <span className="settings-muted">{status}</span> : null}
        {failed ? <span className="settings-error">{METRICS.alertChannelTestFailed}</span> : null}
      </div>

      {smtp ? (
        <details className="settings-developer" open>
          <summary>{METRICS.smtpTitle}</summary>
          <div className="smtp-grid">
            <Field label={METRICS.smtpHost}>
              <Input
                placeholder="smtp.example.com"
                value={smtp.host}
                disabled={smtpBusy}
                onChange={(e) => setSmtp({ ...smtp, host: e.target.value })}
              />
            </Field>
            <Field label={METRICS.smtpPort}>
              <Input
                type="number"
                min={1}
                max={65535}
                value={String(smtp.port)}
                disabled={smtpBusy}
                onChange={(e) => setSmtp({ ...smtp, port: Number(e.target.value) || 587 })}
              />
            </Field>
            <Field label={METRICS.smtpFrom}>
              <Input
                type="email"
                placeholder="alerts@example.com"
                value={smtp.from}
                disabled={smtpBusy}
                onChange={(e) => setSmtp({ ...smtp, from: e.target.value })}
              />
            </Field>
            <Field label={METRICS.smtpUsername}>
              <Input
                value={smtp.username}
                disabled={smtpBusy}
                onChange={(e) => setSmtp({ ...smtp, username: e.target.value })}
              />
            </Field>
            <Field label={METRICS.smtpPassword}>
              <Input
                type="password"
                placeholder={smtp.has_password ? METRICS.smtpPasswordSaved : undefined}
                value={smtpPassword}
                disabled={smtpBusy}
                onChange={(e) => setSmtpPassword(e.target.value)}
              />
            </Field>
            <Field label={METRICS.smtpSecurity}>
              <Select
                value={smtp.security}
                disabled={smtpBusy}
                onChange={(e) =>
                  setSmtp({ ...smtp, security: e.target.value as SMTPSettings['security'] })
                }
              >
                <option value="starttls">starttls</option>
                <option value="tls">tls</option>
                <option value="none">none</option>
              </Select>
            </Field>
          </div>
          <div className="settings-form-actions">
            <Button
              type="button"
              size="sm"
              variant="secondary"
              disabled={smtpBusy}
              onClick={() => void saveSMTP()}
            >
              {METRICS.smtpSave}
            </Button>
            {smtpStatus ? <span className="settings-muted">{smtpStatus}</span> : null}
          </div>
          <p className="settings-muted">{METRICS.smtpHint}</p>
        </details>
      ) : null}
    </section>
  )
}
