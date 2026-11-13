import { Link, useParams } from 'react-router'
import { ArrowRight, Check } from 'lucide-react'
import { ErrorState, LoadingState } from '@/shared/ui/bits'
import { cn } from '@/shared/lib/utils'
import { ServiceMark } from './components/marks'
import { RolesHeader } from './components/RolesHeader'
import { useRolesData } from './hooks/use-roles-data'
import { PLATFORM, permLabel, plural, sourceOf, typeLabel } from './lib/roles'

/**
 * The roles seen from one service: every permission it enforces against every
 * role that uses any of them, so "who can do X in this service" is one look.
 * The service is part of the address.
 */
export default function RolesByServicePage() {
  const { roles, catalog, services, error, loading, reload } = useRolesData()
  const { service: picked } = useParams()

  if (error) {
    return (
      <>
        <RolesHeader />
        <div className="mt-6">
          <ErrorState message={error} onRetry={reload} />
        </div>
      </>
    )
  }
  if (loading && !catalog) {
    return (
      <>
        <RolesHeader />
        <LoadingState />
      </>
    )
  }
  if (!catalog) return null

  const current = services.find((s) => s.id === picked) ?? services.find((s) => s.id !== PLATFORM) ?? services[0]
  const perms = catalog.permissions.filter((p) => p.service === current.id)
  const keys = new Set(perms.map((p) => p.key))
  const columns = roles.filter((r) => r.permissions.some((p) => keys.has(p)))
  const types = catalog.types.filter((t) => t.service === current.id)

  return (
    <>
      <RolesHeader />

      <div className="mt-5 grid grid-cols-[repeat(auto-fill,minmax(300px,1fr))] gap-3">
        {services.map((s) => {
          const on = s.id === current.id
          const sKeys = new Set(catalog.permissions.filter((p) => p.service === s.id).map((p) => p.key))
          const nRoles = roles.filter((r) => r.permissions.some((p) => sKeys.has(p))).length
          const nTypes = catalog.types.filter((t) => t.service === s.id).length
          return (
            <Link
              key={s.id}
              to={`/roles/by-service/${s.id}`}
              aria-current={on ? 'page' : undefined}
              className={cn(
                'flex flex-col gap-2.5 rounded-2xl border bg-background px-[18px] py-4 transition-colors hover:border-primary/40',
                on && 'border-primary ring-3 ring-primary/12 hover:border-primary',
              )}
            >
              <div className="flex items-center gap-3">
                <ServiceMark service={s} size="lg" />
                <div className="flex min-w-0 flex-col">
                  <span className="text-[15px] font-semibold">{s.name}</span>
                  <span className="text-[12px] text-muted-foreground">{s.id === PLATFORM ? 'Platform · built in' : `Service · contract v${s.version}`}</span>
                </div>
              </div>
              {s.description && <p className="line-clamp-2 text-[12.5px] text-muted-foreground">{s.description}</p>}
              <div className="flex flex-wrap gap-4 text-[12px] text-muted-foreground">
                <span>
                  <strong className="font-semibold text-foreground">{sKeys.size}</strong> permissions
                </span>
                <span>
                  <strong className="font-semibold text-foreground">{nRoles}</strong> roles use it
                </span>
                <span>
                  <strong className="font-semibold text-foreground">{nTypes}</strong> node types
                </span>
              </div>
            </Link>
          )
        })}
      </div>

      <section aria-labelledby="matrix" className="mt-9">
        <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 id="matrix" className="text-[15px] font-semibold tracking-[-0.2px]">
              What each role allows in {current.name}
            </h2>
            <p className="mt-0.5 text-[12px] text-muted-foreground">
              Each row is one permission {current.name} enforces. A tick means the role includes it. Roles from elsewhere that borrow {current.name}'s permissions show too.
            </p>
          </div>
        </div>
        {perms.length === 0 ? (
          <p className="text-[13px] text-muted-foreground">{current.name} has no active contract yet, so it enforces no permissions.</p>
        ) : (
          <div className="overflow-x-auto rounded-2xl border">
            <table className="w-full min-w-[760px] border-separate border-spacing-0 text-[12.5px]">
              <thead>
                <tr className="bg-muted/40">
                  <th scope="col" className="w-[300px] border-b px-3.5 py-3 text-left font-semibold text-muted-foreground">
                    Permission
                  </th>
                  <th scope="col" className="w-[110px] border-b px-2.5 py-3 text-left font-semibold text-muted-foreground">
                    Checked on
                  </th>
                  {columns.map((r) => (
                    <th key={r.id} scope="col" className="min-w-[96px] border-b px-1.5 py-2.5 align-bottom">
                      <Link to={`/roles?role=${r.id}`} className="flex flex-col items-center gap-1.5 hover:underline">
                        <span className="text-[10px] font-semibold tracking-wide text-muted-foreground uppercase">
                          {sourceOf(r) === 'builtin' ? 'Built-in' : sourceOf(r) === 'custom' ? 'Yours' : (services.find((s) => s.id === r.service)?.name ?? r.service)}
                        </span>
                        <span className="text-center text-[12px] leading-tight font-semibold">{r.name}</span>
                      </Link>
                    </th>
                  ))}
                  <th scope="col" className="w-[84px] border-b px-3.5 py-3 text-right font-semibold text-muted-foreground">
                    Used by
                  </th>
                </tr>
              </thead>
              <tbody>
                {perms.map((p) => {
                  const used = columns.filter((r) => r.permissions.includes(p.key)).length
                  return (
                    <tr key={p.key} className="transition-colors hover:bg-muted/30 [&:last-child>*]:border-b-0">
                      <th scope="row" className="border-b px-3.5 py-2.5 text-left align-top font-normal">
                        <span className="flex flex-col gap-0.5">
                          <span className="font-semibold">{permLabel(p)}</span>
                          {p.description && <span className="text-[12px] text-muted-foreground">{p.description}</span>}
                          <span className="font-mono text-[11px] text-muted-foreground">{p.key}</span>
                        </span>
                      </th>
                      <td className="border-b px-2.5 py-2.5 align-top">
                        <span className="inline-flex h-[22px] items-center rounded-md bg-muted px-2 text-[11.5px]">{typeLabel(p.scope)}</span>
                      </td>
                      {columns.map((r) => {
                        const on = r.permissions.includes(p.key)
                        return (
                          <td key={r.id} className="border-b px-1.5 py-2.5 text-center align-top">
                            {on ? (
                              <span role="img" aria-label={`${r.name} includes ${permLabel(p)}`} className="inline-flex size-6 items-center justify-center text-primary">
                                <Check className="size-[18px]" strokeWidth={2.5} />
                              </span>
                            ) : (
                              <span role="img" aria-label={`${r.name} doesn't include ${permLabel(p)}`} className="inline-flex size-6 items-center justify-center text-muted-foreground/40">
                                –
                              </span>
                            )}
                          </td>
                        )
                      })}
                      <td className={cn('border-b px-3.5 py-2.5 text-right align-top', used === 0 ? 'font-semibold text-warning' : 'text-muted-foreground')}>
                        {used === 0 ? 'No role' : plural(used, 'role')}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {types.length > 0 && (
        <section aria-labelledby="types" className="mt-9">
          <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
            <div>
              <h2 id="types" className="text-[15px] font-semibold tracking-[-0.2px]">
                Where {current.name} puts things in the tree
              </h2>
              <p className="mt-0.5 text-[12px] text-muted-foreground">Node types {current.name} adds. A role granted on one reaches everything filed below it.</p>
            </div>
            {current.id !== PLATFORM && (
              <Link to={`/services/${current.id}`} className="inline-flex items-center gap-1.5 text-[12.5px] font-medium text-primary hover:underline">
                Open {current.name} in Services <ArrowRight className="size-3.5" />
              </Link>
            )}
          </div>
          <div className="grid grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-3">
            {types.map((t) => (
              <div key={t.key} className="flex flex-col gap-1.5 rounded-xl border px-3.5 py-3">
                <span className="text-[13px] font-semibold">{typeLabel(t.key)}</span>
                <span className="font-mono text-[11px] text-muted-foreground">{t.key}</span>
                {t.description && <span className="text-[12px] text-muted-foreground">{t.description}</span>}
                <span className="text-[11.5px] text-muted-foreground">Lives under: {t.parents.length ? t.parents.map(typeLabel).join(', ') : 'nothing (a root)'}</span>
              </div>
            ))}
          </div>
        </section>
      )}
    </>
  )
}
