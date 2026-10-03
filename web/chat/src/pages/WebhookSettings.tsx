import { type FormEvent, useCallback, useEffect, useState } from 'react'
import {
  getEventsWebhook,
  getEventsWebhookDeliveries,
  putEventsWebhook,
  retryEventsWebhookDelivery,
  testEventsWebhook,
  type EventsWebhookConfig,
  type EventsWebhookDelivery,
} from '../api'
import {
  Badge,
  Button,
  Field,
  Input,
  PageHeader,
  Textarea,
  ToastRegion,
  useToast,
  type BadgeTone,
} from '../components/ui'
import { WEBHOOKS, friendlyError } from '../strings'
import { formatKeyValueMap, parseKeyValueLines } from './connectorForms/lines'

export interface WebhookFormState {
  url: string
  headersText: string
}

const EMPTY_FORM: WebhookFormState = {
  url: '',
  headersText: '',
}

export function formatDeliveryStatus(status: string): string {
  switch (status) {
    case 'dead':
      return WEBHOOKS.statusDead
    case 'pending':
      return WEBHOOKS.statusPending
    case 'delivered':
      return WEBHOOKS.statusDelivered
    default:
      return status
  }
}

function deliveryBadgeTone(status: string): BadgeTone {
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

export function deliverySummary(d: EventsWebhookDelivery): string {
  const kind = d.kind === 'ended' ? 'run.ended' : `event#${d.event_index}`
  const err = d.last_error ? ` · ${d.last_error}` : ''
  return `${d.run_id} · ${kind} · ${d.attempt}/${d.max_attempts}${err}`
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
    return { ok: false, message: WEBHOOKS.errBadHeaderLine }
  }
  return {
    ok: true,
    config: {
      url: form.url.trim(),
      headers: headersParsed.value,
    },
  }
}

export function WebhookSettings() {
  const [form, setForm] = useState<WebhookFormState>(EMPTY_FORM)
  const [deliveries, setDeliveries] = useState<EventsWebhookDelivery[]>([])
  const [loading, setLoading] = useState(true)
  const [headersError, setHeadersError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [testing, setTesting] = useState(false)
  const [retryingId, setRetryingId] = useState<string | null>(null)
  const { toasts, push, dismiss } = useToast()

  const loadDeliveries = useCallback(async () => {
    try {
      const rows = await getEventsWebhookDeliveries()
      setDeliveries(rows)
    } catch {
      setDeliveries([])
    }
  }, [])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const cfg = await getEventsWebhook()
      setForm(configToForm(cfg))
      setHeadersError(null)
      await loadDeliveries()
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: WEBHOOKS.loadFailed, detail: f.detail ?? f.title })
    } finally {
      setLoading(false)
    }
  }, [loadDeliveries, push])

  useEffect(() => {
    void load()
  }, [load])

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    const validated = validateWebhookForm(form)
    if (!validated.ok) {
      setHeadersError(validated.message)
      return
    }
    setSubmitting(true)
    setHeadersError(null)
    try {
      await putEventsWebhook(validated.config)
      setForm(configToForm(validated.config))
      push({ tone: 'success', title: WEBHOOKS.toastSaved })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setSubmitting(false)
    }
  }

  const onTest = async () => {
    setTesting(true)
    try {
      const result = await testEventsWebhook()
      push({
        tone: 'success',
        title: WEBHOOKS.toastTestOk,
        detail: String(result.status),
      })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: WEBHOOKS.toastTestFail, detail: f.detail ?? f.title })
    } finally {
      setTesting(false)
    }
  }

  const onRetry = async (id: string) => {
    setRetryingId(id)
    try {
      await retryEventsWebhookDelivery(id)
      push({ tone: 'success', title: WEBHOOKS.toastRetryQueued })
      await loadDeliveries()
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setRetryingId(null)
    }
  }

  const busy = submitting || testing || retryingId !== null

  return (
    <div className="settings-panel settings-webhook">
      <PageHeader title={WEBHOOKS.title} description={WEBHOOKS.description} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />

      {loading && <p className="settings-muted">加载中…</p>}
      {!loading && (
        <form className="settings-form" onSubmit={(e) => void onSubmit(e)}>
          <Field label={WEBHOOKS.urlLabel} hint={WEBHOOKS.urlHint}>
            <Input
              value={form.url}
              onChange={(e) => setForm((f) => ({ ...f, url: e.target.value }))}
              disabled={busy}
              placeholder="https://example.com/hooks/baize"
            />
          </Field>
          <Field
            label={WEBHOOKS.headersLabel}
            hint={WEBHOOKS.headersHint}
            error={headersError ?? undefined}
          >
            <Textarea
              value={form.headersText}
              onChange={(e) => {
                setForm((f) => ({ ...f, headersText: e.target.value }))
                setHeadersError(null)
              }}
              disabled={busy}
              rows={4}
              placeholder="Authorization=Bearer ${API_TOKEN}"
            />
          </Field>
          <div className="settings-toolbar">
            <Button type="submit" variant="primary" disabled={busy}>
              {submitting ? WEBHOOKS.saving : WEBHOOKS.save}
            </Button>
            <Button type="button" variant="ghost" disabled={busy} onClick={() => void onTest()}>
              {testing ? WEBHOOKS.testing : WEBHOOKS.test}
            </Button>
          </div>
        </form>
      )}
      {!loading && (
        <section className="settings-webhook-deliveries">
          <h2 className="settings-subheading">{WEBHOOKS.deliveriesTitle}</h2>
          <p className="settings-meta">{WEBHOOKS.deliveriesHint}</p>
          {deliveries.length === 0 ? (
            <p className="settings-muted">{WEBHOOKS.deliveriesEmpty}</p>
          ) : (
            <ul className="settings-list">
              {deliveries.map((d) => (
                <li key={d.id} className="settings-list-item">
                  <div className="settings-tool-line">
                    <Badge tone={deliveryBadgeTone(d.status)}>{formatDeliveryStatus(d.status)}</Badge>
                    <span>{deliverySummary(d)}</span>
                  </div>
                  {(d.status === 'dead' || d.status === 'pending') && (
                    <Button
                      type="button"
                      variant="ghost"
                      disabled={busy}
                      onClick={() => void onRetry(d.id)}
                    >
                      {retryingId === d.id ? WEBHOOKS.retrying : WEBHOOKS.retry}
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          )}
        </section>
      )}
    </div>
  )
}
