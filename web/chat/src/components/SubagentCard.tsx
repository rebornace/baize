import { useCallback, useEffect, useRef, useState } from 'react'
import { Bot, ChevronDown, RefreshCw } from 'lucide-react'
import {
  cancelRun,
  getRun,
  getRunBinding,
  listEvents,
  resumeRun,
  type Event,
} from '../api'
import type { ChatBlock } from '../foldEvents'
import { SUBAGENT } from '../strings'
import { Button } from './ui'

type SubagentBlock = Extract<ChatBlock, { kind: 'subagent' }>

export interface SubagentCardProps {
  block: SubagentBlock
  onBrowserLogin?: (connectorId: string) => void
  onError?: (e: unknown) => void
  /** Prefills the composer with a receipt-based continuation prompt. */
  onContinue?: (text: string) => void
  /** A live run is active or waiting for a human. */
  continueDisabled?: boolean
}

type WaitingInfo = {
  kind: 'approval' | 'login'
  toolName: string
  prompt: string
  connectorId: string
  args?: unknown
}

function statusLabel(
  status: SubagentBlock['status'],
  waitingKind?: 'approval' | 'login',
): string {
  switch (status) {
    case 'waiting':
      return waitingKind === 'login' ? SUBAGENT.waitingLogin : SUBAGENT.waitingApproval
    case 'running':
      return SUBAGENT.running
    case 'succeeded':
      return SUBAGENT.succeeded
    case 'failed':
      return SUBAGENT.failed
    case 'cancelled':
      return SUBAGENT.cancelled
    case 'rejected':
      return SUBAGENT.rejected
    default:
      return SUBAGENT.running
  }
}

const terminalSubagentStatuses = new Set<SubagentBlock['status']>([
  'succeeded',
  'failed',
  'cancelled',
  'rejected',
])

const terminalRunStatuses = new Set(['succeeded', 'failed', 'cancelled', 'rejected'])

type RemoteReceipt = {
  summary?: string
  facts?: Record<string, unknown>
  error?: string
}

function receiptFromParent(events: Event[], childRunId: string): RemoteReceipt | undefined {
  for (let i = events.length - 1; i >= 0; i--) {
    const ev = events[i]
    if (ev.type !== 'run.receipt' || ev.data?.child_run_id !== childRunId) continue
    const facts =
      ev.data.facts && typeof ev.data.facts === 'object' && !Array.isArray(ev.data.facts)
        ? (ev.data.facts as Record<string, unknown>)
        : undefined
    return {
      summary: typeof ev.data.summary === 'string' ? ev.data.summary : undefined,
      facts,
      error: typeof ev.data.error === 'string' ? ev.data.error : undefined,
    }
  }
  return undefined
}

function mapRunStatus(status: string | undefined): SubagentBlock['status'] | undefined {
  switch (status) {
    case 'queued':
    case 'running':
      return 'running'
    case 'waiting_human':
      return 'waiting'
    case 'succeeded':
      return 'succeeded'
    case 'failed':
      return 'failed'
    case 'cancelled':
      return 'cancelled'
    case 'rejected':
      return 'rejected'
    default:
      return undefined
  }
}

export function subagentContinuePrompt(block: SubagentBlock): string {
  const status = statusLabel(block.status, block.waitingKind)
  const lines = [`${SUBAGENT.title}「${block.goal || '子任务'}」${status}。`]
  if (block.summary) lines.push(`${SUBAGENT.summary}: ${block.summary}`)
  const facts = Object.entries(block.facts ?? {}).slice(0, 6)
  if (facts.length > 0) {
    lines.push(`${SUBAGENT.facts}:`)
    for (const [k, v] of facts) lines.push(`- ${k}: ${shortText(v, 160)}`)
  }
  if (block.error) lines.push(`${SUBAGENT.error}: ${block.error}`)
  if (block.status === 'succeeded') {
    lines.push('请根据以上回执继续处理；如果没有需要继续的动作，简要告知我结果。')
  } else {
    lines.push('请判断是否需要重试或改用其他方式，并说明结果。')
  }
  return lines.join('\n')
}

function lastWaiting(events: Event[]): WaitingInfo | undefined {
  for (let i = events.length - 1; i >= 0; i--) {
    const ev = events[i]
    if (ev.type === 'hitl.resumed' || ev.type === 'hitl.rejected' || ev.type === 'hitl.expired') {
      return undefined
    }
    if (ev.type !== 'hitl.waiting') continue
    const kind = ev.data?.kind === 'login' ? 'login' : 'approval'
    if (ev.data?.kind === 'waiting_child') continue
    return {
      kind,
      toolName: String(ev.data?.tool_name ?? ''),
      prompt: String(ev.data?.prompt ?? ''),
      connectorId: String(ev.data?.connector_id ?? ''),
      args: ev.data?.arguments,
    }
  }
  return undefined
}

