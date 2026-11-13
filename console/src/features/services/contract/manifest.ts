/**
 * A contract manifest as the form edits it, and the rules the server applies
 * to it (internal/catalog/contract.go: compile + diff), so the form can say
 * what is wrong before publishing. The server stays the judge.
 */
import { yamlString } from './yaml'

// ---------- The manifest as written (YAML/JSON) ----------

export type ManifestAccess = { permission: string; via_placement?: boolean; requires_membership?: boolean }
export type ManifestType = { name: string; description?: string; parents: string[]; access?: ManifestAccess; degradable?: boolean }
export type ManifestPerm = { key: string; scope: string; description?: string }
export type ManifestRole = { slug: string; name?: string; description?: string; permissions: string[]; applies_to?: string[] }
export type Manifest = {
  service: string
  version: number
  description?: string
  resource_types: ManifestType[]
  permissions: ManifestPerm[]
  default_roles?: ManifestRole[]
}

// ---------- The form's draft ----------

export type Access = { permission: string; viaPlacement: boolean; requiresMembership: boolean }
export type TypeRow = { uid: string; published: boolean; name: string; description: string; parents: string[]; access: Access | null; degradable: boolean }
export type PermRow = { uid: string; published: boolean; key: string; scope: string; description: string }
export type RoleRow = { uid: string; published: boolean; slug: string; name: string; description: string; permissions: string[]; appliesTo: string[] }
export type Draft = { service: string; version: number; description: string; types: TypeRow[]; permissions: PermRow[]; roles: RoleRow[] }

/** What the active version already put in the catalog, qualified. Contracts may only add to it. */
export type Baseline = {
  types: Map<string, { parents: string[]; access: Access | null }>
  perms: Map<string, string>
  roles: Map<string, string[]>
}

/** A node type from the catalog (hayat's own and other services'). */
export type KnownType = { key: string; service: string; description: string }

export const PLATFORM = 'hayat'
const TYPE_ROOT = 'hayat/root'
const TYPE_SERVICE = 'hayat/service'
const NAME_RE = /^[a-z][a-z0-9_-]{0,47}$/
const PERM_RE = /^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)*$/

let next = 0
export const uid = () => `r${++next}`

// Names are written short in a manifest and stored qualified:
// "ledger" → "billing/ledger", "ledger.view" → "billing.ledger.view".
export const qType = (svc: string, t: string) => (t.includes('/') ? t : `${svc}/${t}`)
export const qPerm = (svc: string, k: string) => (k.startsWith(`${svc}.`) || k.startsWith(`${PLATFORM}.`) ? k : `${svc}.${k}`)
export const shortType = (svc: string, t: string) => (t.startsWith(`${svc}/`) ? t.slice(svc.length + 1) : t)
export const shortPerm = (svc: string, k: string) => (k.startsWith(`${svc}.`) ? k.slice(svc.length + 1) : k)

export const emptyDraft = (service: string, version: number): Draft => ({ service, version, description: '', types: [], permissions: [], roles: [] })

// ---------- Reading ----------

const KNOWN_FIELDS: Record<string, string[]> = {
  manifest: ['service', 'version', 'description', 'resource_types', 'permissions', 'default_roles'],
  type: ['name', 'description', 'parents', 'access', 'degradable'],
  access: ['permission', 'via_placement', 'requires_membership'],
  permission: ['key', 'scope', 'description'],
  role: ['slug', 'name', 'description', 'permissions', 'applies_to'],
}

function obj(v: unknown, what: string): Record<string, unknown> {
  if (!v || typeof v !== 'object' || Array.isArray(v)) throw new Error(`${what} should be a map of fields`)
  const o = v as Record<string, unknown>
  const unknown = Object.keys(o).find((k) => !KNOWN_FIELDS[what.split(' ')[0]]?.includes(k))
  if (unknown) throw new Error(`"${unknown}" isn't a field of a ${what.split(' ')[0]}`)
  return o
}
function list(v: unknown, what: string): unknown[] {
  if (v == null) return []
  if (!Array.isArray(v)) throw new Error(`${what} should be a list`)
  return v
}
const str = (v: unknown) => (v == null ? '' : String(v))
const strs = (v: unknown, what: string) => list(v, what).map(str)

