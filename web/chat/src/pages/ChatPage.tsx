import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { CornerUpLeft, GitBranch, Menu } from 'lucide-react'
import {
  cancelRun,
  createRun,
  deleteConversation,
  fileToAttachment,
  forkConversation,
  getRun,
  getUIConfig,
  isImageAttachment,
  isTerminal,
  listConversations,
  listEvents,
  listMessages,
  listModelProfiles,
  listSkills,
  listTools,
  openRunStream,
  rollbackMessages,
  type Attachment,
  type ChatMessage,
  type ConversationScope,
  type ConversationSummary,
  type Event,
  type ModelProfile,
  type RunStatus,
  type SkillSummary,
  type ToolInfo,
} from '../api'
import { extractAnalysisPagesFromEvents } from '../analysisPage'
import { AnalysisPagePreview } from '../components/AnalysisPagePreview'
import { Composer } from '../components/Composer'
import { SidebarResizer } from '../components/SidebarResizer'
import { MarkdownText } from '../components/MarkdownText'
import { ModelChip } from '../components/ModelChip'
import { ToolCard } from '../components/ToolCard'
import { UserBubble } from '../components/UserBubble'
import { WorkflowCard } from '../components/WorkflowCard'
import { TypewriterText } from '../components/TypewriterText'
import {
  Button,
  ConfirmDialog,
  DropdownMenu,
  Modal,
  ThemeToggle,
  ToastRegion,
  useToast,
  type MenuItem,
} from '../components/ui'
import { conversationListLabel } from '../conversationLabel'
import { clearControlToken } from '../controlAuth'
import { findLiveRunCandidate, isActiveRunStatus } from '../findLiveRun'
import { foldEvents, type ChatBlock } from '../foldEvents'
import type { ToolCatalog } from '../friendlyTool'
import { useGate } from '../gateContext'
import {
  foldToolBlocks,
  isFirstAssistantMessageOfRun,
  type ToolOrWorkflowBlock,
} from '../historyBlocks'
import { loadModelChoice, resolveModelChoice, saveModelChoice } from '../modelChoice'
import { AUTO_MODEL_ID, buildRunOptions, visionGate } from '../modelSelect'
import { buildLocalPreview, extractBlobURLs } from '../localAttachments'
import { ACTIONS, CHAT, friendlyError, WELCOME } from '../strings'
import { useStickToBottom } from '../useStickToBottom'
import { useDrawer } from '../useDrawer'
import { uuid } from '../uuid'


const CONV_KEY = 'baize.conversation_id'
const SCOPE_KEY = 'baize.conversation_scope'
const POLL_MS = 700
const IDLE_SYNC_MS = 2000
const AGENT_FALLBACK = 'ticket-agent'

