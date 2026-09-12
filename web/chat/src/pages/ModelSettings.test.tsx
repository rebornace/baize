import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import {
  createModelProfile,
  deleteModelProfile,
  listModelProfiles,
  updateModelProfile,
  type ModelProfile,
} from '../api'
import {
  buildCreatePayload,
  buildPatchPayload,
  EMPTY_PROFILE_FORM,
  ModelProfileForm,
  ModelProfileList,
  profileToForm,
  type ProfileFormState,
} from './ModelSettings'

const profile = (over: Partial<ModelProfile> & Pick<ModelProfile, 'id' | 'name'>): ModelProfile => ({
  provider: 'openai_compatible',
  base_url: 'https://api.example.com/v1',
  model: 'gpt-4o',
  disable_thinking: false,
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
    disableThinking: false,
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
    if (!r.ok) expect(r.message).toContain('至少填写一项')
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
        disable_thinking: false,
        context_tokens: 128000,
        auto_tier: 'standard',
      },
    })
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
    form.disableThinking = true
    expect(buildPatchPayload(form, original)).toEqual({
      supports_vision: true,
      disable_thinking: true,
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
    const items = html.split('<li').slice(1)
    expect(items).toHaveLength(2)
    // 标准模型 carries the 标准 tier badge.
    expect(items[0]).toContain('<span class="settings-badge">标准</span>')
    // 轻量模型's name still contains 轻量, so the light tier is asserted via
    // its 快速 tier badge rather than via the 轻量 substring.
    expect(items[1]).toContain('<span class="settings-badge">快速</span>')
    // The vision profile carries a 视觉 badge in addition to its tier badge.
    const visionBadges = html.match(/视觉/g)?.length ?? 0
    expect(visionBadges).toBeGreaterThanOrEqual(1)
  })

  it('offers an enabled delete button for every profile (no default lock)', () => {
    const html = render()
    const items = html.split('<li').slice(1)
    expect(items).toHaveLength(2)
    for (const item of items) {
      expect(item).toContain('删除')
      // The delete button itself must not carry a disabled attribute.
      const delBtn = item.slice(item.lastIndexOf('删除') - 200)
      expect(delBtn).not.toContain('disabled=""')
    }
  })

  it('renders an empty-state prompt when no models exist', () => {
    const html = renderToStaticMarkup(
      createElement(ModelProfileList, {
        profiles: [],
        busy: false,
        onEdit: () => {},
        onDelete: () => {},
      }),
    )
    expect(html).toContain('尚未配置任何模型')
  })
})

describe('ModelProfileForm', () => {
  it('create form hints that an empty key falls back to the environment and shows tier selector', () => {
    const html = renderToStaticMarkup(
      createElement(ModelProfileForm, {
        form: EMPTY_PROFILE_FORM,
        setForm: () => {},
        busy: false,
        isEdit: false,
        title: '新建模型',
        submitLabel: '创建模型',
        onSubmit: () => {},
      }),
    )
    expect(html).toContain('留空则使用环境变量')
    expect(html).toContain('Auto 路由档位')
    expect(html).toContain('自动识别（按模型名）')
  })

  it('edit form hints that an empty key means "keep unchanged"', () => {
    const form: ProfileFormState = {
      ...EMPTY_PROFILE_FORM,
      name: 'p',
      baseUrl: 'https://x/v1',
      model: 'm',
      apiKeyEnv: 'K',
    }
    const html = renderToStaticMarkup(
      createElement(ModelProfileForm, {
        form,
        setForm: () => {},
        busy: false,
        isEdit: true,
        title: '编辑 p',
        submitLabel: '保存',
        onSubmit: () => {},
        onCancel: () => {},
      }),
    )
    expect(html).toContain('留空则不修改')
  })
})
