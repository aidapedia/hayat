import type { ReactNode } from 'react'
import { Lock, Plus, Trash2 } from 'lucide-react'
import { cn } from '@/shared/lib/utils'
import { Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Checkbox } from '@/shared/ui/checkbox'
import { Input } from '@/shared/ui/input'
import { Label } from '@/shared/ui/label'
import { Combobox } from '@/shared/ui/combobox'
import { Switch } from '@/shared/ui/switch'
import { PLATFORM, qPerm, qType, uid, type Baseline, type Draft, type KnownType, type PermRow, type Problem, type RoleRow, type TypeRow } from './manifest'

type Props = {
  draft: Draft
  onChange: (d: Draft) => void
  known: KnownType[]
  base: Baseline
  problems: Map<string, Problem>
  attempted: boolean
}

type Option = { value: string; label: string; desc?: string }

const toggle = (list: string[], v: string) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v])
const swap = (list: string[], from: string, to: string) => list.map((x) => (x === from ? to : x))
const patch = <T extends { uid: string }>(rows: T[], id: string, p: Partial<T>) => rows.map((r) => (r.uid === id ? { ...r, ...p } : r))

/**
 * The contract as a form: the types a service files in Hayat's tree, the
 * permissions that mean something on them, and roles it suggests. What the live
 * version already declares is locked where the server would refuse a change.
 */
