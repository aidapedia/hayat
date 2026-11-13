import { Link } from 'react-router'
import { Copy } from 'lucide-react'
import type { Role } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { Button } from '@/shared/ui/button'
import { grantableLine, groupByService, permLabel, plural, type Catalog, type ServiceMeta } from '../../lib/roles'
import { ServiceMark } from '../marks'

/**
 * A service's contract role can't be edited here (the server refuses it). Say
 * why, show what it allows, and offer the way forward: duplicate it.
 */
export function ServiceRoleNotice({ role, catalog, services }: { role: Role; catalog: Catalog; services: ServiceMeta[] }) {
  const owner = services.find((s) => s.id === role.service)
  const name = owner?.name ?? role.service
  return (
    <div className="mt-6 max-w-[620px] overflow-hidden rounded-2xl border bg-background">
      <div className="flex flex-col gap-2.5 px-6 pt-[22px] pb-4">
        <div className="flex items-center gap-3">
          {owner && <ServiceMark service={owner} size="lg" />}
          <div className="flex min-w-0 flex-col">
            <h2 className="text-[16px] font-semibold">
              {role.name} belongs to {name}
            </h2>
            <span className="font-mono text-[12px] text-muted-foreground">
              {role.slug}
              {owner?.version ? ` · contract v${owner.version}` : ''}
            </span>
          </div>
        </div>
        <p className="text-[13px] text-muted-foreground">
          {name}'s contract defines this role, so it can't be edited here. It changes when {name} publishes a new contract version and a service owner approves it.
        </p>
        <p className="text-[13px] text-muted-foreground">Need something slightly different? Duplicate it: the copy is yours to change, and grants of the original stay as they are.</p>
      </div>
      <div className="mx-6 flex flex-col gap-2 rounded-xl border px-3.5 py-3">
        {groupByService(role.permissions, catalog, services).map((g) => (
          <div key={g.service.id} className="flex flex-col gap-2">
            <span className="text-[12px] font-semibold text-muted-foreground">It allows, in {g.service.name}</span>
            <div className="flex flex-wrap gap-1.5">
              {g.perms.map((p) => (
                <span key={p.key} className="inline-flex h-6 items-center rounded-[7px] bg-muted px-2 text-[12px]">
                  {permLabel(p)}
                </span>
              ))}
            </div>
          </div>
        ))}
        <span className="text-[12px] text-muted-foreground">
          Grantable on {grantableLine(role.appliesTo)} · {plural(role.grantCount, 'grant')}
        </span>
      </div>
      <div className="flex flex-wrap justify-end gap-2 px-6 pt-[18px] pb-[22px]">
        {owner && (
          <Button variant="outline" asChild>
            <Link to={`/services/${owner.id}`}>View contract</Link>
          </Button>
        )}
        <Button asChild>
          <Link to={`/roles/new?from=${role.id}`}>
            <Copy /> Duplicate as my own role
          </Link>
        </Button>
      </div>
    </div>
  )
}
