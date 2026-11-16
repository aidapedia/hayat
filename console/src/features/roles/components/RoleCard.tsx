import { Link, type To } from 'react-router'
import type { Role } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { grantableLine, groupByService, type Catalog, type ServiceMeta } from '../lib/roles'
import { RoleIcon, RoleSourceBadge, ServiceMark } from './marks'

/**
 * One role in the list: who owns it, what it's for, which services' permissions
 * it bundles and where it can be granted. Opening it goes to its own page.
 */
export function RoleCard({ role, catalog, services, to }: { role: Role; catalog: Catalog; services: ServiceMeta[]; to: To }) {
  const groups = groupByService(role.permissions, catalog, services)
  return (
    <Link
      to={to}
      className="flex items-start gap-3.5 rounded-xl border bg-background px-4 py-3.5 transition-colors outline-none hover:border-primary/40 focus-visible:ring-3 focus-visible:ring-ring/40"
    >
      <RoleIcon role={role} services={services} />
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-[14px] font-semibold">{role.name}</span>
          <RoleSourceBadge role={role} services={services} />
        </div>
        {role.description && <p className="text-[12.5px] text-muted-foreground">{role.description}</p>}
        <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
          {groups.map((g) => (
            <span key={g.service.id} className="inline-flex h-6 items-center gap-1.5 rounded-[7px] border bg-background pr-2 pl-[3px] text-[12px]">
              <ServiceMark service={g.service} size="xs" />
              {g.service.name}
              <span className="text-muted-foreground">· {g.perms.length}</span>
            </span>
          ))}
          {groups.length === 0 && <span className="text-[12px] text-muted-foreground">No permissions</span>}
          <span className="ml-1 text-[12px] text-muted-foreground">Grantable on {grantableLine(role.appliesTo).toLowerCase()}</span>
        </div>
      </div>
      <div className="flex shrink-0 flex-col items-end text-right">
        <span className="text-[15px] font-semibold tabular-nums">{role.grantCount}</span>
        <span className="text-[11px] text-muted-foreground">{role.grantCount === 1 ? 'grant' : 'grants'}</span>
      </div>
    </Link>
  )
}
