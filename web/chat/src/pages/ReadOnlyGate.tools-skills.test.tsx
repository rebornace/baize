// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { GateContext } from '../gateContext'
import { TOOLS } from '../strings'
import { ToolsSettings } from './ToolsSettings'
import { SkillsSettings } from './SkillsSettings'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}
let host: HTMLDivElement
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  vi.stubGlobal('fetch', vi.fn())
})
afterEach(() => { host.remove(); vi.unstubAllGlobals() })

async function renderOperator(el: React.ReactNode) {
  await act(async () => {
    createRoot(host).render(
      <MemoryRouter>
        <GateContext.Provider value={{ role: 'operator', gateEnabled: true, operatorId: 'op' }}>
          {el}
        </GateContext.Provider>
      </MemoryRouter>,
    )
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
}

describe('ToolsSettings read-only for operator', () => {
  it('lists tools but hides enable/login/export controls, add drawer and delete', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) => {
      const u = String(url)
      if (u === '/v0/tools') {
        return jsonResponse({ tools: [
          {
            name: 'list_tickets',
            title: '查工单',
            connector_id: 'oa1',
            source: 'spec',
            enabled: true,
            require_login: true,
            require_approval: true,
          },
        ] })
      }
      // connector detail GET is admin-only for operators
      return new Response('forbidden', { status: 403 })
    })
    await renderOperator(<ToolsSettings />)
    expect(host.textContent).toContain('助手功能')
    expect(host.querySelector('h1')?.textContent).not.toBe('Tools')
    expect(host.textContent).toContain('查工单')
    expect(host.textContent).toContain(TOOLS.statusEnabled)
    expect(host.textContent).toContain(TOOLS.requireLogin)
    expect(host.textContent).toContain(TOOLS.requireApprovalBadge)
    expect(host.textContent).not.toContain('全部启用')
    expect(host.textContent).not.toContain('全部停用')
    expect(host.textContent).not.toContain('添加')
    expect(host.textContent).not.toContain(TOOLS.editCopy)
    expect(host.textContent).not.toContain('MCP 导出')
    expect(host.textContent).not.toContain('删除')
    // strong DOM assertions: enable/require-login toggles and MCP export select are not rendered at all
    expect(host.querySelectorAll('input[type="checkbox"]').length).toBe(0)
    expect(host.querySelectorAll('select').length).toBe(0)
  })
})

describe('SkillsSettings read-only for operator', () => {
  it('lists skills but hides upload, save-default and delete', async () => {
    const urls: string[] = []
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) => {
      urls.push(String(url))
      const u = String(url)
      if (u === '/v0/skills') return jsonResponse({ skills: [
        // SkillsSettings renders s.id (name is not displayed); align fixture with real DOM
        { id: '分诊', name: '分诊', description: '', tools: [], source: 'builtin' },
        { id: '自定义', name: '自定义', description: '', tools: [], source: 'user' },
      ] })
      // Real ACL: GET /v0/agents/{id} is RoleAdmin (acl.go); operators get 403 and the
      // read-only skill list must still render (list load must not depend on getAgent).
      if (u.startsWith('/v0/agents/')) return new Response('forbidden', { status: 403 })
      // /v0/ui-config is RoleNone; an empty body makes agent_id fall back to ticket-agent
      return jsonResponse({})
    })
    await renderOperator(<SkillsSettings />)
    expect(host.textContent).toContain('技能')
    expect(host.textContent).toContain('分诊')
    expect(host.textContent).not.toMatch(/\bSkills\b/)
    expect(host.textContent).not.toMatch(/默认 Agent/)
    // real 403 on getAgent must NOT fail the whole page: skill list still renders
    expect(host.textContent).not.toContain('无法加载')
    // read-only path must not even request the admin-only agent config endpoint
    expect(urls.some((u) => u.startsWith('/v0/agents/'))).toBe(false)
    expect(host.querySelector('input[type="file"]')).toBeNull()
    expect(host.textContent).not.toContain('保存为默认技能')
    expect(host.textContent).not.toContain('保存默认勾选')
    // user-source skill shows a delete button for admins; it must be hidden for operators
    expect(host.textContent).not.toContain('删除')
    // zero checkboxes — operators have no getAgent data for default selection
    expect(host.querySelectorAll('input[type="checkbox"]').length).toBe(0)
  })
})
