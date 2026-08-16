import { createContext, useContext } from 'react'

export type GateRole = 'operator' | 'admin'

export interface GateContextValue {
  role: GateRole
  gateEnabled: boolean
}

export const GateContext = createContext<GateContextValue | null>(null)

export function useGate(): GateContextValue {
  const ctx = useContext(GateContext)
  if (!ctx) {
    throw new Error('useGate must be used within GateRoot')
  }
  return ctx
}
