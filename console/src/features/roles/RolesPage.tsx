import { useMemo, useState } from 'react'
import { useSearchParams } from 'react-router'
import { SearchX } from 'lucide-react'
import type { Role } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { EmptyState, ErrorState, LoadingState, SearchInput } from '@/shared/ui/bits'
import { cn } from '@/shared/lib/utils'
import { Sheet, SheetContent } from '@/shared/ui/sheet'
import { DeleteRoleDialog } from './components/DeleteRoleDialog'
import { HowRolesWork } from './components/HowRolesWork'
import { ServiceMark } from './components/marks'
import { RoleCard } from './components/RoleCard'
import { RoleDetail } from './components/RoleDetail'
import { RolesHeader } from './components/RolesHeader'
import { useRolesData } from './hooks/use-roles-data'
import { permLabel, sourceOf, type Catalog, type RoleSource } from './lib/roles'

const SOURCES: { id: 'all' | RoleSource; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'builtin', label: 'Built-in' },
  { id: 'service', label: 'From services' },
  { id: 'custom', label: 'Yours' },
]

/** A role matches a search by its name, description, identifier, or any permission it holds. */
function matches(role: Role, q: string, catalog: Catalog) {
  if (!q) return true
  const perms = catalog.permissions.filter((p) => role.permissions.includes(p.key))
  const text = [role.name, role.description, role.slug, ...perms.flatMap((p) => [p.key, permLabel(p), p.description])].join(' ').toLowerCase()
  return q
    .toLowerCase()
    .split(/\s+/)
    .every((w) => text.includes(w))
}

/**
 * The roles: a list you can narrow by who owns a role, which service's
 * permissions it uses, or a search. Picking one opens it in full in a side
 * drawer, over the list. The selection and filters live in the address, so a
 * link (or a refresh) lands on them.
 */
export default function RolesPage() {
  const { roles, catalog, services, error, loading, reload } = useRolesData()
  const [params, setParams] = useSearchParams()
  const [q, setQ] = useState('')
  const [deleting, setDeleting] = useState<Role | null>(null)

  const source = (params.get('source') ?? 'all') as 'all' | RoleSource
  const service = params.get('service') ?? ''

  const counts = useMemo(() => {
    const c: Record<string, number> = { all: roles.length, builtin: 0, service: 0, custom: 0 }
    for (const r of roles) c[sourceOf(r)]++
    return c
  }, [roles])

  const visible = useMemo(() => {
    if (!catalog) return []
    const servicePerms = new Set(catalog.permissions.filter((p) => p.service === service).map((p) => p.key))
    return roles.filter(
      (r) =>
        (source === 'all' || sourceOf(r) === source) &&
        (!service || r.permissions.some((p) => servicePerms.has(p))) &&
        matches(r, q.trim(), catalog),
    )
  }, [roles, catalog, source, service, q])

  const selected = roles.find((r) => r.id === params.get('role'))

  const linkWith = (changes: Record<string, string | null>) => {
    const next = new URLSearchParams(params)
    for (const [k, v] of Object.entries(changes)) {
      if (v === null || v === '') next.delete(k)
      else next.set(k, v)
    }
    const s = next.toString()
    return { search: s ? `?${s}` : '' }
  }
  const setFilter = (changes: Record<string, string | null>) => setParams(linkWith(changes).search.slice(1), { replace: true })

  return (
    <>
      <RolesHeader />
      <HowRolesWork />

      {error ? (
        <div className="mt-6">
          <ErrorState message={error} onRetry={reload} />
        </div>
      ) : loading && !catalog ? (
        <LoadingState />
      ) : catalog ? (
        <>
          <div className="mt-5 flex flex-wrap items-center gap-2.5">
            <SearchInput value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search roles or permissions, e.g. “connection”" aria-label="Search roles" className="min-w-[240px] flex-1" />
            <div role="group" aria-label="Who owns the role" className="flex gap-0.5 rounded-[10px] bg-muted p-[3px]">
              {SOURCES.map((s) => (
                <button
                  key={s.id}
                  type="button"
                  aria-pressed={source === s.id}
                  onClick={() => setFilter({ source: s.id === 'all' ? null : s.id })}
                  className={cn(
                    'h-[30px] rounded-lg px-3 text-[12.5px] transition-colors',
                    source === s.id ? 'bg-background font-semibold shadow-xs' : 'text-muted-foreground hover:text-foreground',
                  )}
                >
                  {s.label} <span className="ml-0.5 font-normal text-muted-foreground">{counts[s.id]}</span>
                </button>
              ))}
            </div>
          </div>
          {services.length > 1 && (
            <div role="group" aria-label="Uses permissions from" className="mt-2.5 flex flex-wrap items-center gap-1.5">
              <span className="mr-1 text-[12px] text-muted-foreground">Uses permissions from</span>
              {[{ id: '', name: 'Any service' }, ...services].map((s) => {
                const on = service === s.id
                const meta = services.find((m) => m.id === s.id)
                return (
                  <button
                    key={s.id || 'any'}
                    type="button"
                    aria-pressed={on}
                    onClick={() => setFilter({ service: s.id || null })}
                    className={cn(
                      'inline-flex h-7 items-center gap-1.5 rounded-lg border px-2.5 text-[12px] transition-colors',
                      meta && 'pl-1',
                      on ? 'border-primary bg-primary/8 font-semibold' : 'text-muted-foreground hover:text-foreground',
                    )}
                  >
                    {meta && <ServiceMark service={meta} size="xs" />}
                    {s.name}
                  </button>
                )
              })}
            </div>
          )}

          <div className="mt-5 flex flex-col gap-2">
            {visible.length === 0 ? (
              <EmptyState icon={SearchX} title="No role matches">
                Try another word, or clear the filters above.
              </EmptyState>
            ) : (
              visible.map((r) => <RoleCard key={r.id} role={r} catalog={catalog} services={services} to={linkWith({ role: r.id })} selected={r.id === selected?.id} />)
            )}
            <p className="mt-1 px-0.5 text-[12px] text-muted-foreground">Grants are counted across every tenant on this instance.</p>
          </div>

          <Sheet open={!!selected} onOpenChange={(open) => !open && setFilter({ role: null })}>
            <SheetContent side="right" className="w-full gap-0 overflow-y-auto p-0 sm:max-w-[460px] data-[side=right]:sm:max-w-[460px]">
              {selected && <RoleDetail key={selected.id} role={selected} catalog={catalog} services={services} onDelete={() => setDeleting(selected)} />}
            </SheetContent>
          </Sheet>
        </>
      ) : null}

      {deleting && (
        <DeleteRoleDialog
          role={deleting}
          onClose={() => setDeleting(null)}
          onDeleted={() => {
            setDeleting(null)
            setFilter({ role: null })
            void reload()
          }}
        />
      )}
    </>
  )
}
