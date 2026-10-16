// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import { SCHEDULE } from '../strings'
import { ScheduleSettings } from './ScheduleSettings'

const CONV_KEY = 'baize.conversation_id'

async function renderSchedule() {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  vi.spyOn(api, 'listSchedules').mockResolvedValue([])
  await act(async () => {
    root.render(createElement(ScheduleSettings))
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
  return { host, root }
}

function setInputValue(el: HTMLInputElement | HTMLTextAreaElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(
    el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype,
    'value',
  )!.set!
  setter.call(el, value)
  el.dispatchEvent(new Event('input', { bubbles: true }))
  el.dispatchEvent(new Event('change', { bubbles: true }))
}

function setSelectValue(el: HTMLSelectElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')!.set!
  setter.call(el, value)
  el.dispatchEvent(new Event('change', { bubbles: true }))
}

describe('ScheduleSettings', () => {
  beforeEach(() => {
    localStorage.setItem(CONV_KEY, 'conv_1')
  })

  afterEach(() => {
    vi.restoreAllMocks()
    localStorage.removeItem(CONV_KEY)
  })

  it('POSTs exact body when saving a daily schedule at 09:00 Asia/Shanghai', async () => {
    let postedBody = ''
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const url = typeof input === 'string' ? input : input.url
      if (url.includes('/v0/settings/schedules') && init?.method === 'GET') {
        return new Response(JSON.stringify({ items: [] }), { status: 200 })
      }
      if (url.includes('/v0/settings/schedules') && init?.method === 'POST') {
        postedBody = String(init.body ?? '')
        return new Response(
          JSON.stringify({
            id: 'sch_new',
            conversation_id: 'conv_1',
            kind: 'daily',
            instruction: '喝水',
            timezone: 'Asia/Shanghai',
            time_of_day: '09:00',
            status: 'active',
            next_run_at: '2026-10-17T01:00:00Z',
            revision: 1,
          }),
          { status: 201 },
        )
      }
      return new Response('{}', { status: 404 })
    })

    const { host, root } = await renderSchedule()

    const kindSelect = host.querySelector('select[name="kind"]') as HTMLSelectElement
    expect(kindSelect).toBeTruthy()
    await act(async () => {
      setSelectValue(kindSelect, 'daily')
    })

    const instruction = host.querySelector('textarea[name="instruction"]') as HTMLTextAreaElement
    const timezone = host.querySelector('input[name="timezone"]') as HTMLInputElement
    const timeOfDay = host.querySelector('input[name="time_of_day"]') as HTMLInputElement
    await act(async () => {
      setInputValue(instruction, '喝水')
      setInputValue(timezone, 'Asia/Shanghai')
      setInputValue(timeOfDay, '09:00')
    })

    const saveBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(SCHEDULE.save),
    )
    expect(saveBtn).toBeTruthy()
    await act(async () => {
      saveBtn!.click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(fetchSpy).toHaveBeenCalled()
    expect(postedBody).toBe(
      JSON.stringify({
        kind: 'daily',
        conversation_id: 'conv_1',
        time_of_day: '09:00',
        timezone: 'Asia/Shanghai',
        instruction: '喝水',
      }),
    )

    root.unmount()
    host.remove()
  })

  it('pause calls POST /v0/settings/schedules/sch_1/pause', async () => {
    const pauseSpy = vi.spyOn(api, 'pauseSchedule').mockResolvedValue({
      id: 'sch_1',
      owner_id: 'alice',
      conversation_id: 'conv_1',
      instruction: '喝水',
      kind: 'daily',
      timezone: 'Asia/Shanghai',
      time_of_day: '09:00',
      status: 'paused',
      next_run_at: '2026-10-17T01:00:00Z',
      revision: 2,
    })
    vi.spyOn(api, 'listSchedules').mockResolvedValue([
      {
        id: 'sch_1',
        owner_id: 'alice',
        conversation_id: 'conv_1',
        instruction: '喝水',
        kind: 'daily',
        timezone: 'Asia/Shanghai',
        time_of_day: '09:00',
        status: 'active',
        next_run_at: '2026-10-17T01:00:00Z',
        revision: 1,
      },
    ])

    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    await act(async () => {
      root.render(createElement(ScheduleSettings))
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    const pauseBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(SCHEDULE.pause),
    )
    expect(pauseBtn).toBeTruthy()
    await act(async () => {
      pauseBtn!.click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(pauseSpy).toHaveBeenCalledWith('sch_1')

    root.unmount()
    host.remove()
  })
})
