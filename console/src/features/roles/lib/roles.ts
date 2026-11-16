import type { NodeType, PermissionInfo, Role, ServiceInfo } from '@/shared/api/gen/hayat/admin/v1/policy_pb'

/**
 * Plain-language helpers for the roles pages. Everything here is pure: it
 * reads the catalog (node types, permissions), the roles and the services, and
 * answers the questions the pages ask — who made a role, what it lets someone
 * do, and where a permission actually takes effect.
 */

export const PLATFORM = 'hayat'

/** Who a role belongs to: Hayat's built-ins, a service's contract, or this instance's admins. */
export type RoleSource = 'builtin' | 'service' | 'custom'

export const sourceOf = (r: Pick<Role, 'service'>): RoleSource =>
  r.service === PLATFORM ? 'builtin' : r.service === '' ? 'custom' : 'service'

export type Catalog = {
  types: NodeType[]
  permissions: PermissionInfo[]
  services: ServiceInfo[]
}

// ---------- Services ----------

/** Colours a service's mark, so one service reads the same on every page. Hayat is always the accent. */
const TONES = [
  'bg-sky-500/12 text-sky-700 dark:text-sky-300',
  'bg-violet-500/12 text-violet-700 dark:text-violet-300',
  'bg-rose-500/12 text-rose-700 dark:text-rose-300',
  'bg-teal-500/12 text-teal-700 dark:text-teal-300',
  'bg-indigo-500/12 text-indigo-700 dark:text-indigo-300',
]

/** Custom roles' colour; no service uses it. */
export const CUSTOM_TONE = 'bg-amber-500/14 text-amber-800 dark:text-amber-300'

export type ServiceMeta = { id: string; name: string; mono: string; tone: string; description: string; version: number }

/** Every namespace that shows up in the catalog, Hayat first, then services by name. */
export function servicesOf(cat: Catalog): ServiceMeta[] {
  const ids = new Set<string>([PLATFORM])
  for (const p of cat.permissions) ids.add(p.service)
  for (const t of cat.types) ids.add(t.service)
  for (const s of cat.services) ids.add(s.id)
  const others = [...ids].filter((id) => id !== PLATFORM).sort()
  return [PLATFORM, ...others].map((id, i) => serviceMeta(id, cat.services, i - 1))
}

function serviceMeta(id: string, services: ServiceInfo[], index: number): ServiceMeta {
  if (id === PLATFORM) {
    return { id, name: 'Hayat', mono: 'H', tone: 'bg-primary/12 text-primary', description: 'Tenants, members, groups and who may do what.', version: 0 }
  }
  const info = services.find((s) => s.id === id)
  const name = info?.displayName || id
  return { id, name, mono: name.charAt(0).toUpperCase(), tone: TONES[Math.max(index, 0) % TONES.length], description: info?.description ?? '', version: info?.activeVersion ?? 0 }
}

// ---------- Permissions ----------

const capitalise = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

/** The short name of a permission: its label, else its key made readable ("connection.open" → "Connection open"). */
export function permLabel(p: Pick<PermissionInfo, 'key' | 'label' | 'service'>): string {
  if (p.label) return p.label
  const rest = p.key.startsWith(`${p.service}.`) ? p.key.slice(p.service.length + 1) : p.key
  return capitalise(rest.replaceAll('.', ' ').replaceAll('-', ' '))
}

export type PermissionGroup = { service: ServiceMeta; perms: PermissionInfo[]; total: number }

/** Split permission keys by the service that enforces them, in the catalog's service order. */
export function groupByService(keys: string[], cat: Catalog, services = servicesOf(cat)): PermissionGroup[] {
  const byKey = new Map(cat.permissions.map((p) => [p.key, p]))
  return services
    .map((service) => ({
      service,
      perms: keys.map((k) => byKey.get(k)).filter((p): p is PermissionInfo => !!p && p.service === service.id),
      total: cat.permissions.filter((p) => p.service === service.id).length,
    }))
    .filter((g) => g.perms.length > 0)
}

// ---------- Node types and where things take effect ----------

const PLATFORM_TYPE_LABELS: Record<string, string> = {
  'hayat/root': 'Instance root',
  'hayat/tenant': 'Tenant',
  'hayat/group': 'Group',
  'hayat/service': 'Service',
}

/** A node type as people say it: "Tenant", "Connection". */
export function typeLabel(key: string): string {
  if (PLATFORM_TYPE_LABELS[key]) return PLATFORM_TYPE_LABELS[key]
  const name = key.slice(key.indexOf('/') + 1)
  return capitalise(name.replaceAll('-', ' ').replaceAll('_', ' '))
}

