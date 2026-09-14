// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { renderToStaticMarkup } from 'react-dom/server'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import {
  createModelProfile,
  deleteModelProfile,
  listModelProfiles,
  updateModelProfile,
  type ModelProfile,
} from '../api'
import { GateContext } from '../gateContext'
import { MODELS } from '../strings'
import {
  buildCreatePayload,
  buildPatchPayload,
  ModelProfileList,
  ModelSettings,
  profileToForm,
  type ProfileFormState,
} from './ModelSettings'

function setNativeValue(el: HTMLInputElement, value: string) {
  const proto = Object.getPrototypeOf(el)
  const desc = Object.getOwnPropertyDescriptor(proto, 'value')
  desc?.set?.call(el, value)
  el.dispatchEvent(new Event('input', { bubbles: true }))
}

const profile = (over: Partial<ModelProfile> & Pick<ModelProfile, 'id' | 'name'>): ModelProfile => ({
  provider: 'openai_compatible',
  base_url: 'https://api.example.com/v1',
  model: 'gpt-4o',
  disable_thinking: false,
  thinking_level: 'medium',
  thinking_dialect: 'auto',
  supports_vision: false,
  context_tokens: 128000,
  auto_tier: 'standard',
  ...over,
})

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
}

describe('ModelSettings API client', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('listModelProfiles GETs /v0/settings/models and unwraps profiles', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({ profiles: [profile({ id: 'mp_1', name: '标准' })] }),
    )
    const list = await listModelProfiles()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/v0/settings/models')
    expect(init.method).toBeUndefined()
    expect(list.map((p) => p.name)).toEqual(['标准'])
  })

  it('createModelProfile POSTs the profile payload', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({ profile: profile({ id: 'mp_1', name: '标准' }) }, { status: 201 }),
    )
    await createModelProfile({
      name: '标准',
      base_url: 'https://api.example.com/v1',
      model: 'gpt-4o',
      api_key: 'sk-secret',
      supports_vision: true,
    })
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/v0/settings/models')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body)).toMatchObject({
      name: '标准',
      base_url: 'https://api.example.com/v1',
      model: 'gpt-4o',
      api_key: 'sk-secret',
      supports_vision: true,
    })
  })

  it('deleteModelProfile sends DELETE', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ status: 'ok' }))
    await deleteModelProfile('mp_1')
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/v0/settings/models/mp_1')
    expect(init.method).toBe('DELETE')
  })

  it('updateModelProfile PATCHes only the provided fields', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({ profile: profile({ id: 'mp_1', name: '标准', model: 'gpt-4o-mini' }) }),
    )
    await updateModelProfile('mp_1', { model: 'gpt-4o-mini' })
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/v0/settings/models/mp_1')
    expect(init.method).toBe('PATCH')
    expect(JSON.parse(init.body)).toEqual({ model: 'gpt-4o-mini' })
  })
})

describe('buildCreatePayload', () => {
  const valid: ProfileFormState = {
    name: '标准',
    baseUrl: 'https://api.example.com/v1',
    model: 'gpt-4o',
    apiKey: 'sk-secret',
    apiKeyEnv: '',
    supportsVision: true,
    thinkingLevel: 'medium',
    thinkingDialect: 'auto',
    contextTokens: 128000,
    tier: 'standard',
  }

  it('requires name', () => {
    const r = buildCreatePayload({ ...valid, name: '  ' })
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.message).toContain('名称')
  })

  it('requires base url', () => {
    const r = buildCreatePayload({ ...valid, baseUrl: '' })
    expect(r.ok).toBe(false)
  })

  it('requires model', () => {
    const r = buildCreatePayload({ ...valid, model: '' })
    expect(r.ok).toBe(false)
  })

  it('requires at least one credential source', () => {
    const r = buildCreatePayload({ ...valid, apiKey: '', apiKeyEnv: '' })
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.message).toBe(MODELS.errApiKeyRequired)
  })

  it('builds a snake_case payload and omits empty api_key', () => {
    const r = buildCreatePayload({ ...valid, apiKey: '', apiKeyEnv: 'OPENAI_API_KEY' })
    expect(r).toEqual({
      ok: true,
      payload: {
        name: '标准',
        base_url: 'https://api.example.com/v1',
        model: 'gpt-4o',
        api_key_env: 'OPENAI_API_KEY',
        supports_vision: true,
        thinking_level: 'medium',
        thinking_dialect: 'auto',
        context_tokens: 128000,
        auto_tier: 'standard',
      },
    })
  })

  it('includes thinking_level and thinking_dialect from the form', () => {
    const r = buildCreatePayload({
      ...valid,
      thinkingLevel: 'off',
      thinkingDialect: 'deepseek',
    })
    expect(r.ok).toBe(true)
    if (r.ok) {
      expect(r.payload.thinking_level).toBe('off')
      expect(r.payload.thinking_dialect).toBe('deepseek')
    }
  })

  it('includes context_tokens from the form', () => {
    const r = buildCreatePayload({ ...valid, contextTokens: 200000 })
    expect(r.ok).toBe(true)
    if (r.ok) expect(r.payload.context_tokens).toBe(200000)
  })

  it('falls back to the default context length when the value is 0', () => {
    const r = buildCreatePayload({ ...valid, contextTokens: 0 })
    expect(r.ok).toBe(true)
    if (r.ok) expect(r.payload.context_tokens).toBe(128000)
  })
})

