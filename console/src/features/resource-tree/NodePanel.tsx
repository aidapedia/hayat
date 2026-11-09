import { useMemo, useState, type ReactNode } from 'react'
import { Link, useNavigate } from 'react-router'
import { toast } from 'sonner'
import { FolderPlus, MoveRight, Pencil, Search, ShieldPlus, Trash2, UserMinus, UserPlus, X } from 'lucide-react'
import { TreeAdminService, type Node } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import type { Member } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDate, formatDateTime } from '@/shared/lib/datetime'
import { CopyField, DetailList, ErrorState, LoadingState, PersonRow } from '@/shared/ui/bits'
import { Breadcrumb, BreadcrumbItem, BreadcrumbLink, BreadcrumbList, BreadcrumbPage, BreadcrumbSeparator } from '@/shared/ui/breadcrumb'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/shared/ui/select'
import { AddGroupMemberDialog, CreateGroupDialog, GrantDialog, MoveDialog, RenameGroupDialog } from './dialogs'
import { NodeIcon } from './NodeIcon'
import { childrenMap, GROUP, isContainer, nodeHref, TENANT, typeLabel } from './tree'

const P = {
  organise: 'hayat.resources.organise',
  grant: 'hayat.resources.grant',
  groups: 'hayat.groups.manage',
}

/** A titled block inside the panel: a small uppercase heading, an optional action, then the content. */
function PanelSection({ title, count, action, children }: { title: string; count?: number; action?: ReactNode; children: ReactNode }) {
  return (
    <section>
      <div className="mb-2 flex min-h-6 items-center gap-1.5">
        <h3 className="text-[10px] font-bold tracking-wide text-faint uppercase">
          {title}
          {!!count && <span className="ml-1 font-normal tracking-normal normal-case">({count})</span>}
        </h3>
        {action && <div className="ml-auto">{action}</div>}
      </div>
      {children}
    </section>
  )
}

function Empty({ children }: { children: ReactNode }) {
  return <p className="rounded-[10px] border border-dashed px-3 py-4 text-center text-[12px] text-faint">{children}</p>
}

