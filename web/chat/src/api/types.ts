export type RunStatus =
  | 'queued'
  | 'running'
  | 'waiting_human'
  | 'succeeded'
  | 'failed'
  | 'cancelled'
  | 'rejected'

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
  conversation_id?: string
}

export interface ResumeResponse {
  run_id: string
  status: RunStatus
}

export interface IdentityView {
  id: string
  label: string
  scheme?: string
  source: string
  claims_summary?: Record<string, unknown>
  is_default: boolean
  last_used_at?: string
}

export interface Attachment {
  filename: string
  media_type: string
  content_base64: string
}

/** Default thinking intensity for a model profile. */
export type ThinkingLevel = 'off' | 'low' | 'medium' | 'high'

export interface ChatMessage {
  id: string
  conversation_id: string
  role: 'user' | 'assistant' | 'system_note'
  content: string
  run_id?: string
  created_at: string
  /** Persisted model thinking for this assistant turn (optional). */
  thinking?: string
  thinking_redacted?: boolean
}
