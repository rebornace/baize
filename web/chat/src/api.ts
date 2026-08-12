export type RunStatus =
  | 'queued'
  | 'running'
  | 'waiting_human'
  | 'succeeded'
  | 'failed'

export interface Run {
  id: string
  agent_id: string
  input: string
  status: RunStatus
  output?: string
  error?: string
  created_at: string
}

export interface Event {
  type: string
  timestamp: string
  data?: Record<string, unknown>
}

export interface CreateRunResponse {
  run_id: string
  status: RunStatus
}

export interface ResumeResponse {
  run_id: string
  status: RunStatus
}

async function parseJSON<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let detail = res.statusText
    try {
      const body = (await res.json()) as { error?: { message?: string } }
      if (body.error?.message) detail = body.error.message
    } catch {
      /* ignore */
    }
    throw new Error(`HTTP ${res.status}: ${detail}`)
  }
  return (await res.json()) as T
}

export async function createRun(agentId: string, input: string): Promise<CreateRunResponse> {
  const res = await fetch('/v0/runs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ agent_id: agentId, input }),
  })
  return parseJSON<CreateRunResponse>(res)
}

export async function getRun(runId: string): Promise<Run> {
  const res = await fetch(`/v0/runs/${encodeURIComponent(runId)}`)
  return parseJSON<Run>(res)
}

export async function listEvents(runId: string): Promise<Event[]> {
  const res = await fetch(`/v0/runs/${encodeURIComponent(runId)}/events`)
  return parseJSON<Event[]>(res)
}

export async function resumeRun(
  runId: string,
  decision: 'approve' | 'reject',
  comment = '',
): Promise<ResumeResponse> {
  const res = await fetch(`/v0/runs/${encodeURIComponent(runId)}/resume`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ decision, comment }),
  })
  return parseJSON<ResumeResponse>(res)
}

export function isTerminal(status: RunStatus): boolean {
  return status === 'succeeded' || status === 'failed'
}
