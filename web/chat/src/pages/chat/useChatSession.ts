import { useCallback, useEffect, useRef, useState, type MutableRefObject } from 'react'
import {
  deleteConversation,
  forkConversation,
  listConversations,
  listEvents,
  rollbackMessages,
  type ChatMessage,
  type ConversationScope,
  type ConversationSummary,
} from '../../api'
import { extractAnalysisPagesFromEvents } from '../../analysisPage'
import { extractBlobURLs } from '../../localAttachments'
import { foldToolBlocks, type ToolOrWorkflowBlock } from '../../historyBlocks'
import { CHAT } from '../../strings'
import { uuid } from '../../uuid'

export const CONV_KEY = 'baize.conversation_id'
export const SCOPE_KEY = 'baize.conversation_scope'

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

/** Live-run controls owned by useChatRun; session reads via ref to avoid cycles. */
export type ChatLiveControls = {
  busy: boolean
  liveRunId: string | null
  setBusy: (v: boolean) => void
  stopStream: () => void
  stopPoll: () => void
  resetLive: () => void
  attachRun: (runId: string, forConversationId: string, status: string) => void
}

export type UseChatSessionArgs = {
  role: string
  reportError: (e: unknown) => void
  pushToast: (t: { tone: 'success' | 'error' | 'info'; title: string; detail?: string }) => void
  liveRef: MutableRefObject<ChatLiveControls>
  agentId: string
}

