import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { Building2, Plus } from 'lucide-react'
import { TenantService } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useServerPagination } from '@/shared/ui/table/usePagination'
import { formatDate } from '@/shared/lib/datetime'
import { useMe } from '@/features/auth'
import { EmptyState, ErrorState, Field, LoadingState, SearchInput } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { Input } from '@/shared/ui/input'
import { PageHeader } from '@/shared/ui/page/PageHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { Pagination } from '@/shared/ui/table/Pagination'

const slugify = (name: string) =>
  name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 48)

function CreateTenantDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const tenants = useClient(TenantService)
  const navigate = useNavigate()
  const { me } = useMe()
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [slugTouched, setSlugTouched] = useState(false)
  const [owner, setOwner] = useState(me.user?.email ?? '')
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const res = await tenants.createTenant({ name, slug: slugTouched ? slug : '', ownerEmail: owner })
      toast.success(`${res.tenant!.name} created`)
      onOpenChange(false)
      navigate(`/tenants/${res.tenant!.id}`)
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
            <DialogTitle className="text-[15px]">Create a tenant</DialogTitle>
            <DialogDescription className="text-[13px]">An organisation every service shares. The owner becomes its first member.</DialogDescription>
          </DialogHeader>
          <Field label="Name" htmlFor="t-name">
            <Input
              id="t-name"
              value={name}
              onChange={(e) => {
                setName(e.target.value)
                if (!slugTouched) setSlug(slugify(e.target.value))
              }}
              required
              autoFocus
            />
          </Field>
          <Field label="Slug" htmlFor="t-slug" hint="Lowercase letters, digits and dashes. Used in addresses; can't be changed later.">
            <Input
              id="t-slug"
              value={slug}
              onChange={(e) => {
                setSlug(e.target.value)
                setSlugTouched(true)
              }}
              className="font-mono"
            />
          </Field>
          <Field label="Owner's email" htmlFor="t-owner" hint="Someone without an account is invited.">
            <Input id="t-owner" type="email" value={owner} onChange={(e) => setOwner(e.target.value)} required />
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !name || !owner}>
              {busy ? 'Creating…' : 'Create tenant'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export default function TenantsPage() {
  const { isAdmin } = useMe()
  const tenants = useClient(TenantService)
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [creating, setCreating] = useState(false)

  const list = useServerPagination(
    async (token, pageSize) => {
      const res = await tenants.listTenants({ query, pageSize, pageToken: token })
      return { items: res.tenants, next: res.nextPageToken }
    },
    [tenants, query],
  )

  return (
    <>
      <PageHeader
        title="Tenants"
        desc={
          isAdmin
            ? 'Organisations shared by every service. A tabletsgo workspace, a billing account and a payments merchant can all belong to one tenant.'
            : 'The organisations you belong to. Open one to see its members and its Resource Tree.'
        }
        action={
          isAdmin && (
            <Button size="lg" onClick={() => setCreating(true)}>
              <Plus /> Create tenant
            </Button>
          )
        }
      />
      <div className="mt-7 flex flex-col gap-4">
        <SearchInput placeholder="Search by name or slug" value={query} onChange={(e) => setQuery(e.target.value)} className="max-w-sm" />
        {list.error ? (
          <ErrorState message={list.error} onRetry={list.reload} />
        ) : list.loading && list.items.length === 0 ? (
          <LoadingState />
        ) : list.items.length === 0 ? (
          <EmptyState icon={Building2} title={query ? 'No tenant matches that search' : 'No tenants yet'}>
            {!query && (isAdmin ? 'Create the first one to group people and the resources they share.' : "You don't belong to any tenant yet.")}
          </EmptyState>
        ) : (
          <div className="overflow-hidden rounded-xl border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="text-[11px]">Tenant</TableHead>
                  <TableHead className="text-[11px]">Members</TableHead>
                  <TableHead className="text-[11px]">Region</TableHead>
                  <TableHead className="text-[11px]">Created</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {list.items.map((t) => (
                  <TableRow key={t.id} className="cursor-pointer" onClick={() => navigate(`/tenants/${t.id}`)}>
                    <TableCell>
                      <div className="text-[13px] font-semibold">{t.name}</div>
                      <div className="font-mono text-[12px] text-faint">{t.slug}</div>
                    </TableCell>
                    <TableCell className="text-[13px] tabular-nums">{t.memberCount}</TableCell>
                    <TableCell className="text-[12px] text-muted-foreground">{t.region}</TableCell>
                    <TableCell className="text-[12px] text-muted-foreground">{formatDate(t.createdAt)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            <Pagination {...list.pager} className="border-t" />
          </div>
        )}
      </div>
      {creating && <CreateTenantDialog open={creating} onOpenChange={setCreating} />}
    </>
  )
}
