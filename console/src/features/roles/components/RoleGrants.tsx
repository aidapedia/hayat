import { Link } from 'react-router'
import { Users } from 'lucide-react'
import { CatalogService, type RoleGrant } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDate } from '@/shared/lib/datetime'
import { PersonAvatar } from '@/shared/ui/bits'
import { plural, typeLabel } from '../lib/roles'

/** Where a role is granted, read fresh each time the role changes. */
export function useRoleGrants(roleId: string | undefined, grantCount: number) {
  const catalog = useClient(CatalogService)
  return useQuery(async () => {
    // Nothing to ask for a role nobody holds.
    if (!roleId || grantCount === 0) return { grants: [] as RoleGrant[], truncated: false }
    return catalog.listRoleGrants({ roleId })
  }, [catalog, roleId, grantCount])
}

/** "In 2 tenants · 3 to groups, 1 to a person" */
export function usageLine(grants: RoleGrant[], truncated: boolean): string {
  if (grants.length === 0) return 'Not granted anywhere yet'
  const tenants = new Set(grants.map((g) => g.tenantId).filter(Boolean)).size
  const groups = grants.filter((g) => g.grant?.principalType === 'group').length
  const people = grants.length - groups
  const parts = [
    tenants > 0 ? `In ${plural(tenants, 'tenant')}` : 'Outside any tenant',
    [groups && `${groups} to ${groups === 1 ? 'a group' : 'groups'}`, people && `${people} to ${people === 1 ? 'a person' : 'people'}`].filter(Boolean).join(', '),
  ]
  return parts.join(' · ') + (truncated ? ' (showing the newest)' : '')
}

/** One row per grant: who, on which node (with its path), and a way to open it in the tree. */
export function RoleGrantsList({ grants, limit }: { grants: RoleGrant[]; limit?: number }) {
  const shown = limit ? grants.slice(0, limit) : grants
  return (
    <ul className="divide-y rounded-xl border">
      {shown.map((rg) => {
        const g = rg.grant
        if (!g) return null
        const isGroup = g.principalType === 'group'
        const where = [...rg.path, rg.nodeName].join(' › ')
        const notes = [!g.inherit && 'this node only', g.expiresAt && `expires ${formatDate(g.expiresAt)}`].filter(Boolean).join(' · ')
        return (
          <li key={g.id} className="flex items-center gap-3 px-3.5 py-2.5">
            {isGroup ? (
              <span className="flex size-[30px] shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                <Users className="size-[15px]" />
              </span>
            ) : (
              <PersonAvatar name={g.principalName} email={g.principalEmail} size={30} />
            )}
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="truncate text-[13px] font-semibold">
                {g.principalName || g.principalEmail || 'Unknown'}
                <span className="font-normal text-muted-foreground"> · {isGroup ? 'group' : 'person'}</span>
              </span>
              <span className="truncate text-[12px] text-muted-foreground">
                On {typeLabel(rg.nodeType).toLowerCase()}: {where}
                {notes && ` · ${notes}`}
              </span>
            </span>
            {rg.tenantId && (
              <Link to={`/resource-tree/${rg.tenantId}/${g.nodeId}`} className="shrink-0 text-[12px] font-medium text-primary hover:underline">
                Open in tree
              </Link>
            )}
          </li>
        )
      })}
    </ul>
  )
}
