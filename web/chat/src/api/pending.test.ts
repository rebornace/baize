import { afterEach, describe, expect, it, vi } from 'vitest'
import { streamPendingApprovals } from './pending'

const encoder = new TextEncoder()

function sseResponse(parts: string[]): Response {
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const part of parts) controller.enqueue(encoder.encode(part))
      controller.close()
    },
  })
  return new Response(stream, {
    status: 200,
    headers: { 'Content-Type': 'text/event-stream' },
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('streamPendingApprovals', () => {
  it('parses snapshots split across chunks', async () => {
    const payload =
      'retry: 3000\n\n' +
      'event: pending\ndata: {"pending":[{"run_id":"r1","conversation_id":"c1","kind":"approval"}],"count":1}\n\n' +
      'event: pending\ndata: {"pending":[],"count":0}\n\n'
    const split = payload.indexOf('"run_id"')
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => sseResponse([payload.slice(0, split), payload.slice(split)])),
    )

    const snapshots: Array<{ pending: Array<{ run_id: string }>; count: number }> = []
    await streamPendingApprovals(new AbortController().signal, (res) => snapshots.push(res))

    expect(snapshots).toHaveLength(2)
    expect(snapshots[0].pending[0].run_id).toBe('r1')
    expect(snapshots[0].count).toBe(1)
    expect(snapshots[1].pending).toEqual([])
  })

  it('ignores malformed events and keeps the stream alive', async () => {
    const body =
      'event: pending\ndata: {not json}\n\n' +
      'event: pending\ndata: {"pending":[{"run_id":"r2","conversation_id":"c2","kind":"login"}],"count":1}\n\n'
    vi.stubGlobal('fetch', vi.fn(async () => sseResponse([body])))

    const snapshots: Array<{ pending: Array<{ run_id: string }>; count: number }> = []
    await streamPendingApprovals(new AbortController().signal, (res) => snapshots.push(res))

    expect(snapshots).toHaveLength(1)
    expect(snapshots[0].pending[0].run_id).toBe('r2')
  })

  it('rejects on a non-2xx response', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('no', { status: 503 })))
    await expect(
      streamPendingApprovals(new AbortController().signal, () => undefined),
    ).rejects.toThrow('pending stream: HTTP 503')
  })
})
