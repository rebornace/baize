import { type FormEvent, useCallback, useEffect, useState } from 'react'
import {
  getEventsWebhook,
  putEventsWebhook,
  testEventsWebhook,
  type EventsWebhookConfig,
} from '../api'
import { formatKeyValueMap, parseKeyValueLines } from './McpSettings'

export interface WebhookFormState {
  url: string
  headersText: string
}

const EMPTY_FORM: WebhookFormState = {
  url: '',
  headersText: '',
}

export function configToForm(cfg: EventsWebhookConfig): WebhookFormState {
  return {
    url: cfg.url ?? '',
    headersText: formatKeyValueMap(cfg.headers),
  }
}

export function validateWebhookForm(
  form: WebhookFormState,
): { ok: true; config: EventsWebhookConfig } | { ok: false; message: string } {
  const headersParsed = parseKeyValueLines(form.headersText)
  if (!headersParsed.ok) {
    return { ok: false, message: headersParsed.message }
  }
  return {
    ok: true,
    config: {
      url: form.url.trim(),
      headers: headersParsed.value,
    },
  }
}

function apiErrorMessage(err: unknown): string {
  if (err instanceof Error) return err.message
  return String(err)
}

export function WebhookSettings() {
  const [form, setForm] = useState<WebhookFormState>(EMPTY_FORM)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [testing, setTesting] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const cfg = await getEventsWebhook()
      setForm(configToForm(cfg))
      setError(null)
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    const validated = validateWebhookForm(form)
    if (!validated.ok) {
      setError(validated.message)
      return
    }
    setSubmitting(true)
    setError(null)
    setStatus(null)
    setTestResult(null)
    try {
      await putEventsWebhook(validated.config)
      setForm(configToForm(validated.config))
      setStatus('已保存 Webhook 配置')
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  const onTest = async () => {
    setTesting(true)
    setError(null)
    setTestResult(null)
    setStatus(null)
    try {
      const result = await testEventsWebhook()
      setTestResult(`测试投递成功（${result.status}）`)
    } catch (err) {
      setTestResult(`测试投递失败：${apiErrorMessage(err)}`)
    } finally {
      setTesting(false)
    }
  }

  const busy = submitting || testing

  return (
    <div className="settings-section settings-webhook">
      <h1 className="settings-heading">Webhook</h1>
      <p className="settings-meta">
        配置全局 Run 事件 Webhook。每条轨迹事件与终态 <code>run.ended</code> 会异步 POST
        到目标 URL；与 SSE 流并列，不阻塞引擎。
      </p>
      {loading && <p className="settings-muted">加载中…</p>}
      {!loading && error && <p className="settings-error">{error}</p>}
      {!loading && status && <p className="settings-muted">{status}</p>}
      {!loading && testResult && (
        <p className={testResult.startsWith('测试投递失败') ? 'settings-error' : 'settings-muted'}>
          {testResult}
        </p>
      )}
      {!loading && (
        <form className="settings-form" onSubmit={onSubmit}>
          <label className="settings-field">
            <span className="settings-field-label">Webhook URL</span>
            <input
              className="settings-input"
              value={form.url}
              onChange={(e) => setForm((f) => ({ ...f, url: e.target.value }))}
              disabled={busy}
              placeholder="https://example.com/hooks/baize"
            />
          </label>
          <label className="settings-field">
            <span className="settings-field-label">headers（每行 KEY=VALUE）</span>
            <textarea
              className="settings-textarea"
              value={form.headersText}
              onChange={(e) => setForm((f) => ({ ...f, headersText: e.target.value }))}
              disabled={busy}
              rows={4}
              placeholder="Authorization=Bearer ${API_TOKEN}"
            />
          </label>
          <div className="settings-toolbar">
            <button type="submit" className="btn primary" disabled={busy}>
              {submitting ? '保存中…' : '保存'}
            </button>
            <button type="button" className="btn ghost" disabled={busy} onClick={() => void onTest()}>
              {testing ? '测试中…' : '发送测试事件'}
            </button>
          </div>
        </form>
      )}
    </div>
  )
}