function newConversationId(): string {
  return `conv_${uuid()}`
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
  const drawer = useDrawer()
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
  const [composerDraft, setComposerDraft] = useState<string | undefined>(undefined)
  const [skills, setSkills] = useState<SkillSummary[]>([])
  const [supportsVision, setSupportsVision] = useState(true)
  const [modelProfiles, setModelProfiles] = useState<ModelProfile[]>([])
  // Persisted model choice (localStorage via modelChoice.ts); "" only until the
  // lazy initializer runs. Auto is the server-side smart router.
  const [selectedModelId, setSelectedModelId] = useState(loadModelChoice)
  /** run_id → analysis page artifact URLs (kept after live run ends / on reload). */
  const [historyPages, setHistoryPages] = useState<Record<string, string[]>>({})
  /** run_id → folded historical tool/workflow blocks (read-only replay). */
  const [historyBlocks, setHistoryBlocks] = useState<Record<string, ToolOrWorkflowBlock[]>>({})
  const [toolCatalog, setToolCatalog] = useState<ToolCatalog>([])
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null)
  const [visionWarning, setVisionWarning] = useState<string | null>(null)
  const toast = useToast()

  const cancelStreamRef = useRef<(() => void) | null>(null)
  /** runIds whose events have already been fetched once (drives both pages and blocks). */
  const fetchedRunsRef = useRef<Set<string>>(new Set())
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

  // Transient blob: object URLs backing optimistic attachment previews. They
  // are revoked as soon as no message references them (the optimistic bubble
  // is replaced by the server version on run end / refresh / switch / delete).
  const liveBlobURLsRef = useRef<Set<string>>(new Set())
  useEffect(() => {
    const referenced = new Set(extractBlobURLs(messages.map((m) => m.content)))
    for (const url of liveBlobURLsRef.current) {
      if (!referenced.has(url)) URL.revokeObjectURL(url)
    }
    liveBlobURLsRef.current = referenced
  }, [messages])
  // Revoke every preview object URL on unmount (e.g. leaving the chat page).
  useEffect(() => {
    const tracked = liveBlobURLsRef
    return () => {
      for (const url of tracked.current) URL.revokeObjectURL(url)
      tracked.current = new Set()
    }
  }, [])

  const { scrollerRef, bottomRef, onScroll, scrollToBottom } = useStickToBottom([
    messages,
    liveEvents,
    liveRunId,
    conversationId,
    historyPages,
    historyBlocks,
  ])

  // User-initiated actions surface friendly errors via Toast. Background
  // failures (polling / idle sync / background refetches) stay silent so a
  // 700ms poll loop cannot spam toasts; their state keeps advancing on retry.
  const pushToast = toast.push
  const reportError = useCallback(
    (e: unknown) => {
      const f = friendlyError(e)
      pushToast({ tone: 'error', title: f.title, detail: f.detail })
    },
    [pushToast],
  )

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
    } catch {
      // Background list refresh (also runs on the 2s idle sync): stay silent,
      // the next tick retries. Never toast here to avoid notification spam.
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
      } catch {
        // Background refetch when a live run ends: stay silent; the idle sync
        // and conversation switch effects will reload messages on retry.
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
        } catch {
          // Transient 700ms poll failure: stay silent and retry next tick.
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
          setStatus(CHAT.reconnecting)
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
    // One cancel flag for every mount-time fetch: under StrictMode dev double
    // mount, the first effect's stale responses must not fire setState/toasts
    // (in particular a duplicated model-stale-fallback toast).
    let cancelled = false
    void getUIConfig()
      .then((cfg) => {
        if (cancelled) return
        if (cfg.agent_id) setAgentId(cfg.agent_id)
        setSupportsVision(cfg.supports_vision !== false)
      })
      .catch(() => {
        /* keep fallback */
      })
    // Skills drive the Composer @-completion popup. GET /v0/skills is operator
    // readable; load failures just disable the popup rather than blocking chat.
    void listSkills()
      .then((res) => {
        if (!cancelled) setSkills(res.skills ?? [])
      })
      .catch(() => {
        if (!cancelled) setSkills([])
      })
    // Model profiles feed the model chip. GET /v0/settings/models is operator
    // readable; a failure (or an empty list) shows the role-appropriate empty
    // state and falls back to the default model without blocking chat.
    void listModelProfiles()
      .then((list) => {
        if (cancelled) return
        const profiles = Array.isArray(list) ? list : []
        setModelProfiles(profiles)
        // A persisted concrete choice whose model was deleted falls back to
        // Auto; persist the fallback and tell the user once.
        const resolved = resolveModelChoice(profiles)
        if (resolved.stale) {
          setSelectedModelId(AUTO_MODEL_ID)
          saveModelChoice(AUTO_MODEL_ID)
          toast.push({ tone: 'info', title: CHAT.modelStaleFallback })
        }
      })
      .catch(() => {
        if (!cancelled) setModelProfiles([])
      })
    // Tool catalog powers friendly tool names in cards. Failures degrade
    // silently: cards then show the raw tool technical names.
    void listTools()
      .then((tools: ToolInfo[]) => {
        if (cancelled) return
        setToolCatalog(
          tools.map((t) => ({ name: t.name, title: t.title, description: t.description })),
        )
      })
      .catch(() => { /* catalog missing is non-blocking */ })
    return () => {
      cancelled = true
    }
    // toast.push is identity-stable (useCallback); effect runs once on mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
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
    setHistoryBlocks({})
    fetchedRunsRef.current = new Set()
    lastEventIndexRef.current = -1
    setBusy(false)
    setStatus('')

    const id = conversationId
    void (async () => {
      try {
        const msgs = await listMessages(id)
        if (cancelled || conversationIdRef.current !== id) return
        setMessages(msgs)
        requestAnimationFrame(() => scrollToBottom('auto'))
        await restoreLiveRun(id, msgs)
      } catch {
        // Background conversation load: a transient failure stays silent;
        // the idle sync retries listMessages every 2s.
      }
    })()

    return () => {
      cancelled = true
      stopStream()
      stopPoll()
    }
  }, [conversationId, restoreLiveRun, scrollToBottom, stopPoll, stopStream])

  // Idle sync: weixin (and other external) inbound turns append messages / create
  // runs without this tab knowing. Poll while a conversation is open so /ui
  // picks them up without a manual refresh.
  useEffect(() => {
    const id = conversationId
    let cancelled = false
    let fingerprint = ''

    const tick = async () => {
      if (cancelled || conversationIdRef.current !== id) return
      if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
        return
      }
      try {
        await refreshConversations()
        const msgs = await listMessages(id)
        if (cancelled || conversationIdRef.current !== id) return
        const next =
          msgs.length === 0
            ? '0'
            : `${msgs.length}:${msgs[msgs.length - 1]?.id ?? ''}:${msgs[msgs.length - 1]?.run_id ?? ''}`
        if (next !== fingerprint) {
          fingerprint = next
          setMessages(msgs)
        }
        if (!liveRunIdRef.current) {
          await restoreLiveRun(id, msgs)
        }
      } catch {
        /* ignore transient poll errors */
      }
    }

    const timer = window.setInterval(() => {
      void tick()
    }, IDLE_SYNC_MS)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [conversationId, refreshConversations, restoreLiveRun])

  // Load historical artifacts for completed turns (refresh / reopen):
  // for each not-yet-fetched run we list events ONCE and derive both analysis
  // page previews and read-only tool/workflow blocks. A run with events but no
  // analysis page must still produce tool blocks (M2: no early return on pages).
  useEffect(() => {
    const runIds = [
      ...new Set(
        messages
          .filter((m) => m.role === 'assistant' && m.run_id)
          .map((m) => m.run_id as string),
      ),
    ]
    const pending = runIds.filter((runId) => !fetchedRunsRef.current.has(runId))
    if (pending.length === 0) return
    let cancelled = false
    // Runs still in flight when the effect re-runs (also covers StrictMode's
    // mount/cleanup/mount double invoke) must lose their fetched marker so the
    // next effect run fetches them again; completed runs keep theirs.
    const inFlight = new Set(pending)
    void (async () => {
      await Promise.all(
        pending.map(async (runId) => {
          // Mark first so concurrent effect re-runs never double-fetch.
          fetchedRunsRef.current.add(runId)
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
            const blocks = foldToolBlocks(runId, events)
            if (blocks.length > 0) {
              setHistoryBlocks((prev) =>
                prev[runId] ? prev : { ...prev, [runId]: blocks },
              )
            }
            inFlight.delete(runId)
          } catch {
            inFlight.delete(runId)
            // A run that cannot be listed (deleted, transient error) may become
            // fetchable later; drop the marker so a future effect retries once.
            if (!cancelled) fetchedRunsRef.current.delete(runId)
          }
        }),
      )
    })()
    return () => {
      cancelled = true
      inFlight.forEach((runId) => fetchedRunsRef.current.delete(runId))
    }
  }, [messages, mergeHistoryPages])

  const onNewChat = () => {
    stopStream()
    stopPoll()
    setLiveRunId(null)
    setLiveEvents([])
    setHistoryPages({})
    setHistoryBlocks({})
    fetchedRunsRef.current = new Set()
    setMessages([])
    setBusy(false)
    setStatus('')
    setComposerDraft(undefined)
    setConversationId(newConversationId())
  }

  const onSelectConversation = (id: string) => {
    if (id === conversationId) return
    setConversationId(id)
  }

  // The sidebar ✕ (and any future entry point) only opens the confirm dialog;
  // the actual deletion happens in performDelete after explicit confirmation.
  const onDeleteConversation = (id: string) => {
    setConfirmDelete(id)
  }

  const performDelete = async () => {
    const id = confirmDelete
    if (!id) return
    setConfirmDelete(null)
    try {
      await deleteConversation(id)
    } catch (e) {
      // friendlyError maps conversation_busy etc. to a human message.
      reportError(e)
      return
    }
    toast.push({ tone: 'success', title: CHAT.deleteSuccess })
    // If the deleted conversation is the one open, reset to a fresh chat.
    if (id === conversationId) {
      stopStream()
      stopPoll()
      setLiveRunId(null)
      setLiveEvents([])
      setHistoryPages({})
      setHistoryBlocks({})
      fetchedRunsRef.current = new Set()
      setMessages([])
      setBusy(false)
      setStatus('')
      setComposerDraft(undefined)
      setConversationId(newConversationId())
    }
    await refreshConversations()
  }

  // Returns false when the message is rejected up-front (no model / vision
  // gate / attachment failure) so Composer keeps the draft text and files;
  // true once the message has been accepted (optimistic row appended), even if
  // the run creation fails afterwards.
  const onSend = async (text: string, files: File[]): Promise<boolean> => {
    const sentConversationId = conversationId

    // No model configured: block up-front. Guidance is role-specific so an
    // operator is never pointed at the admin-only model settings page.
    if (modelProfiles.length === 0) {
      setBusy(false)
      setStatus('')
      toast.push({
        tone: 'error',
        title: role === 'admin' ? CHAT.noModelAdmin : CHAT.noModelOperator,
      })
      return false
    }

    setBusy(true)
    setStatus('发送中…')

    // Build attachments from selected files. Image attachments are gated by
    // the active model choice: Auto routes to a vision model when available,
    // while a manual pick is honored exactly (a text-only manual choice on an
    // image turn is rejected up-front rather than silently rerouted).
    let attachments: Attachment[] | undefined
    if (files.length > 0) {
      try {
        const built = await Promise.all(files.map((f) => fileToAttachment(f)))
        const gate = visionGate(
          modelProfiles,
          selectedModelId,
          built.some((a) => isImageAttachment(a.media_type)),
          supportsVision,
        )
        if (!gate.allowed) {
          setBusy(false)
          setStatus('')
          // Image capability mismatch is shown as a modal, not a toast, so the
          // user keeps their draft/attachments and the message is not rerouted.
          // Operators without settings access get contact-admin guidance.
          setVisionWarning(
            role === 'admin'
              ? (gate.message ?? CHAT.visionWarningFallback)
              : CHAT.visionBlockedOperator,
          )
          return false
        }
        attachments = built
      } catch (err) {
        setBusy(false)
        setStatus('')
        reportError(err)
        return false
      }
    }

    // Gates passed: the message is accepted. Only now drop the rollback draft
    // source so a rejected send keeps the composer contents.
    setComposerDraft(undefined)

    // Optimistic bubble renders exactly what was sent: typed text plus inline
    // image / file-card previews backed by transient blob: URLs (no redundant
    // "（附件：…）" note). The server version replaces it on run end.
    const preview = buildLocalPreview(text, files)
    const userBubble = preview.content

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
      const runOptions = buildRunOptions(selectedModelId, {
        attachments,
      })
      const created = await createRun(agentId, text, sentConversationId, runOptions)
      // Model choice is persisted via onChooseModel; do not reset after send.
      await refreshConversations()
      // The conversation switched mid-flight: the message was already accepted,
      // so report acceptance even though this view no longer tracks the run.
      if (conversationIdRef.current !== sentConversationId) return true
      setStatus(statusLabel(created.status))
      setLiveRunId(created.run_id)
      startStream(created.run_id, sentConversationId, -1)
    } catch (err) {
      // Post-acceptance failure (vision_unsupported / 5xx / network): the
      // Composer still clears; reconcile messages with the server below.
      if (conversationIdRef.current !== sentConversationId) return true
      setBusy(false)
      setLiveRunId(null)
      // friendlyError maps vision_unsupported / 5xx / network to human titles.
      reportError(err)
      setStatus('')
      try {
        const msgs = await listMessages(sentConversationId)
        if (conversationIdRef.current === sentConversationId) setMessages(msgs)
      } catch {
        /* keep optimistic row */
      }
    }
    return true
  }

  const onRollbackUser = async (m: ChatMessage) => {
    if (busy || liveRunId || historyMutating) return
    setHistoryMutating(true)
    try {
      const res = await rollbackMessages(conversationId, m.id)
      setMessages(res.messages)
      setComposerDraft(m.content)
      await refreshConversations()
    } catch (e) {
      reportError(e)
    } finally {
      setHistoryMutating(false)
    }
  }

  const onRegenerate = async (m: ChatMessage) => {
    if (busy || liveRunId || historyMutating) return
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
    } catch (e) {
      setBusy(false)
      reportError(e)
    } finally {
      setHistoryMutating(false)
    }
  }

  const onRollbackTo = async (m: ChatMessage) => {
    if (busy || liveRunId || historyMutating) return
    setHistoryMutating(true)
    try {
      const res = await rollbackMessages(conversationId, m.id)
      setMessages(res.messages)
      setComposerDraft(undefined)
      await refreshConversations()
    } catch (e) {
      reportError(e)
    } finally {
      setHistoryMutating(false)
    }
  }

  const onFork = async (m: ChatMessage) => {
    if (busy || liveRunId || historyMutating) return
    setHistoryMutating(true)
    try {
      const res = await forkConversation(conversationId, m.id)
      setConversationId(res.conversation_id)
      setMessages(res.messages)
      setComposerDraft(undefined)
      await refreshConversations()
    } catch (e) {
      reportError(e)
    } finally {
      setHistoryMutating(false)
    }
  }

  const onCancelRun = async () => {
    if (!liveRunId) return
    setStatus('正在取消…')
    try {
      await cancelRun(liveRunId)
      await finishLiveRun(conversationId)
      setStatus('已取消')
    } catch (e) {
      reportError(e)
      try {
        await finishLiveRun(conversationId)
      } catch {
        setBusy(false)
        setLiveRunId(null)
      }
    }
  }

  // Model choice persists immediately (localStorage); it survives sends and
  // reloads, and falls back to Auto when it no longer resolves (see mount effect).
  const onChooseModel = (id: string) => {
    setSelectedModelId(id)
    saveModelChoice(id)
  }

  // Copy with a legacy fallback for non-secure (HTTP/LAN) contexts without
  // navigator.clipboard. The hidden textarea is removed synchronously so it
  // never disturbs page focus.
  async function copyText(text: string): Promise<boolean> {
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(text)
        return true
      }
    } catch {
      /* fall through to legacy path */
    }
    const ta = document.createElement('textarea')
    try {
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      return document.execCommand('copy')
    } catch {
      return false
    } finally {
      // Removed even if select()/execCommand() throws so no hidden node lingers.
      ta.remove()
    }
  }

  const onCopyMessage = (m: ChatMessage) => {
    void copyText(m.content).then((ok) => {
      toast.push(
        ok
          ? { tone: 'success', title: CHAT.copySuccess }
          : { tone: 'error', title: CHAT.copyFailed },
      )
    })
  }

  // Low-frequency actions live in the "more" menu. system_note additionally
  // exposes "roll back to here" at the top; it never gets a copy action.
  const buildMessageMenuItems = (m: ChatMessage): MenuItem[] => {
    const items: MenuItem[] = []
    if (m.role === 'system_note') {
      items.push({
        id: 'rollback-here',
        label: ACTIONS.rollbackHere,
        icon: <CornerUpLeft size={15} aria-hidden />,
        onSelect: () => void onRollbackTo(m),
      })
    }
    items.push({
      id: 'fork',
      label: ACTIONS.forkAsNew,
      icon: <GitBranch size={15} aria-hidden />,
      onSelect: () => void onFork(m),
    })
    return items
  }

  const liveBlocks: ChatBlock[] = liveRunId ? foldEvents(liveRunId, liveEvents) : []
  const showWelcome = messages.length === 0 && liveBlocks.length === 0
  const composerDisabled = busy || historyMutating
  const showStop = Boolean(liveRunId && busy)

  return (
    <div className={`chat-shell app-with-drawer${drawer.isOpen ? ' drawer-open' : ''}`}>
      <button
        type="button"
        className="app-drawer-scrim"
        aria-label="关闭菜单"
        onClick={drawer.close}
      />
      <aside className="chat-sidebar" aria-label="对话列表">
        <div className="chat-sidebar-top">
          <button
            type="button"
            className="btn ghost sidebar-new"
            onClick={() => {
              onNewChat()
              drawer.close()
            }}
          >
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
              <li key={c.id} className="conversation-row">
                <button
                  type="button"
                  className={
                    c.id === conversationId
                      ? 'conversation-item active'
                      : 'conversation-item'
                  }
                  onClick={() => {
                    onSelectConversation(c.id)
                    drawer.close()
                  }}
                >
                  {conversationListLabel(c.id, c.title)}
                </button>
                <button
                  type="button"
                  className="conversation-delete"
                  title="删除对话"
                  aria-label={`删除对话 ${conversationListLabel(c.id, c.title)}`}
                  onClick={(e) => {
                    e.stopPropagation()
                    onDeleteConversation(c.id)
                  }}
                >
                  ✕
                </button>
              </li>
            ))}
          </ul>
        </div>
        <div className="chat-sidebar-bottom">
          <ThemeToggle />
          <Link
            to={role === 'admin' ? '/settings' : '/settings/identities'}
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
        <SidebarResizer />
      </aside>

      <main className="chat-main">
        <div className="app-mobile-bar">
          <button
            type="button"
            className="app-menu-btn"
            aria-label="打开对话列表"
            onClick={drawer.open}
          >
            <Menu size={20} aria-hidden="true" />
          </button>
        </div>
        <div
          className="messages"
          aria-live="polite"
          ref={scrollerRef}
          onScroll={onScroll}
        >
          <div className="messages-inner">
            {showWelcome && (
              <div className="welcome">
                <p className="welcome-title">{WELCOME.title}</p>
                <p className="welcome-sub">{WELCOME.subtitle}</p>
              </div>
            )}
            {messages.map((m, msgIndex) => {
              const bubbleClass =
                m.role === 'user' ? 'user' : m.role === 'system_note' ? 'system' : 'assistant'
              const persisted = !m.id.startsWith('local_')
              const runHistoryBlocks =
                m.role === 'assistant' &&
                m.run_id &&
                isFirstAssistantMessageOfRun(msgIndex, messages) &&
                historyBlocks[m.run_id]
              // 分析页产物源自工具结果：该 run 只要有历史工具块（统一在首条
              // assistant 消息处渲染），所有 assistant 消息都不再独立出预览，
              // 避免同一 run 多条 assistant 消息时重复 iframe。
              const pages =
                m.role === 'assistant' &&
                m.run_id &&
                m.run_id !== liveRunId &&
                !historyBlocks[m.run_id]
                  ? historyPages[m.run_id] ?? []
                  : []
              const canAct = persisted && !busy && !liveRunId && !historyMutating
              return (
                <div key={m.id} className={`msg-row ${bubbleClass}`}>
                  {runHistoryBlocks && (
                    <div className="msg-history-blocks" data-testid="history-blocks">
                      {runHistoryBlocks.map((b, i) =>
                        b.kind === 'tool' ? (
                          <ToolCard key={`h-${i}`} block={b} catalog={toolCatalog} readOnly />
                        ) : (
                          <WorkflowCard key={`h-${i}`} block={b} />
                        ),
                      )}
                    </div>
                  )}
                  <div className={`msg ${bubbleClass}`}>
                    {m.role === 'assistant' ? (
                      <MarkdownText text={m.content} />
                    ) : (
                      <UserBubble content={m.content} />
                    )}
                  </div>
                  {pages.length > 0 && (
                    <div className="msg-analysis-pages">
                      {pages.map((url) => (
                        <AnalysisPagePreview key={url} artifactUrl={url} />
                      ))}
                    </div>
                  )}
                  {canAct && (
                    <div className="msg-actions">
                      {m.role === 'user' && (
                        <>
                          <button type="button" className="btn ghost sm" onClick={() => void onRollbackUser(m)}>
                            {ACTIONS.editAndReanswer}
                          </button>
                          <button type="button" className="btn ghost sm" onClick={() => onCopyMessage(m)}>
                            {ACTIONS.copy}
                          </button>
                        </>
                      )}
                      {m.role === 'assistant' && (
                        <>
                          <button type="button" className="btn ghost sm" onClick={() => void onRegenerate(m)}>
                            {ACTIONS.regenerate}
                          </button>
                          <button type="button" className="btn ghost sm" onClick={() => onCopyMessage(m)}>
                            {ACTIONS.copy}
                          </button>
                        </>
                      )}
                      <DropdownMenu
                        triggerLabel={ACTIONS.more}
                        items={buildMessageMenuItems(m)}
                      />
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
                      <ToolCard block={block} catalog={toolCatalog} onError={reportError} />
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
          {(status || showStop) && (
            <div className="chat-status-row">
              <p className="status">{status}</p>
              {showStop && (
                <button type="button" className="btn danger sm" onClick={() => void onCancelRun()}>
                  停止
                </button>
              )}
            </div>
          )}

          <Composer
            disabled={composerDisabled}
            draft={composerDraft}
            skills={skills}
            onSend={onSend}
            toolbar={
              modelProfiles.length > 0 ? (
                <ModelChip
                  profiles={modelProfiles}
                  value={selectedModelId}
                  onChange={onChooseModel}
                  disabled={composerDisabled}
                />
              ) : role === 'admin' ? (
                <Link to="/settings/models" className="model-chip model-chip-empty">
                  {CHAT.addModel}
                </Link>
              ) : (
                <span className="model-chip model-chip-empty" aria-disabled="true">
                  {CHAT.noModelConfigured}
                </span>
              )
            }
          />
        </div>
      </main>

      <ConfirmDialog
        open={confirmDelete !== null}
        danger
        title={CHAT.deleteTitle}
        body={CHAT.deleteBody}
        confirmText={CHAT.deleteConfirm}
        onConfirm={() => void performDelete()}
        onCancel={() => setConfirmDelete(null)}
      />
      <Modal
        open={visionWarning !== null}
        title={CHAT.visionWarningTitle}
        onClose={() => setVisionWarning(null)}
        footer={
          <Button variant="primary" onClick={() => setVisionWarning(null)}>
            {CHAT.visionWarningAck}
          </Button>
        }
      >
        <p className="vision-warning-body">{visionWarning}</p>
      </Modal>
      <ToastRegion toasts={toast.toasts} onDismiss={toast.dismiss} />
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
    case 'rejected':
      return '已停止'
    default:
      return status
  }
}
