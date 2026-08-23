import { useCallback, useEffect, useState } from 'react'
import {
  clearIdentities,
  createIdentity,
  deleteIdentity,
  listIdentities,
  setDefaultIdentity,
  type IdentityView,
} from '../api'
import { redactSensitive } from '../sensitive'

const CONV_KEY = 'baize.conversation_id'

function loadConversationId(): string {
  const existing = localStorage.getItem(CONV_KEY)?.trim()
  if (existing) return existing
  const id = `conv_${crypto.randomUUID()}`
  localStorage.setItem(CONV_KEY, id)
  return id
}

function sourceLabel(source: string): string {
  switch (source) {
    case 'login_capture':
      return '登录捕获'
    case 'env':
      return '环境变量'
    case 'manual':
      return '手动'
    default:
      return source
  }
}

function formatClaims(claims: Record<string, unknown> | undefined): string | null {
  if (!claims || Object.keys(claims).length === 0) return null
  try {
    return JSON.stringify(redactSensitive(claims), null, 2)
  } catch {
    return null
  }
}

export function IdentitiesSettings() {
  const [conversationId] = useState(loadConversationId)
  const [identities, setIdentities] = useState<IdentityView[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState('')
  const [busy, setBusy] = useState(false)
  const [pasteToken, setPasteToken] = useState('')

  const refresh = useCallback(async () => {
    try {
      const list = await listIdentities(conversationId)
      setIdentities(list)
      setError(null)
    } catch (err) {
      setIdentities(null)
      setError(err instanceof Error ? err.message : String(err))
    }
  }, [conversationId])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const runAction = async (fn: () => Promise<void>) => {
    setBusy(true)
    setStatus('')
    try {
      await fn()
      await refresh()
    } catch (err) {
      setStatus(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const hasCaptured = (identities ?? []).some((i) => i.source !== 'env')

  return (
    <div className="settings-section">
      <h1 className="settings-heading">账号</h1>
      <p className="settings-meta">会话 {conversationId}</p>
      <label className="settings-field">
        <span className="settings-field-label">粘贴用户 Token（跳过登录接口）</span>
        <input
          className="settings-input"
          type="password"
          value={pasteToken}
          onChange={(e) => setPasteToken(e.target.value)}
          disabled={busy}
          placeholder="Bearer eyJ… 或 accessToken"
          autoComplete="off"
        />
        <button
          type="button"
          className="btn primary sm"
          disabled={busy || !pasteToken.trim()}
          onClick={() =>
            void runAction(async () => {
              await createIdentity(conversationId, pasteToken.trim())
              setPasteToken('')
              setStatus('已保存临时 Token')
            })
          }
        >
          保存 Token
        </button>
      </label>
      {status && <p className="settings-error">{status}</p>}
      {error && <p className="settings-error">无法加载账号：{error}</p>}
      {!error && identities === null && <p className="settings-muted">加载中…</p>}
      {!error && identities !== null && identities.length === 0 && (
        <p className="settings-empty">暂无已登录账号</p>
      )}
      {!error && identities !== null && identities.length > 0 && (
        <ul className="settings-list accounts-list">
          {identities.map((idt) => {
            const claimsText = formatClaims(idt.claims_summary)
            return (
              <li key={idt.id} className="accounts-item">
                <div className="accounts-main">
                  <div className="accounts-title">{idt.label || idt.id}</div>
                  <div className="accounts-detail">
                    {idt.scheme ? idt.scheme : '—'} · {sourceLabel(idt.source)}
                  </div>
                  {claimsText && (
                    <pre className="accounts-claims">{claimsText}</pre>
                  )}
                </div>
                <div className="accounts-actions">
                  {idt.is_default ? (
                    <span className="settings-badge accounts-default">默认</span>
                  ) : (
                    <button
                      type="button"
                      className="btn ghost sm"
                      disabled={busy}
                      onClick={() =>
                        void runAction(() => setDefaultIdentity(conversationId, idt.id))
                      }
                    >
                      设为默认
                    </button>
                  )}
                  {idt.source !== 'env' && (
                    <button
                      type="button"
                      className="btn ghost sm danger-text"
                      disabled={busy}
                      onClick={() =>
                        void runAction(() => deleteIdentity(conversationId, idt.id))
                      }
                    >
                      退出
                    </button>
                  )}
                </div>
              </li>
            )
          })}
        </ul>
      )}
      {hasCaptured && (
        <button
          type="button"
          className="btn ghost sm accounts-clear"
          disabled={busy}
          onClick={() => void runAction(() => clearIdentities(conversationId))}
        >
          清空捕获账号
        </button>
      )}
    </div>
  )
}