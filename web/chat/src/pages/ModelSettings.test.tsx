import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import {
  createModelProfile,
  deleteModelProfile,
  listModelProfiles,
  setDefaultModelProfile,
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
  is_default: false,
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
      jsonResponse({ profiles: [profile({ id: 'mp_1', name: '主力' })] }),
    )
    const list = await listModelProfiles()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/v0/settings/models')
    expect(init.method).toBeUndefined()
    expect(list.map((p) => p.name)).toEqual(['主力'])
  })

  it('createModelProfile POSTs the profile payload', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({ profile: profile({ id: 'mp_1', name: '主力' }) }, { status: 201 }),
    )
    await createModelProfile({
      name: '主力',
      base_url: 'https://api.example.com/v1',
      model: 'gpt-4o',
      api_key: 'sk-secret',
      supports_vision: true,
    })
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/v0/settings/models')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body)).toMatchObject({
      name: '主力',
      base_url: 'https://api.example.com/v1',
      model: 'gpt-4o',
      api_key: 'sk-secret',
      supports_vision: true,
    })
  })

  it('setDefaultModelProfile POSTs to .../default', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ status: 'ok' }))
    await setDefaultModelProfile('mp_1')
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/v0/settings/models/mp_1/default')
    expect(init.method).toBe('POST')
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
      jsonResponse({ profile: profile({ id: 'mp_1', name: '主力', model: 'gpt-4o-mini' }) }),
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
    name: '主力',
    baseUrl: 'https://api.example.com/v1',
    model: 'gpt-4o',
    apiKey: 'sk-secret',
    apiKeyEnv: '',
    supportsVision: true,
    disableThinking: false,
    contextTokens: 128000,
    isDefault: true,
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
        name: '主力',
        base_url: 'https://api.example.com/v1',
        model: 'gpt-4o',
        api_key_env: 'OPENAI_API_KEY',
        supports_vision: true,
        disable_thinking: false,
        context_tokens: 128000,
        is_default: true,
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
    name: '主力',
    base_url: 'https://api.example.com/v1',
    model: 'gpt-4o',
    api_key: 'sk-…1234',
    api_key_env: 'OPENAI_API_KEY',
    supports_vision: false,
    disable_thinking: false,
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
    form.name = '主力-改'
    expect(buildPatchPayload(form, original)).toEqual({ name: '主力-改' })
  })

  it('sends api_key when a new value is typed', () => {
    const form = profileToForm(original)
    form.apiKey = 'sk-new-key'
    expect(buildPatchPayload(form, original)).toEqual({ api_key: 'sk-new-key' })
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
  it('never prefills the api key field', () => {
    const form = profileToForm(
      profile({ id: 'mp_1', name: 'p', api_key: 'sk-…1234', api_key_env: 'K' }),
    )
    expect(form.apiKey).toBe('')
    expect(form.apiKeyEnv).toBe('K')
  })
})

describe('ModelProfileList', () => {
  const profiles = [
    profile({
      id: 'mp_1',
      name: '默认模型',
      is_default: true,
      supports_vision: true,
    }),
    profile({ id: 'mp_2', name: '廉价模型', model: 'gpt-4o-mini' }),
  ]

  const render = () =>
    renderToStaticMarkup(
      createElement(ModelProfileList, {
        profiles,
        busy: false,
        onSetDefault: () => {},
        onEdit: () => {},
        onDelete: () => {},
      }),
    )

  it('renders every profile with name, model and base url', () => {
    const html = render()
    expect(html).toContain('默认模型')
    expect(html).toContain('廉价模型')
    expect(html).toContain('gpt-4o-mini')
    expect(html).toContain('https://api.example.com/v1')
  })

  it('shows the 默认 badge only on the default profile', () => {
    const html = render()
    const badgeCount = html.match(/settings-badge/g)?.length ?? 0
    expect(badgeCount).toBe(1)
    expect(html).toContain('默认')
  })

  it('disables delete and set-default for the default profile', () => {
    const html = render()
    const items = html.split('<li').slice(1)
    const defaultItem = items.find((s) => s.includes('默认模型'))!
    const otherItem = items.find((s) => s.includes('廉价模型'))!
    // Default row: delete button disabled, set-default shows 当前默认.
    expect(defaultItem).toContain('当前默认')
    expect(defaultItem).toContain('disabled=""')
    // Non-default row offers an active 设为默认 button.
    expect(otherItem).toContain('设为默认')
  })
})

describe('ModelProfileForm', () => {
  it('create form hints that an empty key falls back to the environment', () => {
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
    expect(html).toContain('创建后设为默认')
  })

  it('edit form hints that an empty key means "keep unchanged" and hides default checkbox', () => {
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
    expect(html).not.toContain('创建后设为默认')
  })
})
