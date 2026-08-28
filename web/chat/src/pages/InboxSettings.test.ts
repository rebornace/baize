import { describe, expect, it } from 'vitest'
import {
  channelsToForm,
  inboxUrlFor,
  validateChannelsForm,
  type ChannelFormRow,
} from './InboxSettings'

const baseRow = (): ChannelFormRow => ({
  id: 'alerts',
  agent_id: 'ticket-agent',
  enabled: true,
  skills: [],
  description: '',
  webhook_url: '',
  headersText: '',
})

describe('channelsToForm', () => {
  it('maps channels to form rows', () => {
    expect(
      channelsToForm([
        {
          id: 'alerts',
          agent_id: 'ticket-agent',
          enabled: true,
          skills: ['ticket-triage'],
          description: 'ops',
          webhook_url: 'https://hook.example',
          webhook_headers: { Authorization: 'Bearer x' },
          secret_hint: 'abcd',
        },
      ]),
    ).toEqual([
      {
        id: 'alerts',
        agent_id: 'ticket-agent',
        enabled: true,
        skills: ['ticket-triage'],
        description: 'ops',
        webhook_url: 'https://hook.example',
        headersText: 'Authorization=Bearer x',
        secret_hint: 'abcd',
      },
    ])
  })

  it('handles empty optional fields', () => {
    expect(channelsToForm([{ id: 'a', agent_id: 'b', enabled: false }])).toEqual([
      {
        id: 'a',
        agent_id: 'b',
        enabled: false,
        skills: [],
        description: '',
        webhook_url: '',
        headersText: '',
        secret_hint: undefined,
      },
    ])
  })
})

describe('validateChannelsForm', () => {
  it('requires id and agent_id', () => {
    const r = validateChannelsForm([{ id: '', agent_id: 'a', enabled: true }])
    expect(r.ok).toBe(false)
  })

  it('requires agent_id', () => {
    const r = validateChannelsForm([{ ...baseRow(), agent_id: '' }])
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.message).toContain('agent_id')
  })

  it('rejects invalid channel id slug', () => {
    const r = validateChannelsForm([{ ...baseRow(), id: 'Bad ID' }])
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.message).toContain('id')
  })

  it('rejects duplicate ids', () => {
    const r = validateChannelsForm([baseRow(), { ...baseRow() }])
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.message).toContain('重复')
  })

  it('rejects invalid header lines', () => {
    const r = validateChannelsForm([{ ...baseRow(), headersText: 'badline' }])
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.message).toMatch(/无效键值行/)
  })

  it('accepts valid rows and trims fields', () => {
    const r = validateChannelsForm([
      {
        ...baseRow(),
        id: '  alerts  ',
        agent_id: '  ticket-agent  ',
        skills: ['ticket-triage'],
        description: '  ops  ',
        webhook_url: '  https://hook.example  ',
        headersText: 'X-Test=1',
      },
    ])
    expect(r).toEqual({
      ok: true,
      channels: [
        {
          id: 'alerts',
          agent_id: 'ticket-agent',
          enabled: true,
          skills: ['ticket-triage'],
          description: 'ops',
          webhook_url: 'https://hook.example',
          webhook_headers: { 'X-Test': '1' },
        },
      ],
    })
  })
})

describe('inboxUrlFor', () => {
  it('builds inbox path under origin', () => {
    expect(inboxUrlFor('https://baize.example', 'alerts')).toBe(
      'https://baize.example/v0/inbox/alerts',
    )
  })
})
