import { useMemo, useState, type FormEvent } from 'react'
import { toast } from 'sonner'
import { UserMinus, UserPlus } from 'lucide-react'
import type { User } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { RegistryService, type ServiceInfo } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useMe } from '@/features/auth'
import { Field, PersonRow, Pill, SearchInput, StatusBadge } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { Input } from '@/shared/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { Pagination } from '@/shared/ui/table/Pagination'
import { usePagination } from '@/shared/ui/table/usePagination'

function AddOwnerDialog({ service, onClose, onAdded }: { service: ServiceInfo; onClose: () => void; onAdded: () => unknown }) {
  const registry = useClient(RegistryService)
  const [email, setEmail] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await registry.addServiceOwner({ serviceId: service.id, email })
      toast.success('Owner added')
      await onAdded()
      onClose()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={submit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">Add an owner to {service.displayName}</DialogTitle>
            <DialogDescription className="text-[13px]">They can publish and approve its contracts, manage its clients and add other owners.</DialogDescription>
          </DialogHeader>
          <Field label="Email" htmlFor="owner-email" hint="Someone without an account gets an invitation first.">
            <Input id="owner-email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="name@example.com" required autoFocus />
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !email.trim()}>
              {busy ? 'Adding…' : 'Add owner'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

/** Who owns a service: they publish its contracts and manage its clients. There is always at least one. */
export function OwnersTab({ service: s, canManage, onChanged }: { service: ServiceInfo; canManage: boolean; onChanged: () => unknown }) {
  const registry = useClient(RegistryService)
  const { me } = useMe()
  const [query, setQuery] = useState('')
  const [adding, setAdding] = useState(false)
  const [removing, setRemoving] = useState<User | null>(null)
  const rows = useMemo(() => {
    const q = query.trim().toLowerCase()
    return q ? s.owners.filter((o) => o.name.toLowerCase().includes(q) || o.email.toLowerCase().includes(q)) : s.owners
  }, [s.owners, query])
  const { pageRows, pager } = usePagination(rows, { resetKey: query })
  const lastOne = s.owners.length <= 1

  return (
    <>
      <div className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <SearchInput placeholder="Search by name or email" value={query} onChange={(e) => setQuery(e.target.value)} className="w-full max-w-sm" />
          {canManage && (
            <Button size="lg" onClick={() => setAdding(true)}>
              <UserPlus /> Add owner
            </Button>
          )}
        </div>
        <div className="overflow-hidden rounded-xl border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="text-[11px]">Person</TableHead>
                <TableHead className="text-[11px]">Status</TableHead>
                <TableHead className="w-12 text-[11px]" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {pageRows.length === 0 && (
                <TableRow>
                  <TableCell colSpan={3} className="py-8 text-center text-[12px] text-faint">
                    No owner matches that search.
                  </TableCell>
                </TableRow>
              )}
              {pageRows.map((o) => (
                <TableRow key={o.id}>
                  <TableCell>
                    <PersonRow name={o.name} email={o.email} trailing={o.id === me.user?.id ? <Pill>You</Pill> : undefined} />
                  </TableCell>
                  <TableCell>{o.status === 'pending' ? <Pill className="border-warning/30 text-warning">Invited</Pill> : <StatusBadge status={o.status} />}</TableCell>
                  <TableCell className="text-right">
                    {canManage && (
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={`Remove ${o.name || o.email} as an owner`}
                        title={lastOne ? 'A service always keeps at least one owner' : undefined}
                        disabled={lastOne}
                        onClick={() => setRemoving(o)}
                      >
                        <UserMinus />
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          <Pagination {...pager} className="border-t" />
        </div>
      </div>

      {adding && <AddOwnerDialog service={s} onClose={() => setAdding(false)} onAdded={onChanged} />}
      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={`Remove ${removing?.name || removing?.email} as an owner?`}
        body={`They stop publishing ${s.displayName}'s contracts and managing its clients. Their account and any tenant access are untouched.`}
        confirm="Remove owner"
        destructive
        onConfirm={async () => {
          if (!removing) return
          await registry.removeServiceOwner({ serviceId: s.id, userId: removing.id })
          toast.success('Owner removed')
          await onChanged()
        }}
      />
    </>
  )
}