export function useChatSession({
  role,
  reportError,
  pushToast,
  liveRef,
  agentId,
}: UseChatSessionArgs) {
  const [conversationId, setConversationIdState] = useState(loadConversationId)
  const [conversationScope, setConversationScopeState] = useState<ConversationScope>(() =>
    loadConversationScope(role === 'admin'),
  )
  const [conversations, setConversations] = useState<ConversationSummary[]>([])
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [historyMutating, setHistoryMutating] = useState(false)
  const [composerDraft, setComposerDraft] = useState<string | undefined>(undefined)
  /** run_id → analysis page artifact URLs (kept after live run ends / on reload). */
  const [historyPages, setHistoryPages] = useState<Record<string, string[]>>({})
  /** run_id → folded historical tool/workflow blocks (read-only replay). */
  const [historyBlocks, setHistoryBlocks] = useState<Record<string, ToolOrWorkflowBlock[]>>({})
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null)

  /** runIds whose events have already been fetched once (drives both pages and blocks). */
  const fetchedRunsRef = useRef<Set<string>>(new Set())
  const conversationIdRef = useRef(conversationId)
  conversationIdRef.current = conversationId
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

  const setConversationId = useCallback((id: string) => {
    setConversationIdState(id)
    localStorage.setItem(CONV_KEY, id)
  }, [])

  const setConversationScope = useCallback((scope: ConversationScope) => {
    setConversationScopeState(scope)
    localStorage.setItem(SCOPE_KEY, scope)
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

  useEffect(() => {
    void refreshConversations()
  }, [conversationScope, refreshConversations])

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

  const clearHistoryState = useCallback(() => {
    setHistoryPages({})
    setHistoryBlocks({})
    fetchedRunsRef.current = new Set()
    setMessages([])
    setComposerDraft(undefined)
  }, [])

  const onNewChat = useCallback(() => {
    const live = liveRef.current
    live.stopStream()
    live.stopPoll()
    live.resetLive()
    clearHistoryState()
    setConversationId(newConversationId())
  }, [clearHistoryState, liveRef, setConversationId])

  const onSelectConversation = useCallback(
    (id: string) => {
      if (id === conversationIdRef.current) return
      setConversationId(id)
    },
    [setConversationId],
  )

  // The sidebar ✕ (and any future entry point) only opens the confirm dialog;
  // the actual deletion happens in performDelete after explicit confirmation.
  const onDeleteConversation = useCallback((id: string) => {
    setConfirmDelete(id)
  }, [])

  const performDelete = useCallback(async () => {
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
    pushToast({ tone: 'success', title: CHAT.deleteSuccess })
    // If the deleted conversation is the one open, reset to a fresh chat.
    if (id === conversationIdRef.current) {
      const live = liveRef.current
      live.stopStream()
      live.stopPoll()
      live.resetLive()
      clearHistoryState()
      setConversationId(newConversationId())
    }
    await refreshConversations()
  }, [
    clearHistoryState,
    confirmDelete,
    liveRef,
    pushToast,
    refreshConversations,
    reportError,
    setConversationId,
  ])

  const onRollbackUser = useCallback(
    async (m: ChatMessage) => {
      const live = liveRef.current
      if (live.busy || live.liveRunId || historyMutating) return
      setHistoryMutating(true)
      try {
        const res = await rollbackMessages(conversationIdRef.current, m.id)
        setMessages(res.messages)
        setComposerDraft(m.content)
        await refreshConversations()
      } catch (e) {
        reportError(e)
      } finally {
        setHistoryMutating(false)
      }
    },
    [historyMutating, liveRef, refreshConversations, reportError],
  )

  const onRegenerate = useCallback(
    async (m: ChatMessage) => {
      const live = liveRef.current
      if (live.busy || live.liveRunId || historyMutating) return
      live.setBusy(true)
      setHistoryMutating(true)
      try {
        const res = await rollbackMessages(conversationIdRef.current, m.id, {
          regenerate: true,
          agentId,
        })
        setMessages(res.messages)
        setComposerDraft(undefined)
        await refreshConversations()
        if (!res.regenerated_run) {
          liveRef.current.setBusy(false)
          return
        }
        const runId = res.regenerated_run.run_id
        liveRef.current.attachRun(runId, conversationIdRef.current, res.regenerated_run.status)
      } catch (e) {
        liveRef.current.setBusy(false)
        reportError(e)
      } finally {
        setHistoryMutating(false)
      }
    },
    [agentId, historyMutating, liveRef, refreshConversations, reportError],
  )

  const onRollbackTo = useCallback(
    async (m: ChatMessage) => {
      const live = liveRef.current
      if (live.busy || live.liveRunId || historyMutating) return
      setHistoryMutating(true)
      try {
        const res = await rollbackMessages(conversationIdRef.current, m.id)
        setMessages(res.messages)
        setComposerDraft(undefined)
        await refreshConversations()
      } catch (e) {
        reportError(e)
      } finally {
        setHistoryMutating(false)
      }
    },
    [historyMutating, liveRef, refreshConversations, reportError],
  )

  const onFork = useCallback(
    async (m: ChatMessage) => {
      const live = liveRef.current
      if (live.busy || live.liveRunId || historyMutating) return
      setHistoryMutating(true)
      try {
        const res = await forkConversation(conversationIdRef.current, m.id)
        setConversationId(res.conversation_id)
        setMessages(res.messages)
        setComposerDraft(undefined)
        await refreshConversations()
      } catch (e) {
        reportError(e)
      } finally {
        setHistoryMutating(false)
      }
    },
    [historyMutating, liveRef, refreshConversations, reportError, setConversationId],
  )

  return {
    conversationId,
    setConversationId,
    conversationScope,
    setConversationScope,
    conversations,
    messages,
    setMessages,
    historyMutating,
    composerDraft,
    setComposerDraft,
    historyPages,
    setHistoryPages,
    historyBlocks,
    setHistoryBlocks,
    confirmDelete,
    setConfirmDelete,
    fetchedRunsRef,
    conversationIdRef,
    conversationScopeRef,
    refreshConversations,
    mergeHistoryPages,
    clearHistoryState,
    onNewChat,
    onSelectConversation,
    onDeleteConversation,
    performDelete,
    onRollbackUser,
    onRegenerate,
    onRollbackTo,
    onFork,
  }
}