/** A parsed manifest (YAML or JSON) as a draft. Throws when it isn't shaped like one. */
export function toDraft(raw: unknown, svc: string, base: Baseline): Draft {
  const m = obj(raw, 'manifest')
  if (m.service != null && str(m.service) !== svc) throw new Error(`this contract is for "${str(m.service)}", not "${svc}"`)
  const types = list(m.resource_types, 'resource_types').map((v, i): TypeRow => {
    const t = obj(v, `type #${i + 1}`)
    const a = t.access == null ? null : obj(t.access, 'access')
    const name = shortType(svc, str(t.name))
    return {
      uid: uid(),
      published: base.types.has(qType(svc, name)),
      name,
      description: str(t.description),
      parents: strs(t.parents, 'parents').map((p) => shortType(svc, p)),
      access: a && { permission: shortPerm(svc, str(a.permission)), viaPlacement: !!a.via_placement, requiresMembership: !!a.requires_membership },
      degradable: !!t.degradable,
    }
  })
  const permissions = list(m.permissions, 'permissions').map((v, i): PermRow => {
    const p = obj(v, `permission #${i + 1}`)
    const key = shortPerm(svc, str(p.key))
    return { uid: uid(), published: base.perms.has(qPerm(svc, key)), key, scope: shortType(svc, str(p.scope)), description: str(p.description) }
  })
  const roles = list(m.default_roles, 'default_roles').map((v, i): RoleRow => {
    const r = obj(v, `role #${i + 1}`)
    const slug = str(r.slug)
    return {
      uid: uid(),
      published: base.roles.has(`${svc}.${slug}`),
      slug,
      name: str(r.name),
      description: str(r.description),
      permissions: strs(r.permissions, 'permissions').map((p) => shortPerm(svc, p)),
      appliesTo: strs(r.applies_to, 'applies_to').map((t) => shortType(svc, t)),
    }
  })
  const version = Number(m.version)
  return { service: svc, version: Number.isFinite(version) ? version : 0, description: str(m.description), types, permissions, roles }
}

/** The active version's catalog entries, from its stored manifest. */
export function baselineOf(manifestJson: string | undefined, svc: string): Baseline {
  const base: Baseline = { types: new Map(), perms: new Map(), roles: new Map() }
  if (!manifestJson) return base
  const d = toDraft(JSON.parse(manifestJson), svc, base)
  for (const t of d.types) base.types.set(qType(svc, t.name), { parents: t.parents.map((p) => qType(svc, p)), access: t.access && { ...t.access, permission: qPerm(svc, t.access.permission) } })
  for (const p of d.permissions) base.perms.set(qPerm(svc, p.key), qType(svc, p.scope))
  for (const r of d.roles) base.roles.set(`${svc}.${r.slug}`, r.permissions.map((p) => qPerm(svc, p)))
  return base
}

// ---------- Writing ----------

export function toManifest(d: Draft): Manifest {
  const trim = (s: string) => s.trim() || undefined
  const m: Manifest = {
    service: d.service,
    version: d.version,
    description: trim(d.description),
    resource_types: d.types.map((t) => ({
      name: t.name.trim(),
      description: trim(t.description),
      parents: t.parents,
      access: t.access ? { permission: t.access.permission, via_placement: t.access.viaPlacement || undefined, requires_membership: t.access.requiresMembership || undefined } : undefined,
      degradable: t.degradable || undefined,
    })),
    permissions: d.permissions.map((p) => ({ key: p.key.trim(), scope: p.scope, description: trim(p.description) })),
    default_roles: d.roles.map((r) => ({
      slug: r.slug.trim(),
      name: trim(r.name),
      description: trim(r.description),
      permissions: r.permissions,
      applies_to: r.appliesTo.length ? r.appliesTo : undefined,
    })),
  }
  return m
}

const flowList = (xs: string[]) => `[${xs.map(yamlString).join(', ')}]`
const flowMap = (o: Record<string, unknown>) =>
  `{ ${Object.entries(o)
    .filter(([, v]) => v !== undefined && v !== '' && v !== false)
    .map(([k, v]) => `${k}: ${typeof v === 'string' ? yamlString(v) : String(v)}`)
    .join(', ')} }`