export function ContractForm({ draft: d, onChange, known, base, problems, attempted }: Props) {
  const svc = d.service
  const err = (key: string) => {
    const p = problems.get(key)
    return p && (!p.empty || attempted) ? p.message : undefined
  }

  // Every node type a manifest may name: this contract's own, then the catalog's.
  const ownTypes: Option[] = d.types
    .filter((t) => t.name.trim())
    .map((t) => ({ value: t.name.trim(), label: qType(svc, t.name.trim()), desc: t.description || 'Declared in this contract' }))
  const otherTypes: Option[] = known
    .filter((k) => k.service !== svc)
    .sort((a, b) => Number(b.service === PLATFORM) - Number(a.service === PLATFORM) || a.key.localeCompare(b.key))
    .map((k) => ({ value: k.key, label: k.key, desc: k.description }))
  const allTypes = [...ownTypes, ...otherTypes]
  const parentTypes = allTypes.filter((o) => o.value !== 'hayat/root' && o.value !== 'hayat/service')
  const permOptions: Option[] = d.permissions
    .filter((p) => p.key.trim())
    .map((p) => ({ value: p.key.trim(), label: qPerm(svc, p.key.trim()), desc: p.description }))

  // ----- types -----
  const setType = (id: string, p: Partial<TypeRow>) => onChange({ ...d, types: patch(d.types, id, p) })
  const renameType = (t: TypeRow, name: string) => {
    const from = t.name.trim()
    const to = name.trim()
    const follow = from && from !== to
    onChange({
      ...d,
      types: d.types.map((x) => ({ ...(x.uid === t.uid ? { ...x, name } : x), parents: follow ? swap(x.parents, from, to) : x.parents })),
      permissions: follow ? d.permissions.map((p) => (p.scope === from ? { ...p, scope: to } : p)) : d.permissions,
      roles: follow ? d.roles.map((r) => ({ ...r, appliesTo: swap(r.appliesTo, from, to) })) : d.roles,
    })
  }
  const removeType = (t: TypeRow) => {
    const name = t.name.trim()
    onChange({
      ...d,
      types: d.types.filter((x) => x.uid !== t.uid).map((x) => ({ ...x, parents: x.parents.filter((p) => p !== name) })),
      permissions: d.permissions.map((p) => (p.scope === name ? { ...p, scope: '' } : p)),
      roles: d.roles.map((r) => ({ ...r, appliesTo: r.appliesTo.filter((a) => a !== name) })),
    })
  }
  const addType = () =>
    onChange({ ...d, types: [...d.types, { uid: uid(), published: false, name: '', description: '', parents: [], access: null, degradable: false }] })

  // ----- permissions -----
  const setPerm = (id: string, p: Partial<PermRow>) => onChange({ ...d, permissions: patch(d.permissions, id, p) })
  const renamePerm = (p: PermRow, key: string) => {
    const from = p.key.trim()
    const to = key.trim()
    const follow = from && from !== to
    onChange({
      ...d,
      permissions: patch(d.permissions, p.uid, { key }),
      types: follow ? d.types.map((t) => (t.access?.permission === from ? { ...t, access: { ...t.access, permission: to } } : t)) : d.types,
      roles: follow ? d.roles.map((r) => ({ ...r, permissions: swap(r.permissions, from, to) })) : d.roles,
    })
  }
  const removePerm = (p: PermRow) => {
    const key = p.key.trim()
    onChange({
      ...d,
      permissions: d.permissions.filter((x) => x.uid !== p.uid),
      types: d.types.map((t) => (t.access?.permission === key ? { ...t, access: { ...t.access, permission: '' } } : t)),
      roles: d.roles.map((r) => ({ ...r, permissions: r.permissions.filter((x) => x !== key) })),
    })
  }
  const addPerm = () => {
    const only = d.types.length === 1 ? d.types[0].name.trim() : ''
    onChange({ ...d, permissions: [...d.permissions, { uid: uid(), published: false, key: '', scope: only, description: '' }] })
  }

  // ----- roles -----
  const setRole = (id: string, p: Partial<RoleRow>) => onChange({ ...d, roles: patch(d.roles, id, p) })
  const removeRole = (id: string) => onChange({ ...d, roles: d.roles.filter((r) => r.uid !== id) })
  const addRole = () => onChange({ ...d, roles: [...d.roles, { uid: uid(), published: false, slug: '', name: '', description: '', permissions: [], appliesTo: [] }] })

  return (
    <div className="flex flex-col gap-8">
      <Block
        title="Resource types"
        desc="The things your service keeps in Hayat's tree, like a ledger or a project. People reach one through grants on it or on anything above it."
        add="Add a resource type"
        onAdd={addType}
        empty={d.types.length === 0 && 'No resource types yet. Add the first thing your service keeps.'}
      >
        {d.types.map((t) => {
          const live = t.published ? base.types.get(qType(svc, t.name.trim())) : undefined
          const lockedParents = (live?.parents ?? []).map((p) => (p.startsWith(`${svc}/`) ? p.slice(svc.length + 1) : p))
          const accessLocked = !!live?.access
          const access = t.access
          // Turning the rule on suggests a permission that applies at this type.
          const suggested = d.permissions.find((p) => p.key.trim() && p.scope === t.name.trim())?.key.trim() ?? ''
          return (
            <Card key={t.uid} live={t.published} title={t.name.trim() ? qType(svc, t.name.trim()) : 'New resource type'} onRemove={() => removeType(t)}>
              <div className="grid grid-cols-2 gap-3 max-[700px]:grid-cols-1">
                <FormRow label="Name" error={err(`${t.uid}.name`)} locked={t.published}>
                  <Prefixed prefix={`${svc}/`}>
                    <Input value={t.name} onChange={(e) => renameType(t, e.target.value)} placeholder="ledger" disabled={t.published} aria-invalid={!!err(`${t.uid}.name`)} className="h-8 font-mono text-[13px]" />
                  </Prefixed>
                </FormRow>
                <FormRow label="Description">
                  <Input value={t.description} onChange={(e) => setType(t.uid, { description: e.target.value })} placeholder="What one of these is" className="h-8 text-[13px]" />
                </FormRow>
              </div>
              <FormRow label="Can be filed under" hint="Where one can sit in the tree. Pick at least one." error={err(`${t.uid}.parents`)}>
                <Chips options={parentTypes} selected={t.parents} locked={lockedParents} onToggle={(v) => setType(t.uid, { parents: toggle(t.parents, v) })} />
              </FormRow>
              <div className="rounded-[10px] border bg-muted/30 p-3">
                <Label className="flex cursor-pointer items-start gap-3 font-normal">
                  <Switch
                    checked={!!access}
                    disabled={accessLocked}
                    onCheckedChange={(on) => setType(t.uid, { access: on ? { permission: suggested, viaPlacement: false, requiresMembership: false } : null })}
                    className="mt-0.5"
                  />
                  <span className="min-w-0">
                    <span className="flex items-center gap-1.5 text-[13px] font-semibold">
                      Access rule {accessLocked && <Lock className="size-3 text-faint" />}
                    </span>
                    <span className="block text-[12px] text-muted-foreground">
                      {accessLocked ? 'Live, so it can no longer change.' : 'Name the permission that means "can open one", and how people get it. Once live, it can\'t change.'}
                    </span>
                  </span>
                </Label>
                {access && (
                  <div className="mt-3 flex flex-col gap-3 pl-11 max-[600px]:pl-0">
                    <FormRow label="Opening one needs" error={err(`${t.uid}.access`)}>
                      {permOptions.length === 0 ? (
                        <p className="text-[12px] text-faint">Add a permission below first, then pick it here.</p>
                      ) : (
                        <OptionPick value={access.permission} options={permOptions} placeholder="Pick a permission" disabled={accessLocked} onChange={(v) => setType(t.uid, { access: { ...access, permission: v } })} invalid={!!err(`${t.uid}.access`)} />
                      )}
                    </FormRow>
                    <Tick
                      checked={access.viaPlacement}
                      disabled={accessLocked}
                      onChange={(v) => setType(t.uid, { access: { ...access, viaPlacement: v } })}
                      label="Everyone in a group above it can open it"
                      hint="Without a grant: everyone in the group it is filed in (its sub-teams too), and anyone directly on a group further up. A sibling team never gets it."
                    />
                    <Tick
                      checked={access.requiresMembership}
                      disabled={accessLocked}
                      onChange={(v) => setType(t.uid, { access: { ...access, requiresMembership: v } })}
                      label="Only members of the tenant"
                      hint="Someone who isn't on the tenant's roster gets nothing here, even with a grant."
                    />
                  </div>
                )}
              </div>
            </Card>
          )
        })}
      </Block>

      <Block
        title="Permissions"
        desc="What someone may do. Each one applies at one type of node and to everything filed below it. Your service checks these keys."
        add="Add a permission"
        onAdd={addPerm}
        empty={d.permissions.length === 0 && 'No permissions yet. A permission is what your service asks Hayat about, like ledger.view.'}
      >
        {d.permissions.length > 0 && (
          <div className="divide-y rounded-xl border">
            <div className="grid grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_minmax(0,1.4fr)_32px] gap-3 px-3 py-2 text-[11px] font-semibold text-muted-foreground max-[760px]:hidden">
              <span>Key</span>
              <span>Applies at</span>
              <span>Description</span>
              <span />
            </div>
            {d.permissions.map((p) => {
              const keyErr = err(`${p.uid}.key`)
              const scopeErr = err(`${p.uid}.scope`)
              return (
                <div key={p.uid} className="px-3 py-2.5">
                  <div className="grid grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_minmax(0,1.4fr)_32px] items-start gap-3 max-[760px]:grid-cols-1">
                    <div className="flex flex-col gap-1">
                      <Prefixed prefix={`${svc}.`}>
                        <Input value={p.key} onChange={(e) => renamePerm(p, e.target.value)} placeholder="ledger.view" disabled={p.published} aria-invalid={!!keyErr} aria-label="Key" className="h-8 font-mono text-[13px]" />
                      </Prefixed>
                      {!p.published && (
                        <span>
                          <Pill className="border-primary/30 text-primary">New</Pill>
                        </span>
                      )}
                    </div>
                    <OptionPick value={p.scope} options={allTypes} placeholder="Pick a type" disabled={p.published} onChange={(v) => setPerm(p.uid, { scope: v })} invalid={!!scopeErr} label="Applies at" />
                    <Input value={p.description} onChange={(e) => setPerm(p.uid, { description: e.target.value })} placeholder="What it lets someone do" aria-label="Description" className="h-8 text-[13px]" />
                    <RemoveButton live={p.published} label={p.key || 'permission'} onClick={() => removePerm(p)} />
                  </div>
                  {(keyErr || scopeErr) && <p className="mt-1.5 text-[12px] text-destructive">{[keyErr, scopeErr].filter(Boolean).join(' · ')}</p>}
                </div>
              )
            })}
          </div>
        )}
      </Block>

      <Block
        title="Suggested roles"
        desc="Optional. Ready-made sets of permissions that tenant admins can grant, so they don't have to build their own."
        add="Add a role"
        onAdd={addRole}
        empty={d.roles.length === 0 && 'No suggested roles. Admins can still make their own from your permissions.'}
      >
        {d.roles.map((r) => {
          const lockedPerms = (base.roles.get(`${svc}.${r.slug.trim()}`) ?? []).map((p) => (p.startsWith(`${svc}.`) ? p.slice(svc.length + 1) : p))
          return (
            <Card key={r.uid} live={r.published} title={r.name.trim() || (r.slug.trim() ? `${svc}.${r.slug.trim()}` : 'New role')} onRemove={() => removeRole(r.uid)}>
              <div className="grid grid-cols-2 gap-3 max-[700px]:grid-cols-1">
                <FormRow label="Id" error={err(`${r.uid}.slug`)} locked={r.published}>
                  <Prefixed prefix={`${svc}.`}>
                    <Input value={r.slug} onChange={(e) => setRole(r.uid, { slug: e.target.value })} placeholder="accountant" disabled={r.published} aria-invalid={!!err(`${r.uid}.slug`)} className="h-8 font-mono text-[13px]" />
                  </Prefixed>
                </FormRow>
                <FormRow label="Name people see">
                  <Input value={r.name} onChange={(e) => setRole(r.uid, { name: e.target.value })} placeholder="Accountant" className="h-8 text-[13px]" />
                </FormRow>
              </div>
              <FormRow label="Description">
                <Input value={r.description} onChange={(e) => setRole(r.uid, { description: e.target.value })} placeholder="Who should get this role" className="h-8 text-[13px]" />
              </FormRow>
              <FormRow label="Grants" error={err(`${r.uid}.permissions`)}>
                {permOptions.length === 0 ? (
                  <p className="text-[12px] text-faint">Add permissions above, then tick the ones this role grants.</p>
                ) : (
                  <div className="divide-y rounded-[10px] border">
                    {permOptions.map((o) => {
                      const locked = lockedPerms.includes(o.value)
                      return (
                        <Label key={o.value} className={cn('flex cursor-pointer items-start gap-3 px-3 py-2 font-normal hover:bg-accent/50', locked && 'cursor-default')}>
                          <Checkbox checked={r.permissions.includes(o.value)} disabled={locked} onCheckedChange={() => setRole(r.uid, { permissions: toggle(r.permissions, o.value) })} className="mt-0.5" />
                          <span className="min-w-0">
                            <span className="flex items-center gap-1.5 font-mono text-[12px]">
                              {o.label} {locked && <Lock className="size-3 text-faint" />}
                            </span>
                            {o.desc && <span className="block text-[12px] text-faint">{o.desc}</span>}
                          </span>
                        </Label>
                      )
                    })}
                  </div>
                )}
              </FormRow>
              <FormRow label="Can be granted on" hint="Nothing ticked means any type of node." error={err(`${r.uid}.appliesTo`)}>
                <Chips options={allTypes} selected={r.appliesTo} onToggle={(v) => setRole(r.uid, { appliesTo: toggle(r.appliesTo, v) })} />
              </FormRow>
            </Card>
          )
        })}
      </Block>
    </div>
  )
}

