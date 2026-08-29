import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  ApiError,
  cancelRun,
  createRun,
  fileToAttachment,
  forkConversation,
  getRun,
  getUIConfig,
  isImageAttachment,
  isTerminal,
  listConversations,
  listEvents,
  listMessages,
  listSkills,
  openRunStream,
  rollbackMessages,
  type Attachment,
  type ChatMessage,
  type ConversationScope,
  type ConversationSummary,
  type Event,
  type RunStatus,
  type SkillSummary,
} from '../api'
import { extractAnalysisPagesFromEvents } from '../analysisPage'
import { AnalysisPagePreview } from '../components/AnalysisPagePreview'
import { Composer } from '../components/Composer'
import { MarkdownText } from '../components/MarkdownText'
import { ToolCard } from '../components/ToolCard'
import { WorkflowCard } from '../components/WorkflowCard'
import { TypewriterText } from '../components/TypewriterText'
import { conversationListLabel } from '../conversationLabel'
import { clearControlToken } from '../controlAuth'
import { findLiveRunCandidate, isActiveRunStatus } from '../findLiveRun'
import { foldEvents, type ChatBlock } from '../foldEvents'
import { useGate } from '../gateContext'
import { useStickToBottom } from '../useStickToBottom'

const CONV_KEY = 'baize.conversation_id'
const SCOPE_KEY = 'baize.conversation_scope'
const POLL_MS = 700
const AGENT_FALLBACK = 'ticket-agent'

function newConversationId(): string {
  return `conv_${crypto.randomUUID()}`
}

function loadConversationId(): string {
  const existing = localStorage.getItem(CONV_KEY)?.trim()
  if (existing) return existing
  const id = newConversationId()
  localStorage.setItem(CONV_KEY, id)
  return id
}

function loadConversationScope(isAdmin: boolean): ConversationScope {
  if (!isAdmin) return 'mine'
  const raw = localStorage.getItem(SCOPE_KEY)?.trim()
  return raw === 'mine' ? 'mine' : 'all'
}

