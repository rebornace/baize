import {
  useCallback,
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
  refreshConversations: () => Promise<void>
}

export function useChatRunStream({
  conversationIdRef,
  setMessages,
  mergeHistoryPages,
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
  const liveEventsRef = useRef(liveEvents)
  liveEventsRef.current = liveEvents

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
    setLiveRunId(null)
    setLiveEvents([])
    lastEventIndexRef.current = -1
    setBusy(false)
    setStatus('')
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
    [conversationIdRef, mergeHistoryPages, refreshConversations, setMessages, stopPoll, stopStream],
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
    [conversationIdRef, finishLiveRun, startPoll, stopPoll, stopStream],
  )

  /** Subscribe to a run via the same SSE → poll fallback path as createRun. */
  const attachRun = useCallback(
    (runId: string, forConversationId: string, statusValue: string) => {
      setBusy(true)
      setLiveEvents([])
      lastEventIndexRef.current = -1
      setStatus(statusLabel(statusValue))
      startStream(runId, forConversationId, -1)
    },
    [startStream],
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
