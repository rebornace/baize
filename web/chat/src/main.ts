import './style.css'
import {
  createRun,
  getRun,
  isTerminal,
  listEvents,
  resumeRun,
  type Event,
  type Run,
  type RunStatus,
} from './api'

const AGENT_ID = 'ticket-agent'
const POLL_MS = 700

type ChatItem =
  | { kind: 'user'; text: string }
  | { kind: 'assistant'; text: string }
  | { kind: 'system'; text: string }
  | { kind: 'tool'; text: string }
  | { kind: 'hitl'; event: Event; runId: string }

const app = document.querySelector<HTMLDivElement>('#app')!
app.innerHTML = `
  <div class="shell">
    <header class="header">
      <h1>Baize 对话</h1>
      <button type="button" id="btn-new" class="btn ghost">新对话</button>
    </header>
    <main id="messages" class="messages" aria-live="polite"></main>
    <div id="hitl-slot" class="hitl-slot"></div>
    <footer class="composer">
      <textarea id="input" rows="2" placeholder="输入消息，例如：创建一个工单，标题 VPN 故障"></textarea>
      <button type="button" id="btn-send" class="btn primary">发送</button>
    </footer>
    <p id="status" class="status"></p>
  </div>
`

const messagesEl = document.querySelector<HTMLElement>('#messages')!
const hitlSlot = document.querySelector<HTMLElement>('#hitl-slot')!
const inputEl = document.querySelector<HTMLTextAreaElement>('#input')!
const btnSend = document.querySelector<HTMLButtonElement>('#btn-send')!
const btnNew = document.querySelector<HTMLButtonElement>('#btn-new')!
const statusEl = document.querySelector<HTMLElement>('#status')!

let items: ChatItem[] = []
let pollTimer: number | null = null
let busy = false
let renderedEventCount = 0
let hitlDecisionPending = false

function setStatus(text: string) {
  statusEl.textContent = text
}

function setBusy(v: boolean) {
  busy = v
  btnSend.disabled = v
  inputEl.disabled = v
}

function stopPoll() {
  if (pollTimer !== null) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
}

function resetChat() {
  stopPoll()
  items = []
  renderedEventCount = 0
  hitlDecisionPending = false
  hitlSlot.innerHTML = ''
  setBusy(false)
  setStatus('')
  render()
  inputEl.focus()
}

function render() {
  messagesEl.innerHTML = ''
  for (const item of items) {
    const el = document.createElement('div')
    el.className = `bubble ${item.kind}`
    if (item.kind === 'hitl') {
      el.textContent = formatHitlSummary(item.event)
    } else {
      el.textContent = item.text
    }
    messagesEl.appendChild(el)
  }
  messagesEl.scrollTop = messagesEl.scrollHeight
}

function formatHitlSummary(ev: Event): string {
  const data = ev.data ?? {}
  const tool = String(data.tool_name ?? data.name ?? '工具')
  const prompt = String(data.prompt ?? '需要人工审批')
  return `⏳ 待审批：${tool}\n${prompt}`
}

function statusLabel(s: RunStatus): string {
  switch (s) {
    case 'queued':
      return '排队中'
    case 'running':
      return '运行中'
    case 'waiting_human':
      return '等待审批'
    case 'succeeded':
      return '已完成'
    case 'failed':
      return '失败'
    default: {
      const _exhaustive: never = s
      return String(_exhaustive)
    }
  }
}

function appendEvents(events: Event[], runId: string) {
  const fresh = events.slice(renderedEventCount)
  renderedEventCount = events.length
  for (const ev of fresh) {
    switch (ev.type) {
      case 'llm.message': {
        const content = String(ev.data?.content ?? '')
        if (content) items.push({ kind: 'assistant', text: content })
        break
      }
      case 'llm.tool_call': {
        const name = String(ev.data?.name ?? 'tool')
        const args = JSON.stringify(ev.data?.arguments ?? {}, null, 0)
        items.push({ kind: 'tool', text: `调用工具 ${name}：${args}` })
        break
      }
      case 'tool.result': {
        const name = String(ev.data?.name ?? 'tool')
        const content = JSON.stringify(ev.data?.content ?? {}, null, 0)
        const err = ev.data?.is_error ? '（错误）' : ''
        items.push({ kind: 'tool', text: `工具结果 ${name}${err}：${content}` })
        break
      }
      case 'hitl.waiting':
        items.push({ kind: 'hitl', event: ev, runId })
        break
      case 'hitl.resumed':
        items.push({
          kind: 'system',
          text: `已批准${ev.data?.comment ? `：${String(ev.data.comment)}` : ''}`,
        })
        break
      case 'hitl.rejected':
        items.push({
          kind: 'system',
          text: `已驳回${ev.data?.comment ? `：${String(ev.data.comment)}` : ''}`,
        })
        break
      case 'llm.error':
        items.push({ kind: 'system', text: `错误：${String(ev.data?.error ?? '未知')}` })
        break
      case 'run.started':
        break
      default:
        break
    }
  }
  render()
}

