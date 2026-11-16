import { useMemo, useState } from 'react'
import { Navigate, useSearchParams } from 'react-router'
import { SearchX } from 'lucide-react'
import type { Role } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { EmptyState, ErrorState, LoadingState, SearchInput } from '@/shared/ui/bits'
import { cn } from '@/shared/lib/utils'
import { Combobox } from '@/shared/ui/combobox'
import { Pagination } from '@/shared/ui/table/Pagination'
import { usePagination } from '@/shared/ui/table/usePagination'
import { HowRolesWork } from './components/HowRolesWork'
import { ServiceMark } from './components/marks'
import { RoleCard } from './components/RoleCard'
import { RolesHeader } from './components/RolesHeader'
import { useRolesData } from './hooks/use-roles-data'
import { permLabel, sourceOf, type Catalog, type RoleSource } from './lib/roles'

const SOURCES: { id: 'all' | RoleSource; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'builtin', label: 'Built-in' },
  { id: 'service', label: 'From services' },
  { id: 'custom', label: 'Yours' },
]

// Radix's Select can't hold an empty value, so "any service" has a name of its own.
const ANY_SERVICE = 'any'

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
 * permissions it uses, or a search. Picking one opens it on its own page
 * (/roles/:id). The filters live in the address, so a link (or a refresh)
 * lands on them.
 */
export default function RolesPage() {
  const { roles, catalog, services, error, loading, reload } = useRolesData()
  const [params, setParams] = useSearchParams()
  const [q, setQ] = useState('')

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

  const { pageRows, pager } = usePagination(visible, { pageSize: 10, resetKey: `${source}|${service}|${q.trim()}` })

  // Filters live in the address, so a link (or a refresh) keeps them.
  const setFilter = (changes: Record<string, string | null>) => {
    const next = new URLSearchParams(params)
    for (const [k, v] of Object.entries(changes)) {
      if (v === null || v === '') next.delete(k)
      else next.set(k, v)
    }
    setParams(next, { replace: true })
  }

  // The detail used to be a drawer over this list: ?role=<id> still lands on it.
  const legacy = params.get('role')
  if (legacy) return <Navigate to={`/roles/${legacy}`} replace />

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
            {services.length > 1 && (
              <Combobox
                aria-label="Uses permissions from"
                value={service || ANY_SERVICE}
                onChange={(v) => setFilter({ service: v === ANY_SERVICE ? null : v })}
                options={[
                  { value: ANY_SERVICE, label: 'Any service' },
                  ...services.map((s) => ({ value: s.id, label: s.name, icon: <ServiceMark service={s} size="xs" /> })),
                ]}
                searchPlaceholder="Search services…"
                className="h-9 w-auto min-w-[200px] text-[13px]"
              />
            )}
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
          <p className="mt-4 px-0.5 text-[12px] text-muted-foreground">
            <strong className="font-semibold text-foreground">{visible.length}</strong> {visible.length === 1 ? 'role' : 'roles'} · grants are counted across every tenant on this instance.
          </p>
          <div className="mt-2 flex flex-col gap-2">
            {visible.length === 0 ? (
              <EmptyState icon={SearchX} title="No role matches">
                Try another word, or clear the filters above.
              </EmptyState>
            ) : (
              pageRows.map((r) => <RoleCard key={r.id} role={r} catalog={catalog} services={services} to={`/roles/${r.id}`} />)
            )}
            {visible.length > 0 && <Pagination {...pager} pageSizeOptions={[10, 25, 50]} className="px-0.5" />}          </div>
        </>
      ) : null}
    </>
  )
}
