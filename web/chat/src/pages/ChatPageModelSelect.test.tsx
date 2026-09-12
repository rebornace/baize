import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { createRun, listModelProfiles, type ModelProfile } from '../api'
import { ModelSelect } from '../components/ModelSelect'
import { AUTO_MODEL_ID, buildRunOptions, isAutoChoice, modelOptions, visionGate } from '../modelSelect'

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

const profiles = [
  profile({ id: 'mp_1', name: '标准', model: 'gpt-4o', auto_tier: 'standard' }),
  profile({ id: 'mp_2', name: '轻量', model: 'gpt-4o-mini', auto_tier: 'light' }),
]

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
}

describe('modelOptions', () => {
  it('first option is the Auto smart-router', () => {
    const opts = modelOptions(profiles)
    expect(opts[0]).toEqual({ value: AUTO_MODEL_ID, label: '智能选择' })
  })

  it('lists every profile as a manual choice tagged with its tier', () => {
    const opts = modelOptions(profiles)
    expect(opts.slice(1)).toEqual([
      { value: 'mp_1', label: '标准（gpt-4o） · 标准' },
      { value: 'mp_2', label: '轻量（gpt-4o-mini） · 快速' },
    ])
  })

  it('tags vision-capable profiles', () => {
    const opts = modelOptions([
      profile({ id: 'mp_v', name: '视觉', model: 'gpt-4o', supports_vision: true }),
    ])
    expect(opts[1]).toEqual({ value: 'mp_v', label: '视觉（gpt-4o） · 标准·视觉' })
  })

  it('still yields only the Auto option for an empty profile list', () => {
    expect(modelOptions([])).toEqual([{ value: AUTO_MODEL_ID, label: '智能选择' }])
  })
})

describe('isAutoChoice', () => {
  it('treats empty, whitespace and "auto" as Auto', () => {
    expect(isAutoChoice('')).toBe(true)
    expect(isAutoChoice('   ')).toBe(true)
    expect(isAutoChoice('auto')).toBe(true)
    expect(isAutoChoice('AUTO')).toBe(true)
  })
  it('treats a concrete id as manual', () => {
    expect(isAutoChoice('mp_1')).toBe(false)
  })
})

describe('visionGate', () => {
  const withVision = [
    ...profiles,
    profile({ id: 'mp_3', name: '看图', model: 'gpt-4o', supports_vision: true }),
  ]

  it('always allows non-image turns', () => {
    expect(visionGate(profiles, 'mp_2', false, false).allowed).toBe(true)
  })

  it('Auto mode allows images when a vision profile exists', () => {
    expect(visionGate(withVision, AUTO_MODEL_ID, true, false).allowed).toBe(true)
    expect(visionGate(withVision, '', true, false).allowed).toBe(true)
  })

  it('Auto mode allows images when the default provider is vision-capable', () => {
    expect(visionGate(profiles, AUTO_MODEL_ID, true, true).allowed).toBe(true)
  })

  it('Auto mode blocks images only when no vision model exists at all', () => {
    const r = visionGate(profiles, AUTO_MODEL_ID, true, false)
    expect(r.allowed).toBe(false)
    expect(r.message).toContain('视觉')
  })

  it('manual vision model allows images', () => {
    expect(visionGate(withVision, 'mp_3', true, false).allowed).toBe(true)
  })

  it('manual text-only model blocks images even when a vision model exists (no reroute)', () => {
    const r = visionGate(withVision, 'mp_2', true, false)
    expect(r.allowed).toBe(false)
    expect(r.message).toContain('智能选择')
  })
})

describe('buildRunOptions', () => {
  it('sends the auto sentinel for an empty/auto selection', () => {
    const opts = buildRunOptions('', { webhookUrl: 'https://h', attachments: [] })
    expect(opts.modelProfileId).toBe(AUTO_MODEL_ID)
    expect(opts.webhookUrl).toBe('https://h')
    expect(buildRunOptions('   ').modelProfileId).toBe(AUTO_MODEL_ID)
    expect(buildRunOptions('auto').modelProfileId).toBe(AUTO_MODEL_ID)
  })

  it('sends the concrete id for a named selection and keeps base options', () => {
    const opts = buildRunOptions('mp_2', { sessionToken: 'tok' })
    expect(opts.modelProfileId).toBe('mp_2')
    expect(opts.sessionToken).toBe('tok')
  })
})

describe('ModelSelect', () => {
  it('renders a select whose first option is Auto (value "auto")', () => {
    const html = renderToStaticMarkup(
      createElement(ModelSelect, {
        profiles,
        value: '',
        onChange: () => {},
      }),
    )
    expect(html).toContain('<select')
    expect(html).toContain('智能选择')
    expect(html).toContain('value="auto"')
    expect(html).toContain('标准（gpt-4o） · 标准')
    expect(html).toContain('轻量（gpt-4o-mini） · 快速')
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
    expect(selectTag).not.toContain('value="auto" selected=""')
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

  it('createRun serializes model_profile_id for both auto and named choices', async () => {
    fetchMock.mockImplementation(async () => jsonResponse({ run_id: 'r1', status: 'queued' }))
    await createRun('a1', 'hi', 'c1', buildRunOptions('mp_2', { sessionToken: 'tok' }))
    const body = JSON.parse(fetchMock.mock.calls[0][1].body) as Record<string, unknown>
    expect(body.model_profile_id).toBe('mp_2')
    expect(body.session_token).toBe('tok')

    await createRun('a1', 'hi', 'c1', buildRunOptions(''))
    const body2 = JSON.parse(fetchMock.mock.calls[1][1].body) as Record<string, unknown>
    expect(body2.model_profile_id).toBe('auto')
  })
})