function showHitlCard(runId: string, events: Event[]) {
  const waiting = [...events].reverse().find((e) => e.type === 'hitl.waiting')
  if (!waiting) {
    hitlSlot.innerHTML = ''
    return
  }
  const data = waiting.data ?? {}
  const tool = String(data.tool_name ?? '工具')
  const prompt = String(data.prompt ?? '请审批此工具调用')
  const args = JSON.stringify(data.arguments ?? {}, null, 2)

  hitlSlot.innerHTML = `
    <div class="hitl-card">
      <h2>需要审批</h2>
      <p class="hitl-prompt">${escapeHtml(prompt)}</p>
      <p class="hitl-meta"><strong>工具：</strong>${escapeHtml(tool)}</p>
      <pre class="hitl-args">${escapeHtml(args)}</pre>
      <label class="hitl-comment">
        备注（可选）
        <input type="text" id="hitl-comment" placeholder="审批备注" />
      </label>
      <div class="hitl-actions">
        <button type="button" class="btn primary" id="btn-approve">批准</button>
        <button type="button" class="btn danger" id="btn-reject">驳回</button>
      </div>
    </div>
  `

  const commentEl = document.querySelector<HTMLInputElement>('#hitl-comment')!
  document.querySelector('#btn-approve')!.addEventListener('click', () => {
    void decide(runId, 'approve', commentEl.value.trim())
  })
  document.querySelector('#btn-reject')!.addEventListener('click', () => {
    void decide(runId, 'reject', commentEl.value.trim())
  })
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

async function decide(runId: string, decision: 'approve' | 'reject', comment: string) {
  if (hitlDecisionPending) return
  hitlDecisionPending = true
  try {
    setStatus(decision === 'approve' ? '正在提交批准…' : '正在提交驳回…')
    await resumeRun(runId, decision, comment)
    hitlSlot.innerHTML = ''
    setStatus('已提交决策，继续运行…')
    startPoll(runId)
  } catch (err) {
    setStatus(err instanceof Error ? err.message : String(err))
  } finally {
    hitlDecisionPending = false
  }
}

async function refreshRun(runId: string): Promise<Run> {
  const [run, events] = await Promise.all([getRun(runId), listEvents(runId)])
  appendEvents(events, runId)
  setStatus(`状态：${statusLabel(run.status)}`)

  if (run.status === 'waiting_human') {
    showHitlCard(runId, events)
    setBusy(false)
  } else {
    hitlSlot.innerHTML = ''
  }

  if (isTerminal(run.status)) {
    stopPoll()
    setBusy(false)
    if (run.status === 'succeeded' && run.output) {
      const already = items.some((i) => i.kind === 'assistant' && i.text === run.output)
      if (!already) {
        items.push({ kind: 'assistant', text: run.output })
        render()
      }
    }
    if (run.status === 'failed' && run.error) {
      items.push({ kind: 'system', text: `运行失败：${run.error}` })
      render()
    }
  }
  return run
}

function startPoll(runId: string) {
  stopPoll()
  void refreshRun(runId)
  pollTimer = window.setInterval(() => {
    void refreshRun(runId).catch((err) => {
      setStatus(err instanceof Error ? err.message : String(err))
    })
  }, POLL_MS)
}

async function sendMessage() {
  const text = inputEl.value.trim()
  if (!text || busy) return

  items.push({ kind: 'user', text })
  render()
  inputEl.value = ''
  setBusy(true)
  renderedEventCount = 0
  hitlSlot.innerHTML = ''
  setStatus('正在创建运行…')

  try {
    const created = await createRun(AGENT_ID, text)
    setStatus(`已创建运行 ${created.run_id}（${statusLabel(created.status)}）`)
    startPoll(created.run_id)
  } catch (err) {
    setBusy(false)
    setStatus(err instanceof Error ? err.message : String(err))
    items.push({ kind: 'system', text: '发送失败，请重试' })
    render()
  }
}

btnSend.addEventListener('click', () => {
  void sendMessage()
})
btnNew.addEventListener('click', () => {
  resetChat()
})
inputEl.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    void sendMessage()
  }
})

resetChat()
