import { type ReactNode, useEffect, useState } from 'react'
import { getMe, getUIConfig, setGateEnabled } from '../api'
import { clearControlToken, readControlToken } from '../controlAuth'
import { GateContext, type GateRole } from '../gateContext'
import { UnlockPage } from './UnlockPage'

type GateState =
  | { status: 'loading' }
  | { status: 'locked' }
  | { status: 'ready'; role: GateRole; gateEnabled: boolean; operatorId: string }

function asGateRole(role: string): GateRole | null {
  if (role === 'operator' || role === 'admin') return role
  return null
}

function isUnauthorized(err: unknown): boolean {
  return err instanceof Error && err.message.startsWith('HTTP 401:')
}

export function GateRoot({ children }: { children: ReactNode }) {
  const [state, setState] = useState<GateState>({ status: 'loading' })
  const [reload, setReload] = useState(0)

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const cfg = await getUIConfig()
        if (cancelled) return
        if (!cfg.gate_enabled) {
          setGateEnabled(false)
          setState({
            status: 'ready',
            role: 'admin',
            gateEnabled: false,
            operatorId: 'local-dev',
          })
          return
        }
        setGateEnabled(true)
        if (!readControlToken()) {
          setState({ status: 'locked' })
          return
        }
        try {
          const me = await getMe()
          if (cancelled) return
          const role = asGateRole(me.role)
          if (!role) {
            clearControlToken()
            setState({ status: 'locked' })
            return
          }
          const operatorId =
            (me.operator_id ?? '').trim() || (role === 'admin' ? 'admin' : '')
          setState({ status: 'ready', role, gateEnabled: true, operatorId })
        } catch (err) {
          if (cancelled) return
          if (isUnauthorized(err)) clearControlToken()
          setState({ status: 'locked' })
        }
      } catch {
        if (cancelled) return
        setGateEnabled(false)
        setState({
          status: 'ready',
          role: 'admin',
          gateEnabled: false,
          operatorId: 'local-dev',
        })
      }
    })()
    return () => {
      cancelled = true
    }
  }, [reload])

  switch (state.status) {
    case 'loading':
      return null
    case 'locked':
      return <UnlockPage onUnlocked={() => setReload((n) => n + 1)} />
    case 'ready':
      return (
        <GateContext.Provider
          value={{
            role: state.role,
            gateEnabled: state.gateEnabled,
            operatorId: state.operatorId,
          }}
        >
          {children}
        </GateContext.Provider>
      )
    default: {
      const _exhaustive: never = state
      return _exhaustive
    }
  }
}