describe('buildPatchPayload', () => {
  const original = profile({
    id: 'mp_1',
    name: '标准',
    base_url: 'https://api.example.com/v1',
    model: 'gpt-4o',
    api_key: 'sk-…1234',
    api_key_env: 'OPENAI_API_KEY',
    supports_vision: false,
    disable_thinking: false,
    auto_tier: 'standard',
  })

  it('returns an empty payload when nothing changed', () => {
    expect(buildPatchPayload(profileToForm(original), original)).toEqual({})
  })

  it('only sends changed scalar fields', () => {
    const form = profileToForm(original)
    form.model = 'gpt-4o-mini'
    expect(buildPatchPayload(form, original)).toEqual({ model: 'gpt-4o-mini' })
  })

  it('sends changed booleans', () => {
    const form = profileToForm(original)
    form.supportsVision = true
    expect(buildPatchPayload(form, original)).toEqual({
      supports_vision: true,
    })
  })

  it('sends thinking_level and thinking_dialect when changed', () => {
    const form = profileToForm(original)
    form.thinkingLevel = 'high'
    form.thinkingDialect = 'openai'
    expect(buildPatchPayload(form, original)).toEqual({
      thinking_level: 'high',
      thinking_dialect: 'openai',
    })
  })

  it('omits api_key when left blank (backend keeps the stored key)', () => {
    const form = profileToForm(original)
    form.name = '标准-改'
    expect(buildPatchPayload(form, original)).toEqual({ name: '标准-改' })
  })

  it('sends api_key when a new value is typed', () => {
    const form = profileToForm(original)
    form.apiKey = 'sk-new-key'
    expect(buildPatchPayload(form, original)).toEqual({ api_key: 'sk-new-key' })
  })

  it('sends the tier only when changed', () => {
    const form = profileToForm(original)
    form.tier = 'power'
    expect(buildPatchPayload(form, original)).toEqual({ auto_tier: 'power' })
  })

  it('omits context_tokens when unchanged', () => {
    const form = profileToForm(original)
    expect(form.contextTokens).toBe(128000)
    expect(buildPatchPayload(form, original)).toEqual({})
  })

  it('sends context_tokens only when changed', () => {
    const form = profileToForm(original)
    form.contextTokens = 32000
    expect(buildPatchPayload(form, original)).toEqual({ context_tokens: 32000 })
  })

  it('omits context_tokens when set to 0', () => {
    const form = profileToForm(original)
    form.contextTokens = 0
    expect(buildPatchPayload(form, original)).toEqual({})
  })
})

