import type { ConnectorAuth } from '../api'
import { mergeAuthWithCapture, type CaptureDraft } from './captureForm'

export interface LoginSignalsDraft {
  httpStatusesText: string
  httpStatusesNeedSignalText: string
  bodyCodePath: string
  bodyCodesText: string
  messageContainsText: string
}

export const EMPTY_LOGIN_SIGNALS_DRAFT: LoginSignalsDraft = {
  httpStatusesText: '',
  httpStatusesNeedSignalText: '',
  bodyCodePath: '',
  bodyCodesText: '',
  messageContainsText: '',
}

export function formatIntLines(values: number[] | undefined): string {
  if (!values || values.length === 0) return ''
  return values.join('\n')
}

export function formatStringLines(values: string[] | undefined): string {
  if (!values || values.length === 0) return ''
  return values.join('\n')
}

export function parseIntLines(text: string): number[] {
  const out: number[] = []
  const seen = new Set<number>()
  for (const raw of text.split(/[\n,]+/)) {
    const s = raw.trim()
    if (s === '') continue
    const n = Number(s)
    if (!Number.isInteger(n) || n < 100 || n > 599) continue
    if (seen.has(n)) continue
    seen.add(n)
    out.push(n)
  }
  return out
}

export function parseStringLines(text: string): string[] {
  return text
    .split('\n')
    .map((s) => s.trim())
    .filter((s) => s !== '')
}

export function loginSignalsToDraft(
  signals: ConnectorAuth['login_signals'] | undefined,
): LoginSignalsDraft {
  if (!signals) return { ...EMPTY_LOGIN_SIGNALS_DRAFT }
  return {
    httpStatusesText: formatIntLines(signals.http_statuses),
    httpStatusesNeedSignalText: formatIntLines(signals.http_statuses_need_signal),
    bodyCodePath: signals.body_code_path ?? '',
    bodyCodesText: formatStringLines(signals.body_codes),
    messageContainsText: formatStringLines(signals.message_contains),
  }
}

const AMBIGUOUS_HTTP_STATUSES = new Set([400, 404])

export function buildLoginSignalsFromDraft(
  draft: LoginSignalsDraft,
): NonNullable<ConnectorAuth['login_signals']> {
  const signals: NonNullable<ConnectorAuth['login_signals']> = {}
  // 400/404 are usually ordinary business errors; keep them out of "alone".
  const alone: number[] = []
  const need = new Set(parseIntLines(draft.httpStatusesNeedSignalText))
  for (const status of parseIntLines(draft.httpStatusesText)) {
    if (AMBIGUOUS_HTTP_STATUSES.has(status)) {
      need.add(status)
      continue
    }
    alone.push(status)
  }
  const needSignal = [...need]
  const bodyPath = draft.bodyCodePath.trim()
  const bodyCodes = parseStringLines(draft.bodyCodesText)
  const messages = parseStringLines(draft.messageContainsText)
  if (alone.length > 0) signals.http_statuses = alone
  if (needSignal.length > 0) signals.http_statuses_need_signal = needSignal
  if (bodyPath !== '') signals.body_code_path = bodyPath
  if (bodyCodes.length > 0) signals.body_codes = bodyCodes
  if (messages.length > 0) signals.message_contains = messages
  return signals
}

/** Merge capture + login_signals drafts into existing auth without changing mode / static / etc. */
export function mergeAuthWithAdvanced(
  base: ConnectorAuth | undefined,
  capture: CaptureDraft,
  loginSignals: LoginSignalsDraft,
): ConnectorAuth {
  const auth = mergeAuthWithCapture(base, capture)
  auth.login_signals = buildLoginSignalsFromDraft(loginSignals)
  return auth
}
