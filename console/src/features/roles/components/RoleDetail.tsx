import { Link } from 'react-router'
import { Check, Copy, Info, Pencil, Trash2 } from 'lucide-react'
import type { Role } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { ErrorState, LoadingState } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { SheetDescription, SheetTitle } from '@/shared/ui/sheet'
import { groupByService, permLabel, plural, sourceOf, type Catalog, type ServiceMeta } from '../lib/roles'
import { RoleSourceBadge, ServiceMark } from './marks'
import { RoleGrantsList, usageLine, useRoleGrants } from './RoleGrants'
import { TypeTree } from './TypeTree'

const SHOWN_GRANTS = 5

/** Why a role can or can't be changed, said once, where the buttons are. */
function noteFor(role: Role, services: ServiceMeta[]): string {
  switch (sourceOf(role)) {
    case 'builtin':
      return "Built-in roles can be tuned but not deleted. A change applies to every grant of this role at once."
    case 'service': {
      const name = services.find((s) => s.id === role.service)?.name ?? role.service
      return `${name}'s contract defines this role, so it changes only with a new contract version. Duplicate it to make your own variation.`
    }
    default:
      return role.grantCount > 0 ? `Granted ${plural(role.grantCount, 'time')}. Remove those grants before you can delete it.` : ''
  }
}

/**
 * The selected role in full: what it lets someone do, where it can be granted,
 * and who holds it. It is the body of the roles page's side drawer.
 */
export function RoleDetail({ role, catalog, services, onDelete }: { role: Role; catalog: Catalog; services: ServiceMeta[]; onDelete: () => void }) {
  const src = sourceOf(role)
  const groups = groupByService(role.permissions, catalog, services)
  const note = noteFor(role, services)
  const { data, error, loading, reload } = useRoleGrants(role.id, role.grantCount)

  return (
    <div className="flex flex-col">
      <div className="flex flex-col gap-2.5 border-b px-5 pt-5 pb-4 pr-12">
        <div className="flex flex-wrap items-center gap-2">
          <RoleSourceBadge role={role} services={services} />
          <span className="font-mono text-[11.5px] text-muted-foreground">{role.slug}</span>
        </div>
        <SheetTitle className="text-[18px] font-semibold tracking-[-0.2px]">{role.name}</SheetTitle>
        <SheetDescription className={role.description ? 'text-[13px]' : 'sr-only'}>{role.description || `Details of the ${role.name} role`}</SheetDescription>
        <div className="mt-1 flex flex-wrap gap-2">
          {src !== 'service' && (
            <Button size="sm" asChild>
              <Link to={`/roles/${role.id}/edit`}>
                <Pencil /> Edit role
              </Link>
            </Button>
          )}
          <Button size="sm" variant="outline" asChild>
            <Link to={`/roles/new?from=${role.id}`}>
              <Copy /> Duplicate
            </Link>
          </Button>
          {src === 'custom' && (
            <Button size="sm" variant="outline" onClick={onDelete} className="border-destructive/30 text-destructive hover:bg-destructive/5 hover:text-destructive">
              <Trash2 /> Delete
            </Button>
          )}
        </div>
        {note && (
          <p className="flex items-start gap-2 rounded-[10px] bg-muted/70 px-3 py-2 text-[12px] text-muted-foreground">
            <Info className="mt-0.5 size-3.5 shrink-0" />
            {note}
          </p>
        )}
      </div>

      <section className="flex flex-col gap-3 border-b px-5 py-4">
        <h3 className="text-[12px] font-semibold text-muted-foreground">Someone with this role can</h3>
        {groups.length === 0 && <p className="text-[12.5px] text-muted-foreground">Nothing yet: this role has no permissions.</p>}
        {groups.map((g) => (
          <div key={g.service.id} className="flex flex-col gap-2">
            <div className="flex items-center gap-2">
              <ServiceMark service={g.service} />
              <span className="text-[13px] font-semibold">In {g.service.name}</span>
              <span className="text-[12px] text-muted-foreground">
                {g.perms.length} of {g.total}
              </span>
            </div>
            <ul className="flex flex-col gap-1.5 pl-7">
              {g.perms.map((p) => (
                <li key={p.key} className="flex items-start gap-2">
                  <Check className="mt-0.5 size-3.5 shrink-0 text-primary" strokeWidth={2.6} />
                  <span className="flex flex-col">
                    <span className="text-[13px] font-medium">{permLabel(p)}</span>
                    {p.description && <span className="text-[12px] text-muted-foreground">{p.description}</span>}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </section>

      <section className="flex flex-col gap-2.5 border-b px-5 py-4">
        <h3 className="text-[12px] font-semibold text-muted-foreground">Where it can be granted</h3>
        <TypeTree types={catalog.types} appliesTo={role.appliesTo} />
      </section>

      <section className="flex flex-col gap-2.5 px-5 py-4">
        <div className="flex flex-col">
          <h3 className="text-[13px] font-semibold">{plural(role.grantCount, 'grant')}</h3>
          <span className="text-[12px] text-muted-foreground">{data ? usageLine(data.grants, data.truncated) : role.grantCount === 0 ? 'Not granted anywhere yet' : ' '}</span>
        </div>
        {role.grantCount > 0 &&
          (error ? (
            <ErrorState message={error} onRetry={reload} />
          ) : loading && !data ? (
            <LoadingState />
          ) : (
            <>
              <RoleGrantsList grants={data?.grants ?? []} limit={SHOWN_GRANTS} />
              {(data?.grants.length ?? 0) > SHOWN_GRANTS && (
                <p className="text-[12px] text-muted-foreground">and {(data?.grants.length ?? 0) - SHOWN_GRANTS} more. Each is managed from its node in the Resource Tree.</p>
              )}
            </>
          ))}
      </section>
    </div>
  )
}
