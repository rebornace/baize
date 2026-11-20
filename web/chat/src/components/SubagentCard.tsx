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

export function SubagentCard({ block, onBrowserLogin, onError }: SubagentCardProps) {
  const [expanded, setExpanded] = useState(false)
  const [loading, setLoading] = useState(false)
  const [events, setEvents] = useState<Event[]>([])
  const [binding, setBinding] = useState<Record<string, unknown> | null>(null)
  const [loadError, setLoadError] = useState(false)
  const [busy, setBusy] = useState(false)
  const [copied, setCopied] = useState(false)
  const [remoteStatus, setRemoteStatus] = useState<string | undefined>()
  const [confirmStop, setConfirmStop] = useState(false)
  const inFlight = useRef(false)

  useEffect(() => {
    setEvents([])
    setBinding(null)
    setLoadError(false)
    setExpanded(false)
    setRemoteStatus(undefined)
    setConfirmStop(false)
    inFlight.current = false
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
        setRemoteStatus(run.value.status)
      }
      if (bind.status === 'fulfilled' && bind.value) {
        setBinding(bind.value.binding)
      }
      inFlight.current = false
      if (!silent) setLoading(false)
    },
    [block.childRunId],
  )

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

  const facts = Object.entries(block.facts ?? {}).slice(0, 6)
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
            </span>
          ) : null}
          <ChevronDown
            size={14}
            className={`tool-card-chevron${expanded ? ' expanded' : ''}`}
            aria-hidden
          />
        </span>
      </button>

      {block.summary ? <p className="subagent-summary">{block.summary}</p> : null}
      {block.error ? <p className="subagent-error">{block.error}</p> : null}
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