// ---------- pieces ----------

function Block({ title, desc, add, onAdd, empty, children }: { title: string; desc: string; add: string; onAdd: () => void; empty: ReactNode; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-3">
      <div>
        <h3 className="text-[13px] font-semibold">{title}</h3>
        <p className="mt-0.5 text-[12px] text-muted-foreground">{desc}</p>
      </div>
      {empty ? <p className="rounded-xl border border-dashed px-4 py-5 text-center text-[12px] text-faint">{empty}</p> : children}
      <div>
        <Button type="button" variant="outline" size="sm" onClick={onAdd}>
          <Plus /> {add}
        </Button>
      </div>
    </div>
  )
}

function Card({ live, title, onRemove, children }: { live: boolean; title: string; onRemove: () => void; children: ReactNode }) {
  return (
    <div className={cn('flex flex-col gap-3 rounded-xl border p-4', !live && 'border-primary/30')}>
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate font-mono text-[13px] font-semibold">{title}</span>
          {live ? <Pill className="border-success/30 text-success">Live</Pill> : <Pill className="border-primary/30 text-primary">New</Pill>}
        </div>
        <RemoveButton live={live} label={title} onClick={onRemove} />
      </div>
      {live && <p className="-mt-1 text-[12px] text-faint">Live in the catalog: you can add to it and reword it, but not remove or rename it.</p>}
      {children}
    </div>
  )
}

