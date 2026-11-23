import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type MutableRefObject,
  type SetStateAction,
} from 'react'
import {
  getRun,
  isTerminal,
  listEvents,
  listMessages,
  openRunStream,
  type ChatMessage,
  type Event,
  type RunStatus,
} from '../../api'
import { extractAnalysisPagesFromEvents } from '../../analysisPage'
import { findLiveRunCandidate, isActiveRunStatus } from '../../findLiveRun'
import type { UsageMeta } from '../../foldEvents'
import {
  foldToolBlocks,
  runUsageFromEvents,
  type ToolOrWorkflowBlock,
} from '../../historyBlocks'
import { runDurationMsFromEvents } from '../../runDuration'
import { appendLiveEvent, coalesceLiveEvents } from '../../liveEventAppend'
import { commitLiveAssistant, handoffTranscript } from '../../liveCommit'
import { CHAT } from '../../strings'

export const POLL_MS = 700

export function statusLabel(status: string): string {
  switch (status) {
    case 'queued':
      return CHAT.statusQueued
    case 'running':
      return CHAT.statusRunning
    case 'waiting_human':
      return CHAT.statusWaitingHuman
    case 'succeeded':
      return CHAT.statusSucceeded
    case 'failed':
      return CHAT.statusFailed
    case 'cancelled':
      return CHAT.statusCancelled
    case 'rejected':
      return CHAT.statusRejected
    default:
      return status
  }
}

export type UseChatRunStreamArgs = {
  conversationIdRef: MutableRefObject<string>
  setMessages: Dispatch<SetStateAction<ChatMessage[]>>
  mergeHistoryPages: (runId: string, urls: string[]) => void
  setHistoryBlocks: Dispatch<SetStateAction<Record<string, ToolOrWorkflowBlock[]>>>
  setHistoryUsage: Dispatch<SetStateAction<Record<string, UsageMeta>>>
  setHistoryDurationMs: Dispatch<SetStateAction<Record<string, number>>>
  refreshConversations: () => Promise<void>
}

