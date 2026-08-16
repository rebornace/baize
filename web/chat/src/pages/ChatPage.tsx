import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  createRun,
  getRun,
  getUIConfig,
  isTerminal,
  listConversations,
  listEvents,
  listMessages,
  openRunStream,
  type ChatMessage,
  type Event,
  type RunStatus,
} from '../api'
import { Composer } from '../components/Composer'
import { ToolCard } from '../components/ToolCard'
import { clearControlToken } from '../controlAuth'
import { findLiveRunCandidate, isActiveRunStatus } from '../findLiveRun'
import { foldEvents, type ChatBlock } from '../foldEvents'
import { useGate } from '../gateContext'

const CONV_KEY = 'baize.conversation_id'
const POLL_MS = 700
const AGENT_FALLBACK = 'ticket-agent'

type ConversationSummary = { id: string; title: string; updated_at: string }

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

export function ChatPage() {
  const { role, gateEnabled } = useGate()
  const [agentId, setAgentId] = useState(AGENT_FALLBACK)
  const [conversationId, setConversationIdState] = useState(loadConversationId)
  const [conversations, setConversations] = useState<ConversationSummary[]>([])
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [liveEvents, setLiveEvents] = useState<Event[]>([])
  const [liveRunId, setLiveRunId] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState('')
  const [error, setError] = useState<string | null>(null)

  const cancelStreamRef = useRef<(() => void) | null>(null)
  const pollTimerRef = useRef<number | null>(null)
  const lastEventIndexRef = useRef(-1)
  const conversationIdRef = useRef(conversationId)
  conversationIdRef.current = conversationId

  const setConversationId = useCallback((id: string) => {
    setConversationIdState(id)
    localStorage.setItem(CONV_KEY, id)
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
      const list = await listConversations()
      setConversations(list)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }, [])

  const finishLiveRun = useCallback(
    async (id: string) => {
      stopStream()
      stopPoll()
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
    [refreshConversations, stopPoll, stopStream],
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
      })
      .catch(() => {
        /* keep fallback */
      })
    void refreshConversations()
  }, [refreshConversations])

  useEffect(() => {
    let cancelled = false
    stopStream()
    stopPoll()
    setLiveRunId(null)
    setLiveEvents([])
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
  }, [conversationId, restoreLiveRun, stopPoll, stopStream])

  const onNewChat = () => {
    stopStream()
    stopPoll()
    setLiveRunId(null)
    setLiveEvents([])
    setMessages([])
    setBusy(false)
    setError(null)
    setStatus('')
    setConversationId(newConversationId())
  }

  const onSelectConversation = (id: string) => {
    if (id === conversationId) return
    setConversationId(id)
  }

  const onSend = async (text: string) => {
    const sentConversationId = conversationId
    setBusy(true)
    setError(null)
    setStatus('发送中…')
    setMessages((prev) => [
      ...prev,
      {
        id: `local_${Date.now()}`,
        conversation_id: sentConversationId,
        role: 'user',
        content: text,
        created_at: new Date().toISOString(),
      },
    ])
    setLiveEvents([])
    lastEventIndexRef.current = -1

    try {
      const created = await createRun(agentId, text, sentConversationId)
      await refreshConversations()
      if (conversationIdRef.current !== sentConversationId) return
      setStatus(statusLabel(created.status))
      setLiveRunId(created.run_id)
      startStream(created.run_id, sentConversationId, -1)
    } catch (err) {
      if (conversationIdRef.current !== sentConversationId) return
      setBusy(false)
      setLiveRunId(null)
      setError(err instanceof Error ? err.message : String(err))
      setStatus('')
      try {
        const msgs = await listMessages(sentConversationId)
        if (conversationIdRef.current === sentConversationId) setMessages(msgs)
      } catch {
        /* keep optimistic row */
      }
    }
  }

  const liveBlocks: ChatBlock[] = liveRunId ? foldEvents(liveRunId, liveEvents) : []
  const showWelcome = messages.length === 0 && liveBlocks.length === 0

  return (
    <div className="chat-shell">
      <aside className="chat-sidebar" aria-label="对话列表">
        <div className="chat-sidebar-top">
          <button type="button" className="btn ghost sidebar-new" onClick={onNewChat}>
            新对话
          </button>
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
                  {c.title || '新对话'}
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
        <div className="messages" aria-live="polite">
          {showWelcome && (
            <p className="welcome">你好，发送一条消息开始对话。</p>
          )}
          {messages.map((m) => {
            const bubbleClass =
              m.role === 'user' ? 'user' : m.role === 'system_note' ? 'system' : 'assistant'
            return (
              <div key={m.id} className={`bubble ${bubbleClass}`}>
                {m.content}
              </div>
            )
          })}
          {liveBlocks.map((block, i) => {
            switch (block.kind) {
              case 'assistant':
                return (
                  <div key={`live-a-${i}`} className="bubble assistant">
                    {block.text}
                  </div>
                )
              case 'system':
                return (
                  <div key={`live-s-${i}`} className="bubble system">
                    {block.text}
                  </div>
                )
              case 'tool':
                return <ToolCard key={`live-t-${i}`} block={block} />
              case 'user':
                return (
                  <div key={`live-u-${i}`} className="bubble user">
                    {block.text}
                  </div>
                )
              default: {
                const _exhaustive: never = block
                return _exhaustive
              }
            }
          })}
        </div>

        {(status || error) && (
          <p className={`status${error ? ' status-error' : ''}`}>{error ?? status}</p>
        )}

        <Composer disabled={busy} onSend={(t) => void onSend(t)} />
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
    default:
      return status
  }
}