/** Remove, or a lock where the server would refuse it: what is live stays. */
function RemoveButton({ live, label, onClick }: { live: boolean; label: string; onClick: () => void }) {
  if (live)
    return (
      <span className="flex size-8 shrink-0 items-center justify-center text-faint" title="Live, so it stays" aria-label="Live, so it stays">
        <Lock className="size-3.5" />
      </span>
    )
  return (
    <Button type="button" variant="ghost" size="icon-sm" onClick={onClick} aria-label={`Remove ${label}`} title={`Remove ${label}`}>
      <Trash2 />
    </Button>
  )
}

function FormRow({ label, hint, error, locked, children }: { label: string; hint?: string; error?: string; locked?: boolean; children: ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <span className="flex items-center gap-1.5 text-[12px] font-semibold text-muted-foreground">
        {label} {locked && <Lock className="size-3 text-faint" />}
      </span>
      {children}
      {error ? <p className="text-[12px] text-destructive">{error}</p> : hint && <p className="text-[12px] text-faint">{hint}</p>}
    </div>
  )
}

/** An input with the namespace shown in front of it, so short names read as what they become. */
function Prefixed({ prefix, children }: { prefix: string; children: ReactNode }) {
  return (
    <div className="flex min-w-0 items-center">
      <span className="flex h-8 shrink-0 items-center rounded-l-lg border border-r-0 bg-muted/60 px-2 font-mono text-[12px] text-faint">{prefix}</span>
      <div className="min-w-0 flex-1 [&_input]:rounded-l-none">{children}</div>
    </div>
  )
}

