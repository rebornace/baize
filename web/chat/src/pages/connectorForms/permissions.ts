import type { PermissionFlag, PermissionSelection } from './types'

/** 权限步展示用的工具摘要（至少有 name）。 */
export interface PermissionTool {
  name: string
  title?: string
  description?: string
}

export function toPermissionTools(
  tools: Array<{ name: string; title?: string; description?: string }> | undefined,
): PermissionTool[] {
  return (tools ?? []).map((t) => {
    const title = t.title?.trim()
    const description = t.description?.trim()
    return {
      name: t.name,
      title: title || undefined,
      description: description || undefined,
    }
  })
}

export function filterPermissionTools(tools: PermissionTool[], query: string): PermissionTool[] {
  const q = query.trim().toLowerCase()
  if (q === '') return tools
  return tools.filter((t) => {
    const hay = `${t.name}\n${t.title ?? ''}\n${t.description ?? ''}`.toLowerCase()
    return hay.includes(q)
  })
}

export function emptySelection(tools: PermissionTool[]): PermissionSelection {
  const sel: PermissionSelection = {}
  for (const t of tools) sel[t.name] = { login: false, approval: false }
  return sel
}

export function selectionFromLists(
  tools: PermissionTool[],
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
