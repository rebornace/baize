import { describe, expect, it, vi } from 'vitest'
import {
  confirmAndDeleteConnector,
  confirmDeleteConnectorMessage,
  type ConnectorDeleteDeps,
} from './connectorDelete'

function makeDeps(opts: { confirm: boolean; deleteConnector?: (id: string) => Promise<void> }): ConnectorDeleteDeps {
  return {
    confirm: () => opts.confirm,
    deleteConnector: opts.deleteConnector ?? (async () => undefined),
  }
}

describe('confirmDeleteConnectorMessage', () => {
  it('includes the connector id and a warning that all tools are removed', () => {
    const msg = confirmDeleteConnectorMessage('ticket-api')
    expect(msg).toContain('ticket-api')
    expect(msg).toContain('全部工具')
    expect(msg).toContain('删除')
  })

  it('quotes the id so it is visually distinguishable', () => {
    expect(confirmDeleteConnectorMessage('my-cool-api')).toContain('"my-cool-api"')
  })

  it('produces a different message per id', () => {
    expect(confirmDeleteConnectorMessage('a')).not.toBe(confirmDeleteConnectorMessage('b'))
  })
})

describe('confirmAndDeleteConnector', () => {
  it('does not call deleteConnector when the user cancels', async () => {
    const del = vi.fn().mockResolvedValue(undefined)
    const deleted = await confirmAndDeleteConnector('ticket-api', makeDeps({ confirm: false, deleteConnector: del }))
    expect(deleted).toBe(false)
    expect(del).not.toHaveBeenCalled()
  })

  it('calls deleteConnector once with the id when the user confirms', async () => {
    const del = vi.fn().mockResolvedValue(undefined)
    const deleted = await confirmAndDeleteConnector('ticket-api', makeDeps({ confirm: true, deleteConnector: del }))
    expect(deleted).toBe(true)
    expect(del).toHaveBeenCalledTimes(1)
    expect(del).toHaveBeenCalledWith('ticket-api')
  })

  it('passes the confirm message to the confirm callback', async () => {
    const confirmSpy = vi.fn().mockReturnValue(true)
    const del = vi.fn().mockResolvedValue(undefined)
    await confirmAndDeleteConnector('ticket-api', { confirm: confirmSpy, deleteConnector: del })
    expect(confirmSpy).toHaveBeenCalledTimes(1)
    const message = confirmSpy.mock.calls[0]![0] as string
    expect(message).toContain('ticket-api')
    expect(message).toContain('全部工具')
  })

  it('propagates deleteConnector errors', async () => {
    const del = vi.fn().mockRejectedValue(new Error('boom'))
    await expect(
      confirmAndDeleteConnector('ticket-api', makeDeps({ confirm: true, deleteConnector: del })),
    ).rejects.toThrow('boom')
    expect(del).toHaveBeenCalledTimes(1)
  })
})