/** A manifest as YAML, in the shape the docs use. */
export function manifestYaml(m: Manifest): string {
  const out = [`service: ${yamlString(m.service)}`, `version: ${m.version}`]
  if (m.description) out.push(`description: ${yamlString(m.description)}`)
  const types = m.resource_types ?? []
  out.push(types.length ? 'resource_types:' : 'resource_types: []')
  for (const t of types) {
    out.push(`  - name: ${yamlString(t.name)}`)
    if (t.description) out.push(`    description: ${yamlString(t.description)}`)
    out.push(`    parents: ${flowList(t.parents ?? [])}`)
    if (t.access) out.push(`    access: ${flowMap(t.access)}`)
    if (t.degradable) out.push('    degradable: true')
  }
  const perms = m.permissions ?? []
  out.push(perms.length ? 'permissions:' : 'permissions: []')
  for (const p of perms) out.push(`  - ${flowMap(p)}`)
  const roles = m.default_roles ?? []
  if (roles.length) out.push('default_roles:')
  for (const r of roles) {
    out.push(`  - slug: ${yamlString(r.slug)}`)
    if (r.name) out.push(`    name: ${yamlString(r.name)}`)
    if (r.description) out.push(`    description: ${yamlString(r.description)}`)
    out.push(`    permissions: ${flowList(r.permissions ?? [])}`)
    if (r.applies_to?.length) out.push(`    applies_to: ${flowList(r.applies_to)}`)
  }
  return out.join('\n') + '\n'
}

export const draftYaml = (d: Draft) => manifestYaml(toManifest(d))

// ---------- Checking ----------

/** Field errors keyed `${uid}.${field}`; `empty` ones wait until the person tries to publish. */
export type Problem = { message: string; empty?: boolean }
export type Check = { fields: Map<string, Problem>; general: string[] }

export function check(d: Draft, known: KnownType[], base: Baseline, minVersion: number): Check {
  const svc = d.service
  const fields = new Map<string, Problem>()
  const general: string[] = []
  const put = (key: string, message: string, empty = false) => !fields.has(key) && fields.set(key, { message, empty })

  if (!(d.version > minVersion)) put('version', `Must be greater than ${minVersion}`)

  const own = new Set<string>()
  for (const t of d.types) {
    const key = qType(svc, t.name.trim())
    if (!t.name.trim()) put(`${t.uid}.name`, 'Give the type a name', true)
    else if (!NAME_RE.test(t.name.trim())) put(`${t.uid}.name`, 'Use lowercase letters, digits, - and _, starting with a letter')
    else if (own.has(key)) put(`${t.uid}.name`, 'Another type has this name')
    own.add(key)
  }
  const isKnown = (typ: string) => own.has(typ) || known.some((k) => k.key === typ && k.service !== svc)

  const perms = new Set<string>()
  for (const p of d.permissions) {
    const key = qPerm(svc, p.key.trim())
    if (!p.key.trim()) put(`${p.uid}.key`, 'Give the permission a key', true)
    else if (!PERM_RE.test(p.key.trim())) put(`${p.uid}.key`, 'Use lowercase words joined by dots, like ledger.view')
    else if (perms.has(key)) put(`${p.uid}.key`, 'Another permission has this key')
    perms.add(key)
    if (!p.scope) put(`${p.uid}.scope`, 'Pick where it applies', true)
    else if (!isKnown(qType(svc, p.scope))) put(`${p.uid}.scope`, `"${p.scope}" isn't a known type`)
  }

  for (const t of d.types) {
    if (t.parents.length === 0) put(`${t.uid}.parents`, 'Pick at least one place it can be filed under', true)
    for (const p of t.parents) {
      const q = qType(svc, p)
      if (q === TYPE_ROOT || q === TYPE_SERVICE) put(`${t.uid}.parents`, `It can't sit directly under ${q}`)
      else if (!isKnown(q)) put(`${t.uid}.parents`, `"${p}" isn't a known type`)
    }
    if (t.access) {
      if (!t.access.permission) put(`${t.uid}.access`, 'Pick the permission that opens it', true)
      else if (!perms.has(qPerm(svc, t.access.permission))) put(`${t.uid}.access`, `"${t.access.permission}" isn't declared below`)
    }
  }

  const slugs = new Set<string>()
  for (const r of d.roles) {
    if (!r.slug.trim()) put(`${r.uid}.slug`, 'Give the role an id', true)
    else if (!NAME_RE.test(r.slug.trim())) put(`${r.uid}.slug`, 'Use lowercase letters, digits, - and _, starting with a letter')
    else if (slugs.has(r.slug.trim())) put(`${r.uid}.slug`, 'Another role has this id')
    slugs.add(r.slug.trim())
    if (r.permissions.length === 0) put(`${r.uid}.permissions`, 'A role has to grant at least one permission', true)
    const missing = r.permissions.find((p) => !perms.has(qPerm(svc, p)))
    if (missing) put(`${r.uid}.permissions`, `"${missing}" isn't declared`)
    const unknownType = r.appliesTo.find((t) => !isKnown(qType(svc, t)))
    if (unknownType) put(`${r.uid}.appliesTo`, `"${unknownType}" isn't a known type`)
  }

  // Additive only: what is live can't go away or change.
  const types = new Map(d.types.map((t) => [qType(svc, t.name.trim()), t]))
  for (const [key, old] of base.types) {
    const cur = types.get(key)
    if (!cur) {
      general.push(`Resource type ${key} is live and can't be removed.`)
      continue
    }
    const lost = old.parents.find((p) => !cur.parents.some((c) => qType(svc, c) === p))
    if (lost) general.push(`Resource type ${key} can't stop being filed under ${lost}.`)
    const a = cur.access
    if (old.access && (!a || qPerm(svc, a.permission) !== old.access.permission || a.viaPlacement !== old.access.viaPlacement || a.requiresMembership !== old.access.requiresMembership))
      general.push(`Resource type ${key}'s access rule is live and can't change.`)
  }
  const permRows = new Map(d.permissions.map((p) => [qPerm(svc, p.key.trim()), p]))
  for (const [key, scope] of base.perms) {
    const cur = permRows.get(key)
    if (!cur) general.push(`Permission ${key} is live and can't be removed.`)
    else if (qType(svc, cur.scope) !== scope) general.push(`Permission ${key} applies at ${scope}; that can't change.`)
  }
  const roleRows = new Map(d.roles.map((r) => [`${svc}.${r.slug.trim()}`, r]))
  for (const [slug, old] of base.roles) {
    const cur = roleRows.get(slug)
    if (!cur) general.push(`Role ${slug} is live and can't be removed.`)
    else {
      const lost = old.find((p) => !cur.permissions.some((c) => qPerm(svc, c) === p))
      if (lost) general.push(`Role ${slug} can't stop granting ${lost}.`)
    }
  }
  return { fields, general }
}