/**
 * Whether a node of type `ancestor` can, at any depth, hold one of type
 * `descendant`. The same rule as the server's Snapshot.CanContain: it decides
 * where a permission can be granted — at its scope or anywhere above.
 */
export function canContain(types: NodeType[], ancestor: string, descendant: string): boolean {
  if (ancestor === descendant) return true
  const parents = new Map(types.map((t) => [t.key, t.parents]))
  const seen = new Set<string>()
  const queue = [descendant]
  while (queue.length) {
    const cur = queue.shift()!
    for (const p of parents.get(cur) ?? []) {
      if (p === ancestor) return true
      if (!seen.has(p)) {
        seen.add(p)
        queue.push(p)
      }
    }
  }
  return false
}

/** Whether a permission scoped at `scope` takes effect for a role grantable on `appliesTo` (empty = anywhere). */
export const takesEffect = (types: NodeType[], scope: string, appliesTo: string[]) =>
  appliesTo.length === 0 || appliesTo.some((a) => canContain(types, a, scope))

/** Permissions that can never take effect where the role may be granted, keyed by the scope they need. */
export function unreachable(perms: PermissionInfo[], types: NodeType[], appliesTo: string[]): Map<string, PermissionInfo[]> {
  const out = new Map<string, PermissionInfo[]>()
  for (const p of perms) {
    if (!takesEffect(types, p.scope, appliesTo)) out.set(p.scope, [...(out.get(p.scope) ?? []), p])
  }
  return out
}

/** Node types the role may be granted on where none of its permissions mean anything. */
export const idleTypes = (perms: PermissionInfo[], types: NodeType[], appliesTo: string[]) =>
  perms.length === 0 ? [] : appliesTo.filter((a) => !perms.some((p) => canContain(types, a, p.scope)))

export type TypeRow = { type: NodeType; depth: number; path: string[] }

/**
 * The node types laid out as the tree they form, root first: each type under
 * the first parent it is reached from, so a type with several parents (a
 * connection under a tenant or a group) appears once, at its shallowest place.
 */
export function typeTree(types: NodeType[]): TypeRow[] {
  const children = new Map<string, NodeType[]>()
  for (const t of types) {
    for (const p of t.parents) children.set(p, [...(children.get(p) ?? []), t])
  }
  // Breadth first from the roots: each type is placed under the first parent
  // that reaches it, at its shallowest depth.
  const roots = types.filter((t) => t.parents.length === 0).sort(byPlatformFirst)
  const depth = new Map<string, number>(roots.map((t) => [t.key, 0]))
  const placedUnder = new Map<string, NodeType[]>()
  const queue = roots.map((t) => t.key)
  while (queue.length) {
    const key = queue.shift()!
    for (const c of (children.get(key) ?? []).slice().sort(byPlatformFirst)) {
      if (depth.has(c.key)) continue
      depth.set(c.key, depth.get(key)! + 1)
      placedUnder.set(key, [...(placedUnder.get(key) ?? []), c])
      queue.push(c.key)
    }
  }
  // Then depth first for display, so children sit right under their parent.
  const rows: TypeRow[] = []
  const walk = (t: NodeType, path: string[]) => {
    const here = [...path, typeLabel(t.key)]
    rows.push({ type: t, depth: depth.get(t.key) ?? 0, path: here })
    for (const c of placedUnder.get(t.key) ?? []) walk(c, here)
  }
  roots.forEach((r) => walk(r, []))
  // Anything no root reaches (it shouldn't happen) still shows, at the end.
  for (const t of types) if (!depth.has(t.key)) rows.push({ type: t, depth: 0, path: [typeLabel(t.key)] })
  return rows
}

// Hayat's types in the order people meet them; a service's come after, by key.
const PLATFORM_ORDER = ['hayat/root', 'hayat/tenant', 'hayat/group', 'hayat/service']

function byPlatformFirst(a: NodeType, b: NodeType) {
  const rank = (t: NodeType) => (PLATFORM_ORDER.includes(t.key) ? PLATFORM_ORDER.indexOf(t.key) : PLATFORM_ORDER.length)
  return rank(a) - rank(b) || a.key.localeCompare(b.key)
}

/** "Tenant, Group" — or "any node" when the role isn't limited. */
export const grantableLine = (appliesTo: string[]) => (appliesTo.length ? appliesTo.map(typeLabel).join(', ') : 'any node')

/** The identifier the server will give a new role; mirrors catalog.CreateRole. */
export function slugify(name: string): string {
  return (
    name
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '') || 'role'
  )
}

export const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`
