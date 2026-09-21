import { type FormEvent, useCallback, useEffect, useRef, useState } from 'react'
import {
  getChannelOutboundDeliveries,
  getWeixinLoginStatus,
  getWeixinSettings,
  logoutWeixin,
  putWeixinSettings,
  restartWeixinProcess,
  retryChannelOutboundDelivery,
  startWeixinLogin,
  startWeixinProcess,
  stopWeixinProcess,
  type ChannelOutboundDelivery,
  type WeixinChannelSettings,
} from '../api'
import {
  Badge,
  Button,
  ConfirmDialog,
  PageHeader,
  ToastRegion,
  useToast,
  type BadgeTone,
} from '../components/ui'
import { useGate } from '../gateContext'
import { qrDataUrlFromText } from '../qrDataUrl'
import { WEIXIN, friendlyError } from '../strings'

const POLL_MS = 2000
const WEIXIN_CHANNEL = 'weixin'

export function parseAllowlistText(text: string): string[] {
  return text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line !== '')
}

export function formatAllowlistText(list: string[] | undefined): string {
  return (list ?? []).join('\n')
}

export function loginStatusLabel(status: string): string {
  switch (status) {
    case 'pending':
      return WEIXIN.loginPending
    case 'success':
      return WEIXIN.loginSuccess
    case 'expired':
      return WEIXIN.loginExpired
    default:
      return status || WEIXIN.loginUnknown
  }
}

export function formatOutboundStatus(status: string): string {
  switch (status) {
    case 'dead':
      return WEIXIN.statusDead
    case 'pending':
      return WEIXIN.statusPending
    case 'delivered':
      return WEIXIN.statusDelivered
    default:
      return status
  }
}

function outboundBadgeTone(status: string): BadgeTone {
  switch (status) {
    case 'dead':
      return 'danger'
    case 'pending':
      return 'warning'
    case 'delivered':
      return 'success'
    default:
      return 'neutral'
  }
}

export function outboundSummary(d: ChannelOutboundDelivery): string {
  const peer = d.peer_id || d.conversation_id || d.run_id || d.id
  const err = d.last_error ? ` · ${d.last_error}` : ''
  return `${peer} · ${d.kind} · ${d.attempt}/${d.max_attempts}${err}`
}