function PermissionSelect({ value, onChange, options }: { value: string; onChange: (v: string) => void; options: string[] }) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className="w-full font-mono text-[12px]">
        <SelectValue placeholder="Choose a permission" />
      </SelectTrigger>
      <SelectContent>
        {options.map((p) => (
          <SelectItem key={p} value={p} className="font-mono text-[12px]">
            {p}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

/** Who really holds a permission here: through the chain, rosters, placement and ownership. */
function WhoHasAccess({ node, permissions }: { node: Node; permissions: string[] }) {
  const tree = useClient(TreeAdminService)
  const [perm, setPerm] = useState('')
  const { data, error, loading } = useQuery(
    () => (perm ? tree.whoHasAccess({ nodeId: node.id, permission: perm }).then((r) => r.holders) : Promise.resolve(undefined)),
    [tree, node.id, perm],
  )
  return (
    <div className="flex flex-col gap-3 rounded-[10px] border p-3">
      <PermissionSelect value={perm} onChange={setPerm} options={permissions} />
      {!perm ? (
        <p className="text-[12px] text-faint">Pick a permission to list everyone who holds it here, and why.</p>
      ) : error ? (
        <p className="text-[12px] text-destructive">{error}</p>
      ) : loading && !data ? (
        <LoadingState />
      ) : !data || data.length === 0 ? (
        <p className="text-[12px] text-faint">Nobody holds it here.</p>
      ) : (
        <ul className="flex flex-col gap-2.5">
          {data.map((h) => (
            <li key={h.user!.id} className="flex flex-col gap-1">
              <PersonRow name={h.user!.name} email={h.user!.email} />
              <span className="pl-10 text-[12px] text-muted-foreground">{h.reason?.text}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

/** Why one person may, or may not, do something here. */
function ExplainAccess({ node, permissions, members }: { node: Node; permissions: string[]; members: Member[] }) {
  const tree = useClient(TreeAdminService)
  const [user, setUser] = useState('')
  const [perm, setPerm] = useState('')
  const { data, error } = useQuery(
    () => (user && perm ? tree.explainAccess({ nodeId: node.id, userId: user, permission: perm }) : Promise.resolve(undefined)),
    [tree, node.id, user, perm],
  )
  return (
    <div className="flex flex-col gap-3 rounded-[10px] border p-3">
      <Select value={user} onValueChange={setUser}>
        <SelectTrigger className="w-full">
          <SelectValue placeholder="Choose a person" />
        </SelectTrigger>
        <SelectContent>
          {members.map((m) => (
            <SelectItem key={m.user!.id} value={m.user!.id}>
              {m.user!.name || m.user!.email}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <PermissionSelect value={perm} onChange={setPerm} options={permissions} />
      {error && <p className="text-[12px] text-destructive">{error}</p>}
      {data && (
        <div
          className={`rounded-[10px] border px-3 py-2 text-[13px] ${data.allowed ? 'border-success/30 bg-success/10 text-success' : 'border-destructive/30 bg-destructive/10 text-destructive'}`}
        >
          <div className="font-semibold">{data.allowed ? 'Allowed' : 'Not allowed'}</div>
          <div className="mt-0.5 text-[12px] text-foreground/80">
            {data.allowed ? data.reason?.text : 'No grant, ownership or placement on the way down gives it.'}
          </div>
        </div>
      )}
    </div>
  )
}

/**
 * The selected node, in the right-hand card: what it is, where it sits, what
 * is inside, who was granted what here, its roster, and who really has access.
 */
export function NodePanel({
  tenantId,
  tenantName,
  nodeId,
  nodes,
  members,
  onTreeChange,
  onClose,
}: {
  tenantId: string
  tenantName: string
  nodeId: string
  nodes: Node[]
  members: Member[]
  onTreeChange: () => Promise<void>
  onClose: () => void
}) {
  const tree = useClient(TreeAdminService)
  const navigate = useNavigate()
  const { data, error, loading, reload } = useQuery(() => tree.getNode({ id: nodeId }), [tree, nodeId])
  const [dialog, setDialog] = useState<'group' | 'rename' | 'move' | 'grant' | 'member' | null>(null)
  const [confirm, setConfirm] = useState<{ title: string; body: string; label: string; run: () => Promise<void> } | null>(null)
  const children = useMemo(() => childrenMap(nodes).get(nodeId) ?? [], [nodes, nodeId])

  if (error) return <div className="p-4"><ErrorState message={error} onRetry={reload} /></div>
  if (loading && !data) return <div className="px-4"><LoadingState /></div>
  if (!data?.node) return null
  const n = data.node
  const can = (p: string) => data.myPermissions.includes(p)
  const isGroup = n.type === GROUP
  const owner = n.ownerId ? members.find((m) => m.user?.id === n.ownerId)?.user : undefined
  const ancestors = data.ancestors.filter((a) => a.type !== 'hayat/root' && a.id !== n.id)
  const refresh = async () => {
    setDialog(null)
    await Promise.all([reload(), onTreeChange()])
  }
  const run = async (fn: () => Promise<unknown>, done: string) => {
    try {
      await fn()
      toast.success(done)
      await refresh()
    } catch (err) {
      toast.error(errorMessage(err))
    }
  }

  return (
    <div className="flex flex-col">
      {/* Header */}
      <div className="flex items-start gap-3 border-b p-4">
        <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-[9px] bg-accent">
          <NodeIcon type={n.type} className="size-[18px] text-muted-foreground" />
        </span>
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-[14px] font-semibold">{n.name}</h2>
          <p className="mt-0.5 truncate text-[11px] text-faint">
            {typeLabel(n.type)}
            {owner && ` · Owned by ${owner.name || owner.email}`}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-0.5">
          {isGroup && can(P.organise) && (
            <Button variant="ghost" size="icon-sm" title="Rename group" aria-label="Rename group" onClick={() => setDialog('rename')}>
              <Pencil />
            </Button>
          )}
          {n.type !== TENANT && can(P.organise) && (
            <Button variant="ghost" size="icon-sm" title="Move to another group" aria-label="Move" onClick={() => setDialog('move')}>
              <MoveRight />
            </Button>
          )}
          {isGroup && can(P.organise) && (
            <Button
              variant="ghost"
              size="icon-sm"
              className="hover:text-destructive"
              disabled={children.length > 0}
              title={
                children.length > 0
                  ? `Move or delete the ${children.length} item${children.length === 1 ? '' : 's'} inside this group first.`
                  : 'Delete group'
              }
              aria-label="Delete group"
              onClick={() =>
                setConfirm({
                  title: `Delete ${n.name}?`,
                  body: 'The group, its roster and every grant made on it or to it are removed.',
                  label: 'Delete group',
                  run: async () => {
                    try {
                      await tree.deleteGroup({ id: n.id })
                      toast.success('Group deleted')
                      await onTreeChange()
                      navigate(nodeHref(tenantId, n.parentId))
                    } catch (err) {
                      toast.error(errorMessage(err))
                    }
                  },
                })
              }
            >
              <Trash2 />
            </Button>
          )}
          <Button variant="ghost" size="icon-sm" title="Close" aria-label="Close detail" onClick={onClose}>
            <X />
          </Button>
        </div>
      </div>

      {/* Body */}
      <div className="flex flex-col gap-6 p-4">
        {/* Where it sits: the chain a grant cascades down. */}
        {ancestors.length > 0 && (
          <Breadcrumb>
            <BreadcrumbList className="text-[12px]">
              {ancestors.map((a) => (
                <span key={a.id} className="contents">
                  <BreadcrumbItem>
                    <BreadcrumbLink asChild>
                      <Link to={nodeHref(tenantId, a.type === TENANT ? undefined : a.id)}>{a.name}</Link>
                    </BreadcrumbLink>
                  </BreadcrumbItem>
                  <BreadcrumbSeparator />
                </span>
              ))}
              <BreadcrumbItem>
                <BreadcrumbPage className="text-[12px]">{n.name}</BreadcrumbPage>
              </BreadcrumbItem>
            </BreadcrumbList>
          </Breadcrumb>
        )}

        {!isContainer(n.type) && (
          <div className="rounded-[10px] border p-3">
            <DetailList
              items={[
                ['Resource id', <CopyField key="e" value={n.externalId} />],
                ['Node id', <CopyField key="n" value={n.id} />],
                ['Filed', formatDateTime(n.createdAt)],
              ]}
            />
            <p className="mt-3 text-[12px] text-faint">
              Created, renamed and deleted by the {n.type.split('/')[0]} service. Hayat only files and guards it.
            </p>
          </div>
        )}

        {isContainer(n.type) && (
          <PanelSection
            title="Contains"
            count={children.length}
            action={
              can(P.organise) && (
                <Button variant="ghost" size="icon-xs" title="New group" aria-label="New group" onClick={() => setDialog('group')}>
                  <FolderPlus />
                </Button>
              )
            }
          >
            {children.length === 0 ? (
              <Empty>Nothing filed here yet.</Empty>
            ) : (
              <div className="flex flex-col gap-0.5">
                {children.map((c) => (
                  <Link
                    key={c.id}
                    to={nodeHref(tenantId, c.id)}
                    className="flex items-center gap-2 rounded-[7px] px-2 py-1.5 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
                  >
                    <NodeIcon type={c.type} className="size-[13px] text-faint" />
                    <span className="min-w-0 flex-1 truncate">{c.name}</span>
                    <span className="shrink-0 text-[10px] text-faint">{typeLabel(c.type)}</span>
                  </Link>
                ))}
              </div>
            )}
          </PanelSection>
        )}

        <PanelSection
          title="Access granted here"
          count={data.grants.length}
          action={
            can(P.grant) && (
              <Button variant="outline" size="xs" onClick={() => setDialog('grant')}>
                <ShieldPlus /> Grant
              </Button>
            )
          }
        >
          {data.grants.length === 0 ? (
            <Empty>Nothing granted on this node itself. What was granted above still reaches it.</Empty>
          ) : (
            <ul className="divide-y rounded-[10px] border">
              {data.grants.map((g) => (
                <li key={g.id} className="flex items-center gap-3 px-3 py-2.5">
                  <div className="min-w-0 flex-1">
                    {g.principalType === 'user' ? (
                      <PersonRow name={g.principalName} email={g.principalEmail} />
                    ) : (
                      <span className="flex items-center gap-2 text-[13px] font-semibold">
                        <span className="flex size-[30px] items-center justify-center rounded-full bg-accent">
                          <NodeIcon type={GROUP} className="text-muted-foreground" />
                        </span>
                        <span className="truncate">{g.principalName}</span>
                      </span>
                    )}
                  </div>
                  <div className="shrink-0 text-right">
                    <div className="text-[12px] font-medium">{g.roleName}</div>
                    <div className="text-[11px] text-faint">
                      {g.inherit ? 'Here and below' : 'This node only'}
                      {g.expiresAt && ` · until ${formatDate(g.expiresAt)}`}
                    </div>
                  </div>
                  {can(P.grant) && (
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      aria-label="Remove grant"
                      onClick={() =>
                        setConfirm({
                          title: 'Remove this grant?',
                          body: `${g.principalName || g.principalEmail} loses ${g.roleName} here${g.inherit ? ' and below' : ''}, unless something else gives it.`,
                          label: 'Remove grant',
                          run: () => run(() => tree.removeGrant({ id: g.id }), 'Grant removed'),
                        })
                      }
                    >
                      <Trash2 />
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          )}
        </PanelSection>

        {isGroup && (
          <PanelSection
            title="People in this group"
            count={data.members.length}
            action={
              can(P.groups) && (
                <Button variant="outline" size="xs" onClick={() => setDialog('member')}>
                  <UserPlus /> Add
                </Button>
              )
            }
          >
            {data.members.length === 0 ? (
              <Empty>Nobody yet. Grants to the group reach whoever is listed here.</Empty>
            ) : (
              <ul className="divide-y rounded-[10px] border">
                {data.members.map((m) => (
                  <li key={m.user!.id} className="flex items-center justify-between gap-3 px-3 py-2.5">
                    <PersonRow name={m.user!.name} email={m.user!.email} />
                    {can(P.groups) && (
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        aria-label={`Remove ${m.user!.email}`}
                        onClick={() => run(() => tree.removeGroupMember({ groupId: n.id, userId: m.user!.id }), 'Removed from the group')}
                      >
                        <UserMinus />
                      </Button>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </PanelSection>
        )}

        {can(P.grant) && data.permissions.length > 0 && (
          <div className="grid grid-cols-2 gap-x-4 gap-y-6 max-[1200px]:grid-cols-1">
            <PanelSection title="Who has access">
              <WhoHasAccess node={n} permissions={data.permissions} />
            </PanelSection>
            <PanelSection title="Check a person">
              <ExplainAccess node={n} permissions={data.permissions} members={members} />
            </PanelSection>
          </div>
        )}

        {!can(P.grant) && (
          <p className="flex items-center gap-2 text-[12px] text-faint">
            <Search className="size-3.5" /> You see this node because you belong to {tenantName}; you can't change its access.
          </p>
        )}
      </div>

      {dialog === 'group' && (
        <CreateGroupDialog
          parent={n}
          onClose={() => setDialog(null)}
          onDone={async (g) => {
            setDialog(null)
            await onTreeChange()
            navigate(nodeHref(tenantId, g.id))
          }}
        />
      )}
      {dialog === 'rename' && <RenameGroupDialog node={n} onClose={() => setDialog(null)} onDone={refresh} />}
      {dialog === 'move' && <MoveDialog node={n} nodes={nodes} onClose={() => setDialog(null)} onDone={refresh} />}
      {dialog === 'grant' && (
        <GrantDialog node={n} nodes={nodes} members={members} tenantName={tenantName} onClose={() => setDialog(null)} onDone={refresh} />
      )}
      {dialog === 'member' && (
        <AddGroupMemberDialog
          group={n}
          members={members}
          already={new Set(data.members.map((m) => m.user!.id))}
          onClose={() => setDialog(null)}
          onDone={refresh}
        />
      )}
      <ConfirmDialog
        open={!!confirm}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={confirm?.title ?? ''}
        body={confirm?.body ?? ''}
        confirm={confirm?.label ?? 'Confirm'}
        destructive
        onConfirm={async () => {
          if (confirm) await confirm.run()
        }}
      />
    </div>
  )
}
