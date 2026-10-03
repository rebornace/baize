import { createContext, useContext } from 'react'

export type GateRole = 'operator' | 'admin'

export interface GateContextValue {
  role: GateRole
  gateEnabled: boolean
  /** Current operator id from /v0/me (or local-dev when gate is off). */
  operatorId: string
}

export const GateContext = createContext<GateContextValue | null>(null)

export function useGate(): GateContextValue {
  const ctx = useContext(GateContext)
  if (!ctx) {
    throw new Error('useGate must be used within GateRoot')
  }
  return ctx
}
