import { useState } from 'react'
import { Link } from 'react-router'
import { ChevronRight, X } from 'lucide-react'
import { TreeAdminService, type Grant, type Node } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDate } from '@/shared/lib/datetime'
import { cn } from '@/shared/lib/utils'
import { LoadingState, PersonAvatar } from '@/shared/ui/bits'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { toBlocks, type Person } from './access'
import { AccessDialog } from './AccessDialog'
import { NodeIcon } from './NodeIcon'
import { GROUP, nodeHref } from './tree'

const STACK = 4

/** Up to four faces, overlapping, then "+N". */
function AvatarStack({ people }: { people: Person[] }) {
  return (
    <span className="flex items-center">
      <span className="flex -space-x-1.5">
        {people.slice(0, STACK).map((p) => (
          <PersonAvatar key={p.user.id} name={p.user.name} email={p.user.email} size={22} className="ring-2 ring-card" />
        ))}
      </span>
      {people.length > STACK && <span className="ml-1.5 text-[11px] text-faint">+{people.length - STACK}</span>}
    </span>
  )
}

/** A grant made on this node to a group (or the whole tenant), removable where the viewer may grant. */
function GroupGrantChip({ grant, tenantId, onRemove }: { grant: Grant; tenantId: string; onRemove?: () => void }) {
  const scope = `${grant.inherit ? 'Here and below' : 'This node only'}${grant.expiresAt ? ` · until ${formatDate(grant.expiresAt)}` : ''}`
  return (
    <span title={scope} className="inline-flex h-6 items-center gap-1.5 rounded-full border bg-background pr-1 pl-1.5 text-[11px]">
      <NodeIcon type={GROUP} className="size-3.5 text-muted-foreground" />
      <Link to={nodeHref(tenantId, grant.principalId)} className="font-medium hover:underline">
        {grant.principalName}
      </Link>
      {!grant.inherit && <span className="text-faint">only here</span>}
      {onRemove ? (
        <button
          type="button"
          aria-label={`Remove ${grant.roleName} from ${grant.principalName}`}
          className="flex size-4 items-center justify-center rounded-full text-faint hover:bg-destructive/15 hover:text-destructive"
          onClick={onRemove}
        >
          <X className="size-3" />
        </button>
      ) : (
        <span className="w-0.5" />
      )}
    </span>
  )
}

/**
 * Who reaches this node, one row per role: a count and a few faces. A row
 * opens everyone it reaches in a modal, where people granted it directly here
 * can be ungranted together. Grants to groups sit on their role's row.
 * What was given, not what it adds up to — the permission checks answer that.
 */
export function AccessList({
  node,
  tenantId,
  tenantName,
  grants,
  version,
  onRemoveGrants,
}: {
  node: Node
  tenantId: string
  tenantName: string
  grants: Grant[]
  version: unknown
  /** Removes these grants; absent when the viewer may not grant here. */
  onRemoveGrants?: (grants: Grant[]) => Promise<void>
}) {
  const tree = useClient(TreeAdminService)
  const { data, error, loading } = useQuery(() => tree.listNodeAccess({ nodeId: node.id }).then((r) => r.entries), [tree, node.id, version])
  const [openKey, setOpenKey] = useState<string | null>(null)
  const [pending, setPending] = useState<Grant[] | null>(null)

  if (error) return <p className="text-[12px] text-destructive">{error}</p>
  if (loading && !data) return <LoadingState />
  const blocks = toBlocks(data ?? [], grants)
  if (blocks.length === 0)
    return <p className="rounded-[10px] border border-dashed px-3 py-4 text-center text-[12px] text-faint">Nobody reaches this node yet.</p>

  const ask = onRemoveGrants ? (gs: Grant[]) => setPending(gs) : undefined
  const open = blocks.find((b) => b.key === openKey)
  const one = pending?.length === 1 ? pending[0] : undefined

  return (
    <>
      <ul className="divide-y rounded-[10px] border">
        {blocks.map((b) => {
          const count = b.people.length
          const groupGrants = b.grants.filter((g) => g.principalType !== 'user')
          return (
            <li key={b.key}>
              <button
                type="button"
                onClick={() => setOpenKey(b.key)}
                className="flex w-full items-center gap-3 px-3 py-2.5 text-left hover:bg-accent/50"
              >
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-[13px] font-semibold">{b.title}</span>
                  {b.note && <span className={cn('block truncate text-[11px] text-faint', b.mono && 'font-mono')}>{b.note}</span>}
                </span>
                <AvatarStack people={b.people} />
                <span className="w-16 shrink-0 text-right text-[12px] text-muted-foreground">
                  {count === 0 ? 'Nobody yet' : `${count} ${count === 1 ? 'person' : 'people'}`}
                </span>
                <ChevronRight className="size-3.5 shrink-0 text-faint" />
              </button>
              {groupGrants.length > 0 && (
                <div className="flex flex-wrap items-center gap-1.5 px-3 pb-2.5">
                  <span className="text-[11px] text-faint">Granted here to</span>
                  {groupGrants.map((g) => (
                    <GroupGrantChip key={g.id} grant={g} tenantId={tenantId} onRemove={ask && (() => ask([g]))} />
                  ))}
                </div>
              )}
            </li>
          )
        })}
      </ul>

      {open && <AccessDialog block={open} node={node} tenantName={tenantName} onRemove={ask} onClose={() => setOpenKey(null)} />}

      <ConfirmDialog
        open={!!pending}
        onOpenChange={(o) => !o && setPending(null)}
        title={one ? 'Remove this grant?' : `Ungrant ${pending?.length ?? 0} people?`}
        body={
          one
            ? `${one.principalName || one.principalEmail} loses ${one.roleName} here${one.inherit ? ' and below' : ''}, unless something else gives it.`
            : `They lose ${pending?.[0]?.roleName ?? 'this role'} on ${node.name}, unless something else gives it.`
        }
        confirm={one ? 'Remove grant' : 'Ungrant'}
        destructive
        onConfirm={async () => {
          if (pending && onRemoveGrants) await onRemoveGrants(pending)
          setPending(null)
        }}
      />
    </>
  )
}
