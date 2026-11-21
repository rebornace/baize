// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  desktopNotifyEnabled,
  requestDesktopNotifyPermission,
  setDesktopNotifyEnabled,
  showPendingNotification,
} from './desktopNotify'

type NoteInstance = {
  title: string
  options?: NotificationOptions
  closed: boolean
  onclick: (() => void) | null
  close: () => void
}

let notes: NoteInstance[] = []

class StubNotification {
  static permission: NotificationPermission = 'granted'
  static requestPermission = vi.fn(async () => 'granted' as NotificationPermission)
  title: string
  options?: NotificationOptions
  closed = false
  onclick: (() => void) | null = null
  constructor(title: string, options?: NotificationOptions) {
    this.title = title
    this.options = options
    notes.push(this)
  }
  close = () => {
    this.closed = true
  }
}

beforeEach(() => {
  notes = []
  localStorage.clear()
  vi.stubGlobal('Notification', StubNotification)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('desktop notification preference', () => {
  it('persists the toggle', () => {
    expect(desktopNotifyEnabled()).toBe(false)
    setDesktopNotifyEnabled(true)
    expect(desktopNotifyEnabled()).toBe(true)
    setDesktopNotifyEnabled(false)
    expect(desktopNotifyEnabled()).toBe(false)
  })

  it('requests permission and reports the result', async () => {
    StubNotification.permission = 'default'
    StubNotification.requestPermission.mockResolvedValueOnce('denied')
    await expect(requestDesktopNotifyPermission()).resolves.toBe('denied')
    StubNotification.requestPermission.mockResolvedValueOnce('granted')
    await expect(requestDesktopNotifyPermission()).resolves.toBe('granted')
  })
})

describe('showPendingNotification', () => {
  it('does nothing without permission', () => {
    StubNotification.permission = 'denied'
    const onClick = vi.fn()
    expect(showPendingNotification('t', 'b', onClick)).toBe(false)
    expect(notes).toHaveLength(0)
  })

  it('shows one tagged notification and opens the panel on click', () => {
    StubNotification.permission = 'granted'
    const onClick = vi.fn()
    expect(showPendingNotification('有新的待处理事项', '排查 VPN', onClick)).toBe(true)
    expect(notes).toHaveLength(1)
    expect(notes[0].title).toBe('有新的待处理事项')
    expect(notes[0].options?.body).toBe('排查 VPN')
    expect(notes[0].options?.tag).toBe('baize-pending')
    notes[0].onclick?.()
    expect(notes[0].closed).toBe(true)
    expect(onClick).toHaveBeenCalledOnce()
  })
})
