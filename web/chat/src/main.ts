import './style.css'
import {
  clearIdentities,
  createRun,
  deleteIdentity,
  getRun,
  isTerminal,
  listEvents,
  listIdentities,
  listTools,
  resumeRun,
  setDefaultIdentity,
  getUIConfig,
  type Event,
  type IdentityView,
  type Run,
  type RunStatus,
  type ToolInfo,
} from './api'

const AGENT_ID_FALLBACK = 'ticket-agent'
let agentId = ''
const POLL_MS = 700
const CONV_KEY = 'baize.conversation_id'

const SENSITIVE_KEYS = new Set([
  'accesstoken',
  'access_token',
  'refreshtoken',
  'refresh_token',
  'idtoken',
  'id_token',
  'token',
  'password',
  'secret',
  'authorization',
])

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
    <aside id="accounts-panel" class="side-panel" aria-label="账号">
      <button type="button" id="accounts-toggle" class="side-toggle" aria-expanded="true">
        <span class="side-toggle-label">账号</span>
        <span class="side-toggle-hint">点击折叠</span>
      </button>
      <div id="accounts-body" class="side-body">
        <p class="side-loading">加载中…</p>
      </div>
    </aside>
    <aside id="tools-panel" class="side-panel tools-panel" aria-label="Tools">
      <button type="button" id="tools-toggle" class="side-toggle" aria-expanded="true">
        <span class="side-toggle-label">Tools</span>
        <span class="side-toggle-hint">点击折叠</span>
      </button>
      <div id="tools-body" class="side-body">
        <p class="side-loading">加载中…</p>
      </div>
    </aside>
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
const toolsPanel = document.querySelector<HTMLElement>('#tools-panel')!
const toolsBody = document.querySelector<HTMLElement>('#tools-body')!
const toolsToggle = document.querySelector<HTMLButtonElement>('#tools-toggle')!
const accountsPanel = document.querySelector<HTMLElement>('#accounts-panel')!
const accountsBody = document.querySelector<HTMLElement>('#accounts-body')!
const accountsToggle = document.querySelector<HTMLButtonElement>('#accounts-toggle')!

let items: ChatItem[] = []
let pollTimer: number | null = null
let busy = false
let renderedEventCount = 0
let hitlDecisionPending = false
let conversationId = loadConversationId()

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

function setConversationId(id: string) {
  conversationId = id
  localStorage.setItem(CONV_KEY, id)
}

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

function redactSensitive(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map(redactSensitive)
  }
  if (value && typeof value === 'object') {
    const out: Record<string, unknown> = {}
    for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
      if (SENSITIVE_KEYS.has(k.toLowerCase())) {
        out[k] = '[redacted]'
      } else {
        out[k] = redactSensitive(v)
      }
    }
    return out
  }
  return value
}

function formatToolResultContent(raw: unknown): string {
  return JSON.stringify(redactSensitive(raw ?? {}), null, 0)
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
        const content = formatToolResultContent(ev.data?.content)
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
    void loadAccountsPanel()
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

  const sentConversationId = conversationId
  items.push({ kind: 'user', text })
  render()
  inputEl.value = ''
  setBusy(true)
  renderedEventCount = 0
  hitlSlot.innerHTML = ''
  setStatus('正在创建运行…')

  try {
    if (!agentId) {
      throw new Error('未配置 agent_id，请确认 Runtime 已启动')
    }
    const created = await createRun(agentId, text, sentConversationId)
    // 新对话可能已切换 conversationId：忽略过期 createRun 结果，避免污染新会话。
    if (conversationId !== sentConversationId) {
      return
    }
    if (created.conversation_id) {
      setConversationId(created.conversation_id)
    }
    setStatus(`已创建运行 ${created.run_id}（${statusLabel(created.status)}）`)
    startPoll(created.run_id)
  } catch (err) {
    if (conversationId !== sentConversationId) {
      return
    }
    setBusy(false)
    setStatus(err instanceof Error ? err.message : String(err))
    items.push({ kind: 'system', text: '发送失败，请重试' })
    render()
  }
}

function formatToolLine(t: ToolInfo): string {
  const method = (t.method ?? '').toUpperCase() || '—'
  const path = t.path ?? '—'
  return `${method} ${path} — ${t.name} (${t.connector_id})`
}

function renderTools(tools: ToolInfo[]) {
  if (tools.length === 0) {
    toolsBody.innerHTML = `<p class="side-empty">暂无已注册 Tools</p>`
    return
  }
  const ul = document.createElement('ul')
  ul.className = 'side-list'
  for (const t of tools) {
    const li = document.createElement('li')
    li.className = 'side-item'
    li.textContent = formatToolLine(t)
    if (t.require_approval) {
      const badge = document.createElement('span')
      badge.className = 'side-badge'
      badge.textContent = '需审批'
      li.appendChild(document.createTextNode(' '))
      li.appendChild(badge)
    }
    ul.appendChild(li)
  }
  toolsBody.innerHTML = ''
  toolsBody.appendChild(ul)
}