describe('profileToForm', () => {
  it('never prefills the api key field and maps the tier', () => {
    const form = profileToForm(
      profile({ id: 'mp_1', name: 'p', api_key: 'sk-…1234', api_key_env: 'K', auto_tier: 'power' }),
    )
    expect(form.apiKey).toBe('')
    expect(form.apiKeyEnv).toBe('K')
    expect(form.tier).toBe('power')
  })

  it('maps thinking_level and thinking_dialect', () => {
    const form = profileToForm(
      profile({
        id: 'mp_1',
        name: 'p',
        thinking_level: 'low',
        thinking_dialect: 'qwen',
      }),
    )
    expect(form.thinkingLevel).toBe('low')
    expect(form.thinkingDialect).toBe('qwen')
  })
})

describe('ModelProfileList', () => {
  const profiles = [
    profile({ id: 'mp_1', name: '标准模型', supports_vision: true, auto_tier: 'standard' }),
    profile({ id: 'mp_2', name: '轻量模型', model: 'gpt-4o-mini', auto_tier: 'light' }),
  ]

  const render = () =>
    renderToStaticMarkup(
      createElement(ModelProfileList, {
        profiles,
        busy: false,
        onEdit: () => {},
        onDelete: () => {},
      }),
    )

  it('renders every profile with name, model and base url', () => {
    const html = render()
    expect(html).toContain('标准模型')
    expect(html).toContain('轻量模型')
    expect(html).toContain('gpt-4o-mini')
    expect(html).toContain('https://api.example.com/v1')
  })

  it('shows tier + vision badges', () => {
    const html = render()
    expect(html).toContain('标准')
    expect(html).toContain('快速')
    const visionBadges = html.match(/视觉/g)?.length ?? 0
    expect(visionBadges).toBeGreaterThanOrEqual(1)
    expect(html).toContain('ui-badge')
  })

  it('shows 关思考 when thinking_level is off, not 禁用思考', () => {
    const html = renderToStaticMarkup(
      createElement(ModelProfileList, {
        profiles: [
          profile({ id: 'mp_off', name: '关思考模型', thinking_level: 'off', disable_thinking: true }),
        ],
        busy: false,
        onEdit: () => {},
        onDelete: () => {},
      }),
    )
    expect(html).toContain(MODELS.listThinkingOff)
    expect(html).not.toContain('禁用思考')
  })

  it('offers an enabled delete button for every profile (no default lock)', () => {
    const html = render()
    expect(html).toContain('删除')
    const deleteMatches = html.match(/删除/g)?.length ?? 0
    expect(deleteMatches).toBe(2)
    expect(html).not.toMatch(/删除[^<]*disabled/)
  })

  it('renders nothing when the list is empty (page EmptyState owns that UI)', () => {
    const html = renderToStaticMarkup(
      createElement(ModelProfileList, {
        profiles: [],
        busy: false,
        onEdit: () => {},
        onDelete: () => {},
      }),
    )
    expect(html).toBe('')
  })
})

