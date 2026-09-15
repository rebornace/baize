import { consumeSSE } from '../parseSSE'
import { authInit, parseJSON } from './http'
import type {
  Attachment,
  CreateRunResponse,
  Event,
  ResumeResponse,
  Run,
  RunStatus,
  ThinkingLevel,
} from './types'

export interface CreateRunOptions {
  identityId?: string
  sessionToken?: string
  webhookUrl?: string
  webhookHeaders?: Record<string, string>
  skills?: string[]
  attachments?: Attachment[]
  modelProfileId?: string
  /** Chat per-conversation override; omit to use the model profile default. */
  thinkingLevel?: ThinkingLevel
}

export async function createRun(
  agentId: string,
  input: string,
  conversationId: string,
  options?: CreateRunOptions,
): Promise<CreateRunResponse> {
  const body: Record<string, unknown> = {
    agent_id: agentId,
    input,
    conversation_id: conversationId,
  }
  if (options?.identityId) body.identity_id = options.identityId
  if (options?.sessionToken) body.session_token = options.sessionToken
  if (options?.webhookUrl) body.webhook_url = options.webhookUrl
  if (options?.webhookHeaders && Object.keys(options.webhookHeaders).length > 0) {
    body.webhook_headers = options.webhookHeaders
  }
  if (options?.skills && options.skills.length > 0) {
    body.skills = options.skills
  }
  if (options?.attachments && options.attachments.length > 0) {
    body.attachments = options.attachments
  }
  if (options?.modelProfileId) body.model_profile_id = options.modelProfileId
  if (options?.thinkingLevel) body.thinking_level = options.thinkingLevel
  const res = await fetch('/v0/runs', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<CreateRunResponse>(res)
}

export async function getRun(runId: string): Promise<Run> {
  const res = await fetch(`/v0/runs/${encodeURIComponent(runId)}`, {
    headers: authInit(),
  })
  return parseJSON<Run>(res)
}

export async function listEvents(runId: string): Promise<Event[]> {
  const res = await fetch(`/v0/runs/${encodeURIComponent(runId)}/events`, {
    headers: authInit(),
  })
  return parseJSON<Event[]>(res)
}

export async function resumeRun(
  runId: string,
  decision: 'approve' | 'reject',
  comment = '',
): Promise<ResumeResponse> {
  const res = await fetch(`/v0/runs/${encodeURIComponent(runId)}/resume`, {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ decision, comment }),
  })
  return parseJSON<ResumeResponse>(res)
}

export async function cancelRun(runId: string): Promise<ResumeResponse> {
  const res = await fetch(`/v0/runs/${encodeURIComponent(runId)}/cancel`, {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<ResumeResponse>(res)
}

export function isTerminal(status: RunStatus): boolean {
  return (
    status === 'succeeded' ||
    status === 'failed' ||
    status === 'cancelled' ||
    status === 'rejected'
  )
}

export function openRunStream(
  runId: string,
  after: number,
  onEvent: (e: Event, index: number) => void,
  onEnded: (status: string) => void,
  onFatal: () => void,
): () => void {
  const url =
    `${window.location.origin}/v0/runs/${encodeURIComponent(runId)}/stream` +
    `?after=${after}`
  const ac = new AbortController()
  let closed = false

  const finish = (fn: () => void) => {
    if (closed) return
    closed = true
    ac.abort()
    fn()
  }

  void (async () => {
    try {
      const res = await fetch(url, {
        headers: authInit(),
        signal: ac.signal,
      })
      const contentType = res.headers.get('content-type') ?? ''
      if (!res.ok || !contentType.includes('event-stream') || !res.body) {
        finish(() => onFatal())
        return
      }
      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''
      while (!closed) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })
        const { frames, rest } = consumeSSE(buffer)
        buffer = rest
        for (const frame of frames) {
          if (closed) return
          if (frame.event === 'run.ended') {
            let status = ''
            try {
              status = (JSON.parse(frame.data) as { status?: string }).status ?? ''
            } catch {
              status = ''
            }
            finish(() => onEnded(status))
            return
          }
          let parsed: Event
          try {
            parsed = JSON.parse(frame.data) as Event
          } catch {
            continue
          }
          const index = Number.parseInt(frame.id, 10)
          onEvent(parsed, Number.isFinite(index) ? index : -1)
        }
      }
      if (!closed) finish(() => onFatal())
    } catch (err) {
      if (closed) return
      if (err instanceof DOMException && err.name === 'AbortError') return
      finish(() => onFatal())
    }
  })()

  return () => {
    if (closed) return
    closed = true
    ac.abort()
  }
}
