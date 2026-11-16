import type { ReactNode } from 'react'
import { Pill } from '@/shared/ui/bits'
import { cn } from '@/shared/lib/utils'
import { manifestYaml, qPerm, qType, type Manifest, type Summary } from '../contract/manifest'
import { YamlBlock } from './YamlCode'

function Chip({ children }: { children: ReactNode }) {
  return <span className="inline-flex h-[22px] items-center rounded-md bg-muted px-1.5 font-mono text-[11px]">{children}</span>
}

function NewPill() {
  return <Pill className="h-4 border-success/30 px-1 text-[9px] text-success">New</Pill>
}

function Block({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div>
      <h3 className="mb-2 text-[11px] font-semibold tracking-wide text-faint uppercase">{title}</h3>
      <div className="overflow-hidden rounded-[10px] border">{children}</div>
    </div>
  )
}

const rowClass = (isNew: boolean) => cn('border-t px-3 py-2.5 text-[12px] first:border-t-0', isNew && 'bg-success/10 shadow-[inset_3px_0_0_var(--success)]')

/**
 * A contract version read as a form: its types, permissions and suggested
 * roles, with what it added over the version before marked New.
 */
export function ManifestForm({ manifest: m, summary: s }: { manifest: Manifest; summary: Summary }) {
  const svc = m.service
  const newTypes = new Set(s.added_types ?? [])
  const newPerms = new Set(s.added_permissions ?? [])
  const newRoles = new Set(s.added_roles ?? [])
  const perms = m.permissions ?? []
  const roles = m.default_roles ?? []
  return (
    <div className="flex flex-col gap-5 p-4">
      <Block title="Resource types">
        {(m.resource_types ?? []).length === 0 && <p className="px-3 py-2.5 text-[12px] text-faint">None</p>}
        {(m.resource_types ?? []).map((t) => {
          const key = qType(svc, t.name)
          const addedParents = s.added_parents?.[key] ?? []
          return (
            <div key={key} className={cn(rowClass(newTypes.has(key)), 'flex flex-col gap-1.5')}>
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-mono text-[13px] font-semibold">{key}</span>
                {newTypes.has(key) && <NewPill />}
                {t.description && <span className="text-muted-foreground">{t.description}</span>}
              </div>
              <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-faint">
                Filed under
                {t.parents.map((p) => (
                  <span key={p} className="inline-flex items-center gap-1">
                    <Chip>{qType(svc, p)}</Chip>
                    {addedParents.includes(qType(svc, p)) && !newTypes.has(key) && <NewPill />}
                  </span>
                ))}
              </div>
              {t.access && (
                <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-faint">
                  Opening one needs <Chip>{t.access.permission}</Chip>
                  {t.access.via_placement && <Chip>Group rosters can open</Chip>}
                  {t.access.requires_membership && <Chip>Tenant members only</Chip>}
                </div>
              )}
            </div>
          )
        })}
      </Block>

      <Block title="Permissions">
        {perms.length === 0 && <p className="px-3 py-2.5 text-[12px] text-faint">None</p>}
        {perms.map((p) => {
          const key = qPerm(svc, p.key)
          return (
            <div key={key} className={cn(rowClass(newPerms.has(key)), 'grid grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)_minmax(0,1.6fr)] items-center gap-3 max-[700px]:grid-cols-1 max-[700px]:gap-1')}>
              <span className="flex min-w-0 items-center gap-1.5">
                <span className="truncate font-mono">{p.key}</span>
                {newPerms.has(key) && <NewPill />}
              </span>
              <span className="truncate font-mono text-muted-foreground">{p.scope}</span>
              <span className="truncate text-muted-foreground">{p.label || p.description || ''}</span>
            </div>
          )
        })}
      </Block>

      <Block title="Suggested roles">
        {roles.length === 0 && <p className="px-3 py-2.5 text-[12px] text-faint">None</p>}
        {roles.map((r) => {
          const slug = `${svc}.${r.slug}`
          const addedPerms = s.added_role_permissions?.[slug] ?? []
          return (
            <div key={slug} className={cn(rowClass(newRoles.has(slug)), 'flex flex-col gap-1.5')}>
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-semibold">{r.name || r.slug}</span>
                {newRoles.has(slug) && <NewPill />}
                {(r.applies_to ?? []).length > 0 && <span className="text-faint">at {(r.applies_to ?? []).join(', ')}</span>}
              </div>
              <div className="flex flex-wrap items-center gap-1">
                {r.permissions.map((p) => (
                  <span key={p} className="inline-flex items-center gap-1">
                    <Chip>{p}</Chip>
                    {addedPerms.includes(qPerm(svc, p)) && !newRoles.has(slug) && <NewPill />}
                  </span>
                ))}
              </div>
            </div>
          )
        })}
      </Block>
    </div>
  )
}

/** The same version as the YAML it was written in. */
export function ManifestYaml({ manifest }: { manifest: Manifest }) {
  return <YamlBlock text={manifestYaml(manifest)} className="max-h-[560px]" />
}
