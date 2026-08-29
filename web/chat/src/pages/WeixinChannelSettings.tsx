import { type FormEvent, useCallback, useEffect, useRef, useState } from 'react'
import {
  getWeixinLoginStatus,
  getWeixinSettings,
  logoutWeixin,
  putWeixinSettings,
  startWeixinLogin,
  type WeixinChannelSettings,
} from '../api'
import { qrDataUrlFromText } from '../qrDataUrl'

const POLL_MS = 2000

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
      return '等待扫码…'
    case 'success':
      return '登录成功'
    case 'expired':
      return '二维码已过期，请重新获取'
    default:
      return status || '未知状态'
  }
}

function apiErrorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

export function WeixinChannelSettings() {
  const [agentId, setAgentId] = useState('')
  const [assignee, setAssignee] = useState('')
  const [allowlistText, setAllowlistText] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)

  const [ticket, setTicket] = useState<string | null>(null)
  const [qrUrl, setQrUrl] = useState<string | null>(null)
  /** PNG data URL rendered from qrUrl (liteapp link is not an image). */
  const [qrImgSrc, setQrImgSrc] = useState<string | null>(null)
  const [loginStatus, setLoginStatus] = useState<string | null>(null)
  const pollRef = useRef<number | null>(null)

  const stopPoll = useCallback(() => {
    if (pollRef.current !== null) {
      window.clearInterval(pollRef.current)
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
          setError(apiErrorMessage(err))
        }
      })
    return () => {
      cancelled = true
    }
  }, [qrUrl])

  const applySettings = useCallback((s: WeixinChannelSettings) => {
    setAgentId(s.agent_id ?? '')
    setAssignee(s.assignee ?? '')
    setAllowlistText(formatAllowlistText(s.allowlist))
    setEnabled(Boolean(s.enabled))
  }, [])

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const s = await getWeixinSettings()
      applySettings(s)
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [applySettings])

  useEffect(() => {
    void load()
  }, [load])

  const startPolling = useCallback(
    (loginTicket: string) => {
      stopPoll()
      const tick = async () => {
        try {
          const res = await getWeixinLoginStatus(loginTicket)
          setLoginStatus(res.status)
          if (res.status === 'success' || res.status === 'expired') {
            stopPoll()
            if (res.status === 'success') {
              setStatus('微信渠道已登录')
              setTicket(null)
              setQrUrl(null)
            }
          }
        } catch (err) {
          stopPoll()
          setError(apiErrorMessage(err))
        }
      }
      void tick()
      pollRef.current = window.setInterval(() => {
        void tick()
      }, POLL_MS)
    },
    [stopPoll],
  )

  const onStartLogin = async () => {
    setBusy(true)
    setError(null)
    setStatus(null)
    setLoginStatus(null)
    stopPoll()
    try {
      const res = await startWeixinLogin()
      setTicket(res.ticket)
      setQrUrl(res.qr_url)
      setLoginStatus('pending')
      startPolling(res.ticket)
    } catch (err) {
      setError(apiErrorMessage(err))
      setTicket(null)
      setQrUrl(null)
    } finally {
      setBusy(false)
    }
  }

  const onLogout = async () => {
    setBusy(true)
    setError(null)
    setStatus(null)
    stopPoll()
    setTicket(null)
    setQrUrl(null)
    setLoginStatus(null)
    try {
      await logoutWeixin()
      setStatus('已登出微信渠道')
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      const saved = await putWeixinSettings({
        agent_id: agentId.trim(),
        assignee: assignee.trim(),
        allowlist: parseAllowlistText(allowlistText),
        enabled,
      })
      applySettings(saved)
      setStatus('渠道设置已保存')
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="settings-section">
      <h1 className="settings-heading">渠道 · 微信</h1>
      <div className="settings-meta">
        <p>扫码登录微信个人号 Bot（iLink），配置默认 Agent、受理人与私信 allowlist。</p>
        <p>仅管理员可操作。出站双向同步见后续任务。</p>
      </div>

      {loading && <p className="settings-muted">加载中…</p>}
      {error && <p className="settings-error">{error}</p>}
      {status && <p className="settings-muted">{status}</p>}

      <section className="weixin-login-block">
        <h2 className="settings-subheading">登录</h2>
        <div className="weixin-login-actions">
          <button type="button" className="btn primary" disabled={busy} onClick={() => void onStartLogin()}>
            {qrUrl ? '刷新二维码' : '获取登录二维码'}
          </button>
          <button type="button" className="btn ghost" disabled={busy} onClick={() => void onLogout()}>
            登出
          </button>
        </div>
        {qrUrl && (
          <div className="weixin-qr">
            {qrImgSrc ? (
              <img src={qrImgSrc} alt="微信登录二维码" className="weixin-qr-img" />
            ) : (
              <p className="settings-muted">正在生成二维码…</p>
            )}
            <p className="settings-muted weixin-qr-url">请用手机微信扫码（内容：{qrUrl}）</p>
            {ticket && <p className="settings-muted">ticket: {ticket}</p>}
            {loginStatus && (
              <p className="settings-muted">{loginStatusLabel(loginStatus)}</p>
            )}
          </div>
        )}
      </section>

      {!loading && (
        <form className="settings-form" onSubmit={(e) => void onSubmit(e)}>
          <h2 className="settings-subheading">设置</h2>
          <label className="settings-field">
            <span className="settings-field-label">agent_id</span>
            <input
              className="settings-input"
              value={agentId}
              onChange={(e) => setAgentId(e.target.value)}
              disabled={busy}
              placeholder="ticket-agent"
            />
          </label>
          <label className="settings-field">
            <span className="settings-field-label">受理人（assignee）</span>
            <input
              className="settings-input"
              value={assignee}
              onChange={(e) => setAssignee(e.target.value)}
              disabled={busy}
              placeholder="alice 或 channel:weixin"
            />
          </label>
          <label className="settings-field">
            <span className="settings-field-label">allowlist（每行一个 peer id）</span>
            <textarea
              className="settings-textarea"
              rows={5}
              value={allowlistText}
              onChange={(e) => setAllowlistText(e.target.value)}
              disabled={busy}
              placeholder="peer_id_1&#10;peer_id_2"
            />
          </label>
          <label className="settings-checkbox">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
              disabled={busy}
            />
            启用微信渠道
          </label>
          <button type="submit" className="btn primary" disabled={busy}>
            {busy ? '保存中…' : '保存设置'}
          </button>
        </form>
      )}
    </div>
  )
}