export function ChatPage() {
  const { role, gateEnabled } = useGate()
  const [agentId, setAgentId] = useState(AGENT_FALLBACK)
  const [conversationId, setConversationIdState] = useState(loadConversationId)
  const [conversationScope, setConversationScopeState] = useState<ConversationScope>(() =>
    loadConversationScope(role === 'admin'),
  )
  const [conversations, setConversations] = useState<ConversationSummary[]>([])
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [liveEvents, setLiveEvents] = useState<Event[]>([])
  const [liveRunId, setLiveRunId] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [historyMutating, setHistoryMutating] = useState(false)
  const [status, setStatus] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [runWebhookUrl, setRunWebhookUrl] = useState('')
  const [sessionToken, setSessionToken] = useState('')
  const [composerDraft, setComposerDraft] = useState<string | undefined>(undefined)
  const [skills, setSkills] = useState<SkillSummary[]>([])
  const [supportsVision, setSupportsVision] = useState(true)
  /** run_id → analysis page artifact URLs (kept after live run ends / on reload). */
  const [historyPages, setHistoryPages] = useState<Record<string, string[]>>({})

  const cancelStreamRef = useRef<(() => void) | null>(null)
  const pollTimerRef = useRef<number | null>(null)
  const lastEventIndexRef = useRef(-1)
  const conversationIdRef = useRef(conversationId)
  conversationIdRef.current = conversationId
  const liveRunIdRef = useRef(liveRunId)
  liveRunIdRef.current = liveRunId
  const liveEventsRef = useRef(liveEvents)
  liveEventsRef.current = liveEvents
  const conversationScopeRef = useRef(conversationScope)
  conversationScopeRef.current = conversationScope

  const { scrollerRef, bottomRef, onScroll, scrollToBottom } = useStickToBottom([
    messages,
    liveEvents,
    liveRunId,
    conversationId,
    historyPages,
  ])

  const setConversationId = useCallback((id: string) => {
    setConversationIdState(id)
    localStorage.setItem(CONV_KEY, id)
  }, [])

  const setConversationScope = useCallback((scope: ConversationScope) => {
    setConversationScopeState(scope)
    localStorage.setItem(SCOPE_KEY, scope)
  }, [])

  const stopPoll = useCallback(() => {
    if (pollTimerRef.current !== null) {
      window.clearInterval(pollTimerRef.current)
      pollTimerRef.current = null
    }
  }, [])

  const stopStream = useCallback(() => {
    cancelStreamRef.current?.()
    cancelStreamRef.current = null
  }, [])

  const refreshConversations = useCallback(async () => {
    try {
      const scope = role === 'admin' ? conversationScopeRef.current : undefined
      const list = await listConversations(scope)
      setConversations(list)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }, [role])

  const mergeHistoryPages = useCallback((runId: string, urls: string[]) => {
    if (!runId || urls.length === 0) return
    setHistoryPages((prev) => {
      const existing = prev[runId] ?? []
      const merged = [...existing]
      for (const u of urls) {
        if (!merged.includes(u)) merged.push(u)
      }
      if (merged.length === existing.length) return prev
      return { ...prev, [runId]: merged }
    })
  }, [])

  const finishLiveRun = useCallback(
    async (id: string) => {
      stopStream()
      stopPoll()
      const runId = liveRunIdRef.current
      if (runId) {
        const pages = extractAnalysisPagesFromEvents(liveEventsRef.current)
        if (pages.length > 0) {
          mergeHistoryPages(
            runId,
            pages.map((p) => p.artifactUrl),
          )
        }
      }
      setLiveRunId(null)
      setLiveEvents([])
      lastEventIndexRef.current = -1
      setBusy(false)
      try {
        const msgs = await listMessages(id)
        if (conversationIdRef.current !== id) return
        setMessages(msgs)
      } catch (err) {
        if (conversationIdRef.current !== id) return
        setError(err instanceof Error ? err.message : String(err))
      }
      await refreshConversations()
    },
    [mergeHistoryPages, refreshConversations, stopPoll, stopStream],
  )

  const applyEvents = useCallback((events: Event[]) => {
    setLiveEvents(events)
  }, [])

  const startPoll = useCallback(
    (runId: string, forConversationId: string) => {
      stopPoll()
      const tick = async () => {
        if (conversationIdRef.current !== forConversationId) {
          stopPoll()
          return
        }
        try {
          const [run, events] = await Promise.all([getRun(runId), listEvents(runId)])
          if (conversationIdRef.current !== forConversationId) return
          applyEvents(events)
          lastEventIndexRef.current = events.length - 1
          setStatus(statusLabel(run.status))
          if (isTerminal(run.status)) {
            await finishLiveRun(forConversationId)
          }
        } catch (err) {
          if (conversationIdRef.current !== forConversationId) return
          setError(err instanceof Error ? err.message : String(err))
        }
      }
      void tick()
      pollTimerRef.current = window.setInterval(() => {
        void tick()
      }, POLL_MS)
    },
    [applyEvents, finishLiveRun, stopPoll],
  )

  const startStream = useCallback(
    (runId: string, forConversationId: string, after: number) => {
      stopStream()
      stopPoll()
      setLiveRunId(runId)
      lastEventIndexRef.current = after

      cancelStreamRef.current = openRunStream(
        runId,
        after,
        (ev, index) => {
          if (conversationIdRef.current !== forConversationId) return
          setLiveEvents((prev) => [...prev, ev])
          if (index >= 0) lastEventIndexRef.current = index
        },
        (endedStatus) => {
          if (conversationIdRef.current !== forConversationId) return
          setStatus(statusLabel(endedStatus as RunStatus))
          void finishLiveRun(forConversationId)
        },
        () => {
          if (conversationIdRef.current !== forConversationId) return
          setStatus('SSE 中断，改用轮询…')
          startPoll(runId, forConversationId)
        },
      )
    },
    [finishLiveRun, startPoll, stopPoll, stopStream],
  )

  const restoreLiveRun = useCallback(
    async (id: string, msgs: ChatMessage[]) => {
      const candidate = findLiveRunCandidate(msgs)
      if (!candidate) return
      const run = await getRun(candidate)
      if (conversationIdRef.current !== id) return
      if (!isActiveRunStatus(run.status)) return
      setBusy(true)
      setStatus(statusLabel(run.status))
      setLiveEvents([])
      lastEventIndexRef.current = -1
      startStream(candidate, id, -1)
    },
    [startStream],
  )

  useEffect(() => {
    void getUIConfig()
      .then((cfg) => {
        if (cfg.agent_id) setAgentId(cfg.agent_id)
        setSupportsVision(cfg.supports_vision !== false)
      })
      .catch(() => {
        /* keep fallback */
      })
    // Skills drive the Composer @-completion popup. GET /v0/skills is operator
    // readable; load failures just disable the popup rather than blocking chat.
    void listSkills()
      .then((res) => setSkills(res.skills ?? []))
      .catch(() => setSkills([]))
  }, [])

  useEffect(() => {
    void refreshConversations()
  }, [conversationScope, refreshConversations])

  useEffect(() => {
    let cancelled = false
    stopStream()
    stopPoll()
    setLiveRunId(null)
    setLiveEvents([])
    setHistoryPages({})
    lastEventIndexRef.current = -1
    setBusy(false)
    setError(null)
    setStatus('')

    const id = conversationId
    void (async () => {
      try {
        const msgs = await listMessages(id)
        if (cancelled || conversationIdRef.current !== id) return
        setMessages(msgs)
        requestAnimationFrame(() => scrollToBottom('auto'))
        await restoreLiveRun(id, msgs)
      } catch (err) {
        if (cancelled || conversationIdRef.current !== id) return
        setError(err instanceof Error ? err.message : String(err))
      }
    })()

    return () => {
      cancelled = true
      stopStream()
      stopPoll()
    }
  }, [conversationId, restoreLiveRun, scrollToBottom, stopPoll, stopStream])

  // Load analysis page previews for completed turns (refresh / reopen conversation).
  useEffect(() => {
    const runIds = [
      ...new Set(
        messages
          .filter((m) => m.role === 'assistant' && m.run_id)
          .map((m) => m.run_id as string),
      ),
    ]
    if (runIds.length === 0) return
    let cancelled = false
    void (async () => {
      await Promise.all(
        runIds.map(async (runId) => {
          if (historyPages[runId]?.length) return
          try {
            const events = await listEvents(runId)
            if (cancelled) return
            const pages = extractAnalysisPagesFromEvents(events)
            if (pages.length > 0) {
              mergeHistoryPages(
                runId,
                pages.map((p) => p.artifactUrl),
              )
            }
          } catch {
            /* ignore missing runs */
          }
        }),
      )
    })()
    return () => {
      cancelled = true
    }
    // historyPages intentionally omitted: we only fetch missing run ids.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [messages, mergeHistoryPages])

  const onNewChat = () => {
    stopStream()
    stopPoll()
    setLiveRunId(null)
    setLiveEvents([])
    setHistoryPages({})
    setMessages([])
    setBusy(false)
    setError(null)
    setStatus('')
    setComposerDraft(undefined)
    setConversationId(newConversationId())
  }

  const onSelectConversation = (id: string) => {
    if (id === conversationId) return
    setConversationId(id)
  }

  const onSend = async (text: string, files: File[]) => {
    const sentConversationId = conversationId
    setBusy(true)
    setComposerDraft(undefined)
    setError(null)
    setStatus('发送中…')

    // Build attachments from selected files. Image attachments require a
    // vision-capable model; when supports_vision is false we reject up-front
    // (mirrors the server's vision_unsupported check) so no run is created.
    let attachments: Attachment[] | undefined
    if (files.length > 0) {
      try {
        const built = await Promise.all(files.map((f) => fileToAttachment(f)))
        if (!supportsVision && built.some((a) => isImageAttachment(a.media_type))) {
          setBusy(false)
          setError('当前模型不支持图片附件，请移除图片或切换到支持视觉的模型。')
          setStatus('')
          return
        }
        attachments = built
      } catch (err) {
        setBusy(false)
        setError(err instanceof Error ? err.message : String(err))
        setStatus('')
        return
      }
    }

    const displayNames = attachments && attachments.length > 0
      ? `（附件：${attachments.map((a) => a.filename).join(', ')}）`
      : ''
    const userBubble = (text + (displayNames ? ` ${displayNames}` : '')).trim()

    setMessages((prev) => [
      ...prev,
      {
        id: `local_${Date.now()}`,
        conversation_id: sentConversationId,
        role: 'user',
        content: userBubble,
        created_at: new Date().toISOString(),
      },
    ])
    setLiveEvents([])
    lastEventIndexRef.current = -1
    requestAnimationFrame(() => scrollToBottom('smooth'))

    try {
      const webhookUrl = runWebhookUrl.trim()
      const token = sessionToken.trim()
      const created = await createRun(agentId, text, sentConversationId, {
        webhookUrl: webhookUrl || undefined,
        sessionToken: token || undefined,
        attachments,
      })
      await refreshConversations()
      if (conversationIdRef.current !== sentConversationId) return
      setStatus(statusLabel(created.status))
      setLiveRunId(created.run_id)
      startStream(created.run_id, sentConversationId, -1)
    } catch (err) {
      if (conversationIdRef.current !== sentConversationId) return
      setBusy(false)
      setLiveRunId(null)
      if (err instanceof ApiError && err.code === 'vision_unsupported') {
        setError('当前模型不支持图片附件，请移除图片或切换到支持视觉的模型。')
      } else {
        setError(err instanceof Error ? err.message : String(err))
      }
      setStatus('')
      try {
        const msgs = await listMessages(sentConversationId)
        if (conversationIdRef.current === sentConversationId) setMessages(msgs)
      } catch {
        /* keep optimistic row */
      }
    }
  }

  const onRollbackUser = async (m: ChatMessage) => {
    if (busy || liveRunId || historyMutating) return
    setError(null)
    setHistoryMutating(true)
    try {
      const res = await rollbackMessages(conversationId, m.id)
      setMessages(res.messages)
      setComposerDraft(m.content)
      await refreshConversations()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setHistoryMutating(false)
    }
  }

  const onRegenerate = async (m: ChatMessage) => {
    if (busy || liveRunId || historyMutating) return
    setError(null)
    setBusy(true)
    setHistoryMutating(true)
    try {
      const res = await rollbackMessages(conversationId, m.id, {
        regenerate: true,
        agentId,
      })
      setMessages(res.messages)
      setComposerDraft(undefined)
      await refreshConversations()
      if (!res.regenerated_run) {
        setBusy(false)
        return
      }
      const runId = res.regenerated_run.run_id
      setStatus(statusLabel(res.regenerated_run.status))
      setLiveRunId(runId)
      setLiveEvents([])
      lastEventIndexRef.current = -1
      startStream(runId, conversationId, -1)
    } catch (err) {
      setBusy(false)
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setHistoryMutating(false)
    }
  }

  const onRollbackTo = async (m: ChatMessage) => {
    if (busy || liveRunId || historyMutating) return
    setError(null)
    setHistoryMutating(true)
    try {
      const res = await rollbackMessages(conversationId, m.id)
      setMessages(res.messages)
      setComposerDraft(undefined)
      await refreshConversations()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setHistoryMutating(false)
    }
  }

  const onFork = async (m: ChatMessage) => {
    if (busy || liveRunId || historyMutating) return
    setError(null)
    setHistoryMutating(true)
    try {
      const res = await forkConversation(conversationId, m.id)
      setConversationId(res.conversation_id)
      setMessages(res.messages)
      setComposerDraft(undefined)
      await refreshConversations()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setHistoryMutating(false)
    }
  }

  const onCancelRun = async () => {
    if (!liveRunId) return
    setError(null)
    setStatus('正在取消…')
    try {
      await cancelRun(liveRunId)
      await finishLiveRun(conversationId)
      setStatus('已取消')
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      try {
        await finishLiveRun(conversationId)
      } catch {
        setBusy(false)
        setLiveRunId(null)
      }
    }
  }

  const liveBlocks: ChatBlock[] = liveRunId ? foldEvents(liveRunId, liveEvents) : []
  const showWelcome = messages.length === 0 && liveBlocks.length === 0
  const composerDisabled = busy || historyMutating
  const showStop = Boolean(liveRunId && busy)

  return (
    <div className="chat-shell">
      <aside className="chat-sidebar" aria-label="对话列表">
        <div className="chat-sidebar-top">
          <button type="button" className="btn ghost sidebar-new" onClick={onNewChat}>
            新对话
          </button>
          {role === 'admin' && (
            <div className="conversation-scope" role="group" aria-label="会话范围">
              <button
                type="button"
                className={
                  conversationScope === 'all'
                    ? 'conversation-scope-btn active'
                    : 'conversation-scope-btn'
                }
                onClick={() => setConversationScope('all')}
              >
                全部
              </button>
              <button
                type="button"
                className={
                  conversationScope === 'mine'
                    ? 'conversation-scope-btn active'
                    : 'conversation-scope-btn'
                }
                onClick={() => setConversationScope('mine')}
              >
                我的
              </button>
            </div>
          )}
          <ul className="conversation-list">
            {conversations.map((c) => (
              <li key={c.id}>
                <button
                  type="button"
                  className={
                    c.id === conversationId
                      ? 'conversation-item active'
                      : 'conversation-item'
                  }
                  onClick={() => onSelectConversation(c.id)}
                >
                  {conversationListLabel(c.id, c.title)}
                </button>
              </li>
            ))}
          </ul>
        </div>
        <div className="chat-sidebar-bottom">
          <Link
            to={role === 'admin' ? '/settings/tools' : '/settings/identities'}
            className="settings-link"
          >
            {role === 'admin' ? '设置' : '账号'}
          </Link>
          {gateEnabled && (
            <button
              type="button"
              className="settings-logout"
              onClick={() => {
                clearControlToken()
                window.location.assign('/ui/')
              }}
            >
              退出
            </button>
          )}
        </div>
      </aside>

      <main className="chat-main">
        <div
          className="messages"
          aria-live="polite"
          ref={scrollerRef}
          onScroll={onScroll}
        >
          <div className="messages-inner">
            {showWelcome && (
              <div className="welcome">
                <p className="welcome-title">开始对话</p>
                <p className="welcome-sub">发送一条消息，或粘贴临时 Token 后直接查询数据。</p>
              </div>
            )}
            {messages.map((m) => {
              const bubbleClass =
                m.role === 'user' ? 'user' : m.role === 'system_note' ? 'system' : 'assistant'
              const persisted = !m.id.startsWith('local_')
              const pages =
                m.role === 'assistant' && m.run_id && m.run_id !== liveRunId
                  ? historyPages[m.run_id] ?? []
                  : []
              return (
                <div key={m.id} className={`msg-row ${bubbleClass}`}>
                  <div className={`msg ${bubbleClass}`}>
                    {m.role === 'assistant' ? (
                      <MarkdownText text={m.content} />
                    ) : (
                      <MarkdownText text={m.content} plain />
                    )}
                  </div>
                  {pages.length > 0 && (
                    <div className="msg-analysis-pages">
                      {pages.map((url) => (
                        <AnalysisPagePreview key={url} artifactUrl={url} />
                      ))}
                    </div>
                  )}
                  {persisted && !busy && !liveRunId && !historyMutating && (
                    <div className="msg-actions">
                      {m.role === 'user' && (
                        <button type="button" className="btn ghost sm" onClick={() => void onRollbackUser(m)}>
                          编辑并回滚
                        </button>
                      )}
                      {m.role === 'assistant' && (
                        <button type="button" className="btn ghost sm" onClick={() => void onRegenerate(m)}>
                          重新生成
                        </button>
                      )}
                      {m.role === 'system_note' && (
                        <button type="button" className="btn ghost sm" onClick={() => void onRollbackTo(m)}>
                          回滚到此
                        </button>
                      )}
                      <button type="button" className="btn ghost sm" onClick={() => void onFork(m)}>
                        Fork
                      </button>
                    </div>
                  )}
                </div>
              )
            })}
            {liveBlocks.map((block, i) => {
              switch (block.kind) {
                case 'assistant':
                  return (
                    <div key={`live-a-${i}`} className="msg-row assistant">
                      <div className="msg assistant">
                        <TypewriterText text={block.text} active />
                      </div>
                    </div>
                  )
                case 'system':
                  return (
                    <div key={`live-s-${i}`} className="msg-row system">
                      <div className="msg system">
                        <MarkdownText text={block.text} plain />
                      </div>
                    </div>
                  )
                case 'tool':
                  return (
                    <div key={`live-t-${i}`} className="msg-row tool">
                      <ToolCard block={block} />
                    </div>
                  )
                case 'workflow':
                  return (
                    <div key={`live-w-${i}`} className="msg-row tool">
                      <WorkflowCard block={block} />
                    </div>
                  )
                case 'user':
                  return (
                    <div key={`live-u-${i}`} className="msg-row user">
                      <div className="msg user">
                        <MarkdownText text={block.text} plain />
                      </div>
                    </div>
                  )
                default: {
                  const _exhaustive: never = block
                  return _exhaustive
                }
              }
            })}
            <div ref={bottomRef} className="messages-end" aria-hidden />
          </div>
        </div>

        <div className="chat-footer">
          {(status || error || showStop) && (
            <div className="chat-status-row">
              <p className={`status${error ? ' status-error' : ''}`}>{error ?? status}</p>
              {showStop && (
                <button type="button" className="btn danger sm" onClick={() => void onCancelRun()}>
                  停止
                </button>
              )}
            </div>
          )}

          <details className="chat-advanced">
            <summary className="chat-advanced-summary">高级</summary>
            <label className="chat-advanced-field">
              <span className="chat-advanced-label">
                临时用户 Token（Bearer 或 JWT，仅本次会话；也可在消息里写 token: …）
              </span>
              <input
                className="chat-advanced-input"
                type="password"
                value={sessionToken}
                onChange={(e) => setSessionToken(e.target.value)}
                disabled={composerDisabled}
                placeholder="Bearer eyJ… 或粘贴 accessToken"
                autoComplete="off"
              />
            </label>
            <label className="chat-advanced-field">
              <span className="chat-advanced-label">本次 Run Webhook URL（留空使用全局配置）</span>
              <input
                className="chat-advanced-input"
                type="url"
                value={runWebhookUrl}
                onChange={(e) => setRunWebhookUrl(e.target.value)}
                disabled={busy}
                placeholder="https://example.com/hooks/this-run"
              />
            </label>
          </details>

          <Composer
            disabled={composerDisabled}
            draft={composerDraft}
            skills={skills}
            onSend={(t, f) => void onSend(t, f)}
          />
        </div>
      </main>
    </div>
  )
}

function statusLabel(status: string): string {
  switch (status) {
    case 'queued':
      return '排队中'
    case 'running':
      return '运行中'
    case 'waiting_human':
      return '等待批准'
    case 'succeeded':
      return '已完成'
    case 'failed':
      return '失败'
    case 'cancelled':
      return '已取消'
    default:
      return status
  }
}
