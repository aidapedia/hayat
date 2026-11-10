import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'
import { Crown, FolderTree, MoreHorizontal, Pencil, Trash2, UserMinus, UserPlus } from 'lucide-react'
import { TenantService, type Member } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { useMe } from '@/features/auth'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDate, formatDateTime } from '@/shared/lib/datetime'
import { CopyField, DetailList, ErrorState, Field, LoadingState, PersonRow, Pill, StatusBadge } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/shared/ui/dropdown-menu'
import { Input } from '@/shared/ui/input'
import { PageHeader, Section } from '@/shared/ui/page/PageHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'

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

function RenameDialog({ id, current, open, onOpenChange, onDone }: { id: string; current: string; open: boolean; onOpenChange: (o: boolean) => void; onDone: () => void }) {
  const tenants = useClient(TenantService)
  const [name, setName] = useState(current)
  const [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await tenants.renameTenant({ id, name })
      toast.success('Tenant renamed')
      onOpenChange(false)
      onDone()
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
            <DialogTitle className="text-[15px]">Rename tenant</DialogTitle>
            <DialogDescription className="text-[13px]">The slug stays the same.</DialogDescription>
          </DialogHeader>
          <Field label="Name" htmlFor="r-name">
            <Input id="r-name" value={name} onChange={(e) => setName(e.target.value)} required autoFocus />
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !name || name === current}>
              {busy ? 'Saving…' : 'Rename'}
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
      trailing={m.isOwner ? <Pill className="border-primary/30 text-primary">Owner</Pill> : undefined}
    />
  )
  return link ? <Link to={`/users/${m.user?.id}`}>{person}</Link> : person
}

type Pending = { title: string; body: string; confirm: string; destructive?: boolean; typeToConfirm?: string; run: () => Promise<void> }

export default function TenantDetailPage() {
  const { id = '' } = useParams()
  const { isAdmin, me } = useMe()
  const tenants = useClient(TenantService)
  const navigate = useNavigate()
  const { data, error, loading, reload } = useQuery(() => tenants.getTenant({ id }), [tenants, id])
  const [adding, setAdding] = useState(false)
  const [renaming, setRenaming] = useState(false)
  const [pending, setPending] = useState<Pending | null>(null)

  if (error) return <ErrorState message={error} onRetry={reload} />
  if (loading && !data) return <LoadingState />
  if (!data?.tenant) return null
  const t = data.tenant

  const memberName = (m: Member) => m.user?.name || m.user?.email || 'this member'
  // Show only what the server would allow; it checks every action again.
  const canRename = data.myPermissions.includes('hayat.tenant.manage')
  const canManageMembers = data.myPermissions.includes('hayat.members.manage')
  const canTransfer = isAdmin || t.ownerId === me.user?.id

  return (
    <>
      <PageHeader
        title={t.name}
        crumbs={[{ label: t.name }]}
        meta={<Pill className="font-mono normal-case">{t.slug}</Pill>}
        desc={`${t.memberCount} ${t.memberCount === 1 ? 'member' : 'members'}`}
        action={
          <>
            <Button variant="outline" size="lg" asChild>
              <Link to={`/resource-tree/${id}`}>
                <FolderTree /> Resource tree
              </Link>
            </Button>
            {canRename && (
              <Button variant="outline" size="lg" onClick={() => setRenaming(true)}>
                <Pencil /> Rename
              </Button>
            )}
            {isAdmin && (
              <Button
                variant="destructive"
                size="lg"
                onClick={() =>
                  setPending({
                    title: `Delete ${t.name}?`,
                    body: 'The tenant and its membership list are deleted. Accounts are kept. This is refused while any service still files resources under the tenant.',
                    confirm: 'Delete tenant',
                    destructive: true,
                    typeToConfirm: t.slug,
                    run: async () => {
                      await tenants.deleteTenant({ id })
                      toast.success(`${t.name} deleted`)
                      navigate('/tenants')
                    },
                  })
                }
              >
                <Trash2 /> Delete
              </Button>
            )}
          </>
        }
      />

      <div className="mt-7 grid grid-cols-[minmax(0,1fr)_minmax(280px,360px)] gap-x-10 max-[1000px]:grid-cols-1">
        <div className="min-w-0">
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
          <div className="overflow-x-auto rounded-xl border">
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
                {data.members.map((m) => (
                  <TableRow key={m.user?.id}>
                    <TableCell>
                      <MemberPerson member={m} link={isAdmin} />
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
                                      await tenants.transferOwnership({ tenantId: id, userId: m.user!.id })
                                      toast.success('Ownership transferred')
                                      await reload()
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
                                      await tenants.removeMember({ tenantId: id, userId: m.user!.id })
                                      toast.success('Member removed')
                                      await reload()
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
          </div>
        </Section>
        </div>
        <div className="min-w-0 max-[1000px]:mt-9">
        <Section title="Details">
          <div className="rounded-xl border p-4">
            <DetailList
              stacked
              items={[
                ['Tenant id', <CopyField key="id" value={t.id} />],
                ['Slug', <span key="s" className="font-mono text-[12px]">{t.slug}</span>],
                ['Home region', t.region],
                ['Created', formatDateTime(t.createdAt)],
                ['Updated', formatDateTime(t.updatedAt)],
              ]}
            />
          </div>
        </Section>
        </div>
      </div>

      <AddMemberDialog tenantId={id} open={adding} onOpenChange={setAdding} onAdded={reload} />
      {renaming && <RenameDialog id={id} current={t.name} open={renaming} onOpenChange={setRenaming} onDone={reload} />}
      <ConfirmDialog
        open={!!pending}
        onOpenChange={(o) => !o && setPending(null)}
        title={pending?.title ?? ''}
        body={pending?.body ?? ''}
        confirm={pending?.confirm ?? 'Confirm'}
        destructive={pending?.destructive}
        typeToConfirm={pending?.typeToConfirm}
        onConfirm={async () => {
          if (pending) await pending.run()
        }}
      />
    </>
  )
}