export function WeixinChannelSettings() {
  const [agentId, setAgentId] = useState('')
  const [assignee, setAssignee] = useState('')
  const [allowlistText, setAllowlistText] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [deliveries, setDeliveries] = useState<ChannelOutboundDelivery[]>([])
  const [retryingId, setRetryingId] = useState<string | null>(null)
  const [running, setRunning] = useState<boolean | null>(null)
  const [runReason, setRunReason] = useState<string | null>(null)
  const [confirmLogout, setConfirmLogout] = useState(false)
  const [confirmStop, setConfirmStop] = useState(false)
  const { toasts, push, dismiss } = useToast()

  const { role } = useGate()
  const isAdmin = role === 'admin'
  const uiBusy = busy || retryingId !== null

  const [ticket, setTicket] = useState<string | null>(null)
  const [qrUrl, setQrUrl] = useState<string | null>(null)
  /** PNG data URL rendered from qrUrl (liteapp link is not an image). */
  const [qrImgSrc, setQrImgSrc] = useState<string | null>(null)
  const [loginStatus, setLoginStatus] = useState<string | null>(null)
  const pollRef = useRef<number | null>(null)
  /** Whether a login-status poll loop is active (guards the self-chaining loop). */
  const pollingActiveRef = useRef(false)

  const pushError = useCallback(
    (err: unknown, title?: string) => {
      const f = friendlyError(err)
      push({ tone: 'error', title: title ?? f.title, detail: title ? (f.detail ?? f.title) : f.detail })
    },
    [push],
  )

  const stopPoll = useCallback(() => {
    pollingActiveRef.current = false
    if (pollRef.current !== null) {
      window.clearTimeout(pollRef.current)
      pollRef.current = null
    }
  }, [])

  useEffect(() => () => stopPoll(), [stopPoll])

  useEffect(() => {
    if (!qrUrl) {
      setQrImgSrc(null)
      return
    }
    let cancelled = false
    void qrDataUrlFromText(qrUrl)
      .then((dataUrl) => {
        if (!cancelled) setQrImgSrc(dataUrl)
      })
      .catch((err) => {
        if (!cancelled) {
          setQrImgSrc(null)
          pushError(err)
        }
      })
    return () => {
      cancelled = true
    }
  }, [qrUrl, pushError])

  const applySettings = useCallback((s: WeixinChannelSettings) => {
    setAgentId(s.agent_id ?? '')
    setAssignee(s.assignee ?? '')
    setAllowlistText(formatAllowlistText(s.allowlist))
    setEnabled(Boolean(s.enabled))
    // running/reason come from both GET (current state) and PUT (just applied).
    if (typeof s.running === 'boolean') {
      setRunning(s.running)
      setRunReason(s.reason ?? null)
    }
  }, [])

  const loadDeliveries = useCallback(async () => {
    try {
      const rows = await getChannelOutboundDeliveries(WEIXIN_CHANNEL)
      setDeliveries(rows)
    } catch {
      setDeliveries([])
    }
  }, [])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [s] = await Promise.all([getWeixinSettings(), loadDeliveries()])
      applySettings(s)
    } catch (err) {
      pushError(err, WEIXIN.loadFailed)
    } finally {
      setLoading(false)
    }
  }, [applySettings, loadDeliveries, pushError])

  useEffect(() => {
    void load()
  }, [load])

  const startPolling = useCallback(
    (loginTicket: string) => {
      stopPoll()
      pollingActiveRef.current = true
      // Self-chaining loop (NOT setInterval): login/status is a long poll held
      // open ~30s by iLink until scan/confirm. Firing a fresh request every
      // POLL_MS would pile up dozens of concurrent in-flight requests and
      // exhaust the browser's per-origin connection pool, stalling the UI.
      // Schedule the next tick only after the current one settles, and keep
      // at most one request in flight.
      const tick = async () => {
        if (!pollingActiveRef.current) return
        try {
          const res = await getWeixinLoginStatus(loginTicket)
          if (!pollingActiveRef.current) return
          setLoginStatus(res.status)
          if (res.status === 'success' || res.status === 'expired') {
            stopPoll()
            if (res.status === 'success') {
              push({ tone: 'success', title: WEIXIN.toastLoggedIn })
              setTicket(null)
              setQrUrl(null)
            }
            return
          }
        } catch (err) {
          if (!pollingActiveRef.current) return
          stopPoll()
          pushError(err)
          return
        }
        if (pollingActiveRef.current) {
          pollRef.current = window.setTimeout(tick, POLL_MS)
        }
      }
      void tick()
    },
    [push, pushError, stopPoll],
  )

  const onStartLogin = async () => {
    setBusy(true)
    setLoginStatus(null)
    stopPoll()
    try {
      const res = await startWeixinLogin()
      setTicket(res.ticket)
      setQrUrl(res.qr_url)
      setLoginStatus('pending')
      startPolling(res.ticket)
    } catch (err) {
      pushError(err)
      setTicket(null)
      setQrUrl(null)
    } finally {
      setBusy(false)
    }
  }

  const onLogout = async () => {
    setBusy(true)
    setConfirmLogout(false)
    stopPoll()
    setTicket(null)
    setQrUrl(null)
    setLoginStatus(null)
    try {
      await logoutWeixin()
      push({ tone: 'success', title: WEIXIN.toastLoggedOut })
    } catch (err) {
      pushError(err)
    } finally {
      setBusy(false)
    }
  }

  /**
   * Process-level control (start/stop/restart the adapter OS process), as
   * opposed to the enabled toggle which only starts/stops polling. After the
   * action we re-apply the returned reconciled settings (running/reason).
   */
  const onProcessAction = async (action: 'start' | 'stop' | 'restart') => {
    const toastTitles = {
      start: WEIXIN.toastProcessStart,
      stop: WEIXIN.toastProcessStop,
      restart: WEIXIN.toastProcessRestart,
    } as const
    setBusy(true)
    if (action === 'stop') setConfirmStop(false)
    try {
      const saved =
        action === 'start'
          ? await startWeixinProcess()
          : action === 'stop'
            ? await stopWeixinProcess()
            : await restartWeixinProcess()
      applySettings(saved)
      push({ tone: 'success', title: toastTitles[action] })
      await load() // refresh running/reason after the process settles
    } catch (err) {
      pushError(err)
    } finally {
      setBusy(false)
    }
  }

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const saved = await putWeixinSettings({
        agent_id: agentId.trim(),
        assignee: assignee.trim(),
        allowlist: parseAllowlistText(allowlistText),
        enabled,
      })
      applySettings(saved)
      push({ tone: 'success', title: WEIXIN.toastSaved })
    } catch (err) {
      pushError(err)
    } finally {
      setBusy(false)
    }
  }

  const onRetry = async (id: string) => {
    setRetryingId(id)
    try {
      await retryChannelOutboundDelivery(WEIXIN_CHANNEL, id)
      push({ tone: 'success', title: WEIXIN.toastRetryQueued })
      await loadDeliveries()
    } catch (err) {
      pushError(err)
    } finally {
      setRetryingId(null)
    }
  }

  return (
    <div className="settings-section">
      <PageHeader title={WEIXIN.title} description={WEIXIN.description} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />

      {loading && <p className="settings-muted">{WEIXIN.loading}</p>}
      {running !== null && (
        <p className="settings-muted">
          {WEIXIN.runStatusPrefix}
          {running ? (
            <span className="settings-badge">{WEIXIN.runPolling}</span>
          ) : (
            <span className="settings-badge">{WEIXIN.runStopped}</span>
          )}
          {runReason === 'login_required' &&
            (isAdmin ? WEIXIN.runReasonLoginAdmin : WEIXIN.runReasonLoginOperator)}
          {runReason === 'login_expired' && WEIXIN.runReasonLoginExpired}
          {runReason === 'start_failed' && WEIXIN.runReasonStartFailed}
          {runReason === 'stopped' &&
            (isAdmin ? WEIXIN.runReasonStoppedAdmin : WEIXIN.runReasonStoppedOperator)}
        </p>
      )}

      {isAdmin && (
        <section className="weixin-login-block">
          <h2 className="settings-subheading">{WEIXIN.processSection}</h2>
          <div className="weixin-login-actions">
            <button
              type="button"
              className="btn primary"
              disabled={uiBusy}
              onClick={() => void onProcessAction('start')}
            >
              {WEIXIN.processStart}
            </button>
            <button
              type="button"
              className="btn ghost"
              disabled={uiBusy}
              onClick={() => void onProcessAction('restart')}
            >
              {WEIXIN.processRestart}
            </button>
            <button
              type="button"
              className="btn ghost"
              disabled={uiBusy}
              onClick={() => setConfirmStop(true)}
            >
              {WEIXIN.processStop}
            </button>
          </div>
          <p className="settings-muted">{WEIXIN.processHint}</p>
        </section>
      )}

      <section className="weixin-login-block">
        <h2 className="settings-subheading">{WEIXIN.loginSection}</h2>
        <div className="weixin-login-actions">
          <button type="button" className="btn primary" disabled={uiBusy} onClick={() => void onStartLogin()}>
            {qrUrl ? WEIXIN.refreshQr : WEIXIN.getQr}
          </button>
          {isAdmin && (
            <button type="button" className="btn ghost" disabled={uiBusy} onClick={() => setConfirmLogout(true)}>
              {WEIXIN.logout}
            </button>
          )}
        </div>
        {qrUrl && (
          <div className="weixin-qr">
            {qrImgSrc ? (
              <img src={qrImgSrc} alt={WEIXIN.qrAlt} className="weixin-qr-img" />
            ) : (
              <p className="settings-muted">{WEIXIN.qrGenerating}</p>
            )}
            <p className="settings-muted weixin-qr-url">{WEIXIN.qrScanHint(qrUrl)}</p>
            {ticket && <p className="settings-muted">ticket: {ticket}</p>}
            {loginStatus && (
              <p className="settings-muted">{loginStatusLabel(loginStatus)}</p>
            )}
          </div>
        )}
      </section>

      {!loading && isAdmin && (
        <form className="settings-form" onSubmit={(e) => void onSubmit(e)}>
          <h2 className="settings-subheading">{WEIXIN.settingsSection}</h2>
          <label className="settings-field">
            <span className="settings-field-label">agent_id</span>
            <input
              className="settings-input"
              value={agentId}
              onChange={(e) => setAgentId(e.target.value)}
              disabled={uiBusy}
              placeholder="ticket-agent"
            />
          </label>
          <label className="settings-field">
            <span className="settings-field-label">{WEIXIN.fieldAssignee}</span>
            <input
              className="settings-input"
              value={assignee}
              onChange={(e) => setAssignee(e.target.value)}
              disabled={uiBusy}
              placeholder="alice 或 channel:weixin"
            />
          </label>
          <label className="settings-field">
            <span className="settings-field-label">{WEIXIN.fieldAllowlist}</span>
            <textarea
              className="settings-textarea"
              rows={5}
              value={allowlistText}
              onChange={(e) => setAllowlistText(e.target.value)}
              disabled={uiBusy}
              placeholder="peer_id_1&#10;peer_id_2"
            />
          </label>
          <label className="settings-checkbox">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
              disabled={uiBusy}
            />
            {WEIXIN.enableChannel}
          </label>
          <button type="submit" className="btn primary" disabled={uiBusy}>
            {busy ? WEIXIN.saving : WEIXIN.saveSettings}
          </button>
        </form>
      )}

      {!loading && isAdmin && (
        <section className="settings-webhook-deliveries">
          <h2 className="settings-subheading">{WEIXIN.outboundTitle}</h2>
          <p className="settings-meta">{WEIXIN.outboundHint}</p>
          {deliveries.length === 0 ? (
            <p className="settings-muted">{WEIXIN.outboundEmpty}</p>
          ) : (
            <ul className="settings-list">
              {deliveries.map((d) => (
                <li key={d.id} className="settings-list-item">
                  <div className="settings-tool-line">
                    <Badge tone={outboundBadgeTone(d.status)}>{formatOutboundStatus(d.status)}</Badge>
                    <span>{outboundSummary(d)}</span>
                  </div>
                  {(d.status === 'dead' || d.status === 'pending') && (
                    <Button
                      type="button"
                      variant="ghost"
                      disabled={uiBusy}
                      onClick={() => void onRetry(d.id)}
                    >
                      {retryingId === d.id ? WEIXIN.outboundRetrying : WEIXIN.outboundRetry}
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          )}
        </section>
      )}

      <ConfirmDialog
        open={confirmLogout}
        danger
        title={WEIXIN.confirmLogoutTitle}
        body={WEIXIN.confirmLogoutBody}
        confirmText={WEIXIN.confirmLogoutOk}
        busy={uiBusy}
        onCancel={() => setConfirmLogout(false)}
        onConfirm={() => void onLogout()}
      />

      <ConfirmDialog
        open={confirmStop}
        danger
        title={WEIXIN.confirmStopTitle}
        body={WEIXIN.confirmStopBody}
        confirmText={WEIXIN.confirmStopOk}
        busy={uiBusy}
        onCancel={() => setConfirmStop(false)}
        onConfirm={() => void onProcessAction('stop')}
      />
    </div>
  )
}