describe('ModelSettings empty header dedupe', () => {
  it('shows add only on EmptyState when list empty', async () => {
    vi.spyOn(api, 'listModelProfiles').mockResolvedValue([])
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    await act(async () => {
      root.render(
        createElement(
          GateContext.Provider,
          { value: { role: 'admin', gateEnabled: true, operatorId: 'admin' } },
          createElement(ModelSettings),
        ),
      )
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    const addButtons = [...host.querySelectorAll('button')].filter((b) =>
      b.textContent?.includes(MODELS.add),
    )
    expect(addButtons).toHaveLength(1)
    expect(host.textContent).toContain(MODELS.emptyTitle)
    root.unmount()
    host.remove()
  })

  it('shows PageHeader add when models exist', async () => {
    vi.spyOn(api, 'listModelProfiles').mockResolvedValue([
      profile({ id: 'mp_1', name: '标准模型' }),
    ])
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    await act(async () => {
      root.render(
        createElement(
          GateContext.Provider,
          { value: { role: 'admin', gateEnabled: true, operatorId: 'admin' } },
          createElement(ModelSettings),
        ),
      )
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(host.textContent).toContain(MODELS.title)
    expect(host.textContent).toContain('标准模型')
    const addButtons = [...host.querySelectorAll('button')].filter((b) =>
      b.textContent?.includes(MODELS.add),
    )
    expect(addButtons).toHaveLength(1)
    root.unmount()
    host.remove()
  })
})

async function renderModelSettings(role: 'admin' | 'operator' = 'admin') {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  await act(async () => {
    root.render(
      createElement(
        GateContext.Provider,
        { value: { role, gateEnabled: true, operatorId: role } },
        createElement(ModelSettings),
      ),
    )
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
  return { host, root }
}

describe('ModelSettings create modal', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('opens create modal with main fields; advanced collapsed by default', async () => {
    vi.spyOn(api, 'listModelProfiles').mockResolvedValue([])
    const { host, root } = await renderModelSettings()
    const addBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(MODELS.add),
    )
    expect(addBtn).toBeTruthy()
    await act(async () => {
      addBtn!.click()
    })
    expect(host.querySelector('[role="dialog"]')).toBeTruthy()
    expect(host.textContent).toContain(MODELS.fieldBaseUrl)
    expect(host.textContent).toContain(MODELS.fieldTier)
    expect(host.textContent).toContain(MODELS.fieldThinkingLevel)
    expect(host.textContent).not.toContain('禁用思考')
    const keyInput = host.querySelector('[role="dialog"] input[type="password"]') as HTMLInputElement
    expect(keyInput.placeholder).toBe('留空则使用环境变量')
    const details = host.querySelector('details.settings-advanced') as HTMLDetailsElement | null
    expect(details).toBeTruthy()
    expect(details?.open).toBe(false)
    expect(details?.textContent).toContain(MODELS.fieldThinkingDialect)
    const summary = details!.querySelector('summary')
    await act(async () => {
      summary!.click()
    })
    expect(details!.open).toBe(true)
    root.unmount()
    host.remove()
  })

  it('PATCHes thinking_level and thinking_dialect from the edit form', async () => {
    const existing = profile({
      id: 'mp_1',
      name: '标准模型',
      thinking_level: 'medium',
      thinking_dialect: 'auto',
    })
    vi.spyOn(api, 'listModelProfiles').mockResolvedValue([existing])
    const updateSpy = vi.spyOn(api, 'updateModelProfile').mockResolvedValue({
      ...existing,
      thinking_level: 'high',
      thinking_dialect: 'deepseek',
    })
    const { host, root } = await renderModelSettings()
    const editBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(MODELS.edit),
    )
    await act(async () => {
      editBtn!.click()
    })
    const dialog = host.querySelector('[role="dialog"]')!
    const selects = [...dialog.querySelectorAll('select')] as HTMLSelectElement[]
    const levelSelect = selects.find((s) =>
      [...s.options].some((o) => o.value === 'high' && o.textContent === MODELS.thinkingLevelHigh),
    )!
    const dialectSelect = selects.find((s) =>
      [...s.options].some((o) => o.value === 'deepseek'),
    )!
    expect(levelSelect).toBeTruthy()
    expect(dialectSelect).toBeTruthy()
    await act(async () => {
      levelSelect.value = 'high'
      levelSelect.dispatchEvent(new Event('change', { bubbles: true }))
      dialectSelect.value = 'deepseek'
      dialectSelect.dispatchEvent(new Event('change', { bubbles: true }))
    })
    const saveBtn = [...dialog.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(MODELS.save),
    )
    await act(async () => {
      saveBtn!.click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(updateSpy).toHaveBeenCalledWith(
      'mp_1',
      expect.objectContaining({
        thinking_level: 'high',
        thinking_dialect: 'deepseek',
      }),
    )
    root.unmount()
    host.remove()
  })

  it('saves create via createModelProfile then closes dialog with toast', async () => {
    vi.spyOn(api, 'listModelProfiles').mockResolvedValue([])
    const createSpy = vi.spyOn(api, 'createModelProfile').mockResolvedValue(
      profile({ id: 'mp_new', name: '新模型' }),
    )
    const { host, root } = await renderModelSettings()
    const addBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(MODELS.add),
    )
    await act(async () => {
      addBtn!.click()
    })
    const dialog = host.querySelector('[role="dialog"]')!
    const inputs = [...dialog.querySelectorAll('input')] as HTMLInputElement[]
    const nameInput = inputs.find((i) => i.getAttribute('placeholder')?.includes('工作模型'))!
    const urlInput = inputs.find((i) => i.getAttribute('placeholder')?.includes('api.openai'))!
    const modelInput = inputs.find((i) => i.getAttribute('placeholder') === 'gpt-4o')!
    const keyInput = inputs.find((i) => i.getAttribute('type') === 'password')!
    await act(async () => {
      setNativeValue(nameInput, '新模型')
      setNativeValue(urlInput, 'https://api.example.com/v1')
      setNativeValue(modelInput, 'gpt-4o')
      setNativeValue(keyInput, 'sk-test')
    })
    const saveBtn = [...dialog.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(MODELS.save),
    )
    await act(async () => {
      saveBtn!.click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(createSpy).toHaveBeenCalled()
    expect(host.querySelector('[role="dialog"]')).toBeNull()
    expect(host.textContent).toContain(MODELS.toastSaved)
    root.unmount()
    host.remove()
  })

  it('shows validation error inside modal instead of only toast', async () => {
    vi.spyOn(api, 'listModelProfiles').mockResolvedValue([])
    const { host, root } = await renderModelSettings()
    const addBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(MODELS.add),
    )
    await act(async () => {
      addBtn!.click()
    })
    const dialog = host.querySelector('[role="dialog"]')!
    const saveBtn = [...dialog.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(MODELS.save),
    )
    await act(async () => {
      saveBtn!.click()
    })
    expect(dialog.querySelector('.ui-inline-error')?.textContent).toBe(MODELS.errNameRequired)
    expect(host.querySelector('[role="dialog"]')).toBeTruthy()
    root.unmount()
    host.remove()
  })

  it('opens edit modal and saves via updateModelProfile', async () => {
    const existing = profile({ id: 'mp_1', name: '标准模型', api_key: 'sk-…1234' })
    vi.spyOn(api, 'listModelProfiles').mockResolvedValue([existing])
    const updateSpy = vi.spyOn(api, 'updateModelProfile').mockResolvedValue({
      ...existing,
      name: '标准模型-改',
    })
    const { host, root } = await renderModelSettings()
    const editBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(MODELS.edit),
    )
    await act(async () => {
      editBtn!.click()
    })
    const dialog = host.querySelector('[role="dialog"]')!
    expect(dialog).toBeTruthy()
    expect(host.textContent).toContain(MODELS.edit)
    const keyInput = dialog.querySelector('input[type="password"]') as HTMLInputElement
    expect(keyInput.placeholder).toBe('留空则不修改')
    const nameInput = [...dialog.querySelectorAll('input')].find(
      (i) => (i as HTMLInputElement).value === '标准模型',
    ) as HTMLInputElement
    await act(async () => {
      setNativeValue(nameInput, '标准模型-改')
    })
    const saveBtn = [...dialog.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(MODELS.save),
    )
    await act(async () => {
      saveBtn!.click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(updateSpy).toHaveBeenCalledWith('mp_1', expect.objectContaining({ name: '标准模型-改' }))
    expect(host.querySelector('[role="dialog"]')).toBeNull()
    expect(host.textContent).toContain(MODELS.toastSaved)
    root.unmount()
    host.remove()
  })
})

describe('ModelSettings delete ConfirmDialog', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('asks ConfirmDialog before delete; last model shows extra warning', async () => {
    const onlyProfile = profile({ id: 'mp_1', name: '唯一模型' })
    vi.spyOn(api, 'listModelProfiles').mockResolvedValue([onlyProfile])
    const deleteSpy = vi.spyOn(api, 'deleteModelProfile').mockResolvedValue()
    const confirmSpy = vi.spyOn(window, 'confirm')

    const { host, root } = await renderModelSettings()
    const deleteBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(MODELS.delete),
    )
    expect(deleteBtn).toBeTruthy()
    await act(async () => {
      deleteBtn!.click()
    })

    expect(confirmSpy).not.toHaveBeenCalled()
    expect(host.textContent).toContain(MODELS.confirmDeleteTitle)
    expect(host.textContent).toContain(MODELS.confirmDeleteBody)
    expect(host.textContent).toContain(MODELS.confirmDeleteLast)
    expect(deleteSpy).not.toHaveBeenCalled()

    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(deleteSpy).toHaveBeenCalledWith('mp_1')
    root.unmount()
    host.remove()
  })
})
