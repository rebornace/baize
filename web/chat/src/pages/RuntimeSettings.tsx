import { type FormEvent, useCallback, useEffect, useState } from 'react'
import {
  getCredentials,
  getRuntimeSettings,
  patchCredentials,
  patchRuntimeSettings,
  type CredentialsView,
  type RuntimeKnobsView,
} from '../api'
import { useGate } from '../gateContext'
import {
  buildKnobsPatch,
  knobsToForm,
  KNOB_FIELDS,
  validateKnobField,
  type KnobsForm,
} from './runtimeSettingsHelpers'

function apiErrorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

function CredentialsSection() {
  const { role } = useGate()
  const [view, setView] = useState<CredentialsView | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)
  const [operatorToken, setOperatorToken] = useState('')
  const [adminToken, setAdminToken] = useState('')
  const [newOpId, setNewOpId] = useState('')
  const [newOpToken, setNewOpToken] = useState('')

  const load = useCallback(async () => {
    if (role !== 'admin') {
      setLoading(false)
      return
    }
    setLoading(true)
    setError(null)
    try {
      setView(await getCredentials())
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [role])

  useEffect(() => {
    void load()
  }, [load])

  const apply = useCallback((v: CredentialsView) => {
    setView(v)
    setOperatorToken('')
    setAdminToken('')
    setNewOpId('')
    setNewOpToken('')
  }, [])

  const rotateTokens = async (e: FormEvent) => {
    e.preventDefault()
    if (!operatorToken && !adminToken) {
      setError('请至少填写一个要轮换的口令')
      return
    }
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      const v = await patchCredentials({
        operator_token: operatorToken || undefined,
        admin_token: adminToken || undefined,
      })
      apply(v)
      setStatus('口令已轮换；若改的是当前登录口令，请用新口令重新解锁。')
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const addOperator = async (e: FormEvent) => {
    e.preventDefault()
    const id = newOpId.trim()
    if (!id || !newOpToken) {
      setError('新增 operator 需要 id 与 token')
      return
    }
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      apply(await patchCredentials({ add_operators: [{ id, token: newOpToken }] }))
      setStatus(`已新增 operator：${id}`)
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const removeOperator = async (id: string) => {
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      apply(await patchCredentials({ remove_operators: [id] }))
      setStatus(`已移除 operator：${id}`)
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const resetAll = async () => {
    if (!window.confirm('确定清空全部热更新凭据，回落到 YAML/env 基线口令？')) return
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      apply(await patchCredentials({ reset: true }))
      setStatus('已重置：凭据回落至配置基线（引擎参数不受影响）。')
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  // 凭据（含 operator 列表）仅 admin 可见；hooks 必须保持无条件执行，故早退放在全部 hooks 之后。
  if (role !== 'admin') return null

  return (
    <section className="settings-form">
      <h2 className="settings-subheading">控制面凭据</h2>
      {loading && <p className="settings-muted">加载中…</p>}
      {error && <p className="settings-error">{error}</p>}
      {status && <p className="settings-muted">{status}</p>}
      {view && (
        <>
          <p className="settings-muted">
            来源：{view.source === 'override' ? '热更新覆盖' : '配置基线（config）'} ·
            operator 口令 {view.operator_set ? '已设置' : '未设置'} ·
            admin 口令 {view.admin_set ? '已设置' : '未设置'}
          </p>

          <div className="settings-meta">
            <p>主口令轮换（留空表示不修改；明文经 HTTPS 提交，GET 永不返回口令）。</p>
          </div>
          <form onSubmit={(e) => void rotateTokens(e)}>
            <label className="settings-field">
              <span className="settings-field-label">operator token（留空不修改）</span>
              <input
                className="settings-input"
                type="password"
                autoComplete="off"
                value={operatorToken}
                onChange={(e) => setOperatorToken(e.target.value)}
                disabled={busy}
                placeholder="••••••"
              />
            </label>
            <label className="settings-field">
              <span className="settings-field-label">admin token（留空不修改）</span>
              <input
                className="settings-input"
                type="password"
                autoComplete="off"
                value={adminToken}
                onChange={(e) => setAdminToken(e.target.value)}
                disabled={busy}
                placeholder="••••••"
              />
            </label>
            <button type="submit" className="btn primary" disabled={busy}>
              {busy ? '提交中…' : '轮换主口令'}
            </button>
          </form>

          <h3 className="settings-subheading" style={{ marginTop: '1.25rem' }}>命名 operator</h3>
          {view.operators.length === 0 ? (
            <p className="settings-muted">暂无 operator。</p>
          ) : (
            <ul className="cred-operator-list">
              {view.operators.map((op) => (
                <li key={op.id} className="cred-operator-item">
                  <span className="cred-operator-id">{op.id}</span>
                  <span className="settings-badge">{op.source === 'runtime' ? 'runtime' : 'config'}</span>
                  {op.source === 'runtime' && (
                    <button
                      type="button"
                      className="btn ghost"
                      disabled={busy}
                      onClick={() => void removeOperator(op.id)}
                    >
                      移除
                    </button>
                  )}
                </li>
              ))}
            </ul>
          )}
          <form onSubmit={(e) => void addOperator(e)}>
            <label className="settings-field">
              <span className="settings-field-label">新增 operator id</span>
              <input
                className="settings-input"
                value={newOpId}
                onChange={(e) => setNewOpId(e.target.value)}
                disabled={busy}
                placeholder="bob"
              />
            </label>
            <label className="settings-field">
              <span className="settings-field-label">token</span>
              <input
                className="settings-input"
                type="password"
                autoComplete="off"
                value={newOpToken}
                onChange={(e) => setNewOpToken(e.target.value)}
                disabled={busy}
                placeholder="••••••"
              />
            </label>
            <button type="submit" className="btn primary" disabled={busy}>
              {busy ? '提交中…' : '新增 operator'}
            </button>
          </form>

          <div style={{ marginTop: '1.25rem' }}>
            <button type="button" className="btn danger" disabled={busy} onClick={() => void resetAll()}>
              重置为基线口令（break-glass）
            </button>
          </div>
        </>
      )}
    </section>
  )
}

export function RuntimeSettings() {
  const { role } = useGate()
  const readOnly = role !== 'admin'
  const [knobView, setKnobView] = useState<RuntimeKnobsView | null>(null)
  const [form, setForm] = useState<KnobsForm | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const v = await getRuntimeSettings()
      setKnobView(v)
      setForm(knobsToForm(v.effective))
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const setField = (key: keyof KnobsForm, value: string | boolean) => {
    setForm((f) => (f ? { ...f, [key]: value } : f))
  }

  const onSubmitKnobs = async (e: FormEvent) => {
    e.preventDefault()
    if (!knobView || !form) return
    for (const spec of KNOB_FIELDS) {
      const msg = validateKnobField(spec, form[spec.key])
      if (msg) {
        setError(msg)
        return
      }
    }
    const patch = buildKnobsPatch(form, knobView.effective)
    if (Object.keys(patch).length === 0) {
      setStatus('没有改动')
      return
    }
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      const v = await patchRuntimeSettings(patch)
      setKnobView(v)
      setForm(knobsToForm(v.effective))
      setStatus('引擎参数已热更新，下一次运行立即生效（无需重启）。')
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="settings-section">
      <h1 className="settings-heading">运行时设置</h1>
      <div className="settings-meta">
        {readOnly ? (
          <p>引擎参数保存后立即生效、跨重启保留、多副本约 20 秒内同步（仅管理员可修改）。</p>
        ) : (
          <p>引擎参数与控制面凭据可在线热更新：保存后立即生效、跨重启保留、多副本约 20 秒内同步。</p>
        )}
      </div>

      {loading && <p className="settings-muted">加载中…</p>}
      {error && <p className="settings-error">{error}</p>}
      {status && <p className="settings-muted">{status}</p>}

      {!loading && knobView && form && (
        <form className="settings-form" onSubmit={(e) => void onSubmitKnobs(e)}>
          <h2 className="settings-subheading">引擎参数</h2>
          {KNOB_FIELDS.map((spec) => {
            const overridden = knobView.overridden[spec.key]
            return (
              <label className="settings-field" key={spec.key}>
                <span className="settings-field-label">
                  {spec.label}
                  {overridden && <span className="settings-badge">已覆盖基线</span>}
                </span>
                <input
                  className="settings-input"
                  type="number"
                  step={spec.integer ? 1 : 'any'}
                  min={spec.min}
                  max={spec.max}
                  value={form[spec.key]}
                  onChange={(e) => setField(spec.key, e.target.value)}
                  disabled={busy || readOnly}
                />
                <span className="settings-field-hint">{spec.hint}</span>
              </label>
            )
          })}
          <label className="settings-checkbox">
            <input
              type="checkbox"
              checked={form.compaction_enabled}
              onChange={(e) => setField('compaction_enabled', e.target.checked)}
              disabled={busy || readOnly}
            />
            启用上下文压缩
            {knobView.overridden.compaction_enabled && (
              <span className="settings-badge">已覆盖基线</span>
            )}
          </label>
          {!readOnly && (
            <button type="submit" className="btn primary" disabled={busy}>
              {busy ? '保存中…' : '保存引擎参数'}
            </button>
          )}
        </form>
      )}

      <CredentialsSection />
    </div>
  )
}
