import type { Node } from '@/shared/api/gen/hayat/admin/v1/policy_pb'

export const TENANT = 'hayat/tenant'
export const GROUP = 'hayat/group'

/** A tenant and a group hold things; everything else is a service's resource. */
export const isContainer = (type: string) => type === TENANT || type === GROUP

/** "Tenant", "Group", or a service type as "Connection · tabletsgo". */
export function typeLabel(type: string) {
  if (type === TENANT) return 'Tenant'
  if (type === GROUP) return 'Group'
  const [service, kind = type] = type.split('/')
  return `${kind.charAt(0).toUpperCase()}${kind.slice(1)} · ${service}`
}

/** Children keyed by parent id: groups first, then by name. */
export function childrenMap(nodes: Node[]) {
  const map = new Map<string, Node[]>()
  for (const n of nodes) {
    const list = map.get(n.parentId)
    if (list) list.push(n)
    else map.set(n.parentId, [n])
  }
  for (const list of map.values())
    list.sort((a, b) => Number(isContainer(b.type)) - Number(isContainer(a.type)) || a.name.localeCompare(b.name))
  return map
}

/** Ids of every ancestor of `id` inside the list: what to expand to reveal it. */
export function ancestorIds(nodes: Node[], id: string) {
  const byId = new Map(nodes.map((n) => [n.id, n]))
  const out: string[] = []
  for (let cur = byId.get(byId.get(id)?.parentId ?? ''); cur; cur = byId.get(cur.parentId)) out.push(cur.id)
  return out
}

/** The address of a node in the Resource Tree page. The tenant root is the tenant's own address. */
export const nodeHref = (tenantId: string, nodeId?: string) =>
  nodeId ? `/resource-tree/${tenantId}/${nodeId}` : `/resource-tree/${tenantId}`