function sourceLabel(source: string): string {
  switch (source) {
    case 'login_capture':
      return '登录捕获'
    case 'env':
      return '环境变量'
    case 'manual':
      return '手动'
    default:
      return source
  }
}

function renderAccounts(identities: IdentityView[]) {
  accountsBody.innerHTML = ''

  const meta = document.createElement('p')
  meta.className = 'accounts-meta'
  meta.textContent = `会话 ${conversationId}`
  accountsBody.appendChild(meta)

  if (identities.length === 0) {
    const empty = document.createElement('p')
    empty.className = 'side-empty'
    empty.textContent = '暂无已登录账号'
    accountsBody.appendChild(empty)
    return
  }

  const ul = document.createElement('ul')
  ul.className = 'side-list accounts-list'
  for (const idt of identities) {
    const li = document.createElement('li')
    li.className = 'accounts-item'

    const main = document.createElement('div')
    main.className = 'accounts-main'
    const title = document.createElement('div')
    title.className = 'accounts-title'
    title.textContent = idt.label || idt.id
    const detail = document.createElement('div')
    detail.className = 'accounts-detail'
    const scheme = idt.scheme ? idt.scheme : '—'
    detail.textContent = `${scheme} · ${sourceLabel(idt.source)}`
    main.appendChild(title)
    main.appendChild(detail)

    const actions = document.createElement('div')
    actions.className = 'accounts-actions'
    if (idt.is_default) {
      const badge = document.createElement('span')
      badge.className = 'side-badge accounts-default'
      badge.textContent = '默认'
      actions.appendChild(badge)
    } else {
      const btnDefault = document.createElement('button')
      btnDefault.type = 'button'
      btnDefault.className = 'btn ghost sm'
      btnDefault.textContent = '设为默认'
      btnDefault.addEventListener('click', () => {
        void (async () => {
          try {
            await setDefaultIdentity(conversationId, idt.id)
            await loadAccountsPanel()
          } catch (err) {
            setStatus(err instanceof Error ? err.message : String(err))
          }
        })()
      })
      actions.appendChild(btnDefault)
    }

    if (idt.source !== 'env') {
      const btnExit = document.createElement('button')
      btnExit.type = 'button'
      btnExit.className = 'btn ghost sm danger-text'
      btnExit.textContent = '退出'
      btnExit.addEventListener('click', () => {
        void (async () => {
          try {
            await deleteIdentity(conversationId, idt.id)
            await loadAccountsPanel()
          } catch (err) {
            setStatus(err instanceof Error ? err.message : String(err))
          }
        })()
      })
      actions.appendChild(btnExit)
    }

    li.appendChild(main)
    li.appendChild(actions)
    ul.appendChild(li)
  }
  accountsBody.appendChild(ul)

  const hasCaptured = identities.some((i) => i.source !== 'env')
  if (hasCaptured) {
    const clearBtn = document.createElement('button')
    clearBtn.type = 'button'
    clearBtn.className = 'btn ghost sm accounts-clear'
    clearBtn.textContent = '清空捕获账号'
    clearBtn.addEventListener('click', () => {
      void (async () => {
        try {
          await clearIdentities(conversationId)
          await loadAccountsPanel()
        } catch (err) {
          setStatus(err instanceof Error ? err.message : String(err))
        }
      })()
    })
    accountsBody.appendChild(clearBtn)
  }
}

async function loadToolsPanel() {
  try {
    const tools = await listTools()
    renderTools(tools)
  } catch {
    toolsBody.innerHTML = `<p class="side-error">无法加载 Tools</p>`
  }
}

async function loadAccountsPanel() {
  try {
    const identities = await listIdentities(conversationId)
    renderAccounts(identities)
  } catch {
    accountsBody.innerHTML = `<p class="side-error">无法加载账号</p>`
  }
}

function wireCollapse(
  panel: HTMLElement,
  toggle: HTMLButtonElement,
) {
  toggle.addEventListener('click', () => {
    const collapsed = panel.classList.toggle('collapsed')
    toggle.setAttribute('aria-expanded', collapsed ? 'false' : 'true')
    const hint = toggle.querySelector('.side-toggle-hint')
    if (hint) hint.textContent = collapsed ? '点击展开' : '点击折叠'
  })
}

wireCollapse(toolsPanel, toolsToggle)
wireCollapse(accountsPanel, accountsToggle)

btnSend.addEventListener('click', () => {
  void sendMessage()
})
btnNew.addEventListener('click', () => {
  setConversationId(newConversationId())
  resetChat()
  void loadAccountsPanel()
})
inputEl.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    void sendMessage()
  }
})

resetChat()
void loadUIConfig()
void loadToolsPanel()
void loadAccountsPanel()

async function loadUIConfig() {
  try {
    const cfg = await getUIConfig()
    agentId = (cfg.agent_id || '').trim() || AGENT_ID_FALLBACK
    setStatus(`已连接（agent: ${agentId}）`)
  } catch (err) {
    agentId = AGENT_ID_FALLBACK
    setStatus(`ui-config 不可用，回退 agent: ${agentId}（${String(err)}）`)
  }
}
