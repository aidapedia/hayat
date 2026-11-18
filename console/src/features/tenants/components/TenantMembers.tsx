import { useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import { toast } from 'sonner'
import { Crown, MoreHorizontal, UserMinus, UserPlus } from 'lucide-react'
import { TenantService, type Member, type Tenant } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { formatDate } from '@/shared/lib/datetime'
import { ErrorState, Field, LoadingState, PersonRow, Pill, StatusBadge } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/shared/ui/dropdown-menu'
import { Input } from '@/shared/ui/input'
import { Section } from '@/shared/ui/page/PageHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { Pagination } from '@/shared/ui/table/Pagination'
import { useServerPagination } from '@/shared/ui/table/usePagination'

function AddMemberDialog({ tenantId, open, onOpenChange, onAdded }: { tenantId: string; open: boolean; onOpenChange: (o: boolean) => void; onAdded: () => void }) {
  const tenants = useClient(TenantService)
  const [email, setEmail] = useState('')
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const res = await tenants.addMember({ tenantId, email, name })
      toast.success(res.invited ? `Invitation sent to ${email}` : `${res.member?.user?.name || email} added`)
      setEmail('')
      setName('')
      onOpenChange(false)
      onAdded()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={submit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">Add a member</DialogTitle>
            <DialogDescription className="text-[13px]">
              Someone with an account is added straight away. A new address gets an invitation and joins as a pending account.
            </DialogDescription>
          </DialogHeader>
          <Field label="Email" htmlFor="m-email">
            <Input id="m-email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required autoFocus />
          </Field>
          <Field label="Name" htmlFor="m-name" hint="Used only when the address has no account yet.">
            <Input id="m-name" value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !email}>
              {busy ? 'Adding…' : 'Add member'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

/** A member's row; it links to their user page only for an instance admin, the only one who can open it. */
function MemberPerson({ member: m, link }: { member: Member; link: boolean }) {
  const person = (
    <PersonRow
      name={m.user?.name}
      email={m.user?.email ?? ''}
      src={m.user?.avatarUrl}
      trailing={m.isOwner ? <Pill className="border-primary/30 text-primary">Owner</Pill> : undefined}
    />
  )
  return link ? <Link to={`/users/${m.user?.id}`}>{person}</Link> : person
}

type Pending = { title: string; body: string; confirm: string; destructive?: boolean; run: () => Promise<void> }

/**
 * The tenant's roster. Who may add, remove or hand over ownership is decided
 * by the page (from the caller's permissions); the server checks each again.
 * A roster change moves the member count (and maybe the owner), so
 * `onChanged` re-reads the tenant too.
 */
export function TenantMembers({
  tenant: t,
  canManageMembers,
  canTransfer,
  linkPeople,
  onChanged,
}: {
  tenant: Tenant
  canManageMembers: boolean
  canTransfer: boolean
  linkPeople: boolean
  onChanged: () => Promise<void> | void
}) {
  const tenants = useClient(TenantService)
  const members = useServerPagination(
    (token, pageSize) => tenants.listMembers({ tenantId: t.id, pageSize, pageToken: token }).then((r) => ({ items: r.members, next: r.nextPageToken })),
    [tenants, t.id],
  )
  const [adding, setAdding] = useState(false)
  const [pending, setPending] = useState<Pending | null>(null)
  const reloadAll = async () => {
    await Promise.all([onChanged(), members.reload()])
  }
  const memberName = (m: Member) => m.user?.name || m.user?.email || 'this member'

  return (
    <Section
      title="Members"
      desc="Who belongs to this tenant. What they may do is set in the Resource Tree."
      action={
        canManageMembers && (
          <Button onClick={() => setAdding(true)}>
            <UserPlus /> Add member
          </Button>
        )
      }
    >
      {members.error ? (
        <ErrorState message={members.error} onRetry={members.reload} />
      ) : members.loading && members.items.length === 0 ? (
        <LoadingState />
      ) : (
        <div className="overflow-hidden rounded-xl border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="text-[11px]">Person</TableHead>
                <TableHead className="text-[11px]">Status</TableHead>
                <TableHead className="text-[11px]">Joined</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {members.items.map((m) => (
                <TableRow key={m.user?.id}>
                  <TableCell>
                    <MemberPerson member={m} link={linkPeople} />
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={m.user?.status ?? ''} />
                  </TableCell>
                  <TableCell className="text-[12px] text-muted-foreground">{formatDate(m.joinedAt)}</TableCell>
                  <TableCell className="text-right">
                    {!m.isOwner && (canTransfer || canManageMembers) && (
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon-sm" aria-label={`Actions for ${memberName(m)}`}>
                            <MoreHorizontal />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-52">
                          {canTransfer && (
                            <DropdownMenuItem
                              className="text-[13px]"
                              disabled={m.user?.status !== 'active'}
                              onSelect={() =>
                                setPending({
                                  title: `Make ${memberName(m)} the owner?`,
                                  body: 'A tenant has exactly one owner. The current owner stays a member.',
                                  confirm: 'Transfer ownership',
                                  run: async () => {
                                    await tenants.transferOwnership({ tenantId: t.id, userId: m.user!.id })
                                    toast.success('Ownership transferred')
                                    await reloadAll()
                                  },
                                })
                              }
                            >
                              <Crown /> Make owner
                            </DropdownMenuItem>
                          )}
                          {canManageMembers && (
                            <DropdownMenuItem
                              variant="destructive"
                              className="text-[13px]"
                              onSelect={() =>
                                setPending({
                                  title: `Remove ${memberName(m)}?`,
                                  body: `They stop belonging to ${t.name}. Their account is kept.`,
                                  confirm: 'Remove member',
                                  destructive: true,
                                  run: async () => {
                                    await tenants.removeMember({ tenantId: t.id, userId: m.user!.id })
                                    toast.success('Member removed')
                                    await reloadAll()
                                  },
                                })
                              }
                            >
                              <UserMinus /> Remove from tenant
                            </DropdownMenuItem>
                          )}
                        </DropdownMenuContent>
                      </DropdownMenu>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          <Pagination {...members.pager} total={t.memberCount} className="border-t" />
        </div>
      )}

      <AddMemberDialog tenantId={t.id} open={adding} onOpenChange={setAdding} onAdded={reloadAll} />
      <ConfirmDialog
        open={!!pending}
        onOpenChange={(o) => !o && setPending(null)}
        title={pending?.title ?? ''}
        body={pending?.body ?? ''}
        confirm={pending?.confirm ?? 'Confirm'}
        destructive={pending?.destructive}
        onConfirm={async () => {
          if (pending) await pending.run()
        }}
      />
    </Section>
  )
}
