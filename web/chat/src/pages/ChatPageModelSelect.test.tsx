import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { createRun, listModelProfiles, type ModelProfile } from '../api'
import { ModelSelect } from '../components/ModelSelect'
import { buildRunOptions, modelOptions } from '../modelSelect'

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

const profiles = [
  profile({ id: 'mp_1', name: '主力', model: 'gpt-4o', is_default: true }),
  profile({ id: 'mp_2', name: '廉价', model: 'gpt-4o-mini' }),
]

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
}

describe('modelOptions', () => {
  it('first option is the server default with value ""', () => {
    const opts = modelOptions(profiles)
    expect(opts[0]).toEqual({ value: '', label: '默认：主力' })
  })

  it('labels named profiles as name（model）with the profile id as value', () => {
    const opts = modelOptions(profiles)
    expect(opts.slice(1)).toEqual([{ value: 'mp_2', label: '廉价（gpt-4o-mini）' }])
  })

  it('falls back to a plain 默认模型 label when no profile is marked default', () => {
    const opts = modelOptions([profile({ id: 'mp_2', name: '廉价', is_default: false })])
    expect(opts[0]).toEqual({ value: '', label: '默认模型' })
    expect(opts).toHaveLength(2)
  })

  it('still yields the default option for an empty list', () => {
    expect(modelOptions([])).toEqual([{ value: '', label: '默认模型' }])
  })
})

describe('buildRunOptions', () => {
  it('omits modelProfileId when the selection is empty', () => {
    const opts = buildRunOptions('', { webhookUrl: 'https://h', attachments: [] })
    expect(opts.modelProfileId).toBeUndefined()
    expect(opts).not.toHaveProperty('modelProfileId')
    expect(opts.webhookUrl).toBe('https://h')
  })

  it('adds modelProfileId for a named selection and keeps base options', () => {
    const opts = buildRunOptions('mp_2', { sessionToken: 'tok' })
    expect(opts.modelProfileId).toBe('mp_2')
    expect(opts.sessionToken).toBe('tok')
  })

  it('treats a whitespace-only selection as the default', () => {
    expect(buildRunOptions('   ')).not.toHaveProperty('modelProfileId')
  })
})

describe('ModelSelect', () => {
  it('renders a select whose first option is the default (value "")', () => {
    const html = renderToStaticMarkup(
      createElement(ModelSelect, {
        profiles,
        value: '',
        onChange: () => {},
      }),
    )
    expect(html).toContain('<select')
    expect(html).toContain('value=""')
    expect(html).toContain('默认：主力')
    expect(html).toContain('廉价（gpt-4o-mini）')
    expect(html).toContain('value="mp_2"')
  })

  it('marks the chosen profile as selected', () => {
    const html = renderToStaticMarkup(
      createElement(ModelSelect, {
        profiles,
        value: 'mp_2',
        onChange: () => {},
      }),
    )
    const selectTag = html.slice(html.indexOf('<select'), html.indexOf('</select>'))
    expect(selectTag).toContain('value="mp_2" selected=""')
    expect(selectTag).not.toContain('value="" selected=""')
  })

  it('renders nothing when no profiles are available', () => {
    const html = renderToStaticMarkup(
      createElement(ModelSelect, { profiles: [], value: '', onChange: () => {} }),
    )
    expect(html).toBe('')
  })
})

describe('chat model fetch wiring', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('listModelProfiles GETs /v0/settings/models', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({ profiles: [profile({ id: 'mp_1', name: '主力' })] }),
    )
    const list = await listModelProfiles()
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/v0/settings/models')
    expect(init.method).toBeUndefined()
    expect(list.map((p) => p.id)).toEqual(['mp_1'])
  })

  it('createRun serializes modelProfileId as model_profile_id only when set', async () => {
    fetchMock.mockImplementation(async () => jsonResponse({ run_id: 'r1', status: 'queued' }))
    await createRun('a1', 'hi', 'c1', buildRunOptions('mp_2', { sessionToken: 'tok' }))
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/v0/runs')
    expect(init.method).toBe('POST')
    const body = JSON.parse(init.body) as Record<string, unknown>
    expect(body.model_profile_id).toBe('mp_2')
    expect(body.session_token).toBe('tok')

    await createRun('a1', 'hi', 'c1', buildRunOptions(''))
    const body2 = JSON.parse(fetchMock.mock.calls[1][1].body) as Record<string, unknown>
    expect(body2).not.toHaveProperty('model_profile_id')
  })
})
