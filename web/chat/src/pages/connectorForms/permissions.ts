import type { PermissionFlag, PermissionSelection } from './types'

interface NamedTool {
  name: string
}

export function emptySelection(tools: NamedTool[]): PermissionSelection {
  const sel: PermissionSelection = {}
  for (const t of tools) sel[t.name] = { login: false, approval: false }
  return sel
}

export function selectionFromLists(
  tools: NamedTool[],
  loginNames: readonly string[],
  approvalNames: readonly string[],
): PermissionSelection {
  const login = new Set(loginNames)
  const approval = new Set(approvalNames)
  const sel: PermissionSelection = {}
  for (const t of tools) {
    sel[t.name] = { login: login.has(t.name), approval: approval.has(t.name) }
  }
  return sel
}

export function toggleTool(
  sel: PermissionSelection,
  name: string,
  flag: PermissionFlag,
): PermissionSelection {
  const cur = sel[name] ?? { login: false, approval: false }
  return {
    ...sel,
    [name]: { ...cur, [flag]: !cur[flag] },
  }
}

export function toNameLists(sel: PermissionSelection): {
  loginNames: string[]
  approvalNames: string[]
} {
  const loginNames: string[] = []
  const approvalNames: string[] = []
  for (const [name, p] of Object.entries(sel)) {
    if (p.login) loginNames.push(name)
    if (p.approval) approvalNames.push(name)
  }
  loginNames.sort()
  approvalNames.sort()
  return { loginNames, approvalNames }
}