export function useChatRunStream({
  conversationIdRef,
  setMessages,
  mergeHistoryPages,
  setHistoryBlocks,
  setHistoryUsage,
  setHistoryDurationMs,
  refreshConversations,
}: UseChatRunStreamArgs) {
  const [liveEvents, setLiveEvents] = useState<Event[]>([])
  const [liveRunId, setLiveRunId] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState('')

  const cancelStreamRef = useRef<(() => void) | null>(null)
  const pollTimerRef = useRef<number | null>(null)
  const lastEventIndexRef = useRef(-1)
  const liveRunIdRef = useRef(liveRunId)
  liveRunIdRef.current = liveRunId
  const liveEventsRef = useRef<Event[]>([])
  const flushRafRef = useRef<number | null>(null)

  const cancelScheduledFlush = useCallback(() => {
    if (flushRafRef.current !== null) {
      window.cancelAnimationFrame(flushRafRef.current)
      flushRafRef.current = null
    }
  }, [])

  const writeLiveEvents = useCallback((next: Event[]) => {
    cancelScheduledFlush()
    liveEventsRef.current = next
    setLiveEvents(next)
  }, [cancelScheduledFlush])

  /** Coalesce token deltas and paint at most once per animation frame. */
  const enqueueLiveEvent = useCallback((ev: Event) => {
    liveEventsRef.current = appendLiveEvent(liveEventsRef.current, ev)
    if (flushRafRef.current !== null) return
    flushRafRef.current = window.requestAnimationFrame(() => {
      flushRafRef.current = null
      setLiveEvents(liveEventsRef.current)
    })
  }, [])

  useEffect(() => () => cancelScheduledFlush(), [cancelScheduledFlush])

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

  const resetLive = useCallback(() => {
    cancelScheduledFlush()
    liveEventsRef.current = []
    setLiveRunId(null)
    setLiveEvents([])
    lastEventIndexRef.current = -1
    setBusy(false)
    setStatus('')
  }, [cancelScheduledFlush])

  const finishLiveRun = useCallback(
    async (id: string) => {
      stopStream()
      stopPoll()
      const runId = liveRunIdRef.current
      const events = liveEventsRef.current
      if (runId) {
        const pages = extractAnalysisPagesFromEvents(events)
        if (pages.length > 0) {
          mergeHistoryPages(
            runId,
            pages.map((p) => p.artifactUrl),
          )
        }
        const blocks = foldToolBlocks(runId, events)
        if (blocks.length > 0) {
          setHistoryBlocks((prev) => (prev[runId] ? prev : { ...prev, [runId]: blocks }))
        }
        const usage = runUsageFromEvents(runId, events)
        if (usage) {
          setHistoryUsage((prev) => (prev[runId] ? prev : { ...prev, [runId]: usage }))
        }
        const durationMs = runDurationMsFromEvents(events)
        if (durationMs != null) {
          setHistoryDurationMs((prev) => (prev[runId] != null ? prev : { ...prev, [runId]: durationMs }))
        }
        // Do not mark fetched here: the session effect still lists events so
        // token usage that arrives after the last SSE snapshot is not skipped.
      }
      const dropLiveFold = () => {
        cancelScheduledFlush()
        liveEventsRef.current = []
        setLiveRunId(null)
        setLiveEvents([])
        lastEventIndexRef.current = -1
        setBusy(false)
      }
      // Keep the live fold on screen until the persisted (or snapshotted)
      // assistant is in `messages`. Clearing first leaves only the user bubble.
      try {
        const msgs = await listMessages(id)
        if (conversationIdRef.current !== id) return
        setMessages((prev) =>
          handoffTranscript(prev, msgs, { conversationId: id, runId, events }),
        )
        dropLiveFold()
      } catch {
        if (conversationIdRef.current !== id) return
        setMessages((prev) =>
          commitLiveAssistant(prev, { conversationId: id, runId, events }),
        )
        dropLiveFold()
      }
      await refreshConversations()
    },
    [
      cancelScheduledFlush,
      conversationIdRef,
      mergeHistoryPages,
      refreshConversations,
      setHistoryBlocks,
      setHistoryUsage,
      setHistoryDurationMs,
      setMessages,
      stopPoll,
      stopStream,
    ],
  )

  const applyEvents = useCallback((events: Event[]) => {
    writeLiveEvents(coalesceLiveEvents(events))
  }, [writeLiveEvents])

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
    [applyEvents, conversationIdRef, finishLiveRun, stopPoll],
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
          enqueueLiveEvent(ev)
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
    [conversationIdRef, enqueueLiveEvent, finishLiveRun, startPoll, stopPoll, stopStream],
  )

  /** Subscribe to a run via the same SSE → poll fallback path as createRun. */
  const attachRun = useCallback(
    (runId: string, forConversationId: string, statusValue: string) => {
      setBusy(true)
      writeLiveEvents([])
      lastEventIndexRef.current = -1
      setStatus(statusLabel(statusValue))
      startStream(runId, forConversationId, -1)
    },
    [startStream, writeLiveEvents],
  )

  const restoreLiveRun = useCallback(
    async (id: string, msgs: ChatMessage[]) => {
      const candidate = findLiveRunCandidate(msgs)
      if (!candidate) return
      const run = await getRun(candidate)
      if (conversationIdRef.current !== id) return
      if (!isActiveRunStatus(run.status)) return
      attachRun(candidate, id, run.status)
    },
    [attachRun, conversationIdRef],
  )

  return {
    liveEvents,
    setLiveEvents,
    liveRunId,
    setLiveRunId,
    busy,
    setBusy,
    status,
    setStatus,
    writeLiveEvents,
    lastEventIndexRef,
    liveRunIdRef,
    stopPoll,
    stopStream,
    resetLive,
    finishLiveRun,
    attachRun,
    restoreLiveRun,
  }
}