function shortText(value: unknown, max = 160): string {
  const text = typeof value === 'string' ? value : JSON.stringify(value)
  if (!text) return ''
  return text.length > max ? `${text.slice(0, max)}…` : text
}

function eventLine(ev: Event): string | undefined {
  switch (ev.type) {
    case 'llm.tool_call':
      return `→ ${String(ev.data?.name ?? 'tool')}`
    case 'tool.result':
      return `${ev.data?.is_error ? '✗' : '✓'} ${String(ev.data?.name ?? 'tool')}`
    case 'llm.message':
      return shortText(ev.data?.content, 120)
    case 'llm.error':
      return `! ${shortText(ev.data?.error, 120)}`
    case 'hitl.waiting':
      if (ev.data?.kind === 'waiting_child') return undefined
      return ev.data?.kind === 'login'
        ? `${SUBAGENT.waitingLogin}: ${String(ev.data?.tool_name ?? '')}`
        : `${SUBAGENT.waitingApproval}: ${String(ev.data?.tool_name ?? '')}`
    case 'hitl.resumed':
      return '✓ resumed'
    case 'hitl.rejected':
      return '✗ rejected'
    default:
      return undefined
  }
}

export function SubagentCard({
  block,
  onBrowserLogin,
  onError,
  onContinue,
  continueDisabled = false,
}: SubagentCardProps) {
  const [expanded, setExpanded] = useState(false)
  const [loading, setLoading] = useState(false)
  const [events, setEvents] = useState<Event[]>([])
  const [binding, setBinding] = useState<Record<string, unknown> | null>(null)
  const [loadError, setLoadError] = useState(false)
  const [busy, setBusy] = useState(false)
  const [copied, setCopied] = useState(false)
  const [remoteStatus, setRemoteStatus] = useState<string | undefined>()
  const [remoteReceipt, setRemoteReceipt] = useState<RemoteReceipt | undefined>()
  const [confirmStop, setConfirmStop] = useState(false)
  const inFlight = useRef(false)
  const receiptFetched = useRef(false)

  useEffect(() => {
    setEvents([])
    setBinding(null)
    setLoadError(false)
    setExpanded(false)
    setRemoteStatus(undefined)
    setRemoteReceipt(undefined)
    setConfirmStop(false)
    inFlight.current = false
    receiptFetched.current = false
  }, [block.childRunId])

  const load = useCallback(
    async (silent = false, includeBinding = true) => {
      if (inFlight.current) return
      inFlight.current = true
      if (!silent) setLoading(true)
      setLoadError(false)
      const [evs, run, bind] = await Promise.allSettled([
        listEvents(block.childRunId),
        getRun(block.childRunId),
        includeBinding ? getRunBinding(block.childRunId) : Promise.resolve(undefined),
      ])
      if (evs.status === 'fulfilled') {
        setEvents(evs.value)
      } else {
        setLoadError(true)
      }
      if (run.status === 'fulfilled') {
        const status = run.value.status
        setRemoteStatus(status)
        // A parent run may have ended before its async child; the card in
        // history then lacks summary/facts. Read the parent receipt once.
        if (!receiptFetched.current && terminalRunStatuses.has(status) && block.runId) {
          const parent = await Promise.allSettled([listEvents(block.runId)])
          const parentEvs = parent[0]
          if (parentEvs.status === 'fulfilled') {
            const receipt = receiptFromParent(parentEvs.value, block.childRunId)
            if (receipt) {
              setRemoteReceipt(receipt)
              receiptFetched.current = true
            }
          }
        }
      }
      if (bind.status === 'fulfilled' && bind.value) {
        setBinding(bind.value.binding)
      }
      inFlight.current = false
      if (!silent) setLoading(false)
    },
    [block.childRunId, block.runId],
  )

  // History cards may mount after the async child already finished; a single
  // silent refresh lets them fill in terminal status and receipt facts.
  useEffect(() => {
    if (!block.async || terminalSubagentStatuses.has(block.status)) return
    void load(true, false)
  }, [block.async, block.status, block.childRunId, load])

  const toggle = () => {
    const next = !expanded
    setExpanded(next)
    if (next && events.length === 0 && !loading) void load(false, true)
  }

  const mappedStatus = mapRunStatus(remoteStatus)
  const openWait = lastWaiting(events)
  let effectiveStatus = block.status
  if (terminalSubagentStatuses.has(block.status)) {
    effectiveStatus = block.status
  } else if (mappedStatus && terminalSubagentStatuses.has(mappedStatus)) {
    effectiveStatus = mappedStatus
  } else if (block.status === 'waiting') {
    // Keep a parent-reported wait only while the child events still show one;
    // otherwise a missed run.child_resumed falls back to the remote status.
    effectiveStatus = openWait ? 'waiting' : mappedStatus ?? 'waiting'
  } else {
    effectiveStatus = mappedStatus ?? block.status
  }
  const waiting = effectiveStatus === 'waiting' ? openWait : undefined
  const effectiveWaitingKind =
    effectiveStatus === 'waiting' ? block.waitingKind ?? waiting?.kind : undefined
  const canStop = effectiveStatus === 'running' || effectiveStatus === 'waiting'
  const shownSummary = block.summary?.trim() ? block.summary : remoteReceipt?.summary
  const shownFacts =
    block.facts && Object.keys(block.facts).length > 0 ? block.facts : remoteReceipt?.facts
  const shownError = block.error?.trim() ? block.error : remoteReceipt?.error
  const displayBlock: SubagentBlock = {
    ...block,
    status: effectiveStatus,
    waitingKind: effectiveWaitingKind,
    summary: shownSummary,
    facts: shownFacts,
    error: shownError,
  }

  useEffect(() => {
    if (!expanded) return
    if (effectiveStatus !== 'running' && effectiveStatus !== 'waiting') return
    const id = window.setInterval(() => {
      void load(true, false)
    }, 3000)
    return () => window.clearInterval(id)
  }, [expanded, effectiveStatus, load])

  const decide = async (decision: 'approve' | 'reject', comment = '') => {
    setBusy(true)
    try {
      await resumeRun(block.childRunId, decision, comment)
      await load(false, true)
    } catch (err) {
      onError?.(err)
    } finally {
      setBusy(false)
    }
  }

  const stop = async () => {
    setBusy(true)
    try {
      await cancelRun(block.childRunId)
      await load(false, true)
    } catch (err) {
      onError?.(err)
    } finally {
      setBusy(false)
      setConfirmStop(false)
    }
  }

  const copyId = async () => {
    try {
      await navigator.clipboard?.writeText(block.childRunId)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      /* clipboard is best-effort */
    }
  }

  const facts = Object.entries(shownFacts ?? {}).slice(0, 6)
  const trace = events
    .map((ev) => ({ ev, line: eventLine(ev) }))
    .filter((x) => x.line)
    .slice(-40)

  return (
    <div className="tool-card subagent-card" data-testid="subagent-card">
      <button
        type="button"
        className="tool-card-header subagent-head"
        data-testid="subagent-toggle"
        onClick={toggle}
        aria-expanded={expanded}
      >
        <span className="subagent-title">
          <Bot size={15} aria-hidden />
          <span className="subagent-goal">{block.goal || SUBAGENT.title}</span>
          <span className={`subagent-status subagent-status-${effectiveStatus}`}>
            {statusLabel(effectiveStatus, effectiveWaitingKind)}
          </span>
        </span>
        <span className="subagent-head-right">
          {block.groupId ? (
            <span className="subagent-meta">
              {SUBAGENT.group(block.groupId)}
              {block.mode ? ` · ${block.mode === 'readwrite' ? SUBAGENT.readwrite : SUBAGENT.readonly}` : ''}
              {block.async ? ` · ${SUBAGENT.async}` : ''}
            </span>
          ) : null}
          <ChevronDown
            size={14}
            className={`tool-card-chevron${expanded ? ' expanded' : ''}`}
            aria-hidden
          />
        </span>
      </button>

      {shownSummary ? <p className="subagent-summary">{shownSummary}</p> : null}
      {shownError ? <p className="subagent-error">{shownError}</p> : null}
      {facts.length > 0 ? (
        <dl className="subagent-facts" data-testid="subagent-facts">
          {facts.map(([k, v]) => (
            <div key={k} className="subagent-fact">
              <dt>{k}</dt>
              <dd>{shortText(v)}</dd>
            </div>
          ))}
        </dl>
      ) : null}

      {block.async && terminalSubagentStatuses.has(effectiveStatus) && onContinue ? (
        <div className="subagent-continue">
          <Button
            size="sm"
            variant="secondary"
            disabled={continueDisabled}
            onClick={() => onContinue(subagentContinuePrompt(displayBlock))}
          >
            {SUBAGENT.continueWork}
          </Button>
          {continueDisabled ? (
            <span className="subagent-continue-hint">{SUBAGENT.continueBusy}</span>
          ) : null}
        </div>
      ) : null}

      {expanded ? (
        <div className="subagent-trace" data-testid="subagent-trace">
          <div className="subagent-trace-head">
            <span>{SUBAGENT.viewTrace}</span>
            <span className="subagent-trace-actions">
              {canStop ? (
                confirmStop ? (
                  <>
                    <Button
                      size="sm"
                      variant="danger"
                      disabled={busy}
                      onClick={() => void stop()}
                    >
                      {SUBAGENT.confirmStop}
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={busy}
                      onClick={() => setConfirmStop(false)}
                    >
                      {SUBAGENT.stopCancel}
                    </Button>
                  </>
                ) : (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    onClick={() => setConfirmStop(true)}
                  >
                    {SUBAGENT.stop}
                  </Button>
                )
              ) : null}
              <Button
                size="sm"
                variant="ghost"
                disabled={loading}
                onClick={() => void load(false, true)}
              >
                <RefreshCw size={13} className={loading ? 'icon-spin' : undefined} aria-hidden />
                {SUBAGENT.refresh}
              </Button>
              <Button size="sm" variant="ghost" onClick={() => void copyId()}>
                {copied ? SUBAGENT.copied : SUBAGENT.copyId}
              </Button>
            </span>
          </div>

          {effectiveStatus === 'waiting' ? (
            <div className="subagent-waiting" data-testid="subagent-waiting">
              {waiting ? (
                <>
                  <p className="subagent-waiting-title">
                    {waiting.kind === 'login' ? SUBAGENT.waitingLogin : SUBAGENT.waitingApproval}
                    {waiting.toolName ? ` · ${waiting.toolName}` : ''}
                  </p>
                  {waiting.args !== undefined ? (
                    <pre className="tool-card-json">{JSON.stringify(waiting.args, null, 2)}</pre>
                  ) : null}
                  <div className="subagent-waiting-actions">
                    {waiting.kind === 'login' ? (
                      <>
                        {waiting.connectorId && onBrowserLogin ? (
                          <Button
                            size="sm"
                            variant="primary"
                            disabled={busy}
                            onClick={() => onBrowserLogin(waiting.connectorId)}
                          >
                            {SUBAGENT.browserLogin}
                          </Button>
                        ) : null}
                        <Button
                          size="sm"
                          variant="primary"
                          disabled={busy}
                          onClick={() => void decide('approve')}
                        >
                          {SUBAGENT.continueLogin}
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={busy}
                          onClick={() => void decide('reject', 'abort_login')}
                        >
                          {SUBAGENT.abort}
                        </Button>
                      </>
                    ) : (
                      <>
                        <Button
                          size="sm"
                          variant="primary"
                          disabled={busy}
                          onClick={() => void decide('approve')}
                        >
                          {SUBAGENT.approve}
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={busy}
                          onClick={() => void decide('reject')}
                        >
                          {SUBAGENT.reject}
                        </Button>
                      </>
                    )}
                  </div>
                </>
              ) : (
                <p className="subagent-trace-empty">{SUBAGENT.waitingApproval}</p>
              )}
            </div>
          ) : null}

          {loading ? <p className="subagent-trace-empty">{SUBAGENT.refresh}…</p> : null}
          {loadError ? <p className="subagent-trace-empty">{SUBAGENT.traceUnavailable}</p> : null}
          {!loading && !loadError && trace.length === 0 ? (
            <p className="subagent-trace-empty">{SUBAGENT.traceUnavailable}</p>
          ) : null}
          {trace.length > 0 ? (
            <ul className="subagent-trace-list">
              {trace.map(({ ev, line }, i) => (
                <li key={`${ev.timestamp}-${i}`} className="subagent-trace-item">
                  <span className="subagent-trace-type">{ev.type}</span>
                  <span>{line}</span>
                </li>
              ))}
            </ul>
          ) : null}

          {binding && Object.keys(binding).length > 0 ? (
            <details className="subagent-binding">
              <summary>{SUBAGENT.binding}</summary>
              <pre className="tool-card-json">{JSON.stringify(binding, null, 2)}</pre>
            </details>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