/** Toggle chips over node types; locked ones are live and can't be unticked. Unknown picks still show, so they can be removed. */
function Chips({ options, selected, locked = [], onToggle }: { options: Option[]; selected: string[]; locked?: string[]; onToggle: (v: string) => void }) {
  const extra = selected.filter((s) => !options.some((o) => o.value === s)).map((s): Option => ({ value: s, label: s, desc: 'Not a known type' }))
  return (
    <div className="flex flex-wrap gap-2">
      {[...options, ...extra].map((o) => {
        const isLocked = locked.includes(o.value)
        const unknown = extra.includes(o)
        return (
          <Label
            key={o.value}
            title={isLocked ? 'Live, so it stays' : o.desc}
            className={cn('flex cursor-pointer items-center gap-2 rounded-[10px] border px-2.5 py-1.5 font-mono text-[12px] font-normal', unknown && 'border-destructive/40 text-destructive', isLocked && 'cursor-default')}
          >
            <Checkbox checked={selected.includes(o.value)} disabled={isLocked} onCheckedChange={() => onToggle(o.value)} />
            {o.label}
            {isLocked && <Lock className="size-3 text-faint" />}
          </Label>
        )
      })}
      {options.length === 0 && extra.length === 0 && <p className="text-[12px] text-faint">No types to pick from yet.</p>}
    </div>
  )
}

function OptionPick({ value, options, placeholder, disabled, invalid, label, onChange }: { value: string; options: Option[]; placeholder: string; disabled?: boolean; invalid?: boolean; label?: string; onChange: (v: string) => void }) {
  const all = value && !options.some((o) => o.value === value) ? [...options, { value, label: value, desc: 'Not declared' }] : options
  return (
    <Combobox
      aria-label={label}
      value={value}
      onChange={onChange}
      options={all.map((o) => ({ value: o.value, label: o.label, keywords: o.desc ? [o.desc] : [] }))}
      placeholder={placeholder}
      disabled={disabled}
      invalid={invalid}
      className="font-mono text-[12px]"
      contentClassName="font-mono **:data-[slot=command-item]:text-[12px]"
    />
  )
}

function Tick({ checked, disabled, onChange, label, hint }: { checked: boolean; disabled?: boolean; onChange: (v: boolean) => void; label: string; hint: string }) {
  return (
    <Label className="flex cursor-pointer items-start gap-3 font-normal">
      <Checkbox checked={checked} disabled={disabled} onCheckedChange={(v) => onChange(v === true)} className="mt-0.5" />
      <span className="min-w-0">
        <span className="block text-[13px]">{label}</span>
        <span className="block text-[12px] text-faint">{hint}</span>
      </span>
    </Label>
  )
}