// ---------- What a version adds ----------

export type Summary = {
  added_types?: string[]
  added_permissions?: string[]
  added_roles?: string[]
  added_parents?: Record<string, string[]>
  added_role_permissions?: Record<string, string[]>
}

/** What the draft adds to the live version, in the server's summary shape. */
export function summarize(d: Draft, base: Baseline): Summary {
  const svc = d.service
  const s: Required<Summary> = { added_types: [], added_permissions: [], added_roles: [], added_parents: {}, added_role_permissions: {} }
  for (const t of d.types) {
    if (!t.name.trim()) continue
    const key = qType(svc, t.name.trim())
    const old = base.types.get(key)
    if (!old) s.added_types.push(key)
    else {
      const added = t.parents.map((p) => qType(svc, p)).filter((p) => !old.parents.includes(p))
      if (added.length) s.added_parents[key] = added
    }
  }
  for (const p of d.permissions) {
    const key = qPerm(svc, p.key.trim())
    if (p.key.trim() && !base.perms.has(key)) s.added_permissions.push(key)
  }
  for (const r of d.roles) {
    if (!r.slug.trim()) continue
    const slug = `${svc}.${r.slug.trim()}`
    const old = base.roles.get(slug)
    if (!old) s.added_roles.push(slug)
    else {
      const added = r.permissions.map((p) => qPerm(svc, p)).filter((p) => !old.includes(p))
      if (added.length) s.added_role_permissions[slug] = added
    }
  }
  return s
}

export const EXAMPLE = (svc: string): Manifest => ({
  service: svc,
  version: 1,
  resource_types: [
    {
      name: 'ledger',
      description: 'A set of books',
      parents: ['hayat/tenant', 'hayat/group'],
      access: { permission: 'ledger.view', via_placement: true, requires_membership: true },
    },
  ],
  permissions: [
    { key: 'ledger.view', scope: 'ledger', description: 'See entries and balances' },
    { key: 'ledger.post', scope: 'ledger', description: 'Post journal entries' },
  ],
  default_roles: [{ slug: 'accountant', name: 'Accountant', permissions: ['ledger.view', 'ledger.post'] }],
})
