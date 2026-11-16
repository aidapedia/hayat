import type { Grant, Node, NodeAccess } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { formatDate } from '@/shared/lib/datetime'

export type AccessUser = NonNullable<NodeAccess['user']>
export type Person = { user: AccessUser; ways: NodeAccess[] }
/** Everyone who got the same thing at a node, and the grants made on that node for it. */
export type Block = { key: string; title: string; note?: string; mono?: boolean; people: Person[]; grants: Grant[] }

export const OWNER = 'owner'
export const PLACEMENT = 'placement'

/** Owners first, then each role by name, then placement. One entry per person, with every way they get it. */
export function toBlocks(entries: NodeAccess[], grants: Grant[]): Block[] {
  const blocks = new Map<string, Block>()
  const block = (key: string, make: () => Omit<Block, 'key' | 'people' | 'grants'>) => {
    let b = blocks.get(key)
    if (!b) blocks.set(key, (b = { key, ...make(), people: [], grants: [] }))
    return b
  }
  for (const e of entries) {
    if (!e.user) continue
    const key = e.via === OWNER ? OWNER : e.via === PLACEMENT ? PLACEMENT : e.roleId
    const b = block(key, () =>
      key === OWNER
        ? { title: 'Owner', note: 'Every permission' }
        : key === PLACEMENT
          ? { title: 'Filed under a group', note: e.permission, mono: true }
          : { title: e.roleName },
    )
    const person = b.people.find((p) => p.user.id === e.user!.id)
    if (person) person.ways.push(e)
    else b.people.push({ user: e.user, ways: [e] })
  }
  // A grant made here shows on its role's row even when it reaches nobody yet (an empty group).
  for (const g of grants) block(g.roleId, () => ({ title: g.roleName })).grants.push(g)
  const rank = (b: Block) => (b.key === OWNER ? 0 : b.key === PLACEMENT ? 2 : 1)
  const list = [...blocks.values()].sort((a, b) => rank(a) - rank(b) || a.title.localeCompare(b.title))
  for (const b of list) b.people.sort((x, y) => (x.user.name || x.user.email).localeCompare(y.user.name || y.user.email))
  return list
}

/** The grant made on this node directly to this person for the block's role: the one thing removable per person. */
export const directGrant = (b: Block, userId: string) => b.grants.find((g) => g.principalType === 'user' && g.principalId === userId)

/** How one way reaches them, in a short phrase: "Through Engineering · from Acme". */
export function wayText(w: NodeAccess, node: Node, tenantName: string) {
  const how =
    w.via === 'owner'
      ? 'Owner'
      : w.via === 'user'
        ? 'Direct'
        : w.via === 'group'
          ? `Through ${w.groupName}${w.throughGroupName ? ` (via ${w.throughGroupName})` : ''}`
          : w.via === 'everyone'
            ? `Everyone in ${tenantName}`
            : w.throughGroupName
              ? `In ${w.groupName} via ${w.throughGroupName}`
              : `On ${w.groupName}'s roster`
  const src = w.source
  const from =
    !src || src.id === node.id || w.via === 'placement' ? '' : src.type === 'hayat/root' ? ' · whole instance' : ` · from ${src.name}`
  const until = w.expiresAt ? ` · until ${formatDate(w.expiresAt)}` : ''
  return how + from + until
}

/** Why a person can't be ungranted here, as a short hint at where to change it instead. */
export function whyNotRemovable(p: Person, node: Node) {
  const w = p.ways[0]
  if (!w) return ''
  if (w.via === 'owner') return 'Owner, always has it'
  if (w.via === 'group') return `Remove from ${w.throughGroupName || w.groupName} instead`
  if (w.via === 'everyone') return 'Every tenant member has it'
  if (w.via === 'placement') return `Move it out of ${w.groupName} instead`
  if (w.source && w.source.id !== node.id) return `Ungrant on ${w.source.type === 'hayat/root' ? 'the instance' : w.source.name} instead`
  return ''
}
